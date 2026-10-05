package sema

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haramako/fc/internal/ir"
)

// TestBuiltins: include("*.rb") なしで使える組み込み (Agent/discussions/2026-09-13-v2-grammar.md §3.5)。
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
		if err == nil || !strings.Contains(err.Error(), ".rb macros are not supported") {
			t.Errorf("got %v", err)
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
			{"#fc 2\nconst T = textmap();\n", "textmap takes 1 or 2 arguments"},
			{"#fc 2\nconst T = textmap(\"tbl.txt\", \"a.po\", \"b\");\n", "textmap takes 1 or 2 arguments"},
			{"#fc 2\nconst T = textmap(\"tbl.txt\", \"nosuch.po\");\n", "nosuch.po"},
			{"#fc 2\nconst T = textmap(\"tbl.txt\");\nfunction main():void { var a = T(\"a\", \"b\", \"c\"); }\n", "text conversion takes a string and an optional msgctxt"},
			{"#fc 2\nconst T = textmap(\"tbl.txt\");\nfunction main():void { var a = T(\"a\", 1); }\n", "string literal required"},
			{"#fc 2\nconst T:int = textmap(\"tbl.txt\");\n", "a macro has no type"},
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

// TestTextmapPO: textmap(table, po) の翻訳 (docs/reference/language.md の「独自の文字表と翻訳」)。原文を \r を除いて全角にしたもの (msgid) と
// msgctxt の組で .po を引き、訳があれば訳文を変換する。訳が無い・空・fuzzy なら原文のまま変換して警告する。
func TestTextmapPO(t *testing.T) {
	// 表: あ0 い1 う2 え3 お4 Ｈ5 Ｉ6 ↓7
	const table = "あいうえおＨＩ↓"
	const po = `msgid ""
msgstr ""
"Language: en\n"

msgid "あい"
msgstr "HI"

msgctxt "greeting"
msgid "あい"
msgstr "IH"

msgctxt ""
msgid "あい"
msgstr "HH"

msgid ""
"う\n"
"え"
msgstr ""
"I\n"
"I"

#, fuzzy
msgid "お"
msgstr "I"

msgid "え"
msgstr ""

#~ msgid "あ"
#~ msgstr "H"

#. 原文の英字は全角にしてから照合する
msgctxt "x"
msgid "ＡＢ"
msgstr "IHI"

msgctxt "unknown"
msgid "あ"
msgstr "Hz"
`
	cases := []struct {
		call string
		want []int
		warn string // "" なら警告なし
	}{
		{`T("あい")`, []int{5, 6, 0}, ""},
		{`T("あい", "greeting")`, []int{6, 5, 0}, ""},
		{`T("あい", "")`, []int{5, 5, 0}, ""}, // msgctxt "" は msgctxt なしと別
		{`T("う\nえ")`, []int{6, 7, 6, 0}, ""},
		{`T("AB", "x")`, []int{6, 5, 6, 0}, ""},
		{`T("お")`, []int{4, 0}, `T: no translation for "お" (no msgctxt) in tr.po (fuzzy); using the original text`},
		{`T("え")`, []int{3, 0}, `T: no translation for "え" (no msgctxt) in tr.po (empty msgstr)`},
		{`T("あ")`, []int{0, 0}, `T: no translation for "あ" (no msgctxt) in tr.po (no entry)`}, // #~ は無視
		{`T("い", "greeting")`, []int{1, 0}, `T: no translation for "い" (msgctxt "greeting") in tr.po (no entry)`},
		{`T("")`, []int{0}, ""}, // "" はヘッダーと同じ msgid なので照合しない
		{`T("あ", "unknown")`, []int{5, 8, 0}, `T: "Hz" has characters not in the character table tbl.txt: 'ｚ' (U+FF5A)`},
		{`T("あい", null)`, []int{0, 1, 0}, ""}, // null は翻訳しない (訳があっても)
		{`T("あ", null)`, []int{0, 0}, ""},     // 訳が無くても警告しない
		{`T("あZ", null)`, []int{0, 8, 0}, `T: "あZ" has characters not in the character table tbl.txt: 'Ｚ'`}, // 表に無い文字は警告する
	}
	// 照合のキーは \r を除いて全角にしたもの (convertChar の前。濁点は分けない。fc の文字列リテラルに \r は書けない)
	if k := textKey("Ab-1 が\r\n"); k != "Ａｂ−１ が\n" {
		t.Errorf("textKey: %q", k)
	}
	for _, c := range cases {
		prog, ir := compileWithWarnings(t, map[string]string{
			"tbl.txt": table,
			"tr.po":   po,
			"t.fc":    "#fc 3\nuse * from stdio;\npublic const T = @textmap(\"tbl.txt\", \"tr.po\");\nfunction main():void { print(" + c.call + "); }\n",
		})
		if !strings.Contains(ir, "("+codes(c.want...)+")") {
			t.Errorf("%s: 変換結果が %v でない:\n%s", c.call, c.want, ir)
		}
		var ws []string
		for _, w := range prog.Warnings {
			ws = append(ws, w.Msg)
		}
		if c.warn == "" && len(ws) != 0 || c.warn != "" && (len(ws) != 1 || !strings.Contains(ws[0], c.warn)) {
			t.Errorf("%s: 警告 %q, want %q", c.call, ws, c.warn)
		}
	}

	// .po なし・空文字列なら今までと同じ (2 引数でも呼べ、msgctxt は無視。表に無い文字も警告しない)
	for _, decl := range []string{`@textmap("tbl.txt")`, `@textmap("tbl.txt", "")`} {
		prog, ir := compileWithWarnings(t, map[string]string{
			"tbl.txt": table,
			"t.fc":    "#fc 3\nuse * from stdio;\npublic const T = " + decl + ";\nfunction main():void { print(T(\"あいＺ\", \"greeting\")); print(T(\"あいＺ\", null)); }\n",
		})
		if !strings.Contains(ir, "("+codes(0, 1, 8, 0)+")") || len(prog.Warnings) != 0 {
			t.Errorf("%s: 今までと同じでない: %v\n%s", decl, prog.Warnings, ir)
		}
	}
}

// compileWithWarnings は mustCompileFiles と同じで、プログラム (警告) も返す。
func compileWithWarnings(t *testing.T, files map[string]string) (*Program, string) {
	t.Helper()
	dir := t.TempDir()
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o666); err != nil {
			t.Fatal(err)
		}
	}
	prog, err := Compile(dir, []string{".", filepath.ToSlash(filepath.Join(repoRoot, "fclib")), filepath.ToSlash(filepath.Join(repoRoot, "fclib", "emu"))}, "t.fc")
	if err != nil {
		t.Fatalf("コンパイル失敗: %v", err)
	}
	return prog, ir.DumpProgram(prog.Options, prog.Modules.List())
}

// codes は IR ダンプ上の配列定数の要素列 ({lit nil N #"u8"} ...) を作る。
func codes(ns ...int) string {
	var parts []string
	for _, n := range ns {
		parts = append(parts, fmt.Sprintf("{lit nil %d #\"u8\"}", n))
	}
	return strings.Join(parts, " ")
}
