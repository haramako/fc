package opt

import (
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/types"
)

// foldFieldIndex は struct の配列 (グローバル、全体が 256 バイト以内) の要素のフィールドの読み書きを、添字の読み書きにする:
//
//	index t = &a[i]; field_pget d = *(t + k)     →  mul j = i, #s; index_pget d = <k>a, j (scaled)   lda a+k,y
//	index t = &a[i]; field_pset *(t + k) = v     →  mul j = i, #s; index_pset <k>a, j, v (scaled)    sta a+k,y
//	index t = &a[i]; pget d = *t / pset *t = v   →  (k = 0。先頭のフィールド・要素全体)
//
// s は要素の大きさ。j = i * s は mul の展開 (expandMul) でシフトと加算になる。配列全体が 256 バイト以内なら、範囲内の
// 添字 i で i * s + k + フィールドの大きさ は 256 以下なので 1 バイトの Y に収まる (範囲外の添字は未定義)。
// 今までは `&a[i]` を 16 ビットで組み立てて `sta (p),y` にしていた (要素 1 つで約 20 命令。読み出しは先頭のフィールド
// だけが fusePointer の index_pget になっていた)。SoA (`soa`) はフィールドごとの配列なので最初から `sta a_f,y`。
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
			switch op.Code {
			case ir.OpLabel, ir.OpJump, ir.OpIf, ir.OpIfTrue, ir.OpIfCarry, ir.OpIfNotCarry, ir.OpSwitch, ir.OpReturn,
				ir.OpCall, ir.OpFastcall, ir.OpAsm:
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
		if !ok || t.LocalType != ir.LTTemp || len(ud.Defs[t]) != 1 || len(ud.Uses[t]) == 0 {
			continue
		}
		// t の使用が全部「t (か先頭への cast) を通したフィールドの読み書き」なら置き換える
		type use struct {
			at    int
			off   int
			field *types.Type
		}
		var uses []use
		for _, k := range ud.Uses[t] {
			use0 := ops[k]
			if use0 == nil {
				uses = nil
				break
			}
			var off int
			var ft *types.Type
			switch use0.Code {
			case ir.OpFieldPget, ir.OpFieldPset:
				o, lit := ir.ValIntLiteral(use0.In(1))
				if use0.In(0) != ir.Operand(t) || !lit {
					uses = nil
					break
				}
				off = o
				if use0.Code == ir.OpFieldPget {
					ft = ir.ValType(use0.Dst)
				} else {
					ft = use0.Type
				}
			case ir.OpPget, ir.OpPset:
				p := use0.In(0)
				if ir.UnderlyingValue(p) != t || ir.ValOffset(p) != 0 || ir.ValType(p).Kind != types.Pointer {
					uses = nil
					break
				}
				ft = ir.ValType(p).Base // pset は書く幅がポインタの先の大きさ (codegen の pointerWrite と同じ)
				if use0.Code == ir.OpPget {
					ft = ir.ValType(use0.Dst)
				}
			default:
				uses = nil
			}
			if ft == nil || ft.Size <= 0 || off < 0 || off+ft.Size > es {
				uses = nil
				break
			}
			// ほかの使い方 (t を値として読む) が混ざっていないか: t は 1 つ目の入力だけ
			for m, src := range use0.Src {
				if m > 0 && ir.UnderlyingValue(src) == t {
					ft = nil
				}
			}
			if ft == nil {
				uses = nil
				break
			}
			uses = append(uses, use{at: k, off: off, field: ft})
		}
		if len(uses) != len(ud.Uses[t]) {
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
			ir.ReplaceOp(ops, n, &ir.Op{Code: ir.OpMul, Dst: jv, Src: []ir.Operand{asU8(idx, u8), ir.NewIntLiteral("", u8, es)}, Pos: op.Pos})
			if plain && iv.Kind == ir.KindLocal && !refered[iv] {
				shared[key] = jv
			}
		}
		for _, us := range uses {
			use0 := ops[us.at]
			fa := ir.NewCastedValue(arr, u.ArrayOf(us.field, (at.Size-us.off)/us.field.Size), us.off)
			var nop *ir.Op
			switch use0.Code {
			case ir.OpFieldPget, ir.OpPget:
				nop = &ir.Op{Code: ir.OpIndexPget, Dst: use0.Dst, Src: []ir.Operand{fa, j}, Pos: use0.Pos, Scaled: true}
			case ir.OpFieldPset:
				nop = &ir.Op{Code: ir.OpIndexPset, Src: []ir.Operand{fa, j, use0.In(2)}, Pos: use0.Pos, Scaled: true}
			case ir.OpPset:
				nop = &ir.Op{Code: ir.OpIndexPset, Src: []ir.Operand{fa, j, use0.In(1)}, Pos: use0.Pos, Scaled: true}
			}
			ir.ReplaceOp(ops, us.at, nop)
		}
		changed = true
	}
	if changed {
		expandMul(lmd) // mul j = i, #s をシフトと加算に
	}
	return changed
}

// asU8 は 1 バイトの添字を符号なしとして見る (i8 の添字も i * s はバイトの積で同じ)。
func asU8(o ir.Operand, u8 *types.Type) ir.Operand {
	if ir.ValType(o) == u8 {
		return o
	}
	return ir.NewCastedValue(o, u8, 0)
}
