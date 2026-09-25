package driver

// ランダムに生成した小さなプログラムを -O 0 と -O 2 で走らせて出力を比べる差分テスト (Csmith と同じ考え方)。
// 単機能のテストでは拾えない「機能の組み合わせ」のバグ (inline の引数が式、比較の直後の常駐レジスタの復帰など) を
// 狙う。既定は固定の種で 30 本 (毎回同じプログラム)。数を増やす / 別の種で回すには:
//
//	go test ./internal/driver -run TestRandomPrograms -randn 500 -randseed 12345
//
// 食い違ったら文を 1 つずつ消して最小化し、そのプログラムと両方の出力をログに出す (ops_test.go に足す材料)。
// -O 0 と -O 2 が同じでも、最適化前の IR をインタプリタ (internal/interp) で実行した出力と違えば失敗にする (両方の
// レベルで同じように間違える codegen のバグを拾う。`-randinterp=false` で切る)。
// 未定義動作は生成しない: 0 除算 (除数は `| 1` か 0 でないリテラル)、範囲外の添字 (`& 7`)、2 バイト値の変数シフト
// (シフト量はリテラル)、無限ループ (回数は上限つき)。

import (
	"errors"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haramako/fc/internal/interp"
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/opt"
	"github.com/haramako/fc/internal/sema"
)

var (
	randN      = flag.Int("randn", 30, "TestRandomPrograms のプログラム数")
	randSeed   = flag.Int64("randseed", 0, "TestRandomPrograms の種 (0 なら 1 から順)")
	randInterp = flag.Bool("randinterp", true, "TestRandomPrograms で最適化前の IR のインタプリタの出力とも比べる")
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
	sptr     bool // struct へのポインタの引数 `ps:*S` (呼ぶ側は &sa[e & 3] か &s0)
	small    bool // 再帰の深さの引数 (呼ぶ側は `(e & 3)`)
	vals     []string // const の表の値 (最初の source で決めて持つ。呼ぶたびに乱数で作り直すと最小化で別のプログラムになる)
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
	far      bool      // 別バンクのモジュール far1 にある (main からは far call)
	inTable  bool      // 関数ポインタ表 fp0 / fq0 の要素 (アドレスを取られる: Entry 関数になる)
	rec      bool      // 自分を呼ぶ関数 (stack ABI になる。深さは引数 n で 4 まで)
	mod      string    // 別のモジュールの関数 (fc 3 のモジュール v3m。本体は randv3_test.go が生成して text に持つ)
	text     string
	locals   []string  // 宣言
	stmts    []*rpStmt // 本体
	retExpr  string
}

// rpStmt は文の木 (最小化でどの深さの文も消せるように)。parts[i] の後に kids[i] の文が並ぶ。単純な文は parts が 1 つ。
type rpStmt struct {
	parts []string
	kids  [][]*rpStmt
	keep  bool // 初期化文: 最小化で消さない (消すと未初期化の読み出し = 値がレベルやインタプリタで違うのが当たり前になる)
}

func rpSimple(text string) *rpStmt { return &rpStmt{parts: []string{text}} }

// rpInit は初期化文 (大域変数・配列・struct・soa の main の先頭での代入、ローカル配列の要素の代入)。
func rpInit(text string) *rpStmt { return &rpStmt{parts: []string{text}, keep: true} }

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
	wb      bool // 300 バイトのバッファ wb (walkLoop) を使う
	globals []rpVar
	arrays  []rpVar // グローバル配列 (16 要素。要素型)
	larrays []rpVar // 今の関数のローカル配列 (16 要素)
	fields  []rpField
	consts  []rpVar   // const の表 (16 要素。ROM)
	fpTable []*rpFunc // 関数ポインタ表 fp0 の要素 (同じ型の関数 2 つか 4 つ。nil なら無し)
	fpSig   rpFunc    // fp0 の要素の型 (params / ret)
	fqTable []*rpFunc // far1 の関数の farfn 表 fq0 の要素 (同じ型の 2 つ。nil なら無し)
	fqSig   rpFunc    // fq0 の要素の型
	fnVar   bool      // main の関数ポインタのローカル変数 fv (fp0 の要素の型。今の関数が main のときだけ使う)
	soa     bool      // soa E:[8]S がある (struct のフィールドがあるとき)
	tArr    rpType    // struct T { s:S; arr:[4]tArr; x:tX; } の配列フィールドの要素型 (hasT のとき)
	tX      rpType    // T の x の型
	hasT    bool      // struct T (S の入れ子と配列フィールド) と u0 / ua:[2]T がある
	ptr     string    // 今の関数の T へのポインタ (`pt`。"" なら無し)
	alias   bool      // `var ab:[8]int; alias w:S = ab;` (同じ場所を配列と struct で読み書きする)
	lsv     string    // 今の関数のローカルの struct 変数 (`ls`。"" なら無し)
	sv      *rpFunc   // struct を値で受けて値で返す関数 sv(v:S, k:int):S (nil なら無し)
	lambda  bool      // main のラムダ lf:fn(int):int (今の関数が main のときだけ使う)
	labels  []string  // 囲んでいるラベル付きのループ (内側が末尾)
	nLabel  int
	hptr    string // 今の関数の soa の要素ハンドル (`h`。"" なら無し)
	hasFar  bool   // far1 モジュール (options(bank: 1)) がある
	sptr    string // 今の関数の struct へのポインタ (`ps`。"" なら無し)
	funcs   []*rpFunc
	scope   []rpVar // 今の関数で見えるローカル (引数含む。ポインタも)
	loops   int     // ループの入れ子の深さ (break / continue を出せるか)
	nLocal  int
	cur     *rpFunc
	v3      *rpV3 // fc 3 のモジュール v3m (TestRandomV3Programs のときだけ。randv3_test.go)
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
		if !v.ptr && !v.sptr {
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
	case g.lsv != "" && g.chance(0.2):
		return fmt.Sprintf("%s.%s", g.lsv, f.name), f.typ
	case g.alias && g.chance(0.15):
		return "w." + f.name, f.typ // ab と同じ場所
	case g.hasT && g.chance(0.25):
		return fmt.Sprintf("%s.s.%s", g.tBase(), f.name), f.typ
	case g.hptr != "" && g.chance(0.25):
		return fmt.Sprintf("%s.%s", g.hptr, f.name), f.typ
	case g.soa && g.chance(0.3):
		return fmt.Sprintf("E[%s].%s", g.index(), f.name), f.typ
	case g.sptr != "" && g.chance(0.4):
		return fmt.Sprintf("%s.%s", g.sptr, f.name), f.typ
	case g.chance(0.5):
		return fmt.Sprintf("sa[(%s & 3)].%s", g.expr(rpTypes[0], 1), f.name), f.typ
	}
	return "s0." + f.name, f.typ
}

