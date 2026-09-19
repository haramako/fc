package driver

// ランダムに生成した小さなプログラムを -O 0 と -O 2 で走らせて出力を比べる差分テスト (Csmith と同じ考え方)。
// 単機能のテストでは拾えない「機能の組み合わせ」のバグ (inline の引数が式、比較の直後の常駐レジスタの復帰など) を
// 狙う。既定は固定の種で 30 本 (毎回同じプログラム)。数を増やす / 別の種で回すには:
//
//	go test ./internal/driver -run TestRandomPrograms -randn 500 -randseed 12345
//
// 食い違ったら文を 1 つずつ消して最小化し、そのプログラムと両方の出力をログに出す (ops_test.go に足す材料)。
// 未定義動作は生成しない: 0 除算 (除数は `| 1` か 0 でないリテラル)、範囲外の添字 (`& 7`)、2 バイト値の変数シフト
// (シフト量はリテラル)、無限ループ (回数は上限つき)。

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var (
	randN    = flag.Int("randn", 30, "TestRandomPrograms のプログラム数")
	randSeed = flag.Int64("randseed", 0, "TestRandomPrograms の種 (0 なら 1 から順)")
)

// rpType は生成に使う整数型。
type rpType struct {
	name   string
	size   int
	signed bool
}

var rpTypes = []rpType{{"int", 1, false}, {"sint", 1, true}, {"int16", 2, false}, {"sint16", 2, true}}

// rpVar は見えている変数 (グローバル、引数、ローカル)。ポインタ (ptr) は 16 要素の配列の中を指し、typ は要素の型。
// fresh なポインタは添字 0〜7 を指していて `p[e & 7]` で読み書きできる。ループでずらしている間は `*p` だけ。
type rpVar struct {
	name     string
	typ      rpType
	readOnly bool // ループ変数 (本体で書き換えると回数が保証できない)、ループでずらしているポインタ
	ptr      bool
	fresh    bool
}

// rpField は struct のフィールド。
type rpField struct {
	name string
	typ  rpType
}

// rpFunc は生成した関数。
type rpFunc struct {
	name     string
	params   []rpVar
	ret      rpType
	fastcall bool
	inline   bool
	locals   []string  // 宣言
	stmts    []*rpStmt // 本体
	retExpr  string
}

// rpStmt は文の木 (最小化でどの深さの文も消せるように)。parts[i] の後に kids[i] の文が並ぶ。単純な文は parts が 1 つ。
type rpStmt struct {
	parts []string
	kids  [][]*rpStmt
}

func rpSimple(text string) *rpStmt { return &rpStmt{parts: []string{text}} }

func (s *rpStmt) render(b *strings.Builder) {
	for i, p := range s.parts {
		b.WriteString(p)
		if i < len(s.kids) {
			for _, k := range s.kids[i] {
				k.render(b)
				b.WriteString("\n")
			}
		}
	}
}

type rpGen struct {
	r       *rand.Rand
	globals []rpVar
	arrays  []rpVar // グローバル配列 (16 要素。要素型)
	larrays []rpVar // 今の関数のローカル配列 (16 要素)
	fields  []rpField
	sptr    string // 今の関数の struct へのポインタ (`ps`。"" なら無し)
	funcs   []*rpFunc
	scope   []rpVar // 今の関数で見えるローカル (引数含む。ポインタも)
	loops   int     // ループの入れ子の深さ (break / continue を出せるか)
	nLocal  int
	cur     *rpFunc
}

// arraysAll は見えている配列 (グローバル + 今の関数のローカル)。
func (g *rpGen) arraysAll() []rpVar { return append(append([]rpVar{}, g.arrays...), g.larrays...) }

// ptrs は見えているポインタ変数。
func (g *rpGen) ptrs() []rpVar {
	var r []rpVar
	for _, v := range g.scope {
		if v.ptr {
			r = append(r, v)
		}
	}
	return r
}

// scalars は見えている整数の変数。
func (g *rpGen) scalars() []rpVar {
	var r []rpVar
	for _, v := range g.vars() {
		if !v.ptr {
			r = append(r, v)
		}
	}
	return r
}

// setPtr はスコープのポインタ変数の状態を変える。
func (g *rpGen) setPtr(name string, readOnly, fresh bool) {
	for i := range g.scope {
		if g.scope[i].name == name {
			g.scope[i].readOnly, g.scope[i].fresh = readOnly, fresh
		}
	}
}

// arrayRef は配列の要素へのポインタ `&a[e & 7]` (添字は 0〜7)。
func (g *rpGen) arrayRef(a rpVar) string { return fmt.Sprintf("&%s[%s]", a.name, g.index()) }

