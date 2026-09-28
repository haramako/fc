package regalloc

// 常駐のレジスタを使う・触らない特別な出し方 (形) の表 (2026-09-28。doc/development_notes.md「コードの構造」)。
//
// codegen は命令をこの形で出し (genLoad / genIf / genEq / genLt / genAddSub / genShift / genRotateCarry)、regalloc は同じ形から
// 「変数をレジスタに置いたまま実行できるか」(Classify の friendly)・「A を触らないか」(free) と、その得 (常駐させないときの
// 命令列とのサイクル数の差。m6502 の表) を計算する。以前は codegen の分岐と、それを手で写した予測表 (friendlyY / friendlyX /
// yVariant / freeA / isMemShift …。得は 3 / 6 / 1 の手書きの数字) が別々にあり、食い違いを codegen が関数ごと最大 8 回
// コンパイルし直して吸収していた。
//
// 形が決まる条件のうち、どのレジスタに何があるかは Placement で答える: codegen は割付の後の実際の置き場所 (退避中の常駐は
// メモリ)、regalloc は常駐させたときの見込み (候補の変数をそのレジスタに。割付の前の一時変数はメモリ)。比較の結果がフラグに
// 乗るかは、codegen は実際の割付 (LocCond)、regalloc は命令の並び (condPredicted) で渡す。番地が同じか (inc x の x = x + 1) は、
// codegen は出力の綴りで、regalloc は同じ変数・同じずれで見る (regalloc が認める形は codegen でも必ず同じ形になる)。

import (
	"strconv"

	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/m6502"
	"github.com/haramako/fc/internal/types"
)

// Place は命令のオペランドの置き場所 (形を選ぶための分類)。
type Place uint8

const (
	PMem  Place = iota // メモリ (ゼロページ / 絶対 / フレーム) か即値
	PA                 // A
	PY                 // Y
	PX                 // X
	PCond              // 比較の結果のフラグ (直後の if が見る)
)

// Placement はオペランドがどこにあるか。
type Placement interface {
	Where(o ir.Operand) Place
}

// Arg は形の命令のオペランド。
type Arg struct {
	Text string     // codegen の出力の綴り (regalloc の見積もりでは空)
	Mode m6502.Mode // サイクル数を引くアドレッシングモード
}

// Ins は形の命令 1 つ。Mnem が空ならラベルの行 (Label)。
type Ins struct {
	Mnem  string
	Arg   Arg
	Label string
	Test  bool // ピープホールの「検査のためだけのロード」の印 (codegen の markTest)
}

// Emitter は形の命令のオペランドを作る (codegen は実際の綴り、regalloc の見積もりはアドレッシングモードだけ)。
type Emitter interface {
	Byte(o ir.Operand, i int) Arg         // o の i バイト目
	NewLabel() string                     // 局所ラベル
	SameText(a, b ir.Operand, i int) bool // a と b の i バイト目が同じ綴り (その場の inc x / rol x)
	SameByte(a, b ir.Operand, i int) bool // a と b の i バイト目が同じメモリ (写しを省く。レジスタ・即値は false)
}

// FormKind は形の種類。
type FormKind uint8

