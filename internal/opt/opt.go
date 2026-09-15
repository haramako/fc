// Package opt は IR → IR の最適化 (レジスタ割付の前に走る)。
//
// パスは ir.CFG / ir.UseDef の上に書く。命令の削除は lmd.Ops の要素を nil にし、最後に Compact で詰める。
// 効果は bench/ (go test ./bench) で測る。
package opt

import (
	"github.com/haramako/fc/internal/ir"
)

// Optimize は関数 1 つの IR を書き換える。level 0 なら何もしない。
func Optimize(lmd *ir.Lambda, level int) {
	if level <= 0 || len(lmd.Ops) == 0 {
		return
	}
	fusePointer(lmd)
	compact(lmd)
}

// compact は削除済み (nil) の命令を取り除く。
func compact(lmd *ir.Lambda) {
	ops := lmd.Ops[:0]
	for _, op := range lmd.Ops {
		if op != nil {
			ops = append(ops, op)
		}
	}
	lmd.Ops = ops
}

// isSameOperand は同じ値を指すか (ir.CastedValue は元の値で比べる)。
func isSameOperand(a, b ir.Operand) bool {
	ua, ub := ir.UnderlyingValue(a), ir.UnderlyingValue(b)
	if ua != nil && ub != nil {
		return ua == ub
	}
	return a == b
}
