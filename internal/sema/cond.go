package sema

// fc 4 の条件式 `c ? x : y` (Agent/discussions/2026-10-05-cond-expr-do-while.md)。
//
// 型: 2 つの枝の型から決める (condJoin)。型付きどうしは互換型 (二項演算と同じ Compatible: 大きいほう、同じ大きさなら符号付き)、
// 片方が型のない定数なら比較と同じ規則で相手に合わせる (収まらなければ、文脈の整数型に両方が入ればその型、入らなければエラー:
// `c ? x : 200` (x:i8))、両方とも型のない定数なら
// 文脈の整数型 (代入先・引数・比較の相手。withExpected が渡す) に収まるか見て、文脈が無ければ両方が入る一番小さい整数型。
// 文脈の型 (withExpected) は両方の枝に渡す (`.A`・null・型名を省いた struct リテラル・slice)。
//
// A1: 条件式は算術の区切りにならない (括弧と同じ)。上から来た幅と条件式の型の大きさの広いほうで、両方の枝を計算する。
//
// 評価: 条件を分岐にして (compileCond)、選ばれたほうの枝だけを評価し、一時変数に入れる。条件が定数なら選ばれたほうだけを
// 評価する。条件と両方の枝が整数の定数なら定数に畳む (const の宣言にも書ける)。条件の文脈 (`if (c ? a : b)`) では値を作らずに
// 枝ごとの分岐にする。

import (
	"fmt"

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/types"
)

// isSmallInt は t が 1〜2 バイトの整数型 (enum でない) か。
func isSmallInt(t *types.Type) bool {
	return isPlainInt(t) && t.Size >= 1 && t.Size <= 2
}

// condJoin は条件式の枝の型 a, b から条件式の型を決める (expected は文脈の型、無ければ nil)。合わなければ診断の panic。
func (h *Hlc) condJoin(a, b exprInfo, expected *types.Type) exprInfo {
	u := h.prog.Types
	switch {
	case a.untyped && b.untyped:
		if isSmallInt(expected) {
			for _, x := range []exprInfo{a, b} {
				if lo, hi := intRange(expected); x.n < lo || x.n > hi {
					panic(&diag.Error{Msg: fmt.Sprintf("%d does not fit in %s (a branch of `?:`; write `%d as %s` to truncate)", x.n, expected, x.n, expected)})
				}
			}
			return exprInfo{t: expected}
		}
		lo, hi := min(a.n, b.n), max(a.n, b.n)
		for _, t := range []*types.Type{u.IntType(1, false), u.IntType(1, true), u.IntType(2, false), u.IntType(2, true)} {
			if l, hh := intRange(t); lo >= l && hi <= hh {
				return exprInfo{t: t}
			}
		}
		panic(&diag.Error{Msg: fmt.Sprintf("the branches of `?:` (%d and %d) do not fit in one integer type", a.n, b.n)})
	case a.untyped || b.untyped:
		lit, other := a, b
		if b.untyped {
			lit, other = b, a
		}
		switch literalRule(lit.t, lit.n, false, other.t, true) {
		case litFits:
			return exprInfo{t: other.t}
		case litSigned16:
			return exprInfo{t: u.IntType(2, true)}
		case litCmpError:
			if isSmallInt(expected) && isSmallInt(other.t) && other.t.Size <= expected.Size {
				if lo, hi := intRange(expected); lit.n >= lo && lit.n <= hi {
					return exprInfo{t: expected} // 文脈の型に両方が入る (`g = c ? -2 : x` (g:i16、x:u8))
				}
			}
			panic(&diag.Error{Msg: fmt.Sprintf("%d does not fit in %s, the type of the other branch of `?:` (convert a branch with `as`)", lit.n, other.t)})
		}
		if t := u.Compatible(lit.t, other.t); t != nil {
			return exprInfo{t: t}
		}
		panic(&diag.Error{Msg: fmt.Sprintf("the branches of `?:` have incompatible types %s and %s", a.t, b.t)})
	}
	t := u.Compatible(a.t, b.t)
	if t == nil {
		panic(&diag.Error{Msg: fmt.Sprintf("the branches of `?:` have incompatible types %s and %s", a.t, b.t)})
	}
	return exprInfo{t: t}
}

