// Package frames はフレームの静的割付 (Agent/wiki/design/frame-alloc.md §6)。
//
// Analyze: 全モジュールの呼び出しグラフから各関数の呼び出し規約 (ir.ABI) を決める (sema の後、regalloc の前)。
// Place: regalloc でフレームの大きさが決まった後、static な関数のフレームを固定アドレスに配置し、
// 全モジュールが include する `_frames.inc` の行を作る。
package frames

import (
	"fmt"
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
	hidden  []bool                // include した asm のファイルから呼ばれうる (呼び出し元がグラフに見えない) 関数
}

// Analyze は各関数の ABI を決める。
//
//   - extern: options(abi: "frame") なら ABIStatic (呼ばれる側の葉としてフレームを配置する)、型が fastcall なら ABIFastcall、
//     それ以外は ABIStack
//   - options(abi: "frame") の本体のある関数: ABIStatic (asm から参照されても Entry にしない。レジスタ渡しもしない)
//   - options(abi: "stack"): ABIStack
//   - 呼び出しグラフの閉路に属する (再帰): ABIStack。間接呼び出しは「アドレスを取られた関数」全部への辺とみなす
//   - それ以外: ABIStatic。アドレスを取られた関数 (関数ポインタ / const の表 / インラインアセンブラからの参照) と
//     options(interrupt: true) の関数は Entry (スタック経由で引数を受け取り、プロローグで自分のフレームに写す)
//
// include した asm のファイルが参照する関数は、何の最中に呼ばれるか分からないので、Place でフレームをどの関数とも重ねない
// (インラインアセンブラの参照は、それを含む関数からの呼び出しの辺にする)。
func Analyze(mods []*ir.Module) (*Graph, error) {
	g := &Graph{ByID: map[string]*ir.Lambda{}, index: map[*ir.Lambda]int{}}
	var externs []*ir.Lambda
	modOf := map[*ir.Lambda]*ir.Module{}
	for _, m := range mods {
		for _, d := range m.Defs {
			if d.Kind != ir.DefCode {
				continue
			}
			lmd := d.Lambda
			g.ByID[lmd.Id] = lmd
			modOf[lmd] = m
			lmd.FrameABI = optText(lmd.Options, "abi") == "frame"
			if lmd.Extern {
				externs = append(externs, lmd)
				if lmd.FrameABI {
					// asm の関数の固定の静的フレーム: 呼ばれる側の葉としてフレームを配置する (asm の中の呼び出しは見えない)
					g.index[lmd] = len(g.Lambdas)
					g.Lambdas = append(g.Lambdas, lmd)
				}
				continue
			}
			g.index[lmd] = len(g.Lambdas)
			g.Lambdas = append(g.Lambdas, lmd)
		}
	}
	for _, lmd := range externs {
		switch {
		case lmd.FrameABI:
			lmd.Conv.SetStatic(lmd, false, false)
			lmd.Scratch, _ = lmd.Options.Int("scratch")
			lmd.FrameSize = lmd.Type.Base.Size + lmd.Scratch
			for _, p := range lmd.Type.Params {
				lmd.FrameSize += p.Size
			}
		case optText(lmd.Options, "abi") == "cc65":
			if err := checkCc65(lmd); err != nil {
				return nil, err
			}
			lmd.Conv.SetOther(lmd, ir.ABICc65)
			lmd.ZpUsed = max(lmd.Type.Base.Size, 1)
			for _, p := range lmd.Type.Params {
				lmd.ZpUsed = max(lmd.ZpUsed, p.Size)
			}
		case lmd.Type.Fastcall():
			lmd.Conv.SetOther(lmd, ir.ABIFastcall)
		default:
			lmd.Conv.SetOther(lmd, ir.ABIStack)
		}
	}
	for _, lmd := range g.Lambdas {
		if optText(lmd.Options, "abi") == "cc65" {
			return nil, &diag.Error{Msg: fmt.Sprintf("%s: options(abi: \"cc65\") is for extern functions (declared without a body)", lmd.Name), Pos: lmd.Pos}
		}
	}
	var addrErr error
	n := len(g.Lambdas)

	// アドレスを取られた関数 (スタックの入口が要る) と辺
	entry := make([]bool, n)
	hidden := make([]bool, n)
	indirect := make([][]*ir.Op, n) // 関数ごとの間接呼び出し (飛び先は後で絞る)
	g.callees = make([][]int, n)
	// 関数ポインタのグローバル変数に代入された関数と、関数ポインタの const 表の要素 (間接呼び出しの飛び先を絞るため)
	assigned := map[string][]string{}  // 変数のシンボル → 代入された関数のシンボル
	unknownAssign := map[string]bool{} // リテラル以外が代入された (何が入るか分からない)
	aliased := map[string]bool{}       // equ (const の別名) が参照する関数
	tables := map[string][]string{}    // const 表のシンボル → 要素の関数のシンボル ("" は関数以外)
	funcSym := func(o ir.Operand) string {
		v := ir.ValLiteral(o)
		if v != nil && v.Kind == ir.KindLiteral && !v.IsInt && v.Symbol != "" {
			return v.Symbol
		}
		return ""
	}
	// markSym は fc のコードで関数のアドレスを取ったことを記録する (スタックの入口が要る)。abi: "frame" の関数はエラー (関数ポインタで
	// 呼ぶにはスタックから写すプロローグが要る)
	markSym := func(sym string) {
		if l, ok := g.ByID[sym]; ok {
			if i, ok := g.index[l]; ok {
				switch {
				case !l.FrameABI:
					entry[i] = true
				case addrErr == nil:
					addrErr = &diag.Error{Msg: fmt.Sprintf("%s: cannot take the address of an abi \"frame\" function (it has no entry that takes the arguments from the stack)", l.Name), Pos: l.Pos}
				}
			}
		}
	}
	// markAsmSym は asm のテキストが関数 sym を参照したことを記録する。呼び出し規約が分からないので Entry (abi: "frame" の
	// 関数は asm から呼ぶための固定の規約なので Entry にしない)。caller はそのインラインアセンブラを含む関数 (呼び出しの辺を
	// 足す)。include した asm のファイル (caller < 0、m はそれを include したモジュール) なら呼び出し元が見えないので hidden。
	// ただし extern の関数へのそれ自身のモジュールの asm の参照は、定義のラベルと区別できないので数えない (abi: "frame" の
	// asm の関数どうしは同じモジュールの中で呼び合えない。docs/reference/assembly.md の「frame」)
	markAsmSym := func(sym string, caller int, m *ir.Module) {
		l, ok := g.ByID[sym]
		if !ok {
			return
		}
		j, ok := g.index[l]
		if !ok {
			return
		}
		if !l.FrameABI {
			entry[j] = true
		}
		switch {
		case caller >= 0:
			g.callees[caller] = append(g.callees[caller], j)
		case !l.Extern || modOf[l] != m:
			hidden[j] = true
		}
	}
	var markOperand func(ir.Operand)
	markOperand = func(o ir.Operand) {
		if o == nil {
			return
		}
		v := ir.ValLiteral(o)
		if v != nil && v.Kind == ir.KindArrayLiteral {
			for _, elem := range v.Elems {
				markOperand(elem)
			}
		}
		if v != nil && v.Kind == ir.KindLiteral && !v.IsInt && v.Symbol != "" {
			markSym(v.Symbol)
			// fc のコードでアドレスを取った cc65 規約の関数 (asm からの参照は定義そのものなので構わない)
			if l, ok := g.ByID[v.Symbol]; ok && l.Extern && l.Conv.ABI == ir.ABICc65 && addrErr == nil {
				addrErr = &diag.Error{Msg: fmt.Sprintf("%s: cannot take the address of a cc65 abi function (its arguments are passed in A/X, not on the stack)", l.Name), Pos: l.Pos}
			}
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
			if d.Kind == ir.DefBss && d.Init != nil {
				markOperand(d.Init) // 変数の初期値の関数のアドレス (`var f:fn():u8 = get;`)
			}
			if d.Kind == ir.DefEqu && d.Equ != nil && d.Equ.IsInt {
				unknownAssign[d.Sym] = true // options(address:) の変数は asm 側が書きうる
			}
			// DefEqu の関数シンボル (options(symbol:) の別名、`const f = ->fn ...`) は呼び出しに使う名前で、アドレスを
			// 取ったのではない (Entry にはしない)。ただし equ の行がその関数を参照するので、出力はする (自動インラインで
			// 呼び出しが全部消えても)
			if d.Kind == ir.DefEqu && d.Equ != nil && !d.Equ.IsInt && d.Equ.Symbol != "" {
				aliased[d.Equ.Symbol] = true
				if d.Equ.Type.IsFarFunc() {
					markOperand(d.Equ)
				}
			}
		}
	}
	for _, m := range mods {
		for _, sym := range m.AsmSymbols {
			markAsmSym(sym, -1, m)
		}
	}
	for i, lmd := range g.Lambdas {
		if lmd.Options.Flag("interrupt") || lmd.Id == "_interrupt" || lmd.Id == "_interrupt_irq" {
			// NMI / IRQ の入口の名前 (share/runtime.asm が呼ぶ) の関数は @(interrupt) が無くても割り込み (付け忘れると
			// フレームが main の呼び出しと重なっていた)
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
				if ir.ValType(op.Src[0]).IsFarFunc() {
					markOperand(op.Src[0])
				}
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
				for _, w := range ir.AsmSymbols(op.Text) {
					markAsmSym(w, i, nil)
				}
			default:
				for _, s := range op.Src {
					markOperand(s)
				}
				markOperand(op.Dst)
			}
		}
	}
	if addrErr != nil {
		return nil, addrErr
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
				if g.Lambdas[j].Type == t || (t.IsFarFunc() && types.SameFuncSignature(g.Lambdas[j].Type, t)) {
					g.callees[i] = append(g.callees[i], j)
				}
			}
		}
	}

	// 使われない関数 (tree shaking): main、割り込み、options(symbol:) (asm から呼ばれる名前)、アドレスを取られた /
	// asm から参照される関数 (entry) から呼び出しの辺で届かない関数は出力しない (フレームも割り付けない)
	reached := make([]bool, n)
	var work []int
	hasMain := false
	for i, lmd := range g.Lambdas {
		if entry[i] || hidden[i] || lmd.Id == "_main" || (lmd.Options.Has("symbol") && !lmd.Extern) || aliased[lmd.Id] {
			reached[i] = true
			work = append(work, i)
		}
		hasMain = hasMain || lmd.Id == "_main"
	}
	if !hasMain {
		// main の無いプログラム (ライブラリだけの検査、frames の単体テスト) では何も削らない
		for i := range reached {
			reached[i] = true
		}
	}
	for len(work) > 0 {
		i := work[len(work)-1]
		work = work[:len(work)-1]
		for _, j := range g.callees[i] {
			if !reached[j] {
				reached[j] = true
				work = append(work, j)
			}
		}
	}
	for i, lmd := range g.Lambdas {
		lmd.Unused = !reached[i]
	}

	// 再帰 (閉路) の検出: Tarjan の SCC
	inCycle, cycles := tarjanCycles(n, g.callees)
	g.cycles = cycles
	g.hidden = hidden
	for i, lmd := range g.Lambdas {
		switch {
		case lmd.FrameABI:
			if inCycle[i] {
				return nil, &diag.Error{Msg: fmt.Sprintf("%s: an abi \"frame\" function must not be recursive (its frame is static)", lmd.Name), Pos: lmd.Pos}
			}
			lmd.Conv.SetStatic(lmd, false, false) // スタックの入口・レジスタ渡しはしない (asm と同じ固定の規約)
		case lmd.Options.Has("abi") && optText(lmd.Options, "abi") == "stack":
			lmd.Conv.SetOther(lmd, ir.ABIStack)
		case inCycle[i]:
			lmd.Conv.SetOther(lmd, ir.ABIStack)
		default:
			lmd.Conv.SetStatic(lmd, entry[i], !lmd.Interrupt && !lmd.Extern)
		}
	}

	// 戻り値を A だけで返す関数 (A にも置いて返す関数のうち、フレームの戻り値を読む呼び出しが無いもの)
	for i, lmd := range g.Lambdas {
		c := &lmd.Conv
		c.Result.OnlyA = c.Result.InA && !c.HasStackEntry() && !hidden[i] && !aliased[lmd.Id] && !lmd.Options.Has("symbol")
	}
	for _, caller := range g.Lambdas {
		for _, op := range caller.Ops {
			if op == nil || !op.Code.IsCall() {
				continue
			}
			if v := ir.ValLiteral(op.Src[0]); v != nil && v.Kind == ir.KindLiteral && v.Symbol != "" {
				if l, ok := g.ByID[v.Symbol]; ok && !l.Conv.ResultFromA(caller, op) {
					l.Conv.Result.OnlyA = false // far call と stack 関数は戻り値を呼び先のフレームから読む (codegen.genCall)
				}
			}
		}
	}

	// 割り込みから届く関数は全部 static でなければならない (X が何を指すか分からない)
	for i, lmd := range g.Lambdas {
		if !lmd.Interrupt {
			continue
		}
		if lmd.Conv.ABI != ir.ABIStatic {
			return nil, &diag.Error{Msg: fmt.Sprintf("%s: interrupt function must not be recursive", lmd.Id), Pos: lmd.Pos}
		}
		for _, j := range g.reachable(i) {
			if g.Lambdas[j].Conv.ABI != ir.ABIStatic {
				return nil, &diag.Error{Msg: fmt.Sprintf("%s: interrupt function reaches %s which uses the stack (recursive or options(abi: \"stack\"))",
					lmd.Id, g.Lambdas[j].Id), Pos: lmd.Pos}
			}
		}
	}

	// 深さ: 根 (呼び出し元の無い関数) からの最長距離。閉路 (再帰の連鎖。stack) の内側の辺は数えない (残りは DAG)。閉路の辺も
	// 数えると、閉路の節が入次数 0 にならず、その先 (再帰する関数から呼ばれる関数とその先) の深さが 0 のまま残って、配置の順
	// (深い順) で祖先より後に回っていた (abi "frame" の asm の関数は祖先のフレームでゼロページが埋まった後でエラーになった)
	comp := make([]int, n) // 閉路を含む強連結成分の番号 (-1 は閉路に属さない)
	for i := range comp {
		comp[i] = -1
	}
	for k, c := range cycles {
		for _, v := range c {
			comp[v] = k
		}
	}
	inner := func(i, j int) bool { return i == j || comp[i] >= 0 && comp[i] == comp[j] }
	g.depth = make([]int, n)
	indeg := make([]int, n)
	for i := range g.Lambdas {
		for _, j := range g.callees[i] {
			if !inner(i, j) {
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
			if inner(i, j) {
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

// indirectTargets は間接呼び出しの飛び先の関数 (シンボル) を絞れるなら返す。
//   - グローバルの関数ポインタ変数 (直接、または `load t = g` の t): その変数に代入された関数 (リテラル以外の代入があれば不可)
//   - const 表の要素 (`load_mem t = TABLE, i` / `index p = TABLE, i; load_mem t = p`): 表の要素 (関数以外の要素があれば不可)
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
	def, ok := ud.SingleDef(t)
	if !ok {
		return nil, false
	}
	switch def.Code {
	case ir.OpLoad:
		if syms, ok := fromGlobal(def.Src[0]); ok {
			return syms, true
		}
		if f := ir.ValLiteral(def.Src[0]); f != nil && f.Kind == ir.KindLiteral && !f.IsInt && f.Symbol != "" {
			return []string{f.Symbol}, true
		}
	case ir.OpLoadMem:
		if m := def.Mem(); m.Index != nil {
			return fromTable(m.Base)
		}
		if p, ok := def.Src[0].(*ir.Value); ok && p.Kind == ir.KindLocal {
			if pd, ok := ud.SingleDef(p); ok && pd.Code == ir.OpIndex {
				return fromTable(pd.Src[0])
			}
		}
	}
	return nil, false
}

func optText(o ir.Options, key string) string {
	v, _ := o.Get(key)
	return v.Text()
}

// checkCc65 は options(abi: "cc65") の extern 関数の制約: 引数は 0 か 1 個で 1〜2 バイト (A / A,X で渡す)、
// 戻り値は void か 1〜2 バイト (A / A,X)、fastcall と併用しない。2 個以上の引数は cc65 のパラメータスタックが要るので対象外。
func checkCc65(lmd *ir.Lambda) error {
	bad := func(msg string) error {
		return &diag.Error{Msg: fmt.Sprintf("%s: options(abi: \"cc65\"): %s", lmd.Name, msg), Pos: lmd.Pos}
	}
	if lmd.Type.Fastcall() {
		return bad("cannot combine with fastcall")
	}
	if len(lmd.Type.Params) > 1 {
		return bad("at most one argument (the last argument of a cc65 __fastcall__ function is passed in A/X; the others need the cc65 parameter stack, which fc does not have)")
	}
	for _, p := range lmd.Type.Params {
		if p.Size < 1 || p.Size > 2 {
			return bad(fmt.Sprintf("argument must be 1 or 2 bytes (got %s)", p))
		}
	}
	if s := lmd.Type.Base.Size; s > 2 {
		return bad(fmt.Sprintf("result must be void, 1 or 2 bytes (got %s)", lmd.Type.Base))
	}
	return nil
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
	Warnings        []diag.Warning
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
	// include した asm のファイルから呼ばれうる関数 (とその先) も、何の最中に呼ばれるか分からないので、どのフレームとも重ねない
	hiddenTree := make([]bool, n)
	for i := range g.hidden {
		if g.hidden[i] {
			hiddenTree[i] = true
			for _, j := range g.reachable(i) {
				hiddenTree[j] = true
			}
		}
	}
	conflict := func(a, b int) bool {
		return a == b || reach[a][b] || reach[b][a] || irqTree[a] || irqTree[b] || hiddenTree[a] || hiddenTree[b]
	}
	// 割り込みと通常の処理の両方から呼ばれる関数: 静的フレームは 1 つなので、通常の処理がその関数の中にいるときに
	// 割り込みが来て同じ関数を呼ぶと、フレームが壊れる
	var warnings []diag.Warning
	for j, lmd := range g.Lambdas {
		if !irqTree[j] || lmd.Interrupt || lmd.Unused || lmd.Extern {
			continue
		}
		from := ""
		for i, caller := range g.Lambdas {
			if !irqTree[i] && reach[i][j] && !caller.Unused {
				from = caller.Id
				break
			}
		}
		if from == "" {
			continue
		}
		var irqs []string
		for i, root := range g.Lambdas {
			if root.Interrupt && reach[i][j] {
				irqs = append(irqs, root.Id)
			}
		}
		warnings = append(warnings, diag.Warning{Pos: lmd.Pos, Msg: fmt.Sprintf("%s is called both from the interrupt handler %s and from %s; its static frame is shared, so an interrupt while %s runs corrupts it",
			lmd.Id, strings.Join(irqs, ", "), from, lmd.Id)})
	}

	var order []int
	for i, lmd := range g.Lambdas {
		if lmd.Conv.ABI == ir.ABIStatic && !lmd.Unused {
			order = append(order, i)
		}
	}
	// 置く順: ゼロページが必須のフレーム (abi "frame" の asm の関数) を先に (fc の関数のフレームはゼロページからあふれても RAM に
	// 置けるが、asm の関数は置けない)。残りは深い順 (呼び出しの連鎖の葉から: 兄弟どうしが同じ番地に重なる)、同じ深さなら小さい順
	sort.SliceStable(order, func(x, y int) bool {
		a, b := order[x], order[y]
		if za, zb := needZp(g.Lambdas[a]), needZp(g.Lambdas[b]); za != zb {
			return za
		}
		if g.depth[a] != g.depth[b] {
			return g.depth[a] > g.depth[b]
		}
		if g.Lambdas[a].FrameSize != g.Lambdas[b].FrameSize {
			return g.Lambdas[a].FrameSize < g.Lambdas[b].FrameSize
		}
		return a < b
	})

	// assign は order の順に置く (forced の関数はゼロページに置かない)。slots は関数ごとの置き場所
	type slot struct {
		zp, ram bool
		at      int
	}
	assign := func(forced []bool) ([]slot, error) {
		slots := make([]slot, n)
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
		for _, i := range order {
			lmd := g.Lambdas[i]
			if lmd.FrameSize == 0 {
				slots[i] = slot{zp: true}
				continue
			}
			if !zpOff(lmd) && !forced[i] {
				if at, ok := fit(zp, i, zpBudget); ok {
					slots[i] = slot{zp: true, at: at}
					zp = append(zp, placed{lmd, at, at + lmd.FrameSize})
					continue
				}
			}
			if needZp(lmd) {
				// abi: "frame" の asm の関数はフレームを `(F_sym+k),y` のように間接の番地にも使うのでゼロページが要る
				// (RAM でよいなら options(zeropage: false))
				return nil, &diag.Error{Msg: fmt.Sprintf("the static frame of %s (abi \"frame\", %d bytes) does not fit in the zero page (FC_SZP %d bytes; raise options(static_zp: N), or declare zeropage: false if the assembler does not use it as a zero page address)",
					lmd.Id, lmd.FrameSize, zpBudget), Pos: lmd.Pos}
			}
			at, ok := fit(ram, i, ramBudget)
			if !ok {
				return nil, &diag.Error{Msg: fmt.Sprintf("static frames do not fit: %s needs %d bytes (FC_SZP %d, FC_SRAM %d bytes; raise options(static_zp: N) / options(static_ram: N))",
					lmd.Id, lmd.FrameSize, zpBudget, ramBudget), Pos: lmd.Pos}
			}
			slots[i] = slot{ram: true, at: at}
			ram = append(ram, placed{lmd, at, at + lmd.FrameSize})
		}
		return slots, nil
	}
	forced := make([]bool, n)
	slots, err := assign(forced)
	if err != nil {
		return nil, err
	}
	// ゼロページからあふれた関数があれば、フレームを参照する命令の少ない関数を代わりに RAM へ出して入れ替えを試す (深い順の
	// 詰め方では呼び出しの根 (main のループ) が最後になってあふれる。ゼロページの参照は 1 バイト・1 サイクル短い)。
	// ゼロページに置いた関数の参照の数の和が増えるときだけ採る
	refs := make([]int, n)
	for i, lmd := range g.Lambdas {
		refs[i] = frameRefs(lmd)
	}
	score := func(sl []slot) int {
		t := 0
		for i, x := range sl {
			if x.zp {
				t += refs[i]
			}
		}
		return t
	}
	best := score(slots)
	for iter := 0; iter < 16; iter++ {
		var spilled []int
		for _, i := range order {
			if slots[i].ram && !zpOff(g.Lambdas[i]) && !forced[i] {
				spilled = append(spilled, i)
			}
		}
		sort.SliceStable(spilled, func(x, y int) bool { return refs[spilled[x]] > refs[spilled[y]] })
		improved := false
		for _, f := range spilled {
			var cands []int
			for _, v := range order {
				if slots[v].zp && !needZp(g.Lambdas[v]) && g.Lambdas[v].FrameSize > 0 && refs[v] < refs[f] && conflict(f, v) {
					cands = append(cands, v)
				}
			}
			sort.SliceStable(cands, func(x, y int) bool { return refs[cands[x]] < refs[cands[y]] })
			for k, v := range cands {
				if k >= 8 {
					break
				}
				forced[v] = true
				if sl, err := assign(forced); err == nil && score(sl) > best {
					slots, best, improved = sl, score(sl), true
					break
				}
				forced[v] = false
			}
			if improved {
				break
			}
		}
		if !improved {
			break
		}
	}
	plan := &Plan{Warnings: warnings}
	nzp, nram := 0, 0
	for _, i := range order {
		lmd := g.Lambdas[i]
		lmd.FrameZp = slots[i].zp
		lmd.FrameBase = slots[i].at
		if lmd.FrameSize == 0 {
			continue
		}
		if slots[i].zp {
			plan.ZpUsed = max(plan.ZpUsed, slots[i].at+lmd.FrameSize)
			nzp++
		} else if slots[i].ram {
			plan.RamUsed = max(plan.RamUsed, slots[i].at+lmd.FrameSize)
			nram++
		}
	}

	// _frames.inc
	inc := []string{
		"; 静的フレームの配置 (fc が生成。Agent/wiki/design/frame-alloc.md §6)",
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
	inc = append(inc, frameABISymbols(g)...)
	inc = append(inc, ".endif", "")
	plan.Inc = inc
	plan.Report = g.report(plan, nzp, nram)
	return plan, nil
}

// frameRefs はフレーム (静的なローカル・引数・戻り値) を参照するオペランドの数 (ゼロページに置いたときに縮むバイト数の目安)。
// ループの展開の写し (Op.UnrollCopy) は数えない: 写しの数だけ参照が増えた関数が、展開していないループの中で呼ばれる関数を
// ゼロページから追い出し、-O 2 が -O 0 より遅くなっていた (testdata/perf/unroll-slower: 展開した far1.ff1 がゼロページを取り、
// ff1 のループで呼ばれる far1.ff0 が RAM に出た)。asm の関数は数えられないので大きな値 (needZp で先に置く)。
func frameRefs(lmd *ir.Lambda) int {
	if lmd.Extern {
		return 1 << 20
	}
	n := 0
	var walk func(o ir.Operand)
	walk = func(o ir.Operand) {
		switch x := o.(type) {
		case *ir.Value:
			if x.Kind == ir.KindLocal && x.Location == ir.LocStatic {
				n++
			}
		case *ir.CastedValue:
			walk(x.From)
		case *ir.PointeredArray:
			walk(x.From)
		}
	}
	for _, op := range lmd.Ops {
		if op == nil || op.UnrollCopy {
			continue
		}
		if op.Dst != nil {
			walk(op.Dst)
		}
		for _, o := range op.Src {
			walk(o)
		}
	}
	return n
}

// zpOff は options(zeropage: false) (静的フレームを RAM 側に置く) か。
func zpOff(lmd *ir.Lambda) bool {
	v, ok := lmd.Options.Get("zeropage")
	return ok && v.Text() == "false"
}

// needZp はフレームをゼロページに置かなければならないか (abi "frame" の asm の関数。フレームを `(F_sym+k),y` のように間接の
// 番地にも使う)。
func needZp(lmd *ir.Lambda) bool { return lmd.Extern && lmd.FrameABI && !zpOff(lmd) }

// frameABISymbols は abi: "frame" の関数の、asm が使うシンボル (_frames.inc の行): 引数ごとの位置 `F_sym__名前` と作業領域の
// 先頭 `F_sym__scratch`。使われない extern (fc から呼ばれない) にもフレームの名前を定義する (asm のファイルは丸ごと入るので、
// 参照が未定義だとアセンブルできない。実行されないので番地はどこでもよい)。
func frameABISymbols(g *Graph) []string {
	var r []string
	for _, lmd := range g.Lambdas {
		if !lmd.FrameABI {
			continue
		}
		sym := lmd.FrameSym()
		if lmd.Unused {
			r = append(r, fmt.Sprintf("%s = FC_SZP\t; unused", sym))
		}
		off := lmd.Type.Base.Size
		for i, p := range lmd.Type.Params {
			name := fmt.Sprint(i)
			if i < len(lmd.Params) && lmd.Params[i].Name != "" {
				name = lmd.Params[i].Name
			}
			r = append(r, fmt.Sprintf("%s__%s = %s+%d", sym, name, sym, off))
			off += p.Size
		}
		if lmd.Scratch > 0 {
			r = append(r, fmt.Sprintf("%s__scratch = %s+%d", sym, sym, off))
		}
	}
	return r
}

// report は配置の要約: 種類ごとの数、領域の使用量、stack に残った理由 (再帰の連鎖と options)。
func (g *Graph) report(plan *Plan, nZp, nRam int) []string {
	var static, entry, stack, empty int
	var forced, unused []string
	for _, lmd := range g.Lambdas {
		if lmd.Unused {
			unused = append(unused, lmd.Id)
			continue
		}
		switch {
		case lmd.Conv.ABI == ir.ABIStatic && lmd.FrameSize == 0:
			empty++
		case lmd.Conv.ABI == ir.ABIStatic && lmd.Conv.HasStackEntry():
			entry++
		case lmd.Conv.ABI == ir.ABIStatic:
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
	if len(unused) > 0 {
		sort.Strings(unused)
		r = append(r, fmt.Sprintf("  unused (not emitted): %d functions: %s", len(unused), strings.Join(unused, " ")))
	}
	return r
}
