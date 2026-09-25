package ir

// スコープ (名前 → Value)。lib/fc/base.rb の Scope 由来。

import (
	"fmt"

	"github.com/haramako/fc/internal/diag"
)

type Scope struct {
	Parent *Scope
	Owner  string // モジュールスコープならモジュール id (ローカル/グローバルは "")
	// Reserved は宣言できない名前 → 理由 (fc 3 のモジュールの型名 u8 など。型名は型の位置でスコープより先に引くので、
	// 同名の宣言は黙って隠れてしまう)。子のスコープに引き継ぐ
	Reserved map[string]string
	// Hidden はモジュールスコープから親 (グローバル) へ辿らない名前 → 代わりの名前 (fc 3 のモジュールで、`@` の付かない
	// 組み込みの名前 asm / min など。fc 3 では @asm と書く)。モジュール自身の宣言・取り込みは見える
	Hidden map[string]string
	trace    func(TraceEvent)
	declares map[string]*Value
	order    []string              // 宣言順 (IdList の列挙順が出力に影響するため保つ)
	aliases  map[string]scopeAlias // `use a, b from mod;` (v2) で束縛した名前
	aliasOrd []string
	uses     []scopeUse      // `use * from mod;` で取り込んだモジュール (public のみ見える)
	finding  map[string]bool // glob traversal guard, per name (resolution may look up another name)
	listing  bool
	deferred map[string]deferredBinding
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
	s := &Scope{Parent: parent, declares: map[string]*Value{}}
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

// TraceEvent は名前解決の観測 (fcc migrate の参照解析用。通常のコンパイルでは発生しない)。
//   - TraceHit: モジュール Scope の宣言/束縛 Name が見つかった
//   - TraceGlob: モジュール Scope の glob 取り込み (use * from Via) を辿って Name が見つかった
type TraceEvent struct {
	Kind  TraceKind
	Scope string // 観測したモジュールスコープの Owner
	Via   string // TraceGlob: 辿った glob 先のモジュール id
	Name  string
	Value *Value // TraceHit: 見つかった値
}

type TraceKind int

const (
	TraceHit TraceKind = iota
	TraceGlob
)

// SetTrace は名前解決の観測関数を設定する (最上位スコープに置き、子スコープから辿って呼ぶ)。
func (s *Scope) SetTrace(fn func(TraceEvent)) { s.trace = fn }

// withoutTrace は観測を一時的に止めて fn を実行する (コンパイラ内部の参照用)。
func (s *Scope) withoutTrace(fn func() *Value) *Value {
	for r := s; r != nil; r = r.Parent {
		if r.trace != nil {
			saved := r.trace
			r.trace = nil
			defer func() { r.trace = saved }()
			break
		}
	}
	return fn()
}

func (s *Scope) emitTrace(ev TraceEvent) {
	for r := s; r != nil; r = r.Parent {
		if r.trace != nil {
			r.trace(ev)
			return
		}
	}
}

// Find は id を探す。withPrivate が偽なら外から見える宣言 (public な宣言と再輸出された glob 取り込み) だけを見る。
// 自スコープ → use したスコープ (public のみ) → 親スコープ の順。
func (s *Scope) Find(id string, withPrivate bool) *Value {
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
	if val, ok := s.declares[id]; ok {
		if withPrivate || val.Public {
			if s.Owner != "" {
				s.emitTrace(TraceEvent{Kind: TraceHit, Scope: s.Owner, Name: id, Value: val})
			}
			return val
		}
	}
	if a, ok := s.aliases[id]; ok {
		if withPrivate || a.reexport {
			if s.Owner != "" {
				s.emitTrace(TraceEvent{Kind: TraceHit, Scope: s.Owner, Name: id, Value: a.val})
			}
			return a.val
		}
	}
	for _, u := range s.uses {
		if !withPrivate && !u.reexport {
			continue
		}
		if v := u.mi.LookupPublic(id); v != nil {
			if s.Owner != "" {
				s.emitTrace(TraceEvent{Kind: TraceGlob, Scope: s.Owner, Via: u.mi.Id, Name: id, Value: v})
			}
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
func (s *Scope) FindMust(id string, withPrivate bool) *Value {
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
func (s *Scope) Declare(val *Value) {
	s.checkReserved(val.Name)
	if _, ok := s.declares[val.Name]; ok {
		panic(&diag.Error{Msg: fmt.Sprintf("%s already defined", val.Name)})
	}
	if _, ok := s.aliases[val.Name]; ok {
		panic(&diag.Error{Msg: fmt.Sprintf("%s already imported", val.Name)})
	}
	s.declares[val.Name] = val
	if _, reserved := s.deferred[val.Name]; !reserved {
		s.order = append(s.order, val.Name)
	}
}

// Alias は他モジュールの宣言 val を name でこのスコープに束縛する (`use a, b from mod;`)。
// 自宣言・既存の束縛と同名なら CompileError (規則 S2)。
func (s *Scope) Alias(name string, val *Value, reexport bool) {
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
	s.aliases[name] = scopeAlias{val: val, reexport: reexport}
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
