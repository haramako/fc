package frames

import (
	"testing"

	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/types"
)

var tu = types.NewUniverse()

func fnType() *types.Type { return tu.Func(nil, tu.Void(), false) }

func lambda(id string, ops ...*ir.Op) *ir.Lambda {
	return &ir.Lambda{Id: id, Type: fnType(), Ops: ops}
}

func callOp(target string) *ir.Op {
	return &ir.Op{Code: ir.OpCall, Src: []ir.Operand{ir.NewSymbolLiteral("", fnType(), target)}}
}

func module(lmds ...*ir.Lambda) *ir.Module {
	m := &ir.Module{Id: "m"}
	for _, l := range lmds {
		m.Defs = append(m.Defs, &ir.Def{Sym: l.Id, Kind: ir.DefCode, Lambda: l})
	}
	return m
}

// TestAnalyzeRecursion: 直接の再帰と相互再帰は stack、それ以外は static。
func TestAnalyzeRecursion(t *testing.T) {
	a := lambda("_a", callOp("_b"))
	b := lambda("_b", callOp("_a"))
	c := lambda("_c", callOp("_c"))
	d := lambda("_d", callOp("_a"))
	g, err := Analyze([]*ir.Module{module(a, b, c, d)})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]ir.ABI{"_a": ir.ABIStack, "_b": ir.ABIStack, "_c": ir.ABIStack, "_d": ir.ABIStatic}
	for id, abi := range want {
		if g.ByID[id].Conv.ABI != abi {
			t.Errorf("%s: abi = %s, want %s", id, g.ByID[id].Conv.ABI, abi)
		}
	}
	if len(g.cycles) != 2 {
		t.Errorf("cycles = %v", g.cycles)
	}
}

// TestAnalyzeIndirect: 関数ポインタのグローバル変数経由の呼び出しは、その変数に代入された関数だけに辺を張る。
// 代入されていない同じ型の Entry (表の要素など) には張らないので、偽の再帰にならない。
func TestAnalyzeIndirect(t *testing.T) {
	gvar := ir.NewGlobal("cb", fnType(), "_cb")
	table := ir.NewGlobal("tbl", tu.ArrayOf(fnType(), 2), "_tbl")
	tmp := ir.NewLocal("$1", fnType(), ir.LTTemp)
	// wait: cb() を呼ぶ。main: cb = leaf; wait(); ev: wait() (ev は表に入っている = Entry)
	leaf := lambda("_leaf")
	wait := lambda("_wait",
		&ir.Op{Code: ir.OpLoad, Dst: tmp, Src: []ir.Operand{gvar}},
		&ir.Op{Code: ir.OpCall, Src: []ir.Operand{tmp}})
	ev := lambda("_ev", callOp("_wait"))
	main := lambda("_main",
		&ir.Op{Code: ir.OpLoad, Dst: gvar, Src: []ir.Operand{ir.NewSymbolLiteral("", fnType(), "_leaf")}},
		callOp("_wait"))
	m := module(leaf, wait, ev, main)
	m.Defs = append(m.Defs, &ir.Def{Sym: "_tbl", Kind: ir.DefBlock, Type: table.Type,
		Elems: []ir.Operand{ir.NewSymbolLiteral("", fnType(), "_ev"), ir.NewSymbolLiteral("", fnType(), "_leaf")}})
	g, err := Analyze([]*ir.Module{m})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"_leaf", "_wait", "_ev", "_main"} {
		if g.ByID[id].Conv.ABI != ir.ABIStatic {
			t.Errorf("%s: abi = %s, want static", id, g.ByID[id].Conv.ABI)
		}
	}
	if !g.ByID["_ev"].Conv.HasStackEntry() || !g.ByID["_leaf"].Conv.HasStackEntry() || g.ByID["_wait"].Conv.HasStackEntry() {
		t.Errorf("entry: ev=%v leaf=%v wait=%v", g.ByID["_ev"].Conv.HasStackEntry(), g.ByID["_leaf"].Conv.HasStackEntry(), g.ByID["_wait"].Conv.HasStackEntry())
	}
	// wait → leaf の辺だけ
	wi := g.index[g.ByID["_wait"]]
	if len(g.callees[wi]) != 1 || g.Lambdas[g.callees[wi][0]].Id != "_leaf" {
		t.Errorf("callees of wait = %v", g.callees[wi])
	}

	// 変数のアドレスを取ると何が入るか分からないので、同じ型の Entry 全部 (ev も) に辺が張られ、ev → wait → ev の閉路になる
	ref := ir.NewLocal("$2", tu.PointerTo(fnType()), ir.LTTemp)
	main.Ops = append(main.Ops, &ir.Op{Code: ir.OpRef, Dst: ref, Src: []ir.Operand{gvar}})
	g, err = Analyze([]*ir.Module{m})
	if err != nil {
		t.Fatal(err)
	}
	if g.ByID["_wait"].Conv.ABI != ir.ABIStack || g.ByID["_ev"].Conv.ABI != ir.ABIStack || g.ByID["_main"].Conv.ABI != ir.ABIStatic {
		t.Errorf("after &cb: wait=%s ev=%s main=%s", g.ByID["_wait"].Conv.ABI, g.ByID["_ev"].Conv.ABI, g.ByID["_main"].Conv.ABI)
	}
}