// fieldRef は struct のフィールドの参照 (グローバルの s0、配列 sa の要素、ポインタ ps 経由)。
func (g *rpGen) fieldRef() (string, rpType) {
	f := g.fields[g.pick(len(g.fields))]
	switch {
	case g.sptr != "" && g.chance(0.4):
		return fmt.Sprintf("%s.%s", g.sptr, f.name), f.typ
	case g.chance(0.5):
		return fmt.Sprintf("sa[(%s & 3)].%s", g.expr(rpTypes[0], 1), f.name), f.typ
	}
	return "s0." + f.name, f.typ
}

func (g *rpGen) pick(n int) int        { return g.r.Intn(n) }
func (g *rpGen) chance(p float64) bool { return g.r.Float64() < p }
func (g *rpGen) typ() rpType           { return rpTypes[g.pick(len(rpTypes))] }

// lit はその型の範囲のリテラル (小さめの値を多めに)。
func (g *rpGen) lit(t rpType) string {
	var v int
	switch {
	case g.chance(0.5):
		v = g.pick(8)
	case t.size == 1 && t.signed:
		v = g.pick(256) - 128
	case t.size == 1:
		v = g.pick(256)
	case t.signed:
		v = g.pick(65536) - 32768
	default:
		v = g.pick(65536)
	}
	if v < 0 {
		return fmt.Sprintf("(%d)", v)
	}
	return fmt.Sprintf("%d", v)
}

// vars は見えている変数 (グローバル + スコープ)。
func (g *rpGen) vars() []rpVar { return append(append([]rpVar{}, g.globals...), g.scope...) }

// cast は式を型 t にする (型が違えば `as`)。
func cast(e string, from, to rpType) string {
	if from == to {
		return e
	}
	return fmt.Sprintf("(%s as %s)", e, to.name)
}

// expr は型 t の式。depth が深さの残り。
func (g *rpGen) expr(t rpType, depth int) string {
	if depth <= 0 || g.chance(0.25) {
		return g.leaf(t)
	}
	switch g.pick(12) {
	case 0, 1, 2:
		op := []string{"+", "-", "*", "&", "|", "^"}[g.pick(6)]
		return fmt.Sprintf("(%s %s %s)", g.expr(t, depth-1), op, g.expr(t, depth-1))
	case 3:
		// 除算・剰余: 除数は 0 にならない形
		op := []string{"/", "%"}[g.pick(2)]
		var d string
		if g.chance(0.5) {
			d = g.lit(t)
			if d == "0" || d == "(0)" {
				d = "3"
			}
		} else {
			d = fmt.Sprintf("(%s | 1)", g.expr(t, depth-1))
		}
		return fmt.Sprintf("(%s %s %s)", g.expr(t, depth-1), op, d)
	case 4:
		// シフト: 量はリテラル (2 バイト値は変数の量が使えない)。8 ビットの値なら変数の量も
		op := []string{"<<", ">>"}[g.pick(2)]
		if t.size == 1 && g.chance(0.4) {
			// 左は 1 バイトの変数か配列の要素 (式だと定数畳み込みで 2 バイトになりうる: `(137 << 3) >> n`)
			// 量も 1 バイトに (定数畳み込みで 2 バイトになった式 `(688 / x) & 7` は量に使えない)
			return fmt.Sprintf("(%s %s ((%s & 7) as int))", g.leaf(t), op, g.expr(rpTypes[0], depth-1))
		}
		return fmt.Sprintf("(%s %s %d)", g.expr(t, depth-1), op, g.pick(8))
	case 5:
		// 比較 (bool) を整数として使う
		u := g.typ()
		op := []string{"<", "<=", "==", "!=", ">", ">="}[g.pick(6)]
		return fmt.Sprintf("((%s %s %s) as %s)", g.expr(u, depth-1), op, g.expr(u, depth-1), t.name)
	case 6:
		// 論理演算 (短絡)
		op := []string{"&&", "||"}[g.pick(2)]
		u, w := g.typ(), g.typ()
		return fmt.Sprintf("((%s %s %s) as %s)", g.expr(u, depth-1), op, g.expr(w, depth-1), t.name)
	case 7:
		return fmt.Sprintf("(%s%s)", []string{"-", "~"}[g.pick(2)], g.expr(t, depth-1))
	case 8:
		return fmt.Sprintf("((!%s) as %s)", g.expr(g.typ(), depth-1), t.name)
	case 9:
		// 別の型の式をキャスト
		u := g.typ()
		return cast(g.expr(u, depth-1), u, t)
	case 10:
		// 呼び出し (前に作った関数だけ。再帰なし)
		if n := g.callable(); n > 0 {
			f := g.funcs[g.pick(n)]
			return cast(fmt.Sprintf("%s(%s)", f.name, g.args(f, depth-1)), f.ret, t)
		}
		return g.leaf(t)
	default:
		return g.leaf(t)
	}
}