// condType は条件式 e (評価済みの opCond) の型。枝の型を決められなければ ok = false (soa の要素は型を決める段ではハンドルで、
// 値として読むと struct なので、評価してから決める)。
func (h *Hlc) condType(e *cexpr) (exprInfo, bool) {
	a, ok := h.exprType(e.args[1])
	if !ok || a.t.Kind == types.SoaRef {
		return exprInfo{}, false
	}
	b, ok := h.exprType(e.args[2])
	if !ok || b.t.Kind == types.SoaRef {
		return exprInfo{}, false
	}
	return h.condJoin(a, b, e.ty), true
}

// constEvalCond は条件式の定数の畳み込み: 条件と両方の枝が整数の定数なら、選ばれたほうの値 (両方とも型のない定数なら型のない
// まま、ほかは条件式の型の値)。
func (h *Hlc) constEvalCond(c *cexpr) *cexpr {
	cond, a, b := h.constEval(c.args[0]), h.constEval(c.args[1]), h.constEval(c.args[2])
	if cond.isLiteralInt() && a.isLiteralInt() && b.isLiteralInt() {
		ai, _ := h.exprType(a)
		bi, _ := h.exprType(b)
		j := h.condJoin(ai, bi, c.ty)
		pick := b
		if cond.val.Int != 0 {
			pick = a
		}
		if ai.untyped && bi.untyped {
			return pick
		}
		return cv(ir.NewIntLiteral("", j.t, wrapInt(wrapInt(pick.val.Int, pick.val.Type), j.t)))
	}
	return &cexpr{kind: cOp, op: opCond, args: []*cexpr{cond, a, b}, ty: c.ty}
}

// condValue は値として使う条件式 e (評価済みの opCond) の IR を出す。hint は A1 で上から来た幅。
func (h *Hlc) condValue(e *cexpr, hint int) ir.Operand {
	info, ok := h.condType(e)
	if !ok {
		return h.condValueUntyped(e)
	}
	t, _ := h.condWidth(info, hint)
	tmp := h.newTmp(t)
	h.condEmit(e, info, hint, func(v ir.Operand, _ *cexpr) {
		h.emit(&ir.Op{Code: ir.OpLoad, Dst: tmp, Src: []ir.Operand{v}})
	}, false)
	return tmp
}

// condWidth は条件式 (型は info) を計算する型と A1 の幅 (広げないなら 0)。条件式は A1 の区切りにならない: 上から来た幅 hint と
// 条件式の型の大きさの広いほうで両方の枝を計算する。
func (h *Hlc) condWidth(info exprInfo, hint int) (*types.Type, int) {
	if !h.v4() || !isSmallInt(info.t) {
		return info.t, 0
	}
	w := max(hint, info.t.Size)
	return h.prog.Types.IntType(w, info.t.Signed), w
}

// condEmit は条件式 e (型は info) の選ばれたほうの枝を評価し、条件式の型 (A1 で広げた幅) にした値を sink に渡す (sink は枝ごとに
// 1 回、枝の終わりで呼ぶ)。値を一時変数に集めずに枝ごとに使い切る所 (return・変数への代入: condReturn / condAssign) は sink で
// 直接書く (合流した後の `lda t / sta x` が無くなる)。terminal は sink が枝を終える (return) ので合流のジャンプが要らないか。
func (h *Hlc) condEmit(e *cexpr, info exprInfo, hint int, sink func(v ir.Operand, branch *cexpr), terminal bool) {
	t, w := h.condWidth(info, hint)
	branch := func(c *cexpr) {
		v := h.rvalIn(c, w)
		if lv, ok := v.(*ir.Value); ok && lv.Kind == ir.KindLiteral && lv.IsInt && lv.Untyped {
			v = ir.NewIntLiteral("", t, wrapInt(lv.Int, t)) // condJoin が収まることを確かめている
		} else if vt := ir.ValType(v); vt != t && vt.Kind == types.Int && t.Kind == types.Int && vt.Size == t.Size {
			v = ir.NewCastedValue(v, t, 0) // 同じ大きさで符号だけ違う枝は条件式の型として読む
		} else {
			v = h.cast(v, t)
		}
		sink(v, c)
	}
	if k, ok := h.constCond(e.args[0]); ok {
		// 定数の条件: 選ばれたほうだけを評価する
		if k != 0 {
			branch(e.args[1])
		} else {
			branch(e.args[2])
		}
		return
	}
	labels := h.newLabels("else", "end")
	h.compileCond(e.args[0], labels[0], false)
	branch(e.args[1])
	if !terminal {
		h.emit(&ir.Op{Code: ir.OpJump, Label: labels[1]})
	}
	h.emit(&ir.Op{Code: ir.OpLabel, Label: labels[0]})
	branch(e.args[2])
	h.emit(&ir.Op{Code: ir.OpLabel, Label: labels[1]})
}

