package fc

// lib/fc/base.rb の Scope の移植。

import "fmt"

type Scope struct {
	Parent   *Scope
	Declares *OMap // key: Sym(id), val: *Value
	uses     []*Scope
	finding  bool // useの相互参照による無限再帰の防止フラグ
}

func NewScope(parent *Scope) *Scope {
	return &Scope{Parent: parent, Declares: NewOMap()}
}

func (s *Scope) Find(id Sym, withPrivate bool) *Value {
	if s.finding {
		return nil
	}
	s.finding = true
	defer func() { s.finding = false }()
	if v, ok := s.Declares.Get(id); ok {
		val := v.(*Value)
		if withPrivate || val.Public {
			return val
		}
	}
	for _, sc := range s.uses {
		if v := sc.Find(id, false); v != nil {
			return v
		}
	}
	if s.Parent != nil {
		return s.Parent.Find(id, withPrivate)
	}
	return nil
}

// FindMust は Ruby の find! 相当。
func (s *Scope) FindMust(id Sym, withPrivate bool) *Value {
	if v := s.Find(id, withPrivate); v != nil {
		return v
	}
	panic(&CompileError{Msg: fmt.Sprintf("%s not found", id)})
}

func (s *Scope) Declare(val *Value) {
	if _, ok := s.Declares.Get(val.Id); ok {
		panic(&CompileError{Msg: fmt.Sprintf("%v already defined", val.Id)})
	}
	s.Declares.Set(val.Id, val)
}

func (s *Scope) Use(scope *Scope) {
	s.uses = append(s.uses, scope)
}

// IdList はスコープから見えるIDの列挙 (Ruby と同じ順序)。
func (s *Scope) IdList() []Sym {
	if s.finding {
		return nil
	}
	s.finding = true
	defer func() { s.finding = false }()
	var r []Sym
	for _, e := range s.Declares.Entries() {
		r = append(r, e.Key.(Sym))
	}
	for _, sc := range s.uses {
		r = append(r, sc.IdList()...)
	}
	if s.Parent != nil {
		r = append(r, s.Parent.IdList()...)
	}
	return r
}
