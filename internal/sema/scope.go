package sema

// スコープ (名前 → Symbol) とモジュールの外面 (ModuleInterface)。lib/fc/base.rb の Scope 由来。名前の解決は sema の
// 仕事なので sema が持つ (2026-10-05 に ir から移した。ir.Module は名前の表を持たない)。

import (
	"fmt"

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/ir"
)

type Scope struct {
	Parent *Scope
	// Reserved は宣言できない名前 → 理由 (fc 3 のモジュールの型名 u8 など。型名は型の位置でスコープより先に引くので、
	// 同名の宣言は黙って隠れてしまう)。子のスコープに引き継ぐ
	Reserved map[string]string
	// Hidden はモジュールスコープから親 (グローバル) へ辿らない名前 → 代わりの名前 (fc 3 のモジュールで、`@` の付かない
	// 組み込みの名前 asm / min など。fc 3 では @asm と書く)。モジュール自身の宣言・取り込みは見える
	Hidden   map[string]string
	declares map[string]*Symbol
	order    []string              // 宣言順 (IdList の列挙順が出力に影響するため保つ)
	aliases  map[string]scopeAlias // `use a, b from mod;` (v2) で束縛した名前
	aliasOrd []string
	uses     []scopeUse      // `use * from mod;` で取り込んだモジュール (public のみ見える)
	finding  map[string]bool // glob traversal guard, per name (resolution may look up another name)
	listing  bool
	deferred map[string]deferredBinding
}

// scopeAlias は選択的インポート 1 件。他モジュールの宣言 (Value) をこのスコープの名前に束縛する。
// reexport なら外 (Lookup) からも見える (`public use a from mod;`)。
type scopeAlias struct {
	sym      *Symbol
	reexport bool
}

// scopeUse は glob 取り込み 1 件。reexport なら外 (Lookup) からもこの取り込みを辿れる
// (v1 は常に再輸出、v2 は `public use * from mod;` のときだけ。Agent/discussions/2026-09-13-v2-grammar.md §3.2)。
type scopeUse struct {
	mi       *ModuleInterface
	reexport bool
}

func NewScope(parent *Scope) *Scope {
	s := &Scope{Parent: parent, declares: map[string]*Symbol{}}
	if parent != nil {
		s.Reserved = parent.Reserved
	}
	return s
}

// checkReserved は name が宣言できない名前ならエラー。
func (s *Scope) checkReserved(name string) {
	if why, ok := s.Reserved[name]; ok {
		panic(&diag.Error{Msg: fmt.Sprintf("%s cannot be declared (%s)", name, why)})
	}
}

// Find は id を探す。withPrivate が偽なら外から見える宣言 (public な宣言と再輸出された glob 取り込み) だけを見る。
// 自スコープ → use したスコープ (public のみ) → 親スコープ の順。
func (s *Scope) Find(id string, withPrivate bool) *Symbol {
	if d, ok := s.deferred[id]; ok && (withPrivate || d.public) {
		d.resolve()
	}
	if s.finding[id] {
		return nil
	}
	if s.finding == nil {
		s.finding = map[string]bool{}
	}
	s.finding[id] = true
	defer delete(s.finding, id)
	if sym, ok := s.declares[id]; ok {
		if withPrivate || sym.Public() {
			return sym
		}
	}
	if a, ok := s.aliases[id]; ok {
		if withPrivate || a.reexport {
			return a.sym
		}
	}
	for _, u := range s.uses {
		if !withPrivate && !u.reexport {
			continue
		}
		if v := u.mi.Lookup(id); v != nil {
			return v
		}
	}
	if _, hidden := s.Hidden[id]; hidden {
		return nil
	}
	if s.Parent != nil {
		return s.Parent.Find(id, withPrivate)
	}
	return nil
}

