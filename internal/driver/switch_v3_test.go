package driver

// fc 3 の switch: case ごとのスコープと fallthrough。

import (
	"strings"
	"testing"
)

// TestV3SwitchScopeFallthrough: case / default ごとにスコープ (同じ名前を別の case で宣言できる)。fallthrough は次の
// case の本体へ値の検査をせずに進む (比較の連鎖とジャンプ表の両方、default へも)。return 忘れの検査も fallthrough を見る。
func TestV3SwitchScopeFallthrough(t *testing.T) {
	t.Parallel()
	src := `#fc 3
use * from stdio;

function chain(x:u8):u8
{
	var r:u8 = 0;
	switch (x) {
	case 1:
		var v:u8 = 10;
		r = r + v;
		fallthrough;
	case 2, 3:
		var v:u8 = 20;
		r = r + v;
	case 4:
		r = r + 1;
		fallthrough;
	default:
		var v:u8 = 100;
		r = r + v;
	}
	return r;
}

function table(x:u8):u8
{
	var r:u8 = 0;
	switch (x) {
	case 0: r = 1;
	case 1: r = 2; fallthrough;
	case 2: r = r + 10;
	case 3: r = 4;
	case 4: r = 5;
	case 5: r = 6;
	case 6: r = 7;
	case 7: r = 8;
	case 8: r = 9;
	case 9: r = 50; fallthrough;
	default: r = r + 1;
	}
	return r;
}

function ret(x:u8):u8
{
	switch (x) {
	case 1:
		fallthrough;
	case 2:
		return 7;
	default:
		return 9;
	}
}

function main():void
{
	printf(chain(1), " ", chain(2), " ", chain(3), " ", chain(4), " ", chain(9), "\n");
	printf(table(0), " ", table(1), " ", table(2), " ", table(9), " ", table(20), "\n");
	printf(ret(1), " ", ret(2), " ", ret(3), "\n");
	exit(0);
}
`
	out, err := buildBothLevels(t, map[string]string{"t.fc": src})
	want := "30 20 20 101 100\n1 12 10 51 1\n7 7 9\n"
	if err != nil || out != want {
		t.Errorf("got %q, %v\nwant %q", out, err, want)
	}
}

// TestV3SwitchErrors: case の外で case の変数を使う (案内つき)、fallthrough の置き場所、case の値が文字列 (codegen の panic だった)。
func TestV3SwitchErrors(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ src, msg string }{
		{`function f(x:u8):u8 { switch (x) { case 1: var v:u8 = 1; case 2: v = 2; } return 0; }`, "visible only in that case"},
		{`function f(x:u8):u8 { switch (x) { case 1: var v:u8 = 1; } return v; }`, "visible only in that case"},
		{`function f(x:u8):void { switch (x) { case 1: fallthrough; } }`, "cannot fallthrough from the last case"},
		{`function f(x:u8):void { switch (x) { case 1: default: fallthrough; } }`, "cannot fallthrough from default"},
		{`function f(x:u8):void { switch (x) { case 1: fallthrough; x = 1; case 2: } }`, "must be the last statement"},
		{`function f(x:u8):void { switch (x) { case 1: if (x) { fallthrough; } case 2: } }`, "must be the last statement"},
		{`function f(x:u8):void { fallthrough; }`, "must be the last statement"},
		{`function f(x:u8):u8 { switch (x) { case 1: fallthrough; case 2: x = 1; default: return 1; } }`, "missing return"},
		{`function f(x:u8):void { switch (x) { case "A": x = 1; } }`, "case value must be an integer constant (got [2]u8); a string literal"},
		{`function f(x:u8):void { switch (x) { case "A": } }`, "case value must be an integer constant"},
	} {
		_, err := buildFiles(t, map[string]string{"t.fc": "#fc 3\n" + c.src + "\nfunction main():void {}\n"})
		if err == nil || !strings.Contains(err.Error(), c.msg) {
			t.Errorf("%s: got %v, want %q", c.src, err, c.msg)
		}
	}
}

// TestV2SwitchUnchanged: fc 2 は今までどおり (case の宣言は囲むスコープ、fallthrough は識別子)。
func TestV2SwitchUnchanged(t *testing.T) {
	t.Parallel()
	out, err := buildFiles(t, map[string]string{"t.fc": `#fc 2
use * from stdio;
function main():void
{
	var fallthrough:int = 3;
	switch (fallthrough) {
	case 3:
		var v:int = 5;
	}
	printf(v + fallthrough, "\n");
	exit(0);
}
`})
	if err != nil || out != "8\n" {
		t.Errorf("got %q, %v", out, err)
	}
}
