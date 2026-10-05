package opt

import (
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/types"
)

// fusePointer はアドレス計算とその直後の参照を 1 命令にまとめる (codegen が 1 つのアドレッシングモードで出せる形):
//
//	add t = p + #k; load_mem d = t       → load_mem d = p, disp=k        ldy #k; lda (p),y
//	add t = p + #k; store_mem t, v       → store_mem p, v, disp=k
//	index t = &a[i]; load_mem d = t      → load_mem d = a, i, scale=es    lda a,y (a が配列) / ldy i; lda (p),y (a がポインタ)
//	index t = &a[i]; store_mem t, v      → store_mem a, i, v, scale=es
//
// t はその場でしか使わない一時変数のときだけ (添字もずれも持たない参照だけ)。
func fusePointer(lmd *ir.Lambda, u *types.Universe) {
	ud := ir.BuildUseDef(lmd)
	ops := lmd.Ops
	// t が「この命令で定義され、直後の命令でだけ使われる」一時変数か
	onlyNext := func(dst ir.Operand, next *ir.Op) bool {
		t := ir.UnderlyingValue(dst)
		if t == nil || t.LocalType != ir.LTTemp || ud.NumDefs(t) != 1 {
			return false
		}
		u, ok := ud.SingleUse(t)
		return ok && u == next
	}
	for i, op := range ops {
		if op == nil {
			continue
		}
		ni := ir.NextOp(ops, i)
		if ni < 0 {
			continue
		}
		next := ops[ni]
		switch op.Code {
		case ir.OpAdd:
			// struct のフィールドをポインタ経由で触る
			ptr, off := op.Src[0], op.Src[1]
			k, isLit := ir.ValIntLiteral(off)
			if !isLit || k < 0 || k > 255 || ir.ValType(ptr).Kind != types.Pointer || ir.ValType(ptr).Size != 2 || !onlyNext(op.Dst, next) {
				continue
			}
			if !fusableDeref(next, op.Dst) {
				continue
			}
			m := next.Mem()
			if k+m.Width > 256 {
				continue
			}
			var nop *ir.Op
			if next.Code == ir.OpLoadMem {
				nop = ir.NewLoadMem(next.Dst, ptr, nil, 0, k)
			} else {
				nop = ir.NewStoreMem(ptr, nil, 0, k, m.Width, next.MemValue())
			}
			nop.Pos = next.Pos
			ir.ReplaceOp(ops, i, nop)
			ir.MergeDrop(ops, i, ni)
			ud.Add(nop)
		case ir.OpIndex:
			arr, idx := op.Src[0], op.Src[1]
			es := ir.ValType(arr).Base.Size
			if es != 1 && es != 2 {
				continue // struct の配列 (要素サイズが 1・2 以外) は sym+i,y の形にできない
			}
			// 要素が 2 バイトのときは Y = i * 2 が 1 バイトに収まる (配列全体が 256 バイト以内) ときだけ。ポインタは長さが
			// 分からないので要素 1 バイトだけ (`a[150]` (a:[200]u16) が a[22] を読み書きしていた。survey 2026-09-27)
			isArray := ir.ValKind(arr) == ir.KindGlobal && ir.ValType(arr).Kind == types.Array && (es == 1 || ir.ValType(arr).Size <= 256) // sym+i,y
			k, lit := ir.ValIntLiteral(idx)
			isPtr := ir.ValType(arr).Kind == types.Pointer && (es == 1 || lit && k >= 0 && k*es+es <= 256) // ldy i; lda (p),y (定数の添字なら要素 2 バイトも)
			if (!isArray && !isPtr) ||
				ir.ValLocalType(op.Dst) != ir.LTTemp || // その変数をそこでしか使っていない
				ir.ValType(idx).Size != 1 { // インデックスのサイズが 1 バイト
				continue
			}
			if !fusableDeref(next, op.Dst) {
				continue
			}
			var nop *ir.Op
			if next.Code == ir.OpLoadMem {
				nop = ir.NewLoadMem(next.Dst, arr, idx, es, 0)
			} else {
				// 書く幅は要素の大きさになるので、ポインタが要素の型 (struct の先頭フィールドへの cast ではない) のときだけ
				// (`sa[i].f0 = 4` (S は 2 バイト) が隣のフィールドまで書いていた。fuzz で発覚)
				if next.Width != es {
					continue
				}
				nop = ir.NewStoreMem(arr, idx, es, 0, es, next.MemValue())
			}
			nop.Pos = next.Pos
			ir.ReplaceOp(ops, i, nop)
			ir.MergeDrop(ops, i, ni)
			ud.Add(nop)
		}
	}
	fuseArrayField(lmd, u)
	foldConstIndex(lmd)
}

