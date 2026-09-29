package sema

// 名前付きの文字列定数の長さ (Agent/wiki/plans/v4-plan.md §2)。
//
// 文字列リテラルは長さ (@len・slice への変換・for-each・`a[lo..]` の終わり) に終端の 0 を含めない (データには残る)。fc 3 までの
// 名前付きの文字列定数 (`const NM = "joe"`) は普通の配列の定数と同じで 0 を含めた長さ 4 になっていた (互換性のため後回しにした
// 項目)。fc 4 では、長さを初期値の文字列から決めた名前付きの配列定数 (ir.Value.StrConst: 型を書かない、`[?]u8`、ポインタ型
// `*u8` の宣言) をリテラルと同じ長さにする。規則は使う側のモジュールの版で決まる。
//
// fc 3 → 4 の migrate: fc 3 のモジュールで名前付きの文字列定数の長さを見る所があれば、その宣言を長さつき (`const NM:[4]u8 =
// "joe"`) に書き換える。長さを書いた配列には印を付けないので、fc 4 でも今の長さのままで、使う所の IR も変わらない。

import (
	"fmt"

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
		return true
	case !v.StrConst:
		return false
	case h.v4():
		return true
	case h.rewriting():
		h.rewriteStrConstDecl(v)
	}
	return false
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
