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
