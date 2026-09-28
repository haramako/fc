package driver

// 定数の畳み込みと実行時の計算の差分テスト (TestRandomConstFold)。同じ式を 2 通りに書いて結果を比べる:
//
//	定数の形: 型付きの定数 `(200 as u8)` のまま (sema が畳み込む)
//	変数の形: 同じ値を noinline の関数で返す `id_u8(200)` (実行時に計算する)
//
// TestRandomPrograms の判定 (-O 0 / -O 2 / 最適化前の IR のインタプリタ) は sema の誤りを 3 つとも同じように間違えるので
// 見えない。型付きの定数の畳み込みが型の幅で折り返さない、16 ビットを超える定数の比較、@min の定数、@bitcast の定数
// (2026-09-27 の 2 回目の調査で見つかったもの) はこの形で見える。型のない定数 (リテラル) は相手の型に合わせる規則が
// 型付きの値と違うのが正しいので、どちらの形でもリテラルのまま書く。
//
// 変数の形で型のエラーになる式 (型のない定数が相手の型に収まらない比較など) は捨てて作り直す。

import (
	"errors"
	"flag"
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"github.com/haramako/fc/internal/diag"
)

// rfType は生成に使う整数型 (fc 3 の名前)。
type rfType struct {
	name   string
	size   int
	signed bool
}

var rfTypes = []rfType{{"u8", 1, false}, {"i8", 1, true}, {"u16", 2, false}, {"i16", 2, true}}

func (t rfType) lohi() (int, int) {
	hi := 1<<(8*t.size) - 1
	if t.signed {
		return -(hi + 1) / 2, (hi+1)/2 - 1
	}
	return 0, hi
}

// rfExpr は同じ式の定数の形 (c) と変数の形 (v)。v が `{` で始まれば printf まで含む 1 行の文 (widen)。
type rfExpr struct{ c, v string }

// widen はときどき、式を狭い型 T1 の値にしてから広い型 T2 の変数へ暗黙に代入する形にする (変数の形では暗黙の拡張、
// 定数の形では `as`)。値の出どころは式・大域変数・配列の要素・struct のフィールド (i8 の値で i16 を初期化するときに
// 符号拡張していなかった。-O 0 / -O 2 / インタプリタの判定では見えない)。
func (g *rfGen) widen(e rfExpr) rfExpr {
	if !g.chance(0.3) {
		return e
	}
	t1 := rfTypes[g.pick(2)]   // u8 / i8
	t2 := rfTypes[2+g.pick(2)] // u16 / i16
	c := fmt.Sprintf("((((%s) as %s)) as %s)", e.c, t1.name, t2.name)
	src := fmt.Sprintf("((%s) as %s)", e.v, t1.name)
	var pre string
	switch g.pick(4) {
	case 1:
		pre = fmt.Sprintf("gv_%s = %s; ", t1.name, src)
		src = "gv_" + t1.name
	case 2:
		pre = fmt.Sprintf("ga_%s[1] = %s; ", t1.name, src)
		src = fmt.Sprintf("ga_%s[1]", t1.name)
	case 3:
		pre = fmt.Sprintf("gs.f_%s = %s; ", t1.name, src)
		src = "gs.f_" + t1.name
	}
	return rfExpr{c: c, v: fmt.Sprintf(`{ %svar w:%s = %s; printf(((w) as i16), "\n"); }`, pre, t2.name, src)}
}

// rfN は TestRandomConstFold のプログラム数 (1 本で 3 回ビルドするので既定は少なめ。`-foldn 500 -randseed N` で増やす)。
var rfN = flag.Int("foldn", 10, "TestRandomConstFold のプログラム数")

type rfGen struct{ r *rand.Rand }

func (g *rfGen) pick(n int) int        { return g.r.Intn(n) }
func (g *rfGen) typ() rfType           { return rfTypes[g.pick(len(rfTypes))] }
func (g *rfGen) chance(p float64) bool { return g.r.Float64() < p }

// value は型 t の値 (端の値を多めに)。
func (g *rfGen) value(t rfType) int {
	lo, hi := t.lohi()
	switch g.pick(6) {
	case 0:
		return lo
	case 1:
		return hi
	case 2:
		return g.pick(8)
	case 3:
		return hi - g.pick(4)
	}
	return lo + g.r.Intn(hi-lo+1)
}

