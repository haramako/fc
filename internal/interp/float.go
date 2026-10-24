package interp

// fclib/float.asm の f32 の関数を Go の float32 で実行する natives (asm はインタプリタで走らないので。結果のビットは float.asm と
// 同じ: NaN を作る結果は 0x7fc00000、整数へは 0 へ切り捨てて飽和・NaN は 0。TestFloatLib が両方を Go と比べる)。

import (
	"math"
)

const canonNaN = 0x7fc00000

func le32(args [][]byte, i int) uint32 {
	var v uint32
	if i < len(args) {
		for k := 0; k < 4 && k < len(args[i]); k++ {
			v |= uint32(args[i][k]) << (8 * k)
		}
	}
	return v
}

func f32arg(args [][]byte, i int) float32 { return math.Float32frombits(le32(args, i)) }

func put32(v uint32) []byte { return []byte{byte(v), byte(v >> 8), byte(v >> 16), byte(v >> 24)} }

// fbits は f の f32 のビット (NaN は 0x7fc00000)。
func fbits(f float32) []byte {
	if f != f {
		return put32(canonNaN)
	}
	return put32(math.Float32bits(f))
}

func fbool(b bool) []byte {
	if b {
		return []byte{1}
	}
	return []byte{0}
}

// f64 で計算した値 (正確な値) を f32 に 1 回で丸める関数の natives。
func f32unary(g func(float64) float64) func(m *machine, args [][]byte) []byte {
	return func(m *machine, args [][]byte) []byte {
		x := f32arg(args, 0)
		if x != x {
			return put32(canonNaN)
		}
		return fbits(float32(g(float64(x))))
	}
}

func init() {
	nat := map[string]func(m *machine, args [][]byte) []byte{
		"_float_add32": func(m *machine, a [][]byte) []byte { return fbits(f32arg(a, 0) + f32arg(a, 1)) },
		"_float_sub32": func(m *machine, a [][]byte) []byte { return fbits(f32arg(a, 0) - f32arg(a, 1)) },
		"_float_mul32": func(m *machine, a [][]byte) []byte { return fbits(f32arg(a, 0) * f32arg(a, 1)) },
		"_float_div32": func(m *machine, a [][]byte) []byte { return fbits(f32arg(a, 0) / f32arg(a, 1)) },
		"_float_eq32":  func(m *machine, a [][]byte) []byte { return fbool(f32arg(a, 0) == f32arg(a, 1)) },
		"_float_lt32":  func(m *machine, a [][]byte) []byte { return fbool(f32arg(a, 0) < f32arg(a, 1)) },
		"_float_le32":  func(m *machine, a [][]byte) []byte { return fbool(f32arg(a, 0) <= f32arg(a, 1)) },
		"_float_from_i32": func(m *machine, a [][]byte) []byte {
			return fbits(float32(int32(le32(a, 0))))
		},
		"_float_from_u32": func(m *machine, a [][]byte) []byte { return fbits(float32(le32(a, 0))) },
		"_float_to_i32": func(m *machine, a [][]byte) []byte {
			x := float64(f32arg(a, 0))
			switch {
			case x != x:
				return put32(0)
			case x >= math.MaxInt32:
				return put32(math.MaxInt32)
			case x <= math.MinInt32:
				return put32(1 << 31)
			}
			return put32(uint32(int32(x)))
		},
		"_float_to_u32": func(m *machine, a [][]byte) []byte {
			x := float64(f32arg(a, 0))
			switch {
			case x != x || x <= 0:
				return put32(0)
			case x >= math.MaxUint32:
				return put32(math.MaxUint32)
			}
			return put32(uint32(x))
		},
		// √ は f64 で計算して f32 に丸めても正しく丸めた値になる (f64 は f32 の 2 倍より多い桁がある)
		"_float_sqrt32":  f32unary(math.Sqrt),
		"_float_rint32":  f32unary(math.RoundToEven),
		"_float_floor32": f32unary(math.Floor),
		"_float_ceil32":  f32unary(math.Ceil),
		"_float_trunc32": f32unary(math.Trunc),
		"_float_round32": f32unary(math.Round),
		"_float_ldexp32": func(m *machine, a [][]byte) []byte {
			x := f32arg(a, 0)
			n := int(int16(uint16(le32(a, 1))))
			if x != x {
				return put32(canonNaN)
			}
			return fbits(float32(math.Ldexp(float64(x), n))) // f64 では正確 (範囲の外は f32 でも 0 か無限大)
		},
	}
	// to_sat_i / to_sat_u(x, n, rnd, bits): x × 2^n を整数に (rnd なら最近接偶数、でなければ 0 へ)、bits ビットに飽和、NaN は 0
	toSat := func(signed bool) func(m *machine, a [][]byte) []byte {
		return func(m *machine, a [][]byte) []byte {
			x := f32arg(a, 0)
			n := int(int16(uint16(le32(a, 1))))
			rnd := len(a) > 2 && len(a[2]) > 0 && a[2][0] != 0
			bits := 32
			if len(a) > 3 && len(a[3]) > 0 {
				bits = int(a[3][0])
			}
			if x != x {
				return put32(0)
			}
			v := math.Ldexp(float64(x), n) // f64 で正確 (|v| < 2^-1022 になるのは丸めると 0 の値だけ)
			if rnd {
				v = math.RoundToEven(v)
			} else {
				v = math.Trunc(v)
			}
			lo, hi := 0.0, math.Ldexp(1, bits)-1
			if signed {
				lo, hi = -math.Ldexp(1, bits-1), math.Ldexp(1, bits-1)-1
			}
			v = math.Max(lo, math.Min(hi, v))
			if signed {
				return put32(uint32(int64(v)))
			}
			return put32(uint32(uint64(v)))
		}
	}
	nat["_float_from16"] = func(m *machine, a [][]byte) []byte { return fbits(f16Value(uint16(le32(a, 0)))) }
	nat["_float_to16"] = func(m *machine, a [][]byte) []byte {
		h := to16(f32arg(a, 0))
		return []byte{byte(h), byte(h >> 8)}
	}
	nat["_float_to_sat_i"] = toSat(true)
	nat["_float_to_sat_u"] = toSat(false)
	for k, v := range nat {
		natives[k] = v
	}
}

