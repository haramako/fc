package sema

import (
	"strings"
	"testing"
)

// TestForV2: C 型 for と ++ / -- (doc/v2_grammar.md §3.10)。
func TestForV2(t *testing.T) {
	v2 := func(body string) map[string]string {
		return map[string]string{"t.fc": "#fc 2\nfunction main():void {\nvar x:int;\nvar y:int;\n" + body + "\n}\n"}
	}
	v1 := func(body string) map[string]string {
		return map[string]string{"t.fc": "function main():void {\nvar x:int;\nvar y:int;\n" + body + "\n}\n"}
	}
	// ops は IR ダンプの ops 部分だけ (var の番号やラベル名は同じ形になるはず)
	ops := func(ir string) string {
		i := strings.Index(ir, "(ops")
		if i < 0 {
			return ir
		}
		return ir[i:]
	}

	t.Run("same code as v1 for", func(t *testing.T) {
		old := mustCompileFiles(t, v1("for (x, 0, 10) { y = y + x; }"), "t.fc")
		cfor := mustCompileFiles(t, v2("for (x = 0; x < 10; x++) { y = y + x; }"), "t.fc")
		if ops(old) != ops(cfor) {
			t.Errorf("v1 の for と C 型 for の IR が違う\n--- v1\n%s--- v2\n%s", ops(old), ops(cfor))
		}
	})
	t.Run("continue goes to step", func(t *testing.T) {
		ir := mustCompileFiles(t, v2("for (x = 0; x < 10; x++) { if (x == 2) { continue; } y = y + 1; }"), "t.fc")
		// continue の jump 先は @step_N で、その直後に x = x + 1 が来る
		if !strings.Contains(ir, `(:jump "@step_`) || !strings.Contains(ir, `(:label "@step_`) {
			t.Errorf("continue が step に飛んでいない:\n%s", ops(ir))
		}
	})
	t.Run("no step label without continue", func(t *testing.T) {
		ir := mustCompileFiles(t, v2("for (x = 0; x < 10; x++) { y = y + 1; }"), "t.fc")
		if strings.Contains(ir, "@step_") {
			t.Errorf("continue が無いのに step ラベルがある:\n%s", ops(ir))
		}
	})
	t.Run("labeled continue targets the outer for", func(t *testing.T) {
		ir := mustCompileFiles(t, v2("outer: for (x = 0; x < 3; x++) { for (y = 0; y < 3; y++) { continue outer; } }"), "t.fc")
		if strings.Count(ir, `(:label "@step_`) != 1 {
			t.Errorf("外側の for だけに step ラベルが要る:\n%s", ops(ir))
		}
	})
	t.Run("var in init is scoped to the for", func(t *testing.T) {
		if err := compileFiles(t, v2("for (var i:uint8 = 0; i < 3; i++) { y = i; }\nfor (var i = 5; i > 0; i--) { y = i; }"), "t.fc"); err != nil {
			t.Fatal(err)
		}
		err := compileFiles(t, v2("for (var i = 0; i < 3; i++) { }\ny = i;"), "t.fc")
		if err == nil || !strings.Contains(err.Error(), "i not found") {
			t.Errorf("for の変数が外に漏れている: %v", err)
		}
	})
	t.Run("parts can be omitted", func(t *testing.T) {
		for _, src := range []string{
			"for (;;) { break; }",
			"for (x = 0;; x++) { if (x == 3) { break; } }",
			"for (; x < 3;) { x++; }",
			"x = 0; for (; x < 3; ++x) { }",
		} {
			if err := compileFiles(t, v2(src), "t.fc"); err != nil {
				t.Errorf("%s: %v", src, err)
			}
		}
	})
	t.Run("incdec statements", func(t *testing.T) {
		a := mustCompileFiles(t, v2("x++; --y; x--; ++y;"), "t.fc")
		b := mustCompileFiles(t, v2("x = x + 1; y = y - 1; x = x - 1; y = y + 1;"), "t.fc")
		if ops(a) != ops(b) {
			t.Errorf("++/-- の IR が x = x + 1 と違う\n--- incdec\n%s--- assign\n%s", ops(a), ops(b))
		}
	})
	t.Run("incdec is not an expression", func(t *testing.T) {
		err := compileFiles(t, v2("y = x++;"), "t.fc")
		if err == nil || !strings.Contains(err.Error(), "cannot be used inside an expression") {
			t.Errorf("x++ が式として通ってしまう: %v", err)
		}
	})
	t.Run("v2 rejects v1 for", func(t *testing.T) {
		err := compileFiles(t, v2("for (x, 0, 3) { }"), "t.fc")
		if err == nil || !strings.Contains(err.Error(), "is written `for (i = from; i < to; i++)` in fc 2") {
			t.Errorf("got %v", err)
		}
	})
	t.Run("v1 rejects v2 syntax", func(t *testing.T) {
		for _, src := range []string{"for (x = 0; x < 3; x++) { }", "x++;"} {
			err := compileFiles(t, v1(src), "t.fc")
			if err == nil || !strings.Contains(err.Error(), "fc 2") {
				t.Errorf("%q: got %v", src, err)
			}
		}
	})
}
