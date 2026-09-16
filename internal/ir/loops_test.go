package ir

import (
	"testing"

	"github.com/haramako/fc/internal/types"
)

// TestLoopsAndLiveness: 回転済みの while ループ (hlc + opt の形) でループ・入口・出口・生存が取れる。
//
//	0: load i = #0
//	1: jump @begin
//	2: label @body
//	3: add s = s, i          (s はループ内で更新)
//	4: add i = i, #1
//	5: label @begin          (ヘッダ = 末尾の条件)
//	6: lt c = i, #10
//	7: if_true c @body
//	8: return s
func TestLoopsAndLiveness(t *testing.T) {
	u := types.NewUniverse()
	u8 := u.IntType(1, false)
	i := NewLocal("i", u8, LTNone)
	s := NewLocal("s", u8, LTNone)
	c := NewLocal("c", u8, LTTemp)
	lmd := &Lambda{Ops: []*Op{
		{Code: OpLoad, Dst: i, Src: []Operand{NewIntLiteral("", u8, 0)}},
		{Code: OpJump, Label: "@begin"},
		{Code: OpLabel, Label: "@body"},
		{Code: OpAdd, Dst: s, Src: []Operand{s, i}},
		{Code: OpAdd, Dst: i, Src: []Operand{i, NewIntLiteral("", u8, 1)}},
		{Code: OpLabel, Label: "@begin"},
		{Code: OpLt, Dst: c, Src: []Operand{i, NewIntLiteral("", u8, 10)}},
		{Code: OpIfTrue, Src: []Operand{c}, Label: "@body"},
		{Code: OpReturn, Src: []Operand{s}},
	}}
	cfg := BuildCFG(lmd)
	loops := cfg.Loops()
	if len(loops) != 1 {
		t.Fatalf("loops = %d", len(loops))
	}
	lp := loops[0]
	if lp.Header != cfg.BlockOf("@begin") || len(lp.Blocks) != 2 || !lp.Blocks[cfg.BlockOf("@body")] {
		t.Errorf("loop = header %q blocks %d", lp.Header.Label, len(lp.Blocks))
	}
	if ent := cfg.Entries(lp); len(ent) != 1 || ent[0] != cfg.Blocks[0] {
		t.Errorf("entries = %v", ent)
	}
	if ex := cfg.Exits(lp); len(ex) != 1 || ex[0][0] != lp.Header || ex[0][1].Start != 8 {
		t.Errorf("exits = %v", ex)
	}
	if inner := Innermost(loops); len(inner) != 1 {
		t.Errorf("innermost = %d", len(inner))
	}

	lv := BuildLiveness(lmd)
	// s はループ全体で生きている (入口で live-in、出口でも)。c は 6 の出口と 7 の入口だけ
	if !lv.LiveIn(3, s) || !lv.LiveOut(4, s) || !lv.LiveIn(6, s) || !lv.LiveOut(7, s) || !lv.LiveIn(8, s) || lv.LiveOut(8, s) {
		t.Errorf("s liveness wrong")
	}
	if lv.LiveIn(6, c) || !lv.LiveOut(6, c) || !lv.LiveIn(7, c) || lv.LiveOut(7, c) {
		t.Errorf("c liveness wrong")
	}
	if !lv.LiveOut(0, i) || !lv.LiveIn(3, i) || lv.LiveIn(8, i) {
		t.Errorf("i liveness wrong")
	}
	// s はループの入口 (0 の出口) で未定義 = 生きていない … ではなく、ループ内で読む前に定義されないので live-in
	if !lv.LiveIn(0, s) {
		t.Errorf("s should be live at entry (read before written in the loop)")
	}
}
