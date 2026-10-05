package driver

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/haramako/fc/internal/diag"
)

// TestAnonStructLitArg: 型名を省いた struct リテラルを引数に (引数の型で決まる)、他モジュールの型の struct リテラル
// `pm.Pt{…}`、型の無い var の `{…}` は型を書くよう案内するエラー。
func TestAnonStructLitArg(t *testing.T) {
	t.Parallel()
	out, err := buildBothLevels(t, map[string]string{
		"pm.fc": "#fc 4\npublic struct Pt { x:u8; y:u8; }\n",
		"t.fc": `#fc 4
use console;
use pm;
struct P { x:u8; y:u8; }
function add(a:P, b:P):u8 { return a.x + b.x + a.y + b.y; }
function sum(p:pm.Pt, q:*P):u8 { return p.x + p.y + q.x; }
function main():void
{
	var p = P{x:3, y:4};
	var m = pm.Pt{x:5, y:6};
	var n:pm.Pt = {7, 8};
	@printf("{} {} {} {}\n", add({1, 2}, p), add(p, {x:10, y:20},), sum({1, 1}, &p), m.y + n.x);
	console.exit(0);
}
`})
	if want := "10 37 5 13\n"; err != nil || out != want {
		t.Errorf("got %q, %v, want %q", out, err, want)
	}
	for _, c := range []struct{ src, msg string }{
		{"function main():void { var s = {1, 2}; }", "`s`: a struct literal without a type name needs the variable's type"},
		{"function f(a:u8):void {}\nfunction main():void { f({1, 2}); }", "struct literal cannot be used as u8"},
	} {
		_, err := buildFiles(t, map[string]string{"t.fc": "#fc 4\n" + c.src + "\n"})
		if err == nil || !strings.Contains(err.Error(), c.msg) {
			t.Errorf("%s: got %v, want /%s/", c.src, err, c.msg)
		}
	}
}

// TestNamedConstInTable: 名前付きの const の struct・配列を別の const の表の要素・struct のフィールドに (中身の写し)、
// const の宣言の中で定数の表の整数の要素・フィールドは定数 (`MONS[1].hp`)、定数でない要素の診断はその要素の位置。
func TestNamedConstInTable(t *testing.T) {
	t.Parallel()
	out, err := buildBothLevels(t, map[string]string{"t.fc": `#fc 4
use console;
struct P { x:u8; y:i8; }
struct L { a:P; row:[3]u8; }
struct M { hp:u8; pos:P; }
const ORIGIN = P{x:5, y:6};
const ROW:[3]u8 = [7, 8, 9];
const PS:[2]P = [ORIGIN, {1, 2}];
const A = [ROW, [4, 5, 6]];
const Q = [ORIGIN, ORIGIN];
const LS:[1]L = [{ORIGIN, ROW}];
const LZ = L{a: ORIGIN, row: ROW};
const MONS:[2]M = [{hp:10, pos:{1, -2}}, {hp:20, pos:{3, -4}}];
const B:[2][3]u8 = [ROW, [4, 5, 6]];
const H = MONS[1].hp;
const Y = MONS[1].pos.y;
const OX = ORIGIN.x;
const B12 = B[1][2];
var arr:[H]u8;
function main():void
{
	@printf("{} {} {} {} {} {} {} {}\n", PS[0].x, PS[1].y, A[0][1], A[1][2], Q[1].y, LS[0].a.x, LS[0].row[2], LZ.row[0]);
	@printf("{} {} {} {} {}\n", H, Y, OX, B12, @len(arr));
	console.exit(0);
}
`})
	if want := "5 2 8 6 6 5 9 7\n20 -4 5 6 20\n"; err != nil || out != want {
		t.Errorf("got %q, %v, want %q", out, err, want)
	}
	for _, c := range []struct{ src, msg string }{
		{"var v:u8;\nconst T:[3]u8 = [1,\n   v, 3];", "4:4 `v` is a variable"},
		{"struct P { x:u8; y:u8; }\nvar v:u8;\nconst T:[2]P = [{1, 2},\n {3, v}];", "5:6 `v` is a variable"},
		{"var v:u8;\nconst S = [1, v + 1];", "3:15 constant value required"},
	} {
		_, err := buildFiles(t, map[string]string{"t.fc": "#fc 4\n" + c.src + "\nfunction main():void {}\n"})
		var ce *diag.Error
		if !errors.As(err, &ce) || !strings.HasPrefix(fmt.Sprintf("%d:%d %s", ce.Pos.Line, ce.Pos.Col, ce.Msg), c.msg) {
			t.Errorf("%s: got %v (%+v), want /%s/", c.src, err, ce, c.msg)
		}
	}
}

