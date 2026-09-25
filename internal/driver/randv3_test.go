package driver

// fuzz の生成器の fc 3 の部分 (TestRandomV3Programs): fc 3 のモジュール v3m を生成し、その関数を main などの式から
// ランダムな引数で呼ぶ (fc 2 と fc 3 のモジュールは混ぜられる)。-O 0 / -O 2 / IR のインタプリタで出力を比べる。
//
// v3m の関数は u16 の acc に結果を集めて返す。本体は fc 3 の機能の組み合わせ: slice (範囲・添字・@len・ループ・
// @copy・大域の slice・slice の slice)、広い slice [:u16]T、[]const T / *const T の引数、enum と switch (fallthrough と
// case ごとの宣言、ジャンプ表になる大きさ)、@len(enum)、@null_fn の関数ポインタ。範囲は実行時に検査しないので、
// 生成器が必ず正しい範囲になる式を作る (vb は 24 要素、VT は 12 要素、vbig は 260 要素)。

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
)

// rpV3 は v3m モジュールの生成の状態。
type rpV3 struct {
	enum    []string // enum Color のメンバー
	vt      []int    // const VT の値
	funcs   []*rpFunc
	inline  bool // vsum を @(inline) にする
	n       int  // 文ごとのローカルの名前の番号
	wideArr bool // vbig:[260]u8 と広い slice
}

// v3Type は fc 2 の型名の fc 3 の名前。
func v3Type(t rpType) string {
	switch t.name {
	case "int":
		return "u8"
	case "sint":
		return "i8"
	case "int16":
		return "u16"
	}
	return "i16"
}

// genV3 は v3m の関数を作り、呼べる関数 (g.funcs) に加える。
func (g *rpGen) genV3() {
	v := g.v3
	names := []string{"Red", "Green", "Blue", "Gray", "Cyan"}
	v.enum = names[:3+g.pick(3)]
	for i := 0; i < 12; i++ {
		v.vt = append(v.vt, g.pick(256))
	}
	v.inline = g.chance(0.3)
	v.wideArr = g.chance(0.5)
	for i := 0; i < 1+g.pick(3); i++ {
		f := &rpFunc{name: fmt.Sprintf("v%d", i), mod: "v3m", ret: g.typ()}
		for k := 0; k < 1+g.pick(2); k++ {
			f.params = append(f.params, rpVar{name: fmt.Sprintf("p%d", k), typ: g.typ()})
		}
		var body strings.Builder
		for k := 0; k < 2+g.pick(5); k++ {
			body.WriteString(g.v3Stmt(f, 0))
		}
		ps := make([]string, len(f.params))
		for k, p := range f.params {
			ps[k] = fmt.Sprintf("%s:%s", p.name, v3Type(p.typ))
		}
		f.text = fmt.Sprintf("public function %s(%s):%s\n{\n\tvar acc:u16 = 0;\n%s\treturn acc as %s;\n}\n", f.name, strings.Join(ps, ", "), v3Type(f.ret), body.String(), v3Type(f.ret))
		v.funcs = append(v.funcs, f)
		g.funcs = append(g.funcs, f)
	}
}

// v3Byte は v3m の関数の中の 1 バイトの式 (引数・acc・大域)。
func (g *rpGen) v3Byte(f *rpFunc) string {
	p := f.params[g.pick(len(f.params))].name
	switch g.pick(6) {
	case 0, 1:
		return fmt.Sprintf("(%s as u8)", p)
	case 2:
		return "(acc as u8)"
	case 3:
		return "vcnt"
	case 4:
		return fmt.Sprintf("vb[%d]", g.pick(24))
	}
	return fmt.Sprintf("((%s as u8) + %d)", p, g.pick(256))
}

// v3Stmt は v3m の関数の文 1 つ (タブ 1 つの字下げ、改行つき)。文の中のローカルは文ごとに別の名前 (fc の { } は
// スコープを作らない)。
func (g *rpGen) v3Stmt(f *rpFunc, depth int) string {
	v := g.v3
	v.n++
	r := strings.NewReplacer("$s", fmt.Sprintf("s_%d", v.n), "$lo", fmt.Sprintf("lo_%d", v.n), "$hi", fmt.Sprintf("hi_%d", v.n),
		"$k", fmt.Sprintf("k_%d", v.n), "$u", fmt.Sprintf("u_%d", v.n), "$w", fmt.Sprintf("w_%d", v.n))
	return r.Replace(g.v3StmtText(f))
}

