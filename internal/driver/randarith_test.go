package driver

// fc 4 の整数の規則の Go 模型 (TestRandomArithV4)。
//
// 既存の差分 fuzz は -O 0 / -O 2 / 最適化前の IR のインタプリタを比べるだけで、3 つとも sema の作った同じ IR を実行するので
// **sema と型の規則の誤りは 3 つとも同じように間違えて見えない** (Agent/wiki/testing-and-fuzzing.md「差分テストの判定の弱点」)。
// TestRandomConstFoldV4 は「畳み込みと実行時」の差分、TestTypeRuleMatrix は「通る / エラー」の表で、どちらも**値**を知らない。
// ここは式の意味の模型を Go に書いて、プログラム (変数の形) と定数の形 (畳み込み) の両方の出力と比べる。
//
// 模型の規則 (2026-10-18 に fcc run で実測して確かめたもの。仕様の正は docs/reference/language.md「整数の規則」):
//   - 式の計算の型 E = (式の中のいちばん広い型と代入先の型の広いほうの幅, 式の中の**型が決まっている値**で決まる符号)。
//     代入先は幅だけ効く (`var w:i16 = (p - q) / 7` の p, q が u8 なら無符号のまま = 9333)。
//     型のない定数は符号に効かない (`y_i8 + 300` は i16 の 294、`x_u8 * 300` は u16 の 60000)
//   - 型のない定数は式を広くできる (自分が入るいちばん小さい型の幅)。幅だけ見て、幅が同じときの符号は型が決まっている値が見る
//   - 型のない定数だけの式は任意精度で計算し、代入先の型に入ることを確かめる (黙って切り詰めない)。型が決まっている値が 1 つでも
//     入ればその幅で折り返す (`var b:u8 = 200 + 100` はエラー、`var b:u8 = (200 as u8) + 100` は 44)
//   - 同じ幅で符号が違う型が決まっている値が混ざった式 (mixed) は、符号つきの意味で読む所 (型を書かない変数、`/` `%` `>>` の
//     左辺、大小の比較、@printf の引数) でエラー。この生成器は mixed を作らない (符号を混ぜる `+%` は作れる)
//   - `as`・`+%`・関数の引数は区切り (内側は自分の型で計算し、外側の広い代入先では広げない)。条件式 `c ? x : y` は区切りでない
//   - 比較の型のない定数は相手の型を取り、入らなければエラー (算術のように広くはならない: `x_i8 < 191` はエラー)
//   - `/` と `%` は床除算 (商は負の無限大へ、余りの符号は割る数)。シフトの結果は左辺の型で、量は型の幅未満のみ生成する
//   - 定数の畳み込みは実行時と同じ幅で折り返す (`const c:u8 = (200 as u8) + (100 as u8);` は 44)
//
// 生成する形: 境界値のグローバル変数と型のない定数を混ぜた式を、var への初期化 / グローバルへの代入 / @printf 直書きのいずれかで
// 書き、同じ式を型つき定数に置き換えた形 (畳み込み) も並べて出す (id_T の所は `as` にする。呼び出しと `as` は同じ区切り)。
// 比較は if にして hits に積む。

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
)

// arTy は整数の型 (w バイト、s は符号つき)。
type arTy struct {
	w int
	s bool
}

var (
	arU8  = arTy{1, false}
	arI8  = arTy{1, true}
	arU16 = arTy{2, false}
	arI16 = arTy{2, true}
	arU24 = arTy{3, false}
	arI24 = arTy{3, true}
	arU32 = arTy{4, false}
	arI32 = arTy{4, true}
)

var arAllTy = []arTy{arU8, arI8, arU16, arI16, arU24, arI24, arU32, arI32}

func (t arTy) name() string {
	if t.s {
		return fmt.Sprintf("i%d", t.bits())
	}
	return fmt.Sprintf("u%d", t.bits())
}

func (t arTy) bits() int { return t.w * 8 }

// wrap は v を自分の幅に折り返し、自分の符号で読み直す。
func (t arTy) wrap(v int64) int64 {
	b := uint(t.bits())
	v &= int64(1)<<b - 1
	if t.s && v&(int64(1)<<(b-1)) != 0 {
		v -= int64(1) << b
	}
	return v
}

// fits は v が自分の範囲に入る値か (定数は型に入らないとエラーになる規則の方)。
func (t arTy) fits(v int64) bool { return t.wrap(v) == v }

