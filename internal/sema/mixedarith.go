package sema

// 符号の混ざった演算の結果を解釈する所はエラー (fc 4。2026-09-30)。
//
// 同じ大きさで符号の違う整数の `+ - *` (`y + dy`、y:u8、dy:i8) は、同じ大きさなら符号付きが勝つ (C0) ので i8 になる。結果の
// ビットはどちらの符号で計算しても同じなので、同じ大きさの代入先に入れるだけ (`x += dx`) や `& | ^` は問題ないが、結果を符号
// つきの意味で読む所では、座標のつもりの 128 以上が負になる: 大小の比較、`/`・`%`、`>>`、`as` で 16 ビットに広げる所、型を
// 省いた変数、@printf / @format の引数。これらに届いたらエラーにして、`a +% b` (左の項の型で計算する: wrap.go) か `as` で型を
// 選んでもらう。印はその結果に `+ - * & | ^ <<` と単項の `- ~` をかけた同じ大きさの結果にも引き継ぎ、`as` で消える。型の
// ない定数は相手の型になるので数えない。A1 が部分木ごと 16 ビットに広げる所 (代入先・相手が 16 ビット) は、葉をそれぞれの符号で
// 広げて計算し直すので値が正しく、数えない (文の終わりに見る)。
//
// fc 3 → 4 の migrate (規則 mixed-sign-arith): 起点の演算の符号付きの項が左にあれば演算子を `+%` などに (結果は左の型で
// 同じ)、右にあれば `(a + b) as i8` にする。どちらも fc 3 と同じ IR。

import (
	"fmt"
	"strings"

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/syntax"
	"github.com/haramako/fc/internal/types"
)

// mixedArith は符号の混ざった演算の結果の印。origin は起点の演算 (`y + dy`)、lt / rt はその両辺の型。
type mixedArith struct {
	origin *arithNode
	lt, rt *types.Type
}

// mixedOf は算術の命令 op の結果 tmp の印 (無ければ nil)。lits は recordArith に渡された元のオペランド (型のない定数を見分ける)。
func (h *Hlc) mixedOf(tmp *ir.Value, op *ir.Op, n *arithNode, lits []ir.Operand) *mixedArith {
	switch op.Code {
	case ir.OpAdd, ir.OpSub, ir.OpMul:
		if len(lits) == 2 && n.lits[0] == nil && n.lits[1] == nil {
			lt, rt := ir.ValType(lits[0]), ir.ValType(lits[1])
			if isPlainInt(lt) && isPlainInt(rt) && lt.Size == rt.Size && lt.Signed != rt.Signed && tmp.Type.Size == lt.Size {
				return &mixedArith{origin: n, lt: lt, rt: rt}
			}
		}
	case ir.OpAnd, ir.OpOr, ir.OpXor, ir.OpShiftLeft, ir.OpUminus, ir.OpBitNot:
	default:
		return nil
	}
	srcs := op.Src
	if op.Code == ir.OpShiftLeft {
		srcs = srcs[:1] // シフト量は値に入らない
	}
	for _, s := range srcs {
		if sv, ok := s.(*ir.Value); ok {
			if sn := h.arith[sv]; sn != nil && sn.mixed != nil && sv.Type.Size == tmp.Type.Size {
				return sn.mixed
			}
		}
	}
	return nil
}

func isPlainInt(t *types.Type) bool { return t != nil && t.Kind == types.Int && t.Enum == nil }

// mixedUse は符号の混ざった演算の結果 v を解釈する所 (what はエラーの文言)。
type mixedUse struct {
	v    *ir.Value
	what string
}

// checkMixedUse は v が符号の混ざった演算の結果なら、解釈する所として文の終わりの検査 (checkMixedUses) に控える。文の中で A1 が
// 部分木ごと広げたもの (`var t:u16 = (y + dy) / 16`: 葉を自分の符号で広げて 16 ビットで計算し直すので値は正しい) は、文の終わりに
// 型が広がっているので数えない。
func (h *Hlc) checkMixedUse(v ir.Operand, what string) {
	tv, ok := v.(*ir.Value)
	if !ok || h.arith == nil {
		return
	}
	if n := h.arith[tv]; n != nil && n.mixed != nil {
		h.mixedUses = append(h.mixedUses, mixedUse{v: tv, what: what})
	}
}

