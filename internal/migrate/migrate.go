// Package migrate は v1 ソースを文法 v2 に書き換える (fcc migrate。doc/v2_grammar.md §5)。
//
// 手順:
//  1. Analysis: v1 としてコンパイルしながら名前解決を観測し、モジュールをまたぐ参照を集める
//     (sema.Program.SetTrace)。
//  2. Rewrite: 各ファイルの構文木を書き換える (可視性、use、include、loop、break)。
//  3. 印字: syntax.Print (フォーマッタ) で出力する。
//
// 検証 (移行前後の asm 一致) は driver 側で行う。
package migrate

import (
	"fmt"
	"sort"
	"strings"

	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/syntax"
)

// Visibility は `public` の付け方。
type Visibility int

const (
	// Minimal は他モジュールから参照されている宣言だけを public にする (既定)。
	Minimal Visibility = iota
	// Preserve は v1 の実効可視性 (ラベルとデフォルト public) を public キーワードに写し、
	// さらに参照されている宣言を public にする。ライブラリ (fclib) 向け。
	Preserve
)

// Options は書き換えの設定。
type Options struct {
	Visibility Visibility
	// Textmaps は castle の macro.rb の置換: 名前 → 文字表ファイルのパス。
	// `include("macro.rb")` (組み込みで代替できない Ruby マクロ) をこの const 宣言に置き換える
	Textmaps map[string]string
}

// Analysis はモジュールをまたぐ参照の集計。
type Analysis struct {
	// public[mod][name]: mod の宣言 name が他モジュールから参照された (→ public)
	public map[string]map[string]bool
	// reexportBinding[mod][name]: mod の `use X;` 束縛 name が他モジュールから参照された (→ public use X)
	reexportBinding map[string]map[string]bool
	// reexportGlob[mod][via]: mod の `use * from via;` が他モジュールからの検索で辿られた (→ public use * from via)
	reexportGlob map[string]map[string]bool
}

func NewAnalysis() *Analysis {
	return &Analysis{
		public:          map[string]map[string]bool{},
		reexportBinding: map[string]map[string]bool{},
		reexportGlob:    map[string]map[string]bool{},
	}
}

// Trace は sema.Program.SetTrace に渡す観測関数。
func (a *Analysis) Trace(origin string, ev ir.TraceEvent) {
	if origin == ev.Scope {
		return // 自モジュール内の解決
	}
	switch ev.Kind {
	case ir.TraceHit:
		if ev.Value != nil && ev.Value.Module != nil {
			mark(a.reexportBinding, ev.Scope, ev.Name)
		} else {
			mark(a.public, ev.Scope, ev.Name)
		}
	case ir.TraceGlob:
		mark(a.reexportGlob, ev.Scope, ev.Via)
	}
}

func mark(m map[string]map[string]bool, mod, name string) {
	if m[mod] == nil {
		m[mod] = map[string]bool{}
	}
	m[mod][name] = true
}

// Referenced は mod の宣言 name が他モジュールから参照されているか。
func (a *Analysis) Referenced(mod, name string) bool { return a.public[mod][name] }

// builtinRubyFiles は組み込み (sema/builtins.go) で代替された v1 の Ruby マクロファイル。include は単に削除する。
var builtinRubyFiles = map[string]bool{"stdio.rb": true, "unittest.rb": true, "stdmacro.rb": true, "math.rb": true}

// Rewrite は v1 の構文木を v2 に書き換える (破壊的)。modID はファイルのモジュール id。
// 戻り値は人が確認すべき注記 (手作業が要る箇所など)。
func Rewrite(f *syntax.File, modID string, a *Analysis, opt Options) ([]string, error) {
	r := &rewriter{f: f, mod: modID, a: a, opt: opt}
	if f.Version >= syntax.Version2 {
		// v2 ファイルは、v2 の途中で足した書き換え (v1 形の for → C 型) だけを適用する
		r.bodies()
		return r.notes, nil
	}
	r.topLevel()
	r.bodies()
	f.Version = syntax.Version2 // Pragma は空のまま (ソースにプラグマ行が無いことを印字側に伝える)
	return r.notes, nil
}

type rewriter struct {
	f     *syntax.File
	mod   string
	a     *Analysis
	opt   Options
	notes []string
	seq   int // 合成ラベルの連番
}

func (r *rewriter) note(format string, args ...any) {
	r.notes = append(r.notes, fmt.Sprintf("%s: ", r.f.Filename)+fmt.Sprintf(format, args...))
}

