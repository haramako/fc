package driver

// fc 3 (`#fc 3`) の言語の規則のテスト (doc/v3_plan.md)。fc 2 と fc 3 のモジュールは 1 つのプログラムに混ぜられる。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// buildFiles は files (ファイル名 → ソース) を t.fc から emu でビルドして走らせ、出力とエラーを返す。
func buildFiles(t *testing.T, files map[string]string) (string, error) {
	t.Helper()
	dir := t.TempDir()
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o666); err != nil {
			t.Fatal(err)
		}
	}
	var out strings.Builder
	_, err := NewCompiler(absRepoRoot).Build("t.fc", &BuildOptions{Dir: dir, BuildDir: filepath.Join(dir, "b"), Out: filepath.Join(dir, "a.bin"), Run: true, Stdout: &out, MaxCycles: 10_000_000})
	return out.String(), err
}

// TestV3IntTypes: fc 3 の整数型名は u8 / i8 / u16 / i16 だけ。fc 2 の名前 (int など) は案内つきのエラー、u8 などは宣言できない。
// fc 2 のモジュール (古い名前) と fc 3 のモジュールを混ぜられ、fc 2 でも短い名前を使える。
func TestV3IntTypes(t *testing.T) {
	t.Parallel()
	out, err := buildFiles(t, map[string]string{
		"t.fc": `#fc 3
use * from stdio;
use lib;
function main():void
{
	var a:u8 = 200;
	var b:i8 = -3;
	var c:u16 = 1000;
	var d:i16 = -1000;
	printf(lib.add(a, 55), " ", (b as i16) + d, " ", c + lib.SIZE, "\n");
	exit(0);
}
`,
		"lib.fc": `#fc 2
public const SIZE:int16 = 24;
public function add(x:int, y:u8):int16 { return (x as int16) + y; }
`,
	})
	if err != nil || out != "255 64533 1024\n" {
		t.Errorf("got %q, %v", out, err)
	}
	for _, c := range []struct{ src, msg string }{
		{"#fc 3\nvar a:int;\nfunction main():void { }\n", "int is not a type in fc 3 (write u8; `fcc migrate` rewrites fc 2 sources)"},
		{"#fc 3\nvar a:sint16;\nfunction main():void { }\n", "sint16 is not a type in fc 3 (write i16"},
		{"#fc 3\nfunction main():void { var n = @sizeof(uint8); }\n", "uint8 is not a type in fc 3 (write u8"},
		{"#fc 3\nvar u8:u8;\nfunction main():void { }\n", "u8 cannot be declared (it is a type name in fc 3)"},
		{"#fc 3\nfunction main():void { var i16 = 1; }\n", "i16 cannot be declared"},
		{"#fc 3\nfunction u16():void { }\nfunction main():void { }\n", "u16 cannot be declared"},
	} {
		if _, err := buildFiles(t, map[string]string{"t.fc": c.src}); err == nil || !strings.Contains(err.Error(), c.msg) {
			t.Errorf("%q: got %v, want /%s/", c.src, err, c.msg)
		}
	}
}

