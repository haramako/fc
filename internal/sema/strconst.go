package sema

// 文字列の 0 終端と、名前付きの文字列定数の長さ (Agent/wiki/plans/v4-plan.md §2)。
//
// fc 4 の文字列リテラルは 0 終端にしない (`"abc"` は 3 バイトの配列。2026-09-30)。fc 3 までは終端の 0 を足し、長さには含めない
// (ir.Value.StrTerm)。0 終端の文字列が要るところ (`*const u8` で受ける関数) には `"abc\0"` と書く。fc 4 の 0 を含まない文字列を
// ポインタにするのはエラー (長さの分からないポインタで 0 を探して読み過ぎる。長さを別に渡すなら @ptr(s))。fc 3 → 4 の migrate は、
// ポインタにする文字列リテラルと、型を書かない配列の変数の初期値の文字列 (`var s = "abc"`: 0 を含めた 4 バイトの配列) に `\0` を
// 足して、データも長さも変えない。
//
// 文字列リテラルは長さ (@len・slice への変換・for-each・`a[lo..]` の終わり) に終端の 0 を含めない (データには残る)。fc 3 までの
// 名前付きの文字列定数 (`const NM = "joe"`) は普通の配列の定数と同じで 0 を含めた長さ 4 になっていた (互換性のため後回しにした
// 項目)。fc 4 では、長さを初期値の文字列から決めた名前付きの配列定数 (ir.Value.StrConst: 型を書かない、`[?]u8`、ポインタ型
// `*u8` の宣言) をリテラルと同じ長さにする。規則は使う側のモジュールの版で決まる。
//
// fc 3 → 4 の migrate: fc 3 の名前付きの文字列定数の宣言は長さつき (`const NM:[4]u8 = "joe"`: 余りは 0) に書き換える。長さを
// 書いた配列には印を付けないので、fc 4 でも今の長さ・データのままで、使う所の IR も変わらない。

import (
	"fmt"
	"strings"

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/syntax"
)

// strConstDecl は名前付きの文字列定数の宣言 (fc 3 → 4 の書き換えの位置)。
type strConstDecl struct {
	module  *ir.Module
	typ     syntax.TypeExpr // 書いてある型 (無ければ nil)
	nameEnd syntax.Pos      // 名前の終わり (型が無いとき `:[N]T` を足す位置)
}

// explicitLength は宣言の型が配列の長さを書いているか (`[4]u8`)。型が無い・`[?]T`・ポインタ型は長さを初期値から決める。
func explicitLength(typ syntax.TypeExpr) bool {
	at, ok := typ.(*syntax.ArrayType)
	return ok && at.Len != nil
}

// markStrConst は v を名前付きの文字列定数にする。
func (h *Hlc) markStrConst(v *ir.Value, typ syntax.TypeExpr, nameEnd syntax.Pos) {
	v.StrConst = true
	if h.prog.strConsts == nil {
		h.prog.strConsts = map[*ir.Value]*strConstDecl{}
	}
	h.prog.strConsts[v] = &strConstDecl{module: h.module, typ: typ, nameEnd: nameEnd}
}

// strLen は評価した値 v の長さに終端の 0 を含めないか: 文字列リテラル、または fc 4 のモジュールから見た名前付きの文字列定数。
// fc 3 のモジュールで書き換えを集めているとき、名前付きの文字列定数ならその宣言を長さつきに書き換える報告をする (ここは長さを
// 見る所からだけ呼ぶ)。
func (h *Hlc) strLen(v *ir.Value) bool {
	switch {
	case v == nil:
		return false
	case v.IsString:
		return v.StrTerm // fc 4 の文字列リテラルには 0 が無い (長さは配列の長さそのまま)
	case !v.StrConst:
		return false
	case h.v4():
		return h.strConstTerm(v)
	case h.rewriting():
		h.rewriteStrConstDecl(v)
	}
	return false
}

// strConstTerm は名前付きの文字列定数 v のデータの最後に終端の 0 があるか (初期値が fc 3 のモジュールの文字列リテラル)。
func (h *Hlc) strConstTerm(v *ir.Value) bool {
	init := h.prog.constArrays[v]
	return init == nil || init.StrTerm // 初期値が分からなければ今までどおり 0 がある扱い
}

// rewriteStrConstDecl は名前付きの文字列定数 v の宣言を長さつき (`[N]T`) に書き換える報告をする。
func (h *Hlc) rewriteStrConstDecl(v *ir.Value) {
	d := h.prog.strConsts[v]
	if d == nil {
		return
	}
	if d.module.Version != syntax.Version3 {
		// 宣言が fc 4 のモジュール (もう終端を含めない) にある: 使う所を手で直してもらう
		h.rewriteError("string-length", fmt.Sprintf("%s is a string constant declared in a fc 4 module; its length no longer counts the terminating 0 (write `@len(%s) + 1` or `%s[..@len(%s) + 1]` by hand if the 0 is needed)", v.Name, v.Name, v.Name, v.Name))
		return
	}
	text := fmt.Sprintf("[%d]%s", v.Type.Length, v.Type.Base)
	src := h.prog.Sources[d.module.Id]
	if src == nil {
		h.rewriteError("string-length", "the source of the module is unknown")
		return
	}
	outer := h.module
	h.module = d.module // 書き換えるのは宣言のあるモジュールのソース
	defer func() { h.module = outer }()
	if d.typ != nil && d.typ.Pos().IsValid() && d.typ.End().IsValid() {
		h.addRewrite("string-length", d.typ.Pos().Offset, d.typ.End().Offset, text)
	} else if d.nameEnd.IsValid() {
		h.addRewrite("string-length", d.nameEnd.Offset, d.nameEnd.Offset, ":"+text)
	} else {
		h.rewriteError("string-length", "the declaration has no position")
	}
}