const (
	FormNone FormKind = iota

	// load
	FormLoadY  // ldy src           Y に常駐する変数への代入
	FormStoreY // sty dst           Y に常駐する変数の値を書く
	FormLoadX  // ldx src
	FormStoreX // stx dst
	FormCopyY  // ldy src; sty dst  A が常駐で塞がっている: Y で写す (2 バイトまで)

	// if / if_true
	FormIfCond // 分岐だけ          比較のフラグ
	FormIfA    // cmp #0; bne
	FormIfY    // cpy #0; bne
	FormIfX    // cpx #0; bne
	FormIfViaY // ldy x; bne        A が塞がっている

	// eq / lt (結果がフラグ)
	FormCmpY     // cpy b         a が Y
	FormCmpX     // cpx b
	FormCmpCommY // cpy a         eq で b が Y
	FormCmpCommX // cpx a
	FormCmpCommA // cmp a         eq で b が A
	FormCmpViaY  // ldy a; cpy b  A が塞がっている

	// add / sub (x = x ± k)
	FormStepY  // iny × k
	FormStepX  // inx × k
	FormIncMem // inc x / 2 バイトの inc・dec (メモリ上のその場の ± 1。1 バイトは inc x の 5 サイクルで clc; lda; adc #1; sta の 10 から。2 バイトの +1 は inc lo; bne; inc hi、-1 は lda lo; bne; dec hi; dec lo で A を使う。直後の検査 (dec x; lda x; bne) の lda はピープホールが消す)

	// shift_left / shift_right (定数)
	FormShiftMem // asl x × n / asl lo; rol hi × n (メモリ上のその場のシフト)

	// rolc / rorc
	FormRotA   // rol a
	FormRotMem // rol x
)

// Form は選んだ形。
type Form struct {
	Kind FormKind
	Op   *ir.Op
	K    int // Step / IncMem / ShiftMem の回数
}

// Reg は形が値を置いたまま使うレジスタ (PMem ならどれも使わない: A を触らない形・Y での代用)。
func (f Form) Reg() Place {
	switch f.Kind {
	case FormLoadY, FormStoreY, FormIfY, FormCmpY, FormCmpCommY, FormStepY:
		return PY
	case FormLoadX, FormStoreX, FormIfX, FormCmpX, FormCmpCommX, FormStepX:
		return PX
	case FormIfA, FormCmpCommA, FormRotA:
		return PA
	}
	return PMem
}

// exact は o が 1 バイトの値そのもの (ずれ 0) か (常駐の変数をレジスタのまま使える形)。
func exact(o ir.Operand) bool {
	return o != nil && ir.ValOffset(o) == 0 && ir.ValType(o).Size == 1
}

// ---------------------------------------------------------------
// 形を選ぶ
// ---------------------------------------------------------------

// LoadForm は load の形。aHeld は A が常駐で塞がっていて、この命令を Y で代用する (Classify の UseY)。
func LoadForm(op *ir.Op, p Placement, aHeld bool) (Form, bool) {
	dst, src := op.Dst, op.In(0)
	switch {
	case p.Where(dst) == PY && exact(dst) && isMemOrLit(src) && ir.ValType(src).Size == 1:
		return Form{Kind: FormLoadY, Op: op}, true
	case p.Where(src) == PY && exact(src) && isMemByte(dst):
		return Form{Kind: FormStoreY, Op: op}, true
	case p.Where(dst) == PX && exact(dst) && isMemOrLit(src) && ir.ValType(src).Size == 1:
		return Form{Kind: FormLoadX, Op: op}, true
	case p.Where(src) == PX && exact(src) && isMemByte(dst):
		return Form{Kind: FormStoreX, Op: op}, true
	case aHeld && copyViaY(op):
		return Form{Kind: FormCopyY, Op: op}, true
	}
	return Form{}, false
}

// copyViaY は load を Y で写せるか (メモリか即値からメモリへ、2 バイトまで)。
func copyViaY(op *ir.Op) bool {
	src := op.In(0)
	return isMemOrLit(src) && isMemOrLit(op.Dst) && ir.ValType(op.Dst).Size <= 2 && ir.ValType(src).Kind != types.Array
}

// IfForm は if / if_true の形。
func IfForm(op *ir.Op, p Placement, aHeld bool) (Form, bool) {
	src := op.In(0)
	switch w := p.Where(src); {
	case w == PCond:
		return Form{Kind: FormIfCond, Op: op}, true
	case w == PA && exact(src):
		return Form{Kind: FormIfA, Op: op}, true
	case w == PY && exact(src):
		return Form{Kind: FormIfY, Op: op}, true
	case w == PX && exact(src):
		return Form{Kind: FormIfX, Op: op}, true
	case aHeld && ifViaY(op):
		return Form{Kind: FormIfViaY, Op: op}, true
	}
	return Form{}, false
}

