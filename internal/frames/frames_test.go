package frames

import (
	"testing"

	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/types"
)

var tu = types.NewUniverse()

func fnType() *types.Type { return tu.Func(nil, tu.Void(), false) }

func lambda(id string, ops ...*ir.Op) *ir.Lambda {
	return &ir.Lambda{Id: id, Type: fnType(), Ops: ops, Body: nil}
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
		if g.ByID[id].ABI != abi {
			t.Errorf("%s: abi = %s, want %s", id, g.ByID[id].ABI, abi)
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
		if g.ByID[id].ABI != ir.ABIStatic {
			t.Errorf("%s: abi = %s, want static", id, g.ByID[id].ABI)
		}
	}
	if !g.ByID["_ev"].Entry || !g.ByID["_leaf"].Entry || g.ByID["_wait"].Entry {
		t.Errorf("entry: ev=%v leaf=%v wait=%v", g.ByID["_ev"].Entry, g.ByID["_leaf"].Entry, g.ByID["_wait"].Entry)
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
	if g.ByID["_wait"].ABI != ir.ABIStack || g.ByID["_ev"].ABI != ir.ABIStack || g.ByID["_main"].ABI != ir.ABIStatic {
		t.Errorf("after &cb: wait=%s ev=%s main=%s", g.ByID["_wait"].ABI, g.ByID["_ev"].ABI, g.ByID["_main"].ABI)
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