// arMinType は型のない定数の式への寄与 (入るいちばん小さい型。負でなければ符号なし。sema の IntValue)。32 ビットに入らない値は
// 32 ビットの型 (生成しない: arTooWide)。
func arMinType(v int64) arTy {
	for w := 1; w < 4; w++ {
		if t := (arTy{w, v < 0}); t.fits(v) {
			return t
		}
	}
	return arTy{4, v < 0}
}

// arMaxExact は型のない定数だけの式の途中の値の上限 (超えたら作らない: int64 をはみ出さないように。sema も int で畳む)。
const arMaxExact = int64(1) << 40

// arBeyond は v がどの整数型 (32 ビット) にも入らないか (sema の anyIntRange)。
func arBeyond(v int64) bool { return v < -(int64(1)<<31) || v > int64(1)<<32-1 }

// arCombine は 2 つの値の演算の型 (大きいほう、同じ大きさなら符号つき)。
func arCombine(a, b arTy) arTy {
	if a.w != b.w {
		if a.w > b.w {
			return a
		}
		return b
	}
	if a.s || b.s {
		return arTy{a.w, true}
	}
	return a
}

// arDiv / arMod は床除算 (Go の / % は 0 方向への切り捨てなので補正する)。
func arDiv(a, b int64) int64 {
	q := a / b
	if a%b != 0 && (a < 0) != (b < 0) {
		q--
	}
	return q
}

func arMod(a, b int64) int64 {
	r := a % b
	if r != 0 && (r < 0) != (b < 0) {
		r += b
	}
	return r
}

// arNode は生成した式の節点。src は変数の形、srcConst は型つき定数に置き換えた形 (畳み込みの判定用)。
// val と n は「この節点だけを領域として計算した」値と型 (as / id / +% の内側が自分の領域で計算されるので必要)。
type arNode struct {
	kind     int // 0 値、1 二項、2 as、3 id_T、4 +%
	op       string
	a, b     *arNode
	typ      arTy // kind 0 で型が決まっている値の型
	un       bool // kind 0 で型のない定数
	val      int64
	src      string
	srcConst string
	n        arTy // 自分の領域の計算の型 (代入先を含まない)

	shift int  // 1 ならシフトの量 (必ず型の幅未満の定数)
	bad   bool // 型のない定数だけの式の途中の値が arMaxExact を超えた (作らない: arTooWide)
	// pure は型のない定数だけの式。そこは任意精度で計算して代入先との適合を見る (`var b:u8 = 200 + 100;` は 44 ではなく
	// 「300 does not fit in u8」)。型が決まっている値が 1 つでも入ればその幅で折り返す (`var b:u8 = (200 as u8) + 100;` は 44)。
	pure  bool
	exact int64 // pure の式の任意精度の値
}

// regionTy は領域の計算の型 (代入先の幅 targetW を含める。代入先は幅だけ効くので符号は式の値のまま)。
func (x *arNode) regionTy(targetW int) arTy { return arTy{max(x.n.w, targetW), x.n.s} }

// join2 は 2 つの子から親の領域の型を作る (型のない定数を相手に合わせた型の互換型。sema/typeplan.go planArith)。
func (x *arNode) join2(a, b *arNode) {
	at, _ := arAdaptLit(a, b, false)
	bt, _ := arAdaptLit(b, a, false)
	x.n = arCombine(at, bt)
}

// arAdaptLit は節点 a の型への寄与。a が型のない定数のリテラルのときだけ、相手 b の型に合わせる (sema/literal.go の
// literalRule)。返す bool は比較で相手の型に入らない定数 (エラーになるので生成側で避ける)。
func arAdaptLit(a, b *arNode, cmp bool) (arTy, bool) {
	if !a.pure {
		return a.n, false // 型が決まっている値 (畳み込んだ式も定数として相手に合わせる)
	}
	n, lt := a.exact, a.n
	if a.kind == 0 {
		n = a.val
	}
	other, otherUntyped := b.n, b.pure
	if otherUntyped {
		return lt, false // 定数どうし (普通は畳み込まれている)
	}
	if arBeyond(n) {
		return lt, false // どの整数型にも入らない定数 (生成しない: arTooWide)
	}
	if lt.w > other.w {
		// 定数のほうが大きい型: 広いほうで計算する。比較で相手が符号付きなら定数の大きさの符号付き (`s < 300` (s:i8) は i16)
		if cmp && other.s && !lt.s && n <= int64(1)<<(8*lt.w-1)-1 {
			return arTy{lt.w, true}, false
		}
		// 32768 以上は定数の型 (u16) で比べる (`x == 37440` (x:i8) は u16 の比較。TestConstFoldCases の
		// `(65533 > ((-81 as i8) >> 9))` と同じ)。2026-10-19 まで 8 ビットの式だけ「37440 does not fit in i16」になり (変数は
		// 通る)、模型もそれに合わせてエラーにしていた (31ef042)。コンパイラを変数と同じに直して戻した
		return lt, false
	}
	if other.fits(n) {
		return other, false // 相手の型に収まる
	}
	if cmp {
		return other, true // 比較で相手の型に入らない (`x == -1` (x:u8) はエラー)
	}
	return other, false // 演算は相手の型に切り詰める (`x + -1` (x:u8) は x + 255)
}

