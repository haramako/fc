package sema

import (
	"fmt"

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/types"
)

// adaptLiteral は、片方が型のない整数定数 (ir.Value.Untyped: リテラル・型を書かない const・sizeof など) の二項演算・比較で、
// 定数を相手の整数型に合わせる (doc/v3_plan.md §10.2。Go・Rust と同じ考え方)。
//
// 値が相手の型に収まるときは、今の互換型 (大きいほう、同じ大きさなら符号付き) も相手の型になるので何もしない
// (`vx < 0` (vx:i8) は i8、`w + 5` (w:u16) は u16)。定数のほうが大きい型のとき (`x * 300`、`0x2000 + y * 32`) も今までどおり
// 広いほうで計算する。変わるのは、大きさは相手以下で符号のために収まらないときだけ:
//   - 演算 (cmp == false): 相手の型に切り詰める。`x + -1` (x:u8) は u8 の x + 255 (以前は i8 になっていた)。
//     `x - 128` (x:i8) のように相手が符号付きなら、以前から相手の型なので結果は同じ
//   - 比較 (cmp == true): エラー。`s < 200` (s:i8) は 200 が -56 に、`x == -1` (x:u8) は x == 255 になっていた
func (h *Hlc) adaptLiteral(a, b ir.Operand, cmp bool) (ir.Operand, ir.Operand) {
	for k := 0; k < 2; k++ {
		lit, other := a, b
		if k == 1 {
			lit, other = b, a
		}
		v, ok := lit.(*ir.Value)
		if !ok || v.Kind != ir.KindLiteral || !v.IsInt || !v.Untyped {
			continue
		}
		if ov, ok := other.(*ir.Value); ok && ov.Kind == ir.KindLiteral && ov.Untyped {
			continue // 定数同士 (普通は畳み込まれている)
		}
		t := ir.ValType(other)
		if t.Kind != types.Int || t.Enum != nil || t.Size < 1 || t.Size > 2 {
			continue
		}
		if v.Type.Size > t.Size || v.Int < -32768 || v.Int > 65535 {
			// 定数のほうが大きい型: 今までどおり広げる (16 ビットに収まらない定数の比較は foldBeyond16 が畳む)。
			// 比較で相手が符号付きなら、符号付きの広い型にする (`s < 300` (s:i8) が u16 で比べて -10 < 300 が偽だった)
			if cmp && t.Signed && !v.Type.Signed && v.Int <= 32767 {
				r := ir.NewIntLiteral("", h.prog.Types.IntType(2, true), v.Int)
				if k == 0 {
					a = r
				} else {
					b = r
				}
			}
			continue
		}
		lo, hi := intRange(t)
		if v.Int >= lo && v.Int <= hi {
			continue
		}
		if cmp {
			panic(&diag.Error{Msg: fmt.Sprintf("%s does not fit in %s, the type of the other operand (a constant in a comparison takes that type; convert the other operand with `as` to compare in another type)", describe(v), t)})
		}
		if t.Signed {
			continue // 互換型が既に相手の型 (同じ大きさなら符号付きが勝つ)。値もそのままでよい (IR を以前と同じに保つ)
		}
		n := ir.FloorMod(v.Int, 1<<(8*t.Size))
		if n > hi {
			n -= 1 << (8 * t.Size)
		}
		r := ir.NewIntLiteral("", t, n)
		if k == 0 {
			a = r
		} else {
			b = r
		}
	}
	return a, b
}

// intRange は整数型 t (1〜2 バイト) の値の範囲。
func intRange(t *types.Type) (lo, hi int) {
	hi = 1<<(8*t.Size) - 1
	if t.Signed {
		lo, hi = -(hi+1)/2, (hi+1)/2-1
	}
	return lo, hi
}

// foldBeyond16 は、16 ビットに収まらない定数と整数の値の比較を畳む (結果は値によらず決まる)。広げる先の型が無く、
// 定数を 16 ビットに切り詰めて比べていた (`w == 65537` (w = 1) が真。警告は「決して等しくない」と出ていた)。
func (h *Hlc) foldBeyond16(op cop, left, right ir.Operand) (int, bool) {
	beyond := func(v ir.Operand) (int, bool) {
		k, ok := ir.ValIntLiteral(v)
		return k, ok && (k < -32768 || k > 65535)
	}
	isInt := func(v ir.Operand) bool {
		t := ir.ValType(v)
		return t.Kind == types.Int && t.Enum == nil
	}
	// 相手の値: リテラルなら自分の型で読んだ値 (両方とも定数のときも数学の値で比べる)、変数なら型の範囲のどこか
	cmp := func(a, b int) int {
		switch {
		case op == opEq && a == b, op == opLt && a < b:
			return 1
		}
		return 0
	}
	if k, ok := beyond(right); ok && isInt(left) {
		if n, lit := ir.ValIntLiteral(left); lit {
			return cmp(wrapInt(n, ir.ValType(left)), k), true
		}
		if op == opEq {
			return 0, true
		}
		return b2i(k > 0), true // v < k
	}
	if k, ok := beyond(left); ok && isInt(right) {
		if n, lit := ir.ValIntLiteral(right); lit {
			return cmp(k, wrapInt(n, ir.ValType(right))), true
		}
		if op == opEq {
			return 0, true
		}
		return b2i(k < 0), true // k < v
	}
	return 0, false
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// adaptLiteralNoErr は adaptLiteral と同じだが、比較で収まらない定数のエラーを出さずにそのまま返す (定数の畳み込みの型を
// 決めるため。エラーは実行時の式を作る側 (lval) が出す)。
func (h *Hlc) adaptLiteralNoErr(a, b ir.Operand, cmp bool) (ra, rb ir.Operand) {
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(*diag.Error); !ok {
				panic(r)
			}
			ra, rb = a, b
		}
	}()
	return h.adaptLiteral(a, b, cmp)
}
