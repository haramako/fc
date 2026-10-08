package driver

// fc 4 の書式のランダムテスト (TestRandomFormatV4)。@printf / @format / @try_format の書式 ({} / {0} / {:5} / {:05} / {:<5} /
// {:>5} / {:x} / {:X} / {:b} / {:c} / {:d}、{{ / }}。文字列・bool・文字の幅も) と、整数 (u8 / i8 / u16 / i16)・bool・enum・文字列 (slice・配列・*u8) の引数を混ぜ、期待値は
// 生成器が Go で計算する。同じ値を定数の引数 (`(200 as u8)`: コンパイル時に文字にする sema/format.go の formatConst) と実行時の
// 値 (`id_u8(200)`: fclib/fmt.fc) の両方で書くので、2 つの経路の食い違いも見える。@format は書き先の長さを足りるだけに、
// @try_format は足りないことがある長さにする (足りなければ長さ 0 の slice)。

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"testing"
)

// rfmType は書式の引数の型。
type rfmType struct {
	name   string
	size   int
	signed bool
	bool   bool
	enum   bool
	vals   []int // 境目の値
}

var rfmTypes = []rfmType{
	{name: "u8", size: 1, vals: []int{0, 1, 9, 10, 15, 16, 99, 100, 127, 128, 255}},
	{name: "i8", size: 1, signed: true, vals: []int{0, 1, -1, 9, -10, 127, -127, -128}},
	{name: "u16", size: 2, vals: []int{0, 255, 256, 999, 1000, 9999, 10000, 32768, 65535}},
	{name: "i16", size: 2, signed: true, vals: []int{0, -1, 255, -256, 9999, -10000, 32767, -32767, -32768}},
	{name: "bool", size: 1, bool: true, vals: []int{0, 1}},
	{name: "E", size: 1, enum: true, vals: []int{0, 7, 8, 200}},
	{name: "G", size: 2, signed: true, enum: true, vals: []int{-300, 1000}},
}

// rfmEnumName は enum の値のメンバー名。
func rfmEnumName(t rfmType, v int) string {
	switch t.name {
	case "E":
		return map[int]string{0: "A", 7: "B", 8: "C", 200: "D"}[v]
	case "G":
		return map[int]string{-300: "N", 1000: "M"}[v]
	}
	return ""
}

// rfmArg は書式の引数 1 つ (ソースと、整数なら値)。
type rfmArg struct {
	src   string
	t     *rfmType // nil なら文字列
	v     int
	str   []byte // 文字列の引数の中身 (書かれる部分)
	isStr bool
}

// rfmSpec は書式の 1 つの `{…}`。
type rfmSpec struct {
	verb  byte
	width int
	zero  bool
	align byte // 0 / '<' / '>'
}

func (s rfmSpec) text() string {
	var b strings.Builder
	if s.align != 0 {
		b.WriteByte(s.align)
	}
	if s.zero {
		b.WriteByte('0')
	}
	if s.width > 0 {
		b.WriteString(strconv.Itoa(s.width))
	}
	if s.verb != 0 {
		b.WriteByte(s.verb)
	}
	return b.String()
}

// rfmFormat は v (型 t) を spec で書いた文字 (fclib/fmt.fc と同じ規則: x / X / b は同じ大きさの符号なし、幅は右に寄せ、0 で
// 埋めるときは符号の後ろに)。
func rfmFormat(a rfmArg, sp rfmSpec) string {
	// 文字列・true / false・文字は左に (`>` なら右に)、数は右に (`<` なら左に) 寄せる
	text := func(s string, left bool) string {
		n := max(sp.width-len(s), 0)
		if left {
			return s + strings.Repeat(" ", n)
		}
		return strings.Repeat(" ", n) + s
	}
	if a.isStr {
		return text(string(a.str), sp.align != '>')
	}
	t := a.t
	if t.bool && sp.verb == 0 {
		return text(strconv.FormatBool(a.v != 0), sp.align != '>')
	}
	if sp.verb == 'c' {
		return text(string([]byte{byte(a.v)}), sp.align != '>')
	}
	if sp.align == '<' { // 幅なしで書いてから後ろを埋める
		inner := sp
		inner.width, inner.align = 0, 0
		return text(rfmFormat(a, inner), true)
	}
	mask := 1<<(8*t.size) - 1
	var digits string
	neg := false
	switch sp.verb {
	case 'x':
		digits = strconv.FormatInt(int64(a.v&mask), 16)
	case 'X':
		digits = strings.ToUpper(strconv.FormatInt(int64(a.v&mask), 16))
	case 'b':
		digits = strconv.FormatInt(int64(a.v&mask), 2)
	default:
		k := a.v
		if k < 0 {
			neg, k = true, -k
		}
		digits = strconv.Itoa(k)
	}
	body := len(digits)
	if neg {
		body++
	}
	pad := max(sp.width-body, 0)
	switch {
	case sp.zero && neg:
		return "-" + strings.Repeat("0", pad) + digits
	case sp.zero:
		return strings.Repeat("0", pad) + digits
	case neg:
		return strings.Repeat(" ", pad) + "-" + digits
	}
	return strings.Repeat(" ", pad) + digits
}

