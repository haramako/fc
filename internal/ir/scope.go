package ir

// スコープ (名前 → Value)。lib/fc/base.rb の Scope 由来。

import (
	"fmt"

	"github.com/haramako/fc/internal/diag"
)

type Scope struct {
	Parent   *Scope
	declares map[string]*Value
	order    []string              // 宣言順 (IdList の列挙順が出力に影響するため保つ)
	aliases  map[string]scopeAlias // `use a, b from mod;` (v2) で束縛した名前
	aliasOrd []string
	uses     []scopeUse // `use * from mod;` で取り込んだモジュール (public のみ見える)
	finding  bool       // useの相互参照による無限再帰の防止フラグ
}

// scopeAlias は選択的インポート 1 件。他モジュールの宣言 (Value) をこのスコープの名前に束縛する。
// reexport なら外 (LookupPublic) からも見える (`public use a from mod;`)。
type scopeAlias struct {
	val      *Value
	reexport bool
}

// scopeUse は glob 取り込み 1 件。reexport なら外 (LookupPublic) からもこの取り込みを辿れる
// (v1 は常に再輸出、v2 は `public use * from mod;` のときだけ。doc/v2_grammar.md §3.2)。
type scopeUse struct {
	mi       *ModuleInterface
	reexport bool
}

func NewScope(parent *Scope) *Scope {
	return &Scope{Parent: parent, declares: map[string]*Value{}}
}

// Find は id を探す。withPrivate が偽なら外から見える宣言 (public な宣言と再輸出された glob 取り込み) だけを見る。
// 自スコープ → use したスコープ (public のみ) → 親スコープ の順。
func (s *Scope) Find(id string, withPrivate bool) *Value {
	if s.finding {
		return nil
	}
	s.finding = true
	defer func() { s.finding = false }()
	if val, ok := s.declares[id]; ok {
		if withPrivate || val.Public {
			return val
		}
	}
	if a, ok := s.aliases[id]; ok {
		if withPrivate || a.reexport {
			return a.val
		}
	}
	for _, u := range s.uses {
		if !withPrivate && !u.reexport {
			continue
		}
		if v := u.mi.LookupPublic(id); v != nil {
			return v
		}
	}
	if s.Parent != nil {
		return s.Parent.Find(id, withPrivate)
	}
	return nil
}

// FindMust は Find と同じだが、見つからなければ CompileError。
func (s *Scope) FindMust(id string, withPrivate bool) *Value {
	if v := s.Find(id, withPrivate); v != nil {
		return v
	}
	panic(&diag.Error{Msg: fmt.Sprintf("%s not found", id)})
}

// Declare は値を宣言する。同名が既にあれば CompileError (選択的インポートとの衝突も含む: 規則 S2)。
func (s *Scope) Declare(val *Value) {
	if _, ok := s.declares[val.Name]; ok {
		panic(&diag.Error{Msg: fmt.Sprintf("%s already defined", val.Name)})
	}
	if _, ok := s.aliases[val.Name]; ok {
		panic(&diag.Error{Msg: fmt.Sprintf("%s already imported", val.Name)})
	}
	s.declares[val.Name] = val
	s.order = append(s.order, val.Name)
}

// Alias は他モジュールの宣言 val を name でこのスコープに束縛する (`use a, b from mod;`)。
// 自宣言・既存の束縛と同名なら CompileError (規則 S2)。
func (s *Scope) Alias(name string, val *Value, reexport bool) {
	if _, ok := s.declares[name]; ok {
		panic(&diag.Error{Msg: fmt.Sprintf("%s already defined", name)})
	}
	if _, ok := s.aliases[name]; ok {
		panic(&diag.Error{Msg: fmt.Sprintf("%s already imported", name)})
	}
	if s.aliases == nil {
		s.aliases = map[string]scopeAlias{}
	}
	s.aliases[name] = scopeAlias{val: val, reexport: reexport}
	s.aliasOrd = append(s.aliasOrd, name)
}

// Use はモジュールの public な宣言をこのスコープから見えるようにする (`use * from mod;`)。
// reexport なら、このスコープを外から見たときにも取り込んだ宣言が見える (`public use * from mod;`)。
func (s *Scope) Use(mi *ModuleInterface, reexport bool) {
	s.uses = append(s.uses, scopeUse{mi: mi, reexport: reexport})
}

// IdList はスコープから見えるIDの列挙 (自スコープの宣言順 → use 先 → 親)。
func (s *Scope) IdList() []string {
	if s.finding {
		return nil
	}
	s.finding = true
	defer func() { s.finding = false }()
	var r []string
	r = append(r, s.order...)
	r = append(r, s.aliasOrd...)
	for _, u := range s.uses {
		r = append(r, u.mi.scope.IdList()...)
	}
	if s.Parent != nil {
		r = append(r, s.Parent.IdList()...)
	}
	return r
}
