package driver

// fc 3 の @null_fn (何もしない関数)。

import (
	"strings"
	"testing"
)

// TestNullFn: 戻り値の無い関数の型ならどれにでも入る (引数あり・fastcall・表の要素)。呼んでも何も起きない。比較できる。
func TestNullFn(t *testing.T) {
	t.Parallel()
	src := `#fc 3
use * from stdio;

var hook:fn():void;
var hook2:fn(u8, u16):void;
var fast:fn(u8):void @(fastcall);
var count:u8;

function inc():void { count += 1; }
function add(a:u8, b:u16):void { count += a; }

const TABLE:[?]fn():void = [inc, @null_fn, inc];

function call_all():void
{
	for (var i = 0; i < 3; i += 1) {
		TABLE[i]();
	}
}

function main():void
{
	hook = @null_fn;
	hook();
	hook2 = @null_fn;
	hook2(5, 300);
	fast = @null_fn;
	fast(3);
	var same = hook == @null_fn;
	hook = inc;
	hook();
	hook2 = add;
	hook2(10, 1);
	call_all();
	var f:fn():void = @null_fn;
	f();
	var g = @null_fn;
	g();
	printf(count, " ", same, " ", hook == @null_fn, "\n");
	exit(0);
}
`
	out, err := buildBothLevels(t, map[string]string{"t.fc": src})
	if err != nil || out != "13 1 0\n" {
		t.Errorf("got %q, %v", out, err)
	}
	for _, c := range []struct{ src, msg string }{
		{`var f:fn():u8; function main():void { f = @null_fn; }`, "it returns nothing"},
		{`var p:*u8; function main():void { p = @null_fn; }`, "cannot be used as *u8"},
		{`function main():void { @null_fn(); }`, "remove the call"},
	} {
		_, err := buildFiles(t, map[string]string{"t.fc": "#fc 3\n" + c.src + "\n"})
		if err == nil || !strings.Contains(err.Error(), c.msg) {
			t.Errorf("%s: got %v, want %q", c.src, err, c.msg)
		}
	}
}
