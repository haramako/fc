package syntax

import (
	"bytes"
	"strings"
	"testing"
)

func TestPlacementBlockFormatAndPositions(t *testing.T) {
	src := []byte("// storage\nblock /* group */ { public var a:int; // member\nblock {var b:int;} options(bss:\"INNER\");\n} /* placement */ options(bss:\"OUTER\"); // end\nblock {} options(bss:\"EMPTY\");\n")
	formatted, err := Format(src, "groups.fc")
	if err != nil {
		t.Fatal(err)
	}
	again, err := Format(formatted, "groups.fc")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(formatted, again) {
		t.Fatalf("format is not stable:\n%s\n%s", formatted, again)
	}
	for _, comment := range []string{"// storage", "/* group */", "// member", "/* placement */", "// end"} {
		if strings.Count(string(formatted), comment) != 1 {
			t.Errorf("lost/duplicated comment %q:\n%s", comment, formatted)
		}
	}
	file, err := Parse(formatted, "groups.fc")
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	Inspect(file, func(n Node) bool {
		if _, ok := n.(*PlacementBlock); ok {
			count++
		}
		for _, child := range Children(n) {
			if child.Pos().Offset < n.Pos().Offset || child.End().Offset > n.End().Offset {
				t.Errorf("%T outside %T", child, n)
			}
		}
		return true
	})
	if count != 3 {
		t.Fatalf("placement groups=%d, want 3", count)
	}
}

func TestBlockIsContextual(t *testing.T) {
	for _, src := range []string{
		`var block:int; function f():void { block=1; }`,
		`function block():void {} function f():void {block();}`,
		`struct block {x:int;} const b:block=block{x:1} options(symbol:"_b");`,
		`struct block {} const b:block=block{} options(symbol:"_b");`,
		`struct block {x:int;} function f():void {var b:block=block{x:1}; b=block{2}; block{};}`,
	} {
		if _, err := Format([]byte(src), "legacy.fc"); err != nil {
			t.Errorf("%s: %v", src, err)
		}
	}
}