// v3StmtText は v3Stmt の雛形 ($s などは文ごとのローカルの名前)。
func (g *rpGen) v3StmtText(f *rpFunc) string {
	v := g.v3
	b := func() string { return g.v3Byte(f) }
	switch g.pick(15) {
	case 0: // 範囲 (lo <= 7、hi <= 15 < 24)
		return fmt.Sprintf("\t{\n\t\tvar $lo = %s %% 8;\n\t\tvar $hi = $lo + %s %% 9;\n\t\tvar $s:[]u8 = vb[$lo..$hi];\n\t\tacc += vsum($s);\n\t}\n", b(), b())
	case 1: // 定数の配列の範囲 ([]const)
		return fmt.Sprintf("\tacc += vsum(VT[%s %% 4..8]);\n", b())
	case 2: // slice に書く
		return fmt.Sprintf("\tvfill(vb[%s %% 8..], %s);\n", b(), b())
	case 3: // 添字の読み書き
		return fmt.Sprintf("\t{\n\t\tvar $s:[]u8 = vb[..];\n\t\t$s[%s %% 24] = %s;\n\t\tacc += $s[%s %% 24] + @len($s);\n\t}\n", b(), b(), b())
	case 4: // @copy (短いほうの長さ)
		return fmt.Sprintf("\tacc += @copy(vb[%s %% 8..], VT[%s %% 4..]);\n", b(), b())
	case 5: // 大域の slice
		return fmt.Sprintf("\tvs = vb[%s %% 12..20];\n\tacc += @len(vs) + vs[0];\n", b())
	case 6: // slice を縮めるループ
		return fmt.Sprintf("\t{\n\t\tvar $s:[]u8 = vb[..];\n\t\tvar $k = %s %% 8 + 1;\n\t\twhile (@len($s) > $k) {\n\t\t\tacc += $s[0];\n\t\t\t$s = $s[1..];\n\t\t}\n\t}\n", b())
	case 7: // slice の slice
		return fmt.Sprintf("\t{\n\t\tvar $s:[]u8 = vb[2..20];\n\t\tvar $lo = %s %% 5;\n\t\tvar $u = $s[$lo..$lo + 3];\n\t\tacc += vsum($u) + $u[1];\n\t}\n", b())
	case 8: // *const の引数
		return fmt.Sprintf("\tacc += vrd(&VT[%s %% 4], %s %% 8);\n\tacc += vrd(&vb[%s %% 8], %s %% 16);\n", b(), b(), b(), b())
	case 9: // @null_fn の関数ポインタ
		return fmt.Sprintf("\tvhook = @null_fn;\n\tif (%s & 1) {\n\t\tvhook = vset;\n\t}\n\tvhook(%s);\n\tacc += vcnt;\n", b(), b())
	case 10: // @len(enum)
		return fmt.Sprintf("\tacc += @len(Color) * %s;\n", b())
	case 11: // 広い slice
		if !v.wideArr {
			return fmt.Sprintf("\tacc ^= %s;\n", b())
		}
		return fmt.Sprintf("\t{\n\t\tvar $w:[:u16]u8 = vbig;\n\t\t$w[(%s as u16) + %d] = %s;\n\t\tacc += vsumw($w[(%s as u16)..260]) + @len($w);\n\t}\n", b(), g.pick(4), b(), b())
	case 12, 13: // enum の switch
		return g.v3EnumSwitch(f)
	}
	return g.v3ByteSwitch(f)
}

// v3CaseBody は case の本体 (case ごとの宣言 t を含むことがある)。
func (g *rpGen) v3CaseBody(f *rpFunc) string {
	switch g.pick(3) {
	case 0:
		return fmt.Sprintf("\t\tvar t:u8 = %s;\n\t\tacc += t;\n", g.v3Byte(f))
	case 1:
		return fmt.Sprintf("\t\tvar t:u16 = %d;\n\t\tacc ^= t;\n", g.pick(65536))
	}
	return fmt.Sprintf("\t\tacc += %s;\n", g.v3Byte(f))
}

// v3EnumSwitch は enum の変数を決めて switch する (fallthrough、case ごとの宣言、default の有無)。
func (g *rpGen) v3EnumSwitch(f *rpFunc) string {
	v := g.v3
	var b strings.Builder
	fmt.Fprintf(&b, "\tif (%s & 1) {\n\t\tvc = .%s;\n\t} else {\n\t\tvc = .%s;\n\t}\n", g.v3Byte(f), v.enum[g.pick(len(v.enum))], v.enum[g.pick(len(v.enum))])
	hasDefault := g.chance(0.6)
	b.WriteString("\tswitch (vc) {\n")
	members := v.enum
	if hasDefault {
		members = members[:len(members)-1] // 最後のメンバーは default に任せる
	}
	for i, m := range members {
		fmt.Fprintf(&b, "\tcase .%s:\n%s", m, g.v3CaseBody(f))
		if (i < len(members)-1 || hasDefault) && g.chance(0.4) {
			b.WriteString("\t\tfallthrough;\n")
		}
	}
	if hasDefault {
		fmt.Fprintf(&b, "\tdefault:\n%s", g.v3CaseBody(f))
	}
	b.WriteString("\t}\n")
	return b.String()
}