// ifViaY は if を Y で検査できるか (1 バイトのメモリ上の値)。
func ifViaY(op *ir.Op) bool { return isMemByte(op.In(0)) }

// CmpForm は結果がフラグに乗る (toFlags) eq / lt の形。
func CmpForm(op *ir.Op, p Placement, toFlags, aHeld bool) (Form, bool) {
	if !toFlags {
		return Form{}, false
	}
	a, b := op.In(0), op.In(1)
	eq := op.Code == ir.OpEq
	unsignedOrEq := eq || !op.IsSigned()
	switch {
	case p.Where(a) == PY && exact(a) && cmpOperand(b) && ir.ValType(b).Size == 1 && unsignedOrEq:
		return Form{Kind: FormCmpY, Op: op}, true
	case p.Where(a) == PX && exact(a) && cmpOperand(b) && ir.ValType(b).Size == 1 && unsignedOrEq:
		return Form{Kind: FormCmpX, Op: op}, true
	case eq && p.Where(b) == PY && exact(b) && cmpOperand(a) && ir.ValType(a).Size == 1:
		return Form{Kind: FormCmpCommY, Op: op}, true
	case eq && p.Where(b) == PX && exact(b) && cmpOperand(a) && ir.ValType(a).Size == 1:
		return Form{Kind: FormCmpCommX, Op: op}, true
	case eq && p.Where(b) == PA && exact(b) && isMemOrLit(a) && ir.ValType(a).Size == 1:
		return Form{Kind: FormCmpCommA, Op: op}, true
	case aHeld && cmpViaY(op):
		return Form{Kind: FormCmpViaY, Op: op}, true
	}
	return Form{}, false
}

// cmpViaY は比較を Y で (ldy a; cpy b) できるか (1 バイト同士で、符号付きの lt は除く)。
func cmpViaY(op *ir.Op) bool {
	a, b := op.In(0), op.In(1)
	return isMemOrLit(a) && cmpOperand(b) && ir.ValType(a).Size == 1 && ir.ValType(b).Size == 1 && (op.Code == ir.OpEq || !op.IsSigned())
}

// StepForm は x = x ± k の形。maxK は Y / X に常駐する添字の iny × k の上限 (StepLimit)。
func StepForm(op *ir.Op, p Placement, e Emitter, maxK int) (Form, bool) {
	if op.Code != ir.OpAdd && op.Code != ir.OpSub || len(op.Src) != 2 {
		return Form{}, false
	}
	k, lit := ir.ValIntLiteral(op.In(1))
	dst, src := op.Dst, op.In(0)
	if !lit || k < 1 || ir.ValType(dst).Size > 2 || !valueOrCast(dst) || !valueOrCast(src) {
		return Form{}, false
	}
	switch {
	case p.Where(dst) == PY && exact(dst) && p.Where(src) == PY && isStep(op, maxK):
		return Form{Kind: FormStepY, Op: op, K: k}, true
	case p.Where(dst) == PX && exact(dst) && p.Where(src) == PX && isStep(op, maxK):
		return Form{Kind: FormStepX, Op: op, K: k}, true
	}
	if k != 1 {
		return Form{}, false
	}
	for _, v := range []ir.Operand{dst, src} {
		if ir.ValKind(v) == ir.KindLiteral || p.Where(v) != PMem {
			return Form{}, false
		}
	}
	for i := 0; i < ir.ValType(dst).Size; i++ {
		if !e.SameText(dst, src, i) {
			return Form{}, false
		}
	}
	return Form{Kind: FormIncMem, Op: op, K: 1}, true
}

// StepLimit は Y / X に常駐する添字の `i += k` を iny × k にする k の上限 (FC_DISABLE=step で 1)。
func StepLimit(lmd *ir.Lambda) int { return stepMax(lmd) }