// arMinType は型のない定数の式への寄与 (入るいちばん小さい型)。

// arGen は 1 つのプログラム。
type arGen struct {
	r      *rand.Rand
	vars   []*arNode
	want   []string
	nstmt  int
	hits   int64
	limit  int
	signed bool // 作っている領域の符号 (mixed を作らないための制約)
}

func (g *arGen) pick(n int) int        { return g.r.Intn(n) }
func (g *arGen) chance(p float64) bool { return g.r.Float64() < p }

// arBoundary は折り返し・符号・桁上がりが狙える値 (どの幅の型にも入る値)。arWideBoundary はその幅から入る値。
var arBoundary = map[bool][]int64{
	false: {0, 1, 2, 127, 128, 200, 254, 255},
	true:  {-128, -127, -6, -1, 0, 1, 63, 127},
}

var arWideBoundary = map[arTy][]int64{
	arU24: {65535, 65536, 0x800000, 0xffffff},
	arI24: {-8388608, -32769, 32768, 8388607},
	arU32: {0xffffff, 0x1000000, 0x80000000, 0xffffffff},
	arI32: {-2147483648, -8388609, 8388608, 2147483647},
}

// arBoundaryOf は型 t のグローバル変数の初期値。
func arBoundaryOf(t arTy) []int64 {
	return append(append([]int64{}, arBoundary[t.s]...), arWideBoundary[t]...)
}

// leaf は値の節点 (グローバル変数、型のない定数)。
func (g *arGen) leaf(signed bool) *arNode {
	if typed := g.varLeaf(signed); len(typed) > 0 && g.chance(0.62) {
		return typed[g.pick(len(typed))]
	}
	lo, hi := int64(0), int64(255)
	if signed {
		lo, hi = -128, 300
	} else if g.chance(0.25) {
		hi = 700 // 式を 2 バイトに広げる定数
	}
	if g.chance(0.12) {
		// 式を 3・4 バイトに広げる定数 (型のない定数は入るいちばん狭い型: u24 / i24 / u32 / i32)
		hi = []int64{70000, 20000000, 3000000000}[g.pick(3)]
		if signed {
			lo = -hi / 2
		}
	}
	return arLit(lo + g.r.Int63n(hi-lo+1))
}

// varLeaf はグローバル変数の節点だけ (%+ の左の項など、型が要る所)。
func (g *arGen) varLeaf(signed bool) []*arNode {
	var typed []*arNode
	for _, v := range g.vars {
		if v.typ.s == signed {
			typed = append(typed, v)
		}
	}
	return typed
}

// arLit は型のない定数の節点。
func arLit(v int64) *arNode {
	s := fmt.Sprint(v)
	if v < 0 {
		s = "(" + s + ")"
	}
	x := &arNode{kind: 0, un: true, val: v, typ: arMinType(v), src: s, srcConst: s, pure: true, exact: v, n: arMinType(v)}
	return x
}

