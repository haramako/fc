package driver

// 固定小数 (`u8f8` など) の Go 模型の fuzz (TestRandomFixedV4)。Agent/wiki/design/float-fixed.md。
//
// 1 つの式を 2 通りに書く: 変数の形は葉を noinline の関数 `id_T(定数)` で包んで実行時に計算し、定数の形は葉を `(定数 as T)` に
// して sema に畳ませる。両方を @printf で出し、模型 (中身の整数を Go で同じ規則で計算して ir.FormatFixed で書いたもの) と比べる。
//
// 模型の規則 (docs/reference/language.md の「固定小数」): 中身は値 × 2^F の整数で、型の幅で折り返す。
//   - + - は中身の足し算・引き算、* は (a × b) を F だけ算術右シフト、/ は (a を F だけ左にずらして) b で床除算 (負の無限大へ)
//   - 整数 n との * / は中身 × n・中身 ÷ n (床除算)。n は中身の整数の型の値
//   - 単項の - は中身の符号反転、<< k / >> k は中身のシフト (>> は算術)
//   - `x as T` (分け方の違う固定小数から) は小数のビットの差だけずらす (減らす向きは算術右シフト = 負の無限大へ)
//   - 型のない実数の定数は最近接に丸める (この生成器は正確に表せる値だけを書く)

import (
	"fmt"
	"math/big"
	"math/rand"
	"strings"
	"testing"

	"github.com/haramako/fc/internal/ir"
)

// fxType は生成に使う固定小数の型。
type fxType struct {
	name       string
	size, frac int
	signed     bool
	intU, intS string // 掛ける・割る整数の型 (中身と同じ大きさ以下)
	rawName    string // 中身の整数の型
	idx        int
}

var fxTypes = func() []fxType {
	specs := []struct {
		size, frac int
		signed     bool
	}{
		{1, 4, false}, {1, 4, true}, {1, 7, true}, {2, 8, false}, {2, 8, true}, {2, 4, true}, {2, 12, false}, {2, 16, false},
		{3, 8, true}, {3, 16, false}, {4, 16, true}, {4, 16, false}, {4, 24, true}, {4, 8, false},
	}
	var r []fxType
	for i, s := range specs {
		p := map[bool]string{true: "i", false: "u"}[s.signed]
		r = append(r, fxType{
			name: fmt.Sprintf("%s%df%d", p, s.size*8-s.frac, s.frac), size: s.size, frac: s.frac, signed: s.signed,
			intU: "u8", intS: "i8", rawName: fmt.Sprintf("%s%d", p, s.size*8), idx: i,
		})
	}
	return r
}()

// wrap は中身の整数 v を型の幅に折り返す。
func (t fxType) wrap(v *big.Int) int64 {
	bits := uint(8 * t.size)
	m := new(big.Int).Lsh(big.NewInt(1), bits)
	r := new(big.Int).Mod(v, m)
	x := r.Int64()
	if t.signed && x >= 1<<(bits-1) {
		x -= 1 << bits
	}
	return x
}

// lit は中身 raw の正確な 10 進の綴り (負なら括弧で包む)。
func (t fxType) lit(raw int64) string {
	s := ir.FormatFixed(int(raw), t.frac, ir.LogSpec{})
	if raw < 0 {
		return "(" + s + ")"
	}
	return s
}

// fxNode は生成した式 (変数の形 v・定数の形 c・模型の中身 raw)。
type fxNode struct {
	v, c string
	raw  int64
	t    fxType
}

type fxGen struct {
	r *rand.Rand
}

func (g *fxGen) pick(n int) int        { return g.r.Intn(n) }
func (g *fxGen) chance(p float64) bool { return g.r.Float64() < p }

