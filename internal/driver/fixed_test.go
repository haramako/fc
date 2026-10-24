package driver

// 固定小数 (`u8f8` など。fc 4 だけ。Agent/wiki/design/float-fixed.md) と型のない実数の定数のテスト。

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestFixedRun: 四則 (1・2 バイトは広い整数の命令、i16f16 は fclib/fixed.fc)・整数との * /・型のない定数・比較・`as`・定数の
// 畳み込み (testdata/fixedpoint.fc。値は手で計算して確かめた中身の整数) と、出力の書式 (fixedpoint_fmt.fc: 正確な値・`{:.N}` の
// 四捨五入・幅と 0 埋め) を、-O 0 / -O 2 とインタプリタで走らせて比べる。
func TestFixedRun(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"fixedpoint", "fixedpoint_fmt"} {
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

// TestFixedErrors: 固定小数・型のない実数の定数でエラーになる形。
func TestFixedErrors(t *testing.T) {
	t.Parallel()
	pre := "#fc 4\nvar g:u8;\nvar f:u8f8;\nvar h:i12f4;\nvar w:u16;\nvar d:u32;\n"
	cases := []struct{ name, src, want string }{
		{"fc 3 では型でない", "#fc 3\nvar x:u8f8;\n", "u8f8 is a type of fc 4 only"},
		{"fc 3 では実数のリテラルが無い", "#fc 3\nvar g:u8;\nfunction body():void { g = 1.5 as u8; }\n", "real literal 1.5 is fc 4 only"},
		{"整数と実数の定数", pre + "function body():void { g = g + 1.5; }", "1.5 is not an integer"},
		{"整数に実数の定数", pre + "function body():void { var y:u8 = 2.0; g = y; }", "2 is not an integer, so it cannot be u8"},
		{"型を書かない変数", pre + "function body():void { var x = 1.5; }", "the type of 1.5 is not decided"},
		{"整数 → 固定小数は as", pre + "function body():void { f = g; }", "cannot convert u8 to u8f8 implicitly"},
		{"固定小数 → 整数は as", pre + "function body():void { g = f; }", "cannot convert u8f8 to u8 implicitly"},
		{"分け方の違う固定小数", pre + "function body():void { f = f + h; }", "cannot apply + to u8f8 and i12f4"},
		{"整数の変数と +", pre + "function body():void { f = f + g; }", "cannot apply + to u8f8 and u8"},
		{"整数 ÷ 固定小数", pre + "function body():void { f = g / f; }", "cannot divide an integer by u8f8"},
		{"広い整数との *", pre + "function body():void { f = f * d; }", "narrowing"},
		{"入らない定数", pre + "function body():void { f = 300.0; }", "300 does not fit in u8f8"},
		{"負の定数", pre + "function body():void { f = -1.0; }", "-1 does not fit in u8f8"},
		{"0 で割る", pre + "function body():void { f = f / (0 as u8); }", "div by 0"},
		{"switch", pre + "function body():void { switch (f) { case 1: g = 1; } }", "cannot switch on u8f8"},
		{"ビット演算と定数", pre + "function body():void { f = f & 0x10; }", "is ambiguous"},
		{"実数の定数の %", pre + "const K = 1.5 % 2;", "% is not defined on real constants"},
		{"書式の x", pre + "use console;\nfunction body():void { @printf(\"{:x}\", f); }", "`x` cannot be used for u8f8"},
		{"整数に .N", pre + "use console;\nfunction body():void { @printf(\"{:.2}\", g); }", "`.N` (digits after the point) is for fixed-point"},
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
