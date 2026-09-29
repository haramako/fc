package sema

// A1 (fc 4): 式全体を「代入先の型」と「式の中で一番広い型」の広いほうで計算する (Agent/wiki/plans/v4-plan.md §1.3 A)。
//
// sema は式を下から IR にするので、8 ビットの算術の結果が 16 ビットの値と出会う所 (型を揃える makeCompatible と、代入のような
// 変換 convert) で初めて広げるべきと分かる。そこで、その結果 (一時変数) を作った部分木の命令を、その場で 16 ビットの計算に
// 直す (widen): 命令の結果の型を広げ、葉の符号付きの値は符号拡張の命令を前に足し (符号なしは 0 で広がる)、型のない定数は元の
// 値から広い型で作り直す。部分木の区切りは算術の命令 (+ - * / % & | ^ << >> 単項の - ~) 以外のすべて (変数・呼び出し・読み出し・
// `as`・比較・シフト量)。計算の符号は今の結果の型のまま (同じ大きさなら符号付きが勝つ: C0)。
//
// fc 3 のモジュールで書き換えを集めているとき (rewriting) は広げず、fc 4 で広がる部分木に `(式) as T` (T は今の型) を足す
// 書き換えを報告する (`as` は区切りなので、fc 4 でも中は今の幅で計算される)。

import (
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/types"
)

// arithNode は式の中の算術の命令の結果 (一時変数) の記録。
type arithNode struct {
	op   *ir.Op
	c    *cexpr       // 元の式 (書き換えの位置)
	lits [2]*ir.Value // 型のない定数のオペランドの元の値 (adaptLiteral の前。相手の型に切り詰める前の値)
}

// recordArith は算術の命令 op の結果 tmp を記録する (fc 4 と、書き換えを集めるときだけ)。lits は型のない定数のオペランドの元の値。
func (h *Hlc) recordArith(tmp *ir.Value, op *ir.Op, c *cexpr, lits ...ir.Operand) {
	if !(h.v4() || h.rewriting()) || tmp.Type.Kind != types.Int || tmp.Type.Enum != nil {
		return
	}
	n := &arithNode{op: op, c: c}
	for i, l := range lits {
		if v, ok := l.(*ir.Value); ok && i < 2 && v.Kind == ir.KindLiteral && v.IsInt && v.Untyped {
			n.lits[i] = v
		}
	}
	if h.arith == nil {
		h.arith = map[*ir.Value]*arithNode{}
	}
	h.arith[tmp] = n
	h.noteShift(tmp, op)
}

// widenArith は v が式の中の算術の結果で typ より狭ければ、fc 4 では部分木ごと typ の大きさで計算し直し、fc 3 では書き換えを
// 報告する。v はそのまま (fc 4 では型が広がった同じ一時変数) 返す。
func (h *Hlc) widenArith(v ir.Operand, typ *types.Type) ir.Operand {
	tv, ok := v.(*ir.Value)
	if !ok || typ.Kind != types.Int {
		return v
	}
	if tc := h.taint[tv]; tc != nil && tv.Type.Size < typ.Size {
		// 折り返した型付きの定数の演算 (下の「型付きの定数の演算と A1」)
		switch {
		case h.v4():
			if r := h.foldAtWidth(tc, typ.Size); r != nil {
				return ir.NewIntLiteral("", typ, wrapInt(r.val.Int, typ)) // 揃える先の型の値 (符号の読み替え: 下の widen の後と同じ)
			}
		case h.rewriting():
			h.rewriteAs("widen", tc, tv.Type.String())
		}
		return v
	}
	if h.arith == nil {
		return v
	}
	n := h.arith[tv]
	if n == nil || tv.Type.Size >= typ.Size {
		return v
	}
	switch {
	case h.v4():
		if h.widen(tv, typ.Size, 0) && tv.Type != typ {
			// 広げた計算の型 (演算の符号のまま) と揃える先の型 (大きいほう・同じ大きさなら符号付き) の符号が違えば、揃える先の型として
			// 読む (大きさは同じなので cast は何もしない。比較・右シフトの符号が揃える先の型で決まるように: fc 3 では符号拡張の結果が
			// その型で出る。`(x / 139) >= (0 as u16)` (x:i8) は符号なしの比較)
			return ir.NewCastedValue(tv, typ, 0)
		}
	case h.rewriting():
		h.rewriteAs("widen", n.c, tv.Type.String())
	}
	return v
}

