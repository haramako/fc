package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestBugzooPatchesApply: testdata/bugzoo/*.patch がどれも今のソースに当たる。コードを書き換えると当たらなくなり、zoo が
// そのバグを測らないまま「パッチが当たらない」とだけ出していた (2026-09-30 に 22 個中 16 個)。当たらなくなったら、パッチの先頭の
// 説明の修正を今のコードで探して戻し直す (Agent/wiki/testing-and-fuzzing.md「fuzz の効果の測定」)。
func TestBugzooPatchesApply(t *testing.T) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Skipf("git のリポジトリの中でない: %v", err)
	}
	root := strings.TrimSpace(string(out))
	patches, err := filepath.Glob(filepath.Join(root, "testdata", "bugzoo", "*.patch"))
	if err != nil || len(patches) == 0 {
		t.Fatalf("testdata/bugzoo のパッチが無い: %v", err)
	}
	for _, p := range patches {
		cmd := exec.Command("git", "apply", "--check", p)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("%s が当たらない: %s", filepath.Base(p), out)
		}
	}
}