// ShiftMemForm は定数シフトをメモリ上のその場で出す形 (x = x << n。符号付きの右シフトと 2 バイトの 8 以上は除く: isMemShift)。
func ShiftMemForm(op *ir.Op, p Placement) (Form, bool) {
	if !isMemShift(op) || !valueOrCast(op.Dst) || p.Where(op.Dst) != PMem {
		return Form{}, false
	}
	n, _ := ir.ValIntLiteral(op.In(1))
	return Form{Kind: FormShiftMem, Op: op, K: n}, true
}

// RotForm は rolc / rorc の形。
func RotForm(op *ir.Op, p Placement, e Emitter) (Form, bool) {
	dst, src := op.Dst, op.In(0)
	switch {
	case p.Where(dst) == PA && exact(dst) && p.Where(src) == PA && exact(src):
		return Form{Kind: FormRotA, Op: op}, true
	case valueOrCast(dst) && p.Where(dst) == PMem && ir.ValKind(dst) != ir.KindLiteral && e.SameText(dst, src, 0):
		return Form{Kind: FormRotMem, Op: op}, true
	}
	return Form{}, false
}

// valueOrCast は変数か変数の一部 (リテラルも含む。配列のポインタへの変換は含まない)。
func valueOrCast(o ir.Operand) bool {
	switch o.(type) {
	case *ir.Value, *ir.CastedValue:
		return true
	}
	return false
}

// ---------------------------------------------------------------
// 形の命令列
// ---------------------------------------------------------------

func ins(mnem string, a Arg) Ins { return Ins{Mnem: mnem, Arg: a} }
func imp(mnem string) Ins        { return Ins{Mnem: mnem, Arg: Arg{Mode: m6502.Imp}} }
func acc(mnem string) Ins        { return Ins{Mnem: mnem, Arg: Arg{Text: "a", Mode: m6502.Imp}} } // asl a など
func imm(k int) Arg              { return Arg{Text: "#" + strconv.Itoa(k), Mode: m6502.Imm} }
func rel(label string) Arg       { return Arg{Text: label, Mode: m6502.Rel} }

// branch は if の分岐 (if_true は値が 0 でなければ、if は 0 なら飛ぶ)。
func branch(op *ir.Op) Ins {
	if op.Code == ir.OpIfTrue {
		return ins("bne", rel(op.Label))
	}
	return ins("beq", rel(op.Label))
}

func rotMnem(op *ir.Op) string {
	if op.Code == ir.OpRolC {
		return "rol"
	}
	return "ror"
}