// fuseArrayField は struct の配列フィールドをポインタ経由で引く参照を、ポインタ + 添字 + ずれの 1 命令にする:
//
//	add t1 = p, #k               t1 は *[L]U (配列フィールドの番地)
//	index t2 = <*U>t1, i         配列の添字
//	load_mem d = t2, disp=m      → load_mem d = p, i, scale=sizeof(U), disp=k+m     lda i; (asl); clc; adc #k+m; tay; lda (p),y
//
// 添字は配列の長さ L 未満 (範囲外の添字と、配列の外へのポインタ演算は未定義。docs/reference/language.md の「ポインタ」) なので、
// Y = i * sizeof(U) + k + m に読む幅を足しても k + (配列全体のバイト数) を超えない。それが 256 以下のときだけ。以前は
// add と index で 16 ビットの番地を組み立てて `ldy #m; lda (t2),y` だった (`pt.arr[j]`、`p.items[i].price`)。
//   - U が 3 バイト以上なら添字をバイト単位にする (`mul j = i, #s` を index の位置に。fieldindex と同じく expandMul が
//     シフトと加算にする)。
//   - fusePointer が先に `load_mem d = <*U>t1, i, scale=s` にした形 (要素 1 バイト、定数の添字) も、add を畳む。
//   - t1 が add で作られていない (ずれ 0 の配列フィールド、*[L]U のポインタ変数) か add を動かせないときは、t1 を Base に
//     する (k = 0)。add と index の入力が参照までに書き換わらないことは sinkAddress と同じ条件 (canSink) で見る。
//
// グローバルの配列は fieldindex が `lda a+k,y` にするので対象にしない (Base が配列フィールドのポインタのときだけ)。
func fuseArrayField(lmd *ir.Lambda, u *types.Universe) {
	if lmd.Cfg().Disabled("fieldptr") {
		return
	}
	ud := ir.BuildUseDef(lmd)
	ops := lmd.Ops
	refered := map[*ir.Value]bool{}
	for _, op := range ops {
		if op != nil && op.Code == ir.OpRef {
			refered[ir.UnderlyingValue(op.Src[0])] = true
		}
	}
	// arrayField は o が配列フィールドの番地 t1 (*[L]U) を要素のポインタ (*U) に読み替えたもの (配列の添字の元) なら t1 と U の大きさ、
	// 配列全体の大きさを返す。
	arrayField := func(o ir.Operand) (*ir.Value, int, int, bool) {
		cv, ok := o.(*ir.CastedValue)
		if !ok || cv.Offset != 0 || cv.Type.Kind != types.Pointer || cv.Type.Base == nil {
			return nil, 0, 0, false
		}
		t1, ok := cv.From.(*ir.Value)
		if !ok || t1.Type.Kind != types.Pointer || t1.Type.Size != 2 || t1.Type.Base == nil || t1.Type.Base.Kind != types.Array ||
			t1.Type.Base.Base != cv.Type.Base || cv.Type.Base.Size <= 0 || t1.Type.Base.Size <= 0 {
			return nil, 0, 0, false
		}
		return t1, cv.Type.Base.Size, t1.Type.Base.Size, true
	}
	// foldAdd は t1 が `add t1 = p, #k` (p はポインタ) で、その使用が at だけ、p が read (畳んだ命令が p を読む位置) までに
	// 書き換わらないなら p, k とその add の位置
	changed := false
	foldAdd := func(t1 *ir.Value, at, read, size int) (ir.Operand, int, int, bool) {
		if u1, single := ud.SingleUse(t1); !single || u1 != ops[at] || t1.LocalType != ir.LTTemp || ud.NumDefs(t1) != 1 {
			return nil, 0, 0, false
		}
		add, _ := ud.SingleDef(t1)
		a := lmd.IndexOf(add)
		if add.Code != ir.OpAdd || a >= at || ir.UnderlyingValue(add.Dst) != t1 {
			return nil, 0, 0, false
		}
		k, lit := ir.ValIntLiteral(add.Src[1])
		p := add.Src[0]
		if !lit || k < 0 || k+size > 256 || ir.ValType(p).Kind != types.Pointer || ir.ValType(p).Size != 2 || isVolatile(p) ||
			!canSink(add, ops[a+1:read], refered) {
			return nil, 0, 0, false
		}
		return p, k, a, true
	}
	for i, op := range ops {
		if op == nil {
			continue
		}
		// fusePointer が融合済みの `load_mem d = <*U>t1, i, scale=s` (要素 1 バイト・定数の添字): add を畳むだけ
		if op.IsMem() && op.Src[1] != ir.Operand(ir.NoIndex) {
			m := op.Mem()
			t1, es, size, ok := arrayField(m.Base)
			if !ok || m.Scale != es || m.Disp+m.Width > es {
				continue
			}
			if p, k, a, ok := foldAdd(t1, i, i, size); ok {
				op.Src[0] = p
				op.Disp += k
				ud.Update(op)
				ir.DropOp(ops, a)
				changed = true
			}
			continue
		}
		if op.Code != ir.OpIndex {
			continue
		}
		t1, es, size, ok := arrayField(op.Src[0])
		idx := op.Src[1]
		t2, isV := op.Dst.(*ir.Value)
		if !ok || size > 256 || ir.ValType(idx).Size != 1 || !isV || t2.LocalType != ir.LTTemp || ud.NumDefs(t2) != 1 {
			continue
		}
		use, single := ud.SingleUse(t2)
		if !single {
			continue
		}
		j := lmd.IndexOf(use)
		if j <= i || !use.IsMem() || ir.UnderlyingValue(use.Src[0]) != t2 || ir.ValOffset(use.Src[0]) != 0 ||
			use.Src[1] != ir.Operand(ir.NoIndex) {
			continue
		}
		m := use.Mem()
		if m.Disp < 0 || m.Disp+m.Width > es || use.Code == ir.OpStoreMem && ir.UnderlyingValue(use.MemValue()) == t2 {
			continue // 要素の中に収まる読み書き (フィールド) だけ
		}
		if !canSink(op, ops[i+1:j], refered) || isVolatile(idx) {
			continue // 添字 (と t1) が参照までに書き換わる / 読む時点を動かせない (I/O レジスタ・割り込みが書く変数)
		}
		var base ir.Operand = op.Src[0]
		k := 0
		if p, kk, a, ok := foldAdd(t1, i, j, size); ok {
			base, k = p, kk
			ir.DropOp(ops, a)
		}
		// 添字: 要素 1・2 バイトは scale で、それ以上はバイト単位の j = i * s を index の位置で作る (i は j までに変わらない)
		var j0 ir.Operand = idx
		scale := es
		if es > 2 {
			u8 := u.IntType(1, false)
			jv := ir.NewLocal(t2.Name+"*", u8, ir.LTTemp)
			lmd.Vars = append(lmd.Vars, jv)
			mul := &ir.Op{Code: ir.OpMul, Dst: jv, Src: []ir.Operand{asU8(idx, u8), ir.NewIntLiteral("", u8, es)}, Pos: op.Pos}
			ir.ReplaceOp(ops, i, mul)
			ud.Add(mul)
			j0, scale = jv, 1
		} else {
			ir.DropOp(ops, i) // 注釈は元の位置に残す (sinkAddress と同じ)
		}
		var nop *ir.Op
		if use.Code == ir.OpLoadMem {
			nop = ir.NewLoadMem(use.Dst, base, j0, scale, k+m.Disp)
		} else {
			nop = ir.NewStoreMem(base, j0, scale, k+m.Disp, m.Width, use.MemValue())
		}
		nop.Pos = use.Pos
		ir.ReplaceOp(ops, j, nop)
		ud.Add(nop)
		changed = true
	}
	if changed {
		expandMul(lmd) // mul j = i, #s をシフトと加算に
	}
}

