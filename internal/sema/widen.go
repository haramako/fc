package sema

// A1 (fc 4): 式全体を「代入先の型」と「式の中で一番広い型」の広いほうで計算する (Agent/wiki/plans/v4-plan.md §1.3 A)。
//
// 幅は式を IR にする前に上から決める (2026-09-30。以前は 8 ビットで出した命令を、16 ビットの値と出会った所で書き換えて広げて
// いた)。代入先・引数・戻り値・struct / 配列の要素は代入先の型の大きさ (rvalWide)、比較は両辺の広げる前の型の広いほう
// (compareWidth)、算術の節点は親から来た幅と自分の広げる前の型 (型を決める段 exprType。typing.go) の大きさの広いほう
// (wideWidth) を算術の子に渡す。幅が広げる前の型より広い節点は、最初からその幅の命令を出す: 符号は広げる前の型の符号 (同じ
// 大きさなら符号付きが勝つ: C0)、項は型のない定数なら元の値から、符号付きの狭い値は符号拡張 (符号なしは 0 で広がる)、
// 折り返した型付きの定数は元の式を広い幅で畳み込み直す (widenOperand)。部分木の区切りは算術の命令 (+ - * / % & | ^ << >>
// 単項の - ~) 以外のすべて (変数・呼び出し・読み出し・`as`・比較・シフト量・組み込み)。
//
// fc 3 のモジュールで書き換えを集めているとき (rewriting) は広げず、fc 4 で広がる部分木に `(式) as T` (T は今の型) を足す
// 書き換えを報告する (`as` は区切りなので、fc 4 でも中は今の幅で計算される。widenArith)。

import (
	"fmt"

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/types"
)

// arithNode は式の中の算術の命令の結果 (一時変数) の記録。
type arithNode struct {
	op    *ir.Op
	c     *cexpr       // 元の式 (書き換えの位置)
	lits  [2]*ir.Value // 型のない定数のオペランドの元の値 (adaptLiteral の前。相手の型に切り詰める前の値)
	mixed *mixedArith  // 符号の混ざった演算の結果 (mixedarith.go)。nil でなければ解釈する所でエラー
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
	n.mixed = h.mixedOf(tmp, op, n, lits)
	h.arith[tmp] = n
	h.noteShift(tmp, op)
}

// widenArith は v が式の中の算術の結果で typ より狭いとき: 折り返した型付きの定数なら fc 4 では typ の幅で畳み込み直し、
// fc 3 では書き換えを報告する (算術の命令は fc 4 では上から幅を決めて出すので、ここに狭いまま来ない)。
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
		// fc 4 の算術は、評価する前に上から幅を決めて最初から広い命令を出す (wideWidth)。ここに狭いまま来るのは幅を決め
		// そこねた形 (コンパイラの誤り。以前は出した命令をここで書き換えて広げていた)
		panic(&diag.Error{Msg: fmt.Sprintf("internal: A1: %s was computed in %s but is used as %s (the width was not decided before evaluating it: sema/widen.go)", h.exprText(n.c), tv.Type, typ)})
	case h.rewriting():
		h.rewriteAs("widen", n.c, tv.Type.String())
	}
	return v
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

// 上から決める A1 (fc 4): 式を IR にする前に、算術の部分木を計算する幅を決めて (代入先・比較の相手・親の算術の幅と、
// 型を決める段 exprType が返す広げる前の型の大きさの広いほう)、最初からその幅の命令を出す。符号は広げる前の型の符号
// (同じ大きさなら符号付きが勝つ: C0)。項は widen と同じく、型のない定数は元の値から、符号付きの狭い値は符号拡張、
// 折り返した型付きの定数は広い幅で畳み込み直す。広げる前の型を決められない式は、今までどおり出会った所で広げる (widen)。

// wideWidth は算術の節点 e を計算する幅と、広げる前の型 (hint は親・代入先から来た幅)。決められなければ 0。
func (h *Hlc) wideWidth(e *cexpr, hint int) (int, *types.Type) {
	if !h.v4() {
		return 0, nil
	}
	info, ok := h.exprType(e)
	if !ok || info.untyped || info.t.Kind != types.Int || info.t.Enum != nil || info.t.Size > 2 {
		return 0, nil
	}
	return max(hint, info.t.Size), info.t
}

