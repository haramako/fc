package sema

import (
	"fmt"

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/syntax"
	"github.com/haramako/fc/internal/types"
)

// warnReturnLocalAddr は、return の式そのものがローカル変数 (引数を除く) の中を指すアドレスなら警告する
// (Agent/discussions/2026-09-20-v3-plan.md §10、2026-09-27 決定)。fc のローカル変数は静的フレームで、呼び出し経路の重ならない関数と番地を共有するので、
// 返した後の呼び出しで黙って上書きされる (C のスタックと同じく値は保証されない)。
// 見るのは直接の形だけ: `&x`、`&s.f`、`&a[i]`、配列の名前 (ポインタ / slice の戻り値への暗黙の変換)、`a[lo..hi]`。
// `var p = &x; return p;` のような変数の経由は追わない。
func (h *Hlc) warnReturnLocalAddr(e syntax.Expr, rt *types.Type) {
	decay := rt.Kind == types.Pointer || rt.IsSlice()
	name, ok := h.localAddr(e, decay)
	if !ok {
		return
	}
	h.prog.Warnings = append(h.prog.Warnings, diag.Warning{
		Msg: fmt.Sprintf("returning the address of local variable `%s`: locals live in a static frame shared with other functions, so it is overwritten by later calls (use a global or let the caller pass the buffer)", name),
		Pos: syntax.At(h.module.Path, e.Pos()),
	})
}

// localAddr は e の値がローカル変数の中を指すアドレスか (decay は配列の名前がポインタ / slice になる文脈か)。
func (h *Hlc) localAddr(e syntax.Expr, decay bool) (string, bool) {
	e = unparenExpr(e)
	switch x := e.(type) {
	case *syntax.UnaryExpr:
		if x.Op == syntax.Amp {
			name, t := h.localPath(x.X)
			return name, t != nil
		}
		return "", false
	case *syntax.SliceExpr:
		name, t := h.localPath(x.X)
		return name, t != nil && t.Kind == types.Array
	case *syntax.CastExpr:
		return h.localAddr(x.X, true) // `a as *u8`、`(&x) as *u8`
	case *syntax.CondExpr:
		if name, ok := h.localAddr(x.X, decay); ok {
			return name, true // `c ? &x : p`
		}
		return h.localAddr(x.Y, decay)
	case *syntax.BinaryExpr:
		if x.Op == syntax.Plus || x.Op == syntax.Minus {
			return h.localAddr(x.X, decay) // `&x + 1`、`a + 2`
		} // `s.buf` (Dot) は下の配列の名前として見る
	case *syntax.Ident:
		// for-each の要素のポインタ (`&base[i]` の別名): ローカルの配列を回しているなら同じ
		if v := h.scope.Find(x.Name, true); v != nil {
			if ev := h.exprAliases[v]; ev != nil {
				return aliasLocal(ev)
			}
		}
	}
	if decay {
		name, t := h.localPath(e)
		return name, t != nil && t.Kind == types.Array
	}
	return "", false
}

// localPath は e がローカル変数 (引数を除く) の中の場所 (変数・配列の要素・struct のフィールド) なら、変数名とその場所の型を返す。
// ポインタ・slice を経由する場所 (`p[i]`、`p.f`) はローカル変数の中ではないので nil。
func (h *Hlc) localPath(e syntax.Expr) (string, *types.Type) {
	switch x := unparenExpr(e).(type) {
	case *syntax.Ident:
		v := h.scope.Find(x.Name, true)
		if v == nil || !localStorage(v) {
			return "", nil
		}
		return x.Name, v.Type
	case *syntax.IndexExpr:
		name, t := h.localPath(x.X)
		if t == nil || t.Kind != types.Array {
			return "", nil
		}
		return name, t.Base
	case *syntax.BinaryExpr:
		if x.Op != syntax.Dot {
			return "", nil
		}
		name, t := h.localPath(x.X)
		id, ok := x.Y.(*syntax.Ident)
		if t == nil || t.Kind != types.Struct || !ok {
			return "", nil
		}
		f, ok := t.Field(id.Name)
		if !ok {
			return "", nil
		}
		return name, f.Type
	}
	return "", nil
}

// unparenExpr は括弧を外した式。
func unparenExpr(e syntax.Expr) syntax.Expr {
	for {
		p, ok := e.(*syntax.ParenExpr)
		if !ok {
			return e
		}
		e = p.X
	}
}

// localStorage は v が呼ばれた側の静的フレームにある値か: ユーザーのローカル変数と、値渡しの struct・配列の引数
// (呼ぶ側の値の写し。`function g(s:S):*u8 { return &s.b; }` は次の呼び出しで上書きされる)。
func localStorage(v *ir.Value) bool {
	if v.Kind != ir.KindLocal {
		return false
	}
	switch v.LocalType {
	case ir.LTNone:
		return true
	case ir.LTArg:
		return v.Type.Kind == types.Struct || v.Type.Kind == types.Array
	}
	return false
}

// aliasLocal は評価済みの式 (for-each の要素のポインタの別名 `&base[i]`) の元がローカル変数なら、その名前を返す。
func aliasLocal(c *cexpr) (string, bool) {
	for c != nil {
		switch {
		case c.kind == cValue:
			if localStorage(c.val) {
				return c.val.Name, true
			}
			return "", false
		case c.kind == cOp && (c.op == opRef || c.op == opIndex || c.op == opField):
			c = c.args[0]
		default:
			return "", false
		}
	}
	return "", false
}
