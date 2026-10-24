package driver

// fclib/float.fc (IEEE 754 binary32 / binary16 の演算) を Go の float32 と比べる (TestFloatLib)。Agent/wiki/design/float-fixed.md。
// ビットの並びを乱数で作り (0・非正規化数・最大・無限大・NaN・丸めの境目を多めに)、関数の結果のビットを Go の結果と比べる。
// NaN を作る演算の結果は 0x7fc00000 (f16 は 0x7e00) に揃える約束。

import (
	"flag"
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strings"
	"testing"
)

var floatLibN = flag.Int("floatn", 40, "TestFloatLib のプログラム数 (1 本 60 式。6 本では asm の掛け算の繰り上がりの誤りを見逃した)")

const goNaN32 = 0x7fc00000

// canon は NaN を 0x7fc00000 に揃えたビット。
func canon(f float32) uint32 {
	if f != f {
		return goNaN32
	}
	return math.Float32bits(f)
}

// randF32 は端の値を多めにした f32 のビット。
func randF32(r *rand.Rand) uint32 {
	sign := uint32(r.Intn(2)) << 31
	switch r.Intn(13) {
	case 0:
		return sign // ±0
	case 1:
		return sign | uint32(1+r.Intn(0x7fffff)) // 非正規化数
	case 2:
		return sign | 0x7f800000 // ±inf
	case 3:
		return sign | 0x7f800000 | uint32(1+r.Intn(0x7fffff)) // NaN
	case 4:
		return sign | 0x7f7fffff - uint32(r.Intn(4)) // 最大の近く
	case 5:
		return sign | uint32(0x00800000+r.Intn(4)) // 最小の正規化数の近く
	case 6:
		return sign | 0x3f800000 | uint32(r.Intn(8)) // 1 の近く
	case 7:
		return sign | uint32(r.Intn(255))<<23 | 0x7fffff // 仮数が全部 1
	case 8:
		return math.Float32bits(float32(r.Intn(2000) - 1000)) // 整数
	case 9:
		return math.Float32bits(float32(r.Intn(64)) / 8) // 1/8 刻み (丸めの中間)
	case 10:
		return math.Float32bits(float32(r.Float64()*2000 - 1000)) // 中くらいの大きさで小数の部分のある値
	}
	return r.Uint32()
}

// f16 の全部の値 (NaN を除く) を小さい順に (to16 の正解を探すため)。
var f16Sorted = func() []uint16 {
	var r []uint16
	for h := 0; h < 0x10000; h++ {
		if h&0x7c00 == 0x7c00 && h&0x3ff != 0 {
			continue
		}
		if h == 0x8000 {
			continue // -0 は +0 と同じ値 (符号は別に見る)
		}
		r = append(r, uint16(h))
	}
	sort.Slice(r, func(i, j int) bool { return f16Value(r[i]) < f16Value(r[j]) })
	return r
}()

// f16Value は f16 のビットの値 (正確)。
func f16Value(h uint16) float64 {
	s := 1.0
	if h&0x8000 != 0 {
		s = -1
	}
	e := int(h>>10) & 31
	m := float64(h & 0x3ff)
	switch e {
	case 31:
		if m != 0 {
			return math.NaN()
		}
		return math.Inf(int(s))
	case 0:
		return s * math.Ldexp(m, -24)
	}
	return s * math.Ldexp(1024+m, e-25)
}

// goTo16 は f32 → f16 の正解 (最近接偶数。2^15 × (2 - 2^-11) 以上は無限大)。
func goTo16(f float32) uint16 {
	if f != f {
		return 0x7e00
	}
	v := float64(f)
	sign := uint16(0)
	if math.Signbit(v) {
		sign = 0x8000
	}
	if v == 0 {
		return sign
	}
	a := math.Abs(v)
	if a >= 65520 { // 65504 と 65536 の中間以上
		return sign | 0x7c00
	}
	// 正の値の中で最も近いもの
	pos := f16Sorted[sort.Search(len(f16Sorted), func(i int) bool { return f16Value(f16Sorted[i]) >= 0 }):]
	i := sort.Search(len(pos), func(i int) bool { return f16Value(pos[i]) >= a })
	if i < len(pos) && f16Value(pos[i]) == a {
		return sign | pos[i]
	}
	lo, hi := pos[i-1], pos[i]
	dl, dh := a-f16Value(lo), f16Value(hi)-a
	switch {
	case dl < dh:
		return sign | lo
	case dh < dl:
		return sign | hi
	case lo&1 == 0:
		return sign | lo
	}
	return sign | hi
}

// goFrom16 は f16 → f32 の正解。
func goFrom16(h uint16) uint32 {
	v := f16Value(h)
	if v != v {
		return goNaN32
	}
	if h == 0x8000 {
		return 0x80000000
	}
	return math.Float32bits(float32(v))
}

func satI32(f float32) int32 {
	switch {
	case f != f:
		return 0
	case f >= 2147483648:
		return math.MaxInt32
	case f < -2147483648:
		return math.MinInt32
	}
	return int32(f)
}

func satU32(f float32) uint32 {
	switch {
	case f != f, f <= -1:
		return 0
	case f >= 4294967296:
		return math.MaxUint32
	case f < 0:
		return 0
	}
	return uint32(f)
}