// TestV3AtBuiltins: fc 3 の `@` の組み込み。`@` の無い古い書き方は案内つきのエラー、`min` などは利用者が宣言できる、
// fc 2 のソースでは `@` は使えない。
func TestV3AtBuiltins(t *testing.T) {
	t.Parallel()
	out, err := buildFiles(t, map[string]string{
		"t.fc": `#fc 3
use * from stdio;
@include("k.asm");
const TAB:[?]u8 = @incbin("tab.bin");
var g:u16;
function sizeof(x:u8):u8 { return x + 1; }   // fc 3 では普通の名前
function max(a:u8, b:u8):u8 { return a; }    // 組み込みの @max とは別
function main():void
{
	@asm("lda #7", "sta _t_g");
	var p = @bitcast(*u8, &g);
	printf(@sizeof(i16), " ", @min(TAB[0], TAB[1]), " ", @max(3, 9), " ", max(3, 9), " ", @clamp(20, 0, 10), " ", *p, " ", sizeof(4), "\n");
	exit(0);
}
`,
		"k.asm":   "\t.byte 1 ; @include したファイルもアセンブルされる\n",
		"tab.bin": "\x05\x02",
	})
	if err != nil || out != "2 2 9 3 10 7 5\n" {
		t.Errorf("got %q, %v", out, err)
	}
	for _, c := range []struct{ src, msg string }{
		{"#fc 3\nfunction main():void { asm(\"sei\"); }\n", "asm not found (write @asm in fc 3"},
		{"#fc 3\nfunction main():void { var n = sizeof(u8); }\n", "sizeof not found (write @sizeof in fc 3"},
		{"#fc 3\ninclude(\"k.asm\");\nfunction main():void { }\n", "include not found (write @include in fc 3"},
		{"#fc 3\nfunction main():void { var n = min(1, 2); }\n", "min not found (write @min in fc 3"},
		{"#fc 2\nfunction main():void { var n = @min(1, 2); }\n", "invalid token"},
	} {
		if _, err := buildFiles(t, map[string]string{"t.fc": c.src}); err == nil || !strings.Contains(err.Error(), c.msg) {
			t.Errorf("%q: got %v, want /%s/", c.src, err, c.msg)
		}
	}
}

// TestV3Attributes: fc 3 の属性 `@(...)`。`@(inline)` は `inline: true`、`@(inline: false)` は偽 (以前はキーがあるだけで
// inline になった)。値の要る属性の省略・古い options(...) / block はエラー。`@(bss: ...) { }` は中の変数の既定の置き場所。
func TestV3Attributes(t *testing.T) {
	t.Parallel()
	src := `#fc 3
use * from stdio;
@(bss: "BSS") {
	var b:u8;
}
// ループ入りの関数は自動インラインの対象外なので、展開されるのは明示の inline だけ
function one():u8 @(inline) { var s:u8 = 0; for (var i:u8 = 0; i < 3; i++) { s += i; } return s; }
function two():u8 @(inline: false) { var s:u8 = 0; for (var i:u8 = 0; i < 3; i++) { s += i + 1; } return s; }
function main():void
{
	b = one() + two();
	printf(b, "\n");
	exit(0);
}
`
	out, err := buildFiles(t, map[string]string{"t.fc": src})
	if err != nil || out != "9\n" {
		t.Errorf("got %q, %v", out, err)
	}
	if asm := compileAsmFiles(t, map[string]string{"t.fc": src}); strings.Contains(asm, "jsr _t_one") || !strings.Contains(asm, "jsr _t_two") {
		t.Errorf("@(inline) は展開、@(inline: false) は呼び出しのはず:\n%s", asm)
	}
	for _, c := range []struct{ src, msg string }{
		{"#fc 3\n@(bank);\nfunction main():void { }\n", "@(bank) needs a value"},
		{"#fc 3\nvar v:u8 @(address);\nfunction main():void { }\n", "@(address) needs a value"},
		{"#fc 3\noptions(bank: 1);\nfunction main():void { }\n", "`options(...)` is written `@(...)` in fc 3"},
		{"#fc 3\nblock { var b:u8; } options(bss: \"BSS\");\nfunction main():void { }\n", "is written `@(...) { ... }` in fc 3"},
	} {
		if _, err := buildFiles(t, map[string]string{"t.fc": c.src}); err == nil || !strings.Contains(err.Error(), c.msg) {
			t.Errorf("%q: got %v, want /%s/", c.src, err, c.msg)
		}
	}
}

// compileAsmFiles は files を t.fc からコンパイルして _t.s を返す。
func compileAsmFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o666); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := NewCompiler(absRepoRoot).Build("t.fc", &BuildOptions{Dir: dir, BuildDir: filepath.Join(dir, "b"), Out: filepath.Join(dir, "a.bin"), CompileOnly: true}); err != nil {
		t.Fatalf("コンパイル失敗: %v", err)
	}
	asm, err := os.ReadFile(filepath.Join(dir, "b", "_t.s"))
	if err != nil {
		t.Fatal(err)
	}
	return string(asm)
}

