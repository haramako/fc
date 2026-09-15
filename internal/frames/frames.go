// Package frames はフレームの静的割付 (doc/v2_frame_alloc.md §6)。
//
// Analyze: 全モジュールの呼び出しグラフから各関数の呼び出し規約 (ir.ABI) を決める (sema の後、regalloc の前)。
// Place: regalloc でフレームの大きさが決まった後、static な関数のフレームを固定アドレスに配置し、
// 全モジュールが include する `_frames.inc` の行を作る。
package frames

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/types"
)

// Graph は呼び出しグラフと ABI の決定結果。
type Graph struct {
	Lambdas []*ir.Lambda          // 本体を持つ関数 (プログラム順)
	ByID    map[string]*ir.Lambda // Id (アセンブラシンボル) → 関数 (extern も含む)
	index   map[*ir.Lambda]int    // Lambdas の添字
	callees [][]int               // 直接呼び出しの辺 (Lambdas の添字)
	depth   []int                 // 根からの最長距離 (static の部分グラフ上)
}

// Analyze は各関数の ABI を決める。
//
//   - extern: 型が fastcall なら ABIFastcall、それ以外は ABIStack
//   - options(abi: "stack"): ABIStack
//   - 呼び出しグラフの閉路に属する (再帰): ABIStack。間接呼び出しは「アドレスを取られた関数」全部への辺とみなす
//   - それ以外: ABIStatic。アドレスを取られた関数 (関数ポインタ / const の表 / インラインアセンブラからの参照) と
//     options(interrupt: true) の関数は Entry (スタック経由で引数を受け取り、プロローグで自分のフレームに写す)
func Analyze(mods []*ir.Module) (*Graph, error) {
	g := &Graph{ByID: map[string]*ir.Lambda{}, index: map[*ir.Lambda]int{}}
	var externs []*ir.Lambda
	for _, m := range mods {
		for _, d := range m.Defs {
			if d.Kind != ir.DefCode {
				continue
			}
			lmd := d.Lambda
			g.ByID[lmd.Id] = lmd
			if lmd.Extern {
				externs = append(externs, lmd)
				continue
			}
			g.index[lmd] = len(g.Lambdas)
			g.Lambdas = append(g.Lambdas, lmd)
		}
	}
	for _, lmd := range externs {
		if lmd.Type.Fastcall() {
			lmd.ABI = ir.ABIFastcall
		} else {
			lmd.ABI = ir.ABIStack
		}
	}
	n := len(g.Lambdas)

	// アドレスを取られた関数 (Entry) と辺
	entry := make([]bool, n)
	var indirect [][]*types.Type // 関数ごとの、間接呼び出しの関数型 (辺は同じ型の Entry にだけ張る)
	indirect = make([][]*types.Type, n)
	g.callees = make([][]int, n)
	markSym := func(sym string) {
		if l, ok := g.ByID[sym]; ok {
			if i, ok := g.index[l]; ok {
				entry[i] = true
			}
		}
	}
	markOperand := func(o ir.Operand) {
		if o == nil {
			return
		}
		v := ir.ValLiteral(o)
		if v != nil && v.Kind == ir.KindLiteral && !v.IsInt && v.Symbol != "" {
			markSym(v.Symbol)
		}
	}
	for _, m := range mods {
		for _, d := range m.Defs {
			if d.Kind == ir.DefBlock {
				for _, e := range d.Elems {
					markOperand(e)
				}
			}
			if d.Kind == ir.DefEqu && d.Equ != nil {
				markOperand(d.Equ)
			}
		}
	}
	for i, lmd := range g.Lambdas {
		if lmd.Options.Has("interrupt") {
			lmd.Interrupt = true
			entry[i] = true
		}
		for _, d := range lmd.Defs {
			if d.Kind == ir.DefBlock {
				for _, e := range d.Elems {
					markOperand(e)
				}
			}
		}
		for _, op := range lmd.Ops {
			if op == nil {
				continue
			}
			switch op.Code {
			case ir.OpCall, ir.OpFastcall:
				if v := ir.ValLiteral(op.Src[0]); v != nil && v.Kind == ir.KindLiteral && v.Symbol != "" {
					if l, ok := g.ByID[v.Symbol]; ok {
						if j, ok := g.index[l]; ok {
							g.callees[i] = append(g.callees[i], j)
						}
					}
				} else {
					indirect[i] = append(indirect[i], ir.ValType(op.Src[0]))
				}
				for _, s := range op.Src[1:] {
					markOperand(s)
				}
			case ir.OpAsm:
				// インラインアセンブラが関数名を参照していれば、呼び出し規約が分からないので Entry 扱い
				for _, w := range reAsmSym.FindAllString(op.Text, -1) {
					markSym(w)
				}
			default:
				for _, s := range op.Src {
					markOperand(s)
				}
				markOperand(op.Dst)
			}
		}
	}
	var entries []int
	for i := range g.Lambdas {
		if entry[i] {
			entries = append(entries, i)
		}
	}
	for i := range g.Lambdas {
		for _, t := range indirect[i] {
			for _, j := range entries {
				if g.Lambdas[j].Type == t {
					g.callees[i] = append(g.callees[i], j)
				}
			}
		}
	}

	// 再帰 (閉路) の検出: Tarjan の SCC
	inCycle := tarjanCycles(n, g.callees)
	for i, lmd := range g.Lambdas {
		switch {
		case lmd.Options.Has("abi") && optText(lmd.Options, "abi") == "stack":
			lmd.ABI = ir.ABIStack
		case inCycle[i]:
			lmd.ABI = ir.ABIStack
		default:
			lmd.ABI = ir.ABIStatic
			lmd.Entry = entry[i]
		}
	}

	// 割り込みから届く関数は全部 static でなければならない (X が何を指すか分からない)
	for i, lmd := range g.Lambdas {
		if !lmd.Interrupt {
			continue
		}
		if lmd.ABI != ir.ABIStatic {
			return nil, &diag.Error{Msg: fmt.Sprintf("%s: interrupt function must not be recursive", lmd.Id), Pos: lmd.Pos}
		}
		for _, j := range g.reachable(i) {
			if g.Lambdas[j].ABI != ir.ABIStatic {
				return nil, &diag.Error{Msg: fmt.Sprintf("%s: interrupt function reaches %s which uses the stack (recursive or options(abi: \"stack\"))",
					lmd.Id, g.Lambdas[j].Id), Pos: lmd.Pos}
			}
		}
	}

	// 深さ: 根 (呼び出し元の無い関数) からの最長距離。閉路は stack なので static の間では DAG
	g.depth = make([]int, n)
	indeg := make([]int, n)
	for i := range g.Lambdas {
		for _, j := range g.callees[i] {
			if j != i {
				indeg[j]++
			}
		}
	}
	var queue []int
	for i := range g.Lambdas {
		if indeg[i] == 0 {
			queue = append(queue, i)
		}
	}
	for len(queue) > 0 {
		i := queue[0]
		queue = queue[1:]
		for _, j := range g.callees[i] {
			if j == i {
				continue
			}
			if g.depth[i]+1 > g.depth[j] {
				g.depth[j] = g.depth[i] + 1
			}
			indeg[j]--
			if indeg[j] == 0 {
				queue = append(queue, j)
			}
		}
	}
	return g, nil
}

