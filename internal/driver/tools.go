package driver

// ca65 / ld65 の探索。リリースの配布物には cc65 の ca65 / ld65 を fcc と同じディレクトリに同梱する
// (Windows / Linux。.goreleaser.yaml と .github/workflows/release.yml)。探索順:
//
//  1. 環境変数 FC_CC65_BIN のディレクトリ
//  2. fcc の実行ファイルと同じディレクトリ、およびその下の cc65/ と bin/
//  3. PATH (従来どおり)
//
// 見つからなければ名前のまま返し、起動時の "executable file not found" がそのまま出る。
// 自分の場所は os.Executable (Linux は /proc/self/exe、macOS / Windows は OS の API) で取り、
// それが取れない環境 (/proc の無いコンテナなど) では argv[0] から推定する。

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
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
	if d := selfDir(); d != "" {
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

// selfDir は fcc の実行ファイルがあるディレクトリ (シンボリックリンクは解決)。分からなければ ""。
func selfDir() string {
	self, err := os.Executable()
	if err != nil || self == "" {
		self = execFromArgv0(os.Args)
	}
	if self == "" {
		return ""
	}
	if real, err := filepath.EvalSymlinks(self); err == nil {
		self = real
	}
	return filepath.Dir(self)
}

// execFromArgv0 は argv[0] から実行ファイルのパスを推定する (os.Executable が使えないときの代替)。
// パス区切りを含めばそのまま (相対なら作業ディレクトリ基準)、含まなければ PATH から探す。
func execFromArgv0(args []string) string {
	if len(args) == 0 || args[0] == "" {
		return ""
	}
	a := args[0]
	if strings.ContainsAny(a, `/\`) {
		if abs, err := filepath.Abs(a); err == nil {
			return abs
		}
		return a
	}
	if p, err := exec.LookPath(a); err == nil {
		return p
	}
	return ""
}
