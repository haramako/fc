package driver

// v3m の自己検査の文 (TestRandomV3Programs)。sema の誤りは -O 0 / -O 2 / 最適化前の IR のインタプリタの 3 つの判定が
// 同じように間違えるので見えない。同じことを別の書き方で 2 回計算してプログラムの中で比べ、違えば stdio.exit(77) で
// 止める (rpCheck が "selfcheck" として失敗にする)。

import (
	"fmt"
	"strings"
)

// v3SelfCheck は自己検査の文 1 つ ($k などは v3Stmt が文ごとの名前に置き換える)。
func (g *rpGen) v3SelfCheck(f *rpFunc) string {
	b := func() string { return g.v3Byte(f) }
	var src string
	switch g.pick(5) {
	case 4:
		// for-each の回す値の中の変数は最初に 1 回だけ評価する (`&vm[k]` の k を途中で変えても最初の行を回る。毎周読み直していた)
		src = `{
	for (var $k in 0..4) {
		vm[0][$k] = $k;
		vm[1][$k] = $k + 10;
	}
	var $u:u16 = 0;
	var $w = 0;
	for (var $x in &vm[$w]) {
		$u += *$x;
		$w = 1;
	}
	if ($u != 6) {
		stdio.exit(77);
	}
}`
	case 0:
		// ポインタの負のずれ・i8 のずれで書いた値を、16 ビットの添字で読む (符号拡張していなかった)
		src = fmt.Sprintf(`{
	var $w:*u16 = &vbw[100];
	var $k:i8 = ((%s %% 64) as i8) - 32;
	$w[$k] = %s;
	if ($w[$k] != vbw[((100 as i16) + ($k as i16)) as u16] || *($w - 1) != vbw[99]) {
		stdio.exit(77);
	}
}`, b(), b())
	case 1:
		// for-each の合計と添字のループの合計、文字列リテラルの長さ (終端の 0 まで回っていた)
		src = `{
	var $u:u16 = 0;
	for (var $x in vb) {
		$u += $x;
	}
	for (var $k = 0; $k < 24; $k++) {
		$u -= vb[$k];
	}
	var $w = 0;
	for (var $x in "abc") {
		$w += 1;
	}
	if ($u != 0 || $w != 3) {
		stdio.exit(77);
	}
}`
	case 2:
		// 範囲のループの回数 (実行時の端、`..=`、空の範囲)
		src = fmt.Sprintf(`{
	var $lo = %s %% 12;
	var $hi = %s %% 12;
	var $u = 0;
	for (var $k in $lo..$hi) {
		$u += 1;
	}
	var $w = 0;
	for (var $k in $lo..=$hi) {
		$w += 1;
	}
	if ($lo < $hi && ($u != $hi - $lo || $w != $u + 1) || $lo >= $hi && $u != 0) {
		stdio.exit(77);
	}
}`, b(), b())
	default:
		// case の範囲と比較の連鎖
		src = fmt.Sprintf(`{
	var $k = %s;
	var $u = 0;
	switch ($k) {
	case 0..10:
		$u = 1;
	case 10..=20, 30:
		$u = 2;
	case 100..=255:
		$u = 3;
	default:
		$u = 4;
	}
	var $w = 4;
	if ($k < 10) {
		$w = 1;
	} elsif ($k <= 20 || $k == 30) {
		$w = 2;
	} elsif ($k >= 100) {
		$w = 3;
	}
	if ($u != $w) {
		stdio.exit(77);
	}
}`, b())
	}
	// 字下げ 1 段 (関数の本体の中) にして改行で終える
	return "\t" + strings.ReplaceAll(src, "\n", "\n\t") + "\n"
}
