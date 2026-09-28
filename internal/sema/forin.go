package sema

// fc 3 の for-each (doc/v3_plan.md §10、2026-09-27 決定):
//
//	for (var x in A) { … }         // 配列・slice の要素の値 (コピー。読み取り専用)
//	for (var i, x in A) { … }      // 添字と要素
//	for (var p in &A) { … }        // 回す値が配列・slice へのポインタ (*[N]T / *[]T) なら要素のポインタ *T
//	for (var i in lo..hi) { … }    // 範囲 (hi を含まない。`lo..=hi` は含む)。`var i:u16 in 0..300` で型も書ける
//
// 要素のポインタ p は変数にせず `&A[i]` の別名にする (`p.hp` は `A[i].hp` と同じ IR になり、静的な配列なら `lda A+ofs,x`)。
// ループは C 型 for と同じ IR の形 (begin: if (i < end) { body; step } else break) にして、最適化 (帰納変数・展開) に乗せる。
// 終わりが型に収まらないとき (u8 の `0..256` / `0..=255`) は、後ろで判定する形 (body; i++; if (i != end) goto begin) にして、
// 1 周多い値 (256 → 0) との比較で抜ける。

import (
	"fmt"

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/syntax"
	"github.com/haramako/fc/internal/types"
)

// forInLoop は for-each を数え上げのループにしたもの。
type forInLoop struct {
	i    *ir.Value // ループの変数 (範囲の変数か添字。ユーザーのものでなければ一時変数)
	end  *ir.Value // i がこれになったら抜ける (wrap でなければ i < end の間回る)
	wrap bool      // 後ろで判定する形 (i++ の後 i != end の間回る)
	pre  *cexpr    // wrap のときの、空でないかの検査 (nil なら無し)
}

func (h *Hlc) compileForIn(s *syntax.ForInStmt) {
	h.inScope(func() {
		label := h.pendingLabel // ラベル付きなら continue L の判定に使う (pushBreakable が引き取る)
		var loop forInLoop
		var prologue func()
		if r, ok := s.X.(*syntax.RangeExpr); ok {
			if s.Index != nil {
				panic(&diag.Error{Msg: "a range gives one variable: write `for (var i in a..b)`"})
			}
			loop = h.forInRange(s, r)
		} else {
			if s.Type != nil {
				panic(&diag.Error{Msg: "the type of a for-each variable is the element type; a type can be written only for a range (`for (var i:u16 in 0..300)`)"})
			}
			loop, prologue = h.forInElems(s)
		}
		h.emitForInLoop(s, label, loop, prologue)
	})
}