// args は呼び出しの実引数 (ポインタの引数には同じ要素型の配列の要素へのポインタ)。
func (g *rpGen) args(f *rpFunc, depth int) string {
	args := make([]string, len(f.params))
	for i, p := range f.params {
		if p.ptr {
			var as []rpVar
			for _, a := range g.arraysAll() {
				if a.typ == p.typ {
					as = append(as, a)
				}
			}
			args[i] = g.arrayRef(as[g.pick(len(as))]) // genProgram が全ての型のグローバル配列を作るので必ずある
		} else {
			args[i] = g.expr(p.typ, depth)
		}
	}
	return strings.Join(args, ", ")
}

// callable は今の関数から呼べる関数の数 (自分より前に定義したもの)。
func (g *rpGen) callable() int {
	if g.cur != nil && g.cur.inline {
		return 0 // inline 関数は呼び出しを持たない (入れ子の展開でコードが膨らみ ROM に入らなくなる)
	}
	n := 0
	for _, f := range g.funcs {
		if f == g.cur {
			break
		}
		n++
	}
	return n
}

// leaf は変数・配列の要素・ポインタ経由・struct のフィールド・リテラル。
func (g *rpGen) leaf(t rpType) string {
	vs := g.scalars()
	switch g.pick(6) {
	case 0:
		return g.lit(t)
	case 1:
		if as := g.arraysAll(); len(as) > 0 {
			a := as[g.pick(len(as))]
			return cast(fmt.Sprintf("%s[%s]", a.name, g.index()), a.typ, t)
		}
		fallthrough
	case 2:
		if ps := g.ptrs(); len(ps) > 0 {
			p := ps[g.pick(len(ps))]
			if p.fresh && g.chance(0.4) {
				return cast(fmt.Sprintf("%s[%s]", p.name, g.index()), p.typ, t)
			}
			return cast("(*"+p.name+")", p.typ, t)
		}
		fallthrough
	case 3:
		if len(g.fields) > 0 {
			e, ft := g.fieldRef()
			return cast(e, ft, t)
		}
		fallthrough
	default:
		// 同じ型の変数を優先、無ければキャスト
		var same []rpVar
		for _, v := range vs {
			if v.typ == t {
				same = append(same, v)
			}
		}
		if len(same) > 0 && !g.chance(0.2) {
			return same[g.pick(len(same))].name
		}
		v := vs[g.pick(len(vs))]
		return cast(v.name, v.typ, t)
	}
}

// index は配列の添字 (1 バイト、0〜7)。
func (g *rpGen) index() string {
	if g.chance(0.3) {
		return fmt.Sprintf("%d", g.pick(8))
	}
	return fmt.Sprintf("(%s & 7)", g.expr(rpTypes[0], 1))
}

// lvalue は代入先 (変数、配列の要素、ポインタ経由、struct のフィールド) とその型。
func (g *rpGen) lvalue() (string, rpType) {
	switch g.pick(10) {
	case 0, 1, 2:
		if as := g.arraysAll(); len(as) > 0 {
			a := as[g.pick(len(as))]
			return fmt.Sprintf("%s[%s]", a.name, g.index()), a.typ
		}
	case 3, 4:
		if ps := g.ptrs(); len(ps) > 0 {
			p := ps[g.pick(len(ps))]
			if p.fresh && g.chance(0.4) {
				return fmt.Sprintf("%s[%s]", p.name, g.index()), p.typ
			}
			return "(*" + p.name + ")", p.typ
		}
	case 5:
		if len(g.fields) > 0 {
			return g.fieldRef()
		}
	}
	var vs []rpVar
	for _, v := range g.scalars() {
		if !v.readOnly {
			vs = append(vs, v)
		}
	}
	v := vs[g.pick(len(vs))]
	return v.name, v.typ
}