var reAsmSym = regexp.MustCompile(`_[A-Za-z0-9_$]+`)

func optText(o ir.Options, key string) string {
	v, _ := o.Get(key)
	return v.Text()
}

// reachable は i から (0 回以上の辺で) 届く関数の添字 (i 自身は閉路があるときだけ含む)。
func (g *Graph) reachable(i int) []int {
	seen := make([]bool, len(g.Lambdas))
	var r []int
	stack := append([]int{}, g.callees[i]...)
	for len(stack) > 0 {
		j := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if seen[j] {
			continue
		}
		seen[j] = true
		r = append(r, j)
		stack = append(stack, g.callees[j]...)
	}
	return r
}

// tarjanCycles は閉路に属する節 (自己ループを含む) を返す。
func tarjanCycles(n int, edges [][]int) []bool {
	index := make([]int, n)
	low := make([]int, n)
	onStack := make([]bool, n)
	for i := range index {
		index[i] = -1
	}
	var stack []int
	counter := 0
	inCycle := make([]bool, n)
	var strong func(v int)
	strong = func(v int) {
		index[v] = counter
		low[v] = counter
		counter++
		stack = append(stack, v)
		onStack[v] = true
		for _, w := range edges[v] {
			if index[w] < 0 {
				strong(w)
				low[v] = min(low[v], low[w])
			} else if onStack[w] {
				low[v] = min(low[v], index[w])
			}
		}
		if low[v] == index[v] {
			var scc []int
			for {
				w := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				onStack[w] = false
				scc = append(scc, w)
				if w == v {
					break
				}
			}
			self := false
			for _, w := range edges[v] {
				if w == v {
					self = true
				}
			}
			if len(scc) > 1 || self {
				for _, w := range scc {
					inCycle[w] = true
				}
			}
		}
	}
	for v := 0; v < n; v++ {
		if index[v] < 0 {
			strong(v)
		}
	}
	return inCycle
}

// Plan は配置の結果。
type Plan struct {
	ZpUsed, RamUsed int
	Inc             []string // _frames.inc の行
}

type placed struct {
	lmd   *ir.Lambda
	start int
	end   int
}

