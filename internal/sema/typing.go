package sema

// 式の型を IR を出す前に決める段 (Agent/wiki/plans/roadmap.md の構造の整理「sema の式に型付きの中間表現を」の最初の一歩)。
//
// 今の sema は lval が型の検査・暗黙の変換・診断・IR の出力を同時にするので、式の型は IR を出すまで分からない。そのせいで、
// 型を知るために一度評価した値を捨てて評価し直す (enum の `f() == .A` の二重評価) や、定数の経路と実行時の経路で検査が
// 食い違う (`+%` の型のない定数の検査: wrap.go) が起きた。ここでは IR を出さずに式の型を決める exprType を作り、対象を
// 広げながら lval の経路を置き換えていく。
//
// 今の対象: 定数になる式 (constEval をそのまま使う) と、実行時の二項演算 (算術・ビット演算・シフト・比較・論理演算・
// `+%`)、単項の `- ~ !`、添字・参照はがし・アドレス・struct のフィールド、変数・キャスト・呼び出しの葉。それ以外 (slice の範囲・
// @len・@min などの組み込み・soa) は分からない (ok = false)。
//
// 正しさの物差し: FC_VERIFY_IR (テストと fuzz では常に有効) のとき、lval が式を評価するたびに exprType と実際に出した値の型を
// 比べ、違えばコンパイラの内部エラーにする (checkExprType)。規則は今の lval の規則 (adaptLiteral・Compatible・F1 のシフト・
// wrapExpr) の写しで、A1 の広げ (widen.go) の前の型を返す (広げは親の型に合わせて後から子の型を変える)。

import (
	"fmt"

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/types"
)

// exprInfo は IR を出す前に分かった式の型。untyped は型のない整数定数 (n はその値)。
type exprInfo struct {
	t       *types.Type
	untyped bool
	n       int
}

// exprType は式 c の型を、IR を出さずに決める (分からなければ ok = false)。診断の panic は外に出さない。
func (h *Hlc) exprType(c *cexpr) (info exprInfo, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			if _, isDiag := r.(*diag.Error); !isDiag {
				panic(r)
			}
			info, ok = exprInfo{}, false
		}
	}()
	return h.exprType0(c)
}

func (h *Hlc) exprType0(c *cexpr) (exprInfo, bool) {
	e := h.constEval(c)
	switch e.kind {
	case cValue:
		v := e.val
		if v.Type == nil || v.Type.Kind == types.Bad || v.Type.Kind == types.TypeName || v.Type.Kind == types.Macro || v.Type.IsSoa {
			return exprInfo{}, false
		}
		if v.Kind == ir.KindLiteral && v.IsInt && v.Untyped {
			return exprInfo{t: v.Type, untyped: true, n: v.Int}, true
		}
		if root := h.prog.storageAliases[v]; root != nil {
			return exprInfo{}, false
		}
		return exprInfo{t: v.Type}, true
	case cCast:
		ty := e.ty
		if ty == nil {
			ty = h.typeEval(e.typ)
		}
		return exprInfo{t: ty}, ty != nil
	case cOp:
		return h.opType(e)
	}
	return exprInfo{}, false
}

// opType は実行時の演算 e の型。
func (h *Hlc) opType(e *cexpr) (exprInfo, bool) {
	u := h.prog.Types
	switch e.op {
	case opEq, opNe, opLt, opGt, opLe, opGe, opLand, opLor, opNot:
		return exprInfo{t: u.Bool()}, true
	case opUminus, opBitNot:
		a, ok := h.exprType(e.args[0])
		if !ok || a.untyped || a.t.Kind != types.Int {
			return exprInfo{}, false
		}
		return exprInfo{t: a.t}, true
	case opAddWrap, opSubWrap, opMulWrap:
		a, ok := h.exprType(e.args[0])
		if !ok || a.untyped {
			return exprInfo{}, false
		}
		return exprInfo{t: a.t}, true
	case opCall:
		f, ok := h.exprType(e.args[0])
		if !ok || f.t.Kind != types.Func || f.t.Base == nil || f.t.Base.Kind == types.Void {
			return exprInfo{}, false
		}
		return exprInfo{t: f.t.Base}, true
	case opAdd, opSub, opMul, opDiv, opMod, opAnd, opOr, opXor, opShiftLeft, opShiftRight:
		a, ok := h.exprType(e.args[0])
		if !ok {
			return exprInfo{}, false
		}
		b, ok := h.exprType(e.args[1])
		if !ok {
			return exprInfo{}, false
		}
		return h.binaryType(e.op, a, b)
	case opIndex:
		// a[i]: 配列・ポインタは要素、slice は SliceOf (soa の添字はハンドルなので扱わない)
		a, ok := h.exprType(e.args[0])
		switch {
		case !ok || a.untyped || a.t.IsSoa:
		case a.t.IsSlice():
			return exprInfo{t: a.t.SliceOf}, true
		case (a.t.Kind == types.Array || a.t.Kind == types.Pointer) && a.t.Base != nil && a.t.Base.Kind != types.Void:
			return exprInfo{t: a.t.Base}, true
		}
	case opDeref:
		if a, ok := h.exprType(e.args[0]); ok && a.t.Kind == types.Pointer && a.t.Base.Kind != types.Void {
			return exprInfo{t: a.t.Base}, true
		}
	case opRef:
		if a, ok := h.exprType(e.args[0]); ok && !a.untyped && a.t.Kind != types.SoaRef {
			return exprInfo{t: u.PointerTo(a.t)}, true
		}
	case opField:
		// a.f: struct の値か struct へのポインタ (自動で参照はがし)。soa のフィールドは扱わない
		a, ok := h.exprType(e.args[0])
		if !ok || a.untyped {
			break
		}
		t := a.t
		if t.Kind == types.Pointer {
			t = t.Base
		}
		if t.Kind == types.Struct && !t.IsSlice() {
			if f, ok := t.Field(e.name); ok {
				return exprInfo{t: f.Type}, true
			}
		}
	}
	return exprInfo{}, false
}

