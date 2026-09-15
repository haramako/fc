package sema

import (
	"strings"
	"testing"
)

// TestMissingReturn: 非 void 関数は必ず return で終わる (終端文の規則。doc/archive/go_evolution_plan.md R4)。
func TestMissingReturn(t *testing.T) {
	wrap := func(body string) map[string]string {
		return map[string]string{"t.fc": "#fc 2\nvar x:int;\n" + body + "\nfunction main():void {}\n"}
	}
	ok := []string{
		"function f():int { return 1; }",
		"function f():int { if (x) { return 1; } else { return 2; } }",
		"function f():int { loop { if (x) { return 1; } } }",
		"function f():int { while (1) { if (x) { return 1; } } }",
		"function f():int { for (;;) { return 1; } }",
		"function f():int { switch (x) { case 1: return 1; case 2: x = 0; return 2; default: return 0; } }",
		"function f():int { l: loop { loop { break; } if (x) { return 1; } } }",
		"function f():int { loop { switch (x) { case 1: break; } if (x) { return 1; } } }",
		"function f():void { }",
		"function f():void { if (x) { return; } }",
	}
	bad := []string{
		"function f():int { }",
		"function f():int { x = 1; }",
		"function f():int { if (x) { return 1; } }",
		"function f():int { loop { if (x) { break; } } }",
		"function f():int { while (x) { return 1; } }",
		"function f():int { switch (x) { case 1: return 1; } }",
		"function f():int { switch (x) { case 1: return 1; default: x = 0; } }",
		"function f():int { l: loop { loop { break l; } } }",
	}
	for _, src := range ok {
		if err := compileFiles(t, wrap(src), "t.fc"); err != nil {
			t.Errorf("%s: %v", src, err)
		}
	}
	for _, src := range bad {
		err := compileFiles(t, wrap(src), "t.fc")
		if err == nil || !strings.Contains(err.Error(), "missing return") {
			t.Errorf("%s: got %v, want missing return", src, err)
		}
	}
	// v1: switch の中の break はループを抜けるので、loop { switch { break } } は終端文にならない
	err := compileFiles(t, map[string]string{"t.fc": "var x:int;\nfunction f():int { loop() { switch (x) { case 1: break; } } }\nfunction main():void {}\n"}, "t.fc")
	if err == nil || !strings.Contains(err.Error(), "missing return") {
		t.Errorf("v1 switch break: got %v", err)
	}
}
