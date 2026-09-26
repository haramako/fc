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
//   - switch: default があり、全 case と default の最後の文が終端文 (fc 3 の fallthrough で終わる case は次の case が終端文なら)
//   - ラベル付き文: 中の文
//   - fc 3 の @if: 選ばれた側 (else が無く条件が偽なら終端文でない)。選ばれなかった側は見ない
//
// while / for の無限ループは条件が 0 以外の整数か true (`while (true)`)。
func (h *Hlc) terminates(s syntax.Stmt) bool {
	switch s := s.(type) {
	case *syntax.ReturnStmt:
		return true
	case *syntax.Block:
		if len(s.Stmts) == 0 {
			return false
		}
		return h.terminates(s.Stmts[len(s.Stmts)-1])
	case *syntax.IfStmt:
		return s.Else != nil && h.terminates(s.Then) && h.terminates(s.Else)
	case *syntax.StaticIfStmt:
		body := h.staticBranch(s)
		return len(body) > 0 && h.terminates(body[len(body)-1])
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
		// 後ろから: fallthrough で終わる case は、次の case (最後なら default) が終端するなら終端する
		next := len(s.Default.Body) > 0 && h.terminates(s.Default.Body[len(s.Default.Body)-1])
		if !next {
			return false
		}
		for i := len(s.Cases) - 1; i >= 0; i-- {
			body := s.Cases[i].Body
			switch {
			case len(body) == 0:
				return false
			case endsWithFallthrough(body):
				// next (次の case が終端するか) のまま
			case !h.terminates(body[len(body)-1]):
				return false
			}
		}
		return true
	case *syntax.LabeledStmt:
		switch inner := s.Stmt.(type) {
		case *syntax.LoopStmt:
			return !hasBreakFor(inner.Body, s.Label)
		case *syntax.WhileStmt:
			return isTrueLiteral(inner.Cond) && !hasBreakFor(inner.Body, s.Label)
		case *syntax.ForStmt:
			return !inner.IsV1() && (inner.Cond == nil || isTrueLiteral(inner.Cond)) && !hasBreakFor(inner.Body, s.Label)
		}
		return h.terminates(s.Stmt)
	}
	return false
}

// switchWithoutDefault は、s が default の無い switch で、全 case が終端文か (`missing return` で default を案内する。
// enum の全メンバーを並べても、範囲外の値 (`5 as Dir`、初期化していない値) で落ちうるので終端文にしない: 2026-09-27 決定)。
func (h *Hlc) switchWithoutDefault(s syntax.Stmt) bool {
	sw, ok := s.(*syntax.SwitchStmt)
	if !ok || sw.Default != nil || len(sw.Cases) == 0 {
		return false
	}
	for _, c := range sw.Cases {
		if len(c.Body) == 0 || !endsWithFallthrough(c.Body) && !h.terminates(c.Body[len(c.Body)-1]) {
			return false
		}
	}
	return true
}

// isTrueLiteral は 0 以外の整数リテラルか true (括弧付きも可) か。
func isTrueLiteral(e syntax.Expr) bool {
	for {
		switch x := e.(type) {
		case *syntax.ParenExpr:
			e = x.X
		case *syntax.IntLit:
			return x.Value != 0
		case *syntax.BoolLit:
			return x.Value
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
		case *syntax.LoopStmt, *syntax.WhileStmt, *syntax.ForStmt, *syntax.ForInStmt:
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
