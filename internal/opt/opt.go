// Package opt は IR → IR の最適化 (レジスタ割付の前に走る)。
//
// パスは ir.CFG / ir.UseDef の上に書く。命令の削除は lmd.Ops の要素を nil にし、最後に Compact で詰める。
// 効果は bench/ (go test ./bench) で測る。
package opt

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/types"
)

// Optimize は関数 1 つの IR を書き換える。level 0 なら何もしない。
func Optimize(lmd *ir.Lambda, level int, u *types.Universe) {
	if level <= 0 || len(lmd.Ops) == 0 {
		return
	}
	sinkAddress(lmd)
	fusePointer(lmd)
	compact(lmd)
	coalesceCopies(lmd)
	compact(lmd)
	chainInPlace(lmd)
	narrowBitTest(lmd, u)
	simplifyJumps(lmd)
	compact(lmd)
}

// newLabel は関数内で使われていないラベル名 (@name_N。N は既存のラベル番号の最大 + 1)。
func newLabel(lmd *ir.Lambda, name string) string {
	n := 0
	for _, op := range lmd.Ops {
		if op == nil || op.Code != ir.OpLabel {
			continue
		}
		if i := strings.LastIndex(op.Label, "_"); i >= 0 {
			if k, err := strconv.Atoi(op.Label[i+1:]); err == nil && k > n {
				n = k
			}
		}
	}
	return fmt.Sprintf("@%s_%d", name, n+1)
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
