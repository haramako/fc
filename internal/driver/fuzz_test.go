package driver

// コンパイラ全体 (構文解析〜意味解析〜コード生成、ファイルは書かない) の fuzz テスト。実行は:
//
//	go test ./internal/driver -fuzz FuzzCheck -fuzztime 60s
//
// どんな入力でもエラーで返り、panic しないことを確かめる。
// 見つかった入力は testdata/fuzz/ に保存され、以後は通常の go test でも回帰テストとして走る。

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func FuzzCheck(f *testing.F) {
	for _, dir := range []string{filepath.Join(absRepoRoot, "test"), filepath.Join(absRepoRoot, "examples")} {
		_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".fc") {
				return nil
			}
			if b, err := os.ReadFile(path); err == nil {
				f.Add(b)
			}
			return nil
		})
	}
	f.Fuzz(func(t *testing.T, src []byte) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "t.fc"), src, 0o644); err != nil {
			t.Skip()
		}
		c := NewCompiler(absRepoRoot)
		_, _ = c.Check("t.fc", &CheckOptions{Dir: dir})
	})
}
