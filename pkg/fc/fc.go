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
	"fmt"
	"io"
	"os"

	"github.com/haramako/fc/internal/cc65"
	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/driver"
	"github.com/haramako/fc/internal/fchome"
	"github.com/haramako/fc/internal/nes"
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
	OptimizeLevel int    // 1〜2 (0 は既定の 2、-1 は最適化なし)
	CompileOnly   bool   // アセンブル (.o) まで。リンクしない
	Debug         bool   // Mesen 用のデバッグ情報 (.dbg に fc のソース行、.mlb のラベル) を ROM の隣に書く
	SizeReport    bool   // 関数ごとのコードサイズ (Result.SizeReport)

	// Dir はソースの基準ディレクトリ (use / include の相対パスの起点)。"" なら作業ディレクトリ。
	// BuildDir は中間生成物 (.s / .inc / .o / ld65.cfg) の置き場所。"" なら <Dir>/.fc-build。
	Dir      string
	BuildDir string

	// Jobs はアセンブラ (ca65) の並列数。0 なら CPU 数。
	Jobs int

	// Stdout は Run 時のプログラム出力先 (既定 os.Stdout)。
	Stdout io.Writer

	// Defines は @(build) の const の上書き (`module.NAME=value`。fc.toml の [define.<module>] の後に当てる)。
	Defines []string

	// LibPath は追加のライブラリの探索先 (Dir 相対か絶対。ソースのディレクトリの後、fclib より前に探す)。
	LibPath []string

	// Offline は fc.toml の [lib.*] の git のライブラリを取ってこない (キャッシュに無ければエラー)。
	Offline bool
}

// Result はビルドの結果 (生成物のパスと、Run 時の終了コード)。
type Result = driver.Result

// Error はコンパイルエラー (位置付き)。外部コマンド (ca65 / ld65) の失敗は *CommandError。
type Error = diag.Error

// ErrorList は複数のコンパイルエラー (意味解析は文ごとに回復して集める)。errors.As で *Error を取ると最初の 1 件。
// 全部を出すには Errors(err) を使う。
type ErrorList = diag.ErrorList

// Errors は err に含まれるコンパイルエラーを全部返す (*Error なら 1 件、ErrorList なら全部、それ以外は nil)。
func Errors(err error) []*Error { return diag.Errors(err) }

// Warning は警告 (位置付き)。Result.Warnings / Check で返る。
type Warning = diag.Warning

// Position はソース位置 (file:line:col)。
type Position = syntax.Position

// CheckOptions は Check の設定。
type CheckOptions struct {
	Target  string   // TargetEmu (既定) / TargetNES
	Dir     string   // ソースの基準ディレクトリ ("" なら作業ディレクトリ)
	Defines []string // @(build) の const の上書き (Options.Defines と同じ)
}

// Check は src から始まるプログラムを検査する (ファイルは書かない)。警告を返し、エラーは *Error。
func (c *Compiler) Check(src string, opt CheckOptions) ([]Warning, error) {
	return c.c.Check(src, &driver.CheckOptions{Target: opt.Target, Dir: opt.Dir, Defines: opt.Defines})
}

// MigrateOptions は Migrate の設定。
type MigrateOptions struct {
	Target  string   // 入口としてコンパイルするときのターゲット (TargetEmu (既定) / TargetNES。fclib の探し方)
	Defines []string // @(build) の const の上書き (Options.Defines と同じ)
}

// Migrate は srcs (fc 2 / fc 3 / fc 4 のソース) を最新の版に書き換えた内容を返す (キーは srcs の要素。改行は LF)。
// fc 3 → fc 4 は意味の変わる所の書き換えなので、各ファイルを入口にしたプログラムとしてコンパイルする (エラーは *Error)。
func (c *Compiler) Migrate(srcs []string, opt MigrateOptions) (map[string][]byte, error) {
	return c.c.Migrate(srcs, &driver.MigrateOptions{Target: opt.Target, Defines: opt.Defines})
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
	home, cleanup, err := fchome.Resolve()
	if err != nil {
		return nil, err
	}
	return &Compiler{c: driver.NewCompiler(home), cleanup: cleanup}, nil
}

// Home は FC_HOME (fclib/ share/ を含むディレクトリ)。
func (c *Compiler) Home() string { return c.c.FCHome }

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
	res, err := c.c.BuildContext(ctx, src, &driver.BuildOptions{
		Target:        opt.Target,
		Out:           opt.Out,
		Run:           opt.Run,
		OptimizeLevel: opt.OptimizeLevel,
		CompileOnly:   opt.CompileOnly,
		Debug:         opt.Debug,
		SizeReport:    opt.SizeReport,
		Dir:           opt.Dir,
		BuildDir:      opt.BuildDir,
		Jobs:          opt.Jobs,
		Stdout:        opt.Stdout,
		Defines:       opt.Defines,
		LibPath:       opt.LibPath,
		Offline:       opt.Offline,
	})
	if err != nil || !opt.Run || opt.CompileOnly || res.Target != TargetNES {
		return res, err
	}
	// NES の ROM は内蔵の NES のランナーで console.exit まで走らせる (画面は描かない: console の出力と終了コードだけ)
	stdout := opt.Stdout
	if stdout == nil {
		stdout = os.Stdout
	}
	m, err := nes.LoadFile(res.Out)
	if err != nil {
		return res, err
	}
	m.Output = stdout
	if err := m.RunUntilExit(NESRunFrames); err != nil {
		return res, fmt.Errorf("%v: the built-in NES runner shows only console output (console.exit ends it); open the ROM in an emulator to see the screen", err)
	}
	res.ExitCode = m.ExitCode
	return res, nil
}

// NESRunFrames は Run の NES の ROM を内蔵のランナーで走らせるフレーム数の上限 (1 分)。
const NESRunFrames = 3600

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

// SizeReport は ld65 の --dbgfile (fcc build が ROM の隣に書く <out>.dbg、または自前のリンクで --dbgfile を指定したもの) から
// セグメントと関数ごとのコードサイズの表示を作る (fcc size)。top は表示する関数の数 (0 なら全部)。
// linkConfig はリンカ設定 (ld65 の -C のファイル) で、"" でなければ ROM の領域 (バンク) ごとの使用量と空きの表も足す。
func SizeReport(dbgFile, linkConfig string, top int) ([]string, error) {
	d, err := cc65.ParseDbgFile(dbgFile)
	if err != nil {
		return nil, err
	}
	r := d.SizeReport(top)
	if linkConfig != "" {
		lc, err := cc65.ReadLinkConfig(linkConfig)
		if err != nil {
			return nil, err
		}
		r = append(r, d.BankReport(lc)...)
	}
	return r, nil
}
