package ir

import (
	"testing"

	"github.com/haramako/fc/internal/types"
)

// TestOpInfoConsistency: 命令の性質の表 (opinfo.go) の整合。全部の命令に名前と行があり、分類の包含関係が崩れていないこと、
// 副作用の無い命令は DefUse で Dst を定義すること (結果を捨てられる = 消してよい、の前提)。
func TestOpInfoConsistency(t *testing.T) {
	for c := OpCode(1); c < opCodeCount; c++ {
		if opCodeNames[c] == "" {
			t.Errorf("op %d に名前が無い", c)
		}
		if c.IsCondBranch() && !c.IsBranch() {
			t.Errorf("%s: 条件分岐なのに IsBranch でない", c)
		}
		if c.IsBranch() && !c.IsTerminator() {
			t.Errorf("%s: 分岐なのに IsTerminator でない", c)
		}
		if c.IsPure() && (c.IsTerminator() || c.IsCall() || c.MayTouchGlobals() || c.IsOpaque()) {
			t.Errorf("%s: 副作用が無いはずの命令が終端 / 呼び出し / グローバル書き込み / asm", c)
		}
		if c.IsCommutative() && !c.IsPure() {
			t.Errorf("%s: 可換な演算は副作用が無いはず", c)
		}
		if c.IsPure() {
			d := NewIntLiteral("", types.NewUniverse().IntType(1, false), 0)
			op := &Op{Code: c, Dst: d, Src: []Operand{d, d, d}}
			defs, _ := DefUse(op)
			if len(defs) != 1 || defs[0] != Operand(d) {
				t.Errorf("%s: 副作用の無い命令なのに DefUse が Dst を定義しない", c)
			}
		}
	}
}
