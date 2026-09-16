package opt

import "github.com/haramako/fc/internal/ir"

// commuteTemp は可換な演算の第 2 入力が「直前の命令が定義した一時変数」なら第 1 入力と入れ替える:
//
//	index_pget t = tab[i]; add d = y, t   (codegen は lda tab,y; sta t; clc; lda y; adc t)
//	→ index_pget t = tab[i]; add d = t, y (t が A に割り付き lda tab,y; clc; adc y)
//
// regalloc.allocateA が「定義の直後に第 1 入力として使う」一時変数だけを A に置くため。
func commuteTemp(lmd *ir.Lambda) {
	for i := 1; i < len(lmd.Ops); i++ {
		op, prev := lmd.Ops[i], lmd.Ops[i-1]
		if op == nil || prev == nil || prev.Dst == nil || len(op.Src) != 2 {
			continue
		}
		switch op.Code {
		case ir.OpAdd, ir.OpAnd, ir.OpOr, ir.OpXor, ir.OpEq:
		default:
			continue
		}
		t, ok := op.Src[1].(*ir.Value)
		if !ok || t.Kind != ir.KindLocal || t.LocalType != ir.LTTemp || t != ir.UnderlyingValue(prev.Dst) {
			continue
		}
		if ir.UnderlyingValue(op.Src[0]) == t || ir.ValType(op.Src[0]).Size != ir.ValType(op.Src[1]).Size {
			continue
		}
		op.Src[0], op.Src[1] = op.Src[1], op.Src[0]
	}
}