// hiddenHint は id が fc 3 で隠した組み込みの名前なら、その代わりの名前 (スコープを親へ辿って探す)。
func (s *Scope) hiddenHint(id string) string {
	for x := s; x != nil; x = x.Parent {
		if h, ok := x.Hidden[id]; ok {
			return h
		}
	}
	return ""
}

// FindMust は Find と同じだが、見つからなければ CompileError。
func (s *Scope) FindMust(id string, withPrivate bool) *Symbol {
	if v := s.Find(id, withPrivate); v != nil {
		return v
	}
	msg := fmt.Sprintf("%s not found", id)
	if h := s.hiddenHint(id); h != "" {
		msg += fmt.Sprintf(" (write %s in fc 3; `fcc migrate` rewrites fc 2 sources)", h)
	} else if hint := s.Suggest(id); hint != "" {
		msg += fmt.Sprintf(" (did you mean %s?)", hint)
	}
	panic(&diag.Error{Msg: msg})
}

// Suggest は id に綴りの近い見える名前を返す (編集距離 2 以内で最も近いもの。無ければ "")。
func (s *Scope) Suggest(id string) string {
	best, bestD := "", 3
	if len(id) < 3 {
		return ""
	}
	seen := map[string]bool{}
	for _, name := range s.IdList() {
		if seen[name] || name == id {
			continue
		}
		seen[name] = true
		if d := editDistance(id, name); d < bestD || (d == bestD && best != "" && name < best) {
			best, bestD = name, d
		}
	}
	return best
}

// editDistance はレーベンシュタイン距離 (大文字小文字の違いは 0.5 扱いにせず 1 のまま)。
func editDistance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(rb)]
}

// Local はこのスコープ自身で宣言した id の値 (遅延の解決を起こさない。無ければ、または値でなければ nil)。
func (s *Scope) Local(id string) *ir.Value {
	if sym := s.declares[id]; sym != nil {
		return sym.Val
	}
	return nil
}

// DeclaredHere はこのスコープ自身に id の宣言 (または束縛) があるか。
func (s *Scope) DeclaredHere(id string) bool {
	if _, ok := s.deferred[id]; ok {
		return true
	}
	if _, ok := s.declares[id]; ok {
		return true
	}
	_, ok := s.aliases[id]
	return ok
}

// Declare は値を宣言する。同名が既にあれば CompileError (選択的インポートとの衝突も含む: 規則 S2)。
func (s *Scope) Declare(val *ir.Value) *Symbol {
	sym := valueSym(val)
	s.DeclareSym(sym)
	return sym
}

// DeclareSym は名前 sym.Name を宣言する (値でない束縛も)。
func (s *Scope) DeclareSym(sym *Symbol) {
	s.checkReserved(sym.Name)
	if _, ok := s.declares[sym.Name]; ok {
		panic(&diag.Error{Msg: fmt.Sprintf("%s already defined", sym.Name)})
	}
	if _, ok := s.aliases[sym.Name]; ok {
		panic(&diag.Error{Msg: fmt.Sprintf("%s already imported", sym.Name)})
	}
	s.declares[sym.Name] = sym
	if _, reserved := s.deferred[sym.Name]; !reserved {
		s.order = append(s.order, sym.Name)
	}
}

// Alias は他モジュールの宣言 sym を name でこのスコープに束縛する (`use a, b from mod;`)。
// 自宣言・既存の束縛と同名なら CompileError (規則 S2)。
func (s *Scope) Alias(name string, sym *Symbol, reexport bool) {
	s.checkReserved(name)
	if _, ok := s.declares[name]; ok {
		panic(&diag.Error{Msg: fmt.Sprintf("%s already defined", name)})
	}
	if _, ok := s.aliases[name]; ok {
		panic(&diag.Error{Msg: fmt.Sprintf("%s already imported", name)})
	}
	if s.aliases == nil {
		s.aliases = map[string]scopeAlias{}
	}
	s.aliases[name] = scopeAlias{sym: sym, reexport: reexport}
	if _, reserved := s.deferred[name]; !reserved {
		s.aliasOrd = append(s.aliasOrd, name)
	}
}