// TestConstGlobalAddress: グローバル変数 (の定数の添字の要素・フィールド) のアドレスを const の表・struct のフィールド・
// ポインタの const に。データは `.word sym+N`。ローカル変数・範囲外の添字はエラー。
func TestConstGlobalAddress(t *testing.T) {
	t.Parallel()
	files := map[string]string{"t.fc": `#fc 4
use console;
struct P { x:u8; y:i8; }
struct M { hp:u8; pos:P; }
var gp:u8;
var gs:M;
var ga:[4]M;
const PP:*u8 = &gp;
const T:[3]*u8 = [&gp, &gs.pos.x, &ga[2].hp];
const TY = [&gs.pos.y, &ga[3].pos.y];
struct Item { name:*u8; val:*i8; }
const ITEMS:[1]Item = [{name: &gp, val: &gs.pos.y}];
function main():void
{
	*PP = 3; *T[1] = 4; *T[2] = 9; *TY[0] = -7; *TY[1] = -8; *ITEMS[0].name += 1;
	@printf("{} {} {} {} {} {}\n", gp, gs.pos.x, ga[2].hp, gs.pos.y, *ITEMS[0].val, ga[3].pos.y);
	console.exit(0);
}
`}
	out, err := buildBothLevels(t, files)
	if want := "4 4 9 -7 -7 -8\n"; err != nil || out != want {
		t.Errorf("got %q, %v, want %q", out, err, want)
	}
	if asm := compileAsmFiles(t, files); !strings.Contains(asm, "+7") {
		t.Errorf("&ga[2].hp が sym+7 になっていない:\n%s", asm)
	}
	for _, c := range []struct{ src, msg string }{
		{"var g:[4]u8;\nconst T:[1]*u8 = [&g[4]];", "index 4 is out of the array (length 4)"},
		{"var g:[4]u8;\nconst T:[1]*i16 = [&g[1]];", "cannot assign"},
	} {
		_, err := buildFiles(t, map[string]string{"t.fc": "#fc 4\n" + c.src + "\nfunction main():void {}\n"})
		if err == nil || !strings.Contains(err.Error(), c.msg) {
			t.Errorf("%s: got %v, want /%s/", c.src, err, c.msg)
		}
	}
}

// TestArrayRowValue: 2 次元配列の行・ポインタの先の配列のフィールドは、値の文脈 (型の無い var・配列への代入・初期化) では
// 配列の写し (以前は `var b = a[1]` が *u8 になり、`var c:[2]u8 = a[1]` はポインタの 2 バイトを写していた)。ポインタを
// 受け取る引数・@len はそのまま。
func TestArrayRowValue(t *testing.T) {
	t.Parallel()
	out, err := buildBothLevels(t, map[string]string{"t.fc": `#fc 4
use console;
struct S { arr:[2]u8; }
var a:[3][2]u8;
var ss:[2]S;
function f(p:*u8):u8 { return p[1]; }
function first(r:[2]u8):u8 { return r[0]; }
function main():void
{
	a[0][0] = 1; a[0][1] = 2; a[1][0] = 3; a[1][1] = 7; a[2][0] = 5;
	ss[1].arr[0] = 4;
	var b = a[1];
	var c:[2]u8 = a[1];
	var d:[2]u8;
	var i:u8 = 2;
	d = a[i];
	var p:*S = &ss[1];
	var e = p.arr;
	b[0] = 99;
	@printf("{} {} {} {} {} {} {}\n", a[1][0], b[1], @len(b), c[0], d[0], e[0], @len(e));
	@printf("{} {} {}\n", @len(a[1]), f(a[1]), first(a[2]));
	console.exit(0);
}
`})
	if want := "3 7 2 3 5 4 2\n2 7 5\n"; err != nil || out != want {
		t.Errorf("got %q, %v, want %q", out, err, want)
	}
}
