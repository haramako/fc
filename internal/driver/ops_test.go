package driver

// v2 で足した演算子 (複合代入、~) と switch の case の規則。

import (
	"strings"
	"testing"
)

func TestCompoundAssignAndBitNot(t *testing.T) {
	t.Parallel()
	out := runEmu(t, `var g:int16;
struct P { v:int; }
var ps:[2]P;
function main():void
{
	var a = 0b1010;
	a |= 0b0101;
	var b = a;
	a &= 0b0110;
	var c = a;
	a ^= 0xff;
	var d = a;
	a <<= 2;
	var e = a;
	a >>= 5;
	var f = a;
	a = 7;
	a *= 6;
	var h = a;
	a /= 4;
	var i = a;
	a %= 4;
	var j = a;
	var k = ~a;
	printf(b, " ", c, " ", d, " ", e, " ", f, " ", h, " ", i, " ", j, " ", k, "\n");
	rest();
	exit(0);
}
function rest():void
{
	g = 1000;
	g -= 1;
	g <<= 1;
	g |= 1;
	var p = &ps[1];
	p.v = 3;
	p.v |= 4;
	ps[0].v = 1;
	ps[0].v <<= 3;
	printf(g, " ", p.v, " ", ps[0].v, " ", ~0x0f & 0xff, "\n");
}
`)
	want := "15 6 249 228 7 42 10 2 253\n1999 7 8 240\n"
	if out != want {
		t.Errorf("got %q\nwant %q", out, want)
	}
}

func TestSwitchCases(t *testing.T) {
	t.Parallel()
	// 空の case は「何もしない」(fall through しない)。default も空でよい
	out := runEmu(t, `function f(x:int):int
{
	switch (x) {
	case 0:
	case 1, 2:
		return 10;
	case 3:
		return 30;
	default:
	}
	return 99;
}
function main():void
{
	printf(f(0), " ", f(1), " ", f(2), " ", f(3), " ", f(4), "\n");
	exit(0);
}
`)
	if out != "99 10 10 30 99\n" {
		t.Errorf("got %q", out)
	}

	got := compileErr(t, "function f(x:int):void { switch (x) { case 1: break; case 2, 1: break; } }\n")
	if !strings.Contains(got, "duplicate case value 1") {
		t.Errorf("重複 case: %q", got)
	}
	got = compileErr(t, "const A = 5;\nfunction f(x:int):void { switch (x) { case A: break; case 5: break; } }\n")
	if !strings.Contains(got, "duplicate case value 5") {
		t.Errorf("重複 case (const): %q", got)
	}
}

// TestDivMod16: 16 ビットの除算・剰余 (__div_16s / __mod_16 / __mod_16s と 2 のべき乗の算術シフト)。
// 丸めは 8 ビットと同じ床除算 (商は負の無限大方向、余りの符号は除数に合わせる)。
// bench/math16.fc を書いたときに __mod_16 が rts だけの stub、符号付き 16 ビットが符号無しの __div_16 を呼んでいた、
// 2 のべき乗の符号付き除算が `cmp $80` (ゼロページ参照) を出していた、の 3 つが見つかった。
func TestDivMod16(t *testing.T) {
	t.Parallel()
	out := runEmu(t, `var u:int16;
var s:sint16;
var a:sint8;
function main():void
{
	u = 65236;
	printf(u / 7, " ", u % 7, " ", u / 4, " ", u % 8, "\n");
	s = -300;
	printf(s / 4, " ", s / 3, " ", s % 4, " ", s % 3, " ", s % -7, " ", s / -7, "\n");
	s = 300;
	printf(s / 4, " ", s / 7, " ", s % 7, " ", s / -7, " ", s % -7, "\n");
	s = -32768;
	printf(s / 3, " ", s % 3, "\n");
	a = -100;
	printf(a / 7, " ", a % 7, " ", a / 4, " ", a % 4, "\n");
	exit(0);
}
`)
	// printf は 8 ビット値も 16 ビットに符号拡張して表示する (a / 7 = -15 → 65521)
	want := "9319 3 16309 4\n65461 65436 0 0 65530 42\n75 42 6 65493 65535\n54613 1\n65521 5 65511 0\n"
	if out != want {
		t.Errorf("got %q\nwant %q", out, want)
	}
}

// TestCompoundAssignCallOnce: 複合代入の左辺に関数呼び出しがあっても 1 回しか呼ばない (`a[f()] += 8`、`g().v += 5`)。
// 左辺が単純なときは index を 2 回計算する脱糖のまま (index_pget / index_pset に融合される)。
func TestCompoundAssignCallOnce(t *testing.T) {
	t.Parallel()
	out := runEmu(t, `struct P { v:int; }
var a:[4]int;
var ps:[2]P;
var calls:int;
function f():int { calls++; return 1; }
function g():*P { calls++; return &ps[1]; }
function main():void
{
	a[f()] += 8;
	a[f()] |= 1;
	g().v += 5;
	printf(calls, " ", a[1], " ", ps[1].v, "\n");
	exit(0);
}
`)
	if want := "3 9 5\n"; out != want {
		t.Errorf("got %q\nwant %q", out, want)
	}
}

// TestPointerSelfRead: 読み先がポインタ自身 (`p = p.next`、`q = q[1]`、`pq = *pq`) でも壊れない
// (ゼロページのポインタは (p),y で直接読むが、下位バイトを書いてから上位バイトを読むと壊れるので reg 経由に戻す)。
func TestPointerSelfRead(t *testing.T) {
	t.Parallel()
	out := runEmu(t, `struct Node { value:int; next:*Node; }
var nodes:[3]Node;
var table:[3]*Node;
function main():void
{
	nodes[0] = {1, &nodes[1]};
	nodes[1] = {2, &nodes[2]};
	nodes[2] = {3, null};
	table[0] = &nodes[2];
	table[1] = &nodes[1];
	var s = 0;
	var p = &nodes[0];
	while (p != null) {
		s += p.value;
		p = p.next;
	}
	var q = table as **Node;
	q = bitcast<**Node>(q[1]);
	var pq = &table[0];
	pq = bitcast<**Node>(*pq);
	printf(s, " ", bitcast<*Node>(q).value, " ", bitcast<*Node>(pq).value, "\n");
	exit(0);
}
`)
	if want := "6 2 3\n"; out != want {
		t.Errorf("got %q\nwant %q", out, want)
	}
}
