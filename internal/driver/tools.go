package driver

// ca65 / ld65 の探索。リリースの配布物には cc65 の ca65 / ld65 を fcc と同じディレクトリに同梱する
// (Windows / Linux。.goreleaser.yaml と .github/workflows/release.yml)。探索順:
//
//  1. 環境変数 FC_CC65_BIN のディレクトリ
//  2. fcc の実行ファイルと同じディレクトリ、およびその下の cc65/ と bin/
//  3. PATH (従来どおり)
//
// 見つからなければ名前のまま返し、起動時の "executable file not found" がそのまま出る。

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
)

var (
	toolMu    sync.Mutex
	toolCache = map[string]string{}
)

// ToolPath は ca65 / ld65 などの外部ツールの実行パスを返す (見つからなければ name のまま)。
func ToolPath(name string) string {
	toolMu.Lock()
	defer toolMu.Unlock()
	if p, ok := toolCache[name]; ok {
		return p
	}
	p := findTool(name)
	toolCache[name] = p
	return p
}

// ResetToolCache は探索結果を捨てる (テスト用。環境変数を変えた後に呼ぶ)。
func ResetToolCache() {
	toolMu.Lock()
	defer toolMu.Unlock()
	toolCache = map[string]string{}
}

func findTool(name string) string {
	exe := name
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	var dirs []string
	if d := os.Getenv("FC_CC65_BIN"); d != "" {
		dirs = append(dirs, d)
	}
	if self, err := os.Executable(); err == nil {
		if real, err := filepath.EvalSymlinks(self); err == nil {
			self = real
		}
		d := filepath.Dir(self)
		dirs = append(dirs, d, filepath.Join(d, "cc65"), filepath.Join(d, "bin"))
	}
	for _, d := range dirs {
		p := filepath.Join(d, exe)
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p
		}
	}
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	return name
}
