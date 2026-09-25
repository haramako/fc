// Package opt は IR → IR の最適化 (レジスタ割付の前に走る)。
//
// パスは ir.CFG / ir.UseDef の上に書く。命令の削除は lmd.Ops の要素を nil にし、最後に Compact で詰める。
// 効果は bench/ (go test ./bench) で測る。
package opt

import (
	"fmt"
	"os"
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
	for _, p := range Passes(u) {
		prev := ir.SnapshotLogs(lmd) // @log の注釈を、消えた・動いた命令から付け替える (ir.KeepLogs)
		p.Run(lmd)
		ir.KeepLogs(lmd, prev)
		if os.Getenv("FC_TRACE_LOGS") != "" {
			var at []string
			for i, op := range lmd.Ops {
				if op != nil {
					for _, lp := range op.Logs {
						at = append(at, fmt.Sprintf("%d@%d(%s)", lp.ID, i, op.Code))
					}
				}
			}
			fmt.Fprintf(os.Stderr, "logs %s after %s: %v\n", lmd.Id, p.Name, at)
			if os.Getenv("FC_TRACE_LOGS") == lmd.Id {
				for i, op := range lmd.Ops {
					if op != nil {
						var ids []int
						for _, lp := range op.Logs {
							ids = append(ids, lp.ID)
						}
						var args []string
						for _, lp := range op.Logs {
							if want := os.Getenv("FC_TRACE_LOG_ID"); want != "" && fmt.Sprint(lp.ID) == want {
								for _, a := range lp.Args {
									args = append(args, a.Expr+"="+ir.OperandString(a.Val))
								}
							}
						}
						fmt.Fprintf(os.Stderr, "  %3d %v %s %v\n", i, ids, ir.DumpOp(op, nil), args)
					}
				}
			}
		}
	}
}

// Pass は Optimize の 1 段。Name は FC_DISABLE で切るときの名前 (ir.Disabled。調査用)。
// fuzz の失敗の切り分け (internal/driver の rpLocate) は、全関数に 1 段ずつ当てて IR をインタプリタで実行し、
// 出力が変わった段を探す。
type Pass struct {
	Name string
	Run  func(lmd *ir.Lambda)
}

// Passes は Optimize が順に当てる段。
func Passes(u *types.Universe) []Pass {
	return []Pass{
		{"ssa", func(lmd *ir.Lambda) {
			if !ir.Disabled("ssa") {
				propagateSSA(lmd)
			}
		}},
		{"mul", func(lmd *ir.Lambda) {
			if !ir.Disabled("mul") {
				expandMul(lmd)
			}
		}},
		{"sink", func(lmd *ir.Lambda) {
			if !ir.Disabled("sink") {
				sinkAddress(lmd)
			}
		}},
		{"fuse", func(lmd *ir.Lambda) {
			if !ir.Disabled("fuse") {
				fusePointer(lmd)
			}
			compact(lmd)
		}},
		{"indexoff", func(lmd *ir.Lambda) {
			if !ir.Disabled("indexoff") && foldIndexOffset(lmd) {
				compact(lmd) // fuse が index + pget / pset を index_pget / index_pset にした後
			}
		}},
		{"coalesce", func(lmd *ir.Lambda) {
			if !ir.Disabled("coalesce") {
				coalesceCopies(lmd)
			}
			compact(lmd)
		}},
		{"chain", func(lmd *ir.Lambda) {
			if !ir.Disabled("chain") {
				chainInPlace(lmd)
			}
		}},
		{"induction", func(lmd *ir.Lambda) {
			if !ir.Disabled("ssa") && !ir.Disabled("induction") && eliminateInduction(lmd) {
				// coalesce の後 (`i += s` が `add i = i, s` になってから)。消したカウンタの加算と初期化、lim の計算の定数を畳む
				propagateSSA(lmd)
				compact(lmd)
			}
		}},
		{"unroll", func(lmd *ir.Lambda) {
			if !ir.Disabled("ssa") && !ir.Disabled("unroll") && !lmd.NoGrow && unrollLoops(lmd) {
				propagateSSA(lmd) // 写しごとのカウンタとヘッダの検査を畳む
				compact(lmd)
			}
		}},
		{"narrow", func(lmd *ir.Lambda) {
			if !ir.Disabled("narrow") {
				narrowBitTest(lmd, u)
			}
		}},
		{"scale", func(lmd *ir.Lambda) {
			if !ir.Disabled("scale") {
				scaleIndex(lmd, u)
			}
		}},
		{"commute", func(lmd *ir.Lambda) {
			if !ir.Disabled("commute") {
				commuteTemp(lmd)
			}
		}},
		{"carry", func(lmd *ir.Lambda) {
			for n := 0; n < 20 && !ir.Disabled("carry"); n++ {
				before := len(lmd.Ops)
				carryBranch(lmd)
				compact(lmd)
				if len(lmd.Ops) == before {
					break
				}
			}
		}},
		{"split", func(lmd *ir.Lambda) {
			if !ir.Disabled("split") {
				splitWords(lmd, u)
			}
			compact(lmd)
		}},
		{"jumps", func(lmd *ir.Lambda) {
			simplifyJumps(lmd)
			compact(lmd)
		}},
		{"ywalk", func(lmd *ir.Lambda) {
			if walkPointerY(lmd, u) {
				compact(lmd)
			}
		}},
	}
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