// expr は深さ depth の式 (領域の符号は g.signed)。
func (g *arGen) expr(depth int) *arNode {
	if depth <= 0 || g.chance(0.3) {
		return g.leaf(g.signed)
	}
	switch k := g.pick(11); {
	case k < 5: // 領域をまたがない二項演算
		ops := []string{"+", "-", "*", "&", "|", "^", "/", "%", "<<", ">>"}
		op := ops[g.pick(len(ops))]
		a := g.expr(depth - 1)
		var b *arNode
		if op == "<<" || op == ">>" {
			// シフトの量は型の幅未満の定数 (実測で負の量が実装依存になるので使わない)。2 バイトの値には 8〜15 (上位バイトの抽出)
			b = arLit(int64(g.pick(8 * a.n.w)))
		} else {
			b = g.expr(depth - 1)
			if op == "/" || op == "%" {
				// 0 で割らない: どの幅でも奇数になる `(<式> | 1)` で包む
				inner := b
				b = &arNode{kind: 1, op: "|", a: inner, b: arLit(1),
					src:      "(" + inner.src + " | 1)",
					srcConst: "(" + inner.srcConst + " | 1)"}
				if inner.pure {
					b.pure = true
					b.bad = inner.bad
					b.exact = inner.exact | 1
					b.n = arMinType(b.exact)
				} else {
					b.join2(inner, b.b)
				}
				b.val = arValue(b, 0)
			}
		}
		x := &arNode{kind: 1, op: op, a: a, b: b,
			src:      fmt.Sprintf("(%s %s %s)", a.src, op, b.src),
			srcConst: fmt.Sprintf("(%s %s %s)", a.srcConst, op, b.srcConst)}
		if op == "<<" || op == ">>" {
			x.shift = 1
		}
		if a.pure && b.pure {
			x.pure = true
			x.bad = a.bad || b.bad || arAbs(a.exact) >= arMaxExact || arAbs(b.exact) >= arMaxExact || (op == "<<" && b.exact > 20)
			if !x.bad {
				x.exact = arApply(op, a.exact, b.exact)
				x.bad = arAbs(x.exact) >= arMaxExact
			}
			x.n = arMinType(x.exact) // 型のない定数だけの式は、自分の値が入るいちばん小さい型が型 (シフトも同じ)
		} else {
			x.join2(a, b)
			if x.shift != 0 {
				x.n = a.n // シフトの結果は左辺の型 (シフトの量は幅に効かない)
			}
		}
		x.val = arValue(x, 0)
		return x
	case k == 5: // as (区切り)
		in := g.region()
		to := g.tyOf(g.signed)
		return &arNode{kind: 2, a: in, typ: to, n: to, val: to.wrap(in.val),
			src:      fmt.Sprintf("(%s as %s)", in.src, to.name()),
			srcConst: fmt.Sprintf("(%s as %s)", in.srcConst, to.name())}
	case k == 6: // id_T (noinline の同一関数で包んで畳み込みを防ぐ。引数は代入と同じ区切り)
		in := g.region()
		to := in.n // 呼び出しは幅を広げない (引数の型 = 内側の計算の型)。定数の形も同じ区切りになるよう `as` にする
		if in.pure && !to.fits(in.exact) {
			return in // 引数は代入と同じ区切りで、型のない定数の式は型に入らなければならない (`id_u8(173 << 4)` はエラー)
		}
		return &arNode{kind: 3, a: in, typ: to, n: to, val: to.wrap(arValue(in, to.w)),
			src:      fmt.Sprintf("id_%s(%s)", to.name(), in.src),
			srcConst: fmt.Sprintf("(%s as %s)", in.srcConst, to.name())}
	case k == 7: // +% (左の項の型で計算してその幅で折り返す区切り)
		vs := g.varLeaf(g.signed)
		a := vs[g.pick(len(vs))] // 左の項に型が要る (型のない定数は書けない)
		l := a.n                 // 結果の型は左の項の型
		b := g.leaf(l.s)
		if g.chance(0.3) { // 右を符号違いにもできる (+% の使いどころ)
			b = g.leaf(!l.s)
		}
		for b.n.w > l.w || (b.un && !l.fits(b.val)) {
			b = g.leaf(l.s)
		}
		op := []string{"+%", "-%", "*%"}[g.pick(3)]
		x := &arNode{kind: 4, op: op, a: a, b: b, typ: l, n: l,
			src:      fmt.Sprintf("(%s %s %s)", a.src, op, b.src),
			srcConst: fmt.Sprintf("(%s %s %s)", a.srcConst, op, b.srcConst)}
		x.val = arValue(x, 0)
		return x
	case k == 8:
		return g.expr(depth - 1)
	case k == 9, k == 10: // @min / @max (型は両辺の互換型。型のない定数は比較と同じ規則で、相手の型に入らなければエラー)
		a := g.expr(depth - 1)
		b := g.expr(depth - 1)
		ta, ea := arAdaptLit(a, b, true)
		tb, eb := arAdaptLit(b, a, true)
		if ea || eb || ta.s != tb.s {
			return a // 相手の型に入らない定数と、符号の違う両辺は作らない (中の比較がエラー)
		}
		op := "@min"
		if k == 10 {
			op = "@max"
		}
		x := &arNode{kind: 5, op: op, a: a, b: b,
			src:      fmt.Sprintf("%s(%s, %s)", op, a.src, b.src),
			srcConst: fmt.Sprintf("%s(%s, %s)", op, a.srcConst, b.srcConst)}
		if a.pure && b.pure {
			x.pure = true
			x.bad = a.bad || b.bad
			x.exact = arMinMax(op, a.exact, b.exact)
			x.n = arMinType(x.exact)
		} else {
			x.n = arCombine(ta, tb)
		}
		x.val = arValue(x, 0)
		return x
	}
	return g.leaf(g.signed)
}

