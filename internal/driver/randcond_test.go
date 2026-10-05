package driver

// fc 4 の条件式と do-while を fuzz に混ぜる (runRandomV4)。生成器は fc 2 のプログラムを作るので、fc 4 にした後のソースを、意味を
// 変えない形で書き換える:
//   - 文の代入 `X op= E;` → `rmc_ += 1; X op= (rmc_ & 1) != 0 ? E : E;` (E は 1 回だけ評価される。枝の型は同じなので条件式の型は
//     E の型。rmc_ はモジュールに足す変数で、両方の枝を交互に通す)
//   - 関数の中の変数の初期値 `var x:T = E;` → `rmc_ += 1; var x:T = (rmc_ & 1) != 0 ? E : E;` (初期化の枝ごとの書き込み:
//     sema の retargetCond。定数・リテラルの初期値は型が変わりうるので包まない)
//   - if の条件 C → `(rmc_ & 2) != 0 ? (C) : (C)` (条件の文脈の条件式)
//   - ラベルの無い `while (C) S` → `if (C) do S while (C);` (C を評価する回数と順は同じ。continue はどちらも条件の判定へ)
// 書き換える所は種から決める乱数で選ぶ (生成器の乱数の並びは変えない)。

import (
	"math/rand"
	"sort"
	"strings"

	"github.com/haramako/fc/internal/syntax"
)

// rpCondRewrite は files の fc 4 のソースを書き換えた内容を返す (解析できないファイルはそのまま)。
func rpCondRewrite(files map[string]string, seed int64) map[string]string {
	r := rand.New(rand.NewSource(seed*104729 + 17))
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	out := map[string]string{}
	for _, name := range names {
		src := files[name]
		out[name] = src
		if !strings.HasSuffix(name, ".fc") || !strings.HasPrefix(src, "#fc 4") {
			continue
		}
		f, err := syntax.Parse([]byte(src), name)
		if err != nil {
			continue
		}
		type edit struct {
			start, end int
			text       string
		}
		var edits []edit
		text := func(n syntax.Node) string { return src[n.Pos().Offset:n.End().Offset] }
		used := false
		labeled := map[*syntax.WhileStmt]bool{}
		rv := rand.New(rand.NewSource(seed*7919 + 3)) // 変数の初期値の書き換え (ほかの書き換えの乱数の並びを変えない)
		syntax.Inspect(f, func(n syntax.Node) bool {
			switch n := n.(type) {
			case *syntax.LabeledStmt:
				if w, ok := n.Stmt.(*syntax.WhileStmt); ok {
					labeled[w] = true
				}
			case *syntax.Block:
				for _, st := range n.Stmts {
					if vd, ok := st.(*syntax.VarDecl); ok && !vd.Const && !vd.Alias {
						for _, sp := range vd.Specs {
							if sp.Init == nil || rv.Intn(3) != 0 || isConstExpr(sp.Init) {
								continue
							}
							switch sp.Init.(type) {
							case *syntax.StructLit, *syntax.ArrayLit, *syntax.StringLit:
								continue
							}
							init := text(sp.Init)
							edits = append(edits, edit{vd.Pos().Offset, vd.Pos().Offset, "rmc_ += 1; "},
								edit{sp.Init.Pos().Offset, sp.Init.End().Offset, "(rmc_ & 1) != 0 ? " + init + " : " + init})
							used = true
						}
						continue
					}
					es, ok := st.(*syntax.ExprStmt)
					if !ok || r.Intn(6) != 0 {
						continue
					}
					as, ok := es.X.(*syntax.AssignExpr)
					if !ok {
						continue
					}
					if _, anon := as.Rhs.(*syntax.StructLit); anon {
						continue
					}
					if as.Op.IsCompoundAssign() && isConstExpr(as.Rhs) {
						continue // `x ^= -41262` は演算で定数を相手の型に切り詰めるが、`?:` の枝の定数は切り詰めない (型に入らなければエラー)
					}
					rhs := text(as.Rhs)
					edits = append(edits, edit{es.Pos().Offset, es.Pos().Offset, "rmc_ += 1; "},
						edit{as.Rhs.Pos().Offset, as.Rhs.End().Offset, "(rmc_ & 1) != 0 ? " + rhs + " : " + rhs})
					used = true
				}
			case *syntax.IfStmt:
				if r.Intn(4) == 0 {
					cond := text(n.Cond)
					edits = append(edits, edit{n.Cond.Pos().Offset, n.Cond.End().Offset, "(rmc_ & 2) != 0 ? (" + cond + ") : (" + cond + ")"})
					used = true
				}
			case *syntax.WhileStmt:
				if !labeled[n] && r.Intn(2) == 0 {
					cond := text(n.Cond)
					edits = append(edits, edit{n.While.Offset, n.Rparen.Offset + 1, "if (" + cond + ") do"},
						edit{n.Body.End().Offset, n.Body.End().Offset, " while (" + cond + ");"})
				}
			}
			return true
		})
		if len(edits) == 0 {
			continue
		}
		sort.SliceStable(edits, func(i, j int) bool { return edits[i].start < edits[j].start })
		var b strings.Builder
		at := 0
		for _, e := range edits {
			if e.start < at {
				continue // 重なる (起きないはず)
			}
			b.WriteString(src[at:e.start])
			b.WriteString(e.text)
			at = e.end
		}
		b.WriteString(src[at:])
		s := b.String()
		if used {
			nl := strings.IndexByte(s, '\n')
			s = s[:nl+1] + "var rmc_:u8;\n" + s[nl+1:]
		}
		out[name] = s
	}
	return out
}

// isConstExpr は e が名前・呼び出しを含まない式 (リテラルとその演算。`((37435 << 7) >> 2)` など) か。
func isConstExpr(e syntax.Expr) bool {
	c := true
	syntax.Inspect(e, func(n syntax.Node) bool {
		switch n.(type) {
		case *syntax.Ident, *syntax.CallExpr:
			c = false
		}
		return c
	})
	return c
}
