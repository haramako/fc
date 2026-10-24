package driver

// 浮動小数 (f32 / f16) の Go 模型の fuzz (TestRandomFloatV4)。Agent/wiki/design/float-fixed.md。
//
// 1 つの式を変数の形 (葉を noinline の `id_T(定数)` で包んで実行時に計算) と定数の形 (葉を `(定数 as T)` にして sema に畳ませる) で
// 書き、結果のビット (`@bitcast`) と書式 (`{}` / `{:.N}`) を @printf で出して、Go の float32 で計算した模型と比べる。
//
// 模型の規則 (docs/reference/language.md の「浮動小数」): IEEE 754 の最近接偶数丸め。NaN を作る演算の結果は 0x7fc00000
// (f16 は 0x7e00)。f16 の演算は f32 に広げて計算して f16 に丸め直す。整数への `as` は 0 へ切り捨てて範囲に飽和 (NaN は 0)、
// 整数からは最近接。比較は NaN と偽。

import (
	"fmt"
	"math"
	"math/rand"
	"strings"
	"testing"

	"github.com/haramako/fc/internal/ir"
)

// flNode は生成した式 (変数の形 v・定数の形 c・模型のビット)。
type flNode struct {
	v, c string
	bits uint32
	f16  bool
}

func (n flNode) val() float32 {
	if n.f16 {
		return float32(f16Value(uint16(n.bits)))
	}
	return math.Float32frombits(n.bits)
}

func flBits(f float32, f16 bool) uint32 {
	if f16 {
		return uint32(goTo16(f))
	}
	return canon(f)
}

func flTypeName(f16 bool) string {
	if f16 {
		return "f16"
	}
	return "f32"
}

type flGen struct{ r *rand.Rand }

func (g *flGen) pick(n int) int { return g.r.Intn(n) }

// lit は f32 / f16 の値の正確な 10 進の綴り (NaN・無限大は式で作る)。
func flLit(bits uint32, f16 bool) (string, bool) {
	var r, neg, nan, inf = ir.FloatValue(uint64(bits), map[bool]int{true: 2, false: 4}[f16])
	if nan || inf {
		return "", false
	}
	s := r.FloatString(160)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	if neg && r.Sign() != 0 {
		s = "(-" + s + ")"
	} else if neg {
		return "", false // -0 は定数で書けない (型のない定数に -0 は無い)
	}
	return s, true
}

// leaf は値の葉。
func (g *flGen) leaf(f16 bool) flNode {
	for {
		var bits uint32
		if f16 {
			bits = uint32(goTo16(math.Float32frombits(randF32(g.r))))
			if g.pick(3) == 0 {
				bits = uint32(g.r.Intn(0x10000))
			}
		} else {
			bits = randF32(g.r)
		}
		s, ok := flLit(bits, f16)
		if !ok {
			continue
		}
		t := flTypeName(f16)
		return flNode{v: fmt.Sprintf("id_%s(%s)", t, s), c: fmt.Sprintf("(%s as %s)", s, t), bits: bits, f16: f16}
	}
}