// compareWidth は比較の両辺を計算する幅 (両辺の広げる前の型の広いほう。型のない定数は比較の規則で相手に合わせた型:
// `(x << 5) == 65533` (x:i8) の 65533 は u16 なので、左辺も 16 ビットで計算する)。
func (h *Hlc) compareWidth(a, b *cexpr) int {
	if !h.v4() {
		return 0
	}
	ai, aok := h.exprType(a)
	bi, bok := h.exprType(b)
	if !aok || !bok {
		return 0
	}
	w := 0
	for _, t := range []*types.Type{h.literalAdaptedCmp(ai, bi), h.literalAdaptedCmp(bi, ai)} {
		if t.Kind == types.Int && t.Enum == nil && t.Size <= 2 {
			w = max(w, t.Size)
		}
	}
	return w
}

// rvalWide は代入先の型 typ の幅で式 c を計算する rval (A1)。
func (h *Hlc) rvalWide(c *cexpr, typ *types.Type) ir.Operand {
	if h.v4() && typ != nil && typ.Kind == types.Int && typ.Enum == nil && typ.Size <= 2 {
		h.wide = typ.Size
	}
	return h.rval(c)
}

// widenOperand は算術の項 src を t の幅の計算に合わせる (widen の項の扱いと同じ)。
func (h *Hlc) widenOperand(src ir.Operand, t *types.Type) ir.Operand {
	if sv, ok := src.(*ir.Value); ok {
		if tc := h.taint[sv]; tc != nil && sv.Type.Size < t.Size {
			if r := h.foldAtWidth(tc, t.Size); r != nil {
				return r.val // 折り返した型付きの定数の演算は元の式を広い幅で畳み込み直す
			}
		}
		if sv.Kind == ir.KindLiteral && sv.IsInt && sv.Untyped {
			return ir.NewIntLiteral("", t, wrapInt(sv.Int, t)) // 型のない定数は元の値から (`x + -1` は x + 65535)
		}
	}
	st := ir.ValType(src)
	if st.Kind != types.Int && st.Kind != types.Bool || st.Size >= t.Size || !st.Signed {
		return src // 符号なしの狭い値は 0 で広がる (codegen が上位を 0 で読む)
	}
	if k, ok := ir.ValIntLiteral(src); ok {
		return ir.NewIntLiteral("", h.prog.Types.IntType(t.Size, true), wrapInt(k, st))
	}
	ext := h.newTmp(h.prog.Types.IntType(t.Size, true))
	h.emit(&ir.Op{Code: ir.OpSignExtension, Dst: ext, Src: []ir.Operand{src}})
	return ext
}

// widened は式 c の値 v を、A1 で広げる前の型より広い幅で計算したか。
func (h *Hlc) widened(c *cexpr, v ir.Operand) bool {
	info, ok := h.exprType(c)
	vt := ir.ValType(v)
	return ok && !info.untyped && info.t.Kind == types.Int && vt.Kind == types.Int && vt.Size > info.t.Size
}

// narrowView は A1 で広い幅で計算した式 c の値 v を、広げる前の型として見たもの (検査用。広げていなければ v)。
func (h *Hlc) narrowView(c *cexpr, v ir.Operand) ir.Operand {
	if !h.widened(c, v) {
		return v
	}
	info, _ := h.exprType(c)
	return ir.NewCastedValue(v, info.t, 0)
}

// preArrayBase は型を書かない実行時の配列リテラルの要素の型を、要素を評価する前に決める (runtimeArray と同じ規則: 要素の型の
// 互換型を、定数の要素が入る型に (F4))。決められなければ nil。
func (h *Hlc) preArrayBase(args []*cexpr) *types.Type {
	if !h.v4() || len(args) == 0 {
		return nil
	}
	var base *types.Type
	vals := make([]ir.Operand, len(args))
	for i, a := range args {
		info, ok := h.exprType(a)
		if !ok {
			return nil
		}
		if info.untyped {
			v := ir.NewIntLiteral("", info.t, info.n)
			v.Untyped = true
			vals[i] = v
		} else {
			vals[i] = ir.NewLocal("", info.t, ir.LTTemp) // 型だけの値 (IR には出さない)
		}
		if i == 0 {
			base = info.t
		} else if base = h.prog.Types.Compatible(base, info.t); base == nil {
			return nil
		}
	}
	return h.arrayElemType(base, vals, args, false)
}