// widen は算術の結果 tv を作った部分木を size バイトの計算にする。uses は tv を読んでよい回数 (根はまだ誰も読んでいないので 0、
// 部分木の中は親の命令の 1)。ほかでも使われている一時変数 (普通は無い) は広げない。
func (h *Hlc) widen(tv *ir.Value, size, uses int) bool {
	n := h.arith[tv]
	if n == nil || tv.Type.Size >= size || h.uses(tv, n.op) != uses {
		return false
	}
	op := n.op
	nt := h.prog.Types.IntType(size, tv.Type.Signed)
	for i, src := range op.Src {
		if i == 1 && (op.Code == ir.OpShiftLeft || op.Code == ir.OpShiftRight) {
			continue // シフト量は区切り
		}
		if sv, ok := src.(*ir.Value); ok && h.widen(sv, size, 1) {
			continue
		}
		if sv, ok := src.(*ir.Value); ok && h.taint[sv] != nil {
			if r := h.foldAtWidth(h.taint[sv], size); r != nil {
				op.Src[i] = r.val // 折り返した型付きの定数の演算は元の式を広い幅で畳み込み直す
				continue
			}
		}
		if i < 2 && n.lits[i] != nil {
			op.Src[i] = ir.NewIntLiteral("", nt, wrapInt(n.lits[i].Int, nt)) // 型のない定数は元の値から (`x + -1` は x + 65535)
			continue
		}
		st := ir.ValType(src)
		if st.Kind != types.Int && st.Kind != types.Bool || st.Size >= size || !st.Signed {
			continue // 符号なしの狭い値は 0 で広がる (codegen が上位を 0 で読む)
		}
		if k, ok := ir.ValIntLiteral(src); ok {
			op.Src[i] = ir.NewIntLiteral("", h.prog.Types.IntType(size, true), wrapInt(k, st))
			continue
		}
		ext := h.newTmp(h.prog.Types.IntType(size, true))
		h.insertBefore(op, &ir.Op{Code: ir.OpSignExtension, Dst: ext, Src: []ir.Operand{src}})
		op.Src[i] = ext
	}
	tv.Type = nt
	ir.InferWidthSign(op)
	return true
}

// uses は def (tv を書く命令) より後で tv を読む回数。
func (h *Hlc) uses(tv *ir.Value, def *ir.Op) int {
	ops := h.lmd.Ops
	i := len(ops) - 1
	for i >= 0 && ops[i] != def {
		i--
	}
	cnt := 0
	for _, op := range ops[i+1:] {
		if op == nil {
			continue
		}
		_, us := ir.DefUse(op)
		for _, u := range us {
			if ir.UnderlyingValue(u) == tv {
				cnt++
			}
		}
	}
	return cnt
}

// insertBefore は命令 op の前に nop を入れる (op は関数の命令列の最近のもの)。
func (h *Hlc) insertBefore(op, nop *ir.Op) {
	ir.InferWidthSign(nop)
	nop.Pos = op.Pos
	ops := h.lmd.Ops
	for i := len(ops) - 1; i >= 0; i-- {
		if ops[i] == op {
			ops = append(ops, nil)
			copy(ops[i+1:], ops[i:])
			ops[i] = nop
			h.lmd.Ops = ops
			return
		}
	}
	panic("insertBefore: op not found")
}

// 型付きの定数の演算と A1: sema は型付きの定数の演算を畳み込む (`(200 as u8) + (100 as u8)` は u8 の 44)。fc 4 で代入先や
// 16 ビットの値に広がる式なら、広い幅で計算した値 (300) でなければならない。畳み込みの時点では広がるか分からないので、
// 折り返しや型のない定数の切り詰めで値が変わった畳み込みの結果に元の式の印 (taint) を付けておき、広い型と出会ったら
// (定数の畳み込みの中の演算 foldWidenArgs、実行時の式の widenArith)、fc 4 では元の式をその幅で畳み込み直す (foldAtWidth)。
// 広がらなければ畳み込んだ定数のままで IR は fc 3 と同じ。fc 3 のモジュールでは `(元の式) as T` を migrate に報告する。
// `as` は外からの広がりを止めるだけで、中の式にも「式の中の一番広い型」の規則は効く (印は `as` の中でも付ける)。

// markTaint は折り返した畳み込みの結果 lit に元の式 c を記録する。
func (h *Hlc) markTaint(lit *ir.Value, c *cexpr) {
	if h.taint == nil {
		h.taint = map[*ir.Value]*cexpr{}
	}
	h.taint[lit] = c
}

// taintOf は印の付いた定数なら元の式を返す。
func (h *Hlc) taintOf(c *cexpr) *cexpr {
	if c == nil || c.kind != cValue || h.taint == nil {
		return nil
	}
	return h.taint[c.val]
}

