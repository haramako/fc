package fc

// 型付き構文木 (internal/syntax) → 旧 S 式 AST ([]any) への変換。
//
// R1-b から R1-c までの一時的な境界コード (doc/v2_plan.md §4.3)。sema (hlc.go) が
// 型付き構文木を直接読むようになる R1-c で削除する。
// 変換結果は旧パーサ (parser.y) が生成していた S 式と構造的に同一でなければならない
// (lower_test.go で旧パーサとの差分テストを行う)。

import (
	"github.com/haramako/fc/internal/syntax"
)

// Lower は構文木を旧 AST と pos_info に変換する。
// pos_info のキーは旧実装と同じく文ノードの構造的等値 (Canon)、値は [filename, line]。
// 行番号は文の開始行 (旧実装は reduce 時点のレキサ行で、概ね文の末尾行だった)。
func Lower(f *syntax.File) (ast []any, posInfo *OMap) {
	l := &lowerer{filename: f.Filename, posInfo: NewOMap()}
	return l.stmts(f.Stmts), l.posInfo
}

type lowerer struct {
	filename string
	posInfo  *OMap
}

// stmts は文のリストを変換する。空のリストは nil でなく []any{} (旧 opt_statement_list と同じ)。
func (l *lowerer) stmts(ss []syntax.Stmt) []any {
	r := make([]any, 0, len(ss))
	for _, s := range ss {
		r = append(r, l.stmt(s))
	}
	return r
}

// body は if/else/loop/while の直後の単文を変換する。旧文法では statement_i を経由しない
// (statement_list の要素ではない) ので pos_info には登録しない。
func (l *lowerer) body(s syntax.Stmt) any {
	return l.stmtNode(s)
}

// block は省略可能なブロックを変換する (nil → nil、ブロック → 文リスト)。
func (l *lowerer) block(b *syntax.Block) any {
	if b == nil {
		return nil
	}
	return l.stmts(b.Stmts)
}

func (l *lowerer) stmt(s syntax.Stmt) any {
	node := l.stmtNode(s)
	l.posInfo.Set(node, []any{l.filename, s.Pos().Line})
	return node
}

func scopeSym(pub syntax.Pos) any {
	if pub.IsValid() {
		return Sym("public")
	}
	return nil
}

func (l *lowerer) stmtNode(s syntax.Stmt) any {
	switch s := s.(type) {
	case *syntax.VarDecl:
		kw := Sym("var")
		if s.Const {
			kw = Sym("const")
		}
		return []any{kw, l.varSpecs(s.Specs), scopeSym(s.PublicPos)}
	case *syntax.FuncDecl:
		return []any{Sym("function"), scopeSym(s.PublicPos), Sym(s.Name.Name), l.varSpecs(s.Params),
			l.typ(s.Result), l.options(s.Options), l.block(s.Body)}
	case *syntax.IfStmt:
		return l.ifStmt(s)
	case *syntax.LoopStmt:
		return []any{Sym("loop"), l.body(s.Body)}
	case *syntax.WhileStmt:
		return []any{Sym("while"), l.expr(s.Cond), l.body(s.Body)}
	case *syntax.ForStmt:
		return []any{Sym("for"), Sym(s.Var.Name), l.expr(s.From), l.expr(s.To), l.stmts(s.Body.Stmts)}
	case *syntax.BreakStmt:
		return []any{Sym("break")}
	case *syntax.ContinueStmt:
		return []any{Sym("continue")}
	case *syntax.ReturnStmt:
		return []any{Sym("return"), l.optExpr(s.Value)}
	case *syntax.SwitchStmt:
		cases := make([]any, 0, len(s.Cases))
		for _, c := range s.Cases {
			cases = append(cases, []any{l.exprs(c.Values), l.stmts(c.Body)})
		}
		var def any
		if s.Default != nil {
			def = l.stmts(s.Default.Body)
		}
		return []any{Sym("switch"), l.expr(s.Tag), cases, def}
	case *syntax.ExprStmt:
		return []any{Sym("exp"), l.expr(s.X)}
	case *syntax.OptionsStmt:
		return []any{Sym("options"), l.options(s.Options)}
	case *syntax.UseDecl:
		var as, from any
		if s.As != nil {
			as = Sym(s.As.Name)
		}
		if s.FromAll {
			from = "*"
		}
		return []any{Sym("use"), Sym(s.Module.Name), as, from}
	case *syntax.IncludeDecl:
		var kind any
		if s.Kind != nil {
			kind = Sym(s.Kind.Name)
		}
		return []any{Sym("include"), s.Path.Value, kind, l.options(s.Options)}
	case *syntax.ScopeLabel:
		if s.Public {
			return []any{Sym("public")}
		}
		return []any{Sym("private")}
	case *syntax.Block:
		return []any{Sym("block"), l.stmts(s.Stmts)}
	case *syntax.EmptyStmt:
		return []any{Sym("blank")}
	}
	panic("lower: unknown statement")
}

