package sema

// 名前の表 (Scope) の項目 Symbol。名前が束縛するのは値 (変数・定数・関数: ir.Value) か、値でないもの (モジュール・型名・
// マクロ)。値でないものは ir.Value にしない (2026-10-05。以前は型 module / typename / macro の ir.Value を置き、IR の変数の
// 一覧にも出ていた)。式の中では値でない名前は cName の節点になり (cexpr.go)、値として使うとエラー。ir.Value は置き場所と
// リテラルの表現 (Agent/wiki/plans/roadmap.md の構造の整理「sema の式に型付きの中間表現を」)。

import (
	"fmt"

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/types"
)

// Symbol は名前の表の 1 項目。Val・Module・Type・Macro のどれか (soa は Val と Type の両方: コンテナの値と型名を兼ねる)。
type Symbol struct {
	Name   string
	Val    *ir.Value   // 値
	Module string      // モジュールの束縛 (`use mod;`): モジュールの id
	Type   *types.Type // 型名の束縛 (struct・enum・soa): 実際の型 (エラーになった宣言は Bad)
	Macro  *macroDef   // マクロ (組み込み・外部のマクロ、textmap の変換器)
	public bool        // 値でない束縛の public (値は Val.Public)
}

// Public は外 (`mod.name`・`use * from mod`) から見えるか。
func (s *Symbol) Public() bool {
	if s.Val != nil {
		return s.Val.Public
	}
	return s.public
}

// SetPublic は public にする / しない。
func (s *Symbol) SetPublic(b bool) {
	if s.Val != nil {
		s.Val.Public = b
	}
	s.public = b
}

// valueSym は値の Symbol。
func valueSym(v *ir.Value) *Symbol { return &Symbol{Name: v.Name, Val: v} }

// notValue は値でない名前 s を値として使ったときのエラー。
func (s *Symbol) notValue() *diag.Error {
	switch {
	case s.Module != "":
		return &diag.Error{Msg: fmt.Sprintf("%s is a module, not a value", s.Name)}
	case s.Macro != nil:
		return &diag.Error{Msg: fmt.Sprintf("%s is a macro, not a value (call it: %s(...))", s.Name, s.Name)}
	case s.Type != nil && s.Type.Kind == types.Bad:
		return &diag.Error{Suppressed: true} // エラーになった宣言の参照: 報告済み
	}
	return &diag.Error{Msg: fmt.Sprintf("%s is a type, not a value", s.Name)}
}

// macroDef はマクロ 1 つ。fn (実行時の式・文に展開する) か constFn (定数式で評価する) のどちらか。
type macroDef struct {
	name    string
	fn      MacroFn
	constFn ConstMacroFn
	typing  *macroTyping // 型を決める段が呼び出しの型を知る方法 (typing.go。nil なら分からない)
	textmap *textmapConv // textmap(...) の変換器 (@format が書式の変換に使う)
}

// symName は値でない名前の節点 (cName)。
func symName(s *Symbol) *cexpr { return &cexpr{kind: cName, sym: s} }

// macroOf は c が (評価済みの) マクロの名前ならそのマクロ (ほかは nil)。
func (c *cexpr) macroOf() *macroDef {
	if c != nil && c.kind == cName && c.sym.Macro != nil {
		return c.sym.Macro
	}
	return nil
}

// textmapOf は c が textmap の変換器ならその変換器 (ほかは nil)。
func (c *cexpr) textmapOf() *textmapConv {
	if m := c.macroOf(); m != nil {
		return m.textmap
	}
	return nil
}

// typeOf は c が (評価済みの) 型名ならその型 (ほかは nil。soa のコンテナは値なので nil)。
func (c *cexpr) typeOf() *types.Type {
	if c != nil && c.kind == cName && c.sym.Type != nil {
		return c.sym.Type
	}
	return nil
}

// moduleOf は c が (評価済みの) モジュールの名前ならその id (ほかは "")。
func (c *cexpr) moduleOf() string {
	if c != nil && c.kind == cName {
		return c.sym.Module
	}
	return ""
}