// condSink は、代入のような変換で to にする式 c が値を一時変数に集めずに枝ごとに書ける条件式なら、評価済みの opCond と型を返す
// (fc 4、整数・bool・ポインタなどの 1〜2 バイトの値、枝の型が評価の前に決まり、条件が定数でない)。
func (h *Hlc) condSink(c *cexpr, to *types.Type) (*cexpr, exprInfo, bool) {
	if !h.v4() || to == nil || isAggregate(to) {
		return nil, exprInfo{}, false
	}
	e := h.constEval(c)
	if e.kind != cOp || e.op != opCond {
		return nil, exprInfo{}, false
	}
	if _, isConst := h.constCond(e.args[0]); isConst {
		return nil, exprInfo{}, false
	}
	info, ok := h.condType(e)
	if !ok || isAggregate(info.t) {
		return nil, exprInfo{}, false
	}
	return e, info, true
}

// condReturn は `return c ? x : y` を枝ごとの return にする (rvalAssign と同じ検査。値は枝ごとに戻り値の型にする)。
// 条件式でなければ false。
func (h *Hlc) condReturn(c *cexpr, rt *types.Type, what string) bool {
	e, info, ok := h.condSink(c, rt)
	if !ok {
		return false
	}
	pre := h.assignPre(what, e, rt)
	checked := h.preConvert(e, rt)
	hint := 0
	if isSmallInt(rt) {
		hint = rt.Size
	}
	h.condEmit(e, info, hint, func(v ir.Operand, branch *cexpr) {
		h.assignPost(what, rt, v, pre)
		h.warnDropConst(what, rt, v)
		h.emit(&ir.Op{Code: ir.OpReturn, Src: []ir.Operand{h.convertValue(v, rt, branch, checked)}})
	}, true)
	return true
}

// condAssign は `left = c ? x : y` を枝ごとの代入にする (assign と同じ検査。lv なら left は書く場所のポインタ、dt は代入先の型)。
// 条件式でなければ false。
func (h *Hlc) condAssign(left ir.Operand, lv bool, dt *types.Type, rhs *cexpr, what string, pre bool) bool {
	e, info, ok := h.condSink(rhs, dt)
	if !ok {
		return false
	}
	checked := h.preConvert(e, dt)
	hint := 0
	if isSmallInt(dt) {
		hint = dt.Size
	}
	h.condEmit(e, info, hint, func(v ir.Operand, branch *cexpr) {
		h.assignPost(what, dt, v, pre)
		if lv || !h.readOnly(left) {
			h.warnDropConst(what, dt, v)
		}
		v = h.convertValue(v, dt, branch, checked)
		if lv {
			h.emit(ir.NewStoreMem(left, nil, 0, 0, dt.Size, v))
		} else {
			h.emit(&ir.Op{Code: ir.OpLoad, Dst: left, Src: []ir.Operand{v}})
		}
	}, false)
	return true
}

// constCond は条件式の条件が定数ならその値 (IR は出さない)。
func (h *Hlc) constCond(c *cexpr) (int, bool) {
	if x := h.constEval(c); x.isLiteralInt() {
		return x.val.Int, true
	}
	return 0, false
}

