package driver

// Agent/wiki/plans/roadmap.md の v3 で 2026-09-27 に「する」にした文法・検査 (return 忘れの検査の拡張、ローカル変数のアドレスを返す警告、
// for-each、case の範囲と `..=`) のテスト。

import (
	"fmt"
	"strings"
	"testing"
)

// TestMissingReturnWider: `while (true)` の無限ループと、@if の選ばれた側の return を終端文と認める。default の無い switch は
// enum の全メンバーを並べても終端文にしない (範囲外の値で落ちうる) が、文言で default を案内する。
func TestMissingReturnWider(t *testing.T) {
	t.Parallel()
	out, err := buildBothLevels(t, map[string]string{"t.fc": `#fc 3
use * from stdio;
function a(x:u8):u8 { while (true) { if (x > 3) { return x; } x++; } }
function b(x:u8):u8 { L: for (;true;) { if (x > 5) { return x; } x++; } }
function c():u8 { @if (true) { return 1; } else { return 2; } }
function d():u8 { @if (false) { } else @if (true) { return 4; } }
function main():void { printf(a(1), " ", b(1), " ", c(), " ", d(), "\n"); exit(0); }
`})
	if err != nil || out != "4 6 1 4\n" {
		t.Errorf("got %q, %v", out, err)
	}
	for _, c := range []struct{ src, msg string }{
		{"function f():u8 { @if (false) { return 1; } }", "missing return at end of function f"},
		{"function f(x:u8):u8 { while (true) { if (x > 3) { break; } x++; } }", "missing return at end of function f"},
		{"enum D { Up, Down }\nfunction f(d:D):u8 { switch (d) { case .Up: return 1; case .Down: return 2; } }", "the switch has no default"},
	} {
		_, err := buildFiles(t, map[string]string{"t.fc": "#fc 3\n" + c.src + "\nfunction main():void { }\n"})
		if err == nil || !strings.Contains(err.Error(), c.msg) {
			t.Errorf("%s: got %v, want %q", c.src, err, c.msg)
		}
	}
}

