package starmacro

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMaxSteps: 止まらないマクロは実行の手数の上限で止まる。
func TestMaxSteps(t *testing.T) {
	old := MaxSteps
	MaxSteps = 100_000
	defer func() { MaxSteps = old }()
	dir := t.TempDir()
	p := filepath.Join(dir, "m.star")
	if err := os.WriteFile(p, []byte("def spin():\n    while True:\n        pass\n"), 0o666); err != nil {
		t.Fatal(err)
	}
	set, err := Load([]*Script{{Name: "m", File: p, Root: dir}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := set.Call("spin", nil); err == nil || !strings.Contains(err.Error(), "too many steps") {
		t.Errorf("got %v", err)
	}
}