type rfmGen struct {
	r     *rand.Rand
	src   strings.Builder
	want  strings.Builder
	strs  [][]byte // 文字列の定数 S0..
	arr   []byte   // グローバルの配列 A (0 を含む)
	n     int
	bufSz int
}

func (g *rfmGen) pick(n int) int        { return g.r.Intn(n) }
func (g *rfmGen) chance(p float64) bool { return g.r.Float64() < p }

func (g *rfmGen) line(stmt, want string) {
	g.src.WriteString("\t" + stmt + "\n")
	g.want.WriteString(want)
}

// value は型 t の値 (境目の値か、範囲の中のどれか)。
func (g *rfmGen) value(t *rfmType) int {
	if g.chance(0.6) || t.bool || t.enum {
		return t.vals[g.pick(len(t.vals))]
	}
	n := 1 << (8 * t.size)
	v := g.pick(n)
	if t.signed && v >= n/2 {
		v -= n
	}
	return v
}

// arg は引数 1 つ (整数・bool・enum は定数か実行時の値、ときどき文字列)。
func (g *rfmGen) arg() rfmArg {
	if g.chance(0.15) {
		switch g.pick(3) {
		case 0:
			i := g.pick(len(g.strs))
			return rfmArg{src: fmt.Sprintf("S%d", i), str: g.strs[i], isStr: true}
		case 1:
			return rfmArg{src: "A", str: upToZero(g.arr), isStr: true} // 配列は中の最初の 0 まで
		default:
			k := g.pick(len(g.arr))
			return rfmArg{src: fmt.Sprintf("&A[%d]", k), str: upToZero(g.arr[k:]), isStr: true} // *u8 は終端の 0 まで
		}
	}
	t := &rfmTypes[g.pick(len(rfmTypes))]
	v := g.value(t)
	lit := ""
	switch {
	case t.bool:
		lit = strconv.FormatBool(v != 0)
	case t.enum:
		lit = t.name + "." + rfmEnumName(*t, v)
	default:
		lit = fmt.Sprintf("(%d as %s)", v, t.name)
		if v < 0 {
			lit = fmt.Sprintf("((%d) as %s)", v, t.name)
		}
	}
	src := lit
	if g.chance(0.6) {
		src = fmt.Sprintf("id_%s(%s)", t.name, lit) // 実行時の値 (fmt.fc の関数で書く)
	}
	return rfmArg{src: src, t: t, v: v}
}

// spec は引数 a に使える書式。
func (g *rfmGen) spec(a rfmArg) rfmSpec {
	// textWidth は文字列・true / false・文字の幅と寄せ方
	textWidth := func(sp rfmSpec) rfmSpec {
		if g.chance(0.4) {
			sp.width = 1 + g.pick(12)
			sp.align = []byte{0, '<', '>'}[g.pick(3)]
		}
		return sp
	}
	if a.isStr {
		return textWidth(rfmSpec{})
	}
	t := a.t
	var sp rfmSpec
	switch {
	case t.bool:
		if g.chance(0.5) {
			return textWidth(rfmSpec{}) // true / false
		}
		sp.verb = 'd'
	case t.size == 1 && !t.enum && g.chance(0.12) && a.v >= 0x20 && a.v < 0x7f:
		return textWidth(rfmSpec{verb: 'c'})
	default:
		sp.verb = []byte{0, 0, 'd', 'x', 'X', 'b'}[g.pick(6)]
	}
	if g.chance(0.5) {
		sp.width = 1 + g.pick(8)
		if g.chance(0.1) {
			sp.width = 9 + g.pick(23) // 31 まで
		}
		sp.zero = g.chance(0.4)
		if g.chance(0.3) {
			sp.align = '>'
			if !sp.zero && g.chance(0.6) {
				sp.align = '<'
			}
		}
	}
	return sp
}

// text は書式の文字の部分 (fc の文字列に書けて、`{{` / `}}` を含むことがある) と、書かれる文字。
func (g *rfmGen) text() (string, string) {
	const chars = "abcXYZ 0129:;-+=#!?.,()<>/_*"
	var src, out strings.Builder
	for i := 0; i < g.pick(5); i++ {
		switch g.pick(12) {
		case 0:
			src.WriteString("{{")
			out.WriteByte('{')
		case 1:
			src.WriteString("}}")
			out.WriteByte('}')
		default:
			c := chars[g.pick(len(chars))]
			src.WriteByte(c)
			out.WriteByte(c)
		}
	}
	return src.String(), out.String()
}

