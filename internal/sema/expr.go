package sema

// 式の IR 化 (lval / rval)、代入、cast、型の互換。

import (
	"fmt"

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/syntax"
	"github.com/haramako/fc/internal/types"
)

// compatible は互換型を返す (なければ CompileError)。
func (h *Hlc) compatible(a, b *types.Type) *types.Type {
	r := h.prog.Types.Compatible(a, b)
	if r == nil {
		panic(&diag.Error{Msg: fmt.Sprintf("types %s and %s are not compatible", a, b)})
	}
	return r
}

// compatibleAssign は代入 (初期化・引数・戻り値も) の型検査。to が代入先。
func (h *Hlc) compatibleAssign(what string, to, from *types.Type) *types.Type {
	r := h.prog.Types.Compatible(to, from)
	if r == nil {
		panic(&diag.Error{Msg: fmt.Sprintf("%s: cannot assign %s to %s (not compatible types)", what, from, to)})
	}
	return r
}

// pointerElems はポインタ配列の const (`[N]*T`) の要素をアドレス (シンボルのリテラル) にする。
// 文字列 / 配列リテラルは無名の配列定数に切り出し、配列定数の名前 (グローバル) はそのシンボル、関数や整数 (null の 0) はそのまま。
func (h *Hlc) pointerElems(name string, arr *ir.Value, ptr *types.Type) *ir.Value {
	elems := make([]ir.Operand, len(arr.Elems))
	for i, e := range arr.Elems {
		ev := ir.ValLiteral(e)
		switch {
		case ev != nil && ev.Kind == ir.KindArrayLiteral:
			if !ev.IsString {
				h.compatibleAssign(fmt.Sprintf("element %d of `%s`", i, name), ptr, ev.Type)
			}
			h.strPtr(ev)
			sym := h.addDef(h.tmpName("_"), &ir.Def{Kind: ir.DefBlock, Type: ev.Type, Elems: ev.Elems})
			elems[i] = ir.NewSymbolLiteral("", ptr, sym)
		case ev != nil && ev.Kind == ir.KindGlobal && ev.Type.Kind == types.Array && ev.Symbol != "":
			h.compatibleAssign(fmt.Sprintf("element %d of `%s`", i, name), ptr, ev.Type)
			h.strPtr(ev)
			elems[i] = ir.NewSymbolLiteral("", ptr, ev.Symbol)
		case ev != nil && ev.Kind == ir.KindLiteral:
			elems[i] = e // 関数のシンボル、整数 (null)
		default:
			panic(&diag.Error{Msg: fmt.Sprintf("element %d of `%s`: constant address required (string, array constant, or function)", i, name)})
		}
	}
	return ir.NewArrayLiteral(arr.Name, h.prog.Types.ArrayOf(ptr, len(elems)), elems)
}

// guessType は宣言型 typ (省略可) と初期値 val から変数の型を決める。
func (h *Hlc) guessType(name string, typ *types.Type, val ir.Operand) *types.Type {
	if typ != nil {
		return h.compatibleAssign("`"+name+"`", typ, ir.ValType(val))
	}
	return ir.ValType(val)
}

// fitArrayLiteral は整数の配列リテラルを宣言の型 typ (`[N]T`、T は整数) に合わせる。要素の型は個々の値から推定して
// 統合したもの (`[11902, -3]` は uint16 と sint8 で uint16) なので、宣言があればそちらを優先し、全要素が T に収まるなら
// T の配列に作り直す。収まらない・整数の配列でないときはそのまま (compatibleAssign が報告する)。
func (h *Hlc) fitArrayLiteral(v *ir.Value, typ *types.Type) *ir.Value {
	// soa の型も Kind は Array だが配列ではなく、宣言がエラーだと Base が nil (fuzz で発覚)
	if typ == nil || typ.Kind != types.Array || typ.IsSoa || typ.Base.Kind != types.Int || v.Kind != ir.KindArrayLiteral {
		return v
	}
	vt := ir.ValType(v)
	if vt.Kind != types.Array || vt.Base.Kind != types.Int || vt.Base == typ.Base || (typ.Length >= 0 && typ.Length != len(v.Elems)) {
		return v
	}
	lo, hi := 0, 1<<(8*typ.Base.Size)-1
	if typ.Base.Signed {
		lo, hi = -(hi+1)/2, (hi+1)/2-1
	}
	elems := make([]ir.Operand, len(v.Elems))
	for i, e := range v.Elems {
		n, ok := ir.ValIntLiteral(e)
		if ok && v.IsString && typ.Base.Size == 1 {
			// 文字列はバイトの並びなので、型を書いた 1 バイトの配列 (`const S:[3]i8 = "\n\xff"`) ではその型で読む (255 は -1)。
			// fc 4 の文字列は u8 で、fc 3 → 4 の migrate が fc 3 の i8 の文字列定数を長さつきの [N]i8 にする
			n = wrapInt(n, typ.Base)
		}
		if !ok || n < lo || n > hi {
			return v
		}
		elems[i] = ir.NewIntLiteral("", typ.Base, n)
	}
	r := ir.NewArrayLiteral(v.Name, h.prog.Types.ArrayOf(typ.Base, len(elems)), elems)
	r.IsString, r.StrTerm, r.Str = v.IsString, v.StrTerm, v.Str
	return r
}

// ---------------------------------------------------------------
// 依存モジュール
// ---------------------------------------------------------------

// rval は右辺値として評価し、値を返す。
func (h *Hlc) rval(c *cexpr) ir.Operand {
	v, left := h.lval(c)
	return h.rvalOf(v, left)
}

// lvalValue は値が要る場所の lval。void 関数の呼び出しなど値を持たない式はエラーにする
// (lval は値の無い式に nil を返す。そのまま ValType などに渡すと落ちる: `f().x` `*f()` `&f()` が fuzz で発覚)。
func (h *Hlc) lvalValue(c *cexpr) (ir.Operand, bool) {
	v, left := h.lval(c)
	if v == nil {
		panic(noValueError())
	}
	return v, left
}

func noValueError() *diag.Error {
	return &diag.Error{Msg: "expression has no value (void function call used as a value)"}
}

