package sema

// 型を決める段の「計画」: 演算の節点ごとに、結果の型・項の変換 (型のない定数を相手に合わせる・互換型に揃える)・型の誤りの
// 診断を、項の型 (exprInfo) だけから決める。型を決める段 (typing.go の opType) はこの計画の結果の型を節点の型にし、lval は
// 項を評価したあと同じ計画を受け取って、その変換のとおりに IR を出すだけにする (以前は lval が値から adaptLiteral /
// tryMakeCompatible で型を出し直し、型を決める段は literalAdapted / Compatible で同じ判断を別に書いていた)。
//
// lval に渡す項の型は、型を決める段が決めた型 (planInfo)。決められなかった項 (型を決める段の外の形) だけ評価した値の型から
// 作る。どちらでも同じ計画の関数を通るので、型の規則と診断はここ 1 か所にある。

import (
	"fmt"

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/syntax"
	"github.com/haramako/fc/internal/types"
)

// planInfo は lval が評価した項 c (値 v) の型: 型を決める段が決めた型。決められなければ値の型 (型のない定数は値も)。
func (h *Hlc) planInfo(c *cexpr, v ir.Operand) exprInfo {
	vi := valueInfo(v)
	info, ok := h.exprType(c)
	if !ok {
		return vi
	}
	if h.prog.Config.VerifyIR() && (info.untyped != vi.untyped || info.untyped && info.n != vi.n) {
		panic(&diag.Error{Msg: fmt.Sprintf("internal: the typing stage says %s is %+v but lval made %v (sema/typeplan.go)", h.exprText(c), info, v)})
	}
	return info
}

// valueInfo は評価した値 v の exprInfo。
func valueInfo(v ir.Operand) exprInfo {
	if lv, ok := v.(*ir.Value); ok && lv.Kind == ir.KindLiteral && lv.IsInt && lv.Untyped {
		return exprInfo{t: lv.Type, untyped: true, n: lv.Int, name: lv.Name}
	}
	return exprInfo{t: ir.ValType(v)}
}

// adaptedLit は literalRule で相手に合わせた後の項 (型のない定数の値を作り直すときは changed)。
type adaptedLit struct {
	t       *types.Type // 合わせた後の型 (互換型を決める型)
	changed bool        // 値を t の定数に作り直す (切り詰め・符号付きの 16 ビット)。false なら項はそのまま
	n       int         // 作り直した定数の値
}

// adaptLit は二項演算の項 x を相手 other に合わせる (adaptLiteral と同じ判断。cmp は比較で、収まらなければエラー)。
func (h *Hlc) adaptLit(x, other exprInfo, cmp bool) adaptedLit {
	if !x.untyped {
		return adaptedLit{t: x.t}
	}
	t := other.t
	switch literalRule(x.t, x.n, other.untyped, t, cmp) {
	case litCmpError:
		panic(&diag.Error{Msg: fmt.Sprintf("%s does not fit in %s, the type of the other operand (a constant in a comparison takes that type; convert the other operand with `as` to compare in another type)", describeInt(x), t)})
	case litSigned16:
		return adaptedLit{t: h.prog.Types.IntType(2, true), changed: true, n: x.n}
	case litTruncate:
		_, hi := intRange(t)
		n := ir.FloorMod(x.n, 1<<(8*t.Size))
		if n > hi {
			n -= 1 << (8 * t.Size)
		}
		return adaptedLit{t: t, changed: true, n: n}
	case litFits:
		return adaptedLit{t: t} // 値はそのまま (互換型が相手の型になる)
	}
	return adaptedLit{t: x.t}
}

// describeInt は型のない定数の診断の書き方 (describe と同じ: 名前付きの定数は名前、ほかは値)。
func describeInt(x exprInfo) string {
	return describe(ir.NewIntLiteral(x.name, x.t, x.n))
}

// apply は lval の項の値 v に合わせた結果を当てる (作り直す定数か、そのまま)。
func (a adaptedLit) apply(v ir.Operand) ir.Operand {
	if a.changed {
		return ir.NewIntLiteral("", a.t, a.n)
	}
	return v
}