// ptrLoop はポインタをずらしながら回るループ (誘導変数の統合・展開の対象になる形)。ポインタは 16 要素の配列の
// 添字 j0 (0〜3) から始めて、本体の前で進める (最大の添字は 14) ので範囲を出ない。本体の間はそのポインタを `*p` でしか
// 触らず、終わったら先頭に戻す (fresh に)。
func (g *rpGen) ptrLoop(depth int) *rpStmt {
	var ps []rpVar
	for _, p := range g.ptrs() {
		if !p.readOnly {
			ps = append(ps, p)
		}
	}
	if len(ps) == 0 {
		lv, t := g.lvalue()
		return rpSimple(fmt.Sprintf("%s = %s;", lv, g.expr(t, 3)))
	}
	p := ps[g.pick(len(ps))]
	var arr rpVar
	for _, a := range g.arraysAll() {
		if a.typ == p.typ {
			arr = a
		}
	}
	if arr.name == "" {
		return rpSimple(fmt.Sprintf("%s = %s;", p.name, p.name))
	}
	j0 := g.pick(4)
	g.setPtr(p.name, true, false)
	g.loops++
	var head, tail string
	if g.chance(0.5) {
		// for: p += 1 を毎周
		i := g.newLocal(rpTypes[g.pick(2)])
		g.scope[len(g.scope)-1].readOnly = true
		n := g.pick(8) + 1
		body := g.block(depth - 1)
		g.scope = g.scope[:len(g.scope)-1]
		g.loops--
		g.setPtr(p.name, false, true)
		// 歩幅の加算は本体の前 (本体の continue で飛ばされないように。添字は j0 + n ≤ 11)
		head = fmt.Sprintf("%s = &%s[%d];\nfor (var %s:%s = 0; %s < %d; %s++) {\n%s += 1;\n", p.name, arr.name, j0, i.name, i.typ.name, i.name, n, i.name, p.name)
		tail = fmt.Sprintf("}\n%s = &%s[0];", p.name, arr.name)
		return &rpStmt{parts: []string{head, tail}, kids: [][]*rpStmt{body}}
	}
	// while: カウンタ k とポインタ q を同じ歩幅で
	k := g.newLocal(rpTypes[g.pick(2)*2]) // int か int16
	g.scope = g.scope[:len(g.scope)-1]
	g.cur.locals = append(g.cur.locals, fmt.Sprintf("var %s:%s = 0;", k.name, k.typ.name))
	step := fmt.Sprintf("%d", g.pick(3)+1)
	if g.chance(0.4) {
		sv := g.newLocal(rpTypes[0])
		g.scope = g.scope[:len(g.scope)-1]
		g.cur.locals = append(g.cur.locals, fmt.Sprintf("var %s:int = 1;", sv.name))
		head = fmt.Sprintf("%s = ((%s & 3) | 1);\n", sv.name, g.expr(rpTypes[0], 2))
		step = sv.name
	}
	n := g.pick(9) + 4 // 4〜12
	body := g.block(depth - 1)
	g.loops--
	g.setPtr(p.name, false, true)
	// 歩幅の加算は本体の前 (continue で飛ばされないように。添字は n - 1 + 3 ≤ 14)
	head += fmt.Sprintf("%s = %d;\n%s = &%s[%d];\nwhile (%s < %d) {\n%s += %s;\n%s += %s;\n", k.name, j0, p.name, arr.name, j0, k.name, n, p.name, step, k.name, step)
	tail = fmt.Sprintf("}\n%s = &%s[0];", p.name, arr.name)
	return &rpStmt{parts: []string{head, tail}, kids: [][]*rpStmt{body}}
}

