package sema

import (
	"regexp"
	"strings"
	"testing"
)

// TestBreakV2: break / continue とラベル (doc/v2_grammar.md §3.7)。
// ラベルなし break は最も内側のループまたは switch、continue はループ。`break L;` / `continue L;`
func TestBreakV2(t *testing.T) {
	body := func(version int, stmts string) map[string]string {
		pragma := ""
		if version == 2 {
			pragma = "#fc 2\n"
		}
		return map[string]string{"t.fc": pragma + "function main():void {\nvar x:int;\nvar y:int;\n" + stmts + "\n}\n"}
	}

	t.Run("v2: break in switch leaves the switch", func(t *testing.T) {
		ir := mustCompileFiles(t, body(2, "loop { switch (x) { case 1: break; } y = 1; }"), "t.fc")
		if got, loopEnd := firstJump(ir), lastEndLabel(ir); got == loopEnd || !strings.HasPrefix(got, "@end_") {
			t.Errorf("v2 の break は switch の end へ飛ぶべき (ループの end は %s): %s\n%s", loopEnd, got, ir)
		}
	})
	t.Run("v2: labeled break leaves the loop from a switch", func(t *testing.T) {
		ir := mustCompileFiles(t, body(2, "outer: loop { switch (x) { case 1: break outer; } y = 1; }"), "t.fc")
		if got, want := firstJump(ir), lastEndLabel(ir); got != want {
			t.Errorf("break outer はループの end (%s) へ飛ぶべき: %s\n%s", want, got, ir)
		}
	})
	t.Run("v2: labeled while / for / continue", func(t *testing.T) {
		for _, src := range []string{
			"a: while (x) { b: while (y) { continue a; } }",
			"a: for (x = 0; x < 3; x++) { b: for (y = 0; y < 3; y++) { break a; } }",
			"a: loop { b: switch (x) { case 1: break b; } break; }",
			"a: loop { switch (x) { case 1: continue a; } }",
		} {
			if err := compileFiles(t, body(2, src), "t.fc"); err != nil {
				t.Errorf("%s: %v", src, err)
			}
		}
	})
	t.Run("v2: errors", func(t *testing.T) {
		cases := []struct{ src, want string }{
			{"loop { break nothere; }", "label nothere not found"},
			{"s: switch (x) { case 1: continue s; }", "cannot continue a switch"},
			{"switch (x) { case 1: continue; }", "cannot continue without loop"},
			{"break;", "cannot break without loop"},
			{"a: loop { a: loop { break; } }", "label a is already in use"},
			{"a: x = 1;", "a label must be placed on loop / while / for / switch"},
			{"loop() { break; }", "`loop()` is written `loop` in fc 2"},
		}
		for _, c := range cases {
			err := compileFiles(t, body(2, c.src), "t.fc")
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("%q: got %v, want /%s/", c.src, err, c.want)
			}
		}
	})
}

var jumpRe = regexp.MustCompile(`\(:jump "(@end_\d+)"\)`)
var endLabelRe = regexp.MustCompile(`\(:label "(@end_\d+)"\)`)

// firstJump は IR ダンプ中の最初の `(:jump @end_N)` の飛び先 (break の飛び先)。
func firstJump(ir string) string {
	if m := jumpRe.FindStringSubmatch(ir); m != nil {
		return m[1]
	}
	return "<no jump>"
}

// lastEndLabel は最後に定義される `@end_N` ラベル (最も外側のループの end)。
func lastEndLabel(ir string) string {
	ms := endLabelRe.FindAllStringSubmatch(ir, -1)
	if len(ms) == 0 {
		return "<no end label>"
	}
	return ms[len(ms)-1][1]
}