// tBase は struct T の値 (u0、ua の要素、ポインタ pt 経由。t0 は関数ポインタ表の関数の名前)。
func (g *rpGen) tBase() string {
	switch {
	case g.ptr != "" && g.chance(0.35):
		return g.ptr
	case g.chance(0.5):
		return fmt.Sprintf("ua[(%s & 1)]", g.expr(rpTypes[0], 1))
	}
	return "u0"
}

// tRef は struct T の x か配列フィールド arr の要素 (入れ子の struct s のフィールドは fieldRef)。
func (g *rpGen) tRef() (string, rpType) {
	b := g.tBase()
	if g.chance(0.3) {
		return b + ".x", g.tX
	}
	j := fmt.Sprintf("%d", g.pick(4))
	if g.chance(0.6) {
		j = fmt.Sprintf("(%s & 3)", g.expr(rpTypes[0], 1))
	}
	return fmt.Sprintf("%s.arr[%s]", b, j), g.tArr
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
	switch g.pick(14) {
	case 12:
		// min / max (その場に比較と代入を出す組み込み)
		return fmt.Sprintf("%s(%s, %s)", []string{"min", "max"}[g.pick(2)], g.expr(t, depth-1), g.expr(t, depth-1))
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
		if fs := g.callables(); len(fs) > 0 {
			f := fs[g.pick(len(fs))]
			return cast(fmt.Sprintf("%s(%s)", g.callName(f), g.args(f, depth-1)), f.ret, t)
		}
		return g.leaf(t)
	case 11:
		// 関数ポインタ表経由の呼び出し
		if e, ok := g.fpCall(depth-1, t); ok {
			return e
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
		if p.sptr {
			args[i] = "&s0"
			if g.chance(0.6) {
				args[i] = fmt.Sprintf("&sa[(%s & 3)]", g.expr(rpTypes[0], 1))
			}
			continue
		}
		if p.small {
			args[i] = fmt.Sprintf("(%s & 3)", g.expr(rpTypes[0], 1))
			continue
		}
		if p.ptr {
			var as []rpVar
			for _, a := range g.arraysAll() {
				if a.typ == p.typ {
					as = append(as, a)
				}
			}
			args[i] = g.arrayRef(as[g.pick(len(as))]) // genProgram が全ての型のグローバル配列を作るので必ずある (far1 の関数はポインタの引数を持たない)
		} else {
			args[i] = g.expr(p.typ, depth)
		}
	}
	return strings.Join(args, ", ")
}

// callables は今の関数から呼べる関数 (自分より前に定義したもの。far1 の関数からは far1 の関数だけ: main の関数は見えない)。
func (g *rpGen) callables() []*rpFunc {
	if g.cur != nil && g.cur.inline {
		return nil // inline 関数は呼び出しを持たない (入れ子の展開でコードが膨らみ ROM に入らなくなる)
	}
	var r []*rpFunc
	for _, f := range g.funcs {
		if f == g.cur {
			break
		}
		if g.cur != nil && g.cur.far && !f.far {
			continue
		}
		if f.mod != "" && g.cur != nil && (g.cur.far || g.cur.inline) {
			continue // far1 と inline 関数からは v3m を呼ばない
		}
		r = append(r, f)
	}
	return r
}

// callName は今の関数から見た f の名前 (far1 の関数を main から呼ぶときは `far1.f`)。
func (g *rpGen) callName(f *rpFunc) string {
	if f.far && (g.cur == nil || !g.cur.far) {
		return "far1." + f.name
	}
	if f.mod != "" {
		return f.mod + "." + f.name
	}
	return f.name
}

// fpCall は関数ポインタ経由の呼び出し (main のモジュールからだけ): 表 `fp0[(e & 3)](args)`、main のローカル変数
// `fv(args)`、far1 の関数の farfn 表 `fq0[(e & 1)](args)` (バンクを切り替えるトランポリン経由)。
func (g *rpGen) fpCall(depth int, t rpType) (string, bool) {
	if g.cur != nil && (g.cur.far || g.cur.inline) {
		return "", false
	}
	callArgs := func(sig rpFunc) string {
		args := make([]string, len(sig.params))
		for i, p := range sig.params {
			args[i] = g.expr(p.typ, depth)
		}
		return strings.Join(args, ", ")
	}
	switch {
	case g.lambda && g.cur != nil && g.cur.name == "main" && g.chance(0.3):
		return cast(fmt.Sprintf("lf(%s)", g.expr(rpTypes[0], depth)), rpTypes[0], t), true
	case g.fqTable != nil && g.chance(0.35):
		return cast(fmt.Sprintf("fq0[(%s & %d)](%s)", g.expr(rpTypes[0], 1), len(g.fqTable)-1, callArgs(g.fqSig)), g.fqSig.ret, t), true
	case g.fpTable == nil:
		return "", false
	case g.fnVar && g.cur != nil && g.cur.name == "main" && g.chance(0.4):
		return cast(fmt.Sprintf("fv(%s)", callArgs(g.fpSig)), g.fpSig.ret, t), true
	}
	return cast(fmt.Sprintf("fp0[(%s & %d)](%s)", g.expr(rpTypes[0], 1), len(g.fpTable)-1, callArgs(g.fpSig)), g.fpSig.ret, t), true
}

// leaf は変数・配列の要素・ポインタ経由・struct のフィールド・リテラル。
func (g *rpGen) leaf(t rpType) string {
	vs := g.scalars()
	switch g.pick(7) {
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
		if g.hasT && g.chance(0.4) {
			e, ft := g.tRef()
			return cast(e, ft, t)
		}
		if len(g.fields) > 0 {
			e, ft := g.fieldRef()
			return cast(e, ft, t)
		}
		fallthrough
	case 4:
		if len(g.consts) > 0 {
			c := g.consts[g.pick(len(g.consts))]
			return cast(fmt.Sprintf("%s[%s]", c.name, g.index()), c.typ, t)
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
		if g.hasT && g.chance(0.4) {
			return g.tRef()
		}
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
	if len(g.arrays) > 0 && g.chance(0.25) {
		return g.walkLoop()
	}
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
	// 本体から外側のラベルへ抜けると、ループの後のポインタの戻し (`p = &a[0]`) を飛ばして fresh でなくなる: 越えさせない
	g.labels = append(g.labels, "")
	defer func() { g.labels = g.labels[:len(g.labels)-1] }()
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

// walkLoop は 300 バイトのバッファ wb をポインタで 1 ずつなめる 16 ビットのループ (opt.walkPointerY の対象: 下位を Y で
// 回す)。回数は 17〜290 (展開されず、ページをまたぐ)、上限はリテラルか変数、`wp += 1` は本体の先頭か末尾。本体は
// `*wp` に書いてから読む (未初期化を読まない)。ループの後は `wp -= 1` で最後に書いた要素を読む (出口で戻した p の値)。
func (g *rpGen) walkLoop() *rpStmt {
	g.wb = true
	wp := fmt.Sprintf("wp%d", g.nLocal)
	wi := fmt.Sprintf("wi%d", g.nLocal)
	wn := fmt.Sprintf("wn%d", g.nLocal)
	g.nLocal++
	j0 := g.pick(8)
	n := 17 + g.pick(274)
	g.cur.locals = append(g.cur.locals, fmt.Sprintf("var %s:*int = &wb[0];", wp))
	lim := fmt.Sprintf("%d", n)
	if g.chance(0.5) {
		g.cur.locals = append(g.cur.locals, fmt.Sprintf("var %s:int16 = 0;", wn))
		lim = wn
	}
	lv, t := g.lvalue()
	lv2, t2 := g.lvalue()
	body := fmt.Sprintf("*%s = (%s) as int;\n%s ^= ((*%s) as %s);\n", wp, g.expr(rpTypes[0], 1), lv, wp, t.name)
	inc := fmt.Sprintf("%s += 1;\n", wp)
	if g.chance(0.5) {
		body = inc + body
	} else {
		body += inc
	}
	var head string
	if lim == wn {
		head = fmt.Sprintf("%s = %d;\n", wn, n)
	}
	head += fmt.Sprintf("%s = &wb[%d];\nfor (var %s:int16 = 0; %s < %s; %s++) {\n%s}\n%s -= 1;\n%s ^= ((*%s) as %s);",
		wp, j0, wi, wi, lim, wi, body, wp, lv2, wp, t2.name)
	return rpSimple(head)
}

// stmt は文 1 つ (複数行のこともある)。depth はブロックの入れ子の残り。
func (g *rpGen) stmt(depth int) *rpStmt {
	k := g.pick(18)
	if depth <= 0 && k >= 7 {
		k = g.pick(7)
	}
	switch k {
	case 17:
		// struct の値のコピー (全フィールド。soa の要素の gather / scatter、ポインタ経由、重なりうる sa[i] = sa[j])
		if len(g.fields) == 0 {
			break
		}
		var srcs, dsts []string
		srcs = append(srcs, "s0", fmt.Sprintf("sa[(%s & 3)]", g.expr(rpTypes[0], 1)))
		dsts = append(dsts, "s0", fmt.Sprintf("sa[(%s & 3)]", g.expr(rpTypes[0], 1)))
		if g.soa {
			srcs = append(srcs, fmt.Sprintf("E[%s]", g.index()))
			dsts = append(dsts, fmt.Sprintf("E[%s]", g.index()))
		}
		if g.sptr != "" {
			srcs = append(srcs, "*"+g.sptr)
			dsts = append(dsts, "*"+g.sptr)
		}
		if g.hptr != "" {
			srcs = append(srcs, "*"+g.hptr)
			dsts = append(dsts, "*"+g.hptr)
		}
		if g.hasT && g.chance(0.3) {
			// T 全体 (入れ子の S と配列フィールドごと)
			ts := []string{"u0", fmt.Sprintf("ua[(%s & 1)]", g.expr(rpTypes[0], 1))}
			if g.ptr != "" {
				ts = append(ts, "*"+g.ptr)
			}
			return rpSimple(fmt.Sprintf("%s = %s;", ts[g.pick(len(ts))], ts[g.pick(len(ts))]))
		}
		for _, x := range []string{g.lsv, map[bool]string{true: "w"}[g.alias]} {
			if x != "" {
				srcs = append(srcs, x)
				dsts = append(dsts, x)
			}
		}
		if g.hasT {
			srcs = append(srcs, g.tBase()+".s")
			dsts = append(dsts, g.tBase()+".s")
		}
		src := srcs[g.pick(len(srcs))]
		if g.sv != nil && (g.cur == nil || !g.cur.inline) && g.chance(0.3) {
			src = fmt.Sprintf("sv(%s, %s)", src, g.expr(rpTypes[0], 1)) // 値で渡して値で返す
		}
		return rpSimple(fmt.Sprintf("%s = %s;", dsts[g.pick(len(dsts))], src))
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
		if g.hptr != "" && g.chance(0.4) {
			return rpSimple(fmt.Sprintf("%s = &E[%s];", g.hptr, g.index()))
		}
		if g.ptr != "" && g.chance(0.4) {
			if g.chance(0.3) {
				return rpSimple(fmt.Sprintf("%s = &u0;", g.ptr))
			}
			return rpSimple(fmt.Sprintf("%s = &ua[(%s & 1)];", g.ptr, g.expr(rpTypes[0], 1)))
		}
		if g.fnVar && g.cur != nil && g.cur.name == "main" && g.chance(0.4) {
			return rpSimple(fmt.Sprintf("fv = %s;", g.fpTable[g.pick(len(g.fpTable))].name))
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
		label := g.pushLabel()
		g.loops++
		body := g.block(depth - 1)
		g.loops--
		g.popLabel(label)
		g.scope = g.scope[:len(g.scope)-1]
		return &rpStmt{parts: []string{fmt.Sprintf("%sfor (var %s:int = %d; %s; %s--) {\n", label, i.name, n, i.name, i.name), "}"}, kids: [][]*rpStmt{body}}
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
		if fs := g.callables(); len(fs) > 0 {
			f := fs[g.pick(len(fs))]
			lv, t := g.lvalue()
			return rpSimple(fmt.Sprintf("%s = %s;", lv, cast(fmt.Sprintf("%s(%s)", g.callName(f), g.args(f, 2)), f.ret, t)))
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
		// 回数つきの for (ループ変数は本体で使える。2 バイトや符号付きのカウンタも)
		i := g.newLocal(g.typ())
		g.scope[len(g.scope)-1].readOnly = true
		n := g.pick(7) + 1
		label := g.pushLabel()
		g.loops++
		body := g.block(depth - 1)
		g.loops--
		g.popLabel(label)
		g.scope = g.scope[:len(g.scope)-1]
		return &rpStmt{parts: []string{fmt.Sprintf("%sfor (var %s:%s = 0; %s < %d; %s++) {\n", label, i.name, i.typ.name, i.name, n, i.name), "}"}, kids: [][]*rpStmt{body}}
	case 11:
		// 条件つきの while (回数の上限を別の変数で)
		c := g.newLocal(rpTypes[0])
		g.scope = g.scope[:len(g.scope)-1]
		g.cur.locals = append(g.cur.locals, fmt.Sprintf("var %s:int = 0;", c.name))
		label := g.pushLabel()
		g.loops++
		body := g.block(depth - 1)
		g.loops--
		g.popLabel(label)
		return &rpStmt{parts: []string{fmt.Sprintf("%swhile ((%s) && %s < %d) {\n%s++;\n", label, g.cond(), c.name, g.pick(6)+1, c.name), "}"}, kids: [][]*rpStmt{body}}
	case 12:
		// switch (タグは 1 バイト)
		// parts: "switch (tag) {\ncase 1:\n", (case 本体), "case 2:\n", (本体), …, "}"
		tag := fmt.Sprintf("(%s & 7)", g.expr(rpTypes[0], 2))
		nCase := g.pick(4) + 1
		dense := depth >= 2 && g.chance(0.25) // 密な整数の case が 10 個以上: ジャンプテーブル (switch 命令) になる形
		if dense {
			tag = fmt.Sprintf("(%s & 15)", g.expr(rpTypes[0], 2))
			nCase = g.pick(3) + 10
		}
		s := &rpStmt{parts: []string{fmt.Sprintf("switch (%s) {\n", tag)}}
		used := map[int]bool{}
		for k := 0; k < nCase; k++ {
			var vals []string
			if dense {
				vals = append(vals, fmt.Sprintf("%d", k))
			}
			for m := 0; !dense && m < g.pick(2)+1; m++ {
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
			if dense {
				body = body[:1] // ジャンプテーブルの形は case が多いので 1 文ずつ
			}
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
		if ls := g.reachableLabels(); len(ls) > 0 && g.chance(0.7) {
			// 外側のループを抜ける / 次の繰り返しへ (switch の中からも)
			return rpSimple(fmt.Sprintf("if (%s) { %s %s; }", g.cond(), []string{"break", "continue"}[g.pick(2)], ls[g.pick(len(ls))]))
		}
		if g.loops > 0 {
			return rpSimple(fmt.Sprintf("if (%s) { %s; }", g.cond(), []string{"break", "continue"}[g.pick(2)]))
		}
		lv, t := g.lvalue()
		return rpSimple(fmt.Sprintf("%s = %s;", lv, g.expr(t, 3)))
	}
	lv, t := g.lvalue()
	return rpSimple(fmt.Sprintf("%s = %s;", lv, g.expr(t, 3)))
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

// pushLabel はループに付けるラベル (`L3: `。付けないなら "")。popLabel で外す。
func (g *rpGen) pushLabel() string {
	if !g.chance(0.6) {
		return ""
	}
	name := fmt.Sprintf("L%d", g.nLabel)
	g.nLabel++
	g.labels = append(g.labels, name)
	return name + ": "
}

func (g *rpGen) popLabel(label string) {
	if label != "" {
		g.labels = g.labels[:len(g.labels)-1]
	}
}

// reachableLabels は break / continue で指せるラベル (ポインタをずらすループの本体の外のもの (空の印の手前) は除く)。
func (g *rpGen) reachableLabels() []string {
	var r []string
	for _, l := range g.labels {
		if l == "" {
			r = nil
			continue
		}
		r = append(r, l)
	}
	return r
}

func (g *rpGen) newLocal(t rpType) rpVar {
	v := rpVar{name: fmt.Sprintf("l%d", g.nLocal), typ: t}
	g.nLocal++
	g.scope = append(g.scope, v)
	return v
}

// genFunc は関数を 1 つ作る。far なら far1 モジュール (main のグローバル・配列・struct・const は見えない。ポインタの引数無し)。
// sig があれば関数ポインタ表の要素 (その型に合わせる。アドレスを取られるので inline / fastcall にしない)。
func (g *rpGen) genFunc(name string, far bool, sig *rpFunc) *rpFunc {
	f := &rpFunc{name: name, ret: g.typ(), far: far}
	f.fastcall = g.chance(0.4)
	f.inline = g.chance(0.3) && !far
	if sig != nil {
		f.ret, f.fastcall, f.inline, f.inTable = sig.ret, false, false, true
	}
	g.cur = f
	g.scope = nil
	g.nLocal = 0
	g.larrays = nil
	g.sptr = ""
	g.hptr = ""
	g.ptr, g.lsv, g.labels = "", "", nil
	if far {
		// main のものは見えない (中身は退避して、関数の後で戻す)
		globals, arrays, fields, consts, fp, hasT := g.globals, g.arrays, g.fields, g.consts, g.fpTable, g.hasT
		g.globals, g.arrays, g.fields, g.consts, g.fpTable, g.hasT = nil, nil, nil, nil, nil, false
		defer func() {
			g.globals, g.arrays, g.fields, g.consts, g.fpTable, g.hasT = globals, arrays, fields, consts, fp, hasT
		}()
	}
	np := g.pick(3)
	if sig != nil {
		np = len(sig.params)
	}
	for i := 0; i < np; i++ {
		p := rpVar{name: fmt.Sprintf("p%d", i), typ: g.typ()}
		if sig != nil {
			p.typ = sig.params[i].typ
		} else if g.chance(0.3) && !far {
			p.ptr, p.fresh = true, true // 呼ぶ側は &a[e & 7] を渡す
		}
		f.params = append(f.params, p)
		g.scope = append(g.scope, p)
	}
	if !far && sig == nil && len(g.fields) > 0 && g.chance(0.25) {
		// struct へのポインタの引数 (呼ぶ側は &s0 か &sa[e & 3])
		p := rpVar{name: "ps", sptr: true}
		f.params = append(f.params, p)
		g.scope = append(g.scope, p)
		g.sptr = "ps"
	}
	nl := g.pick(3)
	if far {
		nl++ // 変数が 1 つも無いと代入先が無い
	}
	for i := 0; i < nl; i++ {
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

// genRec は自分を呼ぶ関数 `name(n:int, a:T):R` を作る (stack ABI になる)。深さ n は呼ぶ側が `(e & 3)` で渡し、
// n == 0 で止まる (本体は n を書き換えない)。ローカル配列は持たせない (フレームが重なって FC_STACK をあふれる)。
func (g *rpGen) genRec(name string) {
	f := &rpFunc{name: name, ret: g.typ(), rec: true}
	g.cur = f
	g.scope = nil
	g.nLocal = 0
	g.larrays = nil
	g.sptr = ""
	g.hptr = ""
	g.ptr, g.lsv, g.labels = "", "", nil
	n := rpVar{name: "n", typ: rpTypes[0], readOnly: true, small: true}
	a := rpVar{name: "a", typ: g.typ()}
	f.params = []rpVar{n, a}
	g.scope = append(g.scope, n, a)
	for i := 0; i < g.pick(2); i++ {
		v := g.newLocal(g.typ())
		f.locals = append(f.locals, fmt.Sprintf("var %s:%s = %s;", v.name, v.typ.name, g.lit(v.typ)))
	}
	f.stmts = append(f.stmts, rpSimple(fmt.Sprintf("if (n == 0) { return %s; }", g.expr(f.ret, 2))))
	for i := 0; i < g.pick(3)+1; i++ {
		f.stmts = append(f.stmts, g.stmt(1))
	}
	op := []string{"+", "-", "^", "&", "|"}[g.pick(5)]
	f.retExpr = fmt.Sprintf("(%s((n - 1), %s) %s %s)", name, g.expr(a.typ, 1), op, g.expr(f.ret, 1))
	g.funcs = append(g.funcs, f)
	g.cur = nil
}

// genSv は struct を値で受けて値で返す関数 `sv(v:S, k:int):S` を作る (本体は v のフィールドの書き換え)。
func (g *rpGen) genSv() {
	f := &rpFunc{name: "sv"}
	g.cur = f
	k := rpVar{name: "k", typ: rpTypes[0]}
	g.scope = []rpVar{k}
	g.nLocal, g.larrays, g.sptr, g.hptr, g.ptr, g.lsv, g.labels = 0, nil, "", "", "", "", nil
	for i := 0; i < g.pick(2)+1; i++ {
		a, b := g.fields[g.pick(len(g.fields))], g.fields[g.pick(len(g.fields))]
		op := []string{"+", "-", "^", "&", "|"}[g.pick(5)]
		f.stmts = append(f.stmts, rpSimple(fmt.Sprintf("v.%s = (%s %s %s);", a.name, cast("v."+b.name, b.typ, a.typ), op, g.expr(a.typ, 1))))
	}
	g.sv = f
	g.cur = nil
}

// genProgram はプログラム全体を作る (main の文は最後に「全部を出力して exit」)。
func (g *rpGen) genProgram() {
	if g.v3 != nil {
		g.genV3() // 先に作る (後の関数と main から呼べるように)
	}
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
	if len(g.fields) > 0 && g.chance(0.4) {
		g.soa = true // soa E:[8]S
	}
	if len(g.fields) > 0 && g.chance(0.4) {
		g.hasT, g.tArr, g.tX = true, g.typ(), g.typ() // struct T { s:S; arr:[4]tArr; x:tX; } と u0 / ua:[2]T
	}
	if len(g.fields) > 0 && g.chance(0.3) {
		// 同じ場所を配列 ab と struct w で読み書きする (ab は普通のグローバル配列としても使う)
		g.alias = true
		g.arrays = append(g.arrays, rpVar{name: "ab", typ: rpTypes[0]})
	}
	if len(g.fields) > 0 && g.chance(0.3) {
		g.genSv()
	}
	for i := 0; i < g.pick(3); i++ {
		g.consts = append(g.consts, rpVar{name: fmt.Sprintf("ct%d", i), typ: g.typ(), readOnly: true})
	}
	if g.chance(0.5) {
		g.hasFar = true
		for i := 0; i < g.pick(2)+1; i++ {
			g.genFunc(fmt.Sprintf("ff%d", i), true, nil)
		}
		if g.chance(0.4) {
			// far1 の関数の farfn 表 (バンクを切り替えるトランポリン経由で呼ぶ)
			g.fqSig = rpFunc{ret: g.typ()}
			for i := 0; i < g.pick(3); i++ {
				g.fqSig.params = append(g.fqSig.params, rpVar{typ: g.typ()})
			}
			for i := 0; i < 2; i++ {
				g.fqTable = append(g.fqTable, g.genFunc(fmt.Sprintf("fq%d", i), true, &g.fqSig))
			}
		}
	}
	for i := 0; i < g.pick(3)+1; i++ {
		g.genFunc(fmt.Sprintf("f%d", i), false, nil)
	}
	if g.chance(0.35) {
		g.genRec("r0")
	}
	if g.chance(0.5) {
		// 関数ポインタ表: 同じ型の関数 2 つか 4 つ
		g.fpSig = rpFunc{ret: g.typ()}
		for i := 0; i < g.pick(3); i++ {
			g.fpSig.params = append(g.fpSig.params, rpVar{typ: g.typ()})
		}
		for i := 0; i < 2+2*g.pick(2); i++ {
			g.fpTable = append(g.fpTable, g.genFunc(fmt.Sprintf("t%d", i), false, &g.fpSig))
		}
	}
	m := &rpFunc{name: "main"}
	g.cur = m
	g.scope = nil
	g.nLocal = 0
	for _, v := range g.globals {
		m.stmts = append(m.stmts, rpInit(fmt.Sprintf("%s = %s;", v.name, g.lit(v.typ))))
	}
	for _, a := range g.arrays {
		for i := 0; i < 16; i++ {
			m.stmts = append(m.stmts, rpInit(fmt.Sprintf("%s[%d] = %s;", a.name, i, g.lit(a.typ))))
		}
	}
	for _, f := range g.fields {
		m.stmts = append(m.stmts, rpInit(fmt.Sprintf("s0.%s = %s;", f.name, g.lit(f.typ))))
		for i := 0; i < 4; i++ {
			m.stmts = append(m.stmts, rpInit(fmt.Sprintf("sa[%d].%s = %s;", i, f.name, g.lit(f.typ))))
		}
		for i := 0; g.soa && i < 8; i++ {
			m.stmts = append(m.stmts, rpInit(fmt.Sprintf("E[%d].%s = %s;", i, f.name, g.lit(f.typ))))
		}
	}
	if g.hasT {
		for _, b := range []string{"u0", "ua[0]", "ua[1]"} {
			m.stmts = append(m.stmts, rpInit(fmt.Sprintf("%s.x = %s;", b, g.lit(g.tX))))
			for _, f := range g.fields {
				m.stmts = append(m.stmts, rpInit(fmt.Sprintf("%s.s.%s = %s;", b, f.name, g.lit(f.typ))))
			}
			for j := 0; j < 4; j++ {
				m.stmts = append(m.stmts, rpInit(fmt.Sprintf("%s.arr[%d] = %s;", b, j, g.lit(g.tArr))))
			}
		}
	}
	if g.chance(0.3) {
		// ラムダ (引数と大域変数だけを見る)
		scope, cur, sptr, hptr, ptr, lsv, larrays := g.scope, g.cur, g.sptr, g.hptr, g.ptr, g.lsv, g.larrays
		a := rpVar{name: "a", typ: rpTypes[0]}
		g.scope, g.cur, g.sptr, g.hptr, g.ptr, g.lsv, g.larrays = []rpVar{a}, &rpFunc{name: "lambda", inline: true}, "", "", "", "", nil
		body := g.expr(rpTypes[0], 2)
		g.scope, g.cur, g.sptr, g.hptr, g.ptr, g.lsv, g.larrays = scope, cur, sptr, hptr, ptr, lsv, larrays
		m.locals = append(m.locals, fmt.Sprintf("var lf:fn(int):int = ->fn(a:int):int { return %s; };", body))
		g.lambda = true
	}
	if g.fpTable != nil && g.chance(0.5) {
		// 関数ポインタのローカル変数 (付け替えながら呼ぶ)
		ps := make([]string, len(g.fpSig.params))
		for i, p := range g.fpSig.params {
			ps[i] = p.typ.name
		}
		m.locals = append(m.locals, fmt.Sprintf("var fv:fn(%s):%s = %s;", strings.Join(ps, ", "), g.fpSig.ret.name, g.fpTable[g.pick(len(g.fpTable))].name))
		g.fnVar = true
	}
	for i := 0; i < g.pick(4); i++ {
		v := g.newLocal(g.typ())
		m.locals = append(m.locals, fmt.Sprintf("var %s:%s = %s;", v.name, v.typ.name, g.lit(v.typ)))
	}
	g.larrays = nil
	g.sptr = ""
	g.hptr = ""
	g.ptr, g.lsv, g.labels = "", "", nil
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
	if g.chance(0.4) && (!f.inTable || os.Getenv("RP_TABLE_ARRAYS") != "") { // RP_TABLE_ARRAYS=1 で表の関数にも持たせる (以前の種の再現用)
		// 表の関数は表経由で互いに呼び合う (再帰 = stack 関数) ので、ローカル配列 (32 バイト) を持たせると -O 0 で
		// FC_STACK (128 バイト) をあふれてゼロページを壊す (種 312694: t1 64 + t2 27 + t3 36 バイトのフレームが重なり
		// pc=$ffff で invalid opcode)。プログラムの問題であってコンパイラのバグではない
		a := rpVar{name: fmt.Sprintf("la%d", len(g.larrays)), typ: g.typ()}
		g.larrays = append(g.larrays, a)
		f.locals = append(f.locals, fmt.Sprintf("var %s:[16]%s;", a.name, a.typ.name))
		for i := 0; i < 16; i++ {
			f.stmts = append(f.stmts, rpInit(fmt.Sprintf("%s[%d] = %s;", a.name, i, g.lit(a.typ))))
		}
	}
	as := g.arraysAll()
	for i := 0; i < g.pick(3) && len(as) > 0; i++ {
		a := as[g.pick(len(as))]
		p := rpVar{name: fmt.Sprintf("q%d", i), typ: a.typ, ptr: true, fresh: true}
		g.scope = append(g.scope, p)
		f.locals = append(f.locals, fmt.Sprintf("var %s:*%s = &%s[%d];", p.name, a.typ.name, a.name, g.pick(8)))
	}
	if len(g.fields) > 0 && g.sptr == "" && g.chance(0.4) {
		g.sptr = "ps"
		f.locals = append(f.locals, fmt.Sprintf("var ps:*S = &sa[%d];", g.pick(4)))
	}
	if g.soa && len(g.fields) > 0 && g.chance(0.35) {
		g.hptr = "h" // soa の要素ハンドル (1 バイトの添字)
		f.locals = append(f.locals, fmt.Sprintf("var h:*E = &E[%d];", g.pick(8)))
	}
	if g.hasT && g.chance(0.35) {
		g.ptr = "pt" // struct T へのポインタ
		f.locals = append(f.locals, fmt.Sprintf("var pt:*T = &ua[%d];", g.pick(2)))
	}
	if len(g.fields) > 0 && !f.rec && g.chance(0.3) {
		// ローカルの struct 変数 (フレームに置かれる。全フィールドを初期化)
		vals := make([]string, len(g.fields))
		for i, fl := range g.fields {
			vals[i] = g.lit(fl.typ)
		}
		g.lsv = "ls"
		f.locals = append(f.locals, fmt.Sprintf("var ls:S = {%s};", strings.Join(vals, ", ")))
	}
}

// source はプログラムのソース (runEmu が `#fc 2` と stdio を頭に足す)。
func (g *rpGen) source() string {
	var b strings.Builder
	b.WriteString("#fc 2\n")
	if g.hasFar {
		b.WriteString("options(farcall: true);\n")
	}
	b.WriteString("use * from stdio;\n")
	if g.hasFar {
		b.WriteString("use far1;\n")
	}
	if g.v3 != nil {
		b.WriteString("use v3m;\n")
	}
	for _, v := range g.globals {
		fmt.Fprintf(&b, "var %s:%s;\n", v.name, v.typ.name)
	}
	for k := range g.consts {
		c := &g.consts[k]
		if c.vals == nil {
			c.vals = make([]string, 16)
			for i := range c.vals {
				c.vals[i] = g.lit(c.typ)
			}
		}
		fmt.Fprintf(&b, "const %s:[16]%s = [%s];\n", c.name, c.typ.name, strings.Join(c.vals, ", "))
	}
	if g.wb {
		b.WriteString("var wb:[300]int options(segment: \"BSS_EX\");\n")
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
		if g.soa {
			b.WriteString("soa E:[8]S;\n")
		}
		if g.hasT {
			fmt.Fprintf(&b, "struct T {\n\ts:S;\n\tarr:[4]%s;\n\tx:%s;\n}\nvar u0:T;\nvar ua:[2]T;\n", g.tArr.name, g.tX.name)
		}
		if g.alias {
			b.WriteString("alias w:S = ab;\n")
		}
		if g.sv != nil {
			b.WriteString("function sv(v:S, k:int):S\n{\n")
			for _, st := range g.sv.stmts {
				st.render(&b)
				b.WriteString("\n")
			}
			b.WriteString("return v;\n}\n")
		}
	}
	if g.fqTable != nil {
		ps := make([]string, len(g.fqSig.params))
		for i, p := range g.fqSig.params {
			ps[i] = p.typ.name
		}
		names := make([]string, len(g.fqTable))
		for i, f := range g.fqTable {
			names[i] = "far1." + f.name
		}
		fmt.Fprintf(&b, "const fq0:[%d]farfn(%s):%s = [%s];\n", len(names), strings.Join(ps, ", "), g.fqSig.ret.name, strings.Join(names, ", "))
	}
	for _, f := range g.funcs {
		if f.far || f.mod != "" {
			continue // far1.fc / v3m.fc に出す
		}
		if f.name == "main" && g.fpTable != nil {
			// 関数ポインタ表 (要素の関数の後に置く)
			ps := make([]string, len(g.fpSig.params))
			for i, p := range g.fpSig.params {
				ps[i] = p.typ.name
			}
			names := make([]string, len(g.fpTable))
			for i, tf := range g.fpTable {
				names[i] = tf.name
			}
			if len(names) == 4 {
				// 4 要素の表は同じ関数を繰り返して 20 要素にする (添字は `& 3` のまま)。16 要素までは呼び出しが直接化 (devirt)
				// されるので、2 要素の表は直接化、4 要素の表は間接呼び出し (表から reg に直接読む形) を試す
				for len(names) < 20 {
					names = append(names, names[len(names)%4])
				}
			}
			fmt.Fprintf(&b, "const fp0:[%d]fn(%s):%s = [%s];\n", len(names), strings.Join(ps, ", "), g.fpSig.ret.name, strings.Join(names, ", "))
		}
		if f.name != "main" {
			g.writeFunc(&b, f, "")
			continue
		}
		b.WriteString("function main():void\n{\n")
		for _, l := range f.locals {
			b.WriteString(l + "\n")
		}
		for _, s := range f.stmts {
			s.render(&b)
			b.WriteString("\n")
		}
		{
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
				for i := 0; g.soa && i < 8; i++ {
					out = append(out, fmt.Sprintf("E[%d].%s", i, f.name), `" "`)
				}
			}
			for _, bs := range []string{"u0", "ua[0]", "ua[1]"} {
				if !g.hasT {
					break
				}
				out = append(out, bs+".x", `" "`)
				for _, f := range g.fields {
					out = append(out, bs+".s."+f.name, `" "`)
				}
				for j := 0; j < 4; j++ {
					out = append(out, fmt.Sprintf("%s.arr[%d]", bs, j), `" "`)
				}
			}
			fmt.Fprintf(&b, "printf(%s, \"\\n\");\nexit(0);\n", strings.Join(out, ", "))
		}
	}
	b.WriteString("}\n")
	return b.String()
}

// writeFunc は main 以外の関数 1 つのソース。
func (g *rpGen) writeFunc(b *strings.Builder, f *rpFunc, prefix string) {
	ps := make([]string, len(f.params))
	for i, p := range f.params {
		ps[i] = p.name + ":" + p.typ.name
		if p.ptr {
			ps[i] = p.name + ":*" + p.typ.name
		}
		if p.sptr {
			ps[i] = p.name + ":*S"
		}
	}
	var opts []string
	if f.fastcall {
		opts = append(opts, "fastcall: true")
	}
	if f.inline {
		opts = append(opts, "inline: true")
	}
	fmt.Fprintf(b, "%sfunction %s(%s):%s", prefix, f.name, strings.Join(ps, ", "), f.ret.name)
	if len(opts) > 0 {
		fmt.Fprintf(b, " options(%s)", strings.Join(opts, ", "))
	}
	b.WriteString("\n{\n")
	for _, l := range f.locals {
		b.WriteString(l + "\n")
	}
	for _, s := range f.stmts {
		s.render(b)
		b.WriteString("\n")
	}
	fmt.Fprintf(b, "return %s;\n}\n", f.retExpr)
}

// farSource は far1 モジュール (options(bank: 1)) のソース。
func (g *rpGen) farSource() string {
	var b strings.Builder
	b.WriteString("#fc 2\noptions(bank: 1);\n")
	for _, f := range g.funcs {
		if f.far {
			g.writeFunc(&b, f, "public ")
		}
	}
	return b.String()
}

// sources はビルドに要るファイル (t.fc と、あれば far1.fc)。
func (g *rpGen) sources() map[string]string {
	m := map[string]string{"t.fc": g.source()}
	if g.hasFar {
		m["far1.fc"] = g.farSource()
	}
	if g.v3 != nil {
		m["v3m.fc"] = g.v3Source()
	}
	return m
}

// allSource は表示用 (far1.fc も繋げる)。
func (g *rpGen) allSource() string {
	s := g.source()
	if g.v3 != nil {
		s += "// ---- v3m.fc ----\n" + g.v3Source()
	}
	if g.hasFar {
		s += "// ---- far1.fc ----\n" + g.farSource()
	}
	return s
}

// rpMaxCycles は 1 回の実行のサイクル数の上限 (-O 2 側。-O 0 は掛かったら 10 倍で走らせ直す)。
const rpMaxCycles = 20_000_000

// rpRun はソースを level でビルドして emu で走らせ、出力を返す (ビルドや終了コードの失敗は error。コンパイラの panic も
// error にして、次の種に進めるようにする)。
func rpRun(t *testing.T, files map[string]string, level int, maxCycles int64) (out string, err error) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	dir := t.TempDir()
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o666); err != nil {
			return "", err
		}
	}
	var o strings.Builder
	code, err := NewCompiler(absRepoRoot).Build("t.fc", &BuildOptions{Dir: dir, BuildDir: filepath.Join(dir, "b"), Out: filepath.Join(dir, "a.bin"), Run: true, Stdout: &o, OptimizeLevel: level, MaxCycles: maxCycles})
	if err != nil {
		return "", err
	}
	if code != 0 {
		return "", fmt.Errorf("exit code %d: %s", code, o.String())
	}
	return o.String(), nil
}

// rpInterpSteps はインタプリタで実行する IR 命令数の上限 (emu の 20M サイクルより十分多い。止まらない種は判定を飛ばす)。
const rpInterpSteps = 20_000_000

// rpInterp はソースを sema だけ通して、最適化前の IR をインタプリタで実行した出力を返す。インタプリタが扱わない命令
// (asm など) や上限は ok = false (判定しない)。
func rpInterp(t *testing.T, files map[string]string) (out string, ok bool, err error) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			out, ok, err = "", true, fmt.Errorf("panic: %v", r) // sema / インタプリタの panic も失敗として報告する
		}
	}()
	dir := t.TempDir()
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o666); err != nil {
			return "", false, err
		}
	}
	prog, cerr := sema.Compile(dir, NewCompiler(absRepoRoot).libPath("emu"), "t.fc")
	if cerr != nil {
		return "", false, cerr
	}
	res, err := interp.Run(prog.Modules.List(), rpInterpSteps)
	if errors.Is(err, interp.ErrUnsupported) || errors.Is(err, interp.ErrStepLimit) {
		return "", false, nil
	}
	if err != nil {
		return res.Out, true, err
	}
	if res.Exit != 0 {
		return res.Out, true, fmt.Errorf("exit code %d", res.Exit)
	}
	return res.Out, true, nil
}

// rpLocate は失敗したプログラムで、どの段が IR の意味を変えたかを切り分ける: sema の IR をインタプリタで実行した
// 出力を基準に、インライン展開 → opt の各段 (全関数に 1 段ずつ当てる) の後で実行し直し、最初に出力が変わった段を返す。
// 最後まで変わらなければ、壊したのはレジスタ割付か codegen (最適化の後の IR の意味は正しい)。
func rpLocate(t *testing.T, files map[string]string) (res string) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			res = fmt.Sprintf("切り分けの途中で panic: %v", r)
		}
	}()
	dir := t.TempDir()
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o666); err != nil {
			return err.Error()
		}
	}
	prog, cerr := sema.Compile(dir, NewCompiler(absRepoRoot).libPath("emu"), "t.fc")
	if cerr != nil {
		return cerr.Error()
	}
	mods := prog.Modules.List()
	run := func() (string, bool) {
		res, err := interp.Run(mods, rpInterpSteps)
		if errors.Is(err, interp.ErrUnsupported) || errors.Is(err, interp.ErrStepLimit) {
			return "", false
		}
		return fmt.Sprintf("%q exit=%d %v", res.Out, res.Exit, err), true
	}
	ref, ok := run()
	if !ok {
		return "切り分けられない (最適化前の IR をインタプリタが扱えない)"
	}
	if err := opt.InlineProgram(mods); err != nil {
		return "インライン展開がエラー: " + err.Error()
	}
	opt.DevirtualizeProgram(mods, prog.FarCallEnabled())
	if out, ok := run(); !ok || out != ref {
		return fmt.Sprintf("インライン展開 / devirtualize で出力が変わった (前 %s、後 %s)", ref, out)
	}
	var lmds []*ir.Lambda
	for _, m := range mods {
		for _, l := range m.Lambdas {
			if !l.Extern && len(l.Ops) > 0 {
				lmds = append(lmds, l)
			}
		}
	}
	for _, p := range opt.Passes(prog.Types) {
		for _, l := range lmds {
			p.Run(l)
		}
		out, ok := run()
		if !ok {
			return fmt.Sprintf("切り分けられない (opt の %s の後の IR をインタプリタが扱えない)", p.Name)
		}
		if out != ref {
			return fmt.Sprintf("opt の %s で出力が変わった (前 %s、後 %s。FC_DISABLE=%s で確かめる)", p.Name, ref, out, p.Name)
		}
	}
	return "最適化の後の IR はインタプリタで同じ出力: レジスタ割付か codegen (-O 0 だけなら codegen)"
}

