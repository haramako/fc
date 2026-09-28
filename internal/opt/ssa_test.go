package opt

import (
	"testing"

	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/types"
)

func s8() *types.Type { return tu.IntType(1, true) }

func op(code ir.OpCode, dst ir.Operand, srcs ...ir.Operand) *ir.Op {
	return ir.InferWidthSign(&ir.Op{Code: code, Dst: dst, Src: srcs}) // 比較の幅と符号は sema と同じく型から (ir/sign.go)
}

func label(l string) *ir.Op             { return &ir.Op{Code: ir.OpLabel, Label: l} }
func jump(l string) *ir.Op              { return &ir.Op{Code: ir.OpJump, Label: l} }
func ifz(c ir.Operand, l string) *ir.Op { return &ir.Op{Code: ir.OpIf, Src: []ir.Operand{c}, Label: l} }
func ifnz(c ir.Operand, l string) *ir.Op {
	return &ir.Op{Code: ir.OpIfTrue, Src: []ir.Operand{c}, Label: l}
}
func ret(v ir.Operand) *ir.Op { return &ir.Op{Code: ir.OpReturn, Src: []ir.Operand{v}} }
func pushArg(t *types.Type, v ir.Operand) *ir.Op {
	return &ir.Op{Code: ir.OpPushArg, Type: t, Src: []ir.Operand{v}}
}

// 直線コードの定数畳み込み: 中間の定義は全部消える
func TestSSAConstStraight(t *testing.T) {
	x, y, tv := local("x", u8()), local("y", u8()), tmp("t", u8())
	lmd := lambda(
		op(ir.OpLoad, x, lit(5, u8())),
		op(ir.OpAdd, tv, x, lit(3, u8())),
		op(ir.OpLoad, y, tv),
		ret(y),
	)
	propagateSSA(lmd)
	check(t, lmd, "return #8")
}

// 型のサイズで折り返す。符号付きの読み替え (CastedValue) はビット列の解釈
func TestSSAConstWrap(t *testing.T) {
	x, tv, u := local("x", u8()), tmp("t", u8()), tmp("u", s8())
	lmd := lambda(
		op(ir.OpLoad, x, lit(250, u8())),
		op(ir.OpAdd, tv, x, lit(10, u8())), // 260 → 4
		pushArg(u16(), tv),
		op(ir.OpLoad, u, ir.NewCastedValue(x, s8(), 0)), // 250 を sint8 で読む → -6
		op(ir.OpShiftRight, u, u, lit(1, u8())),         // -3
		pushArg(u16(), u),
	)
	propagateSSA(lmd)
	check(t, lmd, "push_arg nil = #4", "push_arg nil = #-3")
}

// 比較・除算・右シフトの意味は命令の Width / Sign で決まり、入力の型では決まらない (ir/sign.go)。入力を型の違う値に
// 差し替えても (常駐の差し替えが cast を落とす、propagateBytes が一時変数を型の違うリテラルにする) 結果は変わらない
func TestSSASignFromOp(t *testing.T) {
	x, a, b, c, d := local("x", u8()), tmp("a", u8()), tmp("b", u8()), tmp("c", u8()), tmp("d", u8())
	lmd := lambda(
		op(ir.OpLoad, x, lit(100, u8())),
		// 入力は u8 だが、sema が i8 同士の比較として作ったもの: 100 < -56 は偽
		&ir.Op{Code: ir.OpLt, Dst: a, Src: []ir.Operand{x, lit(200, u8())}, Width: 1, Sign: ir.Signed},
		pushArg(u8(), a),
		// 2 バイトの比較として作ったもの (入力が 1 バイトに狭まっても幅は 2): リテラル -56 は 2 バイトで $ffc8 (-56)、
		// x はゼロ拡張で 100。-56 < 100 は真 (型の幅 1 で読むと 200 < 100 で偽)
		&ir.Op{Code: ir.OpLt, Dst: b, Src: []ir.Operand{lit(-56, u8()), x}, Width: 2, Sign: ir.Signed},
		pushArg(u8(), b),
		// 算術右シフト: 200 (-56) >> 1 = -28 (228)
		&ir.Op{Code: ir.OpShiftRight, Dst: c, Src: []ir.Operand{lit(200, u8()), lit(1, u8())}, Sign: ir.Signed},
		pushArg(u8(), c),
		// 符号付きの床除算: -56 / 100 = -1 (255)
		&ir.Op{Code: ir.OpDiv, Dst: d, Src: []ir.Operand{lit(200, u8()), x}, Sign: ir.Signed},
		pushArg(u8(), d),
	)
	propagateSSA(lmd)
	check(t, lmd, "push_arg nil = #0", "push_arg nil = #1", "push_arg nil = #228", "push_arg nil = #255")
}