// binaryType は実行時の二項演算の型 (lval の算術の経路: F1 のシフト、adaptLiteral、Compatible、ポインタの加減算)。
func (h *Hlc) binaryType(op cop, a, b exprInfo) (exprInfo, bool) {
	isInt := func(t *types.Type) bool { return t.Kind == types.Int && t.Enum == nil }
	if (op == opShiftLeft || op == opShiftRight) && h.v4() && isInt(a.t) && (b.t.Kind == types.Int || b.t.Kind == types.Bool) {
		return exprInfo{t: a.t}, true // F1: シフトの結果は左辺の型 (shiftByLeft)
	}
	at, bt := literalAdapted(a, b), literalAdapted(b, a)
	if op == opSub && at.Kind == types.Pointer && bt.Kind == types.Pointer && at.Base == bt.Base {
		return exprInfo{t: h.prog.Types.IntType(2, false)}, true // p - q は要素数 (pointerDiff)
	}
	if t := h.prog.Types.Compatible(at, bt); t != nil {
		return exprInfo{t: t}, true
	}
	if (op == opAdd || op == opSub) && (at.Kind == types.Pointer || at.Kind == types.SoaRef) && bt.Kind == types.Int {
		return exprInfo{t: at}, true // p + i
	}
	return exprInfo{}, false
}

// literalAdapted は二項演算の片方 x の、相手 other に合わせた後の型 (adaptLiteral)。型のない定数は、相手の型より大きい
// 型の値か 16 ビットに収まらなければ自分の型、そうでなければ相手の型 (収まらない値は相手の型に切り詰めるか、相手が符号付き
// なら互換型が相手の型になるので、どちらも相手の型と同じ結果)。
func literalAdapted(x, other exprInfo) *types.Type {
	if !x.untyped || other.untyped {
		return x.t
	}
	t := other.t
	if t.Kind != types.Int || t.Enum != nil || t.Size < 1 || t.Size > 2 {
		return x.t
	}
	if x.t.Size > t.Size || x.n < -32768 || x.n > 65535 {
		return x.t
	}
	return t
}

// checkExprType は lval が式 c を評価した値 v の型が exprType と同じかを確かめる (FC_VERIFY_IR のときだけ)。lv は v が左辺値
// (その場所を指すポインタ) か (値の型はポインタの先)。
func (h *Hlc) checkExprType(c *cexpr, v ir.Operand, lv bool) {
	if v == nil || !h.prog.Config.VerifyIR() || c.kind != cOp {
		return
	}
	switch c.op {
	case opAdd, opSub, opMul, opDiv, opMod, opAnd, opOr, opXor, opShiftLeft, opShiftRight,
		opAddWrap, opSubWrap, opMulWrap, opUminus, opBitNot, opIndex, opDeref, opRef, opField:
	default:
		return
	}
	info, ok := h.exprType(c)
	if !ok {
		return
	}
	got := ir.ValType(v)
	if lv {
		if got.Kind != types.Pointer {
			return // soa のハンドルなど
		}
		got = got.Base
	}
	if info.untyped {
		if lv, isLit := v.(*ir.Value); isLit && lv.Kind == ir.KindLiteral && lv.Untyped {
			return
		}
	}
	if got != info.t {
		panic(&diag.Error{Msg: fmt.Sprintf("internal: exprType of %s is %s but lval made %s (sema/typing.go)", h.exprText(c), info.t, got)})
	}
}
