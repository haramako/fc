package driver

// fc 4 の文字列は 0 終端にしない (sema/strconst.go)。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestV4StringNoTerminator: fc 4 の文字列は文字の数だけの配列 (`var s = "abc"` も @len / @sizeof が 3、"" は 0)。0 終端の文字列が
// 要る所には "…\0" と書く。0 の無い文字列をポインタにするのはエラーで、表として使うなら @ptr(s)。
func TestV4StringNoTerminator(t *testing.T) {
	t.Parallel()
	out, err := buildBothLevels(t, map[string]string{"t.fc": `#fc 4
use console;
const DIGITS = "0123";
const Z = "hi\0";
const EMPTY = "";
const MSGS:[2]*const u8 = ["ab\0", Z];
function main():void
{
	var s = "abc";
	var t:*const u8 = @ptr(DIGITS);
	@printf("{} {} {} {} {} {} {}\n", @len(s), @sizeof(s), @len(DIGITS), @sizeof(DIGITS), @len(Z), @len(EMPTY), t[2]);
	console.write_z("xy\0");
	console.write_z(MSGS[0]);
	console.write_z(MSGS[1]);
	console.newline();
	console.exit(0);
}
`})
	if err != nil || out != "3 3 4 4 3 0 50\nxyabhi\n" {
		t.Errorf("got %q, %v", out, err)
	}
	for src, want := range map[string]string{
		`console.write_z("ab");`:                                `string "ab" has no terminating 0 and cannot be used as a pointer`,
		`var p:*const u8 = DIGITS;`:                             "string `DIGITS` has no terminating 0",
		`var p = "ab" as *const u8;`:                            `string "ab" has no terminating 0`,
		`const T:[1]*const u8 = ["ab"]; console.write_z(T[0]);`: `string "ab" has no terminating 0`,
	} {
		_, err := buildFiles(t, map[string]string{"t.fc": "#fc 4\nuse console;\nconst DIGITS = \"0123\";\nfunction main():void\n{\n\t" + src + "\n}\n"})
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: got %v, want /%s/", src, err, want)
		}
	}
}

// TestV4MigrateStringTerminator: fc 3 → 4 の migrate は、ポインタにする文字列リテラルと型を書かない配列の変数の初期値の文字列に
// `\0` を足し、名前付きの文字列定数は長さつきにする。データも長さも fc 3 と同じで、出力が変わらない。
func TestV4MigrateStringTerminator(t *testing.T) {
	t.Parallel()
	src := `#fc 3
use * from stdio;
const NM = "joe";
const PS:[2]*const u8 = ["p", "q"];
function main():void
{
	var s = "abc";
	print("lit");
	print(NM);
	print(PS[1]);
	printf(" ", @len(s), " ", @len(NM), " ", s[3], "\n");
	exit(0);
}
`
	want := `#fc 4
use * from stdio;
const NM:[4]u8 = "joe";
const PS:[2]*const u8 = ["p\0", "q\0"];
function main():void
{
	var s = "abc\0";
	print("lit\0");
	print(NM);
	print(PS[1]);
	@printf(" {} {} {}\n", @len(s), @len(NM), s[3]);
	exit(0);
}
`
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
	if before != after || before != "litjoeq 4 4 0\n" {
		t.Errorf("fc 3: %q, fc 4: %q", before, after)
	}
}

// TestV4StringRows: fc 4 で型を書かない文字列の配列は、長さを問わず slice の表 (`[?][]const u8`)。fc 3 は同じ長さの行の 2 次元配列
// (終端の 0 を含む) なので、migrate は今の型を書き足して変えない。
func TestV4StringRows(t *testing.T) {
	t.Parallel()
	out, err := buildBothLevels(t, map[string]string{"t.fc": `#fc 4
use console;
const NAMES = ["ab", "cde"];
const SAME = ["xy", "zw"];
function main():void
{
	var loc = ["q", "rs", ""];
	@printf("{} {} {} {} {} {}\n", @len(NAMES), NAMES[1], @len(SAME[0]), @sizeof(SAME), @len(loc), @len(loc[2]));
	for (var s in NAMES) {
		@printf("{}|", s);
	}
	console.newline();
	console.exit(0);
}
`})
	if err != nil || out != "2 cde 2 6 3 0\nab|cde|\n" {
		t.Errorf("got %q, %v", out, err)
	}
	src := `#fc 3
use * from stdio;
const T = ["ab", "cd"];
function main():void
{
	var l = ["x", "y"];
	print(T[1]);
	print(l[0]);
	printf(" ", @sizeof(T), " ", @sizeof(l), "\n");
	exit(0);
}
`
	want := `#fc 4
use * from stdio;
const T:[2][3]u8 = ["ab", "cd"];
function main():void
{
	var l:[2][2]u8 = ["x", "y"];
	print(T[1]);
	print(l[0]);
	@printf(" {} {}\n", @sizeof(T), @sizeof(l));
	exit(0);
}
`
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
	if before != after || before != "cdx 6 4\n" {
		t.Errorf("fc 3: %q, fc 4: %q", before, after)
	}
}
