package project

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestMacroServers: fc.toml の [macro_server.*] (文字列の配列の値・名前の検査)。
func TestMacroServers(t *testing.T) {
	cfg, err := parseConfig("/p/fc.toml", []byte(`[macro_server.tools]
command = ["go", "run", "./tools/x"]  # コメント
macros = ["sin_table", "map_2"]
inputs = ["tools/x/*.go"]
`))
	if err != nil {
		t.Fatal(err)
	}
	ss, err := cfg.MacroServers()
	if err != nil || len(ss) != 1 {
		t.Fatalf("%v %v", ss, err)
	}
	s := ss[0]
	if s.Name != "tools" || strings.Join(s.Command, "|") != "go|run|./tools/x" || strings.Join(s.Macros, "|") != "sin_table|map_2" ||
		strings.Join(s.Inputs, "|") != "tools/x/*.go" || s.Dir != filepath.FromSlash("/p") {
		t.Errorf("got %+v", s)
	}
	for _, c := range []struct{ body, msg string }{
		{"command = \"go\"\nmacros = [\"a\"]", "command must be"},
		{"command = [\"go\"]\nmacros = []", "macros must be"},
		{"command = [\"go\"]\nmacros = [\"@a\"]", "bad macro name"},
		{"command = [\"go\"]\nmacros = [\"a\"]\nfoo = 1", "unknown key foo"},
	} {
		cfg, err := parseConfig("/p/fc.toml", []byte("[macro_server.x]\n"+c.body+"\n"))
		if err == nil {
			_, err = cfg.MacroServers()
		}
		if err == nil || !strings.Contains(err.Error(), c.msg) {
			t.Errorf("%q: got %v, want /%s/", c.body, err, c.msg)
		}
	}
	for v, want := range map[string]string{`[]`: "", `["a"]`: "a", `[ "a" , "b\"c" ]`: `a|b"c`} {
		got, err := StringList(v)
		if err != nil || strings.Join(got, "|") != want {
			t.Errorf("StringList(%s) = %q, %v", v, got, err)
		}
	}
}
