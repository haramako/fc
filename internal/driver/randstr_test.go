package driver

// fc 4 の文字列のランダムテスト (TestRandomStringsV4)。fc 4 のソースを直接作り、期待値は生成器が Go で計算する (sema の誤りは
// -O 0 / -O 2 / インタプリタが同じように間違えるので、差分でなく期待値と比べる)。文字列は 0 終端にしない・エスケープは文字の
// リテラルと同じ・型を書かない文字列の配列は slice の表・型を書いた 2 次元配列は 0 で詰める (2026-09-30 の変更。
// Agent/wiki/fc4-facts.md)。同じバイトでも綴り (`\n` / `\x0A`、`A` / `\x41`) を毎回選ぶので、字句解析のエスケープも見る。

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
)

// rsGen は文字列のテストのプログラムと期待する出力を作る。
type rsGen struct {
	r    *rand.Rand
	src  strings.Builder // main の本体
	want strings.Builder
	n    int // 名前の連番
}

func (g *rsGen) pick(n int) int        { return g.r.Intn(n) }
func (g *rsGen) chance(p float64) bool { return g.r.Float64() < p }

// bytes は長さ 0〜max のランダムなバイト列 (印字できる文字が多め。改行・タブ・0・128 以上も)。
func (g *rsGen) bytes(max int) []byte {
	b := make([]byte, g.pick(max+1))
	for i := range b {
		switch x := g.pick(20); {
		case x < 12:
			b[i] = byte(0x20 + g.pick(0x5f))
		case x < 14:
			b[i] = '\n'
		case x < 15:
			b[i] = '\t'
		case x < 16:
			b[i] = 0
		case x < 17:
			b[i] = []byte{'\\', '"', '\''}[g.pick(3)]
		default:
			b[i] = byte(1 + g.pick(255))
		}
	}
	return b
}

// quote は b の fc 4 の文字列リテラル。同じバイトでも綴りを選ぶ (エスケープ `\n \t \0 \\ \" \'` か `\xNN`、印字できる文字も
// ときどき `\xNN`)。
func (g *rsGen) quote(b []byte) string {
	var s strings.Builder
	s.WriteByte('"')
	hex := func(c byte) {
		if g.chance(0.5) {
			fmt.Fprintf(&s, `\x%02x`, c)
		} else {
			fmt.Fprintf(&s, `\x%02X`, c)
		}
	}
	for _, c := range b {
		switch {
		case c == '\n' && g.chance(0.7):
			s.WriteString(`\n`)
		case c == '\t' && g.chance(0.7):
			s.WriteString(`\t`)
		case c == 0 && g.chance(0.7):
			s.WriteString(`\0`)
		case c == '\\' && g.chance(0.7):
			s.WriteString(`\\`)
		case c == '"' && g.chance(0.7):
			s.WriteString(`\"`)
		case c == '\'' && g.chance(0.5):
			s.WriteString(`\'`)
		case c == '\'' || (c >= 0x20 && c < 0x7f && c != '\\' && c != '"' && g.chance(0.9)):
			s.WriteByte(c)
		default:
			hex(c)
		}
	}
	s.WriteByte('"')
	return s.String()
}

func (g *rsGen) name(p string) string { g.n++; return fmt.Sprintf("%s%d", p, g.n) }

// line は main の 1 文 (字下げ 1 段) と、それが出す行 (want。無ければ "")。
func (g *rsGen) line(stmt, want string) {
	g.src.WriteString("\t" + stmt + "\n")
	g.want.WriteString(want)
}

func sumBytes(b []byte) int {
	n := 0
	for _, c := range b {
		n += int(c)
	}
	return n & 0xffff
}

// upToZero は b の最初の 0 の前まで (配列を @printf で出す・write_z)。
func upToZero(b []byte) []byte {
	for i, c := range b {
		if c == 0 {
			return b[:i]
		}
	}
	return b
}