// rvalOf は lval の結果 (v, left) を右辺値にする。
func (h *Hlc) rvalOf(v ir.Operand, left bool) ir.Operand {
	if v == nil {
		// void 関数の呼び出しなど値を持たない式を、値が要る場所 (条件・代入・引数) に書いた
		panic(noValueError())
	}
	if left {
		if ir.ValType(v).Kind == types.SoaRef {
			return h.soaGather(v)
		}
		if b := ir.ValType(v).Base; b.Kind == types.Array {
			// 配列の値はその番地 (ポインタ経由の配列フィールド `ta[i].arr` / `p.arr`): 要素へのポインタとして読み替える。
			// 中身を load_mem すると、それを番地として添字を足して別の場所を壊していた (-O 0 / -O 2 とも同じ値なので差分の
			// fuzz では見えず、生成器を広げるときの手計算で発覚)
			return ir.NewCastedValue(v, h.prog.Types.PointerTo(b.Base), 0)
		}
		r := h.newTmp(ir.ValType(v).Base)
		h.emit(ir.NewLoadMem(r, v, nil, 0, 0))
		return r
	}
	return v
}

// arrayValue は配列の値を置く所 (配列への代入・初期化・引数・return・フィールド、型を書かない var の初期値) で、型を決める段で
// 配列と分かる添字・フィールド・参照はがしの式 c を配列の値として評価する。ポインタの先の配列 (2 次元配列の行 `a[i]`、
// `p.arr`) は rvalOf が要素へのポインタに読み替えるので、ここで一時変数に写す (`var b = a[1]` が *u8 になり、
// `var c:[2]u8 = a[1]` がポインタの 2 バイトを写していた)。当てはまらなければ nil (c は評価しない)。
func (h *Hlc) arrayValue(c *cexpr) ir.Operand {
	if h.lmd == nil {
		return nil
	}
	if e := h.constEval(c); e.kind != cOp || e.op != opIndex && e.op != opField && e.op != opDeref {
		return nil
	}
	if info, ok := h.exprType(c); !ok || info.t.Kind != types.Array || info.t.IsSoa || info.t.Length < 0 {
		return nil
	}
	v, left := h.lvalValue(c)
	if b := ir.ValType(v).Base; left && b != nil && b.Kind == types.Array {
		tmp := h.newTmp(b)
		h.emit(ir.NewLoadMem(tmp, v, nil, 0, 0))
		return tmp
	}
	return h.rvalOf(v, left)
}

