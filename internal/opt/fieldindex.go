package opt

import (
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/types"
)

// foldFieldIndex は struct の配列 (グローバル、全体が 256 バイト以内) の要素のフィールドの読み書きを、添字の読み書きにする:
//
//	index t = &a[i]; load_mem d = t, disp=k      →  mul j = i, #s; load_mem d = a, j, scale=1, disp=k     lda a+k,y
//	index t = &a[i]; store_mem t, v, disp=k      →  mul j = i, #s; store_mem a, j, v, scale=1, disp=k     sta a+k,y
//	(k = 0 なら先頭のフィールド・要素全体)
//	index t = &a[i]; load_mem d = t, x, scale=1   →  mul j = i, #s; add k = j, x; load_mem d = a, k, scale=1      2 次元の配列
//	                                                   (a[i][x]。定数の添字 i なら load_mem d = a, x, disp=i*s)
//
// s は要素の大きさ。j = i * s は strength (ループで進む i なら加算に) の後の mul の展開 (expandMul) でシフトと加算になる。配列全体が 256 バイト以内なら、範囲内の
// 添字 i で i * s + k + フィールドの大きさ は 256 以下なので 1 バイトの Y に収まる (範囲外の添字は未定義)。
// 今までは `&a[i]` を 16 ビットで組み立てて `sta (p),y` にしていた (要素 1 つで約 20 命令。読み出しは先頭のフィールド
// だけが fusePointer の 添字付きの load_mem になっていた)。SoA (`soa`) はフィールドごとの配列なので最初から `sta a_f,y`。
func foldFieldIndex(lmd *ir.Lambda, u *types.Universe) bool {
	ud := ir.BuildUseDef(lmd)
	ops := lmd.Ops
	u8 := u.IntType(1, false)
	changed := false
	// 同じ基本ブロックの中で、同じ添字の i * s を使い回す (`es[i].hp -= 1` の読みと書き)。i が書き換わるか
	// ブロックの切れ目・呼び出し (グローバルの添字が変わりうる) で捨てる
	type scaledKey struct {
		idx *ir.Value
		es  int
	}
	shared := map[scaledKey]*ir.Value{}
	before := map[int][]*ir.Op{}    // 命令の前に挿す命令 (2 次元の添字の加算。最後にまとめて挿す)
	refered := map[*ir.Value]bool{} // アドレスを取られた変数 (ポインタ経由で書き換わりうるので使い回さない。sinkAddress と同じ)
	for _, op := range ops {
		if op != nil && op.Code == ir.OpRef {
			refered[ir.UnderlyingValue(op.Src[0])] = true
		}
	}
	for n, op := range ops {
		if op == nil {
			continue
		}
		if op.Code != ir.OpIndex {
			switch {
			case op.Code.IsBlockBoundary() || op.Code.IsCall() || op.Code.IsOpaque():
				clear(shared)
			default:
				defs, _ := ir.DefUse(op)
				for _, d := range defs {
					if v := ir.UnderlyingValue(d); v != nil {
						for k := range shared {
							if k.idx == v {
								delete(shared, k)
							}
						}
					}
				}
			}
			continue
		}
		arr, idx := op.Src[0], op.Src[1]
		at := ir.ValType(arr)
		it := ir.ValType(idx)
		if at.Kind != types.Array || ir.ValKind(arr) != ir.KindGlobal || at.Size <= 0 || at.Size > 256 ||
			it.Kind != types.Int || it.Size != 1 || it.Enum != nil {
			continue
		}
		es := at.Base.Size
		t, ok := op.Dst.(*ir.Value)
		if !ok || t.LocalType != ir.LTTemp || ud.NumDefs(t) != 1 {
			continue
		}
		tuses := ud.Uses(t)
		if len(tuses) == 0 {
			continue
		}
		// t の使用が全部「t (か先頭への cast) を通した、添字の無い読み書き」なら置き換える
		type use struct {
			at    int
			off   int
			width int
			inner ir.Operand // 内側の添字 (2 次元の配列 a[i][x] の x。無ければ nil)
		}
		var uses []use
		for _, use0 := range tuses {
			k := lmd.IndexOf(use0)
			if !use0.IsMem() {
				uses = nil
				break
			}
			m := use0.Mem()
			p := m.Base
			if ir.UnderlyingValue(p) != t || ir.ValOffset(p) != 0 || ir.ValType(p).Kind != types.Pointer {
				uses = nil
				break
			}
			// 内側の添字は 1 バイトの要素の 1 バイトの添字だけ (要素の中の位置 x は 0〜s-1 なので i * s + x も 256 未満)
			if m.Index != nil && (m.Scale != 1 || ir.ValType(m.Index).Kind != types.Int || ir.ValType(m.Index).Size != 1 || lmd.Cfg().Disabled("index2d")) {
				uses = nil
				break
			}
			if m.Width <= 0 || m.Disp < 0 || m.Disp+m.Width > es {
				uses = nil
				break
			}
			// ほかの使い方 (t を値として読む) が混ざっていないか: t は番地だけ
			if use0.Code == ir.OpStoreMem && ir.UnderlyingValue(use0.MemValue()) == t {
				uses = nil
				break
			}
			uses = append(uses, use{at: k, off: m.Disp, width: m.Width, inner: m.Index})
		}
		if len(uses) != len(tuses) {
			continue
		}
		if k, lit := ir.ValIntLiteral(idx); lit && k >= 0 && k*es+es <= at.Size && !lmd.Cfg().Disabled("constidx") {
			// 定数の添字: 絶対番地 (`sta a+k*s+off`。foldConstIndex と同じ。fieldindex は fuse の後なので自分で畳む。
			// 入れ子の配列フィールド `ds[1].items[2].id` は fuseArrayField の後にここに来る)
			ir.DropOp(ops, n)
			for _, us := range uses {
				use0 := ops[us.at]
				var nop *ir.Op
				var x ir.Operand
				scale := 0
				if us.inner != nil {
					x, scale = asU8(us.inner, u8), 1
				}
				if use0.Code == ir.OpLoadMem {
					nop = ir.NewLoadMem(use0.Dst, arr, x, scale, k*es+us.off)
				} else {
					nop = ir.NewStoreMem(arr, x, scale, k*es+us.off, us.width, use0.MemValue())
				}
				nop.Pos = use0.Pos
				ir.ReplaceOp(ops, us.at, nop)
				ud.Add(nop)
			}
			changed = true
			continue
		}
		// j = i * s (s == 1 なら i のまま)
		var j ir.Operand = idx
		iv, plain := idx.(*ir.Value)
		key := scaledKey{iv, es}
		switch {
		case es == 1:
			ir.DropOp(ops, n)
		case plain && shared[key] != nil:
			j = shared[key]
			ir.DropOp(ops, n)
		default:
			jv := ir.NewLocal(t.Name+"*", u8, ir.LTNone) // 使い回すので定義 1 つ・使用 1 つの一時変数にしない
			lmd.Vars = append(lmd.Vars, jv)
			j = jv
			mul := &ir.Op{Code: ir.OpMul, Dst: jv, Src: []ir.Operand{asU8(idx, u8), ir.NewIntLiteral("", u8, es)}, Pos: op.Pos}
			ir.ReplaceOp(ops, n, mul)
			ud.Add(mul)
			if plain && iv.Kind == ir.KindLocal && !refered[iv] {
				shared[key] = jv
			}
		}
		for _, us := range uses {
			use0 := ops[us.at]
			jx := j
			if us.inner != nil { // 2 次元: k = j + x (使用の直前に。x はその時点の値)
				kv := ir.NewLocal(t.Name+"+", u8, ir.LTTemp)
				lmd.Vars = append(lmd.Vars, kv)
				add := &ir.Op{Code: ir.OpAdd, Dst: kv, Src: []ir.Operand{asU8(j, u8), asU8(us.inner, u8)}, Pos: use0.Pos}
				before[us.at] = append(before[us.at], add)
				jx = kv
			}
			var nop *ir.Op
			if use0.Code == ir.OpLoadMem {
				nop = ir.NewLoadMem(use0.Dst, arr, jx, 1, us.off)
			} else {
				nop = ir.NewStoreMem(arr, jx, 1, us.off, us.width, use0.MemValue())
			}
			nop.Pos = use0.Pos
			ir.ReplaceOp(ops, us.at, nop)
			ud.Add(nop)
		}
		changed = true
	}
	if len(before) > 0 {
		out := make([]*ir.Op, 0, len(ops)+len(before))
		for i, op := range ops {
			out = append(out, before[i]...)
			out = append(out, op)
		}
		lmd.Ops = out
	}
	return changed // mul j = i, #s のシフトと加算への展開は strength の後 (ループの中なら加算に置き換わる)
}

// asU8 は 1 バイトの添字を符号なしとして見る (i8 の添字も i * s はバイトの積で同じ)。
func asU8(o ir.Operand, u8 *types.Type) ir.Operand {
	if ir.ValType(o) == u8 {
		return o
	}
	return ir.NewCastedValue(o, u8, 0)
}
