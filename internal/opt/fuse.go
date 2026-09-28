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
func fusePointer(lmd *ir.Lambda) {
	ud := ir.BuildUseDef(lmd)
	ops := lmd.Ops
	// t が「この命令で定義され、直後の命令でだけ使われる」一時変数か
	onlyNext := func(dst ir.Operand, i int) bool {
		t := ir.UnderlyingValue(dst)
		if t == nil || t.LocalType != ir.LTTemp || len(ud.Defs[t]) != 1 {
			return false
		}
		u, ok := ud.SingleUse(t)
		return ok && u == i+1
	}
	for i, op := range ops {
		if op == nil || i+1 >= len(ops) || ops[i+1] == nil {
			continue
		}
		next := ops[i+1]
		switch op.Code {
		case ir.OpAdd:
			// struct のフィールドをポインタ経由で触る
			ptr, off := op.Src[0], op.Src[1]
			k, isLit := ir.ValIntLiteral(off)
			if !isLit || k < 0 || k > 255 || ir.ValType(ptr).Kind != types.Pointer || ir.ValType(ptr).Size != 2 || !onlyNext(op.Dst, i) {
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
			ir.MergeDrop(ops, i, i+1)
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
			ir.MergeDrop(ops, i, i+1)
		}
	}
	foldConstIndex(lmd)
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
			continue // 添字の式は 8 ビットで折り返さない前提 (doc/language_reference.md §6) なので 256 を超える形は作らない
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
