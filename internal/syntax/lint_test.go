package syntax

import (
	"strings"
	"testing"
)

func TestLint(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want []string // 警告メッセージの部分文字列 (出現順)。空なら警告なし
	}{
		{"bitwise and comparison", "var x = a & b == 0;\n", []string{"`&` binds looser than `==`"}},
		{"parenthesized is fine", "var x = (a & b) == 0;\nvar y = a & (b == 0);\n", nil},
		{"or with comparison", "var x = a >= 2 | f();\n", []string{"`|` binds looser than `>=`"}},
		{"v2 has no break/continue traps", "#fc 2\nfunction f():void { var i:int; for (i = 0; i < 3; i++) { switch (i) { case 1: break; case 2: continue; } } }\n", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f, err := Parse([]byte(c.src), "t.fc")
			if err != nil {
				t.Fatal(err)
			}
			ws := Lint(f)
			if len(ws) != len(c.want) {
				t.Fatalf("warnings: got %d, want %d: %v", len(ws), len(c.want), ws)
			}
			for i, w := range ws {
				if !strings.Contains(w.Msg, c.want[i]) || !w.Pos.IsValid() {
					t.Errorf("warning %d: got %q at %v, want /%s/", i, w.Msg, w.Pos, c.want[i])
				}
			}
		})
	}
}