// lval は左辺値として評価し、(値, 左辺値かどうか) を返す。
func (h *Hlc) lval(c *cexpr) (ir.Operand, bool) {
	defer h.enterExpr(c.pos)()
	hint := h.wide // A1: この式を計算する幅 (算術の節点だけが使う。子には既定で渡さない)
	h.wide = 0
	defer func() { h.wide = 0 }() // 受け取った幅は使い切り (戻すと次の式に漏れて、型を書かない変数まで広い幅で計算していた)
	leftValue := false
	e := h.constEval(c)
	var r ir.Operand

	switch e.kind {

	case cEnumShort:
		panic(&diag.Error{Msg: fmt.Sprintf("cannot tell the enum type of .%s here (write Type.%s)", e.name, e.name)})

	case cValue:
		if e.val.Type.Kind == types.Bad {
			panic(&diag.Error{Suppressed: true}) // エラーになった宣言の参照: 報告済みなので黙って打ち切る
		}
		if e.val.Type.Kind == types.TypeName {
			panic(&diag.Error{Msg: fmt.Sprintf("%s is a type, not a value", e.val.Name)})
		}
		if e.val.Type.IsSoa {
			panic(&diag.Error{Msg: fmt.Sprintf("soa %s can only be indexed (%s[i]) or used as a type (*%s)", e.val.Name, e.val.Name, e.val.Name)})
		}
		if root := h.prog.storageAliases[e.val]; root != nil {
			r = ir.NewCastedValue(root, e.val.Type, 0)
		} else if e.val.Kind == ir.KindArrayLiteral {
			checkRaggedLiteral(e.val)
			symbol := h.addDef(h.tmpName("_"), &ir.Def{Kind: ir.DefBlock, Type: e.val.Type, Elems: e.val.Elems})
			g := ir.NewGlobal(h.tmpName("$"), e.val.Type, symbol)
			g.ReadOnly = true // 文字列・配列リテラルは ROM (fc 3 の *const)
			if e.val.IsString {
				h.prog.strLitGlobals[g] = e.val
			}
			r = g
		} else {
			r = e.val
		}

	case cOperand:
		r = e.opnd // 既に評価して IR を出した式 (resolveEnumShortPair)

	case cCast:
		v := h.rval(e.args[0])
		r = h.explicitCast(e.ck, v, e.ty)

	case cStructLit:
		// 実行時に組み立てる struct リテラル: 一時変数 (フレーム上) にフィールドごとに代入する
		if e.ty == nil {
			panic(&diag.Error{Msg: "struct literal without a type name needs a context that gives the type (declared type or assignment)"})
		}
		tmp := h.newTmp(e.ty)
		for _, f := range e.ty.Fields {
			dst := ir.NewCastedValue(tmp, f.Type, f.Offset)
			var v ir.Operand
			if fv := structLitField(e, f.Name); fv != nil {
				v = h.rvalAssign(fv, f.Type, fmt.Sprintf("field %s of %s", f.Name, e.ty), nil)
			} else {
				v = h.zeroValue(f.Type)
			}
			h.emit(&ir.Op{Code: ir.OpLoad, Dst: dst, Src: []ir.Operand{v}})
		}
		r = tmp

	case cArray:
		if !e.rt {
			panic(&diag.Error{Msg: "array literal with untyped struct literals needs a declared type"})
		}
		r = h.runtimeArray(e)

	case cNull:
		panic(&diag.Error{Msg: "null needs a context that gives the pointer type (assignment, comparison, argument, or `null as *T`)"})

	case cNullFn:
		r = h.nullFn(h.prog.Types.Func(nil, h.prog.Types.Void(), false))

	case cOp:
		switch e.op {

		case opLoad:
			h.checkAliasAssign(e.args[0])
			if lhs := e.args[0]; lhs.kind == cOp && lhs.op == opCall {
				// 呼び出しの結果は一時変数なので、そのまま進むと `g() = 0` が黙って通り、void なら nil 参照で落ちる (fuzz で発覚)
				panic(&diag.Error{Msg: "cannot assign to the result of a function call"})
			}
			if rhs := e.args[1]; rhs.kind == cOp && len(rhs.args) == 2 && rhs.args[0] == e.args[0] && containsCall(e.args[0]) {
				// 複合代入 `X op= v` は (load X (op X v)) に脱糖されていて X を 2 回評価する。X に関数呼び出しが
				// あるとき (`a[f()] += 1`) は呼び出しを先に 1 回だけ評価して値に置き換えてから続ける
				lhs := h.hoistCalls(e.args[0])
				inner := cop2(rhs.op, lhs, rhs.args[1])
				inner.compound, inner.compoundCall = rhs.compound, true
				e = &cexpr{kind: cOp, op: opLoad, args: []*cexpr{lhs, inner}, pos: e.pos}
			}
			if lhs := e.args[0]; lhs.kind == cOp && lhs.op == opField {
				// struct のフィールドへの代入。SoA の 2 バイト以上のフィールドは 1 つのポインタで表せないのでここで扱う
				fr := h.fieldRef(lhs.args[0], lhs.name)
				if fr.soaConst {
					panic(&diag.Error{Msg: "cannot assign to element of soa const"})
				}
				if fr.split != nil {
					right := h.rvalAssign(h.withExpected(e.args[1], fr.split.typ), fr.split.typ, "assignment to field "+lhs.name, nil)
					h.soaStoreSplit(fr.split, right)
					r = right
					if k, lit := ir.ValIntLiteral(right); lit && ir.ValType(right) != fr.split.typ {
						r = ir.NewIntLiteral("", fr.split.typ, wrapInt(k, fr.split.typ)) // 代入の式の値は左辺の型 (型のない定数のまま残っていた)
					}
					break
				}
				r = h.assign(fr.v, fr.lv, e.args[1])
				leftValue = fr.lv
				break
			}
			left, lv := h.lvalValue(e.args[0])
			r = h.assign(left, lv, e.args[1])
			leftValue = lv

		case opNot, opUminus, opBitNot:
			var w int
			var nt0 *types.Type
			if e.op != opNot {
				w, nt0 = h.wideWidth(e, hint)
				h.wide = w
			}
			left := h.rval(e.args[0])
			h.wide = 0
			if w > 0 && (w > nt0.Size || h.widened(e.args[0], left)) {
				// A1: 広い幅で計算する (項は広げる前の型の符号のまま広げる: widenOperand)
				t := h.prog.Types.IntType(w, nt0.Signed)
				tmp := h.newTmp(t)
				op := &ir.Op{Code: copToOpCode[e.op], Dst: tmp, Src: []ir.Operand{h.widenOperand(left, t)}}
				h.emit(op)
				h.recordArith(tmp, op, e)
				r = tmp
				break
			}
			typ := ir.ValType(left)
			checkEnumOp(e.op, typ, nil)
			checkOperandKinds(e.op, typ, nil)
			if typ.IsFarFunc() && e.op != opNot {
				panic(&diag.Error{Msg: "arithmetic is not supported on farfn"})
			}
			if e.op == opNot {
				typ = h.prog.Types.Bool() // `!x` は 0 / 1
			}
			tmp := h.newTmp(typ)
			op := &ir.Op{Code: copToOpCode[e.op], Dst: tmp, Src: []ir.Operand{left}}
			h.emit(op)
			if e.op != opNot {
				h.recordArith(tmp, op, e)
			}
			r = tmp

		case opAddWrap, opSubWrap, opMulWrap:
			left, right := e.args[0], e.args[1]
			_, lok := h.exprType(left)
			_, rok := h.exprType(right)
			if !lok || !rok {
				// 型を決める段がまだ扱わない項: 左から評価して値の型を使う (評価の順を保つため両方)
				left = &cexpr{kind: cOperand, opnd: h.rval(left), pos: left.pos, end: left.end}
				right = &cexpr{kind: cOperand, opnd: h.rval(right), pos: right.pos, end: right.end}
			}
			r = h.rval(h.wrapExpr(e, left, right))

		case opAdd, opSub, opMul, opDiv, opMod,
			opAnd, opOr, opXor, opShiftLeft, opShiftRight:
			w, nt0 := h.wideWidth(e, hint)
			h.wide = w
			left := h.rval(e.args[0])
			if e.op != opShiftLeft && e.op != opShiftRight {
				h.wide = w // シフト量は区切り (広げない)
			}
			right := h.rval(e.args[1])
			h.wide = 0
			if ir.ValType(left).IsFarFunc() || ir.ValType(right).IsFarFunc() {
				panic(&diag.Error{Msg: "arithmetic is not supported on farfn"})
			}
			checkEnumOp(e.op, ir.ValType(left), ir.ValType(right))
			checkOperandKinds(e.op, ir.ValType(left), ir.ValType(right))
			switch e.op {
			case opDiv, opMod:
				h.checkMixedUse(left, "`"+opSymbol(e.op)+"`")
				h.checkMixedUse(right, "`"+opSymbol(e.op)+"`")
			case opShiftRight:
				h.checkMixedUse(left, "`>>`")
			}
			if lt, rt := ir.ValType(left), ir.ValType(right); e.op == opSub && lt.Kind == types.Pointer && rt.Kind == types.Pointer && lt.Base == rt.Base {
				r = h.pointerDiff(left, right, lt.Base) // p - q は要素数 (C と同じ)
				break
			}
			if w > 0 && (w > nt0.Size || h.widened(e.args[0], left) || e.op != opShiftLeft && e.op != opShiftRight && h.widened(e.args[1], right)) {
				// A1: 広い幅で計算する (項は広げる前の型の符号のまま広げる: widenOperand。シフト量は元のまま)。子を広い幅で
				// 計算したときも、この節点の型は広げる前の型の符号 (子の広い型どうしで揃えると符号が変わる: `y + dy + w`)
				if k, lit := ir.ValIntLiteral(right); lit && k == 0 && (e.op == opDiv || e.op == opMod) {
					panic(&diag.Error{Msg: "div by 0"})
				}
				t := h.prog.Types.IntType(w, nt0.Signed)
				srcs := []ir.Operand{h.widenOperand(left, t), right}
				if e.op != opShiftLeft && e.op != opShiftRight {
					srcs[1] = h.widenOperand(right, t)
				}
				tmp := h.newTmp(t)
				op := &ir.Op{Code: copToOpCode[e.op], Dst: tmp, Src: srcs}
				h.emit(op)
				h.recordArith(tmp, op, e, left, right)
				r = tmp
				break
			}
			if (e.op == opShiftLeft || e.op == opShiftRight) && h.shiftByLeft(e, left, right) {
				// F1 (fc 4): シフトの結果は左辺の型。シフト量は型を揃えない
				typ := ir.ValType(left)
				tmp := h.newTmp(typ)
				op := &ir.Op{Code: copToOpCode[e.op], Dst: tmp, Src: []ir.Operand{left, right}}
				h.emit(op)
				h.recordArith(tmp, op, e, left)
				r = tmp
				break
			}
			origL, origR := left, right // 型のない定数の元の値 (A1 で広げるとき: widen.go)
			left, right = h.adaptLiteral(left, right, false)
			typ, l2, r2, cerr := h.tryMakeCompatible(left, right)
			if cerr != nil {
				if (e.op == opAdd || e.op == opSub) &&
					(ir.ValType(left).Kind == types.Pointer || ir.ValType(left).Kind == types.SoaRef) && ir.ValType(right).Kind == types.Int {
					if ir.ValType(left).Kind == types.Pointer && ir.ValType(left).Base.Kind == types.Void {
						panic(&diag.Error{Msg: "no arithmetic on *void"})
					}
					typ = ir.ValType(left)
					right = h.signedOffset(right) // p + i (i:i8 が負) は後ろへ
					if typ.Kind == types.Pointer && typ.Base.Size > 1 {
						// p + n / p++ は要素 n 個分進める (C と同じ。docs/reference/language.md の「ポインタ」)。バイト単位で進めていて、u16 の配列を
						// p++ でたどると 1 バイトずれ、p[1] と *(p + 1) が違っていた
						right = h.scaleOffset(right, typ.Base.Size)
					}
				} else {
					panic(&diag.Error{Msg: fmt.Sprintf("cannot apply %s to %s and %s (not compatible types)", opSymbol(e.op), ir.ValType(left), ir.ValType(right))})
				}
			} else {
				left, right = l2, r2
			}
			if k, lit := ir.ValIntLiteral(right); lit && k == 0 && (e.op == opDiv || e.op == opMod) {
				panic(&diag.Error{Msg: "div by 0"})
			}
			tmp := h.newTmp(typ)
			if typ.Kind == types.Pointer {
				h.markReadOnly(tmp, h.readOnly(left)) // `p + 1` (p:*const T) も読み取り専用 (外れて書き込めていた。survey 2026-09-27)
			}
			op := &ir.Op{Code: copToOpCode[e.op], Dst: tmp, Src: []ir.Operand{left, right}}
			h.emit(op)
			if typ.Kind == types.Int {
				h.recordArith(tmp, op, e, origL, origR)
			}
			r = tmp

		case opEq, opLt:
			a0, a1 := e.args[0], e.args[1]
			if a0.kind == cNull && a1.kind != cNull {
				a0, a1 = a1, a0 // `null == p` も `p == null` と同じ
			}
			var left, right ir.Operand
			if a0.kind == cEnumShort && a1.kind != cEnumShort {
				// `.A == x` / `.A < x`: 相手の型で .A を決める (.A は定数なので評価の順は変わらない)
				right = h.rval(a1)
				left = h.rval(h.withExpected(a0, ir.ValType(right)))
			} else {
				w := h.compareWidth(a0, a1) // A1: 両辺を広いほうの幅で計算する
				h.wide = w
				left = h.rval(a0)
				if a1.kind == cNull {
					right = h.nullOf(ir.ValType(left))
				} else {
					h.wide = w
					right = h.rval(h.withExpected(a1, ir.ValType(left)))
				}
				h.wide = 0
			}
			checkEnumOp(e.op, ir.ValType(left), ir.ValType(right))
			if e.op == opLt {
				h.checkMixedUse(left, "comparing it")
				h.checkMixedUse(right, "comparing it")
			}
			// F6 (fc 4): 符号の違う整数の大小の比較で、互換型が片方の値を読み替えるものはエラー (intrules.go)。A1 で広い幅で
			// 計算した辺は、広げる前の型で見る (`id_i16(100) >= (id_u8(255) << 3)` は i16 と u8 で、読み替えは起きない)
			nl, nr := h.narrowView(a0, left), h.narrowView(a1, right)
			mixed := e.op == opLt && mixedSign(nl, nr)
			if mixed && h.v4() {
				panic(h.mixedSignError(ir.ValType(nl), ir.ValType(nr)))
			}
			left, right = h.adaptLiteral(left, right, true)
			h.warnConstCompare(e.op, left, right)
			if k, ok := h.foldBeyond16(e.op, left, right); ok {
				r = ir.NewIntLiteral("", h.prog.Types.Bool(), k)
				break
			}
			if e.op == opLt {
				checkOperandKinds(e.op, ir.ValType(left), ir.ValType(right)) // == / != は struct・配列でもよい (バイトの比較)
			}
			if v, ok := left.(*ir.Value); ok && ir.ValType(right).IsFarFunc() {
				left = h.rval(h.withExpected(cv(v), ir.ValType(right)))
			}
			if e.op == opLt && (ir.ValType(left).IsFarFunc() || ir.ValType(right).IsFarFunc()) {
				panic(&diag.Error{Msg: "ordered comparison is not supported on farfn"})
			}
			if e.op == opEq && isVoidPtr(ir.ValType(right)) && !isVoidPtr(ir.ValType(left)) {
				left, right = right, left // *void との == は向きを問わない (Compatible は *void を左に置く)
			}
			if mixed && h.rewriting() {
				// 互換型に揃える (A1 の書き換え) の後に報告する: 同じ式に両方が付くとき `((x + vx) as i8) as u16` の順に当たる
				l0, r0 := left, right
				_, left, right = h.makeCompatible(left, right)
				h.rewriteMixedSign(l0, r0, [2]*cexpr{a0, a1})
			} else {
				_, left, right = h.makeCompatible(left, right)
			}
			if e.op == opEq {
				if bv, ok := h.boolEq(e, left, right); ok {
					r = bv
					break
				}
				if lt := ir.ValType(left); (lt.Kind == types.Array || lt.Kind == types.Struct) && lt.Size == 0 {
					// 大きさ 0 どうし (fc 4 の空の文字列 `"" == ""` は [0]u8): 比べるバイトが無いので等しい (幅の無い比較の命令を
					// 出して IR の検査で落ちていた: TestRandomStringsV4)
					r = ir.NewIntLiteral("", h.prog.Types.Bool(), 1)
					break
				}
			}
			tmp := h.newTmp(h.prog.Types.Bool()) // 比較の結果は bool (uint8 と互換。docs/reference/language.md の「型」)
			h.emit(&ir.Op{Code: copToOpCode[e.op], Dst: tmp, Src: []ir.Operand{left, right}})
			r = tmp

		case opNe, opGt, opLe, opGe:
			// これらは、eq,lt の引数の順番とnotを組合せて合成する
			left := e.args[0]
			right := e.args[1]
			switch e.op {
			case opNe:
				r = h.rval(cop2(opNot, cop2(opEq, left, right)))
			case opGt:
				r = h.rval(cop2(opLt, right, left))
			case opLe:
				r = h.rval(cop2(opNot, cop2(opLt, right, left)))
			case opGe:
				r = h.rval(cop2(opNot, cop2(opLt, left, right)))
			}

		case opCond:
			r = h.condValue(e, hint)

		case opLand, opLor:
			// 値として使う `a && b` / `a || b` は 0 / 1 (条件文脈では compileCond が分岐に展開する)
			endLabel := h.newLabel("end")
			boolT := h.prog.Types.Bool()
			rr := h.newTmp(boolT)
			h.emit(&ir.Op{Code: ir.OpLoad, Dst: rr, Src: []ir.Operand{ir.NewIntLiteral("", boolT, 0)}})
			h.compileCond(e, endLabel, false)
			h.emit(&ir.Op{Code: ir.OpLoad, Dst: rr, Src: []ir.Operand{ir.NewIntLiteral("", boolT, 1)}})
			h.emit(&ir.Op{Code: ir.OpLabel, Label: endLabel})
			r = rr

		case opCall:
			if e.args[0].kind == cNullFn {
				panic(&diag.Error{Msg: "@null_fn does nothing; remove the call (it is a value for function pointers)"})
			}
			lmdV := h.rval(e.args[0])
			args := e.args[1:]
			if ir.ValType(lmdV).Kind == types.Macro {
				// マクロの実行
				x := h.expandMacro(e, ir.ValLiteral(lmdV))
				if x.stmts != nil {
					for _, st := range x.stmts {
						h.lval(st)
					}
				} else if x.expr != nil {
					r = h.rval(x.expr)
				}
			} else {
				// 普通の関数コール
				lmdType := ir.ValType(lmdV)
				if lmdType.Kind != types.Func {
					// 関数でない値の呼び出し (名前が同じ変数に取られて関数の宣言がエラーになったときなど)。以前は
					// lmdType.Base (nil) を見てコンパイラが panic していた (fuzz の生成器の名前の衝突で発覚)
					panic(&diag.Error{Msg: fmt.Sprintf("cannot call %s: type %s is not a function", describe(lmdV), lmdType)})
				}
				args = h.fillDefaultArgs(lmdV, args)
				if lmdType.IsFarFunc() {
					if !h.prog.FarCallEnabled() {
						panic(&diag.Error{Msg: "farfn calls require options(farcall: true)"})
					}
					// Snapshot the callee before argument evaluation (which can change it).
					lmdV = h.freeze(lmdV)
				}
				if lmdType.Base.Kind != types.Void {
					r = h.newTmp(lmdType.Base)
				}
				if len(args) != len(lmdType.Params) {
					panic(&diag.Error{Msg: fmt.Sprintf("%s expects %d argument(s) but %d given", describe(lmdV), len(lmdType.Params), len(args))})
				}
				// 引数に呼び出しを含むときは、全部評価してから積む。積んでいる途中で別の呼び出しが走ると、呼び先の引数領域
				// (FC_FASTCALL_REG や、静的フレームなら呼び先と重なりうる兄弟のフレーム) が壊れるので。後ろの引数に
				// 呼び出しがあるときは、先に評価した値を一時変数に写して左から右の評価順を保つ。
				// 呼び出しを含まなければ評価しながら積む (`sub t; push_arg t` が隣り合い、t が A に割り付く)
				pre := containsCallAny(args)
				evalArg := func(i int) ir.Operand {
					pt := lmdType.Params[i]
					return h.rvalAssign(h.withExpected(args[i], pt), pt, fmt.Sprintf("argument %d of %s", i+1, describe(lmdV)), nil)
				}
				argVals := make([]ir.Operand, len(args))
				if pre {
					for i := range args {
						v := evalArg(i)
						if containsCallAny(args[i+1:]) {
							v = h.freeze(v)
						}
						argVals[i] = v
					}
				}
				pushCode, argCode, callCode := ir.OpPushResult, ir.OpPushArg, ir.OpCall
				if lmdType.Fastcall() {
					if h.fastCalling {
						panic(&diag.Error{Msg: "cannot fastcall in fastcalling"})
					}
					h.fastCalling = true
					pushCode, argCode, callCode = ir.OpPushFastcallResult, ir.OpPushFastcallArg, ir.OpFastcall
				}
				h.emit(&ir.Op{Code: pushCode, Type: lmdType.Base})
				for i := range args {
					v := argVals[i]
					if !pre {
						v = evalArg(i)
					}
					h.emit(&ir.Op{Code: argCode, Type: lmdType.Params[i], Src: []ir.Operand{v}})
				}
				h.emit(&ir.Op{Code: callCode, Dst: r, Src: []ir.Operand{lmdV}, Far: h.isFarCall(lmdV)})
				h.fastCalling = false
			}

		case opRef: // &演算子
			var left ir.Operand
			var lv bool
			if a := e.args[0]; a.kind == cOp && a.op == opField {
				fr := h.fieldRef(a.args[0], a.name)
				if fr.split != nil {
					panic(&diag.Error{Msg: fmt.Sprintf("cannot take the address of soa field %s (2 bytes or more: stored as separate byte arrays)", a.name)})
				}
				left, lv = fr.v, fr.lv
			} else {
				left, lv = h.lvalValue(a)
			}
			if lv {
				r = left
			} else {
				if !ir.ValAssignable(left) {
					panic(&diag.Error{Msg: fmt.Sprintf("cannot take the address of %s (not a variable)", describe(left))})
				}
				tmp := h.newTmp(h.prog.Types.PointerTo(ir.ValType(left)))
				h.markReadOnly(tmp, h.readOnly(left))
				h.emit(&ir.Op{Code: ir.OpRef, Dst: tmp, Src: []ir.Operand{left}})
				r = tmp
			}

		case opDeref: // *演算子
			if v, lv := h.lvalValue(e.args[0]); lv && ir.ValType(v).Kind == types.SoaRef {
				// `*Points[i]`: 要素 (左辺値) の参照はがしは要素そのもの
				r = v
				leftValue = true
				break
			} else if lv {
				r = h.newTmp(ir.ValType(v).Base)
				h.emit(ir.NewLoadMem(r, v, nil, 0, 0))
			} else {
				r = v
			}
			if ir.ValType(r).Kind == types.Pointer && ir.ValType(r).Base.Kind == types.Void {
				panic(&diag.Error{Msg: "cannot dereference *void (bitcast to a typed pointer first)"})
			}
			if ir.ValType(r).Kind != types.Pointer && ir.ValType(r).Kind != types.SoaRef {
				panic(&diag.Error{Msg: fmt.Sprintf("cannot dereference %s (type %s is not a pointer)", describe(r), ir.ValType(r))})
			}
			leftValue = true

		case opMin, opMax, opClamp:
			// 組み込みの min / max / clamp: 互換型の一時変数に入れて、比較して入れ替える
			//   min: t = a; if (b < a) t = b     max: t = a; if (a < b) t = b
			//   clamp: t = x; if (t < lo) t = lo; if (hi < t) t = hi
			vals := make([]ir.Operand, len(e.args))
			for i, a := range e.args {
				vals[i] = h.rval(a)
			}
			for i := 1; i < len(vals); i++ {
				// 比較なので、型のない定数は比較と同じ規則 (`@min(s, 200)` (s:i8) は 200 が収まらずエラー。-56 と比べていた)
				vals[0], vals[i] = h.adaptLiteral(vals[0], vals[i], true)
			}
			typ := ir.ValType(vals[0])
			for _, v := range vals[1:] {
				typ = h.compatible(typ, ir.ValType(v))
			}
			if typ.Kind != types.Int && typ.Kind != types.Bool {
				panic(&diag.Error{Msg: fmt.Sprintf("%s: arguments must be integers (got %s)", e.op, typ)})
			}
			for i := range vals {
				vals[i] = h.cast(vals[i], typ)
			}
			tmp := h.newTmp(typ)
			h.emit(&ir.Op{Code: ir.OpLoad, Dst: tmp, Src: []ir.Operand{vals[0]}})
			replaceIf := func(a, b ir.Operand, with ir.Operand) { // if (a < b) tmp = with
				c := h.newTmp(h.prog.Types.IntType(1, false))
				end := h.newLabel("end")
				h.emit(&ir.Op{Code: ir.OpLt, Dst: c, Src: []ir.Operand{a, b}})
				h.emit(&ir.Op{Code: ir.OpIf, Src: []ir.Operand{c}, Label: end})
				h.emit(&ir.Op{Code: ir.OpLoad, Dst: tmp, Src: []ir.Operand{with}})
				h.emit(&ir.Op{Code: ir.OpLabel, Label: end})
			}
			switch e.op {
			case opMin:
				replaceIf(vals[1], tmp, vals[1])
			case opMax:
				replaceIf(tmp, vals[1], vals[1])
			case opClamp:
				replaceIf(tmp, vals[1], vals[1])
				replaceIf(vals[2], tmp, vals[2])
			}
			r = tmp

		case opField: // struct のフィールド参照 a.f (a が struct へのポインタなら自動で参照はがし)
			fr := h.fieldRef(e.args[0], e.name)
			if fr.split != nil {
				r = h.soaGatherSplit(fr.split)
			} else {
				r, leftValue = fr.v, fr.lv
			}

		case opIndex: // []演算子
			if a := e.args[0]; a.kind == cValue && a.val.Type.IsSoa {
				// `Points[i]`: SoA コンテナの添字はハンドルを作るだけ
				r = h.soaIndex(a.val.Type, h.rval(e.args[1]))
				leftValue = true
				break
			}
			left := h.rval(e.args[0])
			right := h.rval(e.args[1])
			if st := ir.ValType(left); st.IsSlice() {
				// s[i] は s.ptr[i] (範囲の検査はしない)
				left = ir.NewCastedValue(left, h.prog.Types.PointerTo(st.SliceOf), 0)
			}
			if ir.ValType(left).Kind != types.Pointer && ir.ValType(left).Kind != types.Array {
				panic(&diag.Error{Msg: fmt.Sprintf("cannot index %s (type %s is not a pointer or array)", describe(left), ir.ValType(left))})
			}
			if ir.ValType(left).Base.Kind == types.Void {
				panic(&diag.Error{Msg: "cannot index *void (bitcast to a typed pointer first)"})
			}
			if ir.ValType(right).Kind != types.Int && ir.ValType(right).Kind != types.Bool {
				panic(&diag.Error{Msg: fmt.Sprintf("index must be an integer (got %s)", ir.ValType(right))})
			}
			if ir.ValType(left).Kind == types.Pointer {
				right = h.signedOffset(right) // p[-1]、p[i] (i:i8) は負のずれ
			}
			if at := ir.ValType(left); at.Kind == types.Array && at.Length > 0 {
				// 配列の定数の添字は長さの中 (範囲外の読み書きは隣の変数を黙って壊す。slice・ポインタは長さが分からないので見ない)
				if k, ok := ir.ValIntLiteral(right); ok && (k < 0 || k >= at.Length) {
					panic(&diag.Error{Msg: fmt.Sprintf("index %d is out of range for %s (type %s: 0..%d)", k, describe(left), at, at.Length-1)})
				}
			}
			tmp := h.newTmp(h.prog.Types.PointerTo(ir.ValType(left).Base))
			h.markReadOnly(tmp, h.readOnly(left))
			h.emit(&ir.Op{Code: ir.OpIndex, Dst: tmp, Src: []ir.Operand{left, right}})
			r = tmp
			leftValue = true

		case opSlice:
			r = h.sliceRange(e.args[0], e.args[1], e.args[2], e.incl)

		case opToSlice:
			r = h.toSlice(e.args[0], e.ty)

		case opLen:
			if p := h.sliceParts(e.args[0], "@len"); p.n >= 0 {
				r = h.IntValue(p.n)
			} else {
				r = p.len
			}

		default:
			panic(fmt.Sprintf("unknown op %s", e.op))
		}
	default:
		panic(fmt.Sprintf("unknown expression kind %d", e.kind))
	}
	h.checkExprType(c, r, leftValue)
	return r, leftValue
}

