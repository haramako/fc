package opt

import (
	"testing"

	"github.com/haramako/fc/internal/ir"
)

func TestExpandMul(t *testing.T) {
	x, d1, d2, d3, d4, d5 := local("x", u8()), local("d1", u8()), local("d2", u8()), local("d3", u8()), local("d4", u8()), local("d5", u8())
	lmd := lambda(
		op(ir.OpMul, d1, x, lit(10, u8())),
		op(ir.OpMul, d2, lit(7, u8()), x),
		op(ir.OpMul, d3, x, lit(8, u8())), // 2 のべき乗はそのまま
		op(ir.OpMul, d4, x, lit(1, u8())),
		op(ir.OpMul, d5, x, lit(11, u8())),
		op(ir.OpMul, x, x, lit(3, u8())),
	)
	lmd.Args = []*ir.Value{x}
	expandMul(lmd)
	check(t, lmd,
		"shift_left $mul = x, #3",
		"shift_left $mul = x, #1",
		"add d1 = $mul, $mul",
		"shift_left $mul = x, #3",
		"sub d2 = $mul, x",
		"mul d3 = x, #8",
		"load d4 = x",
		"shift_left $mul = x, #3",
		"shift_left $mul = x, #1",
		"add $mul = $mul, $mul",
		"add d5 = $mul, x",
		"shift_left $mul = x, #2",
		"sub x = $mul, x",
	)
}
