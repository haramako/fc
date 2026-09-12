// Package fc は FC コンパイラをライブラリとして使うための公開 API。
//
// CLI (cmd/fcc) はこの薄い層の上にあり、エディタ連携やビルドツールからも同じ入口を使う。
//
//	c, err := fc.New()          // FC_HOME (fclib/ share/) を解決する
//	defer c.Close()
//	res, err := c.Build(ctx, "main.fc", fc.Options{Target: fc.TargetNES, Out: "main.nes"})
//	var ce *fc.Error
//	if errors.As(err, &ce) { fmt.Println(ce.Pos, ce.Msg) }
package fc

import (
	"context"
	"io"

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/driver"
)

// ターゲットプラットフォーム。
const (
	TargetEmu = "emu" // 内蔵 6502 エミュレータ (テスト用。Run で実行できる)
	TargetNES = "nes" // iNES ROM
)

// Options はビルドの設定。
type Options struct {
	Target        string // TargetEmu (既定) / TargetNES
	Out           string // 出力ファイル (既定: a.bin / a.nes。作業ディレクトリ相対)
	Run           bool   // ビルド後に emu で実行する (Target == TargetEmu のみ)
	OptimizeLevel int    // 0〜2 (既定 2)
	CompileOnly   bool   // アセンブル (.o) まで。リンクしない

	// Dir はソースの基準ディレクトリ (use / include の相対パスの起点)。"" なら作業ディレクトリ。
	// BuildDir は中間生成物 (.s / .inc / .o / ld65.cfg) の置き場所。"" なら <Dir>/.fc-build。
	Dir      string
	BuildDir string

	// Jobs はアセンブラ (ca65) の並列数。0 なら CPU 数。
	Jobs int

	// Stdout は Run 時のプログラム出力先 (既定 os.Stdout)。
	Stdout io.Writer
}

// Result はビルドの結果 (生成物のパスと、Run 時の終了コード)。
type Result = driver.Result

// Error はコンパイルエラー (位置付き)。外部コマンド (ca65 / ld65) の失敗は *CommandError。
type Error = diag.Error

// CommandError は外部コマンドの失敗。
type CommandError = driver.CommandError

// Compiler はコンパイラのインスタンス。FC_HOME の解決結果を保持する。
type Compiler struct {
	c       *driver.Compiler
	cleanup func()
}

// New は FC_HOME を解決してコンパイラを作る。同梱データを展開した場合は Close で削除する。
func New() (*Compiler, error) {
	home, cleanup, err := driver.ResolveFCHome()
	if err != nil {
		return nil, err
	}
	return &Compiler{c: driver.NewCompiler(home), cleanup: cleanup}, nil
}

// NewWithHome は FC_HOME (fclib/ share/ を含むディレクトリ) を指定してコンパイラを作る。
func NewWithHome(fcHome string) *Compiler {
	return &Compiler{c: driver.NewCompiler(fcHome)}
}

// Close は New が展開した一時データを削除する。
func (c *Compiler) Close() {
	if c.cleanup != nil {
		c.cleanup()
		c.cleanup = nil
	}
}

// Build は src をビルドする。
func (c *Compiler) Build(ctx context.Context, src string, opt Options) (*Result, error) {
	return c.c.BuildContext(ctx, src, &driver.BuildOptions{
		Target:        opt.Target,
		Out:           opt.Out,
		Run:           opt.Run,
		OptimizeLevel: opt.OptimizeLevel,
		CompileOnly:   opt.CompileOnly,
		Dir:           opt.Dir,
		BuildDir:      opt.BuildDir,
		Jobs:          opt.Jobs,
		Stdout:        opt.Stdout,
	})
}