// Use はモジュールの public な宣言をこのスコープから見えるようにする (`use * from mod;`)。
// reexport なら、このスコープを外から見たときにも取り込んだ宣言が見える (`public use * from mod;`)。
func (s *Scope) Use(mi *ModuleInterface, reexport bool) {
	s.uses = append(s.uses, scopeUse{mi: mi, reexport: reexport})
}

// IdList はスコープから見えるIDの列挙 (自スコープの宣言順 → use 先 → 親)。
func (s *Scope) IdList() []string {
	if s.listing {
		return nil
	}
	s.listing = true
	defer func() { s.listing = false }()
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

type deferredBinding struct {
	public, imported bool
	resolve          func()
}

// Reserve records a declaration before its value is known. The resolver must
// eventually bind it using Declare/Alias and call Complete, including on errors.
// Reservation checks duplicates without forcing either declaration to evaluate.
func (s *Scope) Reserve(name string, public, imported bool, resolve func()) {
	if s.DeclaredHere(name) {
		what := "defined"
		if d, ok := s.deferred[name]; ok && d.imported {
			what = "imported"
		}
		if _, ok := s.aliases[name]; ok {
			what = "imported"
		}
		panic(&diag.Error{Msg: fmt.Sprintf("%s already %s", name, what)})
	}
	if s.deferred == nil {
		s.deferred = map[string]deferredBinding{}
	}
	s.deferred[name] = deferredBinding{public, imported, resolve}
	if imported {
		s.aliasOrd = append(s.aliasOrd, name)
	} else {
		s.order = append(s.order, name)
	}
}
func (s *Scope) Complete(name string) { delete(s.deferred, name) }

// BoundHere checks actual bindings without evaluating a deferred declaration.
func (s *Scope) BoundHere(name string) bool {
	_, declared := s.declares[name]
	_, aliased := s.aliases[name]
	return declared || aliased
}

// ModuleInterface は importer から見えるモジュールの外面 (Agent/discussions/2026-09-12-v2-plan.md C4)。
// 宣言の検索と識別だけを提供し、Lambda 本体や IR には触れさせない。
//
// 可視性は宣言側モジュールの文法バージョンで決まる (Agent/discussions/2026-09-13-v2-grammar.md §3.2, §4.2):
//   - `use * from mod;` と `mod.name` のドット参照 (Lookup) は public だけを見る (規則 S7)
type ModuleInterface struct {
	Id    string
	scope *Scope
}

// Lookup はドット参照 `mod.name` 用に公開宣言を探す (無ければ nil)。
func (mi *ModuleInterface) Lookup(name string) *Symbol {
	return mi.scope.Find(name, false)
}

// LookupInternal は private も含めて探す (コンパイラ組み込み機能の内部参照用。利用者コードの名前解決には使わない)。
func (mi *ModuleInterface) LookupInternal(name string) *Symbol {
	return mi.scope.Find(name, true)
}

// LookupMust は Lookup と同じだが、見つからなければ diag.Error。private を指していたら、その旨を伝える。
func (mi *ModuleInterface) LookupMust(name string) *Symbol {
	if v := mi.Lookup(name); v != nil {
		return v
	}
	if mi.LookupInternal(name) != nil {
		panic(&diag.Error{Msg: fmt.Sprintf("%s.%s is private (declare it with `public` in module %s)", mi.Id, name, mi.Id)})
	}
	panic(&diag.Error{Msg: fmt.Sprintf("%s not found", name)})
}

// Exports は公開宣言の名前を宣言順に列挙する。
func (mi *ModuleInterface) Exports() []string {
	var r []string
	for _, id := range mi.scope.order {
		if sym := mi.scope.declares[id]; sym != nil && sym.Public() {
			r = append(r, id)
		}
	}
	return r
}
