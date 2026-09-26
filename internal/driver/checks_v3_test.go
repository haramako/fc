package driver

// doc/roadmap.md の v3 で「する」にした演算の制限と警告 (2026-09-26〜27 決定) のテスト。

import (
	"fmt"
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

// TestUninitializedLocal: 代入する前に読みうるローカル変数を警告する (静的フレームが重なるので前の関数の値が見え、-O で結果も
// 変わっていた)。両方の枝で代入・アドレスを渡した変数・必ず通る loop の中の代入・配列は警告しない。
func TestUninitializedLocal(t *testing.T) {
	t.Parallel()
	_, res, err := buildFilesDefs(t, map[string]string{"t.fc": `#fc 3
use * from stdio;
function set(p:*u8):void { *p = 3; }
function f(c:bool):u8
{
	var y:u8;
	y += 1;
	var a:u8;
	if (c) { a = 1; }
	var b:u8;
	if (c) { b = 1; } else { b = 2; }
	var d:u8;
	set(&d);
	var g:u8;
	loop { g = 5; break; }
	var k:[4]u8;
	k[0] = 1;
	var m:u8 = 4;
	return y + a + b + d + g + k[1] + m;
}
function main():void { printf(f(true), "\n"); exit(0); }
`}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, w := range res.Warnings {
		if strings.Contains(w.Msg, "may be read before it is assigned") {
			got = append(got, w.Msg[1:strings.Index(w.Msg[1:], "`")+1])
		}
	}
	if strings.Join(got, ",") != "y,a" {
		t.Errorf("警告した変数: %v (want y,a)", got)
	}
}

// TestAlwaysSameComparison: 符号なしの値と範囲外の定数の比較は、型の範囲だけで結果が決まるので警告する (`i < 256` の u8 の
// 無限ループ、`hp - dmg < 0` は常に偽)。範囲内の比較、符号付きの値、負の定数 (`x == -1` は今の規則では x == 255) は警告しない。
func TestAlwaysSameComparison(t *testing.T) {
	t.Parallel()
	_, res, err := buildFilesDefs(t, map[string]string{"t.fc": `#fc 3
use * from stdio;
var buf:[256]u8;
var hp:u8; var dmg:u8; var w:u16; var s:i8; var x:u8;
function main():void
{
	for (var i = 0; i < 300; i++) { break; }
	for (var j = 0; j < @len(buf); j++) { break; }
	for (var k:u8 = 0; k <= 255; k++) { break; }
	if (hp - dmg < 0) { }
	if (x == 300) { }
	if (w < 65536) { }
	if (x < 200) { }
	if (x >= 1) { }
	if (s < 100) { }
	if (w < 300) { }
	if (x == -1) { }
	exit(0);
}
`}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var lines []int
	for _, w := range res.Warnings {
		if strings.Contains(w.Msg, "this comparison always has the same result") {
			lines = append(lines, w.Pos.Line)
		}
	}
	if fmt.Sprint(lines) != "[7 8 9 10 11 12]" {
		t.Errorf("警告した行: %v", lines)
	}
}