// leaf は型付きの定数 (両方の形で値の出どころだけが違う) か、型のないリテラル。
func (g *rfGen) leaf() rfExpr {
	if g.chance(0.25) {
		// 型のないリテラル (0〜300 と小さい負の数。16 ビットを超える大きな数もときどき)
		var n int
		switch g.pick(5) {
		case 0:
			n = -g.pick(130)
		case 1:
			n = 65530 + g.pick(20)
		default:
			n = g.pick(301)
		}
		s := fmt.Sprint(n)
		if n < 0 {
			s = "(" + s + ")"
		}
		return rfExpr{s, s}
	}
	t := g.typ()
	n := g.value(t)
	return rfExpr{fmt.Sprintf("(%d as %s)", n, t.name), fmt.Sprintf("id_%s(%d)", t.name, n)}
}

// expr は深さ depth までの式。
func (g *rfGen) expr(depth int) rfExpr {
	if depth <= 0 || g.chance(0.2) {
		return g.leaf()
	}
	a := g.expr(depth - 1)
	switch g.pick(10) {
	case 0, 1, 2:
		b := g.expr(depth - 1)
		op := []string{"+", "-", "*", "&", "|", "^"}[g.pick(6)]
		return rfExpr{fmt.Sprintf("(%s %s %s)", a.c, op, b.c), fmt.Sprintf("(%s %s %s)", a.v, op, b.v)}
	case 3:
		b := g.expr(depth - 1)
		op := []string{"/", "%"}[g.pick(2)]
		return rfExpr{fmt.Sprintf("(%s %s (%s | 1))", a.c, op, b.c), fmt.Sprintf("(%s %s (%s | 1))", a.v, op, b.v)}
	case 4:
		op := []string{"<<", ">>"}[g.pick(2)]
		n := g.pick(10)
		return rfExpr{fmt.Sprintf("(%s %s %d)", a.c, op, n), fmt.Sprintf("(%s %s %d)", a.v, op, n)}
	case 5:
		b := g.expr(depth - 1)
		op := []string{"<", "<=", "==", "!=", ">", ">="}[g.pick(6)]
		return rfExpr{fmt.Sprintf("((%s %s %s) as u8)", a.c, op, b.c), fmt.Sprintf("((%s %s %s) as u8)", a.v, op, b.v)}
	case 6:
		t := g.typ()
		return rfExpr{fmt.Sprintf("((%s) as %s)", a.c, t.name), fmt.Sprintf("((%s) as %s)", a.v, t.name)}
	case 7:
		// ビットの読み替え (同じ大きさの型同士)
		t := g.typ()
		u := rfTypes[(indexOfType(t)+1)%2+(indexOfType(t)/2)*2] // 同じ大きさで符号だけ違う型
		return rfExpr{fmt.Sprintf("@bitcast(%s, ((%s) as %s))", u.name, a.c, t.name), fmt.Sprintf("@bitcast(%s, ((%s) as %s))", u.name, a.v, t.name)}
	case 8:
		b := g.expr(depth - 1)
		f := []string{"@min", "@max"}[g.pick(2)]
		return rfExpr{fmt.Sprintf("%s(%s, %s)", f, a.c, b.c), fmt.Sprintf("%s(%s, %s)", f, a.v, b.v)}
	}
	op := []string{"-", "~"}[g.pick(2)]
	return rfExpr{fmt.Sprintf("(%s%s)", op, a.c), fmt.Sprintf("(%s%s)", op, a.v)}
}

func indexOfType(t rfType) int {
	for i, u := range rfTypes {
		if u == t {
			return i
		}
	}
	return 0
}

