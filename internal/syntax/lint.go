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
	l := &linter{}
	Inspect(f, func(n Node) bool {
		if e, ok := n.(*BinaryExpr); ok {
			l.bitwiseWithComparison(e)
		}
		return true
	})
	l.attributes(f)
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