func (g *flGen) expr(f16 bool, depth int) flNode {
	if depth <= 0 || g.pick(4) == 0 {
		return g.leaf(f16)
	}
	t := flTypeName(f16)
	switch k := g.pick(9); {
	case k < 4:
		a, b := g.expr(f16, depth-1), g.expr(f16, depth-1)
		op := []string{"+", "-", "*", "/"}[g.pick(4)]
		x, y := a.val(), b.val()
		var r float32
		switch op {
		case "+":
			r = x + y
		case "-":
			r = x - y
		case "*":
			r = x * y
		case "/":
			r = x / y
		}
		return flNode{v: fmt.Sprintf("(%s %s %s)", a.v, op, b.v), c: fmt.Sprintf("(%s %s %s)", a.c, op, b.c), bits: flBits(r, f16), f16: f16}
	case k == 4: // 単項の - (符号のビットの反転。NaN も)
		a := g.expr(f16, depth-1)
		sb := uint32(0x80000000)
		if f16 {
			sb = 0x8000
		}
		return flNode{v: "(-" + a.v + ")", c: "(-" + a.c + ")", bits: a.bits ^ sb, f16: f16}
	case k == 5: // 型のない定数と (2 進で正確な値と大きすぎる値だけ: 10 進から f32 / f16 への丸めを 2 段にしない)
		a := g.expr(f16, depth-1)
		lits := []string{"0.5", "1", "3", "0.25", "1024", "0.0009765625"}
		if !f16 {
			lits = append(lits, "1e10")
		}
		l := lits[g.pick(len(lits))]
		lf := map[string]float64{"0.5": 0.5, "1": 1, "3": 3, "0.25": 0.25, "1024": 1024, "1e10": 1e10, "0.0009765625": 0.0009765625}[l]
		lv := float32(lf)
		if f16 {
			lv = float32(f16Value(goTo16(lv)))
		}
		if g.pick(2) == 0 {
			return flNode{v: fmt.Sprintf("(%s + %s)", a.v, l), c: fmt.Sprintf("(%s + %s)", a.c, l), bits: flBits(a.val()+lv, f16), f16: f16}
		}
		return flNode{v: fmt.Sprintf("(%s * %s)", a.v, l), c: fmt.Sprintf("(%s * %s)", a.c, l), bits: flBits(a.val()*lv, f16), f16: f16}
	case k == 6: // f16 ↔ f32
		a := g.expr(!f16, depth-1)
		return flNode{v: fmt.Sprintf("(%s as %s)", a.v, t), c: fmt.Sprintf("(%s as %s)", a.c, t), bits: flBits(a.val(), f16), f16: f16}
	case k == 7: // 数学の組み込み (f32 で計算して丸め直す)
		a := g.expr(f16, depth-1)
		fns := []string{"@sqrt", "@floor", "@ceil", "@trunc", "@round"}
		fn := fns[g.pick(len(fns))]
		gf := map[string]func(float64) float64{"@sqrt": math.Sqrt, "@floor": math.Floor, "@ceil": math.Ceil, "@trunc": math.Trunc, "@round": math.Round}[fn]
		r := float32(gf(float64(a.val())))
		return flNode{v: fmt.Sprintf("%s(%s)", fn, a.v), c: fmt.Sprintf("%s(%s)", fn, a.c), bits: flBits(r, f16), f16: f16}
	}
	if g.pick(2) == 0 { // 固定小数 (i8f8) から: 中身 × 2^-8 (丸めは 1 回)
		raw := int16(g.r.Uint32())
		lit := ir.FormatFixed(int(raw), 8, ir.LogSpec{})
		if raw < 0 {
			lit = "(" + lit + ")"
		}
		return flNode{v: fmt.Sprintf("(id_i8f8(%s) as %s)", lit, t), c: fmt.Sprintf("((%s as i8f8) as %s)", lit, t), bits: flBits(float32(float64(raw)/256), f16), f16: f16}
	}
	// 整数から
	n := int32(g.r.Uint32()) >> uint(g.pick(32))
	return flNode{v: fmt.Sprintf("(id_i32(%d) as %s)", n, t), c: fmt.Sprintf("((%d as i32) as %s)", n, t), bits: flBits(float32(n), f16), f16: f16}
}