// TestReturnLocalAddr: return の式そのものがローカル変数の中を指すアドレスなら警告する (静的フレームは後の呼び出しで上書きされる)。
// 引数・グローバル・const の表・ポインタの先・変数を経由する形は警告しない。
func TestReturnLocalAddr(t *testing.T) {
	t.Parallel()
	_, res, err := buildFilesDefs(t, map[string]string{"t.fc": `#fc 3
use * from stdio;
struct S { x:u8; buf:[4]u8; }
var g:[4]u8;
const C:[3]u8 = [1, 2, 3];
function f1():*u8 { var x:u8 = 1; return &x; }
function f2():*u8 { var a:[4]u8; a[0] = 1; return a; }
function f3():[]u8 { var a:[4]u8; a[0] = 1; return a[1..3]; }
function f4():*u8 { var s:S; s.x = 1; return &s.x; }
function f5():*u8 { var s:S; s.x = 1; return s.buf; }
function f6():*u8 { var a:[4]u8; a[0] = 1; return (&a[2]); }
function f7():[]u8 { var a:[4]u8; a[0] = 1; return a; }
function n1(p:*u8):*u8 { return &p[1]; }
function n2(x:u8):*u8 { return g; }
function n3():*const u8 { return C; }
function n4(p:*S):*u8 { return &p.x; }
function n5():*u8 { var x:u8 = 1; var p = &x; return p; }
function n6(x:u8):u8 { var a:[4]u8; a[0] = x; return a[0]; }
function main():void { f1(); f2(); f3(); f4(); f5(); f6(); f7(); n1(&g[0]); n2(1); n3(); n4(null); n5(); n6(1); exit(0); }
`}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for _, w := range res.Warnings {
		if strings.Contains(w.Msg, "returning the address of local variable") {
			lines = append(lines, fmt.Sprint(w.Pos.Line))
		}
	}
	if strings.Join(lines, ",") != "6,7,8,9,10,11,12" {
		t.Errorf("警告した行: %v", lines)
	}
}

// TestForIn: for-each (配列・slice の値、添字付き、配列へのポインタの要素のポインタ、範囲 `..` / `..=`、256 回のループ、
// continue / break、実行時の終わり、型を書いた範囲)。
func TestForIn(t *testing.T) {
	t.Parallel()
	out, err := buildBothLevels(t, map[string]string{"t.fc": `#fc 3
use * from stdio;
struct E { hp:u8; x:u8; }
var A:[4]u8;
var es:[3]E;
var big:[256]u8;
var W:[300]u8;
function sum(s:[]u8):u16 { var t:u16 = 0; for (var x in s) { t += x; } return t; }
function hit(p:*[3]E):void { for (var e in p) { e.hp -= 1; } }
function main():void
{
	for (var i in 0..4) { A[i] = i * 10; }
	for (var x in A) { printf(x, " "); }
	printf("\n");
	for (var i, x in A) { printf(i, ":", x, " "); }
	printf("\n");
	for (var i in 0..=3) { es[i].hp = i + 5; }
	for (var e in &es) { e.hp += 1; e.x = e.hp * 2; }
	hit(&es);
	for (var e in es) { printf(e.hp, ",", e.x, " "); }
	printf("\n");
	var n:u16 = 0;
	for (var i in 0..256) { n += 1; }
	var m:u16 = 0;
	for (var i in 0..=255) { m += 1; }
	var k:u16 = 0;
	for (var i, x in big) { k += 1; }
	var w:u16 = 0;
	for (var i, x in W) { w += 1; }
	printf(n, " ", m, " ", k, " ", w, " ", sum(A), " ", sum(A[1..=2]), "\n");
	var c = 0;
	for (var i in 0..10) { if (i == 3) { continue; } if (i == 6) { break; } c += i; }
	var hi:u8 = 255;
	var q:u16 = 0;
	for (var i in 250..=hi) { q += 1; }
	var z:u16 = 0;
	for (var i in 5..hi - 250) { z += 1; }
	for (var i:i8 in -3..3) { printf(i, " "); }
	var y:u16 = 0;
	L: for (var i:u16 in 0..300) { for (var j in 0..3) { if (i == 299) { break L; } y += 1; } }
	printf("\n", c, " ", q, " ", z, " ", y, "\n");
	exit(0);
}
`})
	want := "0 10 20 30 \n0:0 1:10 2:20 3:30 \n5,12 6,14 7,16 \n256 256 256 300 60 30\n-3 -2 -1 0 1 2 \n12 6 0 897\n"
	if err != nil || out != want {
		t.Errorf("got %q, %v\nwant %q", out, err, want)
	}
	for _, c := range []struct{ body, msg string }{
		{"for (var x in A) { x = 1; }", "cannot assign to for-each variable `x`"},
		{"for (var i, x in A) { i += 1; }", "cannot assign to for-each variable `i`"},
		{"for (var i in 0..3) { i++; }", "cannot assign to for-each variable `i`"},
		{"for (var i, x in 0..3) { }", "a range gives one variable"},
		{"for (var x:u16 in A) { }", "a type can be written only for a range"},
		{"var n:u8 = 3; for (var x in n) { }", "cannot iterate over u8"},
		{"var r = 1..3;", "parse error"},
		{"for (var p in &C) { *p = 5; }", "cannot assign through a read-only pointer"},
		{"var q = &C; for (var p in q) { *p = 5; }", "cannot assign through a read-only pointer"},
	} {
		_, err := buildFiles(t, map[string]string{"t.fc": "#fc 3\nvar A:[4]u8;\nconst C:[3]u8 = [1, 2, 3];\nfunction main():void { " + c.body + " }\n"})
		if err == nil || !strings.Contains(err.Error(), c.msg) {
			t.Errorf("%s: got %v, want %q", c.body, err, c.msg)
		}
	}
	// fc 2 では in は名前のまま
	if _, err := buildFiles(t, map[string]string{"t.fc": "#fc 2\nuse * from stdio;\nvar in:int;\nfunction main():void { in = 1; exit(0); }\n"}); err != nil {
		t.Errorf("fc 2 の in: %v", err)
	}
}

// TestCaseRange: case の範囲 `lo..hi` (hi を含まない) / `lo..=hi` (含む)。比較の連鎖とジャンプテーブル、符号付き、enum、u16。
// 空の範囲・重なり・型に収まらない範囲はエラー。
func TestCaseRange(t *testing.T) {
	t.Parallel()
	out, err := buildBothLevels(t, map[string]string{"t.fc": `#fc 3
use * from stdio;
enum D { N, E, S, W }
function k(x:u8):u8 {
	switch (x) {
	case 0..4: return 1;
	case 4..=9, 20: return 2;
	case 10..20: return 3;
	case 100..=255: return 4;
	default: return 9;
	}
}
function tb(x:u8):u8 { switch (x) { case 0..=1: return 1; case 2..4: return 2; case 4: return 3; case 5..=7: return 4; default: return 0; } }
function s(x:i8):u8 { switch (x) { case -5..0: return 1; case 0..=3: return 2; default: return 0; } }
function e(d:D):u8 { switch (d) { case .N..=.E: return 1; default: return 2; } }
function w(x:u16):u8 { switch (x) { case 1000..2000: return 1; case 5: return 2; default: return 0; } }
function main():void
{
	for (var x in [0 as u8, 3, 4, 9, 10, 19, 20, 21, 99, 100, 255]) { printf(k(x)); }
	printf(" ");
	for (var x in 0..9) { printf(tb(x)); }
	printf(" ");
	for (var x in [-6 as i8, -5, -1, 0, 3, 4]) { printf(s(x)); }
	printf(" ", e(.N), e(.E), e(.S), " ", w(999), w(1000), w(1999), w(2000), w(5), "\n");
	exit(0);
}
`})
	if err != nil || out != "11223329944 112234440 011220 112 01102\n" {
		t.Errorf("got %q, %v", out, err)
	}
	for _, c := range []struct{ body, msg string }{
		{"case 3..3: x = 1;", "case range 3..3 is empty"},
		{"case 5..=4: x = 1;", "case range 5..=4 is empty"},
		{"case 0..5: x = 1; case 4: x = 2;", "duplicate case value 4"},
		{"case 0..5: x = 1; case 3..=8: x = 2;", "duplicate case value 3"},
		{"case 200..=300: x = 1;", "does not fit in u8"},
		{"case 0..x: x = 1;", "must be an integer constant"},
	} {
		_, err := buildFiles(t, map[string]string{"t.fc": "#fc 3\nfunction main():void { var x:u8 = 0; switch (x) { " + c.body + " } }\n"})
		if err == nil || !strings.Contains(err.Error(), c.msg) {
			t.Errorf("%s: got %v, want %q", c.body, err, c.msg)
		}
	}
	if _, err := buildFiles(t, map[string]string{"t.fc": "#fc 2\nfunction main():void { var x = 0; switch (x) { case 0..3: x = 1; } }\n"}); err == nil || !strings.Contains(err.Error(), "fc 3 syntax") {
		t.Errorf("fc 2 の case の範囲: %v", err)
	}
}
