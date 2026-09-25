package sema

// fc 3 の static if (`@if`) と、ビルドで上書きできる const (`@(build)`)。doc/v3_plan.md §1。
//
//   - `public const DEBUG = false @(build);` はモジュールが既定値つきで宣言する。値は Program.Defines
//     (fc.toml の [define.<module>] と CLI の -D) で上書きできる。トップレベルの const だけ、初期値はリテラルだけ、@if の中は不可
//   - `@if (cond) { ... } else { ... }` の条件はリテラルと @(build) の定数 (とその演算) だけ。選ばれなかった側は名前解決・
//     型検査をしない (use も読み込まない)。新しいスコープは作らない
//   - トップレベルの @if は宣言を集めた後、use を読み込んでから評価し、選ばれた側の文を集める (その中の use を読み込んで、
//     入れ子の @if を繰り返す)。関数の中の @if は文をコンパイルするときに評価する

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/syntax"
)

// DefineUse は上書きの値 1 つの使われ方 (fcc build -d の表示と検査)。
type DefineUse struct {
	Key    string // module.NAME
	Value  string
	Source string // 出所 (fc.toml のパス / "-D")
	Used   bool   // 宣言された @(build) の定数を上書きした
}

// pendingIf はトップレベルで評価を待つ @if。
type pendingIf struct {
	stmt  *syntax.StaticIfStmt
	group *declaration
}

// staticCond は @if の条件を評価する (リテラルと @(build) の定数だけ)。
func (h *Hlc) staticCond(e syntax.Expr) bool {
	h.checkStaticExpr(e)
	cv := h.constEval(toC(e))
	if cv.kind != cValue || cv.val.Kind != ir.KindLiteral || !cv.val.IsInt {
		panic(&diag.Error{Msg: "@if condition must be a constant (literals and @(build) constants)"})
	}
	return cv.val.Int != 0
}

// checkStaticExpr は @if の条件の式にリテラルと @(build) の定数 (とその演算) しか無いか検査する。
func (h *Hlc) checkStaticExpr(e syntax.Expr) {
	switch x := e.(type) {
	case *syntax.IntLit, *syntax.BoolLit:
	case *syntax.ParenExpr:
		h.checkStaticExpr(x.X)
	case *syntax.UnaryExpr:
		h.checkStaticExpr(x.X)
	case *syntax.BinaryExpr:
		if x.Op == syntax.Dot {
			h.checkBuildRef(e)
			return
		}
		h.checkStaticExpr(x.X)
		h.checkStaticExpr(x.Y)
	case *syntax.Ident:
		h.checkBuildRef(e)
	default:
		h.updatePos(e)
		panic(&diag.Error{Msg: "@if condition can use only literals and @(build) constants"})
	}
}

// checkBuildRef は名前の参照 (`DEBUG` / `common.DEBUG`) が @(build) の定数か検査する。
func (h *Hlc) checkBuildRef(e syntax.Expr) {
	h.updatePos(e)
	cv := h.constEval(toC(e))
	if cv.kind != cValue || !cv.val.Build {
		panic(&diag.Error{Msg: fmt.Sprintf("@if condition can use only literals and @(build) constants (%s is not @(build))", exprText(e))})
	}
}

// exprText は診断用の式の綴り (名前と `a.b` だけ)。
func exprText(e syntax.Expr) string {
	switch x := e.(type) {
	case *syntax.Ident:
		return x.Name
	case *syntax.BinaryExpr:
		if x.Op == syntax.Dot {
			return exprText(x.X) + "." + exprText(x.Y)
		}
	}
	return "the expression"
}

// staticBranch は @if の選ばれた側の文 (無ければ nil)。else @if は 1 つの文として返す。
func (h *Hlc) staticBranch(s *syntax.StaticIfStmt) []syntax.Stmt {
	if h.staticCond(s.Cond) {
		return s.Then.Stmts
	}
	switch e := s.Else.(type) {
	case *syntax.Block:
		return e.Stmts
	case *syntax.StaticIfStmt:
		return []syntax.Stmt{e}
	}
	return nil
}

