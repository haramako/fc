package driver

// fc 4 の左の項の型で折り返す演算 `+%` / `-%` / `*%` (sema/wrap.go)。

import (
	"strings"
	"testing"

	"github.com/haramako/fc/internal/syntax"
)

// TestWrapOps: 結果は左の項の型で、その幅で折り返す。u8 の座標 + i8 の移動量を割る・比べる・シフトする形が符号なしになる
// (`+` は同じ大きさなら符号付きが勝つ)。広い代入先でも広げない (A1 の区切り)。右の項が狭ければ自分の符号で広げる。
func TestWrapOps(t *testing.T) {
	t.Parallel()
	out, err := buildBothLevels(t, map[string]string{"t.fc": `#fc 4
use console;
var y:u8;
var dy:i8;
var vy:i8;
var giff:u8;
var big:u16;
function main():void
{
	y = 200;
	dy = 1;
	vy = -20;
	giff = 3;
	big = 1000;
	@printf("{} {}\n", (y + dy) / 16, (y +% dy) / 16);
	@printf("{} {}\n", y + dy > 100, y +% dy > 100);
	@printf("{} {}\n", (vy +% giff) / 16, (y -% dy) >> 4);
	var n = y +% dy;
	var d:u16 = y +% 100;
	@printf("{} {} {} {} {}\n", n, d, big +% dy, big -% 1001, y *% 3);
	@printf("{} {}\n", y +% -1, (250 as u8) +% 10);
	const K:u8 = 250;
	const K2 = K +% 10;
	@printf("{}\n", K2);
	console.exit(0);
}
`})
	if want := "-4 12\nfalse true\n-2 12\n201 44 1001 65535 88\n199 4\n4\n"; err != nil || out != want {
		t.Errorf("got %q, %v\nwant %q", out, err, want)
	}
	for src, want := range map[string]string{
		"var a:u8; var w:u16; var x = a +% w;": "the right operand (u16) is wider than the left (u8)",
		"var a:u8; var x = a +% 300;":          "300 does not fit in u8",
		"var a:u8; var x = 3 +% a;":            "the left operand decides the type",
		"var p:*u8; var x = p +% 1;":           "the left operand must be an integer",
	} {
		_, err := buildFiles(t, map[string]string{"t.fc": "#fc 4\nfunction main():void\n{\n\t" + src + "\n}\n"})
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: got %v, want /%s/", src, err, want)
		}
	}
	if _, err := buildFiles(t, map[string]string{"t.fc": "#fc 3\nfunction main():void\n{\n\tvar a:u8 = 1;\n\tvar x = a +% 1;\n}\n"}); err == nil || !strings.Contains(err.Error(), "`+%` is fc 4") {
		t.Errorf("fc 3: got %v", err)
	}
	src := "#fc 4\nvar x = a +% b *% c -% d;\n"
	if got, err := syntax.Format([]byte(src), "t.fc"); err != nil || string(got) != src {
		t.Errorf("fmt: got %q, %v", got, err)
	}
}