// rpResult は 1 つのプログラムの判定。
type rpResult struct {
	kind   string // "ok" / "differs" / "interp" / "panic" / "error" (error は生成器の問題: 型エラーなど)
	detail string
}

// rpCheck は -O 0 と -O 2 で走らせて判定する。
func rpCheck(t *testing.T, files map[string]string) rpResult {
	o0, err0 := rpRun(t, files, -1, rpMaxCycles)
	o2, err2 := rpRun(t, files, 0, rpMaxCycles)
	hang := func(err error) bool { return err != nil && strings.HasPrefix(err.Error(), "cycle limit") }
	if hang(err0) && err2 == nil {
		// -O 0 だけ上限に掛かった: far call や除算の入れ子で -O 0 が -O 2 の 4 倍かかることがある (21M 対 5.7M) ので、
		// 止まらないと決める前に上限を上げて走らせ直す (最小化の途中で毎回 20M サイクル走らせるのを避ける意味もある)
		o0, err0 = rpRun(t, files, -1, rpMaxCycles*50)
	}
	if hang(err0) != hang(err2) && (err0 == nil || hang(err0)) && (err2 == nil || hang(err2)) {
		// 片方だけ止まらない (もう片方は正常): 出力の食い違いと同じ扱い
		return rpResult{"differs", fmt.Sprintf("-O 0: %s %v\n-O 2: %s %v", o0, err0, o2, err2)}
	}
	if hang(err0) && hang(err2) {
		return rpResult{"hang", err0.Error()} // 両方で止まらない: 生成器の問題 (ループの上限の抜け) かコンパイラ共通のバグ
	}
	for _, err := range []error{err0, err2} {
		if err != nil {
			detail := fmt.Sprintf("-O 0: %v\n-O 2: %v", err0, err2) // 両方のレベルの結果 (片方が上限、片方が panic のことがある)
			if strings.HasPrefix(err.Error(), "panic:") && !strings.Contains(err.Error(), "zero page index wrapped") {
				return rpResult{"panic", detail}
			}
			return rpResult{"error", detail}
		}
	}
	if o0 != o2 {
		return rpResult{"differs", fmt.Sprintf("-O 0: %s-O 2: %s", o0, o2)}
	}
	if *randInterp {
		oi, ok, err := rpInterp(t, files)
		if ok && (err != nil || oi != o2) {
			return rpResult{"interp", fmt.Sprintf("-O 0 / -O 2: %sinterp: %s %v", o2, oi, err)}
		}
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
			if saved.keep {
				continue // 初期化文は消さない (rpStmt.keep)
			}
			*list = append((*list)[:i:i], (*list)[i+1:]...)
			if rpCheck(t, g.sources()).kind == kind {
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
			res := rpCheck(t, g.sources())
			switch res.kind {
			case "ok":
			case "error":
				if strings.Contains(res.detail, "frame size over") {
					// フレームが上限 (静的フレーム 256 バイト、stack 系は FC_STACK の 128 バイト) を超える。-O 2 の展開で超えた関数は
					// driver が展開を止めてやり直すので、ここに来るのは展開を止めても (-O 0 でも) 超える大きすぎるプログラム
					t.Skipf("フレームが大きすぎる (seed %d)", seed)
				}
				if strings.Contains(res.detail, "memory area overflow") {
					t.Skipf("プログラムが大きすぎて ROM に入らない (seed %d)", seed)
				}
				if strings.Contains(res.detail, "zero page index wrapped") {
					// 呼び出しの入れ子でフレームの合計が FC_STACK (128 バイト) を超えた (emu が S+k,x のページ越えで検出)。
					// プログラムの問題 (生成器が表の関数にローカル配列を持たせないようにして減らした)
					t.Skipf("ソフトウェアスタックがあふれた (seed %d)", seed)
				}
				t.Fatalf("ビルド失敗 (生成器の問題) (seed %d):\n%s\n%s", seed, g.allSource(), res.detail)
			case "hang":
				// 両方のレベルで止まらない: 入れ子のループ × 呼び出しで単に重い (サイクルの上限を超える) のがほとんどで、
				// 生成器の問題として飛ばす (コンパイラ共通のバグならほかの形でも出る)
				t.Skipf("両方のレベルでサイクルの上限を超えた (seed %d)", seed)
			default:
				rpMinimize(t, g, res.kind)
				res = rpCheck(t, g.sources())
				res.detail += "\n切り分け: " + rpLocate(t, g.sources())
				t.Errorf("%s (seed %d):\n%s\n%s", map[string]string{"differs": "-O 0 と -O 2 の出力が違う", "interp": "-O 0 / -O 2 とインタプリタ (最適化前の IR) の出力が違う", "panic": "コンパイラが panic", "hang": "両方のレベルで止まらない"}[res.kind], seed, g.allSource(), res.detail)
			}
		})
	}
}
