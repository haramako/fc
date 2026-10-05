package sema

// 型式 (syntax.TypeExpr) から types.Type へ。

import (
	"fmt"

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/syntax"
	"github.com/haramako/fc/internal/types"
)

// typeOf resolves type identities and array lengths at every nesting depth.
// A named struct does not require its layout until it is used by value.
func (h *Hlc) typeOf(t syntax.TypeExpr) *types.Type {
	ty := h.typeOfRaw(t)
	if ty.Kind == types.Soa {
		// soa は型と実体が 1 対 1 で、値の型にはならない (`var v:E;` は E と同じ形の 2 つ目の実体ではなく、確保した領域を
		// 使わずに E を読み書きしていた)。型として書けるのはハンドル `*E` だけ
		n := shortName(ty.Name)
		panic(&diag.Error{Msg: fmt.Sprintf("soa %s is not a value type; use *%s for an element handle, or %s[i]", n, n, n)})
	}
	return ty
}

// typeOfRaw は typeOf の本体。soa の型もそのまま返す (`*E` の要素のため)。
func (h *Hlc) typeOfRaw(t syntax.TypeExpr) *types.Type {
	switch t := t.(type) {
	case *syntax.NamedType:
		return h.namedType(t)
	case *syntax.PointerType:
		elem := h.typeOfRaw(t.Elem)
		if elem.Kind == types.Soa {
			return h.prog.Types.SoaRef(elem, h.soaElement(elem), "") // `*Points`: SoA の要素ハンドル
		}
		return h.prog.Types.PointerToRO(elem, t.Const.IsValid())
	case *syntax.ArrayType:
		if t.IsSlice(h.version()) {
			wide := false
			if t.LenType != nil {
				lt := h.typeOf(t.LenType)
				if lt.Kind != types.Int || lt.Enum != nil || lt.Signed {
					panic(&diag.Error{Msg: fmt.Sprintf("the length type of a slice must be u8 or u16 (got %s)", lt)})
				}
				wide = lt.Size == 2
			}
			return h.prog.Types.Slice(h.typeOf(t.Elem), t.Const.IsValid(), wide)
		}
		if t.Const.IsValid() {
			panic(&diag.Error{Msg: "const is only for slices ([]const T); an array's elements are read-only when the array is const"})
		}
		n := -1
		if t.Len != nil {
			sv := h.constEval(toC(t.Len))
			if sv.kind != cValue {
				panic(&diag.Error{Msg: "array size must be constant"})
			}
			if sv.val.Kind == ir.KindLiteral && sv.val.IsInt {
				n = sv.val.Int
				if n < 0 {
					// 負の長さは「長さ未定」(-1) と区別できず、ca65 の Range error や slice 扱いになっていた (survey 2026-09-27)
					panic(&diag.Error{Msg: fmt.Sprintf("array length must not be negative (got %d)", n)})
				}
			} else {
				panic(&diag.Error{Msg: fmt.Sprintf("array length must be an integer constant (got %s)", ir.ValType(sv.val))})
			}
		}
		elem := h.typeOf(t.Elem)
		if n >= 0 && elem.Size > 0 && n*elem.Size > 65535 {
			panic(&diag.Error{Msg: fmt.Sprintf("array [%d]%s is too large (%d bytes; the 6502 address space is 64 KB)", n, elem, n*elem.Size)})
		}
		return h.prog.Types.ArrayOf(elem, n)
	case *syntax.FuncType:
		params := make([]*types.Type, len(t.Params))
		for i, p := range t.Params {
			if p.Name != nil {
				panic(&diag.Error{Msg: "named parameter is not allowed in function type"})
			}
			params[i] = h.typeOf(p.Type)
		}
		if t.Far {
			return h.prog.Types.FarFunc(params, h.typeOf(t.Result))
		}
		return h.prog.Types.Func(params, h.typeOf(t.Result), false)
	}
	panic(fmt.Sprintf("typeOf: unknown type expression %T", t))
}

// version はコンパイル中のモジュールの文法バージョン (モジュールの外 (組み込みの登録など) では fc 2)。
func (h *Hlc) version() int {
	if h.module == nil || h.module.Version == 0 {
		return syntax.Version2
	}
	return h.module.Version
}

// namedType は型名 (基本型、または struct / soa 宣言の名前。`mod.Name` は他モジュールの公開型) を型にする。
func (h *Hlc) namedType(t *syntax.NamedType) *types.Type {
	name := t.Name.Name
	if t.Module == nil {
		if ty, ok := h.prog.Types.NamedIn(name, h.version()); ok {
			return ty
		}
		if n, old := types.V2IntTypeNames[name]; old && h.version() >= syntax.Version3 {
			panic(&diag.Error{Msg: fmt.Sprintf("%s is not a type in fc 3 (write %s; `fcc migrate` rewrites fc 2 sources)", name, n)})
		}
		if sym := h.scope.Find(name, true); sym != nil {
			if sym.Type != nil {
				return sym.Type
			}
			if sym.Val != nil && sym.Val.Type.Kind == types.Bad {
				return sym.Val.Type // エラーになった struct / soa 宣言
			}
		}
		panic(&diag.Error{Msg: fmt.Sprintf("unknown type %s", name)})
	}
	mv := h.scope.FindMust(t.Module.Name, true)
	mi := h.prog.boundModule(mv)
	if mi == nil {
		panic(&diag.Error{Msg: fmt.Sprintf("%s is not a module (in type %s.%s)", t.Module.Name, t.Module.Name, name)})
	}
	sym := mi.LookupMust(name)
	if sym.Type == nil {
		panic(&diag.Error{Msg: fmt.Sprintf("%s.%s is not a type", t.Module.Name, name)})
	}
	return sym.Type
}

// checkComplete は変数・フィールド・配列要素に置ける型か検査する (void と未完成の struct は不可)。
func (h *Hlc) checkComplete(t *types.Type, what string) {
	h.completeType(t)
	switch {
	case t.Kind == types.Void:
		panic(&diag.Error{Msg: fmt.Sprintf("%s cannot be void", what)})
	case t.Kind == types.Struct && t.Size < 0:
		panic(&diag.Error{Msg: fmt.Sprintf("%s: struct %s is not complete yet (recursive struct must go through a pointer)", what, t.Name)})
	case t.Kind == types.Array && t.Base.Kind == types.Struct && t.Base.Size < 0:
		panic(&diag.Error{Msg: fmt.Sprintf("%s: struct %s is not complete yet (recursive struct must go through a pointer)", what, t.Base.Name)})
	}
}

// typeEval resolves a type and requests the layout needed to use it by value.
// nil (型省略) なら nil。
func (h *Hlc) typeEval(t syntax.TypeExpr) *types.Type {
	if t == nil {
		return nil
	}
	ty := h.typeOf(t)
	h.completeType(ty)
	return ty
}

// ---------------------------------------------------------------
// 式のコンパイル
// ---------------------------------------------------------------