// v3ByteSwitch は 1 バイトの値の switch (case が 10 個以上で密ならジャンプ表になる)。
func (g *rpGen) v3ByteSwitch(f *rpFunc) string {
	var b strings.Builder
	n := 3 + g.pick(10)
	fmt.Fprintf(&b, "\tswitch (%s %% %d) {\n", g.v3Byte(f), n+2)
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "\tcase %d:\n%s", i, g.v3CaseBody(f))
		if g.chance(0.3) {
			b.WriteString("\t\tfallthrough;\n")
		}
	}
	fmt.Fprintf(&b, "\tdefault:\n%s\t}\n", g.v3CaseBody(f))
	return b.String()
}

// v3Source は v3m.fc のソース。
func (g *rpGen) v3Source() string {
	v := g.v3
	var b strings.Builder
	b.WriteString("#fc 3\nuse mem;\n")
	fmt.Fprintf(&b, "enum Color { %s }\n", strings.Join(v.enum, ", "))
	vals := make([]string, len(v.vt))
	for i, x := range v.vt {
		vals[i] = fmt.Sprint(x)
	}
	fmt.Fprintf(&b, "const VT:[?]u8 = [%s];\n", strings.Join(vals, ", "))
	b.WriteString("var vb:[24]u8;\nvar vs:[]u8;\nvar vc:Color;\nvar vhook:fn(u8):void;\nvar vcnt:u8;\n")
	if v.wideArr {
		b.WriteString("var vbig:[260]u8 @(segment: \"BSS_EX\");\n")
	}
	inline := ""
	if v.inline {
		inline = " @(inline)"
	}
	fmt.Fprintf(&b, "function vsum(s:[]const u8):u16%s\n{\n\tvar n:u16 = 0;\n\tfor (var i = 0; i < @len(s); i += 1) {\n\t\tn += s[i];\n\t}\n\treturn n;\n}\n", inline)
	b.WriteString("function vsumw(s:[:u16]const u8):u16\n{\n\tvar n:u16 = 0;\n\tfor (var i:u16 = 0; i < @len(s); i += 1) {\n\t\tn += s[i];\n\t}\n\treturn n;\n}\n")
	b.WriteString("function vfill(s:[]u8, x:u8):void\n{\n\tfor (var i = 0; i < @len(s); i += 1) {\n\t\ts[i] = x + i;\n\t}\n}\n")
	b.WriteString("function vrd(p:*const u8, n:u8):u8\n{\n\tvar r:u8 = 0;\n\tfor (var i = 0; i < n; i += 1) {\n\t\tr ^= p[i];\n\t}\n\treturn r;\n}\n")
	b.WriteString("function vset(x:u8):void\n{\n\tvcnt += x;\n}\n")
	for _, f := range v.funcs {
		b.WriteString(f.text)
	}
	return b.String()
}

// TestRandomV3Programs は fc 3 のモジュールを含むランダムなプログラムを -O 0 / -O 2 / インタプリタで比べる。
func TestRandomV3Programs(t *testing.T) {
	t.Parallel()
	base := *randSeed
	if base == 0 {
		base = 1
	}
	for k := 0; k < *randN; k++ {
		seed := base + int64(k)
		t.Run(fmt.Sprintf("seed%d", seed), func(t *testing.T) {
			t.Parallel()
			g := &rpGen{r: rand.New(rand.NewSource(seed)), v3: &rpV3{}}
			g.genProgram()
			res := rpCheck(t, g.sources())
			switch res.kind {
			case "ok":
			case "error":
				for _, skip := range []string{"frame size over", "memory area overflow", "zero page index wrapped"} {
					if strings.Contains(res.detail, skip) {
						t.Skipf("生成したプログラムが大きすぎる (seed %d): %s", seed, skip)
					}
				}
				t.Fatalf("ビルド失敗 (生成器の問題) (seed %d):\n%s\n%s", seed, g.allSource(), res.detail)
			case "hang":
				t.Skipf("両方のレベルでサイクルの上限を超えた (seed %d)", seed)
			default:
				rpMinimize(t, g, res.kind)
				res = rpCheck(t, g.sources())
				res.detail += "\n切り分け: " + rpLocate(t, g.sources())
				t.Errorf("%s (seed %d):\n%s\n%s", res.kind, seed, g.allSource(), res.detail)
			}
		})
	}
}

// TestRandomSourceStable: 生成したプログラムの sources() が何度呼んでも同じ (最小化は文を消して呼び直すので、呼ぶたびに
// 乱数で作り直す部分があると別のプログラムを比べてしまう。const の表の値でそうなっていた)。
func TestRandomSourceStable(t *testing.T) {
	t.Parallel()
	for seed := int64(1); seed <= 200; seed++ {
		for _, v3 := range []bool{false, true} {
			g := &rpGen{r: rand.New(rand.NewSource(seed))}
			if v3 {
				g.v3 = &rpV3{}
			}
			g.genProgram()
			a, b := g.sources(), g.sources()
			for name := range a {
				if a[name] != b[name] {
					t.Fatalf("seed %d (v3 %v): %s が呼ぶたびに変わる", seed, v3, name)
				}
			}
		}
	}
}
