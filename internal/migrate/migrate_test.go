package migrate

import (
	"strings"
	"testing"

	"github.com/haramako/fc/internal/syntax"
)

// TestMigratePragma: `#fc 2` の行は `#fc 3` に、プラグマの無いソースには先頭に足す。fc 3 のソースはそのまま。
// コメントと書式は残る。
func TestMigratePragma(t *testing.T) {
	cases := []struct{ in, want string }{
		{"#fc 2\n// c\nvar a:int;  // x\n", "#fc 3\n// c\nvar a:int;  // x\n"},
		{"var a:int;\n", "#fc 3\nvar a:int;\n"},
		{"#fc 3\nvar a:int;\n", "#fc 3\nvar a:int;\n"},
		{"", "#fc 3\n"},
	}
	for _, c := range cases {
		got, err := Migrate([]byte(c.in), "t.fc")
		if err != nil {
			t.Errorf("%q: %v", c.in, err)
			continue
		}
		if string(got) != c.want {
			t.Errorf("%q: got %q want %q", c.in, got, c.want)
		}
	}
	if _, err := Migrate([]byte("var a:int\n"), "t.fc"); err == nil || !strings.Contains(err.Error(), "parse error") {
		t.Errorf("fc 2 として解析できないソース: %v", err)
	}
}

// TestMigrateRule: 規則は Edit を足し、重なる Edit はエラー。
func TestMigrateRule(t *testing.T) {
	saved := Rules
	defer func() { Rules = saved }()
	Rules = []Rule{{Name: "rename", Apply: func(c *Ctx) {
		for _, tk := range c.Tokens {
			if tk.Kind == syntax.Identifier && tk.Text == "a" {
				c.ReplaceToken(tk, "b")
			}
		}
	}}}
	got, err := Migrate([]byte("var a:int;\nfunction f():void { a = 1; }\n"), "t.fc")
	if err != nil || string(got) != "#fc 3\nvar b:int;\nfunction f():void { b = 1; }\n" {
		t.Errorf("got %q, %v", got, err)
	}
	Rules = append(Rules, Rule{Name: "overlap", Apply: func(c *Ctx) { c.Replace(4, 6, "x") }})
	if _, err := Migrate([]byte("var a:int;\n"), "t.fc"); err == nil || !strings.Contains(err.Error(), "overlapping") {
		t.Errorf("重なる Edit: %v", err)
	}
}