// isVolatile は o の元の変数が volatile か (読む時点を動かせない)。
func isVolatile(o ir.Operand) bool {
	v := ir.UnderlyingValue(o)
	return v != nil && v.Volatile
}

// foldConstIndex は定数の添字を Disp に畳む (`a[3]` → `load_mem d = a, disp=3`): グローバルの配列なら codegen が絶対番地
// (`lda a+3`。`ldy #3; lda a,y` より 2 サイクル・2 バイト短く、Y を使わない) で、ポインタなら `ldy #k; lda (p),y` で読む
// (以前と同じ命令列)。展開したループ (unroll) の `a16[3]` や struct の配列の定数の要素 `gs[2].b` が対象。
func foldConstIndex(lmd *ir.Lambda) {
	if lmd.Cfg().Disabled("constidx") {
		return
	}
	for _, op := range lmd.Ops {
		if op == nil || !op.IsMem() {
			continue
		}
		m := op.Mem()
		if m.Index == nil {
			continue
		}
		k, lit := ir.ValIntLiteral(m.Index)
		if !lit || k < 0 || m.Disp+k*m.Scale+m.Width > 256 {
			continue // 添字の式は 8 ビットで折り返さない前提 (docs/reference/language.md の「添字と slice」) なので 256 を超える形は作らない
		}
		op.Disp += k * m.Scale
		op.Src[1] = ir.NoIndex
		op.Scale = 0
	}
}

// plainDeref は op が t そのもの (cast も無し) を番地にした、添字もずれも無い load_mem / store_mem か (sink / devirt / ywalk が
// 動かす・書き換える対象)。
func plainDeref(op *ir.Op, t ir.Operand) bool {
	return op.IsMem() && op.Src[0] == t && op.Src[1] == ir.Operand(ir.NoIndex) && op.Disp == 0
}

// fusableDeref は op が t (先頭への cast でもよい) を番地にした、添字もずれも無い load_mem / store_mem か (fusePointer の対象)。
func fusableDeref(op *ir.Op, t ir.Operand) bool {
	return op.IsMem() && isSameOperand(op.Src[0], t) && op.Src[1] == ir.Operand(ir.NoIndex) && op.Disp == 0
}