// assign は代入 `left = rhs` (left は評価済みの左辺、lv は左辺値 (ポインタ) かどうか)。代入した値 (左辺) を返す。
func (h *Hlc) assign(left ir.Operand, lv bool, rhs *cexpr) ir.Operand {
	if h.needsExpected(rhs) || ir.ValType(left).IsFarFunc() || (lv && ir.ValType(left).Base.IsFarFunc()) || ir.ValType(left).IsSlice() || (lv && ir.ValType(left).Base.IsSlice()) {
		// `p = {1, 2}`: 左辺の型で struct リテラルの型を決める
		lt := ir.ValType(left)
		if lv {
			lt = lt.Base
		}
		rhs = h.withExpected(rhs, lt)
	}
	h.checkLoopVarAssign(left)
	dt := ir.ValType(left) // 代入先の型 (A1 の幅、E・D の判定)
	if lv {
		dt = dt.Base
	}
	// 左辺だけで決まる検査を先に、型の照合 (assignPre) と E・D を評価の前に (rvalAssign と同じ)
	what := "assignment"
	if !lv {
		what = "assignment to " + describe(left)
	}
	soa := lv && ir.ValType(left).Kind == types.SoaRef
	if lv && !soa && h.readOnly(left) {
		panic(&diag.Error{Msg: "cannot assign through a read-only pointer (*const) or to const data"})
	}
	pre := !soa && h.assignPre(what, rhs, dt)
	if !soa && (lv || ir.ValAssignable(left)) && h.condAssign(left, lv, dt, rhs, what, pre) {
		return left // `x = c ? a : b` は枝ごとの代入 (cond.go)
	}
	var right ir.Operand
	checked := false
	if dt.Kind == types.Array && !dt.IsSoa {
		right = h.arrayValue(rhs) // `d = a[1]` (2 次元配列の行): 行の写し
	}
	if right == nil {
		right, checked = h.rvalPreConv(rhs, dt, !soa)
	}
	if soa {
		h.soaScatter(left, right)
		return left
	}
	h.assignPost(what, dt, right, pre)
	if lv {
		h.warnDropConst(what, dt, right)
		right = h.convertValue(right, dt, rhs, checked)
		h.emit(ir.NewStoreMem(left, nil, 0, 0, dt.Size, right))
		return left
	}
	if !h.readOnly(left) { // 代入先が *const の変数 (IR の型は *T) なら読み取り専用のデータを入れてよい
		h.warnDropConst(what, dt, right)
	}
	if !ir.ValAssignable(left) {
		panic(&diag.Error{Msg: fmt.Sprintf("cannot assign to %s (not a variable)", describe(left))})
	}
	right = h.convertValue(right, dt, rhs, checked)
	// A typed storage alias can expose overlapping struct subobjects. Preserve
	// the complete RHS before writing when a forward byte copy would overlap.
	if root := ir.UnderlyingValue(left); root != nil && root == ir.UnderlyingValue(right) {
		a, b, size := ir.ValOffset(left), ir.ValOffset(right), ir.ValType(left).Size
		if a != b && a < b+size && b < a+size {
			snapshot := h.newTmp(ir.ValType(right))
			h.emit(&ir.Op{Code: ir.OpLoad, Dst: snapshot, Src: []ir.Operand{right}})
			right = snapshot
		}
	}
	h.emit(&ir.Op{Code: ir.OpLoad, Dst: left, Src: []ir.Operand{right}})
	return left
}