// checkMixedUses は文の中で控えた解釈する所 (mark 以降) のうち、広がらずに使われたものを、fc 4 ではエラー、fc 3 → 4 の書き換えでは
// 起点の演算の書き換えにする。
func (h *Hlc) checkMixedUses(mark int) {
	if len(h.mixedUses) <= mark {
		return
	}
	list := append([]mixedUse(nil), h.mixedUses[mark:]...)
	h.mixedUses = h.mixedUses[:mark]
	for _, u := range list {
		m := h.arith[u.v].mixed
		if u.v.Type.Size > m.lt.Size {
			continue // A1 が部分木ごと広げた
		}
		switch {
		case h.v4():
			panic(h.mixedArithError(m, u.what))
		case h.rewriting():
			h.rewriteMixedArith(m)
		}
	}
}

// mixedArithError は解釈する所 what に届いた符号の混ざった演算 m のエラー。
func (h *Hlc) mixedArithError(m *mixedArith, what string) *diag.Error {
	text := h.exprText(m.origin.c)
	paren := strings.Trim(text, "`")
	if !strings.HasPrefix(paren, "(") || !strings.HasSuffix(paren, ")") {
		paren = "(" + paren + ")"
	}
	limit := "128"
	if m.lt.Size == 2 {
		limit = "32768"
	}
	msg := fmt.Sprintf("%s mixes %s and %s, so it is %s (the signed type wins at the same size); %s reads a value of %s or more as negative. Choose the type: `a +%% b` / `a -%% b` / `a *%% b` compute in the type of the left operand, or write `%s as T`",
		text, m.lt, m.rt, m.origin.op.Dst.(*ir.Value).Type, what, limit, paren)
	return &diag.Error{Msg: msg, Pos: syntax.At(h.module.Path, m.origin.c.pos)}
}

// exprText は式 c のソースの綴り (`y + dy`)。分からなければ「the expression」。
func (h *Hlc) exprText(c *cexpr) string {
	if src := h.prog.Sources[h.module.Id]; src != nil && c != nil && c.pos.IsValid() && c.end.IsValid() &&
		c.pos.Offset >= 0 && c.end.Offset <= len(src.Src) && c.pos.Offset < c.end.Offset {
		return "`" + string(src.Src[c.pos.Offset:c.end.Offset]) + "`"
	}
	return "the expression"
}

// rewriteMixedArith は fc 3 の符号の混ざった演算 m.origin を fc 4 で同じ意味に書き換える: 符号付きの項が左 (結果の型と同じ) なら
// 演算子を `+%` などに、右なら `(a op b) as T`。
func (h *Hlc) rewriteMixedArith(m *mixedArith) {
	const rule = "mixed-sign-arith"
	c := m.origin.c
	t := m.origin.op.Dst.(*ir.Value).Type
	if m.lt == t && c.compound == nil && len(c.args) == 2 {
		if src := h.prog.Sources[h.module.Id]; src != nil {
			a, b := c.args[0], c.args[1]
			if a.end.IsValid() && b.pos.IsValid() && a.end.Offset <= b.pos.Offset && b.pos.Offset <= len(src.Src) {
				gap := string(src.Src[a.end.Offset:b.pos.Offset])
				sym := map[cop]string{opAdd: "+", opSub: "-", opMul: "*"}[c.op]
				if i := strings.Index(gap, sym); sym != "" && i >= 0 && strings.TrimSpace(gap) == sym {
					at := a.end.Offset + i
					h.addRewrite(rule, at, at+1, sym+"%")
					return
				}
			}
		}
	}
	h.rewriteAs(rule, c, t.String())
}
