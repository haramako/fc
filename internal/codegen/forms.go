package codegen

// regalloc の形 (regalloc/forms.go) を codegen の実際の置き場所と綴りで出す。形は常駐のレジスタを使う・触らない特別な出し方
// (Y に常駐する変数の ldy / cpy / iny、A が塞がっているときの Y での代用、A を使わないメモリ上の inc / シフト / rol …) で、
// regalloc の常駐の見積もりと同じ表を引く (以前は genLoad / genIf / genEq / genLt / incDec (inc / dec の出し方) の分岐と、
// regalloc の予測表の 2 か所にあった)。

import (
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/regalloc"
)

// placement は形を選ぶための実際の置き場所 (割付の後。退避中の常駐はメモリ側 = PMem)。
type placement struct{ l *Llc }

func (p placement) Where(o ir.Operand) regalloc.Place {
	switch {
	case o == nil:
		return regalloc.PMem
	case p.l.inA(o):
		return regalloc.PA
	case p.l.inY(o):
		return regalloc.PY
	case p.l.inX(o):
		return regalloc.PX
	case isValueOrCasted(o) && ir.ValKind(o) == ir.KindLocal && ir.ValLocation(o) == ir.LocCond:
		return regalloc.PCond
	}
	return regalloc.PMem
}

// emitter は形の命令のオペランドを実際の綴りで作る。
type emitter struct{ l *Llc }

func (e emitter) Byte(o ir.Operand, i int) regalloc.Arg {
	t := e.l.byte(o, i)
	return regalloc.Arg{Text: t, Mode: parseOperand(t).mode()}
}

func (e emitter) NewLabel() string                     { return e.l.newLabel() }
func (e emitter) SameText(a, b ir.Operand, i int) bool { return e.l.byte(a, i) == e.l.byte(b, i) }
func (e emitter) SameByte(a, b ir.Operand, i int) bool { return e.l.sameByte(a, b, i) }

// place / emit は形を選ぶ・出すための置き場所と綴り。
func (l *Llc) place() placement { return placement{l} }
func (l *Llc) emit() emitter    { return emitter{l} }

// emitForm は形の命令列を asm の行にする。
func (l *Llc) emitForm(f regalloc.Form) []any {
	var r []any
	for _, in := range f.Emit(l.emit()) {
		if in.Mnem == "" {
			r = append(r, in.Label+":")
			continue
		}
		t := in.Mnem
		if in.Arg.Text != "" {
			t += " " + in.Arg.Text
		}
		if in.Test {
			r = append(r, markTest(t))
		} else {
			r = append(r, t)
		}
	}
	return r
}
