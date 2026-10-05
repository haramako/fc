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
	if d := ud.DefIndexes(x); len(d) != 2 || d[0] != 2 || d[1] != 5 {
		t.Errorf("defs(x) = %v", d)
	}
	if u, ok := ud.SingleUse(x); !ok || u != lmd.Ops[7] {
		t.Errorf("uses(x) = %v", ud.UseIndexes(x))
	}
	if u, ok := ud.SingleUse(c); !ok || u != lmd.Ops[1] {
		t.Errorf("uses(c) = %v", ud.UseIndexes(c))
	}
}

// TestUseDefIncremental: UseDef は *Op を鍵にするので、命令を消す (nil)・動かす・詰めるのを知らせなくてよい。足した命令は
// Add、書き換えた命令は Update で登録し直す。
func TestUseDefIncremental(t *testing.T) {
	u8 := types.NewUniverse().IntType(1, false)
	x := NewLocal("x", u8, LTNone)
	y := NewLocal("y", u8, LTNone)
	one := NewIntLiteral("", u8, 1)
	a := &Op{Code: OpLoad, Dst: x, Src: []Operand{one}}
	b := &Op{Code: OpAdd, Dst: y, Src: []Operand{x, x}}
	c := &Op{Code: OpReturn, Src: []Operand{y}}
	lmd := &Lambda{Ops: []*Op{a, b, c}}
	ud := BuildUseDef(lmd)
	if n := ud.NumUses(x); n != 2 {
		t.Fatalf("uses(x) = %d, want 2 (同じ命令が 2 回使う)", n)
	}
	// 動かす: 位置の順に返す
	lmd.Ops = []*Op{b, a, c}
	if d := ud.DefIndexes(x); len(d) != 1 || d[0] != 1 {
		t.Errorf("動かした後の defs(x) = %v", d)
	}
	// 消す (nil): 引くときに除かれる
	lmd.Ops[0] = nil
	if n := ud.NumUses(x); n != 0 {
		t.Errorf("消した後の uses(x) = %d", n)
	}
	if lmd.IndexOf(b) != -1 || lmd.IndexOf(c) != 2 {
		t.Errorf("IndexOf: b=%d c=%d", lmd.IndexOf(b), lmd.IndexOf(c))
	}
	// 詰めて足す
	d := &Op{Code: OpLoad, Dst: y, Src: []Operand{x}}
	lmd.Ops = []*Op{a, d, c}
	ud.Add(d)
	if u, ok := ud.SingleUse(x); !ok || u != d {
		t.Errorf("足した後の uses(x) = %v", ud.UseIndexes(x))
	}
	// 書き換え
	d.Src[0] = one
	ud.Update(d)
	if n := ud.NumUses(x); n != 0 {
		t.Errorf("書き換えた後の uses(x) = %d", n)
	}
	// ReplaceOp で戻した命令は見つかる
	ReplaceOp(lmd.Ops, 1, b)
	if lmd.IndexOf(b) != 1 {
		t.Errorf("ReplaceOp で戻した b の位置 = %d", lmd.IndexOf(b))
	}
}