// program は 2 つの形のプログラムと模型の出力。
func (g *flGen) program(n int) (vsrc, csrc, want string) {
	var head strings.Builder
	head.WriteString("#fc 4\nuse console;\n")
	for _, t := range []string{"f32", "f16", "i32", "i8f8"} {
		fmt.Fprintf(&head, "function id_%s(x:%s):%s @(noinline) { return x; }\n", t, t, t)
	}
	var vb, cb, wb strings.Builder
	for i := 0; i < n; i++ {
		f16 := g.pick(3) == 0
		e := g.expr(f16, 1+g.pick(3))
		t := flTypeName(f16)
		bt := map[bool]string{true: "u16", false: "u32"}[f16]
		size := map[bool]int{true: 2, false: 4}[f16]
		switch g.pick(5) {
		case 4: // 固定小数へ (最近接偶数、範囲の外は飽和、NaN は 0)
			want := 0
			if f := e.val(); f == f {
				x := math.RoundToEven(float64(float32(math.Ldexp(float64(f), 16))))
				switch {
				case x <= math.MinInt32:
					want = math.MinInt32
				case x >= math.MaxInt32:
					want = math.MaxInt32
				default:
					want = int(x)
				}
			}
			fmt.Fprintf(&vb, "\t@printf(\"{}\\n\", @bitcast(i32, (%s) as i16f16));\n", e.v)
			fmt.Fprintf(&cb, "\t@printf(\"{}\\n\", @bitcast(i32, (%s) as i16f16));\n", e.c)
			fmt.Fprintf(&wb, "%d\n", want)
		case 0: // 書式
			spec, sp := "{}", ir.LogSpec{}
			if g.pick(2) == 0 {
				p := g.pick(6)
				spec, sp = fmt.Sprintf("{:.%d}", p), ir.LogSpec{Prec: p + 1}
			}
			fmt.Fprintf(&vb, "\t@printf(\"%s\\n\", %s);\n", spec, e.v)
			fmt.Fprintf(&cb, "\t@printf(\"%s\\n\", %s);\n", spec, e.c)
			fmt.Fprintf(&wb, "%s\n", ir.FormatFloat(uint64(e.bits), size, sp))
		case 1: // 比較
			b := g.expr(f16, 1)
			op := []string{"==", "!=", "<", "<=", ">", ">="}[g.pick(6)]
			x, y := e.val(), b.val()
			r := map[string]bool{"==": x == y, "!=": x != y, "<": x < y, "<=": x <= y, ">": x > y, ">=": x >= y}[op]
			fmt.Fprintf(&vb, "\t@printf(\"{}\\n\", %s %s %s);\n", e.v, op, b.v)
			fmt.Fprintf(&cb, "\t@printf(\"{}\\n\", %s %s %s);\n", e.c, op, b.c)
			fmt.Fprintf(&wb, "%v\n", r)
		case 2: // 整数へ (0 へ切り捨て、飽和)
			fmt.Fprintf(&vb, "\t@printf(\"{}\\n\", (%s) as i32);\n", e.v)
			fmt.Fprintf(&cb, "\t@printf(\"{}\\n\", (%s) as i32);\n", e.c)
			fmt.Fprintf(&wb, "%d\n", satI32(e.val()))
		default: // ビット
			fmt.Fprintf(&vb, "\t@printf(\"{:x}\\n\", @bitcast(%s, %s));\n", bt, e.v)
			fmt.Fprintf(&cb, "\t@printf(\"{:x}\\n\", @bitcast(%s, %s));\n", bt, e.c)
			fmt.Fprintf(&wb, "%x\n", e.bits)
		}
		_ = t
	}
	tail := "\tconsole.exit(0);\n}\n"
	return head.String() + "function main():void\n{\n" + vb.String() + tail, head.String() + "function main():void\n{\n" + cb.String() + tail, wb.String()
}

func TestRandomFloatV4(t *testing.T) {
	t.Parallel()
	n := max(*randN/4, 5)
	base := *randSeed
	if base == 0 {
		base = 1
	}
	for k := 0; k < n; k++ {
		seed := base + int64(k)
		t.Run(fmt.Sprintf("seed%d", seed), func(t *testing.T) {
			t.Parallel()
			g := &flGen{r: rand.New(rand.NewSource(seed))}
			vsrc, csrc, want := g.program(10)
			for _, level := range []int{-1, 0} {
				out, err := rpRun(t, map[string]string{"t.fc": vsrc}, level, rpMaxCycles)
				if err != nil || out != want {
					t.Fatalf("変数の形 level %d (seed %d): got\n%s\nerr %v\nwant\n%s\n%s", level, seed, out, err, want, vsrc)
				}
			}
			out, err := rpRun(t, map[string]string{"t.fc": csrc}, 0, rpMaxCycles)
			if err != nil || out != want {
				t.Fatalf("定数の形 (seed %d): got\n%s\nerr %v\nwant\n%s\n%s", seed, out, err, want, csrc)
			}
		})
	}
}