// arithKind は二項の算術の計算の仕方。
type arithKind int

const (
	arithCompat  arithKind = iota // 互換型 t に揃えて計算する (型のない定数は相手に合わせてから)
	arithShift                    // F1 (fc 4): シフトの結果は左辺の型、量は揃えない
	arithPtrDiff                  // p - q: 要素数 (u16)
	arithPtrAdd                   // p + i / p - i: 要素の大きさの倍を足す (t はポインタの型)
)

// arithPlan は二項の算術 (+ - * / % & | ^ << >>) の計画。
type arithPlan struct {
	kind arithKind
	t    *types.Type   // 結果の型
	args [2]adaptedLit // arithCompat の項 (相手に合わせた後)
}

// planArith は二項の算術 op の項の型 a, b から計画を決める (型の誤りは診断の panic)。lval の A1 の経路 (広い幅で計算する) も
// 型の検査はここを通る。
func (h *Hlc) planArith(op cop, a, b exprInfo) arithPlan {
	lt, rt := a.t, b.t
	if lt.IsFarFunc() || rt.IsFarFunc() {
		panic(&diag.Error{Msg: "arithmetic is not supported on farfn"})
	}
	checkEnumOp(op, lt, rt)
	checkOperandKinds(op, lt, rt)
	if op == opSub && lt.Kind == types.Pointer && rt.Kind == types.Pointer && lt.Base == rt.Base {
		return arithPlan{kind: arithPtrDiff, t: h.prog.Types.IntType(2, false)} // p - q は要素数 (C と同じ)
	}
	isInt := func(t *types.Type) bool { return t.Kind == types.Int && t.Enum == nil }
	if (op == opShiftLeft || op == opShiftRight) && h.v4() && isInt(lt) && (rt.Kind == types.Int || rt.Kind == types.Bool) {
		return arithPlan{kind: arithShift, t: lt} // 左辺が型のない定数 (`1 << n`) なら、その値の型 (代入先が広ければ A1 で広がる)
	}
	p := arithPlan{kind: arithCompat, args: [2]adaptedLit{h.adaptLit(a, b, false), h.adaptLit(b, a, false)}}
	at, bt := p.args[0].t, p.args[1].t
	if p.t = h.prog.Types.CommonType(at, bt); p.t != nil {
		return p
	}
	if (op == opAdd || op == opSub) && (at.Kind == types.Pointer || at.Kind == types.SoaRef) && bt.Kind == types.Int {
		if at.Kind == types.Pointer && at.Base.Kind == types.Void {
			panic(&diag.Error{Msg: "no arithmetic on *void"})
		}
		return arithPlan{kind: arithPtrAdd, t: at, args: p.args}
	}
	panic(&diag.Error{Msg: fmt.Sprintf("cannot apply %s to %s and %s (not compatible types)", opSymbol(op), at, bt)})
}

// cmpPlan は比較 (== と <。!= > <= >= はこれに直す) の計画。
type cmpPlan struct {
	mixed bool          // 符号の違う整数の大小の比較で、互換型が片方の値を読み替える (F6。fc 4 はエラー、fc 3 は書き換え)
	args  [2]adaptedLit // 型のない定数を比較の規則で相手に合わせた後の項
}

// planCompare は比較 op の項の型から計画を決める (enum・F6・収まらない定数の誤りは診断の panic)。a, b は A1 で広げる前の型
// (F6 を見る: `id_i16(100) >= (id_u8(255) << 3)` は i16 と u8 で、読み替えは起きない)、wa, wb は計算する幅の型 (compareWidth
// で広げた項は広い型。型のない定数を合わせる相手)。互換型と項の種類の検査は、16 ビットを超える定数の比較を畳んだ後なので
// compareType。
func (h *Hlc) planCompare(op cop, a, b, wa, wb exprInfo) cmpPlan {
	checkEnumOp(op, wa.t, wb.t)
	// F6 (fc 4): 符号の違う整数の大小の比較で、互換型が片方の値を読み替えるものはエラー (intrules.go)
	p := cmpPlan{mixed: op == opLt && mixedSignInfo(a, b)}
	if p.mixed && h.v4() {
		panic(h.mixedSignError(a.t, b.t))
	}
	p.args = [2]adaptedLit{h.adaptLit(wa, wb, true), h.adaptLit(wb, wa, true)}
	return p
}