// forInRange は範囲 `lo..hi` / `lo..=hi` のループ。
func (h *Hlc) forInRange(s *syntax.ForInStmt, r *syntax.RangeExpr) forInLoop {
	lo := h.rval(toC(r.Lo))
	hi := h.rval(toC(r.Hi))
	for _, v := range []ir.Operand{lo, hi} {
		if t := ir.ValType(v); t.Kind != types.Int || t.Enum != nil {
			panic(&diag.Error{Msg: fmt.Sprintf("range bounds must be integers (got %s)", t)})
		}
	}
	hk, hiConst := ir.ValIntLiteral(hi)
	var t *types.Type
	if s.Type != nil {
		t = h.typeEval(s.Type)
		if t.Kind != types.Int || t.Enum != nil {
			panic(&diag.Error{Msg: fmt.Sprintf("the variable of a range must be an integer (got %s)", t)})
		}
	} else {
		// 型は両端から (型のない定数は相手に合わせる)。hi を含まない定数の終わりは、最後の値 (hi - 1) で決める
		// (`0..256` は u8 で 256 回、`0..@len(buf)` も同じ)
		last := hi
		if hiConst && !r.Inclusive {
			last = h.IntValue(hk - 1)
		}
		a, b := h.adaptLiteral(lo, last, true)
		t = h.compatible(ir.ValType(a), ir.ValType(b))
	}
	lo0, hi0 := intRange(t)
	if k, ok := ir.ValIntLiteral(lo); ok && (k < lo0 || k > hi0) {
		panic(&diag.Error{Msg: fmt.Sprintf("range start %d does not fit in %s", k, t)})
	}
	i := h.forInVar(s.Elem.Name, t)
	h.lval(cop2(opLoad, cv(i), cv(h.operandValue(lo))))
	h.markLoopVar(i)
	bits := 8 * t.Size
	wrapTo := func(n int) *ir.Value { // n を t の値に (256 → 0)
		n = ir.FloorMod(n, 1<<bits)
		if t.Signed && n > hi0 {
			n -= 1 << bits
		}
		return ir.NewIntLiteral("", t, n)
	}
	if hiConst {
		last := hk
		if !r.Inclusive {
			last--
		}
		if last < lo0-1 || last > hi0 {
			panic(&diag.Error{Msg: fmt.Sprintf("range end %d does not fit in %s", hk, t)})
		}
		if last < hi0 {
			return forInLoop{i: i, end: ir.NewIntLiteral("", t, last+1)} // C 型 for と同じ `i < end`
		}
		// 終わりが型の最大値 (u8 の 255): 1 周多い値 (0) との比較で後ろで抜ける。i は型の値なので空にはならない
		return forInLoop{i: i, end: wrapTo(last + 1), wrap: true}
	}
	// 実行時の終わりは最初に 1 回だけ評価する
	end := h.newTmp(t)
	h.emit(&ir.Op{Code: ir.OpLoad, Dst: end, Src: []ir.Operand{h.cast(hi, t)}})
	if !r.Inclusive {
		return forInLoop{i: i, end: end}
	}
	// `lo..=hi` (hi は実行時): hi が型の最大値でも回るように、hi + 1 (折り返す) と後ろで比べる
	next := h.newTmp(t)
	h.emit(&ir.Op{Code: ir.OpAdd, Dst: next, Src: []ir.Operand{end, ir.NewIntLiteral("", t, 1)}})
	return forInLoop{i: i, end: next, wrap: true, pre: cop2(opLe, cv(i), cv(end))}
}

