package syntax

// 文の無い case の検査 (fc 3): fc の case は落ちない (fall through しない) ので、C の書き方で `case 2: case 3: r = 7;` と並べると
// x == 2 のときは何もしない。後ろに case / default が続く文の無い case を警告し、`case 2, 3:` / `fallthrough;` を案内する。
// 何もしない case は書くので、`:` から次の case までにコメントがあれば (`// 何もしない`) 警告しない (`break;` などの文も)。
// doc/roadmap.md の v3 (2026-09-26 決定)。

func (l *linter) emptyCases(f *File) {
	if f.Version < Version3 {
		return
	}
	Inspect(f, func(n Node) bool {
		s, ok := n.(*SwitchStmt)
		if !ok {
			return true
		}
		for i, c := range s.Cases {
			if len(c.Body) > 0 {
				continue
			}
			next := -1 // 次の case / default の位置
			if i+1 < len(s.Cases) {
				next = s.Cases[i+1].Case.Offset
			}
			if d := s.Default; d != nil && d.Default.Offset > c.Colon.Offset && (next < 0 || d.Default.Offset < next) {
				next = d.Default.Offset
			}
			if next < 0 {
				continue // 最後の case (落ちる先が無い)
			}
			commented := false
			for _, cm := range f.Comments {
				if cm.Pos.Offset > c.Colon.Offset && cm.Pos.Offset < next {
					commented = true
					break
				}
			}
			if !commented {
				l.warn(c.Case, "this case does nothing (fc does not fall through to the next case); write `case a, b:` or `fallthrough;`, or add a comment if it is intended")
			}
		}
		return true
	})
}