// condValueUntyped は、枝の型を評価する前に決められない条件式 (slice の範囲など) の IR: 評価した枝の値の型から条件式の型を
// 決める。1 つめの枝を入れる一時変数は、2 つめの枝を評価してから型が違えば作り直す (1 つめの枝の値を変換に命令が要るときはエラー)。
func (h *Hlc) condValueUntyped(e *cexpr) ir.Operand {
	infoOf := func(v ir.Operand) exprInfo {
		if lv, ok := v.(*ir.Value); ok && lv.Kind == ir.KindLiteral && lv.IsInt && lv.Untyped {
			return exprInfo{t: lv.Type, untyped: true, n: lv.Int}
		}
		return exprInfo{t: ir.ValType(v)}
	}
	if k, ok := h.constCond(e.args[0]); ok {
		// 定数の条件: 選ばれたほうの値 (型の照合のため、選ばれなかったほうの型が分かれば合わせる)
		pick, other := e.args[1], e.args[2]
		if k == 0 {
			pick, other = other, pick
		}
		v := h.rval(pick)
		if oi, ok := h.exprType(other); ok {
			t := h.condJoin(infoOf(v), oi, e.ty).t
			tmp := h.newTmp(t)
			h.emit(&ir.Op{Code: ir.OpLoad, Dst: tmp, Src: []ir.Operand{h.cast(v, t)}})
			return tmp
		}
		return v
	}
	labels := h.newLabels("else", "end")
	h.compileCond(e.args[0], labels[0], false)
	va := h.rval(e.args[1])
	tmp := h.newTmp(ir.ValType(va))
	opA := &ir.Op{Code: ir.OpLoad, Dst: tmp, Src: []ir.Operand{va}}
	h.emit(opA)
	h.emit(&ir.Op{Code: ir.OpJump, Label: labels[1]})
	h.emit(&ir.Op{Code: ir.OpLabel, Label: labels[0]})
	vb := h.rval(e.args[2])
	t := h.condJoin(infoOf(va), infoOf(vb), e.ty).t
	if t != tmp.Type {
		var a ir.Operand
		switch {
		case infoOf(va).untyped:
			a = ir.NewIntLiteral("", t, wrapInt(infoOf(va).n, t))
		case t.Kind == types.Int && ir.ValType(va).Kind == types.Int && (t.Size == ir.ValType(va).Size || !ir.ValType(va).Signed):
			a = va // 同じ大きさか、符号なしの狭い値 (0 で広がる)
		default:
			panic(&diag.Error{Msg: fmt.Sprintf("cannot convert the first branch of `?:` (%s) to %s here; convert it with `as`", ir.ValType(va), t)})
		}
		tmp = h.newTmp(t)
		opA.Dst, opA.Src[0] = tmp, a
	}
	h.emit(&ir.Op{Code: ir.OpLoad, Dst: tmp, Src: []ir.Operand{h.cast(vb, t)}})
	h.emit(&ir.Op{Code: ir.OpLabel, Label: labels[1]})
	return tmp
}

// compileCondCond は条件の文脈の条件式 `if (c ? a : b)`: 値を作らずに、選ばれたほうの枝の条件で分岐する。
func (h *Hlc) compileCondCond(e *cexpr, label string, jumpIfTrue bool) {
	h.condType(e) // 枝の型の照合 (決められるときだけ)
	if k, ok := h.constCond(e.args[0]); ok {
		if k != 0 {
			h.compileCond(e.args[1], label, jumpIfTrue)
		} else {
			h.compileCond(e.args[2], label, jumpIfTrue)
		}
		return
	}
	labels := h.newLabels("else", "end")
	h.compileCond(e.args[0], labels[0], false)
	h.compileCond(e.args[1], label, jumpIfTrue)
	h.emit(&ir.Op{Code: ir.OpJump, Label: labels[1]})
	h.emit(&ir.Op{Code: ir.OpLabel, Label: labels[0]})
	h.compileCond(e.args[2], label, jumpIfTrue)
	h.emit(&ir.Op{Code: ir.OpLabel, Label: labels[1]})
}
