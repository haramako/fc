package opt

import (
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/types"
)

// fusePointer はアドレス計算とその直後の参照を 1 命令にまとめる (codegen が 1 つのアドレッシングモードで出せる形):
//
//	add t = p + #k; pget d = *t      → field_pget d = *(p + k)      ldy #k; lda (p),y
//	add t = p + #k; pset *t = v      → field_pset *(p + k) = v
//	index t = &a[i]; pget d = *t     → index_pget d = a[i]          lda a,y (a が配列) / ldy i; lda (p),y (a がポインタ)
//	index t = &a[i]; pset *t = v     → index_pset a[i] = v
//
// t はその場でしか使わない一時変数のときだけ。
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
			switch next.Code {
			case ir.OpPget:
				if isSameOperand(op.Dst, next.Src[0]) && k+ir.ValType(next.Dst).Size <= 256 {
					ops[i] = &ir.Op{Code: ir.OpFieldPget, Dst: next.Dst, Src: []ir.Operand{ptr, off}, Pos: next.Pos}
					ops[i+1] = nil
				}
			case ir.OpPset:
				if isSameOperand(op.Dst, next.Src[0]) && k+ir.ValType(op.Dst).Base.Size <= 256 {
					ops[i] = &ir.Op{Code: ir.OpFieldPset, Src: []ir.Operand{ptr, off, next.Src[1]}, Type: ir.ValType(op.Dst).Base, Pos: next.Pos}
					ops[i+1] = nil
				}
			}
		case ir.OpIndex:
			arr, idx := op.Src[0], op.Src[1]
			if es := ir.ValType(arr).Base.Size; es != 1 && es != 2 {
				continue // struct の配列 (要素サイズが 1・2 以外) は sym+i,y の形にできない
			}
			isArray := ir.ValKind(arr) == ir.KindGlobal && ir.ValType(arr).Kind == types.Array // グローバル配列: sym+i,y
			isPtr := ir.ValType(arr).Kind == types.Pointer                                     // ポインタ変数: ldy i; lda (p),y
			if (!isArray && !isPtr) ||
				ir.ValLocalType(op.Dst) != ir.LTTemp || // その変数をそこでしか使っていない
				ir.ValType(idx).Size != 1 { // インデックスのサイズが 1 バイト
				continue
			}
			switch next.Code {
			case ir.OpPget:
				if isSameOperand(op.Dst, next.Src[0]) {
					ops[i] = &ir.Op{Code: ir.OpIndexPget, Dst: next.Dst, Src: []ir.Operand{arr, idx}, Pos: next.Pos}
					ops[i+1] = nil
				}
			case ir.OpPset:
				// 書く幅は要素の大きさになるので、ポインタが要素の型 (struct の先頭フィールドへの cast ではない) のときだけ
				// (`sa[i].f0 = 4` (S は 2 バイト) が隣のフィールドまで書いていた。fuzz で発覚)
				if isSameOperand(op.Dst, next.Src[0]) && ir.ValType(next.Src[0]).Base.Size == ir.ValType(arr).Base.Size {
					ops[i] = &ir.Op{Code: ir.OpIndexPset, Src: []ir.Operand{arr, idx, next.Src[1]}, Pos: next.Pos}
					ops[i+1] = nil
				}
			}
		}
	}
}
