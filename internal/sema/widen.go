package sema

// A1 (fc 4): 式全体を「代入先の型」と「式の中で一番広い型」の広いほうで計算する (doc/v4_plan.md §1.3 A)。
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
	c    *cexpr        // 元の式 (書き換えの位置)
	lits [2]*ir.Value  // 型のない定数のオペランドの元の値 (adaptLiteral の前。相手の型に切り詰める前の値)
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
}

// widenArith は v が式の中の算術の結果で typ より狭ければ、fc 4 では部分木ごと typ の大きさで計算し直し、fc 3 では書き換えを
// 報告する。v はそのまま (fc 4 では型が広がった同じ一時変数) 返す。
func (h *Hlc) widenArith(v ir.Operand, typ *types.Type) ir.Operand {
	tv, ok := v.(*ir.Value)
	if !ok || typ.Kind != types.Int || h.arith == nil {
		return v
	}
	n := h.arith[tv]
	if n == nil || tv.Type.Size >= typ.Size {
		return v
	}
	switch {
	case h.v4():
		h.widen(tv, typ.Size, 0)
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