// f16Value は f16 のビットの値 (正確に f32 に入る)。
func f16Value(h uint16) float32 {
	sign := float32(1)
	if h&0x8000 != 0 {
		sign = -1
	}
	e, m := int(h>>10)&31, float64(h&0x3ff)
	switch {
	case e == 31 && m != 0:
		return float32(math.NaN())
	case e == 31:
		return sign * float32(math.Inf(1))
	case e == 0:
		return sign * float32(math.Ldexp(m, -24))
	}
	return sign * float32(math.Ldexp(1024+m, e-25))
}

// to16 は f32 の x を f16 に最近接偶数で丸めたビット (NaN は 0x7e00)。
func to16(x float32) uint16 {
	if x != x {
		return 0x7e00
	}
	var sign uint16
	a := float64(x)
	if math.Signbit(a) {
		sign, a = 0x8000, -a
	}
	if a >= 65520 { // 65504 と次の 65536 (無限大) のちょうど中間から上
		return sign | 0x7c00
	}
	if a == 0 {
		return sign
	}
	_, e := math.Frexp(a) // a = f × 2^e、0.5 <= f < 1
	e--                   // a = 1.x × 2^e
	if e < -14 {
		e = -14 // 非正規化数の刻み
	}
	q := math.RoundToEven(math.Ldexp(a, 10-e)) // 刻み 2^(e-10) の数 (f64 で正確)
	if e == -14 && q < 1024 {
		return sign | uint16(q)
	}
	if q == 2048 {
		e, q = e+1, 1024
	}
	if e > 15 {
		return sign | 0x7c00
	}
	return sign | uint16(e+15)<<10 | uint16(q-1024)
}