// buildFilesDefs は buildFiles に CLI の -D を渡す版。
func buildFilesDefs(t *testing.T, files map[string]string, defines []string) (string, *Result, error) {
	t.Helper()
	dir := t.TempDir()
	for name, src := range files {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o777); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o666); err != nil {
			t.Fatal(err)
		}
	}
	var out strings.Builder
	res, err := NewCompiler(absRepoRoot).BuildContext(t.Context(), "t.fc", &BuildOptions{Dir: dir, BuildDir: filepath.Join(dir, "b"), Out: filepath.Join(dir, "a.bin"), Run: true, Stdout: &out, MaxCycles: 10_000_000, Defines: defines})
	return out.String(), res, err
}

// TestV3StaticIf: @if と @(build) の const。トップレベルの @if は use ごと選び、選ばれなかった側は名前解決しない
// (存在しないモジュール・関数でもよい)。関数の中の @if は同じスコープ。値は fc.toml の [define.<module>] と -D で上書きできる。
func TestV3StaticIf(t *testing.T) {
	t.Parallel()
	files := map[string]string{
		"t.fc": `#fc 3
use * from stdio;
use common;
@if (common.DEBUG) {
	use dbg;
	function f():u8 { return 1; }
} else @if (common.LEVEL > 2) {
	use nosuchmodule;   // 選ばれないので読み込まない
	function f():u8 { return undefined_name; }
} else {
	function f():u8 { return 7; }
}
function main():void
{
	@if (common.DEBUG && LOCAL) {
		var x = dbg.value();
	} else {
		var x = f();
	}
	printf(x, " ", common.LEVEL, "\n");
	exit(0);
}
const LOCAL = true @(build);
`,
		"common.fc": `#fc 3
public const DEBUG = false @(build);
public const LEVEL:u8 = 1 @(build);
`,
		"dbg.fc": `#fc 3
public function value():u8 { return 42; }
`,
	}
	out, res, err := buildFilesDefs(t, files, nil)
	if err != nil || out != "7 1\n" {
		t.Fatalf("既定値: got %q, %v", out, err)
	}
	if len(res.Defines) != 0 {
		t.Errorf("上書きが無いのに Defines: %v", res.Defines)
	}
	if out, res, err = buildFilesDefs(t, files, []string{"common.DEBUG=true"}); err != nil || out != "42 1\n" {
		t.Errorf("-D common.DEBUG=true: got %q, %v", out, err)
	} else if len(res.Defines) != 1 || !res.Defines[0].Used || res.Defines[0].Source != "-D" {
		t.Errorf("Defines: %+v", res.Defines)
	}
	// fc.toml の上書き (CLI の -D が後勝ち)
	files["fc.toml"] = "# project\n[define.common]\nDEBUG = true   # comment\nLEVEL = 3\n[define.t]\nLOCAL = false\n"
	if out, _, err = buildFilesDefs(t, files, []string{"common.DEBUG=false"}); err == nil {
		t.Errorf("LEVEL = 3 なら f は undefined_name を読む (選ばれた側は名前解決する): got %q", out)
	} else if !strings.Contains(err.Error(), "nosuchmodule") && !strings.Contains(err.Error(), "undefined_name") {
		t.Errorf("LEVEL = 3: got %v", err)
	}
	files["fc.toml"] = "[define.common]\nDEBUG = true\n[define.t]\nLOCAL = false\n"
	if out, _, err = buildFilesDefs(t, files, nil); err != nil || out != "1 1\n" {
		t.Errorf("fc.toml DEBUG = true, LOCAL = false: got %q, %v (dbg の側の f、main は else 側)", out, err)
	}
	delete(files, "fc.toml")

	for _, c := range []struct {
		defs []string
		msg  string
	}{
		{[]string{"common.NOPE=1"}, "common has no @(build) const NOPE"},
		{[]string{"nosuch.X=1"}, "module nosuch not found"},
		{[]string{"common.DEBUG=3"}, "DEBUG is a bool @(build) const"},
		{[]string{"common.LEVEL=true"}, "LEVEL is an integer @(build) const"},
		{[]string{"common.LEVEL"}, "expected module.NAME=value"},
	} {
		if _, _, err := buildFilesDefs(t, files, c.defs); err == nil || !strings.Contains(err.Error(), c.msg) {
			t.Errorf("-D %v: got %v, want /%s/", c.defs, err, c.msg)
		}
	}
	// ビルドに含まれないモジュールへの上書きは警告
	if _, res, err := buildFilesDefs(t, files, []string{"dbg.X=1"}); err != nil {
		t.Errorf("dbg.X=1: %v", err)
	} else if len(res.Warnings) == 0 || !strings.Contains(res.Warnings[len(res.Warnings)-1].Msg, "module dbg is not part of this build") {
		t.Errorf("dbg.X=1 の警告: %v", res.Warnings)
	}

	for _, c := range []struct{ src, msg string }{
		{"#fc 3\nconst N = 1;\n@if (N) { }\nfunction main():void { }\n", "@if condition can use only literals and @(build) constants (N is not @(build))"},
		{"#fc 3\nconst N = 1 + 1 @(build);\nfunction main():void { }\n", "@(build) const N must be initialized with a literal"},
		{"#fc 3\n@if (true) { const N = 1 @(build); }\nfunction main():void { }\n", "@(build) const N cannot be declared inside @if"},
		{"#fc 3\nfunction main():void { var a:u8 = 1; @if (a) { } }\n", "a is not @(build)"},
	} {
		if _, err := buildFiles(t, map[string]string{"t.fc": c.src}); err == nil || !strings.Contains(err.Error(), c.msg) {
			t.Errorf("%q: got %v, want /%s/", c.src, err, c.msg)
		}
	}
}