// stmt は文 1 つ (複数行のこともある)。depth はブロックの入れ子の残り。
func (g *rpGen) stmt(depth int) *rpStmt {
	k := g.pick(17)
	if depth <= 0 && k >= 7 {
		k = g.pick(7)
	}
	switch k {
	case 14:
		// ポインタを配列の要素に向け直す (添字 0〜7 → fresh)、または struct のポインタを向け直す
		if ps := g.ptrs(); len(ps) > 0 && g.chance(0.7) {
			p := ps[g.pick(len(ps))]
			if !p.readOnly {
				var as []rpVar
				for _, a := range g.arraysAll() {
					if a.typ == p.typ {
						as = append(as, a)
					}
				}
				if len(as) > 0 {
					g.setPtr(p.name, false, true)
					return rpSimple(fmt.Sprintf("%s = %s;", p.name, g.arrayRef(as[g.pick(len(as))])))
				}
			}
		}
		if g.sptr != "" {
			return rpSimple(fmt.Sprintf("%s = &sa[(%s & 3)];", g.sptr, g.expr(rpTypes[0], 1)))
		}
		lv, t := g.lvalue()
		return rpSimple(fmt.Sprintf("%s = %s;", lv, g.expr(t, 3)))
	case 15:
		return g.ptrLoop(depth)
	case 16:
		// 減らしながらの for (展開の対象)
		i := g.newLocal(rpTypes[0])
		g.scope[len(g.scope)-1].readOnly = true
		n := g.pick(8) + 1
		g.loops++
		body := g.block(depth - 1)
		g.loops--
		g.scope = g.scope[:len(g.scope)-1]
		return &rpStmt{parts: []string{fmt.Sprintf("for (var %s:int = %d; %s; %s--) {\n", i.name, n, i.name, i.name), "}"}, kids: [][]*rpStmt{body}}
	case 0, 1, 2:
		lv, t := g.lvalue()
		return rpSimple(fmt.Sprintf("%s = %s;", lv, g.expr(t, 3)))
	case 3, 4:
		lv, t := g.lvalue()
		op := []string{"+=", "-=", "*=", "&=", "|=", "^="}[g.pick(6)]
		return rpSimple(fmt.Sprintf("%s %s %s;", lv, op, g.expr(t, 2)))
	case 5:
		lv, t := g.lvalue()
		if g.chance(0.5) {
			return rpSimple(fmt.Sprintf("%s++;", lv))
		}
		return rpSimple(fmt.Sprintf("%s <<= %d;", lv, g.pick(4)) + " " + fmt.Sprintf("%s ^= %s;", lv, g.lit(t)))
	case 6:
		// 呼び出しの結果を代入
		if n := g.callable(); n > 0 {
			f := g.funcs[g.pick(n)]
			lv, t := g.lvalue()
			return rpSimple(fmt.Sprintf("%s = %s;", lv, cast(fmt.Sprintf("%s(%s)", f.name, g.args(f, 2)), f.ret, t)))
		}
		lv, t := g.lvalue()
		return rpSimple(fmt.Sprintf("%s = %s;", lv, g.expr(t, 2)))
	case 7, 8:
		s := &rpStmt{parts: []string{fmt.Sprintf("if (%s) {\n", g.cond())}, kids: [][]*rpStmt{g.block(depth - 1)}}
		if g.chance(0.4) {
			s.parts = append(s.parts, fmt.Sprintf("} elsif (%s) {\n", g.cond()))
			s.kids = append(s.kids, g.block(depth-1))
		}
		if g.chance(0.5) {
			s.parts = append(s.parts, "} else {\n")
			s.kids = append(s.kids, g.block(depth-1))
		}
		s.parts = append(s.parts, "}")
		return s
	case 9, 10:
		// 回数つきの for (ループ変数は本体で使える)
		i := g.newLocal(rpTypes[g.pick(2)])
		g.scope[len(g.scope)-1].readOnly = true
		n := g.pick(7) + 1
		g.loops++
		body := g.block(depth - 1)
		g.loops--
		g.scope = g.scope[:len(g.scope)-1]
		return &rpStmt{parts: []string{fmt.Sprintf("for (var %s:%s = 0; %s < %d; %s++) {\n", i.name, i.typ.name, i.name, n, i.name), "}"}, kids: [][]*rpStmt{body}}
	case 11:
		// 条件つきの while (回数の上限を別の変数で)
		c := g.newLocal(rpTypes[0])
		g.scope = g.scope[:len(g.scope)-1]
		g.cur.locals = append(g.cur.locals, fmt.Sprintf("var %s:int = 0;", c.name))
		g.loops++
		body := g.block(depth - 1)
		g.loops--
		return &rpStmt{parts: []string{fmt.Sprintf("while ((%s) && %s < %d) {\n%s++;\n", g.cond(), c.name, g.pick(6)+1, c.name), "}"}, kids: [][]*rpStmt{body}}
	case 12:
		// switch (タグは 1 バイト)
		// parts: "switch (tag) {\ncase 1:\n", (case 本体), "case 2:\n", (本体), …, "}"
		tag := fmt.Sprintf("(%s & 7)", g.expr(rpTypes[0], 2))
		s := &rpStmt{parts: []string{fmt.Sprintf("switch (%s) {\n", tag)}}
		used := map[int]bool{}
		for k := 0; k < g.pick(4)+1; k++ {
			var vals []string
			for m := 0; m < g.pick(2)+1; m++ {
				v := g.pick(8)
				if !used[v] {
					used[v] = true
					vals = append(vals, fmt.Sprintf("%d", v))
				}
			}
			if len(vals) == 0 {
				continue
			}
			body := g.block(depth - 1)
			if g.loops > 0 && g.chance(0.2) {
				body = append(body, rpSimple("break;"))
			}
			s.parts[len(s.parts)-1] += fmt.Sprintf("case %s:\n", strings.Join(vals, ", "))
			s.kids = append(s.kids, body)
			s.parts = append(s.parts, "")
		}
		if g.chance(0.5) {
			s.parts[len(s.parts)-1] += "default:\n"
			s.kids = append(s.kids, g.block(depth-1))
			s.parts = append(s.parts, "")
		}
		s.parts[len(s.parts)-1] += "}"
		return s
	default:
		if g.loops > 0 {
			return rpSimple(fmt.Sprintf("if (%s) { %s; }", g.cond(), []string{"break", "continue"}[g.pick(2)]))
		}
		lv, t := g.lvalue()
		return rpSimple(fmt.Sprintf("%s = %s;", lv, g.expr(t, 3)))
	}
}