// Emit は形の命令列。
func (f Form) Emit(e Emitter) []Ins {
	op := f.Op
	switch f.Kind {
	case FormLoadY:
		return []Ins{ins("ldy", e.Byte(op.In(0), 0))}
	case FormStoreY:
		return []Ins{ins("sty", e.Byte(op.Dst, 0))}
	case FormLoadX:
		return []Ins{ins("ldx", e.Byte(op.In(0), 0))}
	case FormStoreX:
		return []Ins{ins("stx", e.Byte(op.Dst, 0))}
	case FormCopyY:
		var r []Ins
		for i := 0; i < ir.ValType(op.Dst).Size; i++ {
			if !e.SameByte(op.Dst, op.In(0), i) {
				r = append(r, ins("ldy", e.Byte(op.In(0), i)), ins("sty", e.Byte(op.Dst, i)))
			}
		}
		return r
	case FormIfCond:
		return []Ins{ins(CondBranch(ir.UnderlyingValue(op.In(0)), op.Code == ir.OpIfTrue), rel(op.Label))}
	case FormIfA:
		return []Ins{ins("cmp", imm(0)), branch(op)}
	case FormIfY:
		return []Ins{ins("cpy", imm(0)), branch(op)}
	case FormIfX:
		return []Ins{ins("cpx", imm(0)), branch(op)}
	case FormIfViaY:
		t := ins("ldy", e.Byte(op.In(0), 0))
		t.Test = true
		return []Ins{t, branch(op)}
	case FormCmpY:
		return []Ins{ins("cpy", e.Byte(op.In(1), 0))}
	case FormCmpX:
		return []Ins{ins("cpx", e.Byte(op.In(1), 0))}
	case FormCmpCommY:
		return []Ins{ins("cpy", e.Byte(op.In(0), 0))}
	case FormCmpCommX:
		return []Ins{ins("cpx", e.Byte(op.In(0), 0))}
	case FormCmpCommA:
		return []Ins{ins("cmp", e.Byte(op.In(0), 0))}
	case FormCmpViaY:
		return []Ins{ins("ldy", e.Byte(op.In(0), 0)), ins("cpy", e.Byte(op.In(1), 0))}
	case FormStepY, FormStepX:
		mn := map[FormKind][2]string{FormStepY: {"iny", "dey"}, FormStepX: {"inx", "dex"}}[f.Kind]
		m := mn[0]
		if op.Code == ir.OpSub {
			m = mn[1]
		}
		r := make([]Ins, f.K)
		for i := range r {
			r[i] = imp(m)
		}
		return r
	case FormIncMem:
		lo := e.Byte(op.Dst, 0)
		if ir.ValType(op.Dst).Size == 1 {
			if op.Code == ir.OpAdd {
				return []Ins{ins("inc", lo)}
			}
			return []Ins{ins("dec", lo)}
		}
		hi, skip := e.Byte(op.Dst, 1), e.NewLabel()
		if op.Code == ir.OpAdd {
			return []Ins{ins("inc", lo), ins("bne", rel(skip)), ins("inc", hi), {Label: skip}}
		}
		return []Ins{ins("lda", lo), ins("bne", rel(skip)), ins("dec", hi), {Label: skip}, ins("dec", lo)}
	case FormShiftMem:
		left := op.Code == ir.OpShiftLeft
		var r []Ins
		for k := 0; k < f.K; k++ {
			switch {
			case ir.ValType(op.Dst).Size == 1 && left:
				r = append(r, ins("asl", e.Byte(op.Dst, 0)))
			case ir.ValType(op.Dst).Size == 1:
				r = append(r, ins("lsr", e.Byte(op.Dst, 0)))
			case left:
				r = append(r, ins("asl", e.Byte(op.Dst, 0)), ins("rol", e.Byte(op.Dst, 1)))
			default:
				r = append(r, ins("lsr", e.Byte(op.Dst, 1)), ins("ror", e.Byte(op.Dst, 0)))
			}
		}
		return r
	case FormRotA:
		return []Ins{acc(rotMnem(op))}
	case FormRotMem:
		return []Ins{ins(rotMnem(op), e.Byte(op.Dst, 0))}
	}
	return nil
}

// base は形を使わない (常駐させない: 変数がメモリにある) ときの命令列と、命令列に出ない得 (得の見積もり用。両方に共通の
// 部分は省いてよい)。形の得 = base のサイクル数 − 形のサイクル数 + bonus。nil なら得を数えない形 (A を触らない形・Y での代用)。
func (f Form) base(e Emitter) (r []Ins, bonus int) {
	op := f.Op
	switch f.Kind {
	case FormLoadY, FormLoadX:
		return []Ins{ins("lda", e.Byte(op.In(0), 0)), ins("sta", e.Byte(op.Dst, 0))}, 0 // sta v が消える
	case FormStoreY, FormStoreX:
		return []Ins{ins("lda", e.Byte(op.In(0), 0)), ins("sta", e.Byte(op.Dst, 0))}, 0 // lda v が消える
	case FormIfA:
		return []Ins{ins("lda", e.Byte(op.In(0), 0)), branch(op)}, 0
	case FormIfY, FormIfX:
		// 直前が iny / dey なら cpy #0 はピープホールが消す (その分を 1 と見る)
		return []Ins{ins("lda", e.Byte(op.In(0), 0)), branch(op)}, 1
	case FormCmpY, FormCmpX, FormCmpCommY, FormCmpCommX, FormCmpCommA:
		return []Ins{ins("lda", e.Byte(op.In(0), 0)), ins("cmp", e.Byte(op.In(1), 0))}, 0
	case FormStepY, FormStepX:
		if f.K == 1 {
			return []Ins{ins("inc", e.Byte(op.Dst, 0))}, 0
		}
		return []Ins{ins("lda", e.Byte(op.Dst, 0)), imp("clc"), ins("adc", imm(f.K)), ins("sta", e.Byte(op.Dst, 0))}, 0
	case FormRotA:
		return []Ins{ins(rotMnem(op), e.Byte(op.Dst, 0))}, 0
	}
	return nil, 0
}

