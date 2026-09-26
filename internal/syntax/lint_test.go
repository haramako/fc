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
		// 属性: 知らないキー (近い名前を案内) と、その宣言では何もしないキー
		{"unknown attribute", "#fc 3\nvar io:u8 @(adress: 0x6000);\n", []string{"unknown attribute `adress` (did you mean `address`?)"}},
		{"unknown module attribute", "#fc 3\n@(mapperr: \"MMC3\");\n", []string{"unknown attribute `mapperr` (did you mean `mapper`?)"}},
		{"attribute without effect", "#fc 3\nvar z:u8 @(zeropage);\nconst T = [1] @(segment: \"R\");\nfunction f(a:u8 @(address: 1)):void @(address: 2) { var l:u8 @(address: 3); }\n",
			[]string{"`zeropage` has no effect on a global variable; it is ignored (use @(segment: \"ZEROPAGE\"))", "`segment` has no effect on a const",
				"`address` has no effect on a parameter", "`address` has no effect on a function", "`address` has no effect on a local variable"}},
		{"valid attributes", "#fc 3\n@(bank: -1, farcall);\nvar io:u8 @(address: 0x2000, volatile);\nconst T = [1] @(symbol: \"_t\");\nfunction f():void @(inline, segment: \"CODE\") { }\n", nil},
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
