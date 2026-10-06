package driver

// テストの共通の手順: ソースを TempDir に書いて Build し、必要なら emu で走らせる (testBuild)。以前は「TempDir に書いて Build」の
// 包み関数が約 15 種類あり、MaxCycles や ca65 の一時的な失敗のやり直し、panic の扱いがまちまちだった。個々の包み関数
// (runEmu / buildFiles / buildBothLevels / rpRun …) は呼び出し側の読みやすさのために残し、中身をここに寄せる。

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/runner"
)

// testMaxCycles は emu で走らせるときのサイクル数の既定の上限 (無限ループの検出。普通のテストのプログラムは 1M 未満)。
const testMaxCycles = 50_000_000

// buildSpec は 1 回のビルドの指定。
type buildSpec struct {
	Files       map[string]string // 名前 → 中身 (サブディレクトリも可)
	Main        string            // 入口のファイル (既定 t.fc)
	Target      string            // 既定 emu
	Out         string            // 出力のファイル名 (既定 a.bin。nes なら a.nes)
	Level       int               // BuildOptions.OptimizeLevel (0 は既定の -O 2、-1 は -O 0)
	Run         bool              // ビルドの後に runner で走らせる (emu)
	CompileOnly bool
	MaxCycles   int64 // Run のサイクル数の上限 (0 なら testMaxCycles)
	Debug       bool  // -g
	LogEvery    bool  // 文ごとの @log (テスト用)
	Misclassify bool  // 常駐の見積もりをわざと外す (テスト用)
	Defines     []string
	Config      *ir.Config
	// Recover はコンパイラや emu の panic をエラーとして返す (fuzz: 失敗として集めて最小化する)。普通のテストは panic のまま
	// 落として場所を見る
	Recover bool
}

// buildResult はビルドの結果。
type buildResult struct {
	Dir    string // ソースを置いたディレクトリ (中間生成物は Dir/b)
	Out    string // 出力のパス
	Stdout string // emu の printf の出力
	LogOut string // emu の @log の出力
	Res    *Result
	Run    *runner.Result // Run したときの終了コードとサイクル数
	Err    error
}

// ROM は出力のファイルの中身。
func (r *buildResult) ROM(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile(r.Out)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// Built は中間生成物 (Dir/b の name) の中身。
func (r *buildResult) Built(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(r.Dir, "b", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// testBuild は s をビルドする。ca65 / ld65 が何も出さずに失敗したとき (並列で重いときの Windows の一時的な失敗) は
// 2 回までやり直す (文言のある失敗はプログラムかコンパイラの問題なのでそのまま返す)。
func testBuild(t *testing.T, s buildSpec) (r *buildResult) {
	t.Helper()
	dir := t.TempDir()
	for name, src := range s.Files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o777); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(src), 0o666); err != nil {
			t.Fatal(err)
		}
	}
	main, out := s.Main, s.Out
	if main == "" {
		main = "t.fc"
	}
	if out == "" {
		out = "a.bin"
		if s.Target == "nes" {
			out = "a.nes"
		}
	}
	r = &buildResult{Dir: dir, Out: filepath.Join(dir, out)}
	if s.Recover {
		defer func() {
			if p := recover(); p != nil {
				r.Err = fmt.Errorf("panic: %v", p)
			}
		}()
	}
	maxCycles := s.MaxCycles
	if maxCycles == 0 {
		maxCycles = testMaxCycles
	}
	for retry := 0; ; retry++ {
		opt := &BuildOptions{Target: s.Target, Dir: dir, BuildDir: filepath.Join(dir, "b"), Out: r.Out, OptimizeLevel: s.Level,
			CompileOnly: s.CompileOnly, Debug: s.Debug, LogEveryStatement: s.LogEvery, MisclassifyResident: s.Misclassify,
			Defines: s.Defines, Config: s.Config}
		r.Res, r.Err = NewCompiler(absRepoRoot).BuildContext(t.Context(), main, opt)
		var ce *CommandError
		if retry < 2 && errors.As(r.Err, &ce) && strings.TrimSpace(ce.Result) == "" {
			continue
		}
		if r.Err == nil && s.Run {
			var stdout, logs strings.Builder
			r.Run, r.Err = runBuilt(r.Res, runner.Options{Stdout: &stdout, LogOut: &logs, MaxCycles: maxCycles, TracePC: tracePC(s.Config)})
			r.Stdout, r.LogOut = stdout.String(), logs.String()
		}
		return r
	}
}

// runBuilt はビルドした res を runner で走らせる (res.Out・res.Target・@log の地点 res.Log)。
func runBuilt(res *Result, o runner.Options) (*runner.Result, error) {
	if o.Log == nil {
		o.Log = res.Log
	}
	return runner.Run(res.Target, res.Out, o)
}

// buildRun は opt でビルドして走らせる (テスト用。出力は out、サイクル数の上限は testMaxCycles)。
func buildRun(t *testing.T, c *Compiler, main string, opt *BuildOptions, out io.Writer) (*Result, *runner.Result, error) {
	t.Helper()
	res, err := c.BuildContext(t.Context(), main, opt)
	if err != nil {
		return res, nil, err
	}
	run, err := runBuilt(res, runner.Options{Stdout: out, MaxCycles: testMaxCycles, TracePC: tracePC(opt.Config)})
	return res, run, err
}

// tracePC は FC_TRACE=pc (emu の panic のときに直近の PC を出す) か。
func tracePC(cfg *ir.Config) bool {
	if cfg == nil {
		cfg = ir.ConfigFromEnv()
	}
	return cfg.Trace("pc") != ""
}

// exitError は Run の終了コードが 0 でなければそのエラー。
func (r *buildResult) exitError() error {
	if r.Err != nil {
		return r.Err
	}
	if r.Run != nil && r.Run.ExitCode != 0 {
		return fmt.Errorf("exit code %d: %s", r.Run.ExitCode, r.Stdout)
	}
	return nil
}

// buildCode は opt でビルドし、run なら走らせて終了コードを返す (以前の Compiler.Build。テスト用)。out は出力 (nil なら捨てる)、
// maxCycles はサイクル数の上限 (0 なら testMaxCycles)。
func buildCode(t *testing.T, c *Compiler, main string, opt *BuildOptions, run bool, out io.Writer, maxCycles int64) (int, error) {
	t.Helper()
	res, err := c.BuildContext(t.Context(), main, opt)
	if err != nil || !run {
		return 0, err
	}
	if maxCycles == 0 {
		maxCycles = testMaxCycles
	}
	r, err := runBuilt(res, runner.Options{Stdout: out, MaxCycles: maxCycles, TracePC: tracePC(opt.Config)})
	if err != nil {
		return 0, err
	}
	return r.ExitCode, nil
}