// forInElems は配列・slice (またはそのポインタ) の要素のループ。prologue は各周の最初に要素の変数を用意する。
func (h *Hlc) forInElems(s *syntax.ForInStmt) (forInLoop, func()) {
	x := h.hoistCalls(toC(s.X)) // 回す値の中の呼び出しは 1 回だけ評価する
	v := h.rval(x)
	t := ir.ValType(v)
	ptrMode := false
	var base *cexpr // 要素を base[i] で引く
	var bt *types.Type
	switch {
	case t.Kind == types.Pointer && (t.Base.Kind == types.Array || t.Base.IsSlice()):
		ptrMode, bt = true, t.Base
		if ex := h.constEval(x); ex.kind == cOp && ex.op == opRef && bt.Kind == types.Array && staticPlace(ex.args[0]) {
			base = ex.args[0] // `&A`: 要素のポインタを `&A[i]` にする (A が静的な配列なら添字の読み書きのまま)
		} else {
			// ほかは回す値を最初に 1 回だけ評価する (`&M[k]`、`&sp.buf`、`&s` (slice) の中の変数を毎周読み直して、途中で
			// 変えると別の行・範囲外を回っていた。survey 2026-09-27)
			base = cop2(opDeref, cv(h.freezeRO(v)))
		}
	case t.Kind == types.Array:
		base, bt = x, t
	case t.IsSlice():
		base, bt = cv(h.freezeRO(v)), t // slice の値は 1 回だけ評価する
	default:
		panic(&diag.Error{Msg: fmt.Sprintf("cannot iterate over %s (a for-each needs an array, a slice, a pointer to one of them, or a range `a..b`)", t)})
	}
	// 長さと添字の型: 配列は定数 (256 以下なら u8)、slice は長さの型
	var loop forInLoop
	var it *types.Type
	name := h.tmpName("$")
	if s.Index != nil {
		name = s.Index.Name
	}
	if bt.Kind == types.Array {
		n := bt.Length
		if ce := h.constEval(x); ce.kind == cValue && h.strLen(ce.val) && !ptrMode && n > 0 {
			n-- // 文字列リテラルは終端の 0 を回らない (`@len("abc")` や slice にしたときと同じ 3 文字。survey 2026-09-27)
		}
		it = h.prog.Types.IntType(1, false)
		if n > 256 {
			it = h.prog.Types.IntType(2, false)
		}
		loop.i = h.forInVar(name, it)
		h.lval(cop2(opLoad, cv(loop.i), cv(ir.NewIntLiteral("", it, 0))))
		switch {
		case n == 256 || n == 65536:
			loop.end, loop.wrap = ir.NewIntLiteral("", it, 0), true
		default:
			loop.end = ir.NewIntLiteral("", it, n) // n == 0 なら 1 回も回らない
		}
	} else {
		ln := h.rval(&cexpr{kind: cOp, op: opLen, args: []*cexpr{base}})
		it = ir.ValType(ln)
		loop.i = h.forInVar(name, it)
		h.lval(cop2(opLoad, cv(loop.i), cv(ir.NewIntLiteral("", it, 0))))
		end := h.newTmp(it)
		h.emit(&ir.Op{Code: ir.OpLoad, Dst: end, Src: []ir.Operand{ln}})
		loop.end = end
	}
	if s.Index != nil {
		h.markLoopVar(loop.i)
	}
	et := bt.Base // 要素の型
	if bt.IsSlice() {
		et = bt.SliceOf
	}
	elem := cop2(opIndex, base, cv(loop.i))
	if ptrMode {
		// p は `&base[i]` の別名 (変数を作らない)
		h.declareAlias(s.Elem.Name, h.prog.Types.PointerTo(et), cop2(opRef, elem))
		return loop, nil
	}
	xv := h.forInVar(s.Elem.Name, et)
	h.markLoopVar(xv)
	return loop, func() {
		h.writeLoopVar(xv, func() { h.lval(cop2(opLoad, cv(xv), elem)) })
	}
}

// freezeRO は freeze と同じく値を 1 回だけ評価するが、読み取り専用 (const の表を指すポインタ) の印を引き継ぐ
// (写した一時変数から書き込めてしまわないように)。
func (h *Hlc) freezeRO(v ir.Operand) *ir.Value {
	r := h.freeze(v).(*ir.Value)
	h.markReadOnly(r, h.readOnly(v))
	return r
}

// forInVar は for-each の変数を宣言する (ユーザーの変数は markLoopVar で読み取り専用にする)。
func (h *Hlc) forInVar(name string, t *types.Type) *ir.Value {
	return h.addVar(ir.NewLocal(name, t, ir.LTNone))
}

// markLoopVar は for-each の変数 v を読み取り専用にする (代入は checkLoopVarAssign がエラーにする)。
func (h *Hlc) markLoopVar(v *ir.Value) {
	if h.loopVars == nil {
		h.loopVars = map[*ir.Value]bool{}
	}
	h.loopVars[v] = true
}

// writeLoopVar は f (ループ自身の代入: step と要素のコピー) の間だけ v への代入を許す。
func (h *Hlc) writeLoopVar(v *ir.Value, f func()) {
	marked := h.loopVars[v]
	delete(h.loopVars, v)
	f()
	if marked {
		h.loopVars[v] = true
	}
}

// declareAlias は名前 name を式 e の別名にする (for-each の要素のポインタ)。名前の値は置き場所を持たない。
func (h *Hlc) declareAlias(name string, t *types.Type, e *cexpr) {
	v := ir.NewLocal(name, t, ir.LTTemp)
	h.scope.Declare(v)
	if h.exprAliases == nil {
		h.exprAliases = map[*ir.Value]*cexpr{}
		h.aliasNodes = map[*cexpr]string{}
	}
	ev := h.constEval(e) // 名前を読むたびに同じ評価済みの式になる (cmemo)。代入の左辺に来たら checkAliasAssign が見つける
	h.exprAliases[v] = ev
	h.aliasNodes[ev] = name
}

