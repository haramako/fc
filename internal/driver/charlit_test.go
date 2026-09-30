package driver

// fc 4 の文字のリテラル 'A' の migrate と整形 (TestCharLiteral は v4_test.go)。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haramako/fc/internal/syntax"
)

// TestV4MigrateSingleQuote: fc 3 → 4 の migrate は、シングルクォートの文字列 (エスケープを解釈しない) をダブルクォートにする。
// 中の \ と " は \\ / \" にして中身を変えない。属性・include の文字列も。出力は変わらない。
func TestV4MigrateSingleQuote(t *testing.T) {
	t.Parallel()
	src := "#fc 3\nuse * from stdio;\nconst S = 'a\\nb\"c';\nvar v:u8 @(symbol: '_t_v');\nfunction main():void\n{\n\tprintf('x\\n', S, \"\\n\");\n\texit(0);\n}\n"
	want := "#fc 4\nuse * from stdio;\nconst S = \"a\\\\nb\\\"c\";\nvar v:u8 @(symbol: \"_t_v\");\nfunction main():void\n{\n\tprintf(\"x\\x5Cn{}\\n\", S);\n\texit(0);\n}\n"
	dir := t.TempDir()
	path := filepath.Join(dir, "t.fc")
	if err := os.WriteFile(path, []byte(src), 0o666); err != nil {
		t.Fatal(err)
	}
	res, err := NewCompiler(absRepoRoot).Migrate([]string{path}, &MigrateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got := string(res[path]); got != want {
		t.Fatalf("migrate:\n%s\nwant:\n%s", got, want)
	}
	before, err := buildBothLevels(t, map[string]string{"t.fc": src})
	if err != nil {
		t.Fatal(err)
	}
	after, err := buildBothLevels(t, map[string]string{"t.fc": want})
	if err != nil {
		t.Fatal(err)
	}
	if before != after || before != "x\\na\\nb\"c\n" {
		t.Errorf("fc 3: %q, fc 4: %q", before, after)
	}
}

// TestCharLiteralFormat: fcc fmt は文字のリテラルを綴りのまま残す。
func TestCharLiteralFormat(t *testing.T) {
	t.Parallel()
	src := "#fc 4\nconst A = 'A';\nconst NL = '\\n';\nconst Q = '\\'';\n"
	out, err := syntax.Format([]byte(src), "t.fc")
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != src {
		t.Errorf("got %q", out)
	}
}

// TestStringEscapes: fc 4 の文字列のエスケープは文字のリテラルと同じ (\n \t \0 \\ \' \" \xNN)。ほかの `\` はエラー。fc 3 の文字列は
// \n と \xNN だけで、ほかの `\` はそのまま残る。fc 3 → 4 の migrate はそれを `\\` にして中身を変えない。
func TestStringEscapes(t *testing.T) {
	t.Parallel()
	out, err := buildFiles(t, map[string]string{"t.fc": "#fc 4\nuse console;\nconst S = \"a\\tb\\0\\\\\\\"\\'\\x41\";\nfunction main():void\n{\n\tfor (var c in S) { printf(\"{} \", c); }\n\tprintf(\"{}\\n\", @len(S));\n\tconsole.exit(0);\n}\n"})
	if err != nil || out != "97 9 98 0 92 34 39 65 8\n" {
		t.Errorf("got %q, %v", out, err)
	}
	if _, err := buildFiles(t, map[string]string{"t.fc": "#fc 4\nconst S = \"a\\qb\";\nfunction main():void { }\n"}); err == nil || !strings.Contains(err.Error(), `invalid escape \q in string`) {
		t.Errorf("\\q: got %v", err)
	}
	src := "#fc 3\nuse * from stdio;\nconst S = \"a\\tb\\\"c\\n\";\nfunction main():void\n{\n\tprint(S);\n\texit(0);\n}\n"
	want := "#fc 4\nuse * from stdio;\nconst S = \"a\\\\tb\\\\\\\"c\\n\";\nfunction main():void\n{\n\tprint(S);\n\texit(0);\n}\n"
	dir := t.TempDir()
	path := filepath.Join(dir, "t.fc")
	if err := os.WriteFile(path, []byte(src), 0o666); err != nil {
		t.Fatal(err)
	}
	res, err := NewCompiler(absRepoRoot).Migrate([]string{path}, &MigrateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got := string(res[path]); got != want {
		t.Fatalf("migrate:\n%s\nwant:\n%s", got, want)
	}
	before, err := buildBothLevels(t, map[string]string{"t.fc": src})
	if err != nil {
		t.Fatal(err)
	}
	after, err := buildBothLevels(t, map[string]string{"t.fc": want})
	if err != nil {
		t.Fatal(err)
	}
	if before != after || before != "a\\tb\\\"c\n" {
		t.Errorf("fc 3: %q, fc 4: %q", before, after)
	}
}
