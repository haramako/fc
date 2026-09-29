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

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/types"
)

// Optimize は関数 1 つの IR を書き換える。level 0 なら何もしない。段の並びは Passes。
func Optimize(lmd *ir.Lambda, level int, u *types.Universe) {
	if level <= 0 || len(lmd.Ops) == 0 {
		return
	}
	for _, p := range Passes() {
		p.Apply(lmd, u)
	}
}

// traceLogs は調査用 (FC_TRACE_LOGS=1 で段ごとの @log の位置、FC_TRACE_LOGS=<関数の Id> でその関数の IR も。
// FC_TRACE_LOG_ID=<ID> でその注釈の引数の値)。
func traceLogs(lmd *ir.Lambda, pass string, detail bool, wantID string) {
	var at []string
	for i, op := range lmd.Ops {
		if op != nil {
			for _, lp := range op.Logs {
				at = append(at, fmt.Sprintf("%d@%d(%s)", lp.ID, i, op.Code))
			}
		}
	}
	fmt.Fprintf(os.Stderr, "logs %s after %s: %v\n", lmd.Id, pass, at)
	if !detail {
		return
	}
	for i, op := range lmd.Ops {
		if op == nil {
			continue
		}
		var ids []int
		for _, lp := range op.Logs {
			ids = append(ids, lp.ID)
		}
		var args []string
		for _, lp := range op.Logs {
			if wantID != "" && fmt.Sprint(lp.ID) == wantID {
				for _, a := range lp.Args {
					args = append(args, a.Expr+"="+ir.OperandString(a.Val))
				}
			}
		}
		fmt.Fprintf(os.Stderr, "  %3d %v %s %v\n", i, ids, ir.DumpOp(op, nil), args)
	}
}

// Pass は Optimize の 1 段。Name は FC_DISABLE で切るときの名前 (ir.Config。調査用)。
// fuzz の失敗の切り分け (internal/driver の rpLocate) は、全関数に 1 段ずつ Apply して IR をインタプリタで実行し、
// 出力が変わった段を探す。
type Pass struct {
	Name     string
	Requires []string // この名前の段が切られていれば走らない (induction / unroll は ssa の上に書かれている)
	Grows    bool     // フレームを大きくする展開: NoGrow の関数では走らない
	// Run は 1 回の適用。変えたら true (Then を呼び、Repeat なら繰り返す)。消した命令 (nil) は Apply が詰める (compact)
	Run func(lmd *ir.Lambda, u *types.Universe) bool
	// Then は Run が変えたときの後処理 (畳み込みのやり直しなど)
	Then func(lmd *ir.Lambda, u *types.Universe)
	// Repeat は変わる限り繰り返す回数の上限 (0 なら 1 回)
	Repeat int
}

// Apply は段を 1 つ当てる (切られていれば何もしない)。@log の注釈の付け替え、compact、トレース、IR の検証 (FC_VERIFY_IR)
// もここで行う。戻り値は変えたか。
func (p Pass) Apply(lmd *ir.Lambda, u *types.Universe) bool {
	cfg := lmd.Cfg()
	if cfg.Disabled(p.Name) || (p.Grows && lmd.NoGrow) {
		return false
	}
	for _, r := range p.Requires {
		if cfg.Disabled(r) {
			return false
		}
	}
	prev := ir.SnapshotLogs(lmd) // @log の注釈を、消えた・動いた命令から付け替える (ir.KeepLogs)
	changed := false
	for k := 0; k <= p.Repeat; k++ {
		c := p.Run(lmd, u)
		compact(lmd)
		if !c {
			break
		}
		changed = true
		if p.Then != nil {
			p.Then(lmd, u)
			compact(lmd)
		}
	}
	ir.KeepLogs(lmd, prev)
	if tr := cfg.Trace("logs"); tr != "" {
		traceLogs(lmd, p.Name, tr == lmd.Id, cfg.Trace("log_id"))
	}
	if cfg.VerifyIR() {
		if err := ir.Verify(lmd); err != nil {
			panic(&diag.Error{Msg: fmt.Sprintf("internal: IR verify after opt %s in %s: %v", p.Name, lmd.Id, err)})
		}
	}
	return changed
}

// always は変えたかを返さない段を Run の形にする (毎回 compact する。nil を残さない段では何も起きない)。
func always(f func(lmd *ir.Lambda)) func(*ir.Lambda, *types.Universe) bool {
	return func(lmd *ir.Lambda, _ *types.Universe) bool { f(lmd); return true }
}