// region は新しい領域 (as / id の内側)。自分の計算の型で計算される。領域の符号は引き継ぐ。
func (g *arGen) region() *arNode { return g.expr(1 + g.pick(2)) }

// tyOf は符号を固定した型の選び方 (1・2 バイトを多めに)。
func (g *arGen) tyOf(signed bool) arTy {
	w := []int{1, 1, 1, 2, 2, 2, 3, 4}[g.pick(8)]
	return arTy{w, signed}
}

func arAbs(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

// arApply は二項演算の値 (計算の型での読み方は呼び出し側。折り返しも呼び出し側)。
func arApply(op string, a, b int64) int64 {
	switch op {
	case "+", "+%":
		return a + b
	case "-", "-%":
		return a - b
	case "*", "*%":
		return a * b
	case "&":
		return a & b
	case "|":
		return a | b
	case "^":
		return a ^ b
	case "/":
		return arDiv(a, b)
	case "%":
		return arMod(a, b)
	case "<<":
		return a << b
	case ">>":
		return a >> b
	}
	panic("知らない演算 " + op)
}

// arValue は式の評価した値を、そのノード自身の計算の型の解釈で返す (親が自分の型に書き換える)。W は親から来た幅
// (A1。0 なら無い)。ノード自身の計算の型 = (親から来た幅と自分の広げる前の型の広いほうの幅, 自分の広げる前の型の符号)
// で、幅は上から流れるが符号は流れない (internal/sema/widen.go「符号は広げる前の型の符号」)。
// 型のない定数だけの式は任意精度のまま返す (読み取る所で折り返す)。
func arValue(x *arNode, W int) int64 {
	if x.pure {
		return x.exact
	}
	switch x.kind {
	case 0:
		return x.val
	case 1:
		t := arTy{max(W, x.n.w), x.n.s}
		return t.wrap(arApply(x.op, t.wrap(arValue(x.a, t.w)), t.wrap(arValue(x.b, t.w))))
	case 2:
		return x.typ.wrap(arValue(x.a, 0)) // as は区切り (内側に幅を流さない)
	case 3:
		return x.typ.wrap(arValue(x.a, x.typ.w)) // 呼び出しの引数は代入と同じ区切り
	case 4:
		t := x.typ // +% は左の項の型で計算してその幅で折り返す区切り
		return t.wrap(arApply(x.op, t.wrap(arValue(x.a, t.w)), t.wrap(arValue(x.b, t.w))))
	case 5:
		t := arTy{max(W, x.n.w), x.n.s}
		return t.wrap(arMinMax(x.op, t.wrap(arValue(x.a, t.w)), t.wrap(arValue(x.b, t.w))))
	}
	panic("不明な節点")
}

// arMinMax は @min / @max (引数は計算の型で読んだ後なので、そのまま比べる)。
func arMinMax(op string, a, b int64) int64 {
	if (op == "@min") == (a < b) {
		return a
	}
	return b
}

// arCmp は比較 (計算の型の解釈で)。
func arCmp(op string, a, b int64) bool {
	switch op {
	case "==":
		return a == b
	case "!=":
		return a != b
	case "<":
		return a < b
	case "<=":
		return a <= b
	case ">":
		return a > b
	}
	return a >= b
}

// stmtExpr は文に使う式 (16 ビットに収まらない定数や符号を混ぜる式は作り直す。使う所ごとの検査は文の側でやる)。
func (g *arGen) stmtExpr() *arNode {
	for range 8 {
		x := g.expr(1 + g.pick(3))
		if !arTooWide(x) && !arMixed(x, 0) {
			return x
		}
	}
	return nil
}

// arPlace は式 x を型 t の所へ書くときの (計算の型 E, その型で読んだ値)。型のない定数だけの式は折り返さず、
// 代入先の型に入ることを先に確かめる (fc 4 は定数を黙って切り詰めない)。
func arPlace(x *arNode, t arTy) (arTy, int64) {
	e := x.regionTy(t.w)
	if x.pure {
		return e, x.exact
	}
	return e, e.wrap(arValue(x, e.w))
}

// placeTy は書き先の型 (pure な式は値が入る型を選ぶ)。無ければ nil。
func (g *arGen) placeTy(x *arNode) *arTy {
	for range 8 {
		t := g.tyOf(x.n.s)
		if t.w < x.n.w {
			t = arTy{x.n.w, x.n.s}
		}
		// 代入先が式を広くしても、式の型が代入先より広くなったら (暗黙に収まらない幅になったら) 書けない
		e := x.regionTy(t.w)
		if e.w > t.w {
			continue
		}
		if !x.pure || t.fits(x.exact) {
			return &t
		}
	}
	return nil
}

// arTooWide は型のない定数だけの式にどの整数型 (32 ビット) にも収まらない値があるか (foldBeyond など別経路になるので
// 生成しない)。途中の値が大きすぎる式 (bad) も。
func arTooWide(x *arNode) bool {
	if x.pure {
		return x.bad || arBeyond(x.exact)
	}
	if x.kind == 1 || x.kind == 5 {
		return arTooWide(x.a) || arTooWide(x.b)
	}
	if x.kind == 2 || x.kind == 3 {
		return arTooWide(x.a)
	}
	if x.kind == 4 {
		return arTooWide(x.a) || arTooWide(x.b)
	}
	return false
}

// arMixed は式に符号の混ざった演算の印が付くか (sema/mixedarith.go)。同じ大きさで符号の違う型が決まっている値の + - * が起点
// で、A1 で広い幅で計算する所は付かない。印は + - * & | ^ << と単項の - ~ で引き継がり、使う所 (大小の比較、/ % >>、
// @printf の引数) でエラーになる。生成側は式全体で見て避ける (呼ぶ所ごとに分けるほど値がない)。
func arMixed(x *arNode, W int) bool {
	if x.pure {
		return false
	}
	switch x.kind {
	case 5:
		return arMixed(x.a, max(W, x.n.w)) || arMixed(x.b, max(W, x.n.w))
	case 1:
		t := arTy{max(W, x.n.w), x.n.s}
		at, _ := arAdaptLit(x.a, x.b, false)
		bt, _ := arAdaptLit(x.b, x.a, false)
		if !x.a.pure && !x.b.pure && at.w == bt.w && at.s != bt.s && x.n.w == at.w && t.w == x.n.w {
			switch x.op {
			case "+", "-", "*":
				return true
			}
		}
		switch x.op {
		case "/", "%":
			return arMixed(x.a, t.w) || arMixed(x.b, t.w)
		case "@min", "@max": // 中で比較する (印を使う所)
			return arMixed(x.a, t.w) || arMixed(x.b, t.w)
		case ">>":
			return arMixed(x.a, t.w)
		case "+", "-", "*", "&", "|", "^", "<<":
			if arMixed(x.a, t.w) {
				return true
			}
			if x.op == "<<" {
				return false
			}
			return arMixed(x.b, t.w)
		}
	case 2: // as で 16 ビットに広げる所は印を使う所。広げない as は代入と同じ区切りで、中 (広げる as など) を見る
		// (`(id_i16((u8 + i8) as i16)) as i16` の外の as で止めて、中の広げる as を見落としていた。種 2026102201430635)
		if x.typ.w > x.a.n.w {
			return arMixed(x.a, 0)
		}
		return arMixed(x.a, x.typ.w)
	case 3: // 呼び出しの引数は代入と同じ区切り (中の `as` で広げる所などを見る)
		return arMixed(x.a, x.typ.w)
	case 4:
		return arMixed(x.a, x.typ.w) || arMixed(x.b, x.typ.w)
	}
	return false
}

// program は 1 つのプログラム (ソースと期待する出力) を作る。
func (g *arGen) program() (map[string]string, string) {
	var b strings.Builder
	b.WriteString("#fc 4\nuse console;\n")
	for _, t := range arAllTy {
		fmt.Fprintf(&b, "function id_%s(v:%s):%s @(noinline) { return v; }\n", t.name(), t.name(), t.name())
	}
	for _, t := range arAllTy {
		for i, v := range arBoundaryOf(t) {
			if !t.fits(v) {
				panic(fmt.Sprintf("生成器のバグ: %d が %s に入らない", v, t.name()))
			}
			lit := fmt.Sprint(v)
			if v < 0 {
				lit = fmt.Sprintf("(%s as %s)", lit, t.name())
			}
			name := fmt.Sprintf("g%s%d", t.name(), i)
			fmt.Fprintf(&b, "var %s:%s = %s;\n", name, t.name(), lit)
			x := &arNode{kind: 0, typ: t, val: t.wrap(v), src: name,
				srcConst: fmt.Sprintf("(%s as %s)", lit, t.name())}
			x.n = t
			g.vars = append(g.vars, x)
		}
	}
	b.WriteString("var hits:u8;\n")
	for _, t := range arAllTy {
		fmt.Fprintf(&b, "var v%s:%s;\n", t.name(), t.name())
	}

	var body strings.Builder
	for k := 0; k < g.limit; k++ {
		g.signed = g.chance(0.5)
		x := g.stmtExpr()
		if x == nil || arTooWide(x) || arMixed(x, 0) {
			continue
		}
		g.nstmt++
		if g.chance(0.2) {
			g.loop(&body, k, x)
			continue
		}
		switch g.pick(4) {
		case 0, 1, 2: // var への初期化と、同じ式の型つき定数の形 (畳み込み)
			t := g.placeTy(x)
			if t == nil {
				continue
			}
			tt := *t
			_, v := arPlace(x, tt)
			if !tt.fits(v) {
				// const は暗黙に切り詰めない (`const c:i16 = <u16 の値 55834>` は実行時は入り、定数はエラー)
				continue
			}
			name, cname := fmt.Sprintf("r%d", k), fmt.Sprintf("c%d", k)
			fmt.Fprintf(&body, "\tvar %s:%s = %s;\n", name, tt.name(), arRender(x, false))
			fmt.Fprintf(&body, "\tconst %s:%s = %s;\n", cname, tt.name(), arRender(x, true))
			fmt.Fprintf(&body, "\t@printf(\"{} {}\\n\", %s, %s);\n", name, cname)
			w := tt.wrap(v)
			g.want = append(g.want, fmt.Sprintf("%d %d", w, w))
		case 3: // @printf に直書き (符号つきの意味で読む所)
			e := x.n
			v := e.wrap(arValue(x, 0))
			fmt.Fprintf(&body, "\t@printf(\"{} {}\\n\", %s, %s);\n", arRender(x, false), arRender(x, true))
			g.want = append(g.want, fmt.Sprintf("%d %d", v, v))
		default: // グローバルへの代入
			var cands []arTy
			for _, t := range arAllTy {
				if t.w >= x.n.w && x.regionTy(t.w).w <= t.w && (!x.pure || t.fits(x.exact)) {
					cands = append(cands, t)
				}
			}
			if len(cands) == 0 {
				continue
			}
			t := cands[g.pick(len(cands))]
			_, v := arPlace(x, t)
			name := "v" + t.name()
			fmt.Fprintf(&body, "\t%s = %s;\n", name, arRender(x, false))
			fmt.Fprintf(&body, "\t@printf(\"{}\\n\", %s);\n", name)
			g.want = append(g.want, fmt.Sprint(t.wrap(v)))
		}
		if !g.chance(0.6) {
			continue
		}
		// 比較 (if の形。hits に積む)
		a := g.stmtExpr()
		if a == nil || arTooWide(a) {
			continue
		}
		g.signed = a.n.s
		bb := g.stmtExpr()
		if bb == nil || arTooWide(bb) {
			continue
		}
		op := []string{"==", "!=", "<", "<=", ">", ">="}[g.pick(6)]
		// 比較の型のない定数は相手の型を取る (compareWidth。算術のように切り詰めず、入らなければエラー)
		ta, ea := arAdaptLit(a, bb, true)
		tb, eb := arAdaptLit(bb, a, true)
		if ea || eb {
			continue
		}
		w := max(ta.w, tb.w)
		if arMixed(a, w) || arMixed(bb, w) {
			continue // 印を使う所 (大小の比較)
		}
		if ta.s != tb.s && op != "==" && op != "!=" {
			continue // 符号の違う値の大小の比較はエラー (F6)
		}
		e := arCombine(ta, tb)
		if !e.s {
			// == / != の符号違いはビットで比べる (広い幅で無符号として読む)
			e = arTy{w, false}
		}
		fmt.Fprintf(&body, "\tif (%s %s %s) {\n\t\thits = hits + 1;\n\t}\n", arRender(a, false), op, arRender(bb, false))
		va, vb := e.wrap(arValue(a, w)), e.wrap(arValue(bb, w))
		if a.pure && bb.pure {
			// 型のない定数どうしは数として比べる (折り返さない。`(-81) != 175` は真: i8 に折り返すと両方 -81。seed 61437275)
			va, vb = a.exact, bb.exact
		}
		if arCmp(op, va, vb) {
			g.hits++
		}
	}

	fmt.Fprintf(&body, "\t@printf(\"{}\\n\", hits);\n\tconsole.exit(0);\n")
	g.want = append(g.want, fmt.Sprint(arU8.wrap(g.hits)))
	fmt.Fprintf(&b, "function main():void\n{\n%s}\n", body.String())
	return map[string]string{"t.fc": b.String()}, strings.Join(g.want, "\n") + "\n"
}

// loop はループで局所変数を何度も更新する文 (3・4 バイトの局所変数が多め。展開・licm・強さの低減などの最適化を通る形)。
// `var lK:T = 初期値; for (var iK:u8 = 0; iK < n; iK += 1) { lK = lK OP x; lK = lK + iK; }` の後に lK を出す。x はループの外の値
// だけの式 (毎周同じ値)。模型は周ごとに lK の値を入れ直して同じ節点を評価する。
func (g *arGen) loop(body *strings.Builder, k int, x *arNode) {
	t := arTy{[]int{1, 2, 3, 3, 4, 4}[g.pick(6)], g.chance(0.5)}
	init := arBoundaryOf(t)
	cur := init[g.pick(len(init))]
	lv := &arNode{kind: 0, typ: t, n: t, val: cur, src: fmt.Sprintf("loc%d", k)}
	iv := &arNode{kind: 0, typ: arU8, n: arU8, src: fmt.Sprintf("it%d", k)}
	op := []string{"+", "-", "^", "|", "&", "*"}[g.pick(6)]
	upd := &arNode{kind: 1, op: op, a: lv, b: x, src: fmt.Sprintf("(%s %s %s)", lv.src, op, x.src)}
	upd.join2(lv, x)
	addI := g.chance(0.5)
	inc := &arNode{kind: 1, op: "+", a: lv, b: iv, src: fmt.Sprintf("(%s + %s)", lv.src, iv.src)}
	inc.join2(lv, iv)
	// 代入先 (lK の型) より広い計算や符号の混ざった計算は作らない (暗黙の狭め・F6 のエラー)
	for _, u := range []*arNode{upd, inc} {
		if u.regionTy(t.w).w > t.w || arMixed(u, t.w) || arTooWide(u) {
			return
		}
	}
	n := 1 + g.pick(5)
	lit := fmt.Sprint(cur)
	if cur < 0 {
		lit = fmt.Sprintf("(%s as %s)", lit, t.name())
	}
	fmt.Fprintf(body, "\tvar %s:%s = %s;\n", lv.src, t.name(), lit)
	fmt.Fprintf(body, "\tfor (var %s:u8 = 0; %s < %d; %s += 1) {\n", iv.src, iv.src, n, iv.src)
	fmt.Fprintf(body, "\t\t%s = %s;\n", lv.src, upd.src)
	if addI {
		fmt.Fprintf(body, "\t\t%s = %s;\n", lv.src, inc.src)
	}
	fmt.Fprintf(body, "\t}\n\t@printf(\"{}\\n\", %s);\n", lv.src)
	for i := 0; i < n; i++ {
		lv.val = cur
		_, v := arPlace(upd, t)
		cur = t.wrap(v)
		if addI {
			lv.val, iv.val = cur, int64(i)
			_, v = arPlace(inc, t)
			cur = t.wrap(v)
		}
	}
	g.want = append(g.want, fmt.Sprint(cur))
}

// arRender は式を文字列にする (constForm なら型つき定数の形)。
func arRender(x *arNode, constForm bool) string {
	if constForm {
		return x.srcConst
	}
	return x.src
}

func TestRandomArithV4(t *testing.T) {
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
			g := &arGen{r: rand.New(rand.NewSource(seed)), limit: 12}
			files, want := g.program()
			if g.nstmt == 0 {
				t.Skip("式が作れなかった")
			}
			for _, level := range []int{-1, 0} {
				out, err := rpRun(t, files, level, rpMaxCycles)
				if err != nil {
					if s, big := rpTooBig(err.Error()); big {
						t.Skipf("プログラムが大きすぎる (seed %d): %s", seed, s)
					}
				}
				if err != nil || out != want {
					t.Fatalf("level %d (seed %d): got\n%s\nerr %v\nwant\n%s\n%s", level, seed, out, err, want, joinSources(files))
				}
			}
			if *randInterp {
				if out, ok, err := rpInterp(t, files); ok && (err != nil || out != want) {
					t.Fatalf("interp (seed %d): got\n%s\nerr %v\nwant\n%s\n%s", seed, out, err, want, joinSources(files))
				}
			}
		})
	}
}
