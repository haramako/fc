package driver

// doc/v3_plan.md §10（一般的な用途で不便な仕様の調査、2026-09-26）で「修正する」にした項目のテスト。

import (
	"strings"
	"testing"
)

// TestBlockScope: 素のブロック `{ }` はスコープを作る（中の宣言は外から見えず、外と同じ名前を宣言できる）。
func TestBlockScope(t *testing.T) {
	t.Parallel()
	out, err := buildBothLevels(t, map[string]string{"t.fc": `#fc 3
use * from stdio;
function main():void
{
	var x:u8 = 10;
	{ var x:u8 = 20; printf(x, " "); }
	{ var t:u8 = 1; printf(t, " "); }
	{ var t:u8 = 2; printf(t, " "); }
	printf(x, "\n");
	exit(0);
}
`})
	if err != nil || out != "20 1 2 10\n" {
		t.Errorf("got %q, %v", out, err)
	}
	_, err = buildFiles(t, map[string]string{"t.fc": "#fc 3\nfunction main():void { { var q:u8 = 2; } q = 3; }\n"})
	if err == nil || !strings.Contains(err.Error(), "q not found") {
		t.Errorf("ブロックの外から中の変数: %v", err)
	}
}

// TestMemZeroLength: mem.zero / mem.compare の長さ 0 は何もしない・等しい (`iny; cpy; bne` のループで 256 バイトになっていた)。
func TestMemZeroLength(t *testing.T) {
	t.Parallel()
	out, err := buildBothLevels(t, map[string]string{"t.fc": `#fc 3
use * from stdio;
use mem;
var buf:[8]u8;
var n:u8;
function main():void
{
	for (var i:u8 = 0; i < 8; i += 1) { buf[i] = i + 1; }
	n = 0;
	mem.zero(&buf[2], n);
	printf(buf[1], buf[2], buf[7], " ");
	n = 3;
	mem.zero(&buf[2], n);
	printf(buf[1], buf[2], buf[4], buf[5], " ");
	n = 0;
	printf(mem.compare("abc", "xyz", n), mem.compare("abc", "abd", 2), mem.compare("abc", "abd", 3), "\n");
	exit(0);
}
`})
	if err != nil || out != "238 2006 001\n" {
		t.Errorf("got %q, %v", out, err)
	}
}