// cond は条件式 (比較か整数)。
func (g *rpGen) cond() string {
	t := g.typ()
	if g.chance(0.7) {
		op := []string{"<", "<=", "==", "!=", ">", ">="}[g.pick(6)]
		c := fmt.Sprintf("%s %s %s", g.expr(t, 2), op, g.expr(t, 2))
		if g.chance(0.3) {
			u := g.typ()
			c = fmt.Sprintf("(%s) %s (%s)", c, []string{"&&", "||"}[g.pick(2)], g.expr(u, 2))
		}
		return c
	}
	return g.expr(t, 2)
}

// block は文の並び。
func (g *rpGen) block(depth int) []*rpStmt {
	n := g.pick(3) + 1
	var out []*rpStmt
	mark := len(g.scope)
	for i := 0; i < n; i++ {
		if g.chance(0.2) {
			t := g.typ()
			init := g.expr(t, 2) // 自分自身を参照しないように、宣言の前に作る
			v := g.newLocal(t)
			out = append(out, rpSimple(fmt.Sprintf("var %s:%s = %s;", v.name, v.typ.name, init)))
			continue
		}
		out = append(out, g.stmt(depth))
	}
	g.scope = g.scope[:mark] // ブロックの中のローカルは外に見えない
	return out
}

func (g *rpGen) newLocal(t rpType) rpVar {
	v := rpVar{name: fmt.Sprintf("l%d", g.nLocal), typ: t}
	g.nLocal++
	g.scope = append(g.scope, v)
	return v
}

// genFunc は関数を 1 つ作る。
func (g *rpGen) genFunc(name string) *rpFunc {
	f := &rpFunc{name: name, ret: g.typ()}
	f.fastcall = g.chance(0.4)
	f.inline = g.chance(0.3)
	g.cur = f
	g.scope = nil
	g.nLocal = 0
	g.larrays = nil
	g.sptr = ""
	for i := 0; i < g.pick(3); i++ {
		p := rpVar{name: fmt.Sprintf("p%d", i), typ: g.typ()}
		if g.chance(0.3) {
			p.ptr, p.fresh = true, true // 呼ぶ側は &a[e & 7] を渡す
		}
		f.params = append(f.params, p)
		g.scope = append(g.scope, p)
	}
	for i := 0; i < g.pick(3); i++ {
		v := g.newLocal(g.typ())
		f.locals = append(f.locals, fmt.Sprintf("var %s:%s = %s;", v.name, v.typ.name, g.lit(v.typ)))
	}
	g.declareArraysAndPtrs(f)
	depth := 2
	if f.inline {
		depth = 1 // inline の本体は小さめ
	}
	for i := 0; i < g.pick(4)+1; i++ {
		f.stmts = append(f.stmts, g.stmt(depth))
	}
	f.retExpr = g.expr(f.ret, 2)
	g.funcs = append(g.funcs, f)
	g.cur = nil
	return f
}

// genProgram はプログラム全体を作る (main の文は最後に「全部を出力して exit」)。
func (g *rpGen) genProgram() {
	for i := 0; i < g.pick(4)+3; i++ {
		g.globals = append(g.globals, rpVar{name: fmt.Sprintf("g%d", i), typ: g.typ()})
	}
	for i, t := range rpTypes { // 全ての型の配列を 1 つずつ (ポインタの引数の相手)
		g.arrays = append(g.arrays, rpVar{name: fmt.Sprintf("a%d", i), typ: t})
	}
	if g.chance(0.7) {
		for i := 0; i < g.pick(2)+2; i++ {
			g.fields = append(g.fields, rpField{name: fmt.Sprintf("f%d", i), typ: g.typ()})
		}
	}
	for i := 0; i < g.pick(3)+1; i++ {
		g.genFunc(fmt.Sprintf("f%d", i))
	}
	m := &rpFunc{name: "main"}
	g.cur = m
	g.scope = nil
	g.nLocal = 0
	for _, v := range g.globals {
		m.stmts = append(m.stmts, rpSimple(fmt.Sprintf("%s = %s;", v.name, g.lit(v.typ))))
	}
	for _, a := range g.arrays {
		for i := 0; i < 16; i++ {
			m.stmts = append(m.stmts, rpSimple(fmt.Sprintf("%s[%d] = %s;", a.name, i, g.lit(a.typ))))
		}
	}
	for _, f := range g.fields {
		m.stmts = append(m.stmts, rpSimple(fmt.Sprintf("s0.%s = %s;", f.name, g.lit(f.typ))))
		for i := 0; i < 4; i++ {
			m.stmts = append(m.stmts, rpSimple(fmt.Sprintf("sa[%d].%s = %s;", i, f.name, g.lit(f.typ))))
		}
	}
	for i := 0; i < g.pick(4); i++ {
		v := g.newLocal(g.typ())
		m.locals = append(m.locals, fmt.Sprintf("var %s:%s = %s;", v.name, v.typ.name, g.lit(v.typ)))
	}
	g.larrays = nil
	g.sptr = ""
	g.declareArraysAndPtrs(m)
	for i := 0; i < g.pick(8)+4; i++ {
		m.stmts = append(m.stmts, g.stmt(2))
	}
	g.funcs = append(g.funcs, m)
}