// rawValue は型 t の中身の値 (端の値と小さな値を多めに)。
func (g *fxGen) rawValue(t fxType) int64 {
	bits := uint(8 * t.size)
	lo, hi := int64(0), int64(1)<<bits-1
	if t.signed {
		lo, hi = -(int64(1) << (bits - 1)), int64(1)<<(bits-1)-1
	}
	one := int64(1) << uint(t.frac)
	switch g.pick(7) {
	case 0:
		return lo
	case 1:
		return hi
	case 2:
		return t.wrap(big.NewInt(one * int64(g.pick(4)))) // 整数の値 (入らなければ折り返す)
	case 3:
		v := one/2 + int64(g.pick(3)) - 1 // 0.5 の近く
		if t.signed && g.chance(0.5) {
			v = -v
		}
		return t.wrap(big.NewInt(v))
	case 4:
		return int64(g.pick(8))
	}
	return lo + g.r.Int63n(hi-lo+1)
}

// leaf は型 t の値の葉。
func (g *fxGen) leaf(t fxType) fxNode {
	raw := g.rawValue(t)
	return fxNode{v: fmt.Sprintf("id_%s(%s)", t.name, t.lit(raw)), c: fmt.Sprintf("(%s as %s)", t.lit(raw), t.name), raw: raw, t: t}
}

// expr は型 t の深さ depth の式。
func (g *fxGen) expr(t fxType, depth int) fxNode {
	if depth <= 0 || g.chance(0.25) {
		return g.leaf(t)
	}
	one := big.NewInt(1)
	switch k := g.pick(10); {
	case k < 4: // + - * /
		a, b := g.expr(t, depth-1), g.expr(t, depth-1)
		op := []string{"+", "-", "*", "/"}[g.pick(4)]
		A, B := big.NewInt(a.raw), big.NewInt(b.raw)
		var r *big.Int
		switch op {
		case "+":
			r = new(big.Int).Add(A, B)
		case "-":
			r = new(big.Int).Sub(A, B)
		case "*":
			r = new(big.Int).Rsh(new(big.Int).Mul(A, B), uint(t.frac))
		case "/":
			if b.raw == 0 {
				return a
			}
			r = fxFloorDiv(new(big.Int).Lsh(A, uint(t.frac)), B)
		}
		return fxNode{v: fmt.Sprintf("(%s %s %s)", a.v, op, b.v), c: fmt.Sprintf("(%s %s %s)", a.c, op, b.c), raw: t.wrap(r), t: t}
	case k == 4: // 型のない定数と + - (正確に表せる値)
		a := g.expr(t, depth-1)
		lr := int64(g.pick(4)) << uint(max(0, t.frac-2)) // 0, 0.25, 0.5, 0.75 (小数のビットが足りなければ整数の刻み)
		if t.frac < 2 {
			lr = int64(g.pick(4)) << uint(t.frac)
		}
		op := []string{"+", "-"}[g.pick(2)]
		s := t.lit(lr)
		r := a.raw + lr
		if op == "-" {
			r = a.raw - lr
		}
		return fxNode{v: fmt.Sprintf("(%s %s %s)", a.v, op, s), c: fmt.Sprintf("(%s %s %s)", a.c, op, s), raw: t.wrap(big.NewInt(r)), t: t}
	case k == 5: // 整数との * / (中身に掛ける・割る)
		a := g.expr(t, depth-1)
		n := int64(1 + g.pick(9))
		name := t.intU
		if t.signed && g.chance(0.4) {
			n, name = -n, t.intS
		}
		op := []string{"*", "/"}[g.pick(2)]
		var r *big.Int
		if op == "*" {
			r = new(big.Int).Mul(big.NewInt(a.raw), big.NewInt(n))
		} else {
			r = fxFloorDiv(big.NewInt(a.raw), big.NewInt(n))
		}
		nv, nc := fmt.Sprint(n), fmt.Sprint(n)
		if g.chance(0.5) { // 整数の変数
			nv = fmt.Sprintf("id_%s(%d)", name, n)
			nc = fmt.Sprintf("(%d as %s)", n, name)
		} else if n < 0 {
			nv, nc = "("+nv+")", "("+nc+")"
		}
		return fxNode{v: fmt.Sprintf("(%s %s %s)", a.v, op, nv), c: fmt.Sprintf("(%s %s %s)", a.c, op, nc), raw: t.wrap(r), t: t}
	case k == 6: // 単項の -
		a := g.expr(t, depth-1)
		return fxNode{v: "(-" + a.v + ")", c: "(-" + a.c + ")", raw: t.wrap(new(big.Int).Neg(big.NewInt(a.raw))), t: t}
	case k == 7: // シフト
		a := g.expr(t, depth-1)
		s := g.pick(4)
		if g.chance(0.5) {
			return fxNode{v: fmt.Sprintf("(%s << %d)", a.v, s), c: fmt.Sprintf("(%s << %d)", a.c, s), raw: t.wrap(new(big.Int).Lsh(big.NewInt(a.raw), uint(s))), t: t}
		}
		return fxNode{v: fmt.Sprintf("(%s >> %d)", a.v, s), c: fmt.Sprintf("(%s >> %d)", a.c, s), raw: t.wrap(new(big.Int).Rsh(big.NewInt(a.raw), uint(s))), t: t}
	default: // 分け方の違う固定小数からの as
		u := fxTypes[g.pick(len(fxTypes))]
		a := g.expr(u, depth-1)
		x := new(big.Int).Lsh(big.NewInt(a.raw), uint(max(0, t.frac-u.frac)))
		x.Rsh(x, uint(max(0, u.frac-t.frac)))
		_ = one
		return fxNode{v: fmt.Sprintf("(%s as %s)", a.v, t.name), c: fmt.Sprintf("(%s as %s)", a.c, t.name), raw: t.wrap(x), t: t}
	}
}