// TestV3Enum: enum の宣言 (基底型は省けば u8、値は省けば前の値 + 1)、Type.Name / 文脈からの .Name、同じ enum 同士の比較、
// `as` の変換、配列の添字、switch (default が無くメンバーが足りなければ警告)。整数・別の enum とは混ぜられない。
func TestV3Enum(t *testing.T) {
	t.Parallel()
	files := map[string]string{
		"t.fc": `#fc 3
use * from stdio;
use my;
enum Dir:i8 { Left = -1, None, Right, }
const NAMES:[3]u8 = [10, 20, 30];
var state:my.State;
function next(s:my.State):my.State
{
	switch (s) {
	case .Stand: return .Jump;
	case .Jump: return my.State.Die;
	}
	return s;
}
function main():void
{
	var d:Dir = .Right;
	state = .Stand;
	state = next(state);
	var a = state == .Jump;
	var b = .Die > state;
	var c = state < my.State.Stand;
	printf(state as u8, " ", a, " ", b, " ", c, " ", (d as i8) + 1, " ", NAMES[my.State.Die], " ", Dir.Left as u8, "\n");
	exit(0);
}
`,
		"my.fc": `#fc 3
public enum State { Stand, Jump, Die = 2 }
`,
	}
	out, res, err := buildFilesDefs(t, files, nil)
	if err != nil || out != "1 1 1 0 2 30 255\n" {
		t.Fatalf("got %q, %v", out, err)
	}
	found := false
	for _, w := range res.Warnings {
		if strings.Contains(w.Msg, "switch on my.State does not handle .Die") {
			found = true
		}
	}
	if !found {
		t.Errorf("switch の網羅の警告が無い: %v", res.Warnings)
	}
	for _, c := range []struct{ src, msg string }{
		{"enum E { A, B }\nfunction main():void { var e:E = 0; }\n", "cannot assign u8 to t.E"},
		{"enum E { A, B }\nfunction main():void { var e:E = .A; var n = e + 1; }\n", "cannot apply + to enum t.E"},
		{"enum E { A, B }\nenum F { A }\nfunction main():void { var e:E = .A; var f:F = .A; var x = e == f; }\n", "cannot compare t.E and t.F"},
		{"enum E { A, B }\nfunction main():void { var e:E = .C; }\n", "t.E has no member C (members: A, B)"},
		{"enum E { A, B }\nfunction main():void { var n:u8 = .A; }\n", ".A needs an enum type from context"},
		{"enum E { A = 300 }\nfunction main():void { }\n", "enum E: A = 300 does not fit in u8"},
		{"enum E { A, A }\nfunction main():void { }\n", "member A already defined"},
		{"enum E:bool { A }\nfunction main():void { }\n", "the base type must be an integer type"},
	} {
		if _, err := buildFiles(t, map[string]string{"t.fc": "#fc 3\n" + c.src}); err == nil || !strings.Contains(err.Error(), c.msg) {
			t.Errorf("%q: got %v, want /%s/", c.src, err, c.msg)
		}
	}
	// fc 2 では enum は予約語でない
	if _, err := buildFiles(t, map[string]string{"t.fc": "#fc 2\nuse * from stdio;\nvar enum:int;\nfunction main():void { enum = 1; exit(0); }\n"}); err != nil {
		t.Errorf("fc 2 の enum という名前: %v", err)
	}
}