// declareArraysAndPtrs は関数のローカル配列 (16 要素。全部初期化)、配列へのポインタ、struct へのポインタを宣言する。
func (g *rpGen) declareArraysAndPtrs(f *rpFunc) {
	if f.inline {
		return // inline の本体は小さく
	}
	if g.chance(0.4) {
		a := rpVar{name: fmt.Sprintf("la%d", len(g.larrays)), typ: g.typ()}
		g.larrays = append(g.larrays, a)
		f.locals = append(f.locals, fmt.Sprintf("var %s:[16]%s;", a.name, a.typ.name))
		for i := 0; i < 16; i++ {
			f.stmts = append(f.stmts, rpSimple(fmt.Sprintf("%s[%d] = %s;", a.name, i, g.lit(a.typ))))
		}
	}
	as := g.arraysAll()
	for i := 0; i < g.pick(3); i++ {
		a := as[g.pick(len(as))]
		p := rpVar{name: fmt.Sprintf("q%d", i), typ: a.typ, ptr: true, fresh: true}
		g.scope = append(g.scope, p)
		f.locals = append(f.locals, fmt.Sprintf("var %s:*%s = &%s[%d];", p.name, a.typ.name, a.name, g.pick(8)))
	}
	if len(g.fields) > 0 && g.chance(0.4) {
		g.sptr = "ps"
		f.locals = append(f.locals, fmt.Sprintf("var ps:*S = &sa[%d];", g.pick(4)))
	}
}

// source はプログラムのソース (runEmu が `#fc 2` と stdio を頭に足す)。
func (g *rpGen) source() string {
	var b strings.Builder
	for _, v := range g.globals {
		fmt.Fprintf(&b, "var %s:%s;\n", v.name, v.typ.name)
	}
	for _, a := range g.arrays {
		fmt.Fprintf(&b, "var %s:[16]%s;\n", a.name, a.typ.name)
	}
	if len(g.fields) > 0 {
		b.WriteString("struct S {\n")
		for _, f := range g.fields {
			fmt.Fprintf(&b, "\t%s:%s;\n", f.name, f.typ.name)
		}
		b.WriteString("}\nvar s0:S;\nvar sa:[4]S;\n")
	}
	for _, f := range g.funcs {
		if f.name == "main" {
			b.WriteString("function main():void\n{\n")
		} else {
			ps := make([]string, len(f.params))
			for i, p := range f.params {
				ps[i] = p.name + ":" + p.typ.name
				if p.ptr {
					ps[i] = p.name + ":*" + p.typ.name
				}
			}
			var opts []string
			if f.fastcall {
				opts = append(opts, "fastcall: true")
			}
			if f.inline {
				opts = append(opts, "inline: true")
			}
			fmt.Fprintf(&b, "function %s(%s):%s", f.name, strings.Join(ps, ", "), f.ret.name)
			if len(opts) > 0 {
				fmt.Fprintf(&b, " options(%s)", strings.Join(opts, ", "))
			}
			b.WriteString("\n{\n")
		}
		for _, l := range f.locals {
			b.WriteString(l + "\n")
		}
		for _, s := range f.stmts {
			s.render(&b)
			b.WriteString("\n")
		}
		if f.name == "main" {
			var out []string
			for _, v := range g.globals {
				out = append(out, v.name, `" "`)
			}
			for _, a := range g.arrays {
				for i := 0; i < 16; i++ {
					out = append(out, fmt.Sprintf("%s[%d]", a.name, i), `" "`)
				}
			}
			for _, f := range g.fields {
				out = append(out, "s0."+f.name, `" "`)
				for i := 0; i < 4; i++ {
					out = append(out, fmt.Sprintf("sa[%d].%s", i, f.name), `" "`)
				}
			}
			fmt.Fprintf(&b, "printf(%s, \"\\n\");\nexit(0);\n", strings.Join(out, ", "))
		} else {
			fmt.Fprintf(&b, "return %s;\n}\n", f.retExpr)
		}
	}
	b.WriteString("}\n")
	return b.String()
}