// rfSource は式の並びのプログラム (1 行 1 式。行番号 = 4 + 添字)。
func rfSource(exprs []string) string {
	var b strings.Builder
	b.WriteString("#fc 3\nuse * from stdio;\n")
	for _, t := range rfTypes {
		fmt.Fprintf(&b, "function id_%s(x:%s):%s @(noinline) { return x; } var gv_%s:%s; var ga_%s:[3]%s;\n", t.name, t.name, t.name, t.name, t.name, t.name, t.name)
	}
	b.WriteString("struct GS { a:u8; f_u8:u8; f_i8:i8; } var gs:GS; function main():void\n{\n")
	for _, e := range exprs {
		if strings.HasPrefix(e, "{") {
			fmt.Fprintf(&b, "\t%s\n", e) // 文 (暗黙の拡張の初期化を通す形。1 行)
			continue
		}
		fmt.Fprintf(&b, "\tprintf(((%s) as i16), \"\\n\");\n", e)
	}
	b.WriteString("\texit(0);\n}\n")
	return b.String()
}

// rfFirstLine はソースの main の最初の式の行番号。
const rfFirstLine = 2 + 4 + 2 + 1

// TestRandomConstFold は定数の形と変数の形で同じ式の結果が同じかを比べる。
func TestRandomConstFold(t *testing.T) {
	t.Parallel()
	base := *randSeed
	if base == 0 {
		base = 1
	}
	for k := 0; k < *rfN; k++ {
		seed := base + int64(k)
		t.Run(fmt.Sprintf("seed%d", seed), func(t *testing.T) {
			t.Parallel()
			g := &rfGen{r: rand.New(rand.NewSource(seed))}
			var es []rfExpr
			for i := 0; i < 24; i++ {
				es = append(es, g.widen(g.expr(1+g.pick(4))))
			}
			// 変数の形で型のエラーになる式を捨てる (エラーの行から式を引く)
			var vo2 string
			for try := 0; ; try++ {
				vs := make([]string, len(es))
				for i, e := range es {
					vs[i] = e.v
				}
				out, err := rpRun(t, map[string]string{"t.fc": rfSource(vs)}, 0, rpMaxCycles)
				if err == nil {
					vo2 = out
					break
				}
				drop := map[int]bool{}
				var list diag.ErrorList
				var one *diag.Error
				switch {
				case errors.As(err, &list):
				case errors.As(err, &one):
					list = diag.ErrorList{one}
				}
				for _, e := range list {
					drop[e.Pos.Line-rfFirstLine] = true
				}
				if len(drop) == 0 || try > 10 {
					t.Fatalf("変数の形のビルド失敗 (seed %d):\n%s\n%v", seed, rfSource(vs), err)
				}
				var keep []rfExpr
				for i, e := range es {
					if !drop[i] {
						keep = append(keep, e)
					}
				}
				es = keep
			}
			vs := make([]string, len(es))
			cs := make([]string, len(es))
			for i, e := range es {
				vs[i], cs[i] = e.v, e.c
			}
			vo0, err := rpRun(t, map[string]string{"t.fc": rfSource(vs)}, -1, rpMaxCycles)
			if err != nil {
				t.Fatalf("変数の形 -O 0 (seed %d): %v", seed, err)
			}
			co2, err := rpRun(t, map[string]string{"t.fc": rfSource(cs)}, 0, rpMaxCycles)
			if err != nil {
				t.Fatalf("定数の形のビルド失敗 (seed %d): 変数の形は通る\n%s\n%v", seed, rfSource(cs), err)
			}
			if vo0 != vo2 {
				t.Errorf("変数の形の -O 0 と -O 2 が違う (seed %d)", seed)
			}
			cl, vl := strings.Split(co2, "\n"), strings.Split(vo2, "\n")
			for i := range es {
				if i < len(cl) && i < len(vl) && cl[i] != vl[i] {
					t.Errorf("seed %d: 定数の形と変数の形が違う: %s と %s\n  定数: %s\n  変数: %s", seed, cl[i], vl[i], es[i].c, es[i].v)
				}
			}
		})
	}
}