// containsCall は式 (評価済みでもよい) に関数呼び出し (マクロ呼び出しも含む) が含まれるか。
func containsCall(c *cexpr) bool {
	if c == nil {
		return false
	}
	if c.kind == cOp && c.op == opCall {
		return true
	}
	for _, a := range c.args {
		if containsCall(a) {
			return true
		}
	}
	for _, f := range c.flds {
		if containsCall(f.val) {
			return true
		}
	}
	return false
}

// containsCallAny は式のどれかが関数呼び出しを含むか。
func containsCallAny(cs []*cexpr) bool {
	for _, c := range cs {
		if containsCall(c) {
			return true
		}
	}
	return false
}

// freeze は値を「今の値」に固定する (変数なら一時変数に写す。一時変数・リテラルはそのまま)。
func (h *Hlc) freeze(v ir.Operand) ir.Operand {
	if val, ok := v.(*ir.Value); ok {
		if val.Kind == ir.KindLiteral || (val.Kind == ir.KindLocal && val.LocalType == ir.LTTemp) {
			return v
		}
	}
	tmp := h.newTmp(ir.ValType(v))
	h.emit(&ir.Op{Code: ir.OpLoad, Dst: tmp, Src: []ir.Operand{v}})
	return tmp
}

// hoistCalls は式の中の関数呼び出しを今ここで評価し、その値 (cValue) に置き換えた式を返す (呼び出しが無ければそのまま)。
func (h *Hlc) hoistCalls(c *cexpr) *cexpr {
	if !containsCall(c) {
		return c
	}
	if c.kind == cOp && c.op == opCall {
		return cv(h.operandValue(h.rval(c)))
	}
	n := *c
	n.args = make([]*cexpr, len(c.args))
	for i, a := range c.args {
		n.args[i] = h.hoistCalls(a)
	}
	if len(c.flds) > 0 {
		n.flds = make([]cfield, len(c.flds))
		for i, f := range c.flds {
			n.flds[i] = f
			n.flds[i].val = h.hoistCalls(f.val)
		}
	}
	return &n
}

