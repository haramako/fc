package syntax

import (
	"strings"
	"testing"
)

func TestStorageAliasSyntax(t *testing.T) {
	src := []byte(`// typed view
public alias /* binding */ work:Work=shared.data;
var alias:uint8;
function f(alias:uint8):void {
 alias = 1;
 alias /* local */ local:Work=work;
 if(alias) alias other:uint8=shared.data;
}
`)
	f, err := Parse(src, "alias.fc")
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	Inspect(f, func(n Node) bool {
		if v, ok := n.(*VarDecl); ok && v.Alias {
			count++
		}
		return true
	})
	if count != 3 {
		t.Fatalf("aliases=%d", count)
	}
	// Use the same public formatting API as callers, and verify preservation of
	// contextual identifiers and comments as well as idempotence.
	out, err := Format(src, "alias.fc")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"public alias", "/* binding */", "/* local */", "var alias:uint8", "alias = 1;"} {
		if !strings.Contains(string(out), want) {
			t.Fatalf("missing %q:\n%s", want, out)
		}
	}
	again, err := Format(out, "alias.fc")
	if err != nil {
		t.Fatal(err)
	}
	if string(again) != string(out) {
		t.Fatal("format is not idempotent")
	}
}
