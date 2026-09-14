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
		{"v1 continue in for", "function f():void { var i:int; for (i, 0, 3) { continue; } }\n", []string{"skips the increment"}},
		{"v1 continue in loop inside for is fine", "function f():void { var i:int; for (i, 0, 3) { loop() { continue; } } }\n", nil},
		{"v1 continue in switch inside for", "function f():void { var i:int; for (i, 0, 3) { switch (i) { case 1: continue; } } }\n", []string{"skips the increment"}},
		{"v1 break in switch", "function f():void { loop() { switch (x) { case 1: break; } } }\n", []string{"leaves the enclosing loop"}},
		{"v1 break in loop inside switch is fine", "function f():void { switch (x) { case 1: loop() { break; } } }\n", nil},
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
