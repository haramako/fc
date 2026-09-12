package ir

// スコープ (名前 → Value)。lib/fc/base.rb の Scope 由来。

import (
	"fmt"

	"github.com/haramako/fc/internal/diag"
)

type Scope struct {
	Parent   *Scope
	declares map[string]*Value
	order    []string           // 宣言順 (IdList の列挙順が出力に影響するため保つ)
	uses     []*ModuleInterface // `use * from mod;` で取り込んだモジュール (public のみ見える)
	finding  bool               // useの相互参照による無限再帰の防止フラグ
}

func NewScope(parent *Scope) *Scope {
	return &Scope{Parent: parent, declares: map[string]*Value{}}
}

// Find は id を探す。withPrivate が偽なら public な宣言だけを見る。
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
	for _, mi := range s.uses {
		if v := mi.LookupPublic(id); v != nil {
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

// Declare は値を宣言する。同名が既にあれば CompileError。
func (s *Scope) Declare(val *Value) {
	if _, ok := s.declares[val.Name]; ok {
		panic(&diag.Error{Msg: fmt.Sprintf("%s already defined", val.Name)})
	}
	s.declares[val.Name] = val
	s.order = append(s.order, val.Name)
}

// Use はモジュールの public な宣言をこのスコープから見えるようにする (`use * from mod;`)。
func (s *Scope) Use(mi *ModuleInterface) {
	s.uses = append(s.uses, mi)
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
	for _, mi := range s.uses {
		r = append(r, mi.scope.IdList()...)
	}
	if s.Parent != nil {
		r = append(r, s.Parent.IdList()...)
	}
	return r
}