// TestV3ConstPointer: `*const T`。const の配列・文字列リテラル (とそこから作ったポインタ) は読み取り専用。書き込みと
// `as` で const を外すことはエラー、*T として渡すのは警告 (fc 3 の最初の版)。@bitcast で外せる。生成コードは *T と同じ。
// 型を省いた変数は、読み取り専用のポインタで初期化すると *const (警告しない。書き込みはエラー)。
func TestV3ConstPointer(t *testing.T) {
	t.Parallel()
	src := `#fc 3
use * from stdio;
const TABLE:[4]u8 = [1, 2, 3, 4];
struct P { x:u8; }
const PS:[1]P = [{5}];
function sum(p:*const u8, n:u8):u8 { var s:u8 = 0; for (var i:u8 = 0; i < n; i++) { s += p[i]; } return s; }
function first(p:*u8):u8 { return *p; }
function main():void
{
	var p:*const u8 = TABLE;
	var q:*const P = &PS[0];
	var w = @bitcast(*u8, p);
	var r = &PS[0];
	printf(sum(TABLE, 4), " ", sum(&TABLE[1], 2), " ", *p, " ", q.x, " ", first(w), " ", first(TABLE), " ", r.x, "\n");
	exit(0);
}
`
	out, res, err := buildFilesDefs(t, map[string]string{"t.fc": src}, nil)
	if err != nil || out != "10 5 1 5 1 1 5\n" {
		t.Fatalf("got %q, %v", out, err)
	}
	var drops []string
	for _, w := range res.Warnings {
		if strings.Contains(w.Msg, "passes read-only data") {
			drops = append(drops, w.Msg)
		}
	}
	if len(drops) != 1 || !strings.Contains(drops[0], "argument 1 of `first`") {
		t.Errorf("警告は first(TABLE) の 1 つだけのはず: %v", drops)
	}
	for _, c := range []struct{ body, msg string }{
		{"var p:*const u8 = TABLE; *p = 1;", "cannot assign through a read-only pointer"},
		{"var p:*const u8 = TABLE; p[1] = 1;", "cannot assign through a read-only pointer"},
		{"TABLE[0] = 1;", "cannot assign through a read-only pointer"},
		{"var q:*const P = &PS[0]; q.x = 1;", "cannot assign through a read-only pointer"},
		{"var q = &PS[0]; q.x = 1;", "cannot assign through a read-only pointer"},
		{"var p = &TABLE[1]; *p = 1;", "cannot assign through a read-only pointer"},
		{"var p:*const u8 = TABLE; var r = p as *u8;", "with `as` (use bitcast"}, // ポインタ同士の as はもともと不可
	} {
		prog := "#fc 3\nconst TABLE:[4]u8 = [1, 2, 3, 4];\nstruct P { x:u8; }\nconst PS:[1]P = [{5}];\nfunction main():void { " + c.body + " }\n"
		if _, err := buildFiles(t, map[string]string{"t.fc": prog}); err == nil || !strings.Contains(err.Error(), c.msg) {
			t.Errorf("%q: got %v, want /%s/", c.body, err, c.msg)
		}
	}
	if _, err := buildFiles(t, map[string]string{"t.fc": "#fc 2\nvar p:*const int;\nfunction main():void { }\n"}); err == nil || !strings.Contains(err.Error(), "`*const T` is fc 3 syntax") {
		t.Errorf("fc 2 の *const: %v", err)
	}
}