// fxFloorDiv は床除算 (b は 0 でない)。
func fxFloorDiv(a, b *big.Int) *big.Int {
	q, m := new(big.Int).QuoRem(a, b, new(big.Int))
	if m.Sign() != 0 && (m.Sign() < 0) != (b.Sign() < 0) {
		q.Sub(q, big.NewInt(1))
	}
	return q
}

// program は式の並びの 2 つの形のプログラムと、模型の出力。
func (g *fxGen) program(n int) (vsrc, csrc, want string) {
	var head strings.Builder
	head.WriteString("#fc 4\nuse console;\n")
	for _, t := range fxTypes {
		fmt.Fprintf(&head, "function id_%s(x:%s):%s @(noinline) { return x; }\n", t.name, t.name, t.name)
	}
	for _, it := range []string{"u8", "i8"} {
		fmt.Fprintf(&head, "function id_%s(x:%s):%s @(noinline) { return x; }\n", it, it, it)
	}
	var vb, cb, wb strings.Builder
	for i := 0; i < n; i++ {
		t := fxTypes[g.pick(len(fxTypes))]
		e := g.expr(t, 1+g.pick(3))
		spec, sp := "{}", ir.LogSpec{}
		if g.chance(0.3) {
			p := g.pick(5)
			spec, sp = fmt.Sprintf("{:.%d}", p), ir.LogSpec{Prec: p + 1}
		}
		fmt.Fprintf(&vb, "\t@printf(\"%s\\n\", %s);\n", spec, e.v)
		fmt.Fprintf(&cb, "\t@printf(\"%s\\n\", %s);\n", spec, e.c)
		fmt.Fprintf(&wb, "%s\n", ir.FormatFixed(int(e.raw), t.frac, sp))
	}
	tail := "\tconsole.exit(0);\n}\n"
	return head.String() + "function main():void\n{\n" + vb.String() + tail, head.String() + "function main():void\n{\n" + cb.String() + tail, wb.String()
}

func TestRandomFixedV4(t *testing.T) {
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
			g := &fxGen{r: rand.New(rand.NewSource(seed))}
			vsrc, csrc, want := g.program(12)
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
			if *randInterp {
				if out, ok, err := rpInterp(t, map[string]string{"t.fc": vsrc}); ok && (err != nil || out != want) {
					t.Fatalf("interp (seed %d): got\n%s\nerr %v\nwant\n%s\n%s", seed, out, err, want, vsrc)
				}
			}
		})
	}
}
