package driver

import (
	"strings"
	"testing"
)

// TestShrinkNesShapes: fclib の NES のモジュールを縮める生成コードの改良 (2026-10-05)。
//   - volatile なグローバルの定数の連鎖 `(128 - qlen) - 3` を `125 - qlen` に (inline した room() の結果を 1 回だけ使う:
//     読む回数は変わらない。opt の ssa の volatileOnce)
//   - @min の 2 つの引数を計算してから 1 つ目を写す `t = a - 3; u = b - 10; v = t` の写しを消す (opt の coalesce が数命令先の
//     写しまで見る)
//   - `load n = t` で t がそこで終わり n がそこで始まるなら同じ番地に置いて写しを出さない (regalloc の copyHints)
//   - 1 バイトの戻り値はフレームに書かず A だけで返す (どの呼び出しもフレームから読まない関数: CallConv.Result.OnlyA)
//   - ループの回転が前に出した最初の検査 `0 < 40` を畳む (regalloc の foldConstBranches)
func TestShrinkNesShapes(t *testing.T) {
	t.Parallel()
	out, asm := buildShape(t, `#fc 4
use console;
var qlen:u8 @(volatile);
var qcost:u8 @(volatile);
function room():u8
{
	return 128 - qlen;
}
function budget():u8
{
	return 140 - qcost;
}
function take(len:u8):u8 @(noinline)
{
	var n = @min(len, @min(room() - 3, budget() - 10));
	qcost += n;
	return n;
}
var tab:[40]u8;
function scan():u8 @(noinline)
{
	var a:u8 = 0;
	for (var i:u8 = 0; i < 40; i += 1) {
		a ^= tab[i];
		a <<= 1;
	}
	return a;
}
function main():void
{
	qlen = 10;
	qcost = 3;
	for (var i:u8 = 0; i < 40; i += 1) { tab[i] = i * 7 + 1; }
	var x = take(200);
	var y = take(50);
	@printf("{} {} {}\n", x, y, scan());
	console.exit(0);
}
`)
	// take(200): min(200, min(115, 127)) = 115、qcost = 118。take(50): min(50, min(115, 12)) = 12。scan は Python で同じ計算をした値
	if out != "115 12 248\n" {
		t.Errorf("got %q", out)
	}
	take := procBody(t, asm, "_t_take")
	all := strings.Join(take, "\n")
	for _, bad := range []string{"lda #128", "lda #140"} {
		if strings.Contains(all, bad) {
			t.Errorf("take: 定数の連鎖を畳んでいない (%s):\n%s", bad, all)
		}
	}
	if len(take) > 30 { // 以前は 38 行 (定数の連鎖の sec; sbc #k、@min の写し、n への写し)
		t.Errorf("take: %d 行 (30 行以下のはず):\n%s", len(take), all)
	}
	if strings.Contains(all, "F_t_take+0") {
		t.Errorf("take: 戻り値をフレームに書いている:\n%s", all)
	}
	scan := procBody(t, asm, "_t_scan")
	for i := 0; i+1 < len(scan); i++ {
		if scan[i] == "cmp #40" && strings.HasPrefix(scan[i+1], "bcs") {
			t.Errorf("scan: 最初の検査 0 < 40 が残っている:\n%s", strings.Join(scan, "\n"))
			break
		}
	}
}