// wideInfo は lval が評価した項 c (値 v) の、計算した幅の型 (A1 で広い幅で計算した値は広い型、ほかは planInfo)。
func (h *Hlc) wideInfo(c *cexpr, v ir.Operand) exprInfo {
	if h.widened(c, v) {
		return valueInfo(v)
	}
	return h.planInfo(c, v)
}

// compareType は比較の項を揃える互換型 (lt, rt は相手に合わせた後の項の型。lval の farfn の読み替えの後)。順序の比較は項の
// 種類 (struct・配列・ポインタの演算) と farfn も検査する。*void との == は *void を左に置く (swap)。
func (h *Hlc) compareType(op cop, lt, rt *types.Type) (t *types.Type, swap bool) {
	if op == opLt {
		checkOperandKinds(op, lt, rt) // == / != は struct・配列でもよい (バイトの比較)
		if lt.IsFarFunc() || rt.IsFarFunc() {
			panic(&diag.Error{Msg: "ordered comparison is not supported on farfn"})
		}
	}
	if op == opEq && isVoidPtr(rt) && !isVoidPtr(lt) {
		lt, rt, swap = rt, lt, true // *void を左に (CommonType は対称だが、項の順を 2026-10-05 より前と同じに保つ)
	}
	return h.compatible(lt, rt), swap
}

// mixedSignInfo は mixedSign の型を決める段の版 (型のない定数は相手の型になるので混ざらない)。
func mixedSignInfo(a, b exprInfo) bool {
	lt, rt := a.t, b.t
	if lt.Kind != types.Int || rt.Kind != types.Int || lt.Enum != nil || rt.Enum != nil || lt.Signed == rt.Signed || a.untyped || b.untyped {
		return false
	}
	u, s := lt, rt
	if lt.Signed {
		u, s = rt, lt
	}
	return u.Size >= s.Size // 符号なしが狭ければ、符号付きの型に全部の値が入る
}

// valType は合わせた後の項の値の型 (作り直さない定数は元の型のまま: 互換型は相手の型になる)。
func (a adaptedLit) valType(orig exprInfo) *types.Type {
	if a.changed {
		return a.t
	}
	return orig.t
}

// planUnary は単項の `! - ~` の項の型 a から結果の型を決める (型の誤りは診断の panic)。
func (h *Hlc) planUnary(op cop, a exprInfo) *types.Type {
	checkEnumOp(op, a.t, nil)
	checkOperandKinds(op, a.t, nil)
	if a.t.IsFarFunc() && op != opNot {
		panic(&diag.Error{Msg: "arithmetic is not supported on farfn"})
	}
	if op == opNot {
		return h.prog.Types.Bool() // `!x` は 0 / 1
	}
	return a.t
}

// planMinMax は @min / @max / @clamp の引数の型から、揃える型と各引数の合わせ方を決める (型の誤りは診断の panic)。最初の引数を
// 各引数と順に比較の規則で合わせ (作り直した最初の引数が次の相手になる)、全部の値の型の互換型。
func (h *Hlc) planMinMax(op cop, infos []exprInfo) (*types.Type, []adaptedLit) {
	args := make([]adaptedLit, len(infos))
	cur := infos[0]
	args[0] = adaptedLit{t: cur.t}
	for i := 1; i < len(infos); i++ {
		// 比較なので、型のない定数は比較と同じ規則 (`@min(s, 200)` (s:i8) は 200 が収まらずエラー。-56 と比べていた)
		a0, ai := h.adaptLit(cur, infos[i], true), h.adaptLit(infos[i], cur, true)
		if a0.changed {
			args[0], cur = a0, exprInfo{t: a0.t}
		}
		args[i] = ai
	}
	typ := cur.t
	for i := 1; i < len(infos); i++ {
		typ = h.compatible(typ, args[i].valType(infos[i]))
	}
	if typ.Kind != types.Int && typ.Kind != types.Bool {
		panic(&diag.Error{Msg: fmt.Sprintf("%s: arguments must be integers (got %s)", op, typ)})
	}
	return typ, args
}