// operandValue はオペランドを *ir.Value にする (CastedValue などは一時変数に写す)。マクロが cexpr の値として使うため。
func (h *Hlc) operandValue(v ir.Operand) *ir.Value {
	if val, ok := v.(*ir.Value); ok {
		return val
	}
	tmp := h.newTmp(ir.ValType(v))
	h.emit(&ir.Op{Code: ir.OpLoad, Dst: tmp, Src: []ir.Operand{v}})
	return tmp
}

// cast は v を type にキャストする (必要ならコードも生成)。
func (h *Hlc) cast(v ir.Operand, typ *types.Type) ir.Operand {
	if typ.Kind == types.Int {
		// int の変換
		if typ == ir.ValType(v) {
			return v
		}
		if typ.Size <= ir.ValType(v).Size {
			return v
		}
		if !ir.ValType(v).Signed {
			return v
		}
		if k, ok := ir.ValIntLiteral(v); ok {
			// 符号付きのリテラルを広げる: 目的の型の値のリテラルにする (i8 のまま残すと、SSA の定数の評価が「どちらかが
			// 符号付きなら符号付き」で比べて、変数の符号拡張と結果が違った: `(-7 as i8) >= (0 as u16)`。TestRandomConstFold)
			return ir.NewIntLiteral("", typ, wrapInt(wrapInt(k, ir.ValType(v)), typ)) // 読み替えた型 (@bitcast の cast) で読んでから
		}
		if ir.ValKind(v) == ir.KindLiteral {
			return v
		}
		newV := h.newTmp(typ)
		h.emit(&ir.Op{Code: ir.OpSignExtension, Dst: newV, Src: []ir.Operand{v}})
		return newV
	} else if typ.Kind == types.Pointer && ir.ValType(v).Kind == types.Array && (ir.ValType(v).Base == typ.Base || typ.Base.Kind == types.Void) {
		h.strPtr(v)
		return ir.NewPointeredArray(v, h.prog.Types.PointerTo(ir.ValType(v).Base))
	}
	return v
}