// TestPlaceOverlay: 兄弟のフレームは重なり、祖先と子孫は重ならない。深い関数からゼロページに入る。
func TestPlaceOverlay(t *testing.T) {
	a := lambda("_a", callOp("_b"), callOp("_c"))
	b := lambda("_b", callOp("_d"))
	c := lambda("_c")
	d := lambda("_d")
	g, err := Analyze([]*ir.Module{module(a, b, c, d)})
	if err != nil {
		t.Fatal(err)
	}
	a.FrameSize, b.FrameSize, c.FrameSize, d.FrameSize = 4, 3, 5, 2
	plan, err := Place(g, 8, 64)
	if err != nil {
		t.Fatal(err)
	}
	// d (深さ 2) → b, c (深さ 1) → a (深さ 0) の順。d=0..2, b=2..5 (d と衝突), c=0..5 (b とは兄弟なので重なる)、
	// a は 5..9 でゼロページ (8) に入らないので RAM
	if !d.FrameZp || d.FrameBase != 0 || !b.FrameZp || b.FrameBase != 2 || !c.FrameZp || c.FrameBase != 0 {
		t.Errorf("d=%v/%d b=%v/%d c=%v/%d", d.FrameZp, d.FrameBase, b.FrameZp, b.FrameBase, c.FrameZp, c.FrameBase)
	}
	if a.FrameZp || a.FrameBase != 0 || plan.ZpUsed != 5 || plan.RamUsed != 4 {
		t.Errorf("a=%v/%d zp=%d ram=%d", a.FrameZp, a.FrameBase, plan.ZpUsed, plan.RamUsed)
	}
}

// TestPlaceHiddenCaller: include した asm のファイルから呼ばれうる関数は、何の最中に呼ばれるか分からないので、フレームを
// どの関数とも重ねない。extern の関数への自分のモジュールの asm の参照は定義のラベルなので数えない。インラインアセンブラの
// 参照は、それを含む関数からの呼び出しの辺。
func TestPlaceHiddenCaller(t *testing.T) {
	frame := ir.Options{{Key: "abi", Value: ir.OptionValue{Kind: ir.OptStr, Str: "frame"}}, {Key: "scratch", Value: ir.OptionValue{Kind: ir.OptInt, Int: 2}}}
	main := lambda("_main", callOp("_a"), &ir.Op{Code: ir.OpAsm, Text: "\tjsr _leaf"})
	a := lambda("_a")
	leaf := lambda("_leaf")
	cb := lambda("_cb")                                                         // m の asm のファイルから呼ばれる
	ext := &ir.Lambda{Id: "_ext", Type: fnType(), Extern: true, Options: frame} // lib の asm の関数。m の asm から呼ばれる
	own := &ir.Lambda{Id: "_own", Type: fnType(), Extern: true, Options: frame} // lib の asm の関数。lib の asm (定義) だけが参照
	m := module(main, a, leaf, cb)
	m.AsmSymbols = ir.AsmSymbols("\tjsr _cb\n\tjsr _ext\n; void nsd_main(void);\n")
	lib := module(ext, own)
	lib.Id = "lib"
	lib.AsmSymbols = ir.AsmSymbols(".proc _ext\n.endproc\n.proc _own\n.endproc\n")
	g, err := Analyze([]*ir.Module{m, lib})
	if err != nil {
		t.Fatal(err)
	}
	if main.Conv.HasStackEntry() || !cb.Conv.HasStackEntry() || !leaf.Conv.HasStackEntry() || ext.Conv.HasStackEntry() {
		t.Errorf("entry: main=%v cb=%v leaf=%v ext=%v", main.Conv.HasStackEntry(), cb.Conv.HasStackEntry(), leaf.Conv.HasStackEntry(), ext.Conv.HasStackEntry())
	}
	if ext.Unused || !own.Unused || ext.FrameSize != 2 {
		t.Errorf("ext unused=%v size=%d, own unused=%v", ext.Unused, ext.FrameSize, own.Unused)
	}
	mi := g.index[main]
	if len(g.callees[mi]) != 2 || g.Lambdas[g.callees[mi][1]] != leaf {
		t.Errorf("callees of main = %v (want a, leaf)", g.callees[mi])
	}
	main.FrameSize, a.FrameSize, leaf.FrameSize, cb.FrameSize = 1, 1, 1, 1
	if _, err := Place(g, 16, 64); err != nil {
		t.Fatal(err)
	}
	overlap := func(x, y *ir.Lambda) bool {
		return x.FrameZp == y.FrameZp && x.FrameBase < y.FrameBase+y.FrameSize && y.FrameBase < x.FrameBase+x.FrameSize
	}
	if !overlap(a, leaf) {
		t.Errorf("siblings a (%d) and leaf (%d) should share", a.FrameBase, leaf.FrameBase)
	}
	for _, x := range []*ir.Lambda{cb, ext} {
		for _, y := range []*ir.Lambda{main, a, leaf, cb, ext} {
			if x != y && overlap(x, y) {
				t.Errorf("%s (%d+%d) overlaps %s (%d+%d)", x.Id, x.FrameBase, x.FrameSize, y.Id, y.FrameBase, y.FrameSize)
			}
		}
	}
}

