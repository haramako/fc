package driver

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/haramako/fc/internal/diag"
)

// TestLinkOverflowMessage: 変数が RAM からあふれたら、区画の番地と大きさ・あふれた量・モジュールごとの量を fc の言葉で出す。
// 失敗したビルドは ROM を残さない (ld65 の出力はいったん別の名前に書く)。
func TestLinkOverflowMessage(t *testing.T) {
	t.Parallel()
	r := testBuild(t, buildSpec{Target: "nes", Files: map[string]string{
		"fc.toml": "[target]\nmapper = \"NROM\"\n",
		"t.fc":    "#fc 4\nuse frame;\nvar big:[1400]u8;\nfunction main():void\n{\n\tframe.init();\n\tbig[5] = 1;\n\twhile (true) {\n\t\tframe.wait();\n\t}\n}\n",
	}})
	if r.Err == nil {
		t.Fatal("ビルドが通った")
	}
	msg := r.Err.Error()
	var de *diag.Error
	if !errors.As(r.Err, &de) || de.Pos.Filename != "t.fc" {
		t.Errorf("位置が入口のソースでない: %+v", r.Err)
	}
	for _, want := range []string{"the variables do not fit in SRAM ($0200-$06FF, 1280 bytes)", "bytes over", "module t 1400", "module frame", "Make large arrays smaller"} {
		if !strings.Contains(msg, want) {
			t.Errorf("%q が無い: %s", want, msg)
		}
	}
	for _, p := range []string{r.Out, r.Out + ".part"} {
		if _, err := os.Stat(p); err == nil {
			t.Errorf("失敗したビルドの出力が残っている: %s", p)
		}
	}
}

// TestLinkOtherError: 説明の付けられない ld65 の失敗は「link failed」を頭に付ける (asm が無いシンボルを参照する)。
func TestLinkOtherError(t *testing.T) {
	t.Parallel()
	r := testBuild(t, buildSpec{Target: "nes", Files: map[string]string{
		"fc.toml": "[target]\nmapper = \"NROM\"\n",
		"t.asm":   ".import no_such_symbol\n.segment \"CODE\"\n\tjmp no_such_symbol\n",
		"t.fc":    "#fc 4\nuse frame;\n@include(\"t.asm\");\nfunction main():void\n{\n\tframe.init();\n\twhile (true) {\n\t\tframe.wait();\n\t}\n}\n",
	}})
	if r.Err == nil || !strings.Contains(r.Err.Error(), "link failed") {
		t.Errorf("got %v", r.Err)
	}
}

// TestChrIncludeWithChrRAM: CHR RAM (fc.toml の chr = 0) で .chr を @include すると、その場所でエラー (ld65 の生のエラーでなく)。
func TestChrIncludeWithChrRAM(t *testing.T) {
	t.Parallel()
	r := testBuild(t, buildSpec{Target: "nes", Files: map[string]string{
		"fc.toml": "[target]\nmapper = \"NROM\"\nchr = 0\n",
		"a.chr":   strings.Repeat("\x00", 16),
		"t.fc":    "#fc 4\nuse frame;\n@include(\"a.chr\");\nfunction main():void\n{\n\tframe.init();\n}\n",
	}})
	var de *diag.Error
	if !errors.As(r.Err, &de) || de.Pos.Line != 3 || !strings.Contains(de.Msg, "CHR RAM") {
		t.Errorf("got %+v", r.Err)
	}
}
