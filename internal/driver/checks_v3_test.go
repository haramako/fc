package driver

// doc/roadmap.md の v3 で「する」にした演算の制限と警告 (2026-09-26〜27 決定) のテスト。

import (
	"strings"
	"testing"
)

// TestAggregatePointerOps: struct・配列 (slice を含む) は == / != だけ、ポインタは ± 整数・ポインタ - ポインタ・順序比較・
// null の確認だけ (多バイトの整数として算術・順序比較・`!`・条件が通っていた)。
func TestAggregatePointerOps(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ body, msg string }{
		{"var a:P; var c:P; var z = a + c;", "cannot apply + to t.P"},
		{"var a:P; var c:P; var b = a < c;", "cannot apply < to t.P"},
		{"var a:P; if (a) { }", "cannot be used as a condition"},
		{"var a:P; var b = !a;", "cannot apply ! to t.P"},
		{"var x:[3]u8; var y = x + x;", "cannot apply + to [3]u8"},
		{"var s:[]u8; var t = s & s;", "cannot apply & to []u8"},
		{"var p:*u8; var q:*u8; var r = p * q;", "cannot apply * to a pointer"},
		{"var p:*u8; var r = ~p;", "cannot apply ~ to a pointer"},
		{"var p:*u8; var r = p | 1;", "cannot apply | to a pointer"},
	} {
		_, err := buildFiles(t, map[string]string{"t.fc": "#fc 3\nstruct P { x:u8; y:u8; }\nfunction main():void { " + c.body + " }\n"})
		if err == nil || !strings.Contains(err.Error(), c.msg) {
			t.Errorf("%s: got %v, want %q", c.body, err, c.msg)
		}
	}
	out, err := buildBothLevels(t, map[string]string{"t.fc": `#fc 3
use * from stdio;
struct P { x:u8; y:u8; }
var buf:[4]u8;
function main():void
{
	var a:P = {1, 2};
	var c:P = {1, 2};
	var p = &buf[0];
	var q = &buf[3];
	var n:u8 = 0;
	if (p < q) { n += 1; }
	if (p) { n += 2; }
	if (!p) { n += 100; }
	p += 1;
	printf(n, " ", q - p, " ", a == c, " ", a != c, "\n");
	exit(0);
}
`})
	if err != nil || out != "3 2 1 0\n" {
		t.Errorf("使える演算: got %q, %v", out, err)
	}
}

// TestBoolEqualityTruth: bool 同士の == / != は真理値で比べる (バイトの比較で、5 as bool と 3 as bool や true が違っていた)。
// f == true / true == f / f != true / f == false、比較の結果同士。
func TestBoolEqualityTruth(t *testing.T) {
	t.Parallel()
	out, err := buildBothLevels(t, map[string]string{"t.fc": `#fc 3
use * from stdio;
var a:u8; var b:u8;
function t(x:u8, y:u8):void @(noinline)
{
	var f = x as bool;
	var g = y as bool;
	var r1 = 0; var r2 = 0; var r3 = 0; var r4 = 0; var r5 = 0; var r6 = 0;
	if (f == g) { r1 = 1; }
	if (f == true) { r2 = 1; }
	if (f != g) { r3 = 1; }
	if (true == f) { r4 = 1; }
	var v = f == g;
	var w = f == true;
	var z = (x < 3) == (y < 3);
	if (f == false) { r5 = 1; }
	if (f != true) { r6 = 1; }
	printf(r1, r2, r3, r4, r5, r6, " ", v, w, z, "\n");
}
function main():void { a = 5; b = 3; t(a, b); t(0, 7); t(0, 0); exit(0); }
`})
	if err != nil || out != "110100 111\n001011 000\n100011 101\n" {
		t.Errorf("got %q, %v", out, err)
	}
}
