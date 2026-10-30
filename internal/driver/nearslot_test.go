package driver

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestNearSameSlotOtherBank: @(near) (モジュールか関数) の呼び先が、呼ぶ側と同じスロットの別のバンクにあればエラー
// (呼ぶ側を実行している間は決して入っていないので、jsr は別の関数の途中へ飛ぶ。GitHub の Issue #18)。別のスロットならよい。
func TestNearSameSlotOtherBank(t *testing.T) {
	t.Parallel()
	toml := "[target]\nmapper = \"MMC3\"\nprg = \"128K\"\nchr = \"128K\"\n\n[bank.code1]\nslot = 0xA000\nindex = 1\n\n" +
		"[bank.code4]\nslot = 0xA000\nindex = 4\n\n[bank.code2]\nslot = 0x8000\nindex = 2\n"
	main := "#fc 4\n@(farcall);\nuse mmc3;\nuse q;\nfunction main():void\n{\n\tmmc3.init();\n\tq.g();\n}\n"
	q := "#fc 4\n@(bank: \"code4\");\nuse p;\npublic function g():void\n{\n\tp.f();\n}\n"
	for _, c := range []struct {
		name, p string
		ok      bool
	}{
		{"モジュールの near", "#fc 4\n@(bank: \"code1\");\n@(near);\npublic var v:u8;\npublic function f():void @(noinline) { v += 1; }\n", false},
		{"関数の near", "#fc 4\n@(bank: \"code1\");\npublic var v:u8;\npublic function f():void @(noinline, near) { v += 1; }\n", false},
		{"別のスロット", "#fc 4\n@(bank: \"code2\");\n@(near);\npublic var v:u8;\npublic function f():void @(noinline) { v += 1; }\n", true},
	} {
		dir := t.TempDir()
		writeFiles(t, dir, map[string]string{"fc.toml": toml, "main.fc": main, "q.fc": q, "p.fc": c.p})
		_, err := buildCode(t, NewCompiler(absRepoRoot), "main.fc", &BuildOptions{Target: "nes", Dir: dir, BuildDir: filepath.Join(dir, "b"), Out: filepath.Join(dir, "a.nes")}, false, nil, 0)
		if c.ok {
			if err != nil {
				t.Errorf("%s: ビルド失敗: %v", c.name, err)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), `it is @(near) but its module p is in bank "code1", which shares slot $A000 with bank "code4"`) {
			t.Errorf("%s: エラーのはず: %v", c.name, err)
		}
	}
}