func alwaysU(f func(lmd *ir.Lambda, u *types.Universe)) func(*ir.Lambda, *types.Universe) bool {
	return func(lmd *ir.Lambda, u *types.Universe) bool { f(lmd, u); return true }
}

func changes(f func(lmd *ir.Lambda) bool) func(*ir.Lambda, *types.Universe) bool {
	return func(lmd *ir.Lambda, _ *types.Universe) bool { return f(lmd) }
}

// Passes は Optimize が順に当てる段。順序の依存: sink は fuse の前 (アドレス計算を使用位置に寄せてから融合)、induction は
// coalesce の後 (`i += s` が `add i = i, s` になってから)、ywalk は jumps (ループの回転) の後。
func Passes() []Pass {
	return []Pass{
		{Name: "aggcopy", Run: changes(propagateAggregateCopies)}, // インライン展開の引数の slice の写し
		{Name: "aggbuild", Run: changes(assembleInPlace)},         // `s = s[n..]` の組み立ての一時の値
		{Name: "sliceargs", Run: changes(splitSliceArgs)},         // sema の形 (部分の load の直後の push_arg) のうちに
		{Name: "ssa", Run: always(propagateSSA)},
		{Name: "mul", Run: always(expandMul)},
		{Name: "sink", Run: always(sinkAddress)},
		{Name: "fuse", Run: alwaysU(fusePointer)},
		{Name: "fieldindex", Run: foldFieldIndex},
		{Name: "indexoff", Run: changes(foldIndexOffset)}, // fuse が index + load_mem / store_mem を添字付きの load_mem / store_mem にした後
		{Name: "coalesce", Run: always(coalesceCopies)},
		{Name: "chain", Run: always(chainInPlace)},
		{Name: "induction", Requires: []string{"ssa"}, Run: changes(eliminateInduction),
			// 消したカウンタの加算と初期化、lim の計算の定数を畳む
			Then: func(lmd *ir.Lambda, _ *types.Universe) { propagateSSA(lmd) }},
		{Name: "unroll", Requires: []string{"ssa"}, Grows: true, Run: changes(unrollLoops),
			Then: func(lmd *ir.Lambda, u *types.Universe) {
				propagateSSA(lmd) // 写しごとのカウンタとヘッダの検査を畳む
				compact(lmd)
				if !lmd.Cfg().Disabled("fuse") {
					fusePointer(lmd, u) // 添字が定数になった index + load_mem / store_mem (要素 2 バイトのポインタは定数の添字だけ融合できる)
				}
			}},
		{Name: "narrow", Run: alwaysU(narrowBitTest)},
		{Name: "scale", Run: alwaysU(scaleIndex)},
		{Name: "commute", Run: always(commuteTemp)},
		{Name: "avg", Run: averageBytes}, // 1 バイトどうしの平均を adc + ror a に (split の前に)
		{Name: "carry", Repeat: 19, Run: func(lmd *ir.Lambda, _ *types.Universe) bool {
			before := len(lmd.Ops)
			carryBranch(lmd)
			compact(lmd)
			return len(lmd.Ops) != before
		}},
		{Name: "split", Run: alwaysU(splitWords)},
		{Name: "jumps", Run: always(simplifyJumps)},
		{Name: "ywalk", Run: func(lmd *ir.Lambda, u *types.Universe) bool { return walkPointerY(lmd, u) }},
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

// ZeroEmptyCasts は元の値のどのバイトも読まない cast (Width 0: `cast<u8, 1>(t)` で t が 1 バイト。上位のゼロ拡張だけ) の
// 入力を 0 のリテラルにする。byte はそのバイトを #0 と読むが、値がレジスタにあるときの経路 (loadA の A そのまま、tay、
// cmp #0 など) はオフセットを見ずに値そのものを使っていた (`(f() as u16) & 0x3c00` が f() の下位で判定。fuzz で発覚)。
func ZeroEmptyCasts(lmd *ir.Lambda) {
	for _, op := range lmd.Ops {
		if op == nil {
			continue
		}
		for i, src := range op.Src {
			if cv, ok := src.(*ir.CastedValue); ok && cv.Width == 0 {
				if _, isVal := cv.From.(*ir.Value); isVal {
					op.Src[i] = ir.NewIntLiteral("", cv.Type, 0)
				}
			}
		}
	}
}
