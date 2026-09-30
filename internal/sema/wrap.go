package sema

// fc 4 の左の項の型で折り返す演算 `a +% b` / `a -% b` / `a *% b` (2026-09-30)。結果は左の項の型 T で、T の幅で折り返す。
// u8 の座標に i8 の移動量を足して、そのまま比べる・割る・シフトする形 (`(y +% dy) / 16`) に使う (`+` は同じ大きさなら符号付きが
// 勝つので i8 になる)。`(a op (b as T)) as T` に直して評価する: 右の項は T の幅へ自分の符号で広げ (同じ大きさならビットのまま)、
// 外側の `as` は A1 の区切り (外の広い代入先で広げない)。右の項が T より広い、型のない定数が T の大きさに入らない、左の項に型が
// 無い (型のない定数) のはエラー。

import (
	"fmt"

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/syntax"
	"github.com/haramako/fc/internal/types"
)

var wrapOps = map[cop]struct {
	op   cop
	text string
}{
	opAddWrap: {opAdd, "+%"}, opSubWrap: {opSub, "-%"}, opMulWrap: {opMul, "*%"},
}

// wrapExpr は `a op% b` (c) を `(a op (b as T)) as T` の式にする。a / b は評価済み (cValue か cOperand)。
func (h *Hlc) wrapExpr(c *cexpr, a, b *cexpr) *cexpr {
	w := wrapOps[c.op]
	if !h.v4() {
		panic(&diag.Error{Msg: fmt.Sprintf("`%s` is fc 4 (write `(a %s (b as T)) as T` in fc 3 and older modules)", w.text, w.text[:1])})
	}
	lt, lUntyped := wrapOperandType(a)
	rt, rUntyped := wrapOperandType(b)
	switch {
	case lUntyped:
		panic(&diag.Error{Msg: fmt.Sprintf("`%s`: the left operand decides the type, so it cannot be an untyped constant (write `(%d as T) %s b`)", w.text, a.val.Int, w.text)})
	case lt.Kind != types.Int || lt.Enum != nil:
		panic(&diag.Error{Msg: fmt.Sprintf("`%s`: the left operand must be an integer (got %s)", w.text, lt)})
	case rt.Kind != types.Int || rt.Enum != nil:
		panic(&diag.Error{Msg: fmt.Sprintf("`%s`: the right operand must be an integer (got %s)", w.text, rt)})
	case rUntyped:
		// 型のない定数は T の大きさに入れば符号を問わない (`x +% -1`)
		if n, bits := b.val.Int, 8*lt.Size; n < -(1<<(bits-1)) || n >= 1<<bits {
			panic(&diag.Error{Msg: fmt.Sprintf("`%s`: %d does not fit in %s", w.text, n, lt)})
		}
	case rt.Size > lt.Size:
		panic(&diag.Error{Msg: fmt.Sprintf("`%s`: the right operand (%s) is wider than the left (%s); the result has the type of the left operand", w.text, rt, lt)})
	}
	asT := func(x *cexpr) *cexpr {
		return &cexpr{kind: cCast, args: []*cexpr{x}, ty: lt, ck: syntax.CastAs, pos: x.pos, end: x.end}
	}
	inner := cop2(w.op, a, asT(b))
	inner.pos, inner.end = c.pos, c.end
	r := asT(inner)
	r.pos, r.end = c.pos, c.end
	return r
}

// wrapOperandType は評価済みの項 x の型と、型のない整数定数か。
func wrapOperandType(x *cexpr) (*types.Type, bool) {
	if x.kind == cValue {
		return x.val.Type, x.val.Kind == ir.KindLiteral && x.val.IsInt && x.val.Untyped
	}
	return ir.ValType(x.opnd), false
}