// TestConstFoldCases: TestRandomConstFold で見つかった、定数の畳み込みと実行時の計算の食い違い (2026-09-27)。
// 各行の定数の形と変数の形が同じ値になる。
func TestConstFoldCases(t *testing.T) {
	t.Parallel()
	cases := []rfExpr{
		{"(65546 / ((37037 as u16) | 1))", "(65546 / (id_u16(37037) | 1))"},                                                       // 16 ビットを超える定数を型に切り詰めてから割る
		{"@min(((-128 as i8) >> 4), ((49 as u8) / ((2 as u16) | 1)))", "@min((id_i8(-128) >> 4), (id_u8(49) / (id_u16(2) | 1)))"}, // @min の畳み込みの型
		{"(-(~(((0 as u8) + 242) % ((((-5)) as i8) | 1))))", "(-(~((id_u8(0) + 242) % ((((-5)) as i8) | 1))))"},                   // 剰余の前に i8 の値に
		{"(((255 as u8) >= 65539) as u8)", "((id_u8(255) >= 65539) as u8)"},                                                       // 比較では型のない定数は折り返さない
		{"((-4) * (68 as u8))", "((-4) * id_u8(68))"},                                                                             // 互換型の初期値
		{"@max(294, ((-45 as i8) & (-42 as i8)))", "@max(294, (id_i8(-45) & id_i8(-42)))"},                                        // @max は比較の規則で型を決める
		{"(((-7 as i8) >= @min((255 as u8), (0 as u16))) as u8)", "((id_i8(-7) >= @min(id_u8(255), id_u16(0))) as u8)"},           // 符号付きのリテラルを広げる
		{"(@bitcast(i8, (255 as u8)) * (-27779 as i16))", "(@bitcast(i8, id_u8(255)) * id_i16(-27779))"},                          // 読み替えた型で読んでから広げる
		{"((@min((~(-121 as i8)), (-128 as i8)) <= 65544) as u8)", "((@min((~id_i8(-121)), id_i8(-128)) <= 65544) as u8)"},        // 両方とも定数の 16 ビットを超える比較
		{"((65533 > ((-81 as i8) >> 9)) as u8)", "((65533 > (id_i8(-81) >> 9)) as u8)"},                                            // 型のない定数が i16 を超える: u16 で比べる
		{"(((-5 as i8) < 271) as u8)", "((id_i8(-5) < 271) as u8)"},                                                                 // 型のない定数が i16 に収まる: 符号付きで比べる
		{"((65534 >= (127 as i8)) as u8)", "((65534 >= id_i8(127)) as u8)"},
		{"@min(((((@min(65537, 65535)) as u8)) as u16), (~(0 as u8)))", "@min(((((@min(65537, 65535)) as u8)) as u16), (~id_u8(0)))"}, // 切り詰めた定数の上位 (split)
		{"@max(@bitcast(u8, ((@max((-65), 65534)) as i8)), @bitcast(u16, ((((4 as i8) + (0 as u8))) as i16)))", "@max(@bitcast(u8, ((@max((-65), 65534)) as i8)), @bitcast(u16, (((id_i8(4) + id_u8(0))) as i16)))"}, // 狭めた定数 (split)
		{"@max(((@min(180, (-47))) as u8), ((((61 * (-18 as i8)) | (65532 / ((65535 as u16) | 1)))) as i16))", "@max(((@min(180, (-47))) as u8), ((((61 * id_i8(-18)) | (65532 / (id_u16(65535) | 1)))) as i16))"}, // 符号の違う型にした定数 (split)
		{"(@bitcast(u16, ((@bitcast(u8, ((((42272 as u16) >> 1)) as i8))) as i16)) >> 0)", "(@bitcast(u16, ((@bitcast(u8, (((id_u16(42272) >> 1)) as i8))) as i16)) >> 0)"}, // 切り詰めた値のシフトの連鎖 (ssa)
	}
	cs := make([]string, len(cases))
	vs := make([]string, len(cases))
	for i, c := range cases {
		cs[i], vs[i] = c.c, c.v
	}
	co, err := rpRun(t, map[string]string{"t.fc": rfSource(cs)}, 0, rpMaxCycles)
	if err != nil {
		t.Fatal(err)
	}
	vo, err := rpRun(t, map[string]string{"t.fc": rfSource(vs)}, 0, rpMaxCycles)
	if err != nil {
		t.Fatal(err)
	}
	cl, vl := strings.Split(co, "\n"), strings.Split(vo, "\n")
	for i, c := range cases {
		if cl[i] != vl[i] {
			t.Errorf("%s: 定数 %s、変数 %s", c.c, cl[i], vl[i])
		}
	}
}