// ifStmt は if / elsif 連鎖を (if cond then else) の入れ子に変換する。
func (l *lowerer) ifStmt(s *syntax.IfStmt) any {
	// 評価順 = 旧パーサの reduce 順 (then 節の内側の文 → else 節の内側の文) を保つ
	cond := l.expr(s.Cond)
	then := l.body(s.Then)
	var els any
	switch e := s.Else.(type) {
	case nil:
	case *syntax.IfStmt:
		if e.IsElsif {
			els = l.ifStmt(e)
		} else {
			els = l.body(e)
		}
	default:
		els = l.body(e)
	}
	return []any{Sym("if"), cond, then, els}
}

func (l *lowerer) varSpecs(specs []*syntax.VarSpec) []any {
	r := make([]any, 0, len(specs))
	for _, sp := range specs {
		var typ any
		if sp.Type != nil {
			typ = l.typ(sp.Type)
		}
		r = append(r, []any{Sym(sp.Name.Name), typ, l.optExpr(sp.Init), l.options(sp.Options)})
	}
	return r
}

func (l *lowerer) options(o *syntax.Options) any {
	if o == nil {
		return nil
	}
	pairs := make([]any, 0, len(o.Entries)*2)
	for _, e := range o.Entries {
		pairs = append(pairs, Sym(e.Key.Name), l.expr(e.Value))
	}
	return OMapFromPairs(pairs)
}

func (l *lowerer) optExpr(e syntax.Expr) any {
	if e == nil {
		return nil
	}
	return l.expr(e)
}

func (l *lowerer) exprs(es []syntax.Expr) []any {
	r := make([]any, 0, len(es))
	for _, e := range es {
		r = append(r, l.expr(e))
	}
	return r
}

var binaryOpSym = map[syntax.Kind]Sym{
	syntax.Dot: "dot", syntax.Plus: "add", syntax.Minus: "sub", syntax.Star: "mul",
	syntax.Slash: "div", syntax.Percent: "mod", syntax.Amp: "and", syntax.Pipe: "or",
	syntax.Caret: "xor", syntax.AndAnd: "land", syntax.OrOr: "lor",
	syntax.EqEq: "eq", syntax.Neq: "ne", syntax.Lt: "lt", syntax.Gt: "gt",
	syntax.Leq: "le", syntax.Geq: "ge", syntax.Shl: "shift_left", syntax.Shr: "shift_right",
}

var unaryOpSym = map[syntax.Kind]Sym{
	syntax.Not: "not", syntax.Minus: "uminus", syntax.Star: "deref", syntax.Amp: "ref",
}

func (l *lowerer) expr(e syntax.Expr) any {
	switch e := e.(type) {
	case *syntax.Ident:
		return Sym(e.Name)
	case *syntax.IntLit:
		return e.Value
	case *syntax.StringLit:
		return e.Value
	case *syntax.ParenExpr:
		return l.expr(e.X)
	case *syntax.BinaryExpr:
		return []any{binaryOpSym[e.Op], l.expr(e.X), l.expr(e.Y)}
	case *syntax.AssignExpr:
		lhs := l.expr(e.Lhs)
		switch e.Op {
		case syntax.Assign:
			return []any{Sym("load"), lhs, l.expr(e.Rhs)}
		case syntax.AddEq:
			// 旧文法の脱糖: (load X (add X rhs))。X の部分木は同一オブジェクトを共有する
			return []any{Sym("load"), lhs, []any{Sym("add"), lhs, l.expr(e.Rhs)}}
		case syntax.SubEq:
			return []any{Sym("load"), lhs, []any{Sym("sub"), lhs, l.expr(e.Rhs)}}
		}
	case *syntax.UnaryExpr:
		if e.Op == syntax.Plus {
			return l.expr(e.X) // 単項 + は旧文法で消える
		}
		return []any{unaryOpSym[e.Op], l.expr(e.X)}
	case *syntax.CastExpr:
		return []any{Sym("cast"), l.expr(e.X), l.typ(e.Type)}
	case *syntax.CallExpr:
		return []any{Sym("call"), l.expr(e.Fun), l.exprs(e.Args), l.block(e.Block)}
	case *syntax.IndexExpr:
		return []any{Sym("index"), l.expr(e.X), l.expr(e.Index)}
	case *syntax.ArrayLit:
		return []any{Sym("array"), l.exprs(e.Elems)}
	case *syntax.IncbinExpr:
		return []any{Sym("incbin"), e.Path.Value}
	case *syntax.LambdaExpr:
		return []any{Sym("lambda"), l.typ(e.Type), l.block(e.Body)}
	}
	panic("lower: unknown expression")
}

func (l *lowerer) typ(t syntax.TypeExpr) any {
	switch t := t.(type) {
	case *syntax.NamedType:
		return Sym(t.Name.Name)
	case *syntax.ArrayType:
		return []any{Sym("array"), l.optExpr(t.Len), l.typ(t.Elem)}
	case *syntax.PointerType:
		return []any{Sym("pointer"), l.typ(t.Elem)}
	case *syntax.FuncType:
		params := make([]any, 0, len(t.Params))
		for _, p := range t.Params {
			if p.Name != nil {
				params = append(params, []any{Sym(p.Name.Name), l.typ(p.Type)})
			} else {
				params = append(params, l.typ(p.Type))
			}
		}
		return []any{Sym("lambda"), params, l.typ(t.Result)}
	}
	panic("lower: unknown type")
}
