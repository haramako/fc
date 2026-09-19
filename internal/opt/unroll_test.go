package opt

import (
	"testing"

	"github.com/haramako/fc/internal/ir"
)

// 3 回のループを展開する。ヘッダの検査とカウンタは SSA が畳んで消える。写しごとの一時変数は別の変数
func TestUnrollSimple(t *testing.T) {
	j, x, tv := local("j", u8()), local("x", u8()), tmp("t", u8())
	lmd := lambda(
		op(ir.OpLoad, j, lit(3, u8())),
		label("@begin"),
		ifz(j, "@end"),
		op(ir.OpAdd, tv, x, j),
		op(ir.OpShiftLeft, x, tv, lit(1, u8())),
		op(ir.OpSub, j, j, lit(1, u8())),
		jump("@begin"),
		label("@end"),
		ret(x),
	)
	lmd.Args = []*ir.Value{x}
	if !unrollLoops(lmd) {
		t.Fatal("not unrolled")
	}
	propagateSSA(lmd)
	check(t, lmd,
		"label @begin",
		"add t = x, #3",
		"shift_left x = t, #1",
		"jump @begin_1", // 直後へのジャンプは simplifyJumps が消す
		"label @begin_1",
		"add t_u1 = x, #2",
		"shift_left x = t_u1, #1",
		"jump @begin_2",
		"label @begin_2",
		"add t_u2 = x, #1",
		"shift_left x = t_u2, #1",
		"jump @begin_3",
		"label @begin_3",
		"jump @end",
		"label @end",
		"return x",
	)
}

// 対象外: 回数が多い、初期値が変数
func TestUnrollNotApplicable(t *testing.T) {
	mk := func(init ir.Operand) *ir.Lambda {
		j, x := local("j", u8()), local("x", u8())
		lmd := lambda(
			op(ir.OpLoad, j, init),
			label("@begin"),
			ifz(j, "@end"),
			op(ir.OpAdd, x, x, j),
			op(ir.OpSub, j, j, lit(1, u8())),
			jump("@begin"),
			label("@end"),
			ret(x),
		)
		lmd.Args = []*ir.Value{x}
		if v, ok := init.(*ir.Value); ok {
			lmd.Args = append(lmd.Args, v)
		}
		return lmd
	}
	if unrollLoops(mk(lit(20, u8()))) {
		t.Error("too many trips: unrolled")
	}
	if unrollLoops(mk(local("n", u8()))) {
		t.Error("variable start: unrolled")
	}
}