// TestDepthThroughCycle: 再帰の連鎖 (r ↔ s) から呼ばれる関数とその先にも深さが付く (閉路の辺で止まって 0 のままだと、配置の
// 順で祖先より後に回る)。
func TestDepthThroughCycle(t *testing.T) {
	main := lambda("_main", callOp("_r"))
	r := lambda("_r", callOp("_s"), callOp("_a"))
	s := lambda("_s", callOp("_r"))
	a := lambda("_a", callOp("_b"))
	b := lambda("_b")
	g, err := Analyze([]*ir.Module{module(main, r, s, a, b)})
	if err != nil {
		t.Fatal(err)
	}
	d := func(l *ir.Lambda) int { return g.depth[g.index[l]] }
	if !(d(main) < d(r) && d(r) < d(a) && d(a) < d(b)) {
		t.Errorf("depth: main=%d r=%d a=%d b=%d", d(main), d(r), d(a), d(b))
	}
}

// TestPlaceNeedZpFirst: ゼロページが必須のフレーム (abi "frame" の asm の関数) を先に置く。割り込みから届く asm の関数は
// どのフレームとも重ねないので、深い fc の関数を先に置くとゼロページが埋まってエラーになっていた。fc の関数は RAM にあふれる。
func TestPlaceNeedZpFirst(t *testing.T) {
	frame := ir.Options{{Key: "abi", Value: ir.OptionValue{Kind: ir.OptStr, Str: "frame"}}, {Key: "scratch", Value: ir.OptionValue{Kind: ir.OptInt, Int: 4}}}
	main := lambda("_main", callOp("_a"))
	a := lambda("_a", callOp("_b"))
	b := lambda("_b")
	irq := lambda("_interrupt", callOp("_ext"))
	ext := &ir.Lambda{Id: "_ext", Type: fnType(), Extern: true, Options: frame}
	g, err := Analyze([]*ir.Module{module(main, a, b, irq, ext)})
	if err != nil {
		t.Fatal(err)
	}
	main.FrameSize, a.FrameSize, b.FrameSize, irq.FrameSize = 1, 1, 6, 1
	if _, err := Place(g, 8, 64); err != nil {
		t.Fatal(err)
	}
	if !ext.FrameZp || ext.FrameBase != 0 || b.FrameZp {
		t.Errorf("ext=%v/%d b=%v/%d", ext.FrameZp, ext.FrameBase, b.FrameZp, b.FrameBase)
	}
}

// TestPlaceSwapByRefs: 深い順に詰めると根 (main のループ) がゼロページからあふれるとき、フレームを参照する命令の少ない関数を
// 代わりに RAM へ出す (ゼロページの参照の数の和が増えるときだけ)。
func TestPlaceSwapByRefs(t *testing.T) {
	x := ir.NewLocal("x", tu.IntType(1, false), ir.LTNone)
	x.Location = ir.LocStatic
	use := func(k int) []*ir.Op {
		var ops []*ir.Op
		for i := 0; i < k; i++ {
			ops = append(ops, &ir.Op{Code: ir.OpLoad, Dst: x, Src: []ir.Operand{ir.NewIntLiteral("", tu.IntType(1, false), i)}})
		}
		return ops
	}
	a := lambda("_a", append(use(10), callOp("_b"))...)
	b := lambda("_b", use(1)...)
	g, err := Analyze([]*ir.Module{module(a, b)})
	if err != nil {
		t.Fatal(err)
	}
	a.FrameSize, b.FrameSize = 3, 5
	if _, err := Place(g, 6, 64); err != nil {
		t.Fatal(err)
	}
	// 深い順なら b=0..5、a は 5..8 で入らない。参照の多い a をゼロページに、b を RAM に
	if !a.FrameZp || a.FrameBase != 0 || b.FrameZp {
		t.Errorf("a=%v/%d b=%v/%d", a.FrameZp, a.FrameBase, b.FrameZp, b.FrameBase)
	}
}