// topLevel はトップレベルの文を書き換える: ラベル削除、可視性、use、include。
func (r *rewriter) topLevel() {
	curPublic := true // v1 のデフォルトは public。`private:` / `public:` で切り替わる
	var out []syntax.Stmt
	for _, s := range r.f.Stmts {
		switch s := s.(type) {
		case *syntax.ScopeLabel:
			curPublic = s.Public
			continue // v2 にラベルは無い

		case *syntax.VarDecl:
			names := make([]string, len(s.Specs))
			for i, sp := range s.Specs {
				names[i] = sp.Name.Name
			}
			s.PublicPos = r.publicPos(s.PublicPos, s.Keyword, curPublic, names...)

		case *syntax.FuncDecl:
			s.PublicPos = r.publicPos(s.PublicPos, s.Keyword, curPublic, s.Name.Name)

		case *syntax.UseDecl:
			switch {
			case s.FromAll:
				if r.a.reexportGlob[r.mod][s.Module.Name] {
					s.PublicPos = s.Use
				}
			default:
				name := s.Module.Name
				if s.As != nil {
					name = s.As.Name
				}
				if r.a.reexportBinding[r.mod][name] {
					s.PublicPos = s.Use
				}
			}

		case *syntax.IncludeDecl:
			if strings.HasSuffix(s.Path.Value, ".rb") {
				repl := r.replaceRubyInclude(s)
				out = append(out, repl...)
				continue
			}
			s.Kind = nil // `include macro(...)` のキンド指定は v2 に無い (拡張子で決まる)
		}
		out = append(out, s)
	}
	r.f.Stmts = out
}

// publicPos は宣言に付ける `public` の位置を決める (無効なら付けない)。
func (r *rewriter) publicPos(cur, keyword syntax.Pos, curPublic bool, names ...string) syntax.Pos {
	referenced := false
	for _, n := range names {
		if r.a.Referenced(r.mod, n) {
			referenced = true
		}
	}
	effective := cur.IsValid() || curPublic // v1 の実効可視性
	want := referenced
	if r.opt.Visibility == Preserve {
		want = want || effective
	}
	if !want {
		return syntax.Pos{}
	}
	if cur.IsValid() {
		return cur
	}
	return keyword // 合成した `public` はキーワードと同じ位置 (コメントの順序を保つため)
}

// replaceRubyInclude は include("*.rb") を v2 の形に置き換える。
func (r *rewriter) replaceRubyInclude(s *syntax.IncludeDecl) []syntax.Stmt {
	base := s.Path.Value
	if i := strings.LastIndexAny(base, "/\\"); i >= 0 {
		base = base[i+1:]
	}
	switch {
	case builtinRubyFiles[base]:
		return nil // printf / unittest_run_tests は組み込み
	case len(r.opt.Textmaps) > 0:
		var out []syntax.Stmt
		names := make([]string, 0, len(r.opt.Textmaps))
		for n := range r.opt.Textmaps {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			out = append(out, textmapConst(s.Include, n, r.opt.Textmaps[n], r.a.Referenced(r.mod, n)))
		}
		return out
	default:
		r.note("include(%q) は Ruby マクロなので v2 では使えない。textmap 等に手で置き換えること (--textmap NAME=PATH で自動置換できる)", s.Path.Value)
		return []syntax.Stmt{s}
	}
}

// textmapConst は `[public] const NAME = textmap("PATH");` を合成する。
func textmapConst(pos syntax.Pos, name, path string, public bool) syntax.Stmt {
	d := &syntax.VarDecl{Keyword: pos, Const: true, Semi: pos, Specs: []*syntax.VarSpec{{
		Name: &syntax.Ident{NamePos: pos, Name: name},
		Init: &syntax.CallExpr{
			Fun: &syntax.Ident{NamePos: pos, Name: "textmap"}, Lparen: pos, Rparen: pos,
			Args: []syntax.Expr{&syntax.StringLit{ValuePos: pos, EndPos: pos, Value: path, Text: fmt.Sprintf("%q", path)}},
		},
	}}}
	if public {
		d.PublicPos = pos
	}
	return d
}

// bodies は関数本体を書き換える: loop() → loop、switch 内の loop-break をラベル付きに。
func (r *rewriter) bodies() {
	for _, s := range r.f.Stmts {
		if fd, ok := s.(*syntax.FuncDecl); ok && fd.Body != nil {
			r.stmtList(fd.Body.Stmts, nil)
		}
	}
	// トップレベルのラムダ定数 (const f = ->void() { ... }) の本体も
	for _, s := range r.f.Stmts {
		if vd, ok := s.(*syntax.VarDecl); ok {
			for _, sp := range vd.Specs {
				if sp.Init != nil {
					r.lambdasIn(sp.Init)
				}
			}
		}
	}
}

