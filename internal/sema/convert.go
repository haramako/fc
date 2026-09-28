package sema

// 代入のような変換 (代入・初期化・引数・戻り値・struct / 配列の要素) の整数の規則 (doc/v4_plan.md §1.3)。
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

// v4 はコンパイル中のモジュールが fc 4 以降か (整数の規則などの意味の変更。doc/v4_plan.md)。
func (h *Hlc) v4() bool { return h.version() >= syntax.Version4 }

// convert は代入のような変換で v を typ にする。c は値の式 (fc 3 → 4 の書き換えの位置。nil なら書き換えを作れない)。
func (h *Hlc) convert(v ir.Operand, typ *types.Type, c *cexpr) ir.Operand {
	vt := ir.ValType(v)
	if typ.Kind == types.Int && typ.Enum == nil && (vt.Kind == types.Int || vt.Kind == types.Bool) && vt.Enum == nil && typ.Size <= 2 {
		if k, ok := ir.ValIntLiteral(v); ok {
			if lo, hi := intRange(typ); k < lo || k > hi {
				switch {
				case h.v4():
					panic(&diag.Error{Msg: fmt.Sprintf("%d does not fit in %s (fc 4 does not truncate a constant implicitly; write `%d as %s` to truncate)", k, typ, k, typ)})
				case h.rewriting():
					h.rewriteAs("constant-range", c, typ.String())
				}
			}
		} else if typ.Size < vt.Size {
			switch {
			case h.v4():
				panic(&diag.Error{Msg: fmt.Sprintf("cannot convert %s to %s implicitly (narrowing; write `x as %s`)", vt, typ, typ)})
			case h.rewriting():
				h.rewriteAs("narrowing", c, typ.String())
			}
		}
	}
	return h.cast(v, typ)
}

// checkConstRange は const の型の注釈に値が収まるかを見る (E)。fc 3 は注釈が値の型に負ける (`const B:u8 = 300` は u16 の 300、
// `const F:u8 = -1` は i8)。fc 4 はエラー。fc 3 の migrate は注釈を今の型 t に直す (注釈が符号付きで、同じ大きさの符号なしの値が
// 収まらないときは t も注釈のままなので、値を `v as T` に)。
func (h *Hlc) checkConstRange(name string, typ syntax.TypeExpr, declType *types.Type, v *ir.Value, t *types.Type, val *cexpr) {
	if declType == nil || declType.Kind != types.Int || declType.Enum != nil || declType.Size > 2 || v.Kind != ir.KindLiteral || !v.IsInt {
		return
	}
	if lo, hi := intRange(declType); v.Int >= lo && v.Int <= hi {
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
