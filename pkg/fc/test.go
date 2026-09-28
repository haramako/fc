package fc

// fcc test (doc/v4_stdlib.md §7.1): モジュールの @(test) の関数を走らせる。

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Test は files (モジュールの .fc) の @(test) の関数を走らせる。files を use して @run_tests() を呼ぶ main を一時ディレクトリに
// 作り、files のディレクトリを探索先 (Options.LibPath) に足してビルドし、emu なら実行する (結果の ExitCode が 0 なら全部通った。
// 出力は Options.Stdout)。
func (c *Compiler) Test(ctx context.Context, files []string, opt Options) (*Result, error) {
	if len(files) == 0 {
		return nil, fmt.Errorf("fcc test: no module is given")
	}
	dir, err := os.MkdirTemp("", "fctest")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	var b strings.Builder
	b.WriteString("#fc 4\n")
	var libs []string
	seen := map[string]bool{}
	for _, f := range files {
		abs, err := filepath.Abs(f)
		if err != nil {
			return nil, err
		}
		if _, err := os.Stat(abs); err != nil {
			return nil, err
		}
		fmt.Fprintf(&b, "use %s;\n", strings.TrimSuffix(filepath.Base(abs), ".fc"))
		if d := filepath.Dir(abs); !seen[d] {
			seen[d] = true
			libs = append(libs, d)
		}
	}
	b.WriteString("function main():void { @run_tests(); }\n")
	const main = "fctest_main.fc"
	if err := os.WriteFile(filepath.Join(dir, main), []byte(b.String()), 0o666); err != nil {
		return nil, err
	}
	opt.Dir = dir
	opt.BuildDir = filepath.Join(dir, "build")
	opt.LibPath = append(libs, opt.LibPath...)
	if opt.Target == "" {
		opt.Target = TargetEmu
	}
	opt.Run = opt.Target == TargetEmu
	if opt.Out == "" {
		opt.Out = filepath.Join(dir, "test.bin")
	}
	return c.Build(ctx, main, opt)
}
