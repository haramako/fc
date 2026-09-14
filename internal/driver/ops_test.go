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