// isVoidPtr は *void か。
func isVoidPtr(t *types.Type) bool { return t.Kind == types.Pointer && t.Base.Kind == types.Void }

// nullOf は型 t (ポインタ / 関数ポインタ) の null (0 のリテラル)。SoA のハンドルは 0 が有効な要素なので null を持たない。
func (h *Hlc) nullOf(t *types.Type) ir.Operand {
	switch t.Kind {
	case types.Pointer, types.Func:
		return ir.NewIntLiteral("", t, 0)
	case types.SoaRef:
		panic(&diag.Error{Msg: fmt.Sprintf("soa handle %s has no null (index 0 is a valid element)", t)})
	}
	panic(&diag.Error{Msg: fmt.Sprintf("null cannot be used as %s", t)})
}

// checkCast はキャストの種類ごとの規則を検査する (Agent/wiki/design/types-struct.md §3.5)。
func (h *Hlc) checkCast(kind syntax.CastKind, from, to *types.Type) {
	switch kind {
	case syntax.CastAs:
		// 数値変換: 整数 → 整数、配列 → 同じ要素型のポインタ
		if (from.Kind == types.Int || from.Kind == types.Bool) && (to.Kind == types.Int || to.Kind == types.Bool) {
			return
		}
		if from.Kind == types.Array && to.Kind == types.Pointer && (from.Base == to.Base || to.Base.Kind == types.Void) {
			return
		}
		if (from.Kind == types.Int && to.Kind == types.SoaRef) || (from.Kind == types.SoaRef && to.Kind == types.Int) {
			return // SoA のハンドルはインデックス (整数) と相互に変換できる
		}
		panic(&diag.Error{Msg: fmt.Sprintf("cannot convert %s to %s with `as` (use bitcast for bit reinterpretation)", from, to)})
	case syntax.CastBit:
		// ビット読み替え: サイズが同じもの同士 (配列はポインタ = 2 バイトとみなす)
		fromSize := from.Size
		if from.Kind == types.Array {
			fromSize = 2
		}
		if to.Kind == types.Array || to.Kind == types.Void || from.Kind == types.Void {
			panic(&diag.Error{Msg: fmt.Sprintf("cannot bitcast %s to %s", from, to)})
		}
		if fromSize != to.Size {
			panic(&diag.Error{Msg: fmt.Sprintf("cannot bitcast %s (%d bytes) to %s (%d bytes): sizes differ", from, fromSize, to, to.Size)})
		}
	}
}

