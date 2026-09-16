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
	cycles  [][]int               // 再帰の連鎖 (閉路を含む強連結成分。Lambdas の添字)
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
	indirect := make([][]*ir.Op, n) // 関数ごとの間接呼び出し (飛び先は後で絞る)
	g.callees = make([][]int, n)
	// 関数ポインタのグローバル変数に代入された関数と、関数ポインタの const 表の要素 (間接呼び出しの飛び先を絞るため)
	assigned := map[string][]string{} // 変数のシンボル → 代入された関数のシンボル
	unknownAssign := map[string]bool{} // リテラル以外が代入された (何が入るか分からない)
	tables := map[string][]string{}   // const 表のシンボル → 要素の関数のシンボル ("" は関数以外)
	funcSym := func(o ir.Operand) string {
		v := ir.ValLiteral(o)
		if v != nil && v.Kind == ir.KindLiteral && !v.IsInt && v.Symbol != "" {
			return v.Symbol
		}
		return ""
	}
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
					tables[d.Sym] = append(tables[d.Sym], funcSym(e))
				}
			}
			if d.Kind == ir.DefEqu && d.Equ != nil && d.Equ.IsInt {
				unknownAssign[d.Sym] = true // options(address:) の変数は asm 側が書きうる
			}
			// DefEqu の関数シンボル (options(symbol:) の別名) は呼び出しに使う名前で、アドレスを取ったのではない
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
					indirect[i] = append(indirect[i], op)
				}
				for _, s := range op.Src[1:] {
					markOperand(s)
				}
			case ir.OpLoad:
				// 関数ポインタのグローバル変数への代入
				if dv, ok := op.Dst.(*ir.Value); ok && dv.Kind == ir.KindGlobal && dv.Symbol != "" && dv.Type.Kind == types.Func {
					if f := funcSym(op.Src[0]); f != "" {
						assigned[dv.Symbol] = append(assigned[dv.Symbol], f)
					} else {
						unknownAssign[dv.Symbol] = true
					}
				}
				markOperand(op.Src[0])
			case ir.OpRef:
				// アドレスを取られたグローバルの関数ポインタ変数はポインタ経由で何が入るか分からない
				if v, ok := op.Src[0].(*ir.Value); ok && v.Kind == ir.KindGlobal && v.Symbol != "" {
					unknownAssign[v.Symbol] = true
				}
				markOperand(op.Src[0])
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
	// 間接呼び出しの飛び先: 関数ポインタのグローバル変数ならそれに代入された関数、const 表の要素ならその表の要素、
	// それ以外 (ローカル変数、struct のフィールド経由など) は同じ関数型でアドレスを取られた関数の全部
	for i, lmd := range g.Lambdas {
		if len(indirect[i]) == 0 {
			continue
		}
		ud := ir.BuildUseDef(lmd)
		for _, op := range indirect[i] {
			syms, ok := indirectTargets(lmd, ud, op.Src[0], assigned, unknownAssign, tables)
			if ok {
				for _, s := range syms {
					if l, ok := g.ByID[s]; ok {
						if j, ok := g.index[l]; ok {
							g.callees[i] = append(g.callees[i], j)
						}
					}
				}
				continue
			}
			t := ir.ValType(op.Src[0])
			for _, j := range entries {
				if g.Lambdas[j].Type == t {
					g.callees[i] = append(g.callees[i], j)
				}
			}
		}
	}

	// 再帰 (閉路) の検出: Tarjan の SCC
	inCycle, cycles := tarjanCycles(n, g.callees)
	g.cycles = cycles
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

