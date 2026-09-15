package ir

import (
	"testing"

	"github.com/haramako/fc/internal/types"
)

// TestBuildCFG: ラベル・if・jump・return からブロックと辺が正しく切れる。
//
//	0: label L0        (b0: L0)
//	1: if c goto L2    (b0 → b1, b2)
//	2: x = 1           (b1)
//	3: jump L3         (b1 → b3)
//	4: label L2        (b2: L2)
//	5: x = 2           (b2 → b3)
//	6: label L3        (b3: L3)
//	7: return x        (b3 → なし)
//	8: label L4        (b4: 到達不能)
//	9: jump L0         (b4 → b0)
func TestBuildCFG(t *testing.T) {
	u := types.NewUniverse()
	c := &Value{Name: "c", Kind: KindLocal, Type: u.IntType(1, false), LocalType: LTNone}
	x := &Value{Name: "x", Kind: KindLocal, Type: u.IntType(1, false), LocalType: LTNone}
	one := NewIntLiteral("", u.IntType(1, false), 1)
	lmd := &Lambda{Ops: []*Op{
		{Code: OpLabel, Label: "L0"},
		{Code: OpIf, Src: []Operand{c}, Label: "L2"},
		{Code: OpLoad, Dst: x, Src: []Operand{one}},
		{Code: OpJump, Label: "L3"},
		{Code: OpLabel, Label: "L2"},
		{Code: OpLoad, Dst: x, Src: []Operand{one}},
		{Code: OpLabel, Label: "L3"},
		{Code: OpReturn, Src: []Operand{x}},
		{Code: OpLabel, Label: "L4"},
		{Code: OpJump, Label: "L0"},
	}}
	cfg := BuildCFG(lmd)
	if len(cfg.Blocks) != 5 {
		t.Fatalf("blocks = %d, want 5", len(cfg.Blocks))
	}
	want := [][]int{{1, 2}, {3}, {3}, {}, {0}}
	for i, b := range cfg.Blocks {
		var got []int
		for _, s := range b.Succs {
			got = append(got, s.Index)
		}
		if len(got) != len(want[i]) {
			t.Errorf("b%d succs = %v, want %v", i, got, want[i])
			continue
		}
		for j := range got {
			if got[j] != want[i][j] {
				t.Errorf("b%d succs = %v, want %v", i, got, want[i])
			}
		}
	}
	if cfg.BlockOf("L3") != cfg.Blocks[3] || cfg.Blocks[3].Start != 6 || cfg.Blocks[3].End != 8 {
		t.Errorf("L3 = %+v", cfg.Blocks[3])
	}
	if len(cfg.Blocks[0].Preds) != 1 || cfg.Blocks[0].Preds[0] != cfg.Blocks[4] {
		t.Errorf("b0 preds = %v", cfg.Blocks[0].Preds)
	}
	r := cfg.Reachable()
	if !r[cfg.Blocks[3]] || r[cfg.Blocks[4]] {
		t.Errorf("reachable: b3=%v b4=%v", r[cfg.Blocks[3]], r[cfg.Blocks[4]])
	}

	ud := BuildUseDef(lmd)
	if d := ud.Defs[x]; len(d) != 2 || d[0] != 2 || d[1] != 5 {
		t.Errorf("defs(x) = %v", d)
	}
	if u, ok := ud.SingleUse(x); !ok || u != 7 {
		t.Errorf("uses(x) = %v", ud.Uses[x])
	}
	if u, ok := ud.SingleUse(c); !ok || u != 1 {
		t.Errorf("uses(c) = %v", ud.Uses[c])
	}
}
