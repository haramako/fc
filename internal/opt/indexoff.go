package opt

import (
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/types"
)

// foldIndexOffset は `a[i + k]` (k は定数、a はグローバルの要素 1 バイトの配列) の添字の加算を配列のアドレス側に移す:
//
//	add t = i, #k; load_mem d = a, t       →  load_mem d = a, i, disp=k     (codegen: lda a+k,y)
//	add t = i, #k; store_mem a, t, v       →  store_mem a, i, v, disp=k     (codegen: sta a+k,y)
//
// 添字が同じ i のままなので Y に置いた添字がそのまま使え、`clc; lda i; adc #k; tay` (約 10 サイクル) が消える
// (castle の OAM への `buf[idx+1]` … `buf[idx+3]` は手書き asm の `iny` 相当になる)。
// 前提: 添字の式 `i + k` は 8 ビットで折り返さない (i + k > 255 は範囲外の添字と同じ未定義。
// docs/reference/language.md の「添字と slice」)。折り返しを当てにした `[256]` のリングバッファは書けない。
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
			if use.Code.IsBlockBoundary() {
				m = len(ops) // 直線の範囲を出た
				continue
			}
			if use.IsMem() && use.Src[1] == ir.Operand(t) && use.Scale == 1 {
				if m := use.Mem(); m.BaseIsArray() && ir.ValType(m.Base).Base.Size == 1 {
					use.Src[1] = base
					use.Disp += k
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