// planIndex は a[i] の要素の型 (配列・ポインタの要素、slice は要素。soa は soaIndex)。idx は添字の型 (分からなければ nil)。
// desc は診断で a を指す書き方 (lval が評価した値から作る。型を決める段では診断は外に出ないので使われない)。
func (h *Hlc) planIndex(a exprInfo, idx *exprInfo, desc func() string) *types.Type {
	t := a.t
	if t.IsSlice() {
		t = h.prog.Types.PointerTo(t.SliceOf) // s[i] は s.ptr[i] (範囲の検査はしない)
	}
	if t.Kind != types.Pointer && t.Kind != types.Array {
		panic(&diag.Error{Msg: fmt.Sprintf("cannot index %s (type %s is not a pointer or array)", desc(), t)})
	}
	if t.Base.Kind == types.Void {
		panic(&diag.Error{Msg: "cannot index *void (bitcast to a typed pointer first)"})
	}
	if idx != nil && idx.t.Kind != types.Int && idx.t.Kind != types.Bool {
		panic(&diag.Error{Msg: fmt.Sprintf("index must be an integer (got %s)", idx.t)})
	}
	return t.Base
}

// planDeref は `*p` の値の型 (ポインタの先。soa の要素のハンドルはそのまま)。
func (h *Hlc) planDeref(a exprInfo, desc func() string) *types.Type {
	t := a.t
	if t.Kind == types.Pointer && t.Base.Kind == types.Void {
		panic(&diag.Error{Msg: "cannot dereference *void (bitcast to a typed pointer first)"})
	}
	switch t.Kind {
	case types.Pointer:
		return t.Base
	case types.SoaRef:
		return t // `*Points[i]` は要素そのもの
	}
	panic(&diag.Error{Msg: fmt.Sprintf("cannot dereference %s (type %s is not a pointer)", desc(), t)})
}

// planField は struct の値かポインタ (型 t) のフィールド name (viaPtr はポインタ経由で、自動で参照はがし)。soa の要素のフィールドは
// soaField。
func (h *Hlc) planField(t *types.Type, name string, desc func() string) (f types.Field, viaPtr bool) {
	switch {
	case t.Kind == types.Pointer && t.Base.Kind == types.Struct:
		return h.fieldOf(t.Base, name), true
	case t.Kind == types.Struct:
		return h.fieldOf(t, name), false
	}
	panic(&diag.Error{Msg: fmt.Sprintf("cannot access field %s: %s is not a struct (type %s)", name, desc(), t)})
}

// noDesc は型を決める段が計画を呼ぶときの desc (診断は外に出ない)。
func noDesc() string { return "" }

// planCast は明示の変換 (`x as T`・`@bitcast(T, x)`・v1 の `<T>x`) の型の検査 (checkCast の種類ごとの規則と、`as` で const を
// 外さない)。
func (h *Hlc) planCast(kind syntax.CastKind, from, to *types.Type) {
	h.checkCast(kind, from, to)
	if kind == syntax.CastAs && from.Kind == types.Pointer && from.ReadOnly && to.Kind == types.Pointer && !to.ReadOnly {
		panic(&diag.Error{Msg: fmt.Sprintf("cannot drop const with `as` (%s to %s); use @bitcast(%s, x)", from, to, to)})
	}
}