// lambdasIn は式の中のラムダ本体を書き換える。
func (r *rewriter) lambdasIn(e syntax.Expr) {
	syntax.Inspect(e, func(n syntax.Node) bool {
		if l, ok := n.(*syntax.LambdaExpr); ok && l.Body != nil {
			r.stmtList(l.Body.Stmts, nil)
			return false
		}
		return true
	})
}

// encl は break の解決に使う、囲んでいる文の情報。
type encl struct {
	stmt     syntax.Stmt  // ループ文 (LoopStmt / WhileStmt / ForStmt) または SwitchStmt
	isSwitch bool         //
	holder   *syntax.Stmt // その文が入っている場所 (LabeledStmt で包んで差し替えるため)
}

// stmtList は文リストを走査する。chain は外側の囲み (末尾が最も内側)。
func (r *rewriter) stmtList(stmts []syntax.Stmt, chain []*encl) {
	for i := range stmts {
		r.stmt(&stmts[i], chain)
	}
}

func (r *rewriter) stmt(holder *syntax.Stmt, chain []*encl) {
	switch s := (*holder).(type) {
	case *syntax.LoopStmt:
		s.Rparen = syntax.Pos{} // loop() → loop
		if _, ok := s.Body.(*syntax.Block); !ok {
			// v2 の loop の本体はブロックだけ: `loop() stmt;` → `loop { stmt; }`
			s.Body = &syntax.Block{Lbrace: s.Body.Pos(), Stmts: []syntax.Stmt{s.Body}, Rbrace: s.Body.End()}
		}
		e := &encl{stmt: s, holder: holder}
		r.stmt(&s.Body, append(chain, e))
	case *syntax.WhileStmt:
		e := &encl{stmt: s, holder: holder}
		r.stmt(&s.Body, append(chain, e))
	case *syntax.ForStmt:
		if s.IsV1() {
			// for (i, from, to) → for (i = from; i < to; i++)。生成コードは同じ
			// (continue を含む場合だけ、v2 では step に飛ぶので変わる)
			pos := s.For
			s.Init = &syntax.ExprStmt{X: &syntax.AssignExpr{Lhs: s.Var, OpPos: pos, Op: syntax.Assign, Rhs: s.From}}
			s.Cond = &syntax.BinaryExpr{X: &syntax.Ident{NamePos: pos, Name: s.Var.Name}, OpPos: pos, Op: syntax.Lt, Y: s.To}
			s.Step = &syntax.IncDecStmt{X: &syntax.Ident{NamePos: pos, Name: s.Var.Name}, OpPos: pos, Op: syntax.Inc}
			s.Var, s.From, s.To = nil, nil, nil
		}
		e := &encl{stmt: s, holder: holder}
		r.stmtList(s.Body.Stmts, append(chain, e))
	case *syntax.SwitchStmt:
		e := &encl{stmt: s, isSwitch: true, holder: holder}
		for _, c := range s.Cases {
			r.stmtList(c.Body, append(chain, e))
		}
		if s.Default != nil {
			r.stmtList(s.Default.Body, append(chain, e))
		}
	case *syntax.LabeledStmt:
		r.stmt(&s.Stmt, chain)
	case *syntax.IfStmt:
		r.stmt(&s.Then, chain)
		if s.Else != nil {
			r.stmt(&s.Else, chain)
		}
	case *syntax.Block:
		r.stmtList(s.Stmts, chain)
	case *syntax.BreakStmt:
		// v1: break は最も内側のループを抜ける。v2 でも同じ意味になるように、
		// 最も内側の囲みが switch ならループにラベルを付けて `break L;` にする
		if s.Label != nil || len(chain) == 0 || !chain[len(chain)-1].isSwitch {
			return
		}
		for i := len(chain) - 1; i >= 0; i-- {
			if !chain[i].isSwitch {
				s.Label = &syntax.Ident{NamePos: s.Keyword, Name: r.labelFor(chain[i])}
				return
			}
		}
	case *syntax.ExprStmt:
		r.lambdasIn(s.X) // 式の中のラムダ本体
	case *syntax.VarDecl:
		for _, sp := range s.Specs {
			if sp.Init != nil {
				r.lambdasIn(sp.Init)
			}
		}
	case *syntax.ReturnStmt:
		if s.Value != nil {
			r.lambdasIn(s.Value)
		}
	}
}

// labelFor はループ文にラベルを付け (無ければ合成して LabeledStmt で包む)、その名前を返す。
func (r *rewriter) labelFor(e *encl) string {
	if l, ok := (*e.holder).(*syntax.LabeledStmt); ok {
		return l.Label.Name
	}
	r.seq++
	name := fmt.Sprintf("loop_%d", r.seq)
	pos := e.stmt.Pos()
	*e.holder = &syntax.LabeledStmt{Label: &syntax.Ident{NamePos: pos, Name: name}, Colon: pos, Stmt: e.stmt}
	return name
}