// checkAliasAssign は for-each の要素のポインタ (式の別名) への代入をエラーにする (`p++`、`p = &b` が置き場所の無い
// 一時変数に入って黙って捨てられていた。survey 2026-09-27)。
func (h *Hlc) checkAliasAssign(lhs *cexpr) {
	if name, ok := h.aliasNodes[lhs]; ok {
		panic(&diag.Error{Msg: fmt.Sprintf("cannot assign to for-each variable `%s` (it is read-only; it points to the current element)", name)})
	}
}

// checkLoopVarAssign は for-each の変数への代入をエラーにする。
func (h *Hlc) checkLoopVarAssign(left ir.Operand) {
	if root := ir.UnderlyingValue(left); root != nil && h.loopVars[root] {
		panic(&diag.Error{Msg: fmt.Sprintf("cannot assign to for-each variable `%s` (it is read-only; write the element through the index `A[i]` or iterate pointers with `for (var p in &A)`)", root.Name)})
	}
}

// emitForInLoop はループの本体を出す。
func (h *Hlc) emitForInLoop(s *syntax.ForInStmt, label *syntax.Ident, loop forInLoop, prologue func()) {
	step := func() {
		h.writeLoopVar(loop.i, func() {
			h.lval(cop2(opLoad, cv(loop.i), cop2(opAdd, cv(loop.i), cv(ir.NewIntLiteral("", ir.ValType(loop.i), 1)))))
		})
	}
	body := func() {
		if prologue != nil {
			prologue()
		}
		h.compileStatement(s.Body)
	}
	if !loop.wrap {
		// C 型 for と同じ形: begin: if (i < end) { body; step: i++ } else break; goto begin; end:
		stepLabel := ""
		if forHasContinue(s.Body, label) {
			stepLabel = h.newLabel("step")
		}
		labels := h.newLabels("begin", "end")
		h.pushBreakable(breakable{continueLabel: stepLabel, breakLabel: labels[1]})
		h.emit(&ir.Op{Code: ir.OpLabel, Label: labels[0]})
		ifl := h.newLabels("then", "else", "end")
		h.compileCond(cop2(opLt, cv(loop.i), cv(loop.end)), ifl[1], false)
		h.emit(&ir.Op{Code: ir.OpLabel, Label: ifl[0]})
		h.inScope(func() {
			body()
			if stepLabel != "" {
				h.emit(&ir.Op{Code: ir.OpLabel, Label: stepLabel})
			}
			step()
		})
		h.emit(&ir.Op{Code: ir.OpJump, Label: ifl[2]})
		h.emit(&ir.Op{Code: ir.OpLabel, Label: ifl[1]})
		h.emit(&ir.Op{Code: ir.OpJump, Label: labels[1]})
		h.emit(&ir.Op{Code: ir.OpLabel, Label: ifl[2]})
		h.emit(&ir.Op{Code: ir.OpJump, Label: labels[0]})
		h.emit(&ir.Op{Code: ir.OpLabel, Label: labels[1]})
		h.popBreakable()
		return
	}
	// 後ろで判定する形: if (!pre) goto end; begin: body; step: i++; if (i != end) goto begin; end:
	labels := h.newLabels("begin", "step", "end")
	if loop.pre != nil {
		h.compileCond(loop.pre, labels[2], false)
	}
	h.pushBreakable(breakable{continueLabel: labels[1], breakLabel: labels[2]})
	h.emit(&ir.Op{Code: ir.OpLabel, Label: labels[0]})
	h.inScope(body)
	h.emit(&ir.Op{Code: ir.OpLabel, Label: labels[1]})
	step()
	h.compileCond(cop2(opNe, cv(loop.i), cv(loop.end)), labels[0], true)
	h.emit(&ir.Op{Code: ir.OpLabel, Label: labels[2]})
	h.popBreakable()
}

