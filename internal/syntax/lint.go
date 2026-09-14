package syntax

// 構文だけで分かる注意点の検査 (fcc check / ビルド時の警告)。意味解析は要らないものだけをここに置く。

import "fmt"

// Warning は構文検査の警告。
type Warning struct {
	Pos Pos
	Msg string
}

// Lint は f を検査して警告を返す (出現順)。
func Lint(f *File) []Warning {
	l := &linter{version: f.Version}
	for _, s := range f.Stmts {
		l.stmt(s, nil)
	}
	Inspect(f, func(n Node) bool {
		if e, ok := n.(*BinaryExpr); ok {
			l.bitwiseWithComparison(e)
		}
		return true
	})
	// 位置順に
	for i := 1; i < len(l.warnings); i++ {
		for j := i; j > 0 && l.warnings[j].Pos.Offset < l.warnings[j-1].Pos.Offset; j-- {
			l.warnings[j], l.warnings[j-1] = l.warnings[j-1], l.warnings[j]
		}
	}
	return l.warnings
}

type linter struct {
	version  int
	warnings []Warning
}

func (l *linter) warn(pos Pos, format string, args ...any) {
	l.warnings = append(l.warnings, Warning{Pos: pos, Msg: fmt.Sprintf(format, args...)})
}

func isComparison(k Kind) bool {
	switch k {
	case EqEq, Neq, Lt, Gt, Leq, Geq:
		return true
	}
	return false
}

// bitwiseWithComparison: `a & b == c` は C と同じく `a & (b == c)` に読まれる。
// 意図が `(a & b) == c` であることが多いので、括弧の無い組み合わせに警告する。
func (l *linter) bitwiseWithComparison(e *BinaryExpr) {
	switch e.Op {
	case Amp, Pipe, Caret:
	default:
		return
	}
	for _, side := range []Expr{e.X, e.Y} {
		if b, ok := side.(*BinaryExpr); ok && isComparison(b.Op) {
			l.warn(e.OpPos, "`%s` binds looser than `%s`: `a %s b %s c` means `a %s (b %s c)`; add parentheses", e.Op, b.Op, e.Op, b.Op, e.Op, b.Op)
		}
	}
}

// stmt は v1 の break / continue の罠を検査する (v2 では意味が直っている)。
//   - v1 の for の中の continue はインクリメントを飛ばす (無限ループになりうる)
//   - v1 の switch の中の break は switch ではなく外側のループを抜ける
//
// encl は囲んでいる文の種類 ("for" / "loop" / "switch")、内側が末尾。
func (l *linter) stmt(s Stmt, encl []string) {
	inner := func() string {
		if len(encl) == 0 {
			return ""
		}
		return encl[len(encl)-1]
	}
	switch s := s.(type) {
	case *FuncDecl:
		if s.Body != nil {
			l.stmt(s.Body, nil)
		}
	case *Block:
		for _, c := range s.Stmts {
			l.stmt(c, encl)
		}
	case *IfStmt:
		l.stmt(s.Then, encl)
		if s.Else != nil {
			l.stmt(s.Else, encl)
		}
	case *LabeledStmt:
		l.stmt(s.Stmt, encl)
	case *LoopStmt:
		l.stmt(s.Body, append(encl, "loop"))
	case *WhileStmt:
		l.stmt(s.Body, append(encl, "loop"))
	case *ForStmt:
		kind := "loop"
		if s.IsV1() {
			kind = "for"
		}
		l.stmt(s.Body, append(encl, kind))
	case *SwitchStmt:
		for _, c := range s.Cases {
			for _, b := range c.Body {
				l.stmt(b, append(encl, "switch"))
			}
		}
		if s.Default != nil {
			for _, b := range s.Default.Body {
				l.stmt(b, append(encl, "switch"))
			}
		}
	case *ContinueStmt:
		if l.version < Version2 && s.Label == nil {
			// 最も内側のループが v1 の for なら、インクリメントを飛ばす
			for i := len(encl) - 1; i >= 0; i-- {
				if encl[i] == "switch" {
					continue
				}
				if encl[i] == "for" {
					l.warn(s.Keyword, "in fc 1, `continue` inside `for (i, from, to)` skips the increment (infinite loop); use fc 2 (`fcc migrate`)")
				}
				break
			}
		}
	case *BreakStmt:
		if l.version < Version2 && s.Label == nil && inner() == "switch" {
			l.warn(s.Keyword, "in fc 1, `break` inside `switch` leaves the enclosing loop, not the switch; use fc 2 (`fcc migrate`)")
		}
	}
}