// planCall は関数の呼び出しの結果の型 (void の関数は Void)。f は呼ぶ値の型 (マクロは expandMacro)。
func (h *Hlc) planCall(f exprInfo, desc func() string) *types.Type {
	if f.t.Kind != types.Func || f.t.Base == nil {
		// 関数でない値の呼び出し (名前が同じ変数に取られて関数の宣言がエラーになったときなど)。以前は lmdType.Base (nil) を
		// 見てコンパイラが panic していた (fuzz の生成器の名前の衝突で発覚)
		panic(&diag.Error{Msg: fmt.Sprintf("cannot call %s: type %s is not a function", desc(), f.t)})
	}
	return f.t.Base
}

// slicePlan は配列・slice の値から slice を作るとき (a[lo..hi]・slice への変換・@len・for-each など) の計画: 要素の型・長さの幅・
// 定数の長さ (slice は -1)。
type slicePlan struct {
	elem *types.Type
	n    int
	wide bool
}

// planSliceOf は配列・slice (型 a) の slicePlan。str は文字列リテラル (長さに終端の 0 を含めない)。配列・slice でなければ、
// または配列の長さが分からなければ診断の panic (what は診断の主語、desc は値の書き方)。
func (h *Hlc) planSliceOf(a exprInfo, str bool, what string, desc func() string) slicePlan {
	t := a.t
	switch {
	case a.untyped:
	case t.IsSlice():
		return slicePlan{elem: t.SliceOf, n: -1, wide: t.IsWideSlice()}
	case t.Kind == types.Array:
		n := t.Length
		if n < 0 {
			panic(&diag.Error{Msg: fmt.Sprintf("%s: the length of %s is not known (make the slice with @slice(p, n))", what, t)})
		}
		if str && n > 0 {
			n--
		}
		return slicePlan{elem: t.Base, n: n, wide: n > maxSliceLen(false)}
	case t.Kind == types.SoaRef:
		t = t.Base // soa の要素は値として読むと struct
	}
	panic(&diag.Error{Msg: fmt.Sprintf("%s: %s is not an array or a slice (type %s)", what, desc(), t)})
}

// planSliceBound は a[lo..hi] の lo / hi (型 x。which は "lo" / "hi") の検査 (整数でなければ診断の panic)。
func planSliceBound(x exprInfo, which string) {
	if t := x.t; t.Kind != types.Int && t.Kind != types.Bool || t.Enum != nil {
		panic(&diag.Error{Msg: fmt.Sprintf("a[lo..hi]: %s must be an integer (got %s)", which, t)})
	}
}

// sliceType は slicePlan の slice の型 (読み取り専用かは型でなく値の印: markReadOnly)。
func (h *Hlc) sliceType(p slicePlan) *types.Type {
	return h.prog.Types.Slice(p.elem, false, p.wide)
}

// toSliceKind は slice への変換 (toSlice) の仕方。
type toSliceKind int

const (
	toSliceAsIs  toSliceKind = iota // 変換しない (配列・slice でない、同じ幅の slice、要素の違う slice: 代入側が検査する)
	toSliceBuild                    // 配列から slice を作る
	toSliceWiden                    // 普通の slice から広い slice を作り直す (長さを広げる)
)

