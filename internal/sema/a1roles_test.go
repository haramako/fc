package sema

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// TestA1RolesComplete: cexpr.go の演算 (cop の定数) が全部 a1Roles にある (演算を足して A1 の役割を決め忘れると、実行時と
// 定数の畳み込みの片方だけが広げる・どちらも広げない、が黙って起きる。@min / @max / @clamp がそうだった: 2026-10-18)。
func TestA1RolesComplete(t *testing.T) {
	f, err := parser.ParseFile(token.NewFileSet(), "cexpr.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, d := range f.Decls {
		g, ok := d.(*ast.GenDecl)
		if !ok || g.Tok != token.CONST {
			continue
		}
		for _, sp := range g.Specs {
			vs := sp.(*ast.ValueSpec)
			if id, ok := vs.Type.(*ast.Ident); !ok || id.Name != "cop" {
				continue
			}
			for _, v := range vs.Values {
				lit := v.(*ast.BasicLit)
				op := cop(lit.Value[1 : len(lit.Value)-1])
				n++
				if a1Roles[op] == 0 {
					t.Errorf("演算 %s (%s) の A1 の役割が a1Roles (widen.go) に無い", vs.Names[0].Name, op)
				}
			}
		}
	}
	if n == 0 {
		t.Fatal("cexpr.go に cop の定数が見つからない")
	}
	if len(a1Roles) != n {
		t.Errorf("a1Roles に %d 個、cexpr.go に %d 個 (消した演算が残っている)", len(a1Roles), n)
	}
}
