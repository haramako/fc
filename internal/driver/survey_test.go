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

// TestMemZeroLength: mem.set / zero / compare の長さは u16 で、0 は何もしない・等しい (長さ u8 の `iny; cpy; bne` のループで
// 0 が 256 バイトになっていた。256 を渡すと 0 に切り詰められ、それに頼っていた)。256 バイトを超える長さも。
func TestMemZeroLength(t *testing.T) {
	t.Parallel()
	out, err := buildBothLevels(t, map[string]string{"t.fc": `#fc 3
use * from stdio;
use mem;
var buf:[8]u8;
var n:u8;
var big:[301]u8;
var big2:[301]u8;
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
	printf(mem.compare("abc", "xyz", n), mem.compare("abc", "abd", 2), mem.compare("abc", "abd", 3), " ");
	mem.set(&big[0], 7, 300);
	printf(big[0], big[255], big[299], big[300], " ");
	mem.copy(&big2[0], &big[0], 300);
	printf(mem.compare(&big[0], &big2[0], 300), " ");
	big2[299] = 1;
	printf(mem.compare(&big[0], &big2[0], 300), mem.compare(&big[0], &big2[0], 299), " ");
	mem.zero(&big[1], 298);
	printf(big[0], big[1], big[298], big[299], "\n");
	exit(0);
}
`})
	if err != nil || out != "238 2006 001 7770 0 10 7007\n" {
		t.Errorf("got %q, %v", out, err)
	}
}

// TestInlineLocalConst: インライン展開する関数の中の const の表・文字列 (その関数の .proc の中のラベル) を、展開先へ別の名前で
// 写す (ca65 の `Symbol 'T' is undefined` だった)。表が別の表を指す形、呼び出し側の同じ名前の表とも衝突しない。
func TestInlineLocalConst(t *testing.T) {
	t.Parallel()
	out, err := buildBothLevels(t, map[string]string{"t.fc": `#fc 3
use * from stdio;
function f(i:u8):u8 { const T = [5, 6, 7]; var p:*const u8 = "ab"; return T[i] + p[0]; }
function g(i:u8):u8 { const T = [50, 60, 70]; const PS:[?]*const u8 = ["xy", "zw"]; return T[i] + PS[i][1]; }
function main():void
{
	const T = [1, 2, 3];
	printf(f(1), " ", f(2), " ", g(0), " ", g(1), " ", T[2], "\n");
	exit(0);
}
`})
	if err != nil || out != "103 104 171 179 3\n" {
		t.Errorf("got %q, %v", out, err)
	}
}

// TestLocalInferredArray: ローカルの `var a:[?]T = [...]` は初期値から長さを決める (長さ未定のまま領域が取られず、ほかの
// ローカルを壊していた。-O 0 では ca65 の Range error)。
func TestLocalInferredArray(t *testing.T) {
	t.Parallel()
	out, err := buildBothLevels(t, map[string]string{"t.fc": `#fc 3
use * from stdio;
struct P { x:u8; y:u8; }
function main():void
{
	var a:[?]u8 = [1, 2, 3];
	var z:u8 = 50;
	var ps:[?]P = [{1, 2}, {3, 4}];
	a[0] = 9;
	ps[1].y = 7;
	printf(a[0], a[1], a[2], " ", z, " ", @len(a), " ", @sizeof(ps), " ", ps[0].x, ps[1].y, "\n");
	exit(0);
}
`})
	if err != nil || out != "923 50 3 4 17\n" {
		t.Errorf("got %q, %v", out, err)
	}
}
