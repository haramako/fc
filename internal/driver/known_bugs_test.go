package driver

// 既知のバグ (未修正の Issue) が「まだ再現する」ことを見張るテスト。生成器が既知のバグの形を避けている間 (毎晩の fuzz を
// 赤にしないため)、バグが直ったらこのテストが落ちて「回避を外す」ことを知らせる (避けたまま外し忘れると、その形のほかの
// バグも見えなくなる。2026-10-19 の決定: Agent/discussions/2026-10-19-known-bug-xfail.md)。
// 直ったら: そのテストをここから bugs_test.go などの回帰テスト (今度は「通る」ことを見る) に移し、生成器の回避を外し、
// 書いてある種の範囲で fuzz を回して確かめる。

import (
	"strings"
	"testing"
)

// TestKnownBugF6FalseError: 符号の混ざり (F6) のエラーが、広げた 8 ビットの式を同じ大きさの符号違いと見なして誤検出する
// (`a - (b >> 2)` (a:i16, b:u8) がエラー。Agent/issues/local-2026-10-18-mixed-sign-false-error.md)。
// TestRandomArithV4 は randarith_test.go の arMarkTy でこの形を避けている。
func TestKnownBugF6FalseError(t *testing.T) {
	t.Parallel()
	r := testBuild(t, buildSpec{Files: map[string]string{"t.fc": `#fc 4
use console;
var a:i16 = 100;
var b:u8 = 200;
function main():void
{
	@printf("{}\n", a - (b >> 2));
	console.exit(0);
}
`}, Run: true})
	if r.Err == nil || !strings.Contains(r.Err.Error(), "mixes") {
		t.Fatalf("F6 の誤検出が直った (err %v、出力 %q)。randarith_test.go の arMixed の arMarkTy の分岐を外し、"+
			"`go test ./internal/driver -run TestRandomArithV4 -randn 4000 -randseed 20261018000` で落ちないことを確かめて、"+
			"このテストを「通る」回帰テストに直す", r.Err, r.Stdout)
	}
}
