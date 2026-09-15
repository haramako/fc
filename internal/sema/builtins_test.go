package sema

import (
	"fmt"
	"strings"
	"testing"
)

// TestBuiltins: include("*.rb") なしで使える組み込み (doc/v2_grammar.md §3.5)。
func TestBuiltins(t *testing.T) {
	t.Run("printf without include (v2)", func(t *testing.T) {
		ir := mustCompileFiles(t, map[string]string{
			"t.fc": "#fc 2\nuse * from stdio;\nfunction main():void { printf(\"a\", 1); }\n",
		}, "t.fc")
		if !strings.Contains(ir, "_stdio_print ") || !strings.Contains(ir, "_stdio_print_int16") {
			t.Errorf("printf が print / print_int16 に展開されていない:\n%s", ir)
		}
	})
	t.Run("printf requires stdio", func(t *testing.T) {
		err := compileFiles(t, map[string]string{"t.fc": "#fc 2\nfunction main():void { printf(\"a\"); }\n"}, "t.fc")
		if err == nil || !strings.Contains(err.Error(), "printf requires the stdio module") {
			t.Errorf("got %v", err)
		}
	})
	t.Run("include rb is an error in v2", func(t *testing.T) {
		err := compileFiles(t, map[string]string{"t.fc": "#fc 2\nuse * from stdio;\ninclude(\"stdio.rb\");\nfunction main():void {}\n"}, "t.fc")
		if err == nil || !strings.Contains(err.Error(), "macros are not supported in fc 2") {
			t.Errorf("got %v", err)
		}
	})
	t.Run("include rb is ignored in v1", func(t *testing.T) {
		if err := compileFiles(t, map[string]string{"t.fc": "use * from stdio;\ninclude(\"stdio.rb\");\nfunction main():void { printf(\"a\"); }\n"}, "t.fc"); err != nil {
			t.Error(err)
		}
	})
	t.Run("textmap", func(t *testing.T) {
		// 表は登録順にコード 0, 1, 2, ...。表にない文字は末尾に追加される
		ir := mustCompileFiles(t, map[string]string{
			"tbl.txt": "あいう",
			"t.fc":    "#fc 2\nuse * from stdio;\npublic const T = textmap(\"tbl.txt\");\nfunction main():void { print(T(\"うあい\")); print(T(\"え\")); }\n",
		}, "t.fc")
		if !strings.Contains(ir, codes(2, 0, 1, 0)) || !strings.Contains(ir, codes(3, 0)) {
			t.Errorf("textmap の変換結果が違う:\n%s", ir)
		}
	})
	t.Run("textmap is usable from another module via public", func(t *testing.T) {
		ir := mustCompileFiles(t, map[string]string{
			"tbl.txt": "あい",
			"c.fc":    "#fc 2\npublic const T = textmap(\"tbl.txt\");\n",
			"t.fc":    "#fc 2\nuse * from stdio;\nuse * from c;\nfunction main():void { print(T(\"いあ\")); }\n",
		}, "t.fc")
		if !strings.Contains(ir, codes(1, 0, 0)) {
			t.Errorf("textmap の変換結果が違う:\n%s", ir)
		}
	})
	t.Run("textmap errors", func(t *testing.T) {
		cases := []struct{ src, want string }{
			{"#fc 2\nconst T = textmap();\n", "textmap takes 1 argument"},
			{"#fc 2\nconst T:int = textmap(\"tbl.txt\");\n", "not compatible type"},
			{"#fc 2\nconst T = textmap(\"tbl.txt\");\nfunction main():void { var a = T(1); }\n", "string literal required"},
		}
		for _, c := range cases {
			err := compileFiles(t, map[string]string{"tbl.txt": "ab", "t.fc": c.src}, "t.fc")
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("%q: got %v, want /%s/", c.src, err, c.want)
			}
		}
	})
}

// codes は IR ダンプ上の配列定数の要素列 ({lit nil N #"uint8"} ...) を作る。
func codes(ns ...int) string {
	var parts []string
	for _, n := range ns {
		parts = append(parts, fmt.Sprintf("{lit nil %d #\"uint8\"}", n))
	}
	return strings.Join(parts, " ")
}
