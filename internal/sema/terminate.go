package sema

// 非 void 関数の「return 忘れ」の検査 (doc/archive/go_evolution_plan.md R4)。
//
// v1 のコンパイラは非 void 関数が return せずに終端へ到達しても何も言わず、実行時は rts が無いので
// 次の関数へ落ちて暴走する。Go と同じ「終端文」の規則で、本体の最後が必ず return で終わることを
// 構文的に確かめる (到達可能性の解析はしない)。

import (
	"github.com/haramako/fc/internal/syntax"
)

// terminates は文 s が「必ず return で抜ける (終端に落ちない)」かを返す。
//   - return
//   - ブロック: 最後の文が終端文
//   - if: else があり、then / else とも終端文
//   - loop / while (1) / 条件なし for: 中にそのループを抜ける break が無い
//   - switch: default があり、全 case と default の最後の文が終端文
//   - ラベル付き文: 中の文
func terminates(s syntax.Stmt) bool {
	switch s := s.(type) {
	case *syntax.ReturnStmt:
		return true
	case *syntax.Block:
		if len(s.Stmts) == 0 {
			return false
		}
		return terminates(s.Stmts[len(s.Stmts)-1])
	case *syntax.IfStmt:
		return s.Else != nil && terminates(s.Then) && terminates(s.Else)
	case *syntax.LoopStmt:
		return !hasBreakFor(s.Body, nil)
	case *syntax.WhileStmt:
		// while (1) は無限ループ
		return isTrueLiteral(s.Cond) && !hasBreakFor(s.Body, nil)
	case *syntax.ForStmt:
		return !s.IsV1() && (s.Cond == nil || isTrueLiteral(s.Cond)) && !hasBreakFor(s.Body, nil)
	case *syntax.SwitchStmt:
		if s.Default == nil {
			return false
		}
		for _, c := range s.Cases {
			if len(c.Body) == 0 || !terminates(c.Body[len(c.Body)-1]) {
				return false
			}
		}
		return len(s.Default.Body) > 0 && terminates(s.Default.Body[len(s.Default.Body)-1])
	case *syntax.LabeledStmt:
		switch inner := s.Stmt.(type) {
		case *syntax.LoopStmt:
			return !hasBreakFor(inner.Body, s.Label)
		case *syntax.WhileStmt:
			return isTrueLiteral(inner.Cond) && !hasBreakFor(inner.Body, s.Label)
		case *syntax.ForStmt:
			return !inner.IsV1() && (inner.Cond == nil || isTrueLiteral(inner.Cond)) && !hasBreakFor(inner.Body, s.Label)
		}
		return terminates(s.Stmt)
	}
	return false
}

// isTrueLiteral は 0 以外の整数リテラル (括弧付きも可) か。
func isTrueLiteral(e syntax.Expr) bool {
	for {
		switch x := e.(type) {
		case *syntax.ParenExpr:
			e = x.X
		case *syntax.IntLit:
			return x.Value != 0
		default:
			return false
		}
	}
}

// hasBreakFor は body の中に「このループを抜ける break」があるか
// (ラベルなしで間にループ/switch を挟まないもの、または label を指すもの)。
func hasBreakFor(body syntax.Stmt, label *syntax.Ident) bool {
	found := false
	var walk func(n syntax.Node, nested bool)
	walk = func(n syntax.Node, nested bool) {
		if found {
			return
		}
		switch n := n.(type) {
		case *syntax.BreakStmt:
			if n.Label == nil && !nested || n.Label != nil && label != nil && n.Label.Name == label.Name {
				found = true
			}
			return
		case *syntax.LoopStmt, *syntax.WhileStmt, *syntax.ForStmt:
			nested = true
		case *syntax.SwitchStmt:
			nested = true
		case *syntax.LambdaExpr:
			return
		}
		for _, c := range syntax.Children(n) {
			walk(c, nested)
		}
	}
	walk(body, false)
	return found
}