// format は書式の文字列 (fc のソース)・引数のソース・書かれる文字。
func (g *rfmGen) format() (string, []string, string) {
	args := make([]rfmArg, 1+g.pick(4))
	for i := range args {
		args[i] = g.arg()
	}
	used := make([]bool, len(args))
	var src, out strings.Builder
	next := 0
	emit := func(i int, explicit bool) {
		a := args[i]
		sp := g.spec(a)
		src.WriteByte('{')
		if explicit {
			src.WriteString(strconv.Itoa(i))
		}
		if t := sp.text(); t != "" {
			src.WriteString(":" + t)
		}
		src.WriteByte('}')
		out.WriteString(rfmFormat(a, sp))
		used[i] = true
	}
	for k := 0; k < 1+g.pick(5); k++ {
		s, o := g.text()
		src.WriteString(s)
		out.WriteString(o)
		if next < len(args) && g.chance(0.7) {
			emit(next, false)
			next++
		} else {
			emit(g.pick(len(args)), true)
		}
	}
	for i := range args {
		if !used[i] {
			emit(i, true)
		}
	}
	s, o := g.text()
	src.WriteString(s)
	out.WriteString(o)
	var as []string
	for _, a := range args {
		as = append(as, a.src)
	}
	return src.String(), as, out.String()
}

// program は書式のプログラムと期待する出力。
func (g *rfmGen) program() (string, string) {
	var decl strings.Builder
	decl.WriteString("#fc 4\nuse console;\n")
	decl.WriteString("enum E:u8 {\n\tA,\n\tB = 7,\n\tC,\n\tD = 200,\n}\n")
	decl.WriteString("enum G:i16 {\n\tN = -300,\n\tM = 1000,\n}\n")
	for _, t := range rfmTypes {
		fmt.Fprintf(&decl, "function id_%s(x:%s):%s @(noinline)\n{\n\treturn x;\n}\n", t.name, t.name, t.name)
	}
	const chars = "abcdefXYZ 0123456789-+.:!"
	for i := 0; i < 3; i++ {
		b := make([]byte, g.pick(9))
		for k := range b {
			b[k] = chars[g.pick(len(chars))]
		}
		g.strs = append(g.strs, b)
		fmt.Fprintf(&decl, "const S%d = %q;\n", i, string(b))
	}
	g.arr = make([]byte, 8)
	for k := 0; k < 7; k++ {
		if g.chance(0.15) {
			continue // 0
		}
		g.arr[k] = chars[g.pick(len(chars))]
	}
	var as []string
	for _, c := range g.arr {
		as = append(as, strconv.Itoa(int(c)))
	}
	fmt.Fprintf(&decl, "var A:[8]u8 = [%s];\n", strings.Join(as, ", "))
	g.bufSz = 64
	fmt.Fprintf(&decl, "var buf:[%d]u8;\n", g.bufSz)

	for k := 0; k < 10+g.pick(10); k++ {
		f, args, out := g.format()
		call := func(name, dst string) string {
			return fmt.Sprintf(`%s(%s"%s", %s)`, name, dst, f, strings.Join(args, ", "))
		}
		switch x := g.pick(10); {
		case x < 5 || len(out) > g.bufSz:
			g.line(fmt.Sprintf(`@printf("%s\n", %s);`, f, strings.Join(args, ", ")), out+"\n")
		case x < 8:
			l := min(len(out)+g.pick(4), g.bufSz)
			g.line(fmt.Sprintf(`{ var s = %s; @printf("[{}]{}\n", s, @len(s)); }`, call("@format", fmt.Sprintf("buf[..%d], ", l))),
				fmt.Sprintf("[%s]%d\n", out, len(out)))
		default:
			l := max(len(out)-g.pick(4), 0)
			if g.chance(0.5) {
				l = min(len(out)+g.pick(3), g.bufSz)
			}
			got := out
			if l < len(out) {
				got = "" // 足りなければ長さ 0
			}
			g.line(fmt.Sprintf(`{ var s = %s; @printf("[{}]{}\n", s, @len(s)); }`, call("@try_format", fmt.Sprintf("buf[..%d], ", l))),
				fmt.Sprintf("[%s]%d\n", got, len(got)))
		}
	}
	return decl.String() + "function main():void\n{\n" + g.src.String() + "\tconsole.exit(0);\n}\n", g.want.String()
}

// TestRandomFormatV4 は書式のプログラムを -O 0 / -O 2 / インタプリタで走らせ、生成器の期待値と比べる。
func TestRandomFormatV4(t *testing.T) {
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
			g := &rfmGen{r: rand.New(rand.NewSource(seed))}
			src, want := g.program()
			files := map[string]string{"t.fc": src}
			for _, level := range []int{-1, 0} {
				out, err := rpRun(t, files, level, rpMaxCycles)
				if err != nil || out != want {
					t.Fatalf("level %d (seed %d): got %q, %v\nwant %q\n%s", level, seed, out, err, want, src)
				}
			}
			if *randInterp {
				if out, ok, err := rpInterp(t, files); ok && (err != nil || out != want) {
					t.Fatalf("interp (seed %d): got %q, %v\nwant %q\n%s", seed, out, err, want, src)
				}
			}
		})
	}
}
