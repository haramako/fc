package syntax

// Inspect は構文木を深さ優先で走査し、各ノードについて f を呼ぶ。
// f が false を返すとそのノードの子には降りない。nil の省略可能ノードは訪問しない。
func Inspect(node Node, f func(Node) bool) {
	if node == nil || !f(node) {
		return
	}
	for _, c := range Children(node) {
		Inspect(c, f)
	}
}

// Children はノードの直接の子をソース順に返す。
func Children(node Node) []Node {
	var r []Node
	add := func(n Node) {
		if n != nil && !isNilNode(n) {
			r = append(r, n)
		}
	}
	addStmts := func(ss []Stmt) {
		for _, s := range ss {
			add(s)
		}
	}
	addExprs := func(es []Expr) {
		for _, e := range es {
			add(e)
		}
	}
	addSpecs := func(specs []*VarSpec) {
		for _, sp := range specs {
			add(sp)
		}
	}

	switch n := node.(type) {
	case *File:
		addStmts(n.Stmts)
	case *VarDecl:
		addSpecs(n.Specs)
	case *VarSpec:
		add(n.Name)
		add(n.Type)
		add(n.Init)
		add(n.Options)
	case *FuncDecl:
		add(n.Name)
		addSpecs(n.Params)
		add(n.Result)
		add(n.Options)
		add(n.Body)
	case *IfStmt:
		add(n.Cond)
		add(n.Then)
		add(n.Else)
	case *LoopStmt:
		add(n.Body)
	case *WhileStmt:
		add(n.Cond)
		add(n.Body)
	case *ForStmt:
		add(n.Var)
		add(n.From)
		add(n.To)
		add(n.Body)
	case *BreakStmt, *ContinueStmt, *EmptyStmt, *ScopeLabel:
	case *ReturnStmt:
		add(n.Value)
	case *SwitchStmt:
		add(n.Tag)
		for _, c := range n.Cases {
			add(c)
		}
		add(n.Default)
	case *CaseClause:
		addExprs(n.Values)
		addStmts(n.Body)
	case *DefaultClause:
		addStmts(n.Body)
	case *ExprStmt:
		add(n.X)
	case *OptionsStmt:
		add(n.Options)
	case *UseDecl:
		add(n.Module)
		add(n.As)
	case *IncludeDecl:
		add(n.Kind)
		add(n.Path)
		add(n.Options)
	case *Block:
		addStmts(n.Stmts)
	case *Ident, *IntLit, *StringLit:
	case *ParenExpr:
		add(n.X)
	case *BinaryExpr:
		add(n.X)
		add(n.Y)
	case *AssignExpr:
		add(n.Lhs)
		add(n.Rhs)
	case *UnaryExpr:
		add(n.X)
	case *CastExpr:
		add(n.Type)
		add(n.X)
	case *CallExpr:
		add(n.Fun)
		addExprs(n.Args)
		add(n.Block)
	case *IndexExpr:
		add(n.X)
		add(n.Index)
	case *ArrayLit:
		addExprs(n.Elems)
	case *IncbinExpr:
		add(n.Path)
	case *LambdaExpr:
		add(n.Type)
		add(n.Body)
	case *NamedType:
		add(n.Name)
	case *ArrayType:
		add(n.Elem)
		add(n.Len)
	case *PointerType:
		add(n.Elem)
	case *FuncType:
		add(n.Result)
		for _, p := range n.Params {
			add(p)
		}
	case *Param:
		add(n.Name)
		add(n.Type)
	case *Options:
		for _, e := range n.Entries {
			add(e)
		}
	case *OptionEntry:
		add(n.Key)
		add(n.Value)
	}
	return r
}

// isNilNode は型付き nil ポインタをインターフェースに入れたものを検出する。
func isNilNode(n Node) bool {
	switch v := n.(type) {
	case *Block:
		return v == nil
	case *Options:
		return v == nil
	case *Ident:
		return v == nil
	case *StringLit:
		return v == nil
	case *DefaultClause:
		return v == nil
	case *IfStmt:
		return v == nil
	}
	return false
}
