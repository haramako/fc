package driver

import (
	"strings"
	"testing"

	"github.com/haramako/fc/internal/syntax"
)

// TestCondExpr: fc 4 の条件式 `c ? x : y` (sema/cond.go)。選ばれたほうだけを評価する、枝の型の決め方 (互換型・型のない定数は相手に
// 合わせる・文脈の型)、A1 で広げる、`.A`・null・slice の文脈、定数の畳み込み、条件の文脈、return と代入の枝ごとの書き込み。
func TestCondExpr(t *testing.T) {
	t.Parallel()
	out, err := buildBothLevels(t, map[string]string{"t.fc": `#fc 4
use console;
enum Dir { Up, Down }
const DEBUG = false;
const K = DEBUG ? 3 : 400;
const N:u8 = true ? 7 : 9;
var calls:u8;
var g:i16;
function f(x:u8):u8 { calls += 1; return x; }
function pick(c:bool, a:u8, b:u8):u8 { return c ? a : b; }
function sgn(x:i8):i8 { return x < 0 ? -1 : x > 0 ? 1 : 0; }
function wide(c:bool, a:u8, b:u8):u16 { return c ? a + b : 1000; }
function mixed(c:bool, a:i8, b:u8):i16 { return c ? a : b; }
function dir(c:bool):Dir { return c ? .Up : .Down; }
function first(s:[]const u8):u8 { return s[0]; }
function main():void
{
	var a:u8 = 5;
	var b = a > 3 ? f(1) : f(2);
	var p:*u8 = null;
	var q:*u8 = p == null ? &a : p;
	var r:*u8 = a == 0 ? null : &a;
	var s:[]const u8 = a > 3 ? "ab" : "cde";
	g = a > 3 ? -2 : a;
	var arr:[3]u8;
	arr[a > 3 ? 1 : 2] = a < 3 ? 10 : 20;
	var w:u16 = a > 3 ? a * 100 : 0;
	if (a > 3 ? a == 5 : false) { @printf("cond "); }
	if (DEBUG ? true : a != 5) { @printf("bad "); }
	@printf("{} {} {} {} {} {} {}\n", b, calls, K, N, *q, *r, @len(s));
	@printf("{} {} {} {} {} {} {}\n", pick(true, 1, 2), pick(false, 1, 2), sgn(-5), sgn(0), sgn(9), wide(true, 200, 100), wide(false, 1, 1));
	@printf("{} {} {} {} {} {}\n", mixed(true, -1, 200), mixed(false, -1, 200), dir(false) == .Down, g, arr[1], w);
	@printf("{} {}\n", first(a > 3 ? "x" : "yz"), (a > 3 ? 250 : 0) + 10);
	console.exit(0);
}
`})
	want := "cond 1 1 400 7 5 5 2\n1 2 -1 0 1 300 1000\n-1 200 true -2 20 500\n120 4\n"
	if err != nil || out != want {
		t.Errorf("got %q, %v\nwant %q", out, err, want)
	}
}

// TestCondExprErrors: 条件式の型の誤り。
func TestCondExprErrors(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ body, msg string }{
		{"var x:i8 = 0; var y = a > 0 ? x : 200;", "200 does not fit in i8, the type of the other branch of `?:`"},
		{"var y:u8 = a > 0 ? -1 : 2;", "-1 does not fit in u8 (a branch of `?:`"},
		{"var p:*u8 = null; var y = a > 0 ? p : a;", "the branches of `?:` have incompatible types"},
		{"var y = a > 0 ? 1 : 70000;", "do not fit in one integer type"},
		{"var y:u8 = a > 0 ? a : 300 as u16;", "narrowing"},
		{"var y = a > 0 ? 1;", "parse error"},
	} {
		src := "#fc 4\nfunction main():void\n{\n\tvar a:u8 = 1;\n\t" + c.body + "\n}\n"
		_, err := buildFiles(t, map[string]string{"t.fc": src})
		if err == nil || !strings.Contains(err.Error(), c.msg) {
			t.Errorf("%s: got %v, want /%s/", c.body, err, c.msg)
		}
	}
	// fc 3 では使えない
	if _, err := syntax.Parse([]byte("#fc 3\nconst A = true ? 1 : 2;\n"), "t.fc"); err == nil || !strings.Contains(err.Error(), "fc 4 syntax") {
		t.Errorf("fc 3: got %v", err)
	}
}

// TestDoWhile: fc 4 の `do stmt while (cond);`。本体を先に 1 回実行する、continue は条件の判定へ、break・ラベル、return 忘れの判定
// (本体が終端文なら終端文)、fc 3 では do は名前。
func TestDoWhile(t *testing.T) {
	t.Parallel()
	out, err := buildBothLevels(t, map[string]string{"t.fc": `#fc 4
use console;
function once(x:u8):u8
{
	do {
		return x + 1;
	} while (x > 0);
}
function forever(x:u8):u8
{
	do {
		if (x > 10) { return x; }
		x += 3;
	} while (true);
}
function main():void
{
	var n:u8 = 0;
	var i:u8 = 0;
	do {
		n += i;
		i++;
	} while (i < 5);
	var j:u8 = 0;
	do j++; while (false);
	var k:u8 = 0;
	var odd:u8 = 0;
	outer: do {
		k++;
		if (k % 2 == 0) { continue; }
		odd++;
		do {
			if (k == 7) { break outer; }
		} while (false);
	} while (k < 100);
	@printf("{} {} {} {} {} {}\n", n, j, k, odd, once(0), forever(1));
	console.exit(0);
}
`})
	if want := "10 1 7 4 1 13\n"; err != nil || out != want {
		t.Errorf("got %q, %v, want %q", out, err, want)
	}
	for _, c := range []struct{ src, msg string }{
		{"function f(x:u8):u8 { do { if (x > 0) { break; } return 1; } while (true); }", "missing return"},
		{"function f(x:u8):u8 { do { if (x > 0) { continue; } return 1; } while (x < 3); }", "missing return"},
		{"function f():void { do { var v:u8 = 1; } while (v > 0); }", "v not found"},
	} {
		_, err := buildFiles(t, map[string]string{"t.fc": "#fc 4\n" + c.src + "\nfunction main():void {}\n"})
		if err == nil || !strings.Contains(err.Error(), c.msg) {
			t.Errorf("%s: got %v, want /%s/", c.src, err, c.msg)
		}
	}
	// fc 3 では do は名前
	if _, err := buildFiles(t, map[string]string{"t.fc": "#fc 3\nvar do:u8;\nfunction main():void { do = 1; }\n"}); err != nil {
		t.Errorf("fc 3 do: %v", err)
	}
}

// TestCondFormat: fcc fmt の条件式と do-while。
func TestCondFormat(t *testing.T) {
	t.Parallel()
	src := "#fc 4\nfunction f(c:bool, a:u8):u8\n{\n\tvar x:u8 = c ? a : 3;\n\tdo {\n\t\tx++;\n\t} while (x < 10);\n\tdo\n\t\tx--;\n\twhile (x > 5);\n\treturn c ? x : a > 2 ? 1 : 2;\n}\n"
	out, err := syntax.Format([]byte(src), "t.fc")
	if err != nil || string(out) != src {
		t.Errorf("got %q, %v", out, err)
	}
}
