package opt

import (
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/types"
)

// foldIndexOffset は `a[i + k]` (k は定数、a はグローバルの要素 1 バイトの配列) の添字の加算を配列のアドレス側に移す:
//
//	add t = i, #k; index_pget d = a, t     →  index_pget d = <k>a, i     (codegen: lda a+k,y)
//	add t = i, #k; index_pset a, t, v      →  index_pset <k>a, i, v      (codegen: sta a+k,y)
//
// 添字が同じ i のままなので Y に置いた添字がそのまま使え、`clc; lda i; adc #k; tay` (約 10 サイクル) が消える
// (castle の OAM への `buf[idx+1]` … `buf[idx+3]` は手書き asm の `iny` 相当になる)。
// 前提: 添字の式 `i + k` は 8 ビットで折り返さない (i + k > 255 は範囲外の添字と同じ未定義。
// doc/language_reference.md §6)。折り返しを当てにした `[256]` のリングバッファは書けない。
// 一時変数 t は定義が 1 つで、使用が index の添字だけのものを対象にし、定義から使用まで直線 (ラベル・分岐を挟まない)
// で i が書き換えられないこと。使用が全部置き換わったら add を消す。
func foldIndexOffset(lmd *ir.Lambda) bool {
	ops := lmd.Ops
	ndef := map[*ir.Value]int{}
	nuse := map[*ir.Value]int{}
	for _, op := range ops {
		if op == nil {
			continue
		}
		defs, uses := ir.DefUse(op)
		for _, d := range defs {
			ndef[ir.UnderlyingValue(d)]++
		}
		for _, u := range uses {
			if v := ir.UnderlyingValue(u); v != nil {
				nuse[v]++
			}
		}
	}
	changed := false
	for j, op := range ops {
		if op == nil || op.Code != ir.OpAdd {
			continue
		}
		t, ok := op.Dst.(*ir.Value)
		if !ok || t.Kind != ir.KindLocal || t.LocalType != ir.LTTemp || ndef[t] != 1 || !byteUnsigned(t.Type) {
			continue
		}
		base, k := op.Src[0], 0
		if n, lit := ir.ValIntLiteral(op.Src[1]); lit {
			k = n
		} else if n, lit := ir.ValIntLiteral(op.Src[0]); lit {
			base, k = op.Src[1], n
		} else {
			continue
		}
		if k <= 0 || k > 255 || !isPlainByte(base) {
			continue
		}
		bv := ir.UnderlyingValue(base)
		for m := j + 1; m < len(ops) && nuse[t] > 0; m++ {
			use := ops[m]
			if use == nil {
				continue
			}
			switch use.Code {
			case ir.OpLabel, ir.OpJump, ir.OpIf, ir.OpIfTrue, ir.OpIfCarry, ir.OpIfNotCarry, ir.OpSwitch, ir.OpReturn:
				m = len(ops) // 直線の範囲を出た
				continue
			}
			if (use.Code == ir.OpIndexPget || use.Code == ir.OpIndexPset) && use.In(1) == ir.Operand(t) && !use.Scaled {
				if arr, ok := use.In(0).(*ir.Value); ok && arr.Kind == ir.KindGlobal && arr.Type.Kind == types.Array && arr.Type.Base.Size == 1 {
					use.Src[0] = ir.NewCastedValue(arr, arr.Type, k)
					use.Src[1] = base
					nuse[t]--
					changed = true
				}
			}
			defs, _ := ir.DefUse(use)
			for _, d := range defs {
				if ir.UnderlyingValue(d) == bv {
					m = len(ops) // i が書き換わった: これより後の使用は置き換えない
				}
			}
		}
		if nuse[t] == 0 {
			ir.DropOp(ops, j)
		}
	}
	return changed
}

// byteUnsigned は 1 バイトの符号なし整数型か。
func byteUnsigned(t *types.Type) bool {
	return t.Kind == types.Int && t.Size == 1 && !t.Signed
}

// isPlainByte は添字にそのまま使える値か (変数か cast。1 バイト符号なし)。
func isPlainByte(o ir.Operand) bool {
	switch o.(type) {
	case *ir.Value, *ir.CastedValue:
		return byteUnsigned(ir.ValType(o)) && ir.ValOffset(o) == 0
	}
	return false
}
