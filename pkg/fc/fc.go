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
	"errors"
	"io"

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/driver"
	"github.com/haramako/fc/internal/migrate"
	"github.com/haramako/fc/internal/syntax"
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

// Warning は警告 (位置付き)。Result.Warnings / Check で返る。
type Warning = diag.Warning

// CheckOptions は Check の設定。
type CheckOptions struct {
	Target string // TargetEmu (既定) / TargetNES
	Dir    string // ソースの基準ディレクトリ ("" なら作業ディレクトリ)
}

// Check は src から始まるプログラムを検査する (ファイルは書かない)。警告を返し、エラーは *Error。
func (c *Compiler) Check(src string, opt CheckOptions) ([]Warning, error) {
	return c.c.Check(src, &driver.CheckOptions{Target: opt.Target, Dir: opt.Dir})
}

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

// Format は fc ソースを正規形に整形する (fcc fmt)。構文エラーは *Error で返す。
// CRLF は LF に正規化される。
func Format(src []byte, filename string) ([]byte, error) {
	out, err := syntax.Format(src, filename)
	if err != nil {
		var se *syntax.Error
		if errors.As(err, &se) {
			return nil, &diag.Error{Msg: se.Msg, Pos: se.Position()}
		}
		return nil, err
	}
	return out, nil
}

// Visibility は Migrate での `public` の付け方。
type Visibility = migrate.Visibility

const (
	VisibilityMinimal  = migrate.Minimal  // 他モジュールから参照されている宣言だけ public (既定)
	VisibilityPreserve = migrate.Preserve // v1 の実効可視性を保ち、さらに参照されているものを public (ライブラリ向け)
)

// MigrateOptions は fc 1 → fc 2 の移行 (fcc migrate) の設定。
type MigrateOptions struct {
	Target     string            // TargetEmu (既定) / TargetNES
	Dir        string            // ソースの基準ディレクトリ ("" なら作業ディレクトリ)
	Mains      []string          // 解析の起点 (Dir 相対)。全 main の参照を合わせて可視性を決める
	Libs       []string          // ライブラリのディレクトリ (preserve で移行)
	Visibility Visibility        // Libs 以外の既定
	Textmaps   map[string]string // include("macro.rb") を置き換える const NAME = textmap("PATH")
	Write      bool              // 書き込む (偽なら対象を報告するだけ)
	Force      bool              // asm が変わっても書き込んだままにする
	Out        io.Writer         // 報告の出力先 (nil なら os.Stdout)
}

// MigrateResult は移行の結果。
type MigrateResult = driver.MigrateResult

// Migrate は v1 ソースを文法 v2 に書き換える。Write のとき、書き換え後に再コンパイルして
// asm が一致しなければ元に戻してエラーを返す (Force で受け入れる)。
func (c *Compiler) Migrate(opt MigrateOptions) (*MigrateResult, error) {
	return c.c.Migrate(&driver.MigrateOptions{
		Target: opt.Target, Dir: opt.Dir, Mains: opt.Mains, Libs: opt.Libs,
		Visibility: opt.Visibility, Textmaps: opt.Textmaps,
		Write: opt.Write, Force: opt.Force, Out: opt.Out,
	})
}
