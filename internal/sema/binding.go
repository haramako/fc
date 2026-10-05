package sema

// 名前の表 (Scope) の、値でない束縛: モジュール (`use mod;`) と型名 (`struct S`・enum・soa)。名前の表には ir.Value の入れ物で
// 置くが、何を束縛したか (モジュールの id・実際の型) は sema が持つ。IR の変数の一覧 (ir.Module.Vars) にも入れない
// (2026-10-05。以前は ir.Value.Module / TypeRef に持ち、IR の変数として出ていた)。ir.Value は置き場所とリテラル (と、
// マクロの入れ物) の表現にする方向 (Agent/wiki/plans/roadmap.md の構造の整理「sema の式に型付きの中間表現を」)。

import (
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/types"
)

// binding は値でない束縛の中身。
type binding struct {
	module  string      // モジュールの束縛ならモジュールの id
	typeRef *types.Type // 型名の束縛なら実際の型 (束縛の値の型は TypeName か、soa は soa の型)
}

// newModuleBinding はモジュールの束縛 (`use mod;` の名前 name → モジュール id)。
func (p *Program) newModuleBinding(name, id string) *ir.Value {
	v := ir.NewGlobal(name, p.Types.Module(), "")
	p.bindings[v] = binding{module: id}
	return v
}

// newTypeBinding は型名の束縛 (typ は束縛の値の型: TypeName、soa は soa の型。ref が実際の型)。
func (p *Program) newTypeBinding(name string, typ, ref *types.Type) *ir.Value {
	v := ir.NewGlobal(name, typ, "")
	p.bindings[v] = binding{typeRef: ref}
	return v
}

// moduleID は v がモジュールの束縛ならそのモジュールの id (ほかは "")。
func (p *Program) moduleID(v *ir.Value) string { return p.bindings[v].module }

// typeRef は v が型名の束縛なら実際の型 (ほかは nil)。
func (p *Program) typeRef(v *ir.Value) *types.Type { return p.bindings[v].typeRef }
