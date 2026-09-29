package driver

// 2026-09-27 の 2 回目の調査 (Agent/discussions/2026-09-20-v3-plan.md §10.7) で見つけたバグの回帰テスト。

import (
	"fmt"
	"strings"
	"testing"
)

// TestSurvey2Values: -O 0 と -O 2 で同じ、正しい値になるもの。
//   - 初期値なしの変数にループで代入した後の値 (SSA の φ(未定義, v) を v にしていた)
//   - i8 で i16 / u16 を初期化するときの符号拡張
//   - 2 バイトの要素の配列の 129 番目以降 (u8 の添字 * 2 の桁上がり)
//   - ポインタの負の添字・i8 のずれ
//   - 型付きの定数の畳み込みの折り返しと @bitcast の定数
//   - 16 ビットに収まらない定数・符号付きと広い定数の比較、@min / @max の定数
//   - ポインタの形の for-each は回す値を 1 回だけ評価する、文字列リテラルは終端の 0 を回らない
//   - const の表の無名関数
func TestSurvey2Values(t *testing.T) {
	t.Parallel()
	out, err := buildBothLevels(t, map[string]string{"t.fc": `#fc 3
use * from stdio;
struct O { vx:i8; }
struct S { buf:[3]u8; }
var a4:[4]u8;
var gv:i8; var G:[3]i8; var o:O;
var w200:[200]u16;
var b8:[8]u8; var w8:[8]u16;
var M:[2][4]u8; var A:[8]u8; var s1:S; var s2:S;
const CA:u8 = 200;
const K = @bitcast(i8, 254 as u8);
const OPS:[?]fn(u8):u8 = [->fn(x:u8):u8 { return x + 1; }];
function best():u8 @(noinline) { var b:u8; for (var i, x in a4) { if (x != 0) { b = x; } } return b; }
function wide(v:i8):i16 @(noinline) { var r:i16 = v; return r; }
function main():void
{
	a4[0] = 5; a4[1] = 6;
	var m:u8;
	for (var i = 0; i < 3; i++) { m = i; }
	printf(m, " ", best(), "\n");
	gv = -4; G[1] = -2; o.vx = -3;
	var j = 1;
	var c:i16 = gv; var y:i16 = G[j]; var a:i16 = o.vx; var d:u16 = gv;
	printf(c, " ", y, " ", a, " ", d, " ", wide(-5), "\n");
	for (var i in 0..200) { w200[i] = i as u16; }
	var k:u8 = 150;
	w200[k] = 7777;
	var s:u16 = 0;
	for (var x in w200) { s += x; }
	printf(w200[150], " ", w200[22], " ", s, "\n");
	for (var i in 0..8) { b8[i] = i + 10; w8[i] = i as u16 * 100; }
	var p = &b8[4]; var q = &w8[4]; var ni:i8 = -2;
	printf(p[-1], " ", p[ni], " ", *(p + ni), " ", q[-1], " ", q[ni], " ", *(q - 1), "\n");
	var yy:u16 = CA + 100;
	printf((200 as u8) + (100 as u8), " ", yy, " ", (1 as u8) << 9, " ", K, " ", K < 0, " ", @bitcast(u8, -1), "\n");
	var u:u8 = 2; var w:u16 = 1; var sw:i16 = 25536; var si:i8 = -10;
	printf(u > 65537, w == 65537, sw == -40000, sw > -40000, si < 300, " ", @min(si, 300), " ", @max(si, 300), "\n");
	for (var i in 0..8) { A[i] = i; }
	for (var i in 0..4) { M[0][i] = i; M[1][i] = 10 + i; }
	s1.buf[0] = 1; s1.buf[1] = 2; s1.buf[2] = 3; s2.buf[0] = 7;
	var r:u8 = 0;
	for (var e in &M[r]) { printf(*e); r = 1; }
	var lo:u8 = 1;
	for (var e in &A[lo..5]) { printf(*e); lo = 3; }
	var sp = &s1;
	for (var e in &sp.buf) { printf(*e); sp = &s2; }
	var nc = 0;
	for (var ch in "abc") { nc += 1; }
	printf(" ", nc, " ", OPS[0](1), "\n");
	exit(0);
}
`})
	want := "2 6\n-4 -2 -3 65532 -5\n7777 22 27527\n13 12 12 300 200 300\n44 44 0 -2 1 255\n00011 -10 300\n01231234123 3 2\n"
	if err != nil || out != want {
		t.Errorf("got %q, %v\nwant %q", out, err, want)
	}
}

// TestSurvey2Errors: エラーにする・警告するもの。
func TestSurvey2Errors(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ src, msg string }{
		{"function f(x:u8):u8 { L: switch (x) { case 1: loop { break L; } default: return 2; } }", "missing return"},
		{"function f(x:u8):u8 { switch (x) { case 1: if (x > 0) { break; } return 1; default: return 2; } }", "missing return"},
		{"const T:[3]u8 = [1, 2, 3];\nfunction f():void { var p:*const u8 = T; var q = p + 1; *q = 2; }", "read-only"},
		{"const M = [[1, 2, 3], [4, 5]];", "different lengths"},
		{"var g:[-1]u8;", "must not be negative"},
		{"var g:[70000]u8;", "too large"},
		{"var A:[4]u8;\nfunction f():void { for (var p in &A) { p++; } }", "cannot assign to for-each variable `p`"},
		{"function f(s:i8):i8 { return @min(s, 200); }", "200 does not fit in i8"},
		{"function f(u:u8):void { switch (u) { case 300: u = 1; } }", "case value 300 does not fit in u8"},
		{"function f(u:u8):void { switch (u) { case -1: u = 1; } }", "case value -1 does not fit in u8"},
		{"function f(s:i8):void { switch (s) { case 200: s = 1; } }", "case value 200 does not fit in i8"},
	} {
		_, err := buildFiles(t, map[string]string{"t.fc": "#fc 3\n" + c.src + "\nfunction main():void { }\n"})
		if err == nil || !strings.Contains(err.Error(), c.msg) {
			t.Errorf("%s: got %v, want %q", c.src, err, c.msg)
		}
	}
	if _, err := buildFiles(t, map[string]string{"a-b.fc": "#fc 3\nfunction main():void { }\n", "t.fc": "#fc 3\nuse a-b;\n"}); err == nil {
		t.Errorf("a-b.fc: エラーにならない")
	}
	// ローカル変数のアドレスを返す警告: 値渡しの引数、for-each の要素のポインタ、`&x + 1`
	_, res, err := buildFilesDefs(t, map[string]string{"t.fc": `#fc 3
use * from stdio;
struct S { a:u8; b:u8; }
var G:[4]u8;
function g(s:S):*u8 { return &s.b; }
function h(a:[4]u8):*u8 { return &a[0]; }
function k():*u8 { var loc:[4]u8; loc[0] = 1; for (var p in &loc) { return p; } return null; }
function m():*u8 { var x:u8 = 1; return &x + 1; }
function n1(p:*S):*u8 { return &p.b; }
function n2():*u8 { for (var p in &G) { return p; } return null; }
function main():void { var s:S; var a:[4]u8; g(s); h(a); k(); m(); n1(&s); n2(); exit(0); }
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
	if strings.Join(lines, ",") != "5,6,7,8" {
		t.Errorf("警告した行: %v", lines)
	}
}