// planToSlice は配列・slice (型 a) を slice の型 st にする変換の計画 (変換した型と仕方)。変換できない形 (広い → 普通、要素の
// 違う配列、長すぎる配列) は診断の panic。str は文字列リテラル、desc は値の書き方。
func (h *Hlc) planToSlice(a exprInfo, st *types.Type, str bool, desc func() string) (*types.Type, toSliceKind) {
	t := a.t
	switch {
	case a.untyped || t.Kind != types.Array && !t.IsSlice():
		return t, toSliceAsIs // 型の検査は代入側がする
	case t.IsSlice():
		switch {
		case t.SliceOf != st.SliceOf || t.IsWideSlice() == st.IsWideSlice():
			return t, toSliceAsIs // 同じ幅 (と要素の違い) は代入側が検査する
		case st.IsWideSlice():
			return h.prog.Types.Slice(t.SliceOf, false, true), toSliceWiden
		}
		panic(&diag.Error{Msg: fmt.Sprintf("cannot use %s as %s implicitly (the length may not fit; use @slice(@ptr(s), n))", t, st)})
	}
	p := h.planSliceOf(a, str, "slice", desc)
	if p.elem != st.SliceOf {
		panic(&diag.Error{Msg: fmt.Sprintf("cannot use %s as %s (element types differ)", t, st)})
	}
	if max := maxSliceLen(st.IsWideSlice()); p.n > max {
		hint := ""
		if !st.IsWideSlice() {
			hint = "; use [:u16]" + st.SliceOf.String() + " for longer ones"
		}
		panic(&diag.Error{Msg: fmt.Sprintf("cannot use %s as %s: the slice has at most %d elements%s", t, st, max, hint)})
	}
	return h.prog.Types.Slice(st.SliceOf, false, st.IsWideSlice()), toSliceBuild
}

// planStructLit は実行時に組み立てる struct リテラルの型 (型名が無く文脈の型も無ければ診断の panic)。
func planStructLit(ty *types.Type) *types.Type {
	if ty == nil {
		panic(&diag.Error{Msg: "struct literal without a type name needs a context that gives the type (declared type or assignment)"})
	}
	return ty
}

// planRuntimeArray は実行時に組み立てる配列リテラル e の型 (文脈の配列型があればその要素と長さ、無ければ要素の型から先に決めた
// 要素の型: preArrayBase)。要素の型を評価の前に決められなければ nil (lval は評価した値から決める: fc 3)。文脈の型より要素が
// 多ければ診断の panic。
func (h *Hlc) planRuntimeArray(e *cexpr) *types.Type {
	n := len(e.args)
	if e.ty != nil && e.ty.Kind == types.Array {
		if e.ty.Length > n {
			n = e.ty.Length
		} else if e.ty.Length >= 0 && e.ty.Length < n {
			panic(&diag.Error{Msg: fmt.Sprintf("%d elements given for %s", len(e.args), e.ty)})
		}
		return h.prog.Types.ArrayOf(e.ty.Base, n)
	}
	base := h.preArrayBase(e.args)
	if base == nil {
		return nil
	}
	return h.prog.Types.ArrayOf(base, n)
}

// planSoaIndex は soa のコンテナ soa の添字 (型 idx。lit は定数) の検査と、要素のハンドルの型。
func (h *Hlc) planSoaIndex(soa *types.Type, idx exprInfo, lit bool) *types.Type {
	if idx.t.Kind != types.Int {
		panic(&diag.Error{Msg: fmt.Sprintf("index must be an integer (got %s)", idx.t)})
	}
	if !lit && idx.t.Size != 1 {
		panic(&diag.Error{Msg: fmt.Sprintf("soa %s: index must be 1 byte", shortName(soa.Name))})
	}
	// 宣言がエラーだと soa.Base は nil のまま (fuzz で発覚)。soaElement ならエラーにできる
	return h.prog.Types.SoaRef(soa, h.soaElement(soa), "")
}

// planSoaField は soa の要素のハンドル (型 t) のフィールド name の型 (入れ子の struct はハンドル)。
func (h *Hlc) planSoaField(t *types.Type, name string) *types.Type {
	f := h.fieldOf(t.Base, name)
	if f.Type.Kind == types.Struct {
		return h.prog.Types.SoaRef(t.Soa, f.Type, t.Path+name+"_")
	}
	return f.Type
}

// planAssignTarget は代入の左辺 lhs (評価済みの節点) の形の検査 (関数の呼び出しの結果には代入できない)。
func planAssignTarget(lhs *cexpr) {
	if lhs.kind == cOp && lhs.op == opCall {
		// 呼び出しの結果は一時変数なので、そのまま進むと `g() = 0` が黙って通り、void なら nil 参照で落ちる (fuzz で発覚)
		panic(&diag.Error{Msg: "cannot assign to the result of a function call"})
	}
}