// caseItem は switch の case の値 1 つ (値か、fc 3 の範囲 `lo..hi` / `lo..=hi`)。
type caseItem struct {
	v        ir.Operand // 範囲でなければ値
	isRange  bool
	lo, last int // 範囲の最初と最後 (last を含む)
}

// caseRange は `case lo..hi:` の範囲を評価する (両端は定数。enum なら `.A..=.C`)。
func (h *Hlc) caseRange(r *syntax.RangeExpr, t *types.Type) caseItem {
	bound := func(e syntax.Expr, what string) int {
		v := h.constEvalOperand(h.withExpected(toC(e), t))
		k, ok := ir.ValIntLiteral(v)
		if !ok {
			panic(&diag.Error{Msg: fmt.Sprintf("case range: %s must be an integer constant", what), Pos: syntax.At(h.module.Path, e.Pos())})
		}
		return k
	}
	lo, hi := bound(r.Lo, "lo"), bound(r.Hi, "hi")
	last := hi
	if !r.Inclusive {
		last--
	}
	if last < lo {
		panic(&diag.Error{Msg: fmt.Sprintf("case range %s is empty", rangeText(lo, hi, r.Inclusive)), Pos: syntax.At(h.module.Path, r.Op)})
	}
	if t.Kind == types.Int && t.Size <= 2 {
		if a, b := intRange(t); lo < a || last > b {
			panic(&diag.Error{Msg: fmt.Sprintf("case range %s does not fit in %s", rangeText(lo, hi, r.Inclusive), t), Pos: syntax.At(h.module.Path, r.Op)})
		}
	}
	return caseItem{isRange: true, lo: lo, last: last}
}

func rangeText(lo, hi int, incl bool) string {
	if incl {
		return fmt.Sprintf("%d..=%d", lo, hi)
	}
	return fmt.Sprintf("%d..%d", lo, hi)
}

// caseRangeTest は dst に「cond が範囲 it に入るか」を出す: 符号なしで (cond - lo) < 個数 (比較 1 回。`sec; sbc #lo; cmp #n`)。
// 範囲が型の全体なら常に真。
func (h *Hlc) caseRangeTest(dst *ir.Value, cond ir.Operand, it caseItem) {
	size := ir.ValType(cond).Size
	ut := h.prog.Types.IntType(size, false)
	count := it.last - it.lo + 1
	if count >= 1<<(8*size) {
		h.emit(&ir.Op{Code: ir.OpLoad, Dst: dst, Src: []ir.Operand{ir.NewIntLiteral("", ut, 1)}})
		return
	}
	u := ir.Operand(ir.NewCastedValue(cond, ut, 0))
	if it.lo != 0 {
		d := h.newTmp(ut)
		h.emit(&ir.Op{Code: ir.OpSub, Dst: d, Src: []ir.Operand{u, ir.NewIntLiteral("", ut, ir.FloorMod(it.lo, 1<<(8*size)))}})
		u = d
	}
	h.emit(&ir.Op{Code: ir.OpLt, Dst: dst, Src: []ir.Operand{u, ir.NewIntLiteral("", ut, count)}})
}

// staticPlace は評価済みの式 c が、変数を読まずに決まる置き場所 (変数そのもの・定数の添字・struct のフィールドをたどったもの)
// か。for-each が `&A[i]` の別名で回してよい (周ごとに評価し直しても同じ場所) かの判定。
func staticPlace(c *cexpr) bool {
	switch {
	case c.kind == cValue:
		v := c.val
		return (v.Kind == ir.KindGlobal || v.Kind == ir.KindLocal) && (v.Type.Kind == types.Array || v.Type.Kind == types.Struct)
	case c.kind == cOp && c.op == opField:
		return staticPlace(c.args[0])
	case c.kind == cOp && c.op == opIndex:
		return c.args[1].isLiteralInt() && staticPlace(c.args[0])
	}
	return false
}