// explicitCast は明示キャストの値を作る。
//   - v1 `<T>x` と `bitcast<T>(x)`: 型ラベルの貼り替え (CastedValue)
//   - `x as T`: 数値変換。拡張は元が符号付きなら符号拡張、縮小は下位バイト、同サイズはビットそのまま
func (h *Hlc) explicitCast(kind syntax.CastKind, v ir.Operand, to *types.Type) ir.Operand {
	from := ir.ValType(v)
	h.checkCast(kind, from, to)
	if from.Kind == types.Array && to.Kind == types.Pointer {
		h.strPtr(v)
	}
	if from.Kind == types.Int && to.Kind == types.Int && to.Size > from.Size {
		h.checkMixedUse(v, "widening it to "+to.String())
	}
	if kind == syntax.CastAs && from.Kind == types.Pointer && from.ReadOnly && to.Kind == types.Pointer && !to.ReadOnly {
		panic(&diag.Error{Msg: fmt.Sprintf("cannot drop const with `as` (%s to %s); use @bitcast(%s, x)", from, to, to)})
	}
	if kind == syntax.CastAs && to.Kind == types.Pointer && !to.ReadOnly {
		h.warnDropConst("`as`", to, v)
	}
	if kind == syntax.CastAs {
		if from.Kind == types.Array {
			return ir.NewPointeredArray(v, to)
		}
		if tv, ok := v.(*ir.Value); ok && from == to && to.Kind == types.Int && tv.Kind == ir.KindLiteral && tv.IsInt && tv.Untyped {
			// 型のない定数 (`@min(0, -6)` の結果など) への同じ型の `as` は、型付きの定数にする (値そのものを返していて、
			// `(@min(0, -6) as i8) + x` (x:u8) が型のない -6 として u8 に合わせられ 253 になっていた。定数の経路 (constEval) は
			// 型付きになる。sema/typing.go の検査で発覚)
			return ir.NewIntLiteral("", to, tv.Int)
		}
		if tv, ok := v.(*ir.Value); ok && from == to && to.Kind == types.Int {
			// 同じ型への `as` は値そのもの (包まない。fc 3 → 4 の migrate が足す `(式) as T` で IR が変わらないように、どの版も)。
			// `as` は A1 の区切りなので、式の中の算術の結果なら広げない印にする (widen.go)
			delete(h.arith, tv)
			return v
		}
		if to.Size > from.Size && from.Signed {
			newV := h.newTmp(to)
			h.emit(&ir.Op{Code: ir.OpSignExtension, Dst: newV, Src: []ir.Operand{v}})
			return newV
		}
	}
	c := ir.NewCastedValue(v, to, 0)
	if kind == syntax.CastBit && h.prog.unconst != nil {
		h.prog.unconst[c] = true // @bitcast は const を外す (読み取り専用にしない)
	}
	return c
}

// makeCompatible は互換型に変換する (キャストコード生成込み)。
func (h *Hlc) makeCompatible(a, b ir.Operand) (*types.Type, ir.Operand, ir.Operand) {
	typ := h.compatible(ir.ValType(a), ir.ValType(b))
	a, b = h.widenArith(a, typ), h.widenArith(b, typ) // A1: 16 ビットの値と出会う 8 ビットの算術は部分木ごと広げる (widen.go)
	a = h.cast(a, typ)
	b = h.cast(b, typ)
	return typ, a, b
}

// tryMakeCompatible は makeCompatible の CompileError を捕捉するバージョン。
func (h *Hlc) tryMakeCompatible(a, b ir.Operand) (typ *types.Type, ra, rb ir.Operand, err *diag.Error) {
	defer func() {
		if r := recover(); r != nil {
			if ce, ok := r.(*diag.Error); ok {
				err = ce
				return
			}
			panic(r)
		}
	}()
	typ, ra, rb = h.makeCompatible(a, b)
	return
}

// signedOffset は、ポインタに足す符号付きの 1 バイトの値 (添字・ずれ) を 2 バイトに符号拡張する。1 バイトのまま
// アドレスの下位に足すと上位の桁を借りず、`p[-1]` や `p[i]` (i:i8 = -2) が 255 / 254 先を読んでいた (survey 2026-09-27)。
func (h *Hlc) signedOffset(v ir.Operand) ir.Operand {
	t := ir.ValType(v)
	if t.Kind != types.Int || !t.Signed || t.Size != 1 {
		return v
	}
	i16 := h.prog.Types.IntType(2, true)
	if k, ok := ir.ValIntLiteral(v); ok {
		return ir.NewIntLiteral("", i16, k)
	}
	return h.cast(v, i16)
}
