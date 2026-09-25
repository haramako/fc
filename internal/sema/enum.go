package sema

// fc 3 の enum (doc/language_feature_candidates.md §1)。型は整数型 (Kind Int) に EnumInfo を付けた別の型で、コード生成は
// 基底型の整数と同じ。同じ enum 同士の比較 (== != < …) はできるが、算術は `as` で整数にしてから。整数・別の enum とは
// 互換でない (types.Compatible)。`.Name` は型が文脈 (代入先・比較の相手・case・引数・戻り値) から分かるときのメンバー。

import (
	"fmt"
	"strings"

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/syntax"
	"github.com/haramako/fc/internal/types"
)

// compileEnumDecl は enum の宣言 (型とメンバーの値を決め、型名を宣言する)。
func (h *Hlc) compileEnumDecl(s *syntax.EnumDecl) {
	h.mustInModule()
	name := s.Name.Name
	base := h.prog.Types.IntType(1, false)
	if s.Base != nil {
		base = h.typeEval(s.Base)
		if base.Kind != types.Int || base.Enum != nil {
			panic(&diag.Error{Msg: fmt.Sprintf("enum %s: the base type must be an integer type (u8 / i8 / u16 / i16), not %s", name, base)})
		}
	}
	t := h.prog.Types.NewEnum(h.module.Id+"."+name, base)
	lo, hi := 0, 1<<(8*base.Size)-1
	if base.Signed {
		lo, hi = -(1 << (8*base.Size - 1)), 1<<(8*base.Size-1)-1
	}
	next := 0
	var members []types.EnumMember
	for _, m := range s.Members {
		h.updatePos(m.Name)
		v := next
		if m.Value != nil {
			cv := h.constEval(toC(m.Value))
			if !cv.isLiteralInt() {
				panic(&diag.Error{Msg: fmt.Sprintf("enum %s: the value of %s must be a constant integer", name, m.Name.Name)})
			}
			v = cv.val.Int
		}
		if v < lo || v > hi {
			panic(&diag.Error{Msg: fmt.Sprintf("enum %s: %s = %d does not fit in %s", name, m.Name.Name, v, base)})
		}
		for _, o := range members {
			if o.Name == m.Name.Name {
				panic(&diag.Error{Msg: fmt.Sprintf("enum %s: member %s already defined", name, m.Name.Name)})
			}
		}
		members = append(members, types.EnumMember{Name: m.Name.Name, Value: v})
		next = v + 1
	}
	t.Enum.Members = members
	v := ir.NewTypeValue(name, h.prog.Types.TypeName(), t)
	v.Public = s.PublicPos.IsValid()
	h.scope.Declare(v)
}

// enumMember は enum 型 t のメンバー name の値 (無ければエラー)。
func (h *Hlc) enumMember(t *types.Type, name string) *ir.Value {
	m, ok := t.Enum.Member(name)
	if !ok {
		var names []string
		for _, x := range t.Enum.Members {
			names = append(names, x.Name)
		}
		panic(&diag.Error{Msg: fmt.Sprintf("%s has no member %s (members: %s)", t.Enum.Name, name, strings.Join(names, ", "))})
	}
	return ir.NewIntLiteral("", t, m.Value)
}

// enumShort は `.Name` を型 t (文脈の型) のメンバーにする。
func (h *Hlc) enumShort(c *cexpr, t *types.Type) *cexpr {
	if t == nil || t.Enum == nil {
		what := "no type from context"
		if t != nil {
			what = "the context type is " + t.String()
		}
		panic(&diag.Error{Msg: fmt.Sprintf(".%s needs an enum type from context (%s; write Type.%s)", c.name, what, c.name)})
	}
	return cv(h.enumMember(t, c.name))
}

// resolveEnumShortPair は二項の比較で片方が `.Name` なら、もう片方の型で解決する (もう片方を先に評価する)。
func (h *Hlc) resolveEnumShortPair(a, b *cexpr) (*cexpr, *cexpr) {
	if a.kind == cEnumShort && b.kind != cEnumShort {
		bb := h.constEval(b)
		return h.enumShort(a, h.cexprType(bb)), bb
	}
	if b.kind == cEnumShort && a.kind != cEnumShort {
		aa := h.constEval(a)
		return aa, h.enumShort(b, h.cexprType(aa))
	}
	return a, b
}

// cexprType は評価済みの式の型 (定数ならその型。実行時の式なら評価して型を得る)。
func (h *Hlc) cexprType(c *cexpr) *types.Type {
	if c.kind == cValue {
		return c.val.Type
	}
	return ir.ValType(h.rval(c))
}

// checkEnumOp は enum の演算の規則: 比較は同じ enum 同士だけ、それ以外 (算術・ビット・論理) は不可。
func checkEnumOp(op cop, a, b *types.Type) {
	ae, be := a != nil && a.Enum != nil, b != nil && b.Enum != nil
	if !ae && !be {
		return
	}
	switch op {
	case opEq, opNe, opLt, opGt, opLe, opGe:
		if a != b {
			panic(&diag.Error{Msg: fmt.Sprintf("cannot compare %s and %s (convert with `as`)", a, b)})
		}
	default:
		t := a
		if !ae {
			t = b
		}
		panic(&diag.Error{Msg: fmt.Sprintf("cannot apply %s to enum %s (convert with `as` first)", opSymbol(op), t)})
	}
}

// warnEnumSwitch は enum の switch で default が無く、書かれていないメンバーがあれば警告する。
func (h *Hlc) warnEnumSwitch(t *types.Type, seen map[int]bool, hasDefault bool) {
	if t.Enum == nil || hasDefault {
		return
	}
	var missing []string
	for _, m := range t.Enum.Members {
		if !seen[m.Value] {
			missing = append(missing, "."+m.Name)
		}
	}
	if len(missing) > 0 {
		h.warn("switch on %s does not handle %s (add the cases or a default)", t.Enum.Name, strings.Join(missing, ", "))
	}
}
