package ir

import (
	"math"
	"testing"
)

func TestFormatFloat(t *testing.T) {
	f := func(x float32) uint64 { return uint64(math.Float32bits(x)) }
	cases := []struct {
		bits uint64
		size int
		sp   LogSpec
		want string
	}{
		{f(1.5), 4, LogSpec{}, "1.5"},
		{f(0.1), 4, LogSpec{}, "0.1"},
		{f(100), 4, LogSpec{}, "100"},
		{f(-0.0), 4, LogSpec{}, "0"},
		{0x80000000, 4, LogSpec{}, "-0"},
		{f(1e10), 4, LogSpec{}, "1e+10"},
		{f(1.5e10), 4, LogSpec{}, "1.5e+10"},
		{f(math.MaxFloat32), 4, LogSpec{}, "3.402823e+38"},
		{f(1e-5), 4, LogSpec{}, "1e-05"},
		{f(0.0001), 4, LogSpec{}, "0.0001"},
		{f(1e9), 4, LogSpec{}, "1e+09"},
		{f(999999936), 4, LogSpec{}, "999999936"},
		{f(1.005), 4, LogSpec{Prec: 3}, "1.00"},
		{f(2), 4, LogSpec{Prec: 4}, "2.000"},
		{f(2.5), 4, LogSpec{Prec: 1}, "3"},
		{f(1e20), 4, LogSpec{Prec: 3}, "1.00e+20"},
		{0x7fc00000, 4, LogSpec{Width: 5, Zero: true}, "  nan"},
		{0xff800000, 4, LogSpec{}, "-inf"},
		{f(-1.25), 4, LogSpec{Width: 7, Zero: true}, "-001.25"},
		{f(1.4e-45), 4, LogSpec{}, "1.401298e-45"},
		{0x3c00, 2, LogSpec{}, "1"},            // f16 の 1.0
		{0x7bff, 2, LogSpec{}, "65504"},        // f16 の最大
		{0x0001, 2, LogSpec{}, "5.960464e-08"}, // f16 の最小の非正規化数
	}
	for _, c := range cases {
		if got := FormatFloat(c.bits, c.size, c.sp); got != c.want {
			t.Errorf("FormatFloat(%#x, %d, %+v) = %q, want %q", c.bits, c.size, c.sp, got, c.want)
		}
	}
}
