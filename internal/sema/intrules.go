package sema

// fc 4 の整数の規則のうち、式の形を見る検査 (Agent/wiki/plans/v4-plan.md §1.3 F)。
//   - F2: 値が必ず 0 になるシフト (`hi << 8`、hi:u8。量が型のビット数以上の `<<` と符号なしの `>>`): fc 4 はエラー。A1 で
//     広げた後の型で見る (`var h:u16 = hi << 8 | lo` は 16 ビットで計算されるので通る)。fc 3 でも値は 0 なので、migrate は
//     書き換えられない (手で直してもらう)
//   - F4: 型を書かない配列リテラルの要素の型: fc 3 までは要素の型の互換型 (同じ大きさなら符号付きが勝つ) で、`[128, -1]` の
//     128 が -128 になる。fc 4 は定数の要素の値を黙って変えないよう、互換型以上の大きさで全部の定数が入る一番小さい型
//     (`[128, -1]` は i16)。fc 3 のモジュールでは、入らない要素に今の型の `as` を足す書き換えを報告する (`[128 as i8, -1]`)
//   - F6: 符号の違う整数どうしの大小の比較で、互換型が片方の値を読み替えるもの (同じ大きさ: 符号なしの 128 以上が負に、広い
//     符号なしと狭い符号付き: 負の値が大きな正の値に): fc 4 はエラー。fc 3 のモジュールでは互換型でない側に `as` を足す。
//     型のない定数は相手の型になるので数えない。`==` / `!=` は同じ大きさならビットの比較で、読み替えで結果が変わらないので対象外

import (
	"fmt"

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/syntax"
	"github.com/haramako/fc/internal/types"
)

// noteShift は量が定数のシフトの結果 tmp を、文の終わりの F2 の検査 (checkShifts) に控える (recordArith から)。
func (h *Hlc) noteShift(tmp *ir.Value, op *ir.Op) {
	if op.Code != ir.OpShiftLeft && op.Code != ir.OpShiftRight {
		return
	}
	if _, ok := ir.ValIntLiteral(op.Src[1]); ok {
		h.shifts = append(h.shifts, tmp)
	}
}

// checkShifts は文の中で控えたシフト (mark 以降) のうち、値が必ず 0 になるものを報告する (F2)。A1 で広げるのは文の中で
// 終わるので、文の終わりに見る。
func (h *Hlc) checkShifts(mark int) {
	if len(h.shifts) <= mark {
		return
	}
	list := append([]*ir.Value(nil), h.shifts[mark:]...)
	h.shifts = h.shifts[:mark]
	for _, tv := range list {
		n := h.arith[tv]
		if n == nil {
			continue
		}
		k, _ := ir.ValIntLiteral(n.op.Src[1])
		bits := 8 * tv.Type.Size
		if k < bits || (n.op.Code == ir.OpShiftRight && tv.Type.Signed) {
			continue
		}
		dir, hint := "left", "; write 0, or fix the shift amount"
		if n.op.Code == ir.OpShiftRight {
			dir = "right"
		} else if bits == 8 {
			hint = "; widen the left operand first (`x as u16 << 8`)"
		}
		msg := fmt.Sprintf("shifting %s %s by %d always gives 0 (the result of a shift has the type of its left operand, %d bits)%s", tv.Type, dir, k, bits, hint)
		if h.rewriting() {
			restore := h.enterExpr(n.c.pos)
			h.rewriteError("shift-zero", msg+"; it is 0 in fc 3 too, so fix it by hand")
			restore()
			continue
		}
		panic(&diag.Error{Msg: msg, Pos: syntax.At(h.module.Path, n.c.pos)})
	}
}

// arrayElemType は型を書かない配列リテラルの要素の型 (F4)。typ は要素の型の互換型 (fc 3 の型)、vals は要素の値、args は要素の
// 式 (書き換えの位置)。report が偽なら fc 3 のモジュールでも書き換えを報告しない (実行時に組み立てる配列は、要素の変換 convert が
// 範囲外の定数を E として報告する)。
func (h *Hlc) arrayElemType(typ *types.Type, vals []ir.Operand, args []*cexpr, report bool) *types.Type {
	if typ == nil || typ.Kind != types.Int || typ.Enum != nil || typ.Size > 2 || !(h.v4() || h.rewriting()) {
		return typ
	}
	tlo, thi := intRange(typ)
	lo, hi := 0, 0
	var bad []int // typ に入らない定数の要素
	for i, v := range vals {
		k, ok := ir.ValIntLiteral(v)
		if !ok {
			continue
		}
		lo, hi = min(lo, k), max(hi, k)
		if k < tlo || k > thi {
			bad = append(bad, i)
		}
	}
	if len(bad) == 0 {
		return typ
	}
	if h.rewriting() {
		if report {
			for _, i := range bad {
				if i < len(args) {
					h.rewriteAs("array-type", args[i], typ.String())
				}
			}
		}
		return typ
	}
	// 変数の要素が符号付きなら符号付きのまま (実行時に負の値がありうる)
	for size := typ.Size; size <= 2; size++ {
		t := h.prog.Types.IntType(size, typ.Signed || lo < 0)
		if l, u := intRange(t); lo >= l && hi <= u {
			return t
		}
	}
	panic(&diag.Error{Msg: fmt.Sprintf("the constants in the array literal (%d to %d) do not fit in one integer type of 16 bits; write the element type", lo, hi)})
}

// mixedSign は大小の比較の両辺 l, r が、互換型で比べると片方の値が読み替えられる整数か (F6)。
func mixedSign(l, r ir.Operand) bool {
	lt, rt := ir.ValType(l), ir.ValType(r)
	if lt.Kind != types.Int || rt.Kind != types.Int || lt.Enum != nil || rt.Enum != nil || lt.Signed == rt.Signed {
		return false
	}
	for _, v := range []ir.Operand{l, r} {
		if x, ok := v.(*ir.Value); ok && x.Kind == ir.KindLiteral && x.Untyped {
			return false // 型のない定数は相手の型になる
		}
	}
	u, s := lt, rt
	if lt.Signed {
		u, s = rt, lt
	}
	return u.Size >= s.Size // 符号なしが狭ければ、符号付きの型に全部の値が入る
}

// mixedSignError は F6 のエラー (fc 4)。
func (h *Hlc) mixedSignError(lt, rt *types.Type) *diag.Error {
	ct := h.prog.Types.Compatible(lt, rt)
	why := "an unsigned value of 128 or more reads as negative"
	if !ct.Signed {
		why = "a negative value reads as a large unsigned one"
	} else if ct.Size == 2 {
		why = "an unsigned value of 32768 or more reads as negative"
	}
	if !lt.Signed {
		lt, rt = rt, lt // 符号付きを先に (`a > b` は `b < a` にしてから来るので、書いた順とは限らない)
	}
	return &diag.Error{Msg: fmt.Sprintf("ordered comparison of signed %s and unsigned %s would compare as %s (%s); convert one side with `as`", lt, rt, ct, why)}
}

// rewriteMixedSign は fc 3 のモジュールの F6 の比較 (l, r は両辺の値、cs はその式) に、互換型でない側を互換型にする `as` を
// 報告する (fc 3 の比較の型のまま)。
func (h *Hlc) rewriteMixedSign(l, r ir.Operand, cs [2]*cexpr) {
	ct := h.prog.Types.Compatible(ir.ValType(l), ir.ValType(r))
	for i, v := range []ir.Operand{l, r} {
		if ir.ValType(v) != ct {
			h.rewriteAs("sign-compare", cs[i], ct.String())
		}
	}
}
