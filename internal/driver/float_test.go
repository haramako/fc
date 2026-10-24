package driver

// 浮動小数 (f32 / f16。fc 4 だけ。Agent/wiki/design/float-fixed.md) のテスト。

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestFloatRun: 四則・単項の -・比較 (NaN・<= を !(b < a) にしない)・型を書かない変数 (f32)・整数 / 固定小数との `as`
// (切り捨てと飽和・最近接)・f16・定数の畳み込み・条件 (testdata/floating.fc。ビットは Go の float32 で確かめた) と、出力の書式
// (floating_fmt.fc: `{}` の決め方・`{:.N}`・nan / inf・幅と 0 埋め・f16) を -O 0 / -O 2 とインタプリタで走らせて比べる。
func TestFloatRun(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"floating", "floating_fmt", "floating_init", "floating_args"} {
		src, err := os.ReadFile(filepath.Join("testdata", name+".fc"))
		if err != nil {
			t.Fatal(err)
		}
		want, err := os.ReadFile(filepath.Join("testdata", name+".txt"))
		if err != nil {
			t.Fatal(err)
		}
		files := map[string]string{"t.fc": string(src)}
		out, err := buildBothLevels(t, files)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if out != string(want) {
			t.Errorf("%s: 出力が違う:\n got: %q\nwant: %q", name, out, want)
		}
		if iout, ok, err := rpInterp(t, files); err != nil || !ok || iout != string(want) {
			t.Errorf("%s: インタプリタ: %q ok=%v err=%v", name, iout, ok, err)
		}
	}
}

// TestFloatErrors: 浮動小数でエラーになる形。
func TestFloatErrors(t *testing.T) {
	t.Parallel()
	pre := "#fc 4\nvar g:u8;\nvar f:f32;\nvar h:f16;\nvar q:u8f8;\n"
	cases := []struct{ name, src, want string }{
		{"fc 3 では型でない", "#fc 3\nvar x:f32;\n", "f32 is a type of fc 4 only"},
		{"f32 と f16", pre + "function body():void { f = f + h; }", "cannot apply + to f32 and f16"},
		{"f32 と整数の変数", pre + "function body():void { f = f + g; }", "cannot apply + to f32 and u8"},
		{"f32 と固定小数", pre + "function body():void { f = f * q; }", "cannot apply * to"},
		{"%", pre + "function body():void { f = f % 2.0; }", "% is not defined on f32"},
		{"ビット演算", pre + "function body():void { f = f & f; }", "& is not defined on f32"},
		{"シフト", pre + "function body():void { f = f << 1; }", "is not defined on f32"},
		{"!", pre + "function body():void { var b = !f; }", "! is not defined on f32"},
		{"条件", pre + "function body():void { if (f) { g = 1; } }", "cannot be used as a condition"},
		{"switch", pre + "function body():void { switch (f) { case 1: g = 1; } }", "cannot switch on f32"},
		{"整数 → f32 は as", pre + "function body():void { f = g; }", "cannot convert u8 to f32 implicitly"},
		{"f32 → 整数は as", pre + "function body():void { g = f; }", "cannot convert f32 to u8 implicitly"},
		{"f16 → f32 は as", pre + "function body():void { f = h; }", "cannot convert f16 to f32 implicitly"},
		{"範囲の外の定数", pre + "function body():void { h = 70000.0; }", "70000 does not fit in f16"},
		{"大きな整数のリテラル", pre + "function body():void { var x:u32 = 100000000000000000000; g = x as u8; }", "100000000000000000000 does not fit in u32"},
		{"書式の x", pre + "use console;\nfunction body():void { @printf(\"{:x}\", f); }", "`x` cannot be used for f32"},
		// 整数の式の `as` は整数の `as` と同じく自分の型の値を変換する (符号の混ざった結果はエラー。floating_init.fc)
		{"符号の混ざった式 → f32", pre + "var s:i8;\nfunction body():void { f = (g + s) as f32; }", "mixes u8 and i8"},
		{"符号の混ざった式 → 固定小数", pre + "var s:i8;\nvar w:i16f8;\nfunction body():void { w = (g + s) as i16f8; }", "mixes u8 and i8"},
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
