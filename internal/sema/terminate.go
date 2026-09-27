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
		return h.switchTerminates(s, nil)
	case *syntax.LabeledStmt:
		switch inner := s.Stmt.(type) {
		case *syntax.LoopStmt:
			return !hasBreakFor(inner.Body, s.Label)
		case *syntax.WhileStmt:
			return isTrueLiteral(inner.Cond) && !hasBreakFor(inner.Body, s.Label)
		case *syntax.ForStmt:
			return !inner.IsV1() && (inner.Cond == nil || isTrueLiteral(inner.Cond)) && !hasBreakFor(inner.Body, s.Label)
		case *syntax.SwitchStmt:
			return h.switchTerminates(inner, s.Label)
		}
		return h.terminates(s.Stmt)
	}
	return false
}

// switchTerminates は switch が終端文か: default があり、全 case と default の最後の文が終端文 (fallthrough で終わる case は
// 次の case が終端するなら)。case の中にこの switch を抜ける break (ラベル無しか label) や、外のループ・ラベルへの
// break / continue があれば終端文でない (`case 1: if (c) { break; } return 1;` が落ちる)。
func (h *Hlc) switchTerminates(sw *syntax.SwitchStmt, label *syntax.Ident) bool {
	for _, c := range sw.Cases {
		if hasBreakForSwitch(c.Body, label) {
			return false
		}
	}
	if sw.Default != nil && hasBreakForSwitch(sw.Default.Body, label) {
		return false
	}
	if sw.Default == nil {
		return false
	}
	// 後ろから: fallthrough で終わる case は、次の case (最後なら default) が終端するなら終端する
	next := len(sw.Default.Body) > 0 && h.terminates(sw.Default.Body[len(sw.Default.Body)-1])
	if !next {
		return false
	}
	for i := len(sw.Cases) - 1; i >= 0; i-- {
		body := sw.Cases[i].Body
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

// hasBreakFor は body (ループの本体) の中に「このループを抜ける」文があるか: このループを指す break (ラベル無しで間に
// ループ / switch を挟まないもの、または label を指すもの) と、body の外のラベルへの break / continue
// (`L: switch (x) { case 1: loop { break L; } … }` の loop は L へ抜ける。見ていなくて return 忘れを見逃した)。
func hasBreakFor(body syntax.Stmt, label *syntax.Ident) bool {
	return escapes(body, label, false)
}

// hasBreakForSwitch は switch の case の本体 stmts の中に、switch を抜ける文があるか (hasBreakFor と同じ。加えて、ループを
// 挟まないラベル無しの continue は外のループへ抜ける)。
func hasBreakForSwitch(stmts []syntax.Stmt, label *syntax.Ident) bool {
	return escapes(&syntax.Block{Stmts: stmts}, label, true)
}

func escapes(body syntax.Stmt, label *syntax.Ident, isSwitch bool) bool {
	inner := map[string]bool{} // body の中で付けたラベル (そこへの break / continue は body の中に留まる)
	var collect func(n syntax.Node)
	collect = func(n syntax.Node) {
		switch n := n.(type) {
		case *syntax.LabeledStmt:
			inner[n.Label.Name] = true
		case *syntax.LambdaExpr:
			return
		}
		for _, c := range syntax.Children(n) {
			collect(c)
		}
	}
	collect(body)
	outside := func(l *syntax.Ident) bool {
		return (label != nil && l.Name == label.Name) || !inner[l.Name]
	}
	found := false
	var walk func(n syntax.Node, inLoop, inBreakable bool)
	walk = func(n syntax.Node, inLoop, inBreakable bool) {
		if found {
			return
		}
		switch n := n.(type) {
		case *syntax.BreakStmt:
			if n.Label == nil && !inBreakable || n.Label != nil && outside(n.Label) {
				found = true
			}
			return
		case *syntax.ContinueStmt:
			switch {
			case n.Label == nil:
				found = isSwitch && !inLoop // switch の中の continue は外のループへ
			case label != nil && n.Label.Name == label.Name && !isSwitch:
				// このループの次の繰り返し (抜けない)
			case !inner[n.Label.Name]:
				found = true
			}
			return
		case *syntax.LoopStmt, *syntax.WhileStmt, *syntax.ForStmt, *syntax.ForInStmt:
			inLoop, inBreakable = true, true
		case *syntax.SwitchStmt:
			inBreakable = true
		case *syntax.LambdaExpr:
			return
		}
		for _, c := range syntax.Children(n) {
			walk(c, inLoop, inBreakable)
		}
	}
	walk(body, false, false)
	return found
}