// strLitSpan は fc 3 の文字列リテラルのソースの範囲。
type strLitSpan struct {
	module     *ir.Module
	start, end int
}

// noteStrLit は fc 3 → 4 の書き換えを集めているとき、文字列リテラル v の位置を覚える (c はリテラルの式)。
func (h *Hlc) noteStrLit(v *ir.Value, c *cexpr) {
	if h.rewriting() && c.pos.IsValid() && c.end.IsValid() {
		h.prog.strLitSpans[v] = strLitSpan{module: h.module, start: c.pos.Offset, end: c.end.Offset}
	}
}

// rewriteStrTerm は fc 3 の文字列リテラル lit を、終端の 0 を書いたリテラル (`"abc\0"`) にする書き換えを足す。
func (h *Hlc) rewriteStrTerm(lit *ir.Value) {
	sp, ok := h.prog.strLitSpans[lit]
	if !ok {
		return // ソースに無い文字列 (@run_tests が作る名前など。fc 4 では cstrZ が 0 を足す)
	}
	outer := h.module
	h.module = sp.module
	defer func() { h.module = outer }()
	h.addRewrite("string-terminator", sp.start, sp.end, syntax.QuoteString(lit.Str+"\x00"))
}

// strPtr は配列の値 v (文字列リテラル・名前付きの文字列定数かもしれない) をポインタにするときの検査: fc 4 の 0 を含まない文字列は
// エラー (途中の 0 でもよい: 0 終端の文字列を読む側は最初の 0 で止まる。`"ab\0cd"` を write_z に渡すと "ab")。fc 3 の文字列 (0 がある) は、書き換えを集めているなら fc 4 で 0 を書くように書き換える。
func (h *Hlc) strPtr(v ir.Operand) {
	g := ir.UnderlyingValue(v)
	if g == nil {
		return
	}
	if lit := h.prog.strLitGlobals[g]; lit != nil {
		g = lit
	}
	var str string
	switch {
	case g.IsString:
		if g.StrTerm {
			if h.rewriting() {
				h.rewriteStrTerm(g)
			}
			return
		}
		str = fmt.Sprintf("%q", g.Str)
		if strings.IndexByte(g.Str, 0) >= 0 {
			return
		}
	case g.StrConst:
		init := h.prog.constArrays[g]
		if init == nil || init.StrTerm {
			if init != nil && h.rewriting() {
				h.rewriteStrConstDecl(g)
			}
			return
		}
		str = "`" + g.Name + "`"
		if strings.IndexByte(init.Str, 0) >= 0 {
			return
		}
	default:
		return
	}
	panic(&diag.Error{Msg: fmt.Sprintf("string %s has no terminating 0 and cannot be used as a pointer (fc 4 strings are not 0-terminated; write \"...\\0\" for a 0-terminated string, or pass a slice / @ptr(s) with the length)", str)})
}

// cstrZ はコンパイラが作る 0 終端の文字列 s (write_z に渡す)。fc 4 の文字列リテラルは 0 終端にしないので 0 を書き足す。
func (h *Hlc) cstrZ(s string) *cexpr {
	if h.v4() {
		return cstr(s + "\x00")
	}
	return cstr(s)
}

// stringRows は配列リテラルの要素が全部文字列リテラルか (fc 4 で型を書かなければ slice の表にする)。
func stringRows(args []*cexpr) bool {
	for _, a := range args {
		if a.kind != cStr {
			return false
		}
	}
	return len(args) > 0
}

// rewriteStringRows は fc 3 の型を書かない文字列の配列 (`const T = ["ab", "cd"]` は `[2][3]u8`: 終端の 0 を含む行の 2 次元配列) の
// 宣言に、今の型を書き足す書き換えを足す (fc 4 では型を書かないと slice の表になる)。val は初期値の式、nameEnd は名前の終わり。
func (h *Hlc) rewriteStringRows(val *cexpr, nameEnd syntax.Pos) {
	if !h.rewriting() || val == nil || val.kind != cArray || !stringRows(val.args) {
		return
	}
	e := h.constEval(val)
	if e.kind != cValue || !nameEnd.IsValid() {
		h.rewriteError("string-rows", "cannot tell the type of the string array (write its type by hand)")
		return
	}
	h.addRewrite("string-rows", nameEnd.Offset, nameEnd.Offset, ":"+e.val.Type.String())
}
