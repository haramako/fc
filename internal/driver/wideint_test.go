package driver

// 24 / 32 ビットの整数 (u24 / i24 / u32 / i32。fc 4 だけ。Agent/wiki/plans/wide-int.md) のテスト。

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWideIntRun: 加減算・比較・シフト・符号拡張・乗除算 (ランタイムの 32 ビットの関数)・A1 の広げ・型のない定数の型・定数の表・
// struct のフィールド・引数と戻り値・@printf を、-O 0 / -O 2 とインタプリタで走らせて期待の出力 (Python で計算して確かめた
// testdata/wideint.txt) と比べる。
func TestWideIntRun(t *testing.T) {
	t.Parallel()
	src, err := os.ReadFile(filepath.Join("testdata", "wideint.fc"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "wideint.txt"))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{"t.fc": string(src)}
	out, err := buildBothLevels(t, files)
	if err != nil {
		t.Fatal(err)
	}
	if out != string(want) {
		t.Errorf("出力が違う:\n got: %q\nwant: %q", out, want)
	}
	iout, ok, err := rpInterp(t, files)
	if err != nil || !ok {
		t.Fatalf("インタプリタ: ok=%v err=%v", ok, err)
	}
	if iout != string(want) {
		t.Errorf("インタプリタの出力が違う:\n got: %q\nwant: %q", iout, want)
	}
}

// TestWideIntNoRuntimeUnlessUsed: 3・4 バイトの乗除算を使わないビルドは runtime_wide.asm をリンクしない (ROM を変えない)。
func TestWideIntNoRuntimeUnlessUsed(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		body string
		want bool
	}{
		{"var a:u32 = g; g = a + 1;", false},
		{"var a:u32 = g; g = a * g;", true},
		{"var a:i24 = g as i24; g = (a % 7) as u32;", true},
	} {
		src := "#fc 4\nuse console;\nvar g:u32;\nfunction main():void { " + c.body + " console.exit(0); }\n"
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "t.fc"), []byte(src), 0o666); err != nil {
			t.Fatal(err)
		}
		res, err := NewCompiler(absRepoRoot).BuildContext(context.Background(), "t.fc", &BuildOptions{Dir: dir, BuildDir: filepath.Join(dir, "b"), Out: filepath.Join(dir, "a.bin"), CompileOnly: true})
		if err != nil {
			t.Fatalf("%s: %v", c.body, err)
		}
		got := false
		for _, o := range res.Objects {
			got = got || filepath.Base(o) == "runtime_wide.o"
		}
		if got != c.want {
			t.Errorf("%s: runtime_wide.o がある = %v、want %v", c.body, got, c.want)
		}
	}
}

// TestWideIntErrors: 3・4 バイトの整数でエラーになる形。
func TestWideIntErrors(t *testing.T) {
	t.Parallel()
	pre := "#fc 4\nvar g:u32;\nvar p:*u8;\nvar arr:[4]u8;\n"
	cases := []struct{ name, src, want string }{
		{"fc 3 では型でない", "#fc 3\nvar x:u32;\n", "u32 is a type of fc 4 only"},
		{"狭める変換", pre + "function f():void { var a:u16 = g; arr[0] = a as u8; }", "cannot convert u32 to u16 implicitly"},
		{"定数の範囲", pre + "function f():void { var a:u24 = 16777216; g = a; }", "16777216 does not fit in u24"},
		{"32 ビットに入らない定数", pre + "function f():void { g = g + 5000000000; }", "does not fit in any integer type (32 bits)"},
		{"添字", pre + "function f():void { arr[g] = 1; }", "an index must be u8, i8, u16 or i16 (got u32"},
		{"ポインタの足し算", pre + "function f():void { p = p + g; }", "an offset added to a pointer must be at most 16 bits"},
		{"slice の長さの型", pre + "var s:[:u32]u8;\n", "the length type of a slice must be u8 or u16"},
		{"@slice の長さ", pre + "function f():void { var s = @slice(p, g); arr[0] = s[0]; }", "@slice: the length must be u8 or u16"},
		{"比較の定数", pre + "var h:i32;\nfunction f():void { if (h < 3000000000) { g = 1; } }", "does not fit in i32"},
		{"符号の混ざった比較", pre + "var h:i32;\nfunction f():void { if (h < g) { g = 1; } }", "ordered comparison of signed i32 and unsigned u32"},
		{"case の値", pre + "var c:u24;\nfunction f():void { switch (c) { case 16777216: g = 1; } }", "case value 16777216 does not fit in u24"},
	}
	for i, c := range cases {
		c := c
		t.Run(fmt.Sprintf("err%02d", i+1), func(t *testing.T) {
			t.Parallel()
			_, err := buildFiles(t, map[string]string{"t.fc": c.src + "\nfunction main():void { }\n"})
			if err == nil {
				t.Fatalf("%s: エラーにならない: %s", c.name, c.src)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("%s: got %v, want %q", c.name, err, c.want)
			}
		})
	}
}