// cast の連鎖: `((x as int) as i16)` は下位バイトのゼロ拡張 (外側の型だけで読むと符号拡張してしまう)
func TestSSAConstCastChain(t *testing.T) {
	s16 := tu.IntType(2, true)
	x, tv := local("x", u16()), tmp("t", s16)
	lmd := lambda(
		op(ir.OpLoad, x, lit(59037, u16())),
		op(ir.OpXor, tv, ir.NewCastedValue(ir.NewCastedValue(x, u8(), 0), s16, 0), lit(0, s16)),
		pushArg(u16(), tv),
	)
	propagateSSA(lmd)
	check(t, lmd, "push_arg nil = #157")
}

// コピー伝播: `load x = a` の後の x は a に。a が書き換えられた後は伝播しない
func TestSSACopy(t *testing.T) {
	a, c, x, t1, t2 := local("a", u8()), local("c", u8()), local("x", u8()), tmp("t1", u8()), tmp("t2", u8())
	lmd := lambda(
		op(ir.OpLoad, x, a),
		op(ir.OpAdd, t1, x, lit(1, u8())),
		pushArg(u8(), t1),
		op(ir.OpAdd, a, a, c),
		op(ir.OpAdd, t2, x, a),
		pushArg(u8(), t2),
	)
	lmd.Args = []*ir.Value{a, c}
	propagateSSA(lmd)
	check(t, lmd,
		"load x = a",
		"add t1 = a, #1",
		"push_arg nil = t1",
		"add a = a, c",
		"add t2 = x, a",
		"push_arg nil = t2",
	)
}

// 一時変数からのコピーは広げない (coalesceCopies / A 割付の形を保つ)
func TestSSACopyKeepsTemp(t *testing.T) {
	a, x, tv, t2 := local("a", u8()), local("x", u8()), tmp("t", u8()), tmp("t2", u8())
	lmd := lambda(
		op(ir.OpAdd, tv, a, a),
		op(ir.OpLoad, x, tv),
		op(ir.OpAdd, t2, x, lit(1, u8())),
		pushArg(u8(), t2),
	)
	lmd.Args = []*ir.Value{a}
	propagateSSA(lmd)
	check(t, lmd, "add t = a, a", "load x = t", "add t2 = x, #1", "push_arg nil = t2")
}

// φ: 両方の枝で同じ定数なら合流後も定数。ループの中で変わらない変数も定数
func TestSSAConstPhi(t *testing.T) {
	c, x, s, i, tv := local("c", u8()), local("x", u8()), local("s", u8()), local("i", u8()), tmp("t", u8())
	lmd := lambda(
		op(ir.OpLoad, x, lit(3, u8())),
		ifz(c, "@L1"),
		op(ir.OpLoad, x, lit(3, u8())),
		label("@L1"),
		op(ir.OpLoad, s, lit(7, u8())),
		op(ir.OpLoad, i, lit(0, u8())),
		label("@loop"),
		op(ir.OpAdd, i, i, x),
		op(ir.OpLt, tv, i, lit(10, u8())),
		ifnz(tv, "@loop"),
		op(ir.OpAdd, tv, s, i),
		ret(tv),
	)
	lmd.Args = []*ir.Value{c}
	propagateSSA(lmd)
	check(t, lmd,
		"if c @L1",
		"label @L1",
		"load i = #0",
		"label @loop",
		"add i = i, #3",
		"lt t = i, #10",
		"if_true t @loop",
		"add t = #7, i",
		"return t",
	)
}

