package driver

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestToolPath: FC_CC65_BIN のディレクトリが PATH より優先され、無ければ PATH (または名前のまま)。
func TestToolPath(t *testing.T) {
	exe := "ca65"
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	dir := t.TempDir()
	fake := filepath.Join(dir, exe)
	if err := os.WriteFile(fake, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FC_CC65_BIN", dir)
	ResetToolCache()
	defer ResetToolCache()
	if got := ToolPath("ca65"); got != fake {
		t.Errorf("FC_CC65_BIN: got %q want %q", got, fake)
	}
	// 無い名前は名前のまま
	if got := ToolPath("no_such_tool_xyz"); got != "no_such_tool_xyz" {
		t.Errorf("missing: got %q", got)
	}
	// 環境変数を外せば PATH 側 (テスト環境には ca65 がある)
	t.Setenv("FC_CC65_BIN", "")
	ResetToolCache()
	if got := ToolPath("ca65"); got == fake || got == "ca65" {
		t.Errorf("PATH: got %q", got)
	}
}
