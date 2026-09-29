package fc

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestFclibNoWarnings: fclib の fc 4 のモジュール (fclib/*.fc と fclib/<target>/*.fc) は、それぞれを use したプログラムで警告が
// 出ない (`a & b == c` の優先順位などの lint。@(test) の無いモジュールも。fc 3 のまま残した旧 stdio / unittest は除く)。
func TestFclibNoWarnings(t *testing.T) {
	t.Parallel()
	c, err := New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	root := filepath.Join("..", "..", "fclib")
	for _, target := range []string{TargetEmu, TargetNES} {
		files, _ := filepath.Glob(filepath.Join(root, "*.fc"))
		sub, _ := filepath.Glob(filepath.Join(root, target, "*.fc"))
		for _, f := range append(files, sub...) {
			src, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.HasPrefix(src, []byte("#fc 4")) {
				continue
			}
			mod := strings.TrimSuffix(filepath.Base(f), ".fc")
			dir := t.TempDir()
			main := filepath.Join(dir, "t.fc")
			if err := os.WriteFile(main, []byte("#fc 4\nuse "+mod+";\nfunction main():void { }\n"), 0o666); err != nil {
				t.Fatal(err)
			}
			ws, err := c.Check(main, CheckOptions{Target: target, Dir: dir})
			if err != nil {
				t.Errorf("%s (%s): %v", f, target, err)
				continue
			}
			for _, w := range ws {
				t.Errorf("%s (%s): warning: %s: %s", f, target, w.Pos, w.Msg)
			}
		}
	}
}