// buildConstInit は @(build) の const の初期値 (上書きがあればその値) を返す。宣言の場所・初期値の形を検査する。
func (h *Hlc) buildConstInit(name string, sp *syntax.VarSpec) syntax.Expr {
	if h.lmd != nil || h.module == nil {
		panic(&diag.Error{Msg: fmt.Sprintf("@(build) const %s must be at module level", name)})
	}
	if h.inStaticIf {
		panic(&diag.Error{Msg: fmt.Sprintf("@(build) const %s cannot be declared inside @if", name)})
	}
	isBool, ok := literalKind(sp.Init)
	if !ok {
		panic(&diag.Error{Msg: fmt.Sprintf("@(build) const %s must be initialized with a literal (true / false / integer)", name)})
	}
	key := h.module.Id + "." + name
	d := h.prog.Defines[key]
	if d == nil {
		return sp.Init
	}
	d.Used = true
	pos := sp.Init.Pos()
	switch {
	case d.Value == "true" || d.Value == "false":
		if !isBool {
			panic(&diag.Error{Msg: fmt.Sprintf("%s = %s (%s): %s is an integer @(build) const", key, d.Value, d.Source, name)})
		}
		return &syntax.BoolLit{ValuePos: pos, Value: d.Value == "true"}
	default:
		n, err := strconv.ParseInt(d.Value, 0, 32)
		if err != nil {
			panic(&diag.Error{Msg: fmt.Sprintf("%s = %s (%s): the value must be true, false or an integer", key, d.Value, d.Source)})
		}
		if isBool {
			panic(&diag.Error{Msg: fmt.Sprintf("%s = %s (%s): %s is a bool @(build) const", key, d.Value, d.Source, name)})
		}
		if n < 0 {
			return &syntax.UnaryExpr{OpPos: pos, Op: syntax.Minus, X: &syntax.IntLit{ValuePos: pos, Value: int(-n), Text: strconv.Itoa(int(-n))}}
		}
		return &syntax.IntLit{ValuePos: pos, Value: int(n), Text: d.Value}
	}
}

// literalKind は e が bool / 整数のリテラル (負の整数を含む) か。
func literalKind(e syntax.Expr) (isBool, ok bool) {
	switch x := e.(type) {
	case *syntax.BoolLit:
		return true, true
	case *syntax.IntLit:
		return false, true
	case *syntax.UnaryExpr:
		if _, isInt := x.X.(*syntax.IntLit); isInt && x.Op == syntax.Minus {
			return false, true
		}
	}
	return false, false
}

// CheckDefines はビルドの後、上書きの値が宣言された @(build) の定数に当たったかを検査する。モジュールがビルドに
// 含まれていない (@if で読み込まれなかった) だけなら警告、モジュールが見つからない (exists が false) ならエラー。
func (p *Program) CheckDefines(exists func(module string) bool) error {
	keys := make([]string, 0, len(p.Defines))
	for k := range p.Defines {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		d := p.Defines[k]
		if d.Used {
			continue
		}
		mod, name := splitDefine(k)
		if m, ok := p.Modules.Get(mod); ok {
			_ = m
			return &diag.Error{Msg: fmt.Sprintf("%s = %s (%s): %s has no @(build) const %s", k, d.Value, d.Source, mod, name)}
		}
		if !exists(mod) {
			return &diag.Error{Msg: fmt.Sprintf("%s = %s (%s): module %s not found", k, d.Value, d.Source, mod)}
		}
		p.Warnings = append(p.Warnings, diag.Warning{Msg: fmt.Sprintf("%s = %s (%s): module %s is not part of this build (not used)", k, d.Value, d.Source, mod)})
	}
	return nil
}

func splitDefine(k string) (mod, name string) {
	for i := len(k) - 1; i >= 0; i-- {
		if k[i] == '.' {
			return k[:i], k[i+1:]
		}
	}
	return "", k
}
