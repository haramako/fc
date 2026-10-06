package fc

// fcc test (Agent/wiki/plans/v4-stdlib.md §7.1): モジュールの @(test) の関数を走らせる。

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// TestFrames は Target が TargetNES のテストを内蔵の NES のランナーで走らせるフレーム数の上限 (60 フレームで 1 秒。2 分)。
const TestFrames = 60 * 120

// TestCycles は Target が TargetEmu のテストを走らせるサイクル数の上限 (NES の CPU の 2 分ほど。無限ループで止まらなかった)。
const TestCycles = 1_789_773 * 120

// Test は files (モジュールの .fc) の @(test) の関数を走らせる。files を use して @run_tests() を呼ぶ main を一時ディレクトリに
// 作り、files のディレクトリを探索先 (Options.LibPath) に足してビルドし、Run で走らせる (emu は内蔵の 6502、nes は内蔵の NES の
// ランナー。結果の ExitCode が 0 なら全部通った。出力は run.Stdout (nil なら捨てる)。上限は run で決めなければ TestCycles /
// TestFrames)。ビルドの結果 (警告) も返す。
func (c *Compiler) Test(ctx context.Context, files []string, opt Options, run RunOptions) (*Result, *RunResult, error) {
	if len(files) == 0 {
		return nil, nil, fmt.Errorf("fcc test: no module is given")
	}
	dir, err := os.MkdirTemp("", "fctest")
	if err != nil {
		return nil, nil, err
	}
	defer os.RemoveAll(dir)
	var b strings.Builder
	b.WriteString("#fc 4\n")
	var libs []string
	seen := map[string]bool{}
	for _, f := range files {
		abs, err := filepath.Abs(f)
		if err != nil {
			return nil, nil, err
		}
		if _, err := os.Stat(abs); err != nil {
			return nil, nil, err
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
		return nil, nil, err
	}
	opt.Dir = dir
	opt.BuildDir = filepath.Join(dir, "build")
	opt.LibPath = append(libs, opt.LibPath...)
	if opt.Target == "" {
		opt.Target = TargetEmu
	}
	if opt.Out == "" {
		opt.Out = filepath.Join(dir, "test.bin")
	}
	res, err := c.Build(ctx, main, opt)
	if err != nil {
		return res, nil, err
	}
	if run.Stdout == nil {
		run.Stdout = io.Discard
	}
	if run.MaxCycles == 0 {
		run.MaxCycles = TestCycles
	}
	if run.MaxFrames == 0 {
		run.MaxFrames = TestFrames
	}
	r, err := c.Run(ctx, res, run)
	if err != nil {
		return res, nil, fmt.Errorf("fcc test: %w", err)
	}
	return res, r, nil
}