// ---------------------------------------------------------------
// 形の性質 (regalloc の見積もり)
// ---------------------------------------------------------------

// describe は regalloc の見積もりの Emitter: オペランドはゼロページ (リテラルなら即値) と見る (見積もりの数字は
// ゼロページのサイクル数)。番地が同じかは、同じ変数・同じずれ・そのまま読むオペランドどうしか。
type describe struct{}

func (describe) Byte(o ir.Operand, i int) Arg {
	if ir.ValKind(o) == ir.KindLiteral {
		return Arg{Mode: m6502.Imm}
	}
	return Arg{Mode: m6502.ZP}
}

func (describe) NewLabel() string { return "@f" }

func (describe) SameText(a, b ir.Operand, i int) bool {
	ua, ub := ir.UnderlyingValue(a), ir.UnderlyingValue(b)
	return ua != nil && ua == ub && ir.ValOffset(a) == ir.ValOffset(b) && ir.ValType(a).Size == ir.ValType(b).Size &&
		ir.PlainOperand(a) && ir.PlainOperand(b)
}

func (d describe) SameByte(a, b ir.Operand, i int) bool { return d.SameText(a, b, i) }

// Cycles は命令列のサイクル数 (m6502。分岐は成立しない 2、ラベルは 0)。
func Cycles(r []Ins) int {
	n := 0
	for _, x := range r {
		if x.Mnem != "" {
			n += m6502.Cycles(x.Mnem, x.Arg.Mode)
		}
	}
	return n
}

// Writes は命令列が書くレジスタ (m6502)。
func Writes(r []Ins) (a, x, y bool) {
	for _, in := range r {
		if in.Mnem == "" {
			continue
		}
		wa, wx, wy := m6502.Writes(in.Mnem, in.Arg.Mode)
		a, x, y = a || wa, x || wx, y || wy
	}
	return
}

// Gain は形を使うことで節約できるサイクル数の目安 (常駐の損得の計算)。
func (f Form) Gain() int {
	b, bonus := f.base(describe{})
	if b == nil {
		return 0
	}
	return Cycles(b) - Cycles(f.Emit(describe{})) + bonus
}

// WritesA は形が A を書くか (A に常駐する変数を壊すか)。
func (f Form) WritesA() bool {
	a, _, _ := Writes(f.Emit(describe{}))
	return a
}

// CondBranch はフラグ (LocCond の値 v) が真 (onTrue) / 偽のときに飛ぶ分岐のニーモニック。比較 a < b は符号なしなら
// C クリアで真 (CondCarry の CondPositive は「真 ⇔ C クリア」。allocateCond)。
func CondBranch(v *ir.Value, onTrue bool) string {
	trueIsSet := v.CondPositive
	if v.CondReg == ir.CondCarry {
		trueIsSet = !v.CondPositive
	}
	jumpIfSet := trueIsSet == onTrue
	pick := func(set, clear string) string {
		if jumpIfSet {
			return set
		}
		return clear
	}
	switch v.CondReg {
	case ir.CondZero:
		return pick("beq", "bne")
	case ir.CondCarry:
		return pick("bcs", "bcc")
	case ir.CondNegative:
		return pick("bmi", "bpl")
	}
	panic("invalid cond_reg")
}