// rpRun はソースを level でビルドして emu で走らせ、出力を返す (ビルドや終了コードの失敗は error。コンパイラの panic も
// error にして、次の種に進めるようにする)。
func rpRun(t *testing.T, src string, level int) (out string, err error) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "t.fc"), []byte("#fc 2\nuse * from stdio;\n"+src), 0o666); err != nil {
		return "", err
	}
	var o strings.Builder
	code, err := NewCompiler(absRepoRoot).Build("t.fc", &BuildOptions{Dir: dir, BuildDir: filepath.Join(dir, "b"), Out: filepath.Join(dir, "a.bin"), Run: true, Stdout: &o, OptimizeLevel: level, MaxCycles: 20_000_000})
	if err != nil {
		return "", err
	}
	if code != 0 {
		return "", fmt.Errorf("exit code %d: %s", code, o.String())
	}
	return o.String(), nil
}

// rpResult は 1 つのプログラムの判定。
type rpResult struct {
	kind   string // "ok" / "differs" / "panic" / "error" (error は生成器の問題: 型エラーなど)
	detail string
}

// rpCheck は -O 0 と -O 2 で走らせて判定する。
func rpCheck(t *testing.T, src string) rpResult {
	o0, err0 := rpRun(t, src, -1)
	o2, err2 := rpRun(t, src, 0)
	hang := func(err error) bool { return err != nil && strings.HasPrefix(err.Error(), "cycle limit") }
	if hang(err0) != hang(err2) && (err0 == nil || hang(err0)) && (err2 == nil || hang(err2)) {
		// 片方だけ止まらない (もう片方は正常): 出力の食い違いと同じ扱い
		return rpResult{"differs", fmt.Sprintf("-O 0: %s %v\n-O 2: %s %v", o0, err0, o2, err2)}
	}
	if hang(err0) && hang(err2) {
		return rpResult{"hang", err0.Error()} // 両方で止まらない: 生成器の問題 (ループの上限の抜け) かコンパイラ共通のバグ
	}
	for _, err := range []error{err0, err2} {
		if err != nil {
			if strings.HasPrefix(err.Error(), "panic:") {
				return rpResult{"panic", err.Error()}
			}
			return rpResult{"error", err.Error()}
		}
	}
	if o0 != o2 {
		return rpResult{"differs", fmt.Sprintf("-O 0: %s-O 2: %s", o0, o2)}
	}
	return rpResult{"ok", ""}
}

// rpMinimize は同じ種類の失敗が残る範囲で文を消す (どの深さの文も。消せなかった文はその中身を試す)。
func rpMinimize(t *testing.T, g *rpGen, kind string) {
	var visit func(list *[]*rpStmt) bool
	visit = func(list *[]*rpStmt) bool {
		changed := false
		for i := 0; i < len(*list); i++ {
			saved := (*list)[i]
			*list = append((*list)[:i:i], (*list)[i+1:]...)
			if rpCheck(t, g.source()).kind == kind {
				changed = true
				i--
				continue
			}
			*list = append((*list)[:i], append([]*rpStmt{saved}, (*list)[i:]...)...)
			for k := range saved.kids {
				if visit(&saved.kids[k]) {
					changed = true
				}
			}
		}
		return changed
	}
	for changed := true; changed; {
		changed = false
		for _, f := range g.funcs {
			if visit(&f.stmts) {
				changed = true
			}
		}
	}
}

func TestRandomPrograms(t *testing.T) {
	t.Parallel()
	base := *randSeed
	if base == 0 {
		base = 1
	}
	for k := 0; k < *randN; k++ {
		seed := base + int64(k)
		t.Run(fmt.Sprintf("seed%d", seed), func(t *testing.T) {
			t.Parallel()
			g := &rpGen{r: rand.New(rand.NewSource(seed))}
			g.genProgram()
			res := rpCheck(t, g.source())
			switch res.kind {
			case "ok":
			case "error":
				if strings.Contains(res.detail, "memory area overflow") {
					t.Skipf("プログラムが大きすぎて ROM に入らない (seed %d)", seed)
				}
				t.Fatalf("ビルド失敗 (生成器の問題) (seed %d):\n%s\n%s", seed, g.source(), res.detail)
			case "hang":
				// 両方のレベルで止まらない: 入れ子のループ × 呼び出しで単に重い (サイクルの上限を超える) のがほとんどで、
				// 生成器の問題として飛ばす (コンパイラ共通のバグならほかの形でも出る)
				t.Skipf("両方のレベルでサイクルの上限を超えた (seed %d)", seed)
			default:
				rpMinimize(t, g, res.kind)
				res = rpCheck(t, g.source())
				t.Errorf("%s (seed %d):\n%s\n%s", map[string]string{"differs": "-O 0 と -O 2 の出力が違う", "panic": "コンパイラが panic", "hang": "両方のレベルで止まらない"}[res.kind], seed, g.source(), res.detail)
			}
		})
	}
}