// foldWidenArgs は、畳み込もうとしている演算 c の型 t より狭い、印の付いたオペランドを、fc 4 では元の式を t の幅で畳み込み
// 直した定数に置き換える (置き換えたら true)。fc 3 のモジュールでは migrate にそのオペランドの `as` を報告する。
func (h *Hlc) foldWidenArgs(c *cexpr, args []*cexpr, t *types.Type) bool {
	if !(h.v4() || h.rewriting()) || h.taint == nil {
		return false
	}
	changed := false
	for i, a := range args {
		if i == 1 && (c.op == opShiftLeft || c.op == opShiftRight) {
			continue
		}
		tc := h.taintOf(a)
		if tc == nil || a.val.Type.Size >= t.Size {
			continue
		}
		switch {
		case h.v4():
			if r := h.foldAtWidth(tc, t.Size); r != nil {
				args[i] = r
				changed = true
			}
		case h.rewriting():
			h.rewriteAs("widen", tc, a.val.Type.String())
		}
	}
	return changed
}

// foldOverflows は型付きの定数の演算 c の畳み込みで、結果に印を付けるか: 折り返して値が変わった (over)、オペランドを t に
// 読み替えて値が変わった (広い幅では元の値のまま計算する)、または同じ幅のオペランドに印がある。fc 4 か書き換えを集めているときだけ。
func (h *Hlc) foldOverflows(c *cexpr, args []*cexpr, t *types.Type, over bool) bool {
	if !(h.v4() || h.rewriting()) {
		return false
	}
	for i, a := range args {
		if i == 1 && (c.op == opShiftLeft || c.op == opShiftRight) {
			continue
		}
		if wrapInt(a.val.Int, t) != a.val.Int {
			over = true // オペランドを演算の型に読み替えて値が変わった (型のない定数の切り詰め、`(253 as u8) | (127 as i8)` の 253 → -3)
		}
		if h.taintOf(a) != nil && a.val.Type.Size == t.Size {
			over = true
		}
	}
	return over
}

// foldAtWidth は印の付いた定数の元の式 c を size バイトの幅で畳み込み直す。定数にならなければ nil。
func (h *Hlc) foldAtWidth(c *cexpr, size int) *cexpr {
	v, t, ok := h.wideValue(c, size)
	if !ok || t == nil {
		return nil
	}
	return cv(ir.NewIntLiteral("", t, v))
}

// wideValue は定数の式 c を、A1 で広げた実行時の計算 (widen) と同じ手順で size バイトの幅で計算する: 算術の演算は、元の幅で
// 畳み込んだときの型の符号のまま size バイトで計算し、算術でない式 (`as`・比較・組み込み・名前など。区切り) は元の幅の値、
// 型のない定数は元の値から。シフト量は元の値。型のない値 (オペランドが全部型のない定数) なら t は nil。
func (h *Hlc) wideValue(c *cexpr, size int) (v int, t *types.Type, ok bool) {
	narrow := h.constEval(c)
	if !narrow.isLiteralInt() {
		return 0, nil, false
	}
	if !isArithNode(c) || narrow.val.Untyped || narrow.val.Type.Kind != types.Int {
		if narrow.val.Untyped {
			return narrow.val.Int, nil, true
		}
		return narrow.val.Int, narrow.val.Type, true
	}
	tw := h.prog.Types.IntType(max(size, narrow.val.Type.Size), narrow.val.Type.Signed)
	vals := make([]int, len(c.args))
	for i, a := range c.args {
		if i == 1 && (c.op == opShiftLeft || c.op == opShiftRight) {
			x := h.constEval(a)
			if !x.isLiteralInt() {
				return 0, nil, false
			}
			vals[i] = x.val.Int
			continue
		}
		x, _, ok := h.wideValue(a, size)
		if !ok {
			return 0, nil, false
		}
		vals[i] = wrapInt(x, tw) // 型付きの値はその型の値を、型のない定数は元の値を広い型で読む
	}
	v1, v2 := vals[0], 0
	if len(vals) > 1 {
		v2 = vals[1]
	}
	return wrapInt(foldIntOp(c.op, v1, v2), tw), tw, true
}

// isArithNode は c が算術の演算 (+ - * / % & | ^ << >> 単項の - ~) か。
func isArithNode(c *cexpr) bool {
	if c.kind != cOp {
		return false
	}
	switch c.op {
	case opAdd, opSub, opMul, opDiv, opMod, opAnd, opOr, opXor, opShiftLeft, opShiftRight, opUminus, opBitNot:
		return true
	}
	return false
}