// Place は static な関数のフレームを配置する (regalloc で FrameSize が決まった後)。
//
// 同時に活性になりうる関数 (呼び出しグラフで一方から他方へ届く) のフレームは重ねない。割り込み関数の部分木は全部と重ねない。
// ZP (zpBudget バイト) には深さの深い順 (葉に近いほど呼ばれる回数が多い、という近似)、同じ深さならフレームの小さい順に
// 入るだけ入れ、残りは RAM (ramBudget)。options(zeropage: false) は RAM に置く。
func Place(g *Graph, zpBudget, ramBudget int) (*Plan, error) {
	n := len(g.Lambdas)
	// 到達関係 (推移閉包)
	reach := make([][]bool, n)
	for i := range g.Lambdas {
		reach[i] = make([]bool, n)
		for _, j := range g.reachable(i) {
			reach[i][j] = true
		}
	}
	irqTree := make([]bool, n) // 割り込みから届く (自身を含む)
	for i, lmd := range g.Lambdas {
		if lmd.Interrupt {
			irqTree[i] = true
			for _, j := range g.reachable(i) {
				irqTree[j] = true
			}
		}
	}
	conflict := func(a, b int) bool {
		return a == b || reach[a][b] || reach[b][a] || irqTree[a] || irqTree[b]
	}

	var order []int
	for i, lmd := range g.Lambdas {
		if lmd.ABI == ir.ABIStatic {
			order = append(order, i)
		}
	}
	sort.SliceStable(order, func(x, y int) bool {
		a, b := order[x], order[y]
		if g.depth[a] != g.depth[b] {
			return g.depth[a] > g.depth[b]
		}
		if g.Lambdas[a].FrameSize != g.Lambdas[b].FrameSize {
			return g.Lambdas[a].FrameSize < g.Lambdas[b].FrameSize
		}
		return a < b
	})

	var zp, ram []placed
	fit := func(region []placed, i int, budget int) (int, bool) {
		size := g.Lambdas[i].FrameSize
		// 衝突する配置済みの区間を並べ、隙間を探す
		var ivs []placed
		for _, p := range region {
			if conflict(i, g.index[p.lmd]) {
				ivs = append(ivs, p)
			}
		}
		sort.Slice(ivs, func(x, y int) bool { return ivs[x].start < ivs[y].start })
		at := 0
		for _, iv := range ivs {
			if at+size <= iv.start {
				break
			}
			if iv.end > at {
				at = iv.end
			}
		}
		if at+size > budget {
			return 0, false
		}
		return at, true
	}
	plan := &Plan{}
	for _, i := range order {
		lmd := g.Lambdas[i]
		if lmd.FrameSize == 0 {
			lmd.FrameZp = true
			lmd.FrameBase = 0
			continue
		}
		wantZp := true
		if v, ok := lmd.Options.Get("zeropage"); ok && v.Text() == "false" {
			wantZp = false
		}
		if wantZp {
			if at, ok := fit(zp, i, zpBudget); ok {
				lmd.FrameZp = true
				lmd.FrameBase = at
				zp = append(zp, placed{lmd, at, at + lmd.FrameSize})
				plan.ZpUsed = max(plan.ZpUsed, at+lmd.FrameSize)
				continue
			}
		}
		at, ok := fit(ram, i, ramBudget)
		if !ok {
			return nil, &diag.Error{Msg: fmt.Sprintf("static frames do not fit: %s needs %d bytes (FC_SZP %d, FC_SRAM %d bytes; raise options(static_zp: N) / options(static_ram: N))",
				lmd.Id, lmd.FrameSize, zpBudget, ramBudget), Pos: lmd.Pos}
		}
		lmd.FrameZp = false
		lmd.FrameBase = at
		ram = append(ram, placed{lmd, at, at + lmd.FrameSize})
		plan.RamUsed = max(plan.RamUsed, at+lmd.FrameSize)
	}

	// _frames.inc
	inc := []string{
		"; 静的フレームの配置 (fc が生成。doc/v2_frame_alloc.md §6)",
		".ifndef __FC_FRAMES__",
		"__FC_FRAMES__ = 1",
		"\t.importzp FC_SZP",
		"\t.import FC_SRAM",
		"\t.import FC_SZP_SIZE, FC_SRAM_SIZE",
		fmt.Sprintf("\t.assert FC_SZP_SIZE >= %d, error, \"static frames need %d bytes of FC_SZP (raise .res of FC_SZP and FC_SZP_SIZE in base.asm)\"", plan.ZpUsed, plan.ZpUsed),
		fmt.Sprintf("\t.assert FC_SRAM_SIZE >= %d, error, \"static frames need %d bytes of FC_SRAM (raise .res of FC_SRAM and FC_SRAM_SIZE in base.asm)\"", plan.RamUsed, plan.RamUsed),
	}
	for _, i := range order {
		lmd := g.Lambdas[i]
		region := "FC_SRAM"
		if lmd.FrameZp {
			region = "FC_SZP"
		}
		inc = append(inc, fmt.Sprintf("%s = %s+%d\t; %d bytes, depth %d", lmd.FrameSym(), region, lmd.FrameBase, lmd.FrameSize, g.depth[i]))
	}
	inc = append(inc, ".endif", "")
	plan.Inc = inc
	return plan, nil
}

// Summary は配置の要約 (デバッグ表示用)。
func (g *Graph) Summary() string {
	var b strings.Builder
	for i, lmd := range g.Lambdas {
		fmt.Fprintf(&b, "%s abi=%s entry=%v size=%d depth=%d\n", lmd.Id, lmd.ABI, lmd.Entry, lmd.FrameSize, g.depth[i])
	}
	return b.String()
}
