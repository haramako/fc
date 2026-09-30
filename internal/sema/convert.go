package sema

// 代入のような変換 (代入・初期化・引数・戻り値・struct / 配列の要素) の整数の規則 (Agent/wiki/plans/v4-plan.md §1.3)。
//   - E: 定数の値が代入先の型に収まらない (`var c:u8 = 300`、`return -1` (戻り値 u8)): fc 4 はエラー (明示の `as` で切り詰める)。
//     fc 3 は今までどおり黙って切り詰め、migrate には `c as T` を報告する
//   - D: 大きさが減る変換 (u16 / i16 → u8 / i8): fc 4 はエラー。fc 3 は下位バイト、migrate には `c as T`。同じ大きさで符号だけ
//     違う変換 (i8 → u8。`x = x + vx`) は、どの版も通す (ビットがそのまま)
// 演算の中の型の揃え方 (makeCompatible) と、コンパイラが作る変換 (ポインタのずれなど) は h.cast を直接使う。

import (
	"fmt"

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/syntax"
	"github.com/haramako/fc/internal/types"
)

// v4 はコンパイル中のモジュールが fc 4 以降か (整数の規則などの意味の変更。Agent/wiki/plans/v4-plan.md)。
func (h *Hlc) v4() bool { return h.version() >= syntax.Version4 }

// convert は代入のような変換で v を typ にする。c は値の式 (fc 3 → 4 の書き換えの位置。nil なら書き換えを作れない)。
func (h *Hlc) convert(v ir.Operand, typ *types.Type, c *cexpr) ir.Operand {
	return h.convertValue(v, typ, c, false)
}

// convertValue は convert の本体。checked なら E・D の判定は評価の前に済んでいる (preConvert)。
func (h *Hlc) convertValue(v ir.Operand, typ *types.Type, c *cexpr, checked bool) ir.Operand {
	vt := ir.ValType(v)
	if typ.Kind == types.Int && typ.Enum == nil && (vt.Kind == types.Int || vt.Kind == types.Bool) && vt.Enum == nil && typ.Size <= 2 {
		if typ.Size > vt.Size {
			v = h.widenArith(v, typ) // A1: 代入先が広ければ代入先の幅の値 (折り返した型付きの定数。widen.go)
			vt = ir.ValType(v)
		}
		k, isConst := ir.ValIntLiteral(v)
		kind := convRule(vt, k, isConst, typ)
		if !checked {
			h.convReport(kind, vt, k, typ, c)
		}
		if kind == convNarrowing {
			// fc 2 / fc 3 の暗黙の縮小は、明示の `x as T` (explicitCast) と同じ cast の形にする (値のままだと IR が違い、migrate が
			// 足す `as` で -O 2 の ROM が変わっていた: レジスタの割付が変わる。fuzz の TestRandomMigrate。cast の形のほうが最適化も効く)
			return ir.NewCastedValue(v, typ, 0)
		}
	}
	return h.cast(v, typ)
}

// convKind は代入のような変換の整数の規則の判定。
type convKind int

const (
	convOK         convKind = iota
	convConstRange          // E: 定数の値が代入先の型に収まらない
	convNarrowing           // D: 大きさが減る変換 (同じ大きさで符号だけ違うのは通す)
)

// convRule は型 from (定数なら値 k) を代入のような変換で to にするときの E・D の判定 (IR を出さない。実行時の値の convert・
// 評価の前の preConvert・const の宣言の checkConstRange が使う)。
func convRule(from *types.Type, k int, isConst bool, to *types.Type) convKind {
	if to.Kind != types.Int || to.Enum != nil || to.Size > 2 || from.Kind != types.Int && from.Kind != types.Bool || from.Enum != nil {
		return convOK
	}
	if isConst {
		if lo, hi := intRange(to); k < lo || k > hi {
			return convConstRange
		}
		return convOK
	}
	if to.Size < from.Size {
		return convNarrowing
	}
	return convOK
}

// convReport は代入のような変換の判定 kind の診断: fc 4 はエラー、fc 3 の書き換えを集めているときは migrate に `c as T`。
func (h *Hlc) convReport(kind convKind, from *types.Type, k int, to *types.Type, c *cexpr) {
	switch kind {
	case convConstRange:
		switch {
		case h.v4():
			panic(&diag.Error{Msg: fmt.Sprintf("%d does not fit in %s (fc 4 does not truncate a constant implicitly; write `%d as %s` to truncate)", k, to, k, to)})
		case h.rewriting():
			h.rewriteAs("constant-range", c, to.String())
		}
	case convNarrowing:
		switch {
		case h.v4():
			panic(&diag.Error{Msg: fmt.Sprintf("cannot convert %s to %s implicitly (narrowing; write `x as %s`)", from, to, to)})
		case h.rewriting():
			h.rewriteAs("narrowing", c, to.String())
		}
	}
}

// checkConstRange は const の型の注釈に値が収まるかを見る (E)。fc 3 は注釈が値の型に負ける (`const B:u8 = 300` は u16 の 300、
// `const F:u8 = -1` は i8)。fc 4 はエラー。fc 3 の migrate は注釈を今の型 t に直す (注釈が符号付きで、同じ大きさの符号なしの値が
// 収まらないときは t も注釈のままなので、値を `v as T` に)。
func (h *Hlc) checkConstRange(name string, typ syntax.TypeExpr, declType *types.Type, v *ir.Value, t *types.Type, val *cexpr) {
	if declType == nil || v.Kind != ir.KindLiteral || !v.IsInt || convRule(v.Type, v.Int, true, declType) != convConstRange {
		return
	}
	switch {
	case h.v4():
		panic(&diag.Error{Msg: fmt.Sprintf("const %s: %d does not fit in %s (write the type of the value, or `%d as %s` to truncate)", name, v.Int, declType, v.Int, declType)})
	case h.rewriting():
		if t != declType && typ != nil && typ.Pos().IsValid() && typ.End().IsValid() {
			h.addRewrite("constant-range", typ.Pos().Offset, typ.End().Offset, t.String())
		} else {
			h.rewriteAs("constant-range", val, declType.String())
		}
	}
}

// shiftByLeft はシフト `left << right` / `>>` の結果を左辺の型にするか (F1。Agent/wiki/plans/v4-plan.md §1.3)。fc 2 / fc 3 は両辺の互換型
// (`x << n` (x:u8、n:u16) が u16)。fc 4 は左辺の型 (C・Go・Rust・Zig と同じ) で true を返す。fc 3 のモジュールで、互換型が
// 左辺の型と違うときは、migrate に左辺を `x as T` (T は互換型) にする書き換えを報告する (fc 4 でも同じ型になる)。
func (h *Hlc) shiftByLeft(e *cexpr, left, right ir.Operand) bool {
	lt, rt := ir.ValType(left), ir.ValType(right)
	if lt.Kind != types.Int || lt.Enum != nil || rt.Kind != types.Int && rt.Kind != types.Bool {
		return false
	}
	if h.v4() {
		return true // 左辺が型のない定数 (`1 << n`) なら、その値の型 (1 は u8。代入先が広ければ A1 で広がる)
	}
	if h.rewriting() {
		l, r := h.adaptLiteralNoErr(left, right, false)
		if ct := h.prog.Types.Compatible(ir.ValType(l), ir.ValType(r)); ct != nil && ct != lt && len(e.args) == 2 {
			h.rewriteAs("shift-type", e.args[0], ct.String())
		}
	}
	return false
}
