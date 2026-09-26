package driver

// 型のない整数定数 (リテラル・型を書かない const) は演算・比較で相手の型に合わせる (doc/v3_plan.md §10.2、2026-09-27 決定)。

import (
	"strings"
	"testing"
)

// TestLiteralAdaptArith: 相手の型に収まらない定数は、演算では相手の型に切り詰める (`x + -1` (x:u8) は u8。以前は i8 で
// 200 + -1 が -57 と表示された)。定数のほうが大きい型なら今までどおり広げ、型付きの定数 (`as`・型を書いた const) は普通の値と同じ。
func TestLiteralAdaptArith(t *testing.T) {
	t.Parallel()
	out, err := buildBothLevels(t, map[string]string{"t.fc": `#fc 3
use * from stdio;
const N = -1;
const M:i8 = -1;
var gx:u8; var gw:u16; var gs:i8;
function main():void
{
	gx = 200; gw = 40000; gs = -3;
	printf(gx + -1, " ", gx + N, " ", gw + -1, " ", gs - 128, " ", gx * 300, " ", gx + M, " ", gx + (1 as u16), "\n");
	exit(0);
}
`})
	if err != nil || out != "199 199 39999 125 60000 -57 201\n" {
		t.Errorf("got %q, %v", out, err)
	}
}

// TestLiteralAdaptCompare: 相手の型に収まらない定数との比較はエラー (`s < 200` (s:i8) は 200 が -56 に、`x == -1` (x:u8) は
// x == 255 になっていた)。収まるもの・広い定数・型付きの定数は通る。
func TestLiteralAdaptCompare(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ body, msg string }{
		{"var s:i8 = g; if (s < 200) { }", "200 does not fit in i8"},
		{"var x:u8 = g; if (x == -1) { }", "-1 does not fit in u8"},
		{"var x:u8 = g; var b = x != -1;", "-1 does not fit in u8"},
		{"var x:u8 = g; if (-1 < x) { }", "-1 does not fit in u8"},
		{"var x:u8 = g; if (x >= -5) { }", "-5 does not fit in u8"},
		{"var w:u16 = g; if (w == -1) { }", "-1 does not fit in u16"},
		{"var x:u8 = g; if (x == N) { }", "`N` does not fit in u8"},
	} {
		_, err := buildFiles(t, map[string]string{"t.fc": "#fc 3\nconst N = -1;\nvar g:u8;\nfunction main():void { " + c.body + " }\n"})
		if err == nil || !strings.Contains(err.Error(), c.msg) {
			t.Errorf("%s: got %v, want %q", c.body, err, c.msg)
		}
	}
	out, err := buildBothLevels(t, map[string]string{"t.fc": `#fc 3
use * from stdio;
const M:i8 = -1;
var gx:u8; var gs:i8; var gw:u16;
function main():void
{
	gx = 255; gs = -3; gw = 300;
	printf(gs < 0, gs < 64, gx == 255, gx == M, gw > 256, 0 < gs, "\n");
	exit(0);
}
`})
	if err != nil || out != "111110\n" {
		t.Errorf("got %q, %v", out, err)
	}
}