// program はプログラム (t.fc) と期待する出力。
func (g *rsGen) program() (string, string) {
	var decl strings.Builder
	decl.WriteString("#fc 4\nuse console;\n")
	// 名前付きの文字列定数
	strs := make([][]byte, 2+g.pick(3))
	for i := range strs {
		strs[i] = g.bytes(20)
		fmt.Fprintf(&decl, "const S%d = %s;\n", i, g.quote(strs[i]))
	}
	// 型を書かない文字列の配列 (slice の表)
	tab := make([][]byte, 1+g.pick(4))
	var ts []string
	for i := range tab {
		tab[i] = g.bytes(8)
		ts = append(ts, g.quote(tab[i]))
	}
	fmt.Fprintf(&decl, "const T = [%s];\n", strings.Join(ts, ", "))
	// 型を書いた 2 次元配列 (行の長さまで 0 で詰める)
	rows := make([][]byte, 1+g.pick(3))
	width := 1
	var ps []string
	for i := range rows {
		rows[i] = g.bytes(6)
		width = max(width, len(rows[i]))
		ps = append(ps, g.quote(rows[i]))
	}
	width += g.pick(3)
	fmt.Fprintf(&decl, "const P:[%d][%d]u8 = [%s];\n", len(rows), width, strings.Join(ps, ", "))
	bufLen := 1 + g.pick(16)
	buf := make([]byte, bufLen)
	fmt.Fprintf(&decl, "var buf:[%d]u8;\n", bufLen)

	for k := 0; k < 12+g.pick(8); k++ {
		i := g.pick(len(strs))
		s := strs[i]
		sn := fmt.Sprintf("S%d", i)
		switch g.pick(11) {
		case 0:
			g.line(fmt.Sprintf(`@printf("{} {}\n", @len(%s), @sizeof(%s));`, sn, sn), fmt.Sprintf("%d %d\n", len(s), len(s)))
		case 1:
			v := g.name("c")
			g.line(fmt.Sprintf(`sum = 0; for (var %s in %s) { sum += %s; } @printf("{}\n", sum);`, v, sn, v), fmt.Sprintf("%d\n", sumBytes(s)))
		case 2:
			if len(s) > 0 {
				j := g.pick(len(s))
				g.line(fmt.Sprintf(`@printf("{}\n", %s[%d]);`, sn, j), fmt.Sprintf("%d\n", s[j]))
			}
		case 3:
			a := g.pick(len(s) + 1)
			b := a + g.pick(len(s)-a+1)
			sl, c := g.name("sl"), g.name("c")
			g.line(fmt.Sprintf(`{ var %s:[]const u8 = %s[%d..%d]; sum = 0; for (var %s in %s) { sum += %s; } @printf("{} {}\n", @len(%s), sum); }`, sl, sn, a, b, c, sl, c, sl),
				fmt.Sprintf("%d %d\n", b-a, sumBytes(s[a:b])))
		case 4:
			// 配列は最初の 0 まで、slice は長さの分
			sl := g.name("sv")
			g.line(fmt.Sprintf(`{ var %s:[]const u8 = %s; @printf("[{}]<{}>\n", %s, %s); }`, sl, sn, sn, sl), "["+string(upToZero(s))+"]<"+string(s)+">\n")
		case 5:
			j := g.pick(len(tab))
			w := fmt.Sprintf("%d %d %d", len(tab), 3*len(tab), len(tab[j]))
			if len(tab[j]) > 0 {
				x := g.pick(len(tab[j]))
				g.line(fmt.Sprintf(`@printf("{} {} {} {}\n", @len(T), @sizeof(T), @len(T[%d]), T[%d][%d]);`, j, j, x), fmt.Sprintf("%s %d\n", w, tab[j][x]))
			} else {
				g.line(fmt.Sprintf(`@printf("{} {} {}\n", @len(T), @sizeof(T), @len(T[%d]));`, j), w+"\n")
			}
		case 6:
			tot := 0
			for _, t := range tab {
				tot += len(t)
			}
			v := g.name("t")
			g.line(fmt.Sprintf(`sum = 0; for (var %s in T) { sum += @len(%s); } @printf("{}\n", sum);`, v, v), fmt.Sprintf("%d\n", tot))
		case 7:
			j, x := g.pick(len(rows)), g.pick(width)
			var c byte
			if x < len(rows[j]) {
				c = rows[j][x]
			}
			g.line(fmt.Sprintf(`@printf("{} {}\n", P[%d][%d], @sizeof(P));`, j, x), fmt.Sprintf("%d %d\n", c, len(rows)*width))
		case 8:
			// ローカルの配列の変数 (3 バイトの文字列は 3 バイトの配列) に書く
			b := g.bytes(10)
			if len(b) == 0 {
				b = []byte{'x'}
			}
			lv, c := g.name("lv"), g.name("c")
			j := g.pick(len(b))
			nb := append([]byte(nil), b...)
			nb[j] = 65
			g.line(fmt.Sprintf(`{ var %s = %s; %s[%d] = 65; sum = 0; for (var %s in %s) { sum += %s; } @printf("{} {}\n", @len(%s), sum); }`, lv, g.quote(b), lv, j, c, lv, c, lv),
				fmt.Sprintf("%d %d\n", len(b), sumBytes(nb)))
		case 9:
			// 別の綴りの同じ文字列と、1 バイト違う文字列との ==
			other := append([]byte(nil), s...)
			eq := "true"
			if len(other) > 0 && g.chance(0.5) {
				other[g.pick(len(other))]++
				eq = "false"
				if string(other) == string(s) {
					eq = "true"
				}
			}
			g.line(fmt.Sprintf(`@printf("{}\n", %s == %s);`, sn, g.quote(other)), eq+"\n")
		default:
			n := copy(buf, s)
			v := g.name("c")
			g.line(fmt.Sprintf(`{ var n = @copy(buf, %s); sum = 0; for (var %s in buf) { sum += %s; } @printf("{} {}\n", n, sum); }`, sn, v, v),
				fmt.Sprintf("%d %d\n", n, sumBytes(buf)))
		}
		if g.chance(0.15) {
			// 0 終端の文字列 (明示の \0) を write_z に
			b := append(g.bytes(8), 0)
			b = append(b, g.bytes(3)...)
			g.line(fmt.Sprintf(`console.write_z(%s); console.newline();`, g.quote(b)), string(upToZero(b))+"\n")
		}
	}
	return decl.String() + "function main():void\n{\n\tvar sum:u16 = 0;\n" + g.src.String() + "\tconsole.exit(0);\n}\n", g.want.String()
}

// TestRandomStringsV4 は fc 4 の文字列のプログラムを -O 0 / -O 2 / インタプリタで走らせ、生成器の期待値と比べる。
func TestRandomStringsV4(t *testing.T) {
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
			g := &rsGen{r: rand.New(rand.NewSource(seed))}
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