// indirectTargets は間接呼び出しの飛び先の関数 (シンボル) を絞れるなら返す。
//   - グローバルの関数ポインタ変数 (直接、または `load t = g` の t): その変数に代入された関数 (リテラル以外の代入があれば不可)
//   - const 表の要素 (`index_pget t = TABLE, i` / `index p = TABLE, i; pget t = p`): 表の要素 (関数以外の要素があれば不可)
func indirectTargets(lmd *ir.Lambda, ud *ir.UseDef, callee ir.Operand, assigned map[string][]string, unknown map[string]bool, tables map[string][]string) ([]string, bool) {
	fromGlobal := func(o ir.Operand) ([]string, bool) {
		v, ok := o.(*ir.Value)
		if !ok || v.Kind != ir.KindGlobal || v.Symbol == "" || v.Type.Kind != types.Func {
			return nil, false
		}
		if unknown[v.Symbol] {
			return nil, false
		}
		return assigned[v.Symbol], true
	}
	fromTable := func(o ir.Operand) ([]string, bool) {
		v := ir.ValLiteral(o)
		if v == nil || v.Kind != ir.KindGlobal || v.Symbol == "" {
			return nil, false
		}
		elems, ok := tables[v.Symbol]
		if !ok {
			return nil, false
		}
		for _, e := range elems {
			if e == "" {
				return nil, false
			}
		}
		return elems, true
	}
	if syms, ok := fromGlobal(callee); ok {
		return syms, true
	}
	t, ok := callee.(*ir.Value)
	if !ok || t.Kind != ir.KindLocal {
		return nil, false
	}
	defs := ud.Defs[t]
	if len(defs) != 1 {
		return nil, false
	}
	def := lmd.Ops[defs[0]]
	switch def.Code {
	case ir.OpLoad:
		if syms, ok := fromGlobal(def.Src[0]); ok {
			return syms, true
		}
		if f := ir.ValLiteral(def.Src[0]); f != nil && f.Kind == ir.KindLiteral && !f.IsInt && f.Symbol != "" {
			return []string{f.Symbol}, true
		}
	case ir.OpIndexPget:
		return fromTable(def.Src[0])
	case ir.OpPget:
		if p, ok := def.Src[0].(*ir.Value); ok && p.Kind == ir.KindLocal {
			if pd := ud.Defs[p]; len(pd) == 1 && lmd.Ops[pd[0]].Code == ir.OpIndex {
				return fromTable(lmd.Ops[pd[0]].Src[0])
			}
		}
	}
	return nil, false
}

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

// tarjanCycles は閉路に属する節 (自己ループを含む) と、閉路を含む強連結成分の一覧を返す。
func tarjanCycles(n int, edges [][]int) ([]bool, [][]int) {
	index := make([]int, n)
	low := make([]int, n)
	onStack := make([]bool, n)
	for i := range index {
		index[i] = -1
	}
	var stack []int
	counter := 0
	inCycle := make([]bool, n)
	var cycles [][]int
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
				cycles = append(cycles, scc)
			}
		}
	}
	for v := 0; v < n; v++ {
		if index[v] < 0 {
			strong(v)
		}
	}
	return inCycle, cycles
}

// Plan は配置の結果。
type Plan struct {
	ZpUsed, RamUsed int
	Inc             []string // _frames.inc の行
	Report          []string // 配置の要約 (fcc build -d で表示)
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
	plan.Report = g.report(plan, len(zp), len(ram))
	return plan, nil
}

// report は配置の要約: 種類ごとの数、領域の使用量、stack に残った理由 (再帰の連鎖と options)。
func (g *Graph) report(plan *Plan, nZp, nRam int) []string {
	var static, entry, stack, empty int
	var forced []string
	for _, lmd := range g.Lambdas {
		switch {
		case lmd.ABI == ir.ABIStatic && lmd.FrameSize == 0:
			empty++
		case lmd.ABI == ir.ABIStatic && lmd.Entry:
			entry++
		case lmd.ABI == ir.ABIStatic:
			static++
		default:
			stack++
			if lmd.Options.Has("abi") {
				forced = append(forced, lmd.Id)
			}
		}
	}
	r := []string{fmt.Sprintf("frames: static %d (entry %d, no frame %d), stack %d; FC_SZP %d bytes (%d functions), FC_SRAM %d bytes (%d functions)",
		static+entry+empty, entry, empty, stack, plan.ZpUsed, nZp, plan.RamUsed, nRam)}
	for _, c := range g.cycles {
		names := make([]string, 0, len(c))
		for _, i := range c {
			names = append(names, g.Lambdas[i].Id)
		}
		sort.Strings(names)
		r = append(r, fmt.Sprintf("  recursive (stack): %s", strings.Join(names, " ")))
	}
	if len(forced) > 0 {
		r = append(r, fmt.Sprintf("  options(abi: \"stack\"): %s", strings.Join(forced, " ")))
	}
	return r
}

// Summary は配置の要約 (デバッグ表示用)。
func (g *Graph) Summary() string {
	var b strings.Builder
	for i, lmd := range g.Lambdas {
		fmt.Fprintf(&b, "%s abi=%s entry=%v size=%d depth=%d\n", lmd.Id, lmd.ABI, lmd.Entry, lmd.FrameSize, g.depth[i])
	}
	return b.String()
}
