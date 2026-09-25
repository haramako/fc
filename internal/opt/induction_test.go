package opt

import (
	"testing"

	"github.com/haramako/fc/internal/ir"
)

// 比較にしか使われないカウンタ i を、同じ歩幅のポインタ p の比較に置き換えて消す。初期値がリテラルなら入口の検査は無い
func TestInductionLiteralStart(t *testing.T) {
	i, p, tv := local("i", u16()), local("p", tu.PointerTo(u8())), tmp("t", u8())
	lmd := lambda(
		op(ir.OpLoad, i, lit(0, u16())),
		label("@begin"),
		op(ir.OpLt, tv, i, lit(100, u16())),
		ifz(tv, "@end"),
		op(ir.OpPset, nil, p, lit(1, u8())),
		op(ir.OpAdd, p, p, lit(1, u8())),
		op(ir.OpAdd, i, i, lit(1, u8())),
		jump("@begin"),
		label("@end"),
		ret(p),
	)
	lmd.Args = []*ir.Value{p}
	propagateSSA(lmd)
	if !eliminateInduction(lmd) {
		t.Fatal("not transformed")
	}
	propagateSSA(lmd)
	check(t, lmd,
		"add $lim = p, #100",
		"label @begin",
		"lt t = p, $lim",
		"if t @end",
		"pset p, #1",
		"add p = p, #1",
		"jump @begin",
		"label @end",
		"return p",
	)
}

// 初期値が変数なら入口で 1 度検査する。歩幅が変数のときはカウンタより狭い型でなければならない
func TestInductionGuard(t *testing.T) {
	i, k0, s, p, tv := local("i", u16()), local("k0", u16()), local("s", u8()), local("p", tu.PointerTo(u8())), tmp("t", u8())
	lmd := lambda(
		op(ir.OpLoad, i, k0),
		label("@begin"),
		op(ir.OpLt, tv, i, lit(1000, u16())),
		ifz(tv, "@end"),
		op(ir.OpPset, nil, p, lit(1, u8())),
		op(ir.OpAdd, p, p, s),
		op(ir.OpAdd, i, i, s),
		jump("@begin"),
		label("@end"),
		ret(p),
	)
	lmd.Args = []*ir.Value{k0, s, p}
	propagateSSA(lmd)
	if !eliminateInduction(lmd) {
		t.Fatal("not transformed")
	}
	propagateSSA(lmd)
	check(t, lmd,
		"lt $guard = k0, #1000",
		"if $guard @end",
		"sub $diff = #1000, k0",
		"add $lim = p, $diff",
		"label @begin",
		"lt t = p, $lim",
		"if t @end",
		"pset p, #1",
		"add p = p, s",
		"jump @begin",
		"label @end",
		"return p",
	)
}

// 上限がループ不変の変数 (crc8 の `i < length`) でも、歩幅が 1 なら置き換える (k < LIM なので k + 1 は折り返さない)。
// 初期値が 0 なら入口の検査は要らない。0 でないリテラルなら入口で 1 度検査する。歩幅が 1 でなければ対象外
func TestInductionVariableLimit(t *testing.T) {
	build := func(k0, step int) (*ir.Lambda, bool) {
		i, n, p, tv := local("i", u16()), local("n", u16()), local("p", tu.PointerTo(u8())), tmp("t", u8())
		lmd := lambda(
			op(ir.OpLoad, i, lit(k0, u16())),
			label("@begin"),
			op(ir.OpLt, tv, i, n),
			ifz(tv, "@end"),
			op(ir.OpPset, nil, p, lit(1, u8())),
			op(ir.OpAdd, p, p, lit(step, u8())),
			op(ir.OpAdd, i, i, lit(step, u8())),
			jump("@begin"),
			label("@end"),
			ret(p),
		)
		lmd.Args = []*ir.Value{n, p}
		propagateSSA(lmd)
		ok := eliminateInduction(lmd)
		propagateSSA(lmd)
		return lmd, ok
	}
	lmd, ok := build(0, 1)
	if !ok {
		t.Fatal("k0 = 0: not transformed")
	}
	check(t, lmd,
		"add $lim = p, n",
		"label @begin",
		"lt t = p, $lim",
		"if t @end",
		"pset p, #1",
		"add p = p, #1",
		"jump @begin",
		"label @end",
		"return p",
	)
	lmd, ok = build(3, 1)
	if !ok {
		t.Fatal("k0 = 3: not transformed")
	}
	check(t, lmd,
		"lt $guard = #3, n",
		"if $guard @end",
		"sub $diff = n, #3",
		"add $lim = p, $diff",
		"label @begin",
		"lt t = p, $lim",
		"if t @end",
		"pset p, #1",
		"add p = p, #1",
		"jump @begin",
		"label @end",
		"return p",
	)
	if _, ok := build(0, 2); ok {
		t.Error("歩幅 2 (変数の上限で折り返しうる) を置き換えた")
	}
}

// 対象外: カウンタが本体でも使われる、歩幅の型がカウンタと同じ幅 (折り返しうる)、上限 + 歩幅が型に収まらない
func TestInductionNotApplicable(t *testing.T) {
	mk := func(step ir.Operand, lim int, useI bool) *ir.Lambda {
		i, p, tv, u := local("i", u16()), local("p", tu.PointerTo(u8())), tmp("t", u8()), tmp("u", u16())
		ops := []*ir.Op{
			op(ir.OpLoad, i, lit(0, u16())),
			label("@begin"),
			op(ir.OpLt, tv, i, lit(lim, u16())),
			ifz(tv, "@end"),
			op(ir.OpPset, nil, p, lit(1, u8())),
		}
		if useI {
			ops = append(ops, op(ir.OpAdd, u, i, lit(1, u8())), pushArg(u16(), u))
		}
		ops = append(ops,
			op(ir.OpAdd, p, p, step),
			op(ir.OpAdd, i, i, step),
			jump("@begin"),
			label("@end"),
			ret(p),
		)
		lmd := lambda(ops...)
		lmd.Args = []*ir.Value{p}
		if sv, ok := step.(*ir.Value); ok {
			lmd.Args = append(lmd.Args, sv)
		}
		return lmd
	}
	for name, lmd := range map[string]*ir.Lambda{
		"used in body": mk(lit(1, u8()), 100, true),
		"wide step":    mk(local("s", u16()), 100, false),
		"overflow":     mk(lit(2, u8()), 65535, false),
	} {
		propagateSSA(lmd)
		if eliminateInduction(lmd) {
			t.Errorf("%s: transformed", name)
		}
	}
}
