package driver

// テストの共通の手順: ソースを TempDir に書いて Build し、必要なら emu で走らせる (testBuild)。以前は「TempDir に書いて Build」の
// 包み関数が約 15 種類あり、MaxCycles や ca65 の一時的な失敗のやり直し、panic の扱いがまちまちだった。個々の包み関数
// (runEmu / buildFiles / buildBothLevels / rpRun …) は呼び出し側の読みやすさのために残し、中身をここに寄せる。

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haramako/fc/internal/ir"
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
	Run         bool              // emu で走らせる
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
		var stdout, logs strings.Builder
		opt := &BuildOptions{Target: s.Target, Dir: dir, BuildDir: filepath.Join(dir, "b"), Out: r.Out, OptimizeLevel: s.Level,
			Run: s.Run, CompileOnly: s.CompileOnly, Stdout: &stdout, LogOut: &logs, MaxCycles: maxCycles,
			Debug: s.Debug, LogEveryStatement: s.LogEvery, MisclassifyResident: s.Misclassify, Defines: s.Defines, Config: s.Config}
		r.Res, r.Err = NewCompiler(absRepoRoot).BuildContext(t.Context(), main, opt)
		r.Stdout, r.LogOut = stdout.String(), logs.String()
		var ce *CommandError
		if retry < 2 && errors.As(r.Err, &ce) && strings.TrimSpace(ce.Result) == "" {
			continue
		}
		return r
	}
}

// exitError は Run の終了コードが 0 でなければそのエラー。
func (r *buildResult) exitError() error {
	if r.Err != nil {
		return r.Err
	}
	if r.Res != nil && r.Res.ExitCode != 0 {
		return fmt.Errorf("exit code %d: %s", r.Res.ExitCode, r.Stdout)
	}
	return nil
}
