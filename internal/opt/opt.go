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
	// 各パスは FC_DISABLE=名前 で切れる (ir.Disabled。調査用)
	if !ir.Disabled("ssa") {
		propagateSSA(lmd)
	}
	if !ir.Disabled("mul") {
		expandMul(lmd)
	}
	if !ir.Disabled("sink") {
		sinkAddress(lmd)
	}
	if !ir.Disabled("fuse") {
		fusePointer(lmd)
	}
	compact(lmd)
	if !ir.Disabled("indexoff") && foldIndexOffset(lmd) {
		compact(lmd) // fuse が index + pget / pset を index_pget / index_pset にした後
	}
	if !ir.Disabled("coalesce") {
		coalesceCopies(lmd)
	}
	compact(lmd)
	if !ir.Disabled("chain") {
		chainInPlace(lmd)
	}
	if !ir.Disabled("ssa") && !ir.Disabled("induction") && eliminateInduction(lmd) {
		// coalesce の後 (`i += s` が `add i = i, s` になってから)。消したカウンタの加算と初期化、lim の計算の定数を畳む
		propagateSSA(lmd)
		compact(lmd)
	}
	if !ir.Disabled("ssa") && !ir.Disabled("unroll") && unrollLoops(lmd) {
		propagateSSA(lmd) // 写しごとのカウンタとヘッダの検査を畳む
		compact(lmd)
	}
	if !ir.Disabled("narrow") {
		narrowBitTest(lmd, u)
	}
	if !ir.Disabled("scale") {
		scaleIndex(lmd, u)
	}
	if !ir.Disabled("commute") {
		commuteTemp(lmd)
	}
	for n := 0; n < 20 && !ir.Disabled("carry"); n++ {
		before := len(lmd.Ops)
		carryBranch(lmd)
		compact(lmd)
		if len(lmd.Ops) == before {
			break
		}
	}
	if !ir.Disabled("split") {
		splitWords(lmd, u)
	}
	compact(lmd)
	simplifyJumps(lmd)
	compact(lmd)
}

// newLabel は関数内で使われていないラベル名 (@name_N。N は既存のラベル番号の最大 + 1)。
func newLabel(lmd *ir.Lambda, name string) string {
	return fmt.Sprintf("@%s_%d", name, maxLabelNumber(lmd)+1)
}

// maxLabelNumber は関数内のラベルの末尾の番号 (`_N`) の最大 (無ければ 0)。
func maxLabelNumber(lmd *ir.Lambda) int {
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
	return n
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