// floatCase は 1 つの式 (ソースと正解の出力)。
type floatCase struct{ src, want string }

func floatLibCase(r *rand.Rand) floatCase {
	a, b := randF32(r), randF32(r)
	fa, fb := math.Float32frombits(a), math.Float32frombits(b)
	hex := func(v uint32) string { return fmt.Sprintf("%x", v) }
	boolS := func(v bool) string { return fmt.Sprint(v) }
	call := func(f string, args ...any) string {
		s := make([]string, len(args))
		for i, a := range args {
			s[i] = fmt.Sprint(a)
		}
		return fmt.Sprintf("float.%s(%s)", f, strings.Join(s, ", "))
	}
	r64 := func(f func(float64) float64) string {
		if fa != fa {
			return hex(goNaN32)
		}
		return hex(math.Float32bits(float32(f(float64(fa)))))
	}
	switch r.Intn(18) {
	case 0:
		return floatCase{call("add32", a, b), hex(canon(fa + fb))}
	case 1:
		return floatCase{call("sub32", a, b), hex(canon(fa - fb))}
	case 2:
		return floatCase{call("mul32", a, b), hex(canon(fa * fb))}
	case 3:
		return floatCase{call("div32", a, b), hex(canon(fa / fb))}
	case 4:
		return floatCase{call("sqrt32", a), hex(canon(float32(math.Sqrt(float64(fa)))))}
	case 5:
		return floatCase{call("eq32", a, b), boolS(fa == fb)}
	case 6:
		return floatCase{call("lt32", a, b), boolS(fa < fb)}
	case 7:
		return floatCase{call("le32", a, b), boolS(fa <= fb)}
	case 8:
		n := int32(r.Uint32()) >> uint(r.Intn(32))
		return floatCase{call("from_i32", fmt.Sprintf("(%d as i32)", n)), hex(math.Float32bits(float32(n)))}
	case 9:
		n := r.Uint32() >> uint(r.Intn(32))
		return floatCase{call("from_u32", fmt.Sprintf("(%d as u32)", n)), hex(math.Float32bits(float32(n)))}
	case 10:
		return floatCase{call("to_i32", a), fmt.Sprint(satI32(fa))}
	case 11:
		return floatCase{call("to_u32", a), fmt.Sprint(satU32(fa))}
	case 12:
		return floatCase{call("rint32", a), r64(math.RoundToEven)}
	case 13:
		f := []string{"floor32", "ceil32", "trunc32", "round32"}[r.Intn(4)]
		g := map[string]func(float64) float64{"floor32": math.Floor, "ceil32": math.Ceil, "trunc32": math.Trunc, "round32": math.Round}[f]
		return floatCase{call(f, a), r64(g)}
	case 14:
		n := r.Intn(80) - 40
		want := uint32(goNaN32)
		if fa == fa {
			want = math.Float32bits(float32(math.Ldexp(float64(fa), n)))
		}
		return floatCase{call("ldexp32", a, fmt.Sprintf("(%d as i16)", n)), hex(want)}
	case 15, 16:
		return floatCase{call("to16", a), fmt.Sprintf("%x", goTo16(fa))}
	}
	h := uint16(r.Uint32())
	return floatCase{call("from16", fmt.Sprintf("(%d as u16)", h)), hex(goFrom16(h))}
}

// TestFloatLib は fclib/float.fc の関数の結果のビットを Go と比べる (-O 2 の emu と、インタプリタ)。
func TestFloatLib(t *testing.T) {
	t.Parallel()
	base := *randSeed
	if base == 0 {
		base = 1
	}
	for k := 0; k < *floatLibN; k++ {
		seed := base + int64(k)
		t.Run(fmt.Sprintf("seed%d", seed), func(t *testing.T) {
			t.Parallel()
			r := rand.New(rand.NewSource(seed))
			var src, want strings.Builder
			src.WriteString("#fc 4\nuse console;\nuse float;\nfunction main():void\n{\n")
			var cases []floatCase
			for i := 0; i < 60; i++ {
				c := floatLibCase(r)
				cases = append(cases, c)
				verb := "{:x}"
				if strings.HasPrefix(c.want, "true") || strings.HasPrefix(c.want, "false") || strings.Contains(c.src, "to_i32") || strings.Contains(c.src, "to_u32") {
					verb = "{}"
				}
				fmt.Fprintf(&src, "\t@printf(\"%s\\n\", %s);\n", verb, c.src)
				want.WriteString(c.want + "\n")
			}
			src.WriteString("\tconsole.exit(0);\n}\n")
			files := map[string]string{"t.fc": src.String()}
			out, err := rpRun(t, files, 0, rpMaxCycles)
			if err != nil {
				t.Fatalf("seed %d: %v\n%s", seed, err, src.String())
			}
			got, exp := strings.Split(out, "\n"), strings.Split(want.String(), "\n")
			for i, c := range cases {
				if i >= len(got) || got[i] != exp[i] {
					g := ""
					if i < len(got) {
						g = got[i]
					}
					t.Errorf("seed %d: %s = %s, want %s", seed, c.src, g, exp[i])
				}
			}
			if *randInterp {
				if iout, ok, err := rpInterp(t, files); ok && (err != nil || iout != want.String()) {
					t.Errorf("interp (seed %d): err %v\n%s", seed, err, iout)
				}
			}
		})
	}
}