// 枝で違う値なら伝播しない。使われない定義 (ループの中の自己参照だけのものも) は消える
func TestSSAPhiVarAndDCE(t *testing.T) {
	c, x, d, i, tv := local("c", u8()), local("x", u8()), local("d", u8()), local("i", u8()), tmp("t", u8())
	f := ir.NewGlobal("f", tu.Func(nil, tu.Void(), false), "_f")
	lmd := lambda(
		op(ir.OpLoad, x, lit(3, u8())),
		op(ir.OpLoad, d, lit(9, u8())), // d は使われない
		ifz(c, "@L1"),
		op(ir.OpLoad, x, lit(4, u8())),
		label("@L1"),
		op(ir.OpLoad, i, lit(0, u8())),
		label("@loop"),
		op(ir.OpAdd, i, i, lit(1, u8())), // i はループの中で自分しか使わない
		op(ir.OpCall, nil, f),
		op(ir.OpLt, tv, x, lit(10, u8())),
		ifnz(tv, "@loop"),
		ret(x),
	)
	lmd.Args = []*ir.Value{c}
	propagateSSA(lmd)
	check(t, lmd,
		"load x = #3",
		"if c @L1",
		"load x = #4",
		"label @L1",
		"label @loop",
		"call nil = f",
		"lt t = x, #10",
		"if_true t @loop",
		"return x",
	)
}

// 定数の条件分岐は jump / 削除に。到達しなくなったコードは simplifyJumps が消す (ここでは残るが、φ の入力には数えない)
func TestSSAFoldBranch(t *testing.T) {
	c, x := local("c", u8()), local("x", u8())
	lmd := lambda(
		op(ir.OpLoad, c, lit(0, u8())),
		ifz(c, "@L1"),
		op(ir.OpLoad, x, lit(1, u8())),
		label("@L1"),
		ifnz(c, "@L2"),
		op(ir.OpLoad, x, lit(2, u8())),
		label("@L2"),
		ret(x),
	)
	propagateSSA(lmd)
	check(t, lmd,
		"jump @L1",
		"load x = #1",
		"label @L1",
		"label @L2",
		"return #2",
	)
}

// 代数の簡約: `y / 16 * 16` → `y & 0xf0`、and の連鎖、シフトの連鎖。中間の版が他でも使われていれば残る
func TestSSASimplify(t *testing.T) {
	y, t1, t2, t3, t4, t5 := local("y", u8()), tmp("t1", u8()), tmp("t2", u8()), tmp("t3", u8()), tmp("t4", u8()), tmp("t5", u8())
	lmd := lambda(
		op(ir.OpDiv, t1, y, lit(16, u8())),
		op(ir.OpMul, t2, lit(16, u8()), t1),
		pushArg(u8(), t2),
		op(ir.OpAnd, t3, y, lit(0x3c, u8())),
		op(ir.OpAnd, t4, t3, lit(0x0f, u8())),
		pushArg(u8(), t4),
		pushArg(u8(), t3),
		op(ir.OpShiftRight, t5, t1, lit(2, u8())),
		pushArg(u8(), t5),
	)
	lmd.Args = []*ir.Value{y}
	propagateSSA(lmd)
	check(t, lmd,
		"div t1 = y, #16",
		"and t2 = y, #240",
		"push_arg nil = t2",
		"and t3 = y, #60",
		"and t4 = y, #12",
		"push_arg nil = t4",
		"push_arg nil = t3",
		"shift_right t5 = t1, #2",
		"push_arg nil = t5",
	)
}

// アドレスを取られた変数・一部だけ書かれる変数は対象外
func TestSSAExcluded(t *testing.T) {
	x, w, p, tv := local("x", u8()), local("w", u16()), local("p", tu.PointerTo(u8())), tmp("t", u8())
	lmd := lambda(
		op(ir.OpLoad, x, lit(5, u8())),
		op(ir.OpRef, p, x),
		ir.NewStoreMem(p, nil, 0, 0, 1, lit(1, u8())),
		op(ir.OpAdd, tv, x, lit(1, u8())),
		pushArg(u8(), tv),
		op(ir.OpLoad, w, lit(0, u16())),
		op(ir.OpLoad, ir.NewCastedValue(w, u8(), 1), lit(2, u8())),
		pushArg(u16(), w),
	)
	propagateSSA(lmd)
	check(t, lmd,
		"load x = #5",
		"ref p = x",
		"store_mem p, #1, w=1",
		"add t = x, #1",
		"push_arg nil = t",
		"load w = #0",
		"load w.1:1 = #2",
		"push_arg nil = w",
	)
}
