package driver

// パイプライン統括 (ソース → sema → codegen → ca65 → ld65 → emu 実行)。lib/fc/compiler.rb 由来。
// base.asm / ld65.cfg のテンプレートは小さく静的なので text/template を使わず文字列生成している。

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/haramako/fc/internal/cc65"
	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/extmacro"
	"github.com/haramako/fc/internal/fclog"
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/project"
	"github.com/haramako/fc/internal/sema"
	"github.com/haramako/fc/internal/starmacro"
	"github.com/haramako/fc/internal/syntax"
)

// toolSlots は外部のツール (ca65 / ld65) を同時に起動する数の上限 (プロセス全体。Compiler ごとの並列 (jobs) とは別)。テストのように
// 1 つのプロセスで多くのビルドを並べると、ビルドごとに CPU の数だけ ca65 を起動して数百のプロセスになり、Windows がプロセスを
// 作れなくなっていた ("Not enough memory resources are available to process this command")。
var toolSlots = make(chan struct{}, max(4, 2*runtime.NumCPU()))

// DefaultBuildDirName はソースディレクトリ直下に作る中間生成物ディレクトリの名前。
const DefaultBuildDirName = ".fc-build"

// CommandError は外部コマンド (ca65 / ld65) の失敗。
type CommandError struct {
	Msg     string
	Command []string
	Result  string // コマンドの出力
}

// Error はメッセージ・コマンド行・出力をまとめて返す (アセンブラのエラー行がそのまま読めるように)。
func (e *CommandError) Error() string {
	return e.Msg + "\n" + strings.Join(e.Command, " ") + "\n" + e.Result
}

type BuildOptions struct {
	Target string // emu / nes (デフォルト emu)
	// LibPath は追加のライブラリの探索先 (Dir 相対か絶対)。use / @include と asm の include で、ソースのディレクトリの後、fclib
	// より前に探す (fcc test がテストするモジュールのディレクトリを足す。fc.toml の [lib.*] もここに入る: Agent/wiki/plans/v4-stdlib.md §9)
	LibPath []string
	// Offline は fc.toml の [lib.*] の git のライブラリを取ってこない (キャッシュに無ければエラー。Agent/wiki/plans/v4-stdlib.md §9)
	Offline       bool
	Out           string // 出力ファイル (デフォルト a.bin / a.nes。作業ディレクトリ相対)
	OptimizeLevel int    // -O。0 は未指定 (既定の 2)、-1 は最適化なし (`fcc -O 0`)
	CompileOnly   bool
	Debug         bool // -g: fc のソース位置を .dbg line で埋め、ROM の隣に Mesen 用の .dbg / .mlb を書く (@log の地点: Result.Log)
	// LogEveryStatement はテスト用: 文ごとに変数を全部出す @log を置く (sema.Program.LogEveryStatement)
	LogEveryStatement bool
	// MisclassifyResident はテスト用: 常駐レジスタの見積もりをわざと外す (codegen.Llc.MisclassifyResident)
	MisclassifyResident bool
	SizeReport          bool   // --size-report: 関数ごとのコードサイズ (Result.SizeReport)
	SizeHTML            string // --size-html: 同じ情報を HTML のファイルに (internal/sizehtml)
	// Config は調査用の設定 (パスの入れ切り・トレース・検証。ir/config.go)。nil なら環境変数 (FC_DISABLE など) から作る
	Config *ir.Config

	// Dir はソースの基準ディレクトリ (use / include / incbin の相対パスの起点)。"" なら作業ディレクトリ。
	// BuildDir は中間生成物 (.s / .inc / .o / base.o / ld65.cfg) の置き場所。"" なら <Dir>/.fc-build。
	// CLI はどちらも既定のままなので外部挙動は従来どおり (Agent/discussions/2026-09-12-v2-plan.md G6)。
	Dir      string
	BuildDir string

	// Jobs は ca65 を同時に走らせる数。0 なら CPU 数。1 で逐次。
	Jobs int

	// Defines は CLI の -D (`module.NAME=value`)。fc.toml の [define.<module>] の後に当てる (Agent/discussions/2026-09-20-v3-plan.md §1)
	Defines []string
}

// Result はビルドの結果。走らせるのは internal/runner (ビルドと分けてある)。
type Result struct {
	Target     string         // ビルドしたターゲット (-t を省けば fc.toml の [target] の有無で決まる)
	Out        string         // 出力ファイル (CompileOnly なら "")
	MapFile    string         // ld65 のマップファイル (CompileOnly なら "")
	DbgFile    string         // ld65 の --dbgfile (CompileOnly なら "")。Debug なら Mesen 用の .mlb も隣に書く
	Objects    []string       // fc ソースから生成したオブジェクトファイル (モジュール順 = リンク順)
	BuildDir   string         // 中間生成物ディレクトリ
	Warnings   []diag.Warning // 警告 (構文検査 + 意味解析。ファイル・位置順)
	FarCalls   []sema.FarCall // far call になった呼び出し (options(farcall: true) のとき。fcc build -d で表示)
	StaticZp   int            // 静的フレームの使用量 (ゼロページ側 FC_SZP / RAM 側 FC_SRAM)
	StaticRam  int
	Frames     []string         // 静的フレームの配置の要約 (fcc build -d で表示)
	Defines    []sema.DefineUse // @(build) の const の上書き (fc.toml / -D。fcc build -d で表示)
	Libs       []string         // fc.toml の [lib.*] の要約: ライブラリと、そこから使ったモジュール (fclib を置き換えたもの) (fcc build -d で表示)
	Interfaces []string         // interface ごとの実装と ID (fcc build -d で表示)
	Log        *fclog.LogFile   // @log の地点 (Debug のとき。emu で走らせるときに出す: runner.Options.Log)
	SizeReport []string         // 関数ごとのコードサイズ (fcc build --size-report で表示)
	// ResidentFixes は常駐レジスタの見積もりが外れて、退避 / 復帰に直した命令の数 (codegen.Llc.ResidentFixes)
	ResidentFixes int
}

// Compiler はコンパイラ。持つのは FC_HOME とビルドをまたぐ数だけで、ビルドごとの状態は compilation に置く (同じ Compiler で
// 並行にビルドしてよい。中間生成物ディレクトリは別々にすること)。
type Compiler struct {
	FCHome  string       // fclib/ share/ を含むディレクトリ
	asmRuns atomic.Int64 // 実際に ca65 を起動した回数 (オブジェクトの再利用のテスト用。asmcache.go)
}

func NewCompiler(fcHome string) *Compiler {
	return &Compiler{FCHome: fcHome}
}

// compilation は 1 回のビルド (fcc build / check / migrate の 1 ファイル) の状態。BuildContext・Check・Migrate が
// 呼ぶたびに作る。
type compilation struct {
	*Compiler
	ctx      context.Context
	jobs     int                   // ca65 の並列数
	libDirs  []string              // 追加のライブラリの探索先 (optLibs と fc.toml の [lib.*])
	libs     []project.ResolvedLib // fc.toml の [lib.*] (fcc build -d の要約)
	optLibs  []string              // BuildOptions.LibPath を絶対パスにしたもの
	offline  bool                  // BuildOptions.Offline
	target   string
	dir      string // ソースの基準ディレクトリ (BuildOptions.Dir)
	buildDir string // 中間生成物ディレクトリ (BuildOptions.BuildDir)
	prog     *sema.Program
	layout   *project.BankLayout // fc.toml のバンクの表 (nil なら options(bank_count / bank) で配置する。layout.go)
	hashes   *hashMemo           // このビルドの中のファイルのハッシュ (asmcache.go)
	cfg      *ir.Config          // 調査用の設定 (BuildOptions.Config)
	// globalInit は初期値のある変数があるビルド (runtime.asm を FC_GLOBAL_INIT つきでアセンブルする。globalInitTable)
	globalInit bool
	linkCfg    string // リンクに使ったリンカ設定のパス (--size-report のバンクの表)
	// macroServers / macroScripts は fc.toml の [macro_server.*] / [macro_script.*] (外部コマンドと Starlark の定数マクロ:
	// projectMacros)
	macroServers []*extmacro.Server
	macroScripts []*starmacro.Script
}

// newCompilation はビルドの状態を作る。driver の入口 (BuildContext・Check・Migrate) はみなここを通るので、ソースの基準
// ディレクトリ dir ("" なら作業ディレクトリ) はここで 1 度だけ正規の形 ("." など。相対のまま: 生成物に埋め込むファイルの参照と
// エラーの位置は基準からの相対で出す) にし、以降は c.dir だけを使う。target は "" なら既定 (defaultTarget)。
func (c *Compiler) newCompilation(ctx context.Context, dir, target string) *compilation {
	dir = filepath.Clean(dir) // "" → "."
	if target == "" {
		target = defaultTarget(dir)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return &compilation{Compiler: c, ctx: ctx, dir: dir, target: target, hashes: newHashMemo()}
}

// findShare は share以下のファイルを検索する (target優先)。
func (c *compilation) findShare(filename string) string {
	dir := filepath.Join(c.FCHome, "share")
	if p := filepath.Join(dir, c.target, filename); fileExists(p) {
		return p
	}
	if p := filepath.Join(dir, filename); fileExists(p) {
		return p
	}
	panic(fmt.Sprintf("file '%s' not found in share directories", filename))
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// BuildContext はソースをビルドし、生成物の情報を返す。ctx のキャンセルは外部コマンド (ca65 / ld65) に伝わる。
// エラーは *diag.Error (コンパイルエラー) または *CommandError (外部コマンドの失敗)。
func (c *Compiler) BuildContext(ctx context.Context, filename string, opt *BuildOptions) (*Result, error) {
	return c.newCompilation(ctx, opt.Dir, opt.Target).build(filename, opt)
}

// recoverError は深い所から panic で投げたコンパイルエラー (*diag.Error) と外部コマンドの失敗 (*CommandError) を err にする
// (build・check・migrate の入口で defer する。それ以外の panic はそのまま)。
func recoverError(err *error) {
	if r := recover(); r != nil {
		switch e := r.(type) {
		case *diag.Error:
			*err = e
		case *CommandError:
			*err = e
		default:
			panic(r)
		}
	}
}

// checkTarget はターゲットが emu / nes か (ほかは fclib/<target> が無く、後で ca65 の分かりにくい失敗になっていた)。
func (c *compilation) checkTarget() error {
	if c.target != "emu" && c.target != "nes" {
		return &diag.Error{Msg: fmt.Sprintf("unknown target %q (emu / nes)", c.target)}
	}
	return nil
}

// build は BuildContext の本体。
func (c *compilation) build(filename string, opt *BuildOptions) (result *Result, err error) {
	defer recoverError(&err)
	if err := c.checkTarget(); err != nil {
		return nil, err
	}
	if opt.Out == "" {
		if c.target == "nes" {
			opt.Out = "a.nes"
		} else {
			opt.Out = "a.bin"
		}
	}
	switch {
	case opt.OptimizeLevel == 0:
		opt.OptimizeLevel = 2 // 未指定
	case opt.OptimizeLevel < 0:
		opt.OptimizeLevel = 0 // -O 0
	}
	if opt.Config == nil {
		opt.Config = ir.ConfigFromEnv()
	}
	c.cfg = opt.Config
	c.buildDir = opt.BuildDir
	if c.buildDir == "" {
		c.buildDir = filepath.Join(c.dir, DefaultBuildDirName)
	}
	for _, d := range opt.LibPath {
		if !filepath.IsAbs(d) {
			d = filepath.Join(c.dir, d)
		}
		if a, err := filepath.Abs(d); err == nil {
			d = a
		}
		c.optLibs = append(c.optLibs, filepath.ToSlash(d)) // 探索先 libDirs は projectDefines が fc.toml の [lib.*] と合わせて作る
	}
	c.offline = opt.Offline
	c.jobs = opt.Jobs
	if c.jobs <= 0 {
		c.jobs = runtime.NumCPU()
	}

	if err := os.MkdirAll(c.buildDir, 0o777); err != nil {
		return nil, err
	}
	result = &Result{BuildDir: c.buildDir, Target: c.target}

	// 前段: 意味解析 → 全関数の最適化と割付 → 静的フレームの配置 (frontend.go)
	defs, derr := c.projectDefines(opt.Defines)
	if derr != nil {
		return nil, derr
	}
	front, ferr := c.compileFront(&frontOptions{
		Main: filename, Defines: defs, OptimizeLevel: opt.OptimizeLevel,
		Debug: opt.Debug, LogEveryStatement: opt.LogEveryStatement, Config: opt.Config,
		MisclassifyResident: opt.MisclassifyResident, DebugFile: c.debugFileFunc(opt),
	})
	if ferr != nil {
		return nil, ferr
	}
	prog, llc, plan := front.Prog, front.Llc, front.Plan
	c.prog = prog
	result.Warnings = collectWarnings(prog)
	result.FarCalls = prog.FarCalls
	result.Defines = sortedDefines(prog.Defines)
	result.Libs = c.libSummary(prog)
	result.Interfaces = prog.InterfaceSummary()
	if err := writeIfChanged(filepath.Join(c.buildDir, "_frames.inc"), []byte(strings.Join(plan.Inc, "\n"))); err != nil {
		return nil, err
	}
	result.StaticZp, result.StaticRam = plan.ZpUsed, plan.RamUsed
	result.Frames = plan.Report
	for _, mod := range prog.Modules.List() {
		asm, inc, lerr := llc.Compile(mod)
		if lerr != nil {
			return nil, lerr
		}
		if err := writeIfChanged(filepath.Join(c.buildDir, fmt.Sprintf("_%s.inc", mod.Id)), []byte(strings.Join(inc, "\n"))); err != nil {
			return nil, err
		}
		if err := writeIfChanged(filepath.Join(c.buildDir, fmt.Sprintf("_%s.s", mod.Id)), []byte(strings.Join(asm, "\n"))); err != nil {
			return nil, err
		}
	}
	result.ResidentFixes = llc.ResidentFixes

	// assemble (アセンブラ -> オブジェクトファイル)。各 .s は独立なので並列にアセンブルする
	// (.inc は上で全部書き終えている)。objs の並び = リンク順はモジュール順のまま
	var objs, sources []string
	for _, mod := range prog.Modules.List() {
		objs = append(objs, filepath.Join(c.buildDir, fmt.Sprintf("_%s.o", mod.Id)))
		sources = append(sources, filepath.Join(c.buildDir, fmt.Sprintf("_%s.s", mod.Id)))
	}
	sources = append(sources, c.findShare("runtime.asm"), filepath.Join(c.FCHome, "fclib", c.target, "runtime_init.asm"))
	if src, err := c.defaultInterrupts(prog.Modules.List()); err != nil {
		return nil, err
	} else if src != "" {
		sources = append(sources, src)
		objs = append(objs, c.objPath(src))
	}
	if fc := c.farcallAsm(); fc != "" {
		sources = append(sources, fc)
	}
	if src, err := c.globalInitTable(prog.Modules.List()); err != nil {
		return nil, err
	} else if src != "" {
		sources = append(sources, src)
		objs = append(objs, c.objPath(src))
	}
	if err := c.assembleAll(sources); err != nil {
		return nil, err
	}
	result.Objects = objs
	if opt.CompileOnly {
		return result, nil
	}

	baseObj := c.makeBase()

	result.Out = opt.Out
	result.MapFile, result.DbgFile = c.link(baseObj, objs, opt)
	// dbgfile は要るときに 1 回だけ読む (castle では 13MB。検査と -g のラベルで 2 回読んでいた)
	var dbgParsed *cc65.DbgFile
	loadDbg := func() (*cc65.DbgFile, error) {
		if dbgParsed == nil {
			d, err := cc65.ParseDbgFile(result.DbgFile)
			if err != nil {
				return nil, err
			}
			dbgParsed = d
		}
		return dbgParsed, nil
	}
	if err := c.checkAddressVars(loadDbg); err != nil {
		return nil, err
	}
	var logFile *fclog.LogFile
	stem := strings.TrimSuffix(opt.Out, filepath.Ext(opt.Out)) // ROM の隣のファイル (.fclog.json / .mlb)
	if opt.Debug || opt.SizeReport || opt.SizeHTML != "" {
		dbg, err := loadDbg()
		if err != nil {
			return nil, err
		}
		if opt.Debug && len(llc.LogSites) > 0 {
			// @log の地点 (<rom>.fclog.json / .fclog.lua)
			if logFile, err = fclog.Build(llc.LogSites, dbg, c.target, llc.DebugFile); err != nil {
				return nil, err
			}
			if err := fclog.WriteFiles(stem, logFile); err != nil {
				return nil, err
			}
			result.Warnings = append(result.Warnings, fclog.Warnings(llc.LogSites)...)
		}
		if opt.Debug && c.target == "nes" {
			battery := false
			if b, err := os.ReadFile(opt.Out); err == nil && len(b) > 6 {
				battery = b[6]&2 != 0
			}
			if err := dbg.WriteMlb(stem+".mlb", battery); err != nil {
				return nil, err
			}
		}
		if opt.SizeReport {
			result.SizeReport = c.sizeReport(dbg)
		}
		if opt.SizeHTML != "" {
			if err := c.writeSizeHTML(dbg, opt.SizeHTML, filepath.Base(filename)); err != nil {
				return nil, err
			}
		}
	}

	result.Log = logFile
	return result, nil
}

// defaultTarget は -t を省いたときのターゲット: fc.toml に [target] があれば nes、無ければ emu (dir は正規化した基準ディレクトリ)。
func defaultTarget(dir string) string {
	if cfg, err := project.FindConfig(dir); err == nil && cfg.Sections["target"] != nil {
		return "nes"
	}
	return "emu"
}

// libPath は use / include の検索パス (カレント → 追加のライブラリ (BuildOptions.LibPath) → fclib → fclib/<target>)。
func (c *compilation) libPath() []string {
	p := []string{"."}
	p = append(p, c.libDirs...)
	return append(p, filepath.ToSlash(filepath.Join(c.FCHome, "fclib")), filepath.ToSlash(filepath.Join(c.FCHome, "fclib", c.target)))
}

// makeBase は base.s (ランタイムの土台: ZP のレジスタ・スタック・FC_FARCALL などの定義) を生成してアセンブルする。
// options(base: "data.asm") があれば生成せず、そのファイル (Dir 相対) をアセンブルする (castle のように ZP 配置や
// iNES ヘッダを自前で持つプロジェクト。fc の領域 (L / reg / FC_FASTCALL_REG / FC_SZP / FC_SRAM / FC_SP / FC_FARCALL) を
// 同じ名前で定義すること。docs/reference/assembly.md の「リンクと土台の指定」)。戻り値はリンクに渡すオブジェクト。
func (c *compilation) makeBase() string {
	opts := c.prog.Options
	if base, ok := opts.Get("base"); ok && base.Kind == ir.OptStr {
		path := filepath.Join(c.dir, base.Str)
		c.ca65(path)
		return c.objPath(path)
	}
	assemble := func(str string) string {
		path := filepath.Join(c.buildDir, "base.s")
		if err := writeIfChanged(path, []byte(str)); err != nil {
			panic(err)
		}
		c.ca65(path)
		return c.objPath(path)
	}
	if c.layout != nil && c.target == "nes" {
		// fc.toml の [target] から (layout.go)
		l := c.layout
		flags := l.Mirror
		if l.Battery {
			flags |= 2
		}
		return assemble(c.baseAsmTemplate(l.PRGSize/0x4000, l.CHRSize/0x2000, flags, l.Profile.INES)) // CHR 0 は CHR-RAM
	}
	inesmap := 0
	if m, ok := opts.Get("mapper"); ok {
		switch m.Kind {
		case ir.OptStr:
			switch m.Str {
			case "MMC0":
				inesmap = 0
			case "MMC3":
				inesmap = 4
			}
		case ir.OptInt:
			inesmap = m.Int
		}
	}

	bankCount := 4
	if bc, ok := opts.Int("bank_count"); ok {
		bankCount = bc
	}
	inesprg := bankCount / 2
	ineschr := 1
	if cb, ok := opts.Int("char_banks"); ok {
		ineschr = cb
	}

	return assemble(c.baseAsmTemplate(inesprg, ineschr, 1, inesmap))
}

type bankInfo struct {
	size, org int
}

// link はオブジェクトファイルをリンクし、マップファイルのパスを返す。
func (c *compilation) link(baseObj string, objs []string, opt *BuildOptions) (mapFile, dbgFile string) {
	opts := c.prog.Options
	cfgPath := filepath.Join(c.buildDir, "ld65.cfg")
	if custom, ok := opts.Get("linker_config"); ok && custom.Kind == ir.OptStr {
		// options(linker_config: "../ld65.cfg"): 自前のリンカ設定 (Dir 相対)。bank / org は配置に使われない (far call の判定だけ)
		cfgPath = filepath.Join(c.dir, custom.Str)
	} else if c.layout != nil && c.target == "nes" {
		c.writeLayoutConfig() // fc.toml のバンクの表から (layout.go)
	} else {
		c.writeLinkerConfig()
	}
	c.linkCfg = cfgPath
	stem := strings.TrimSuffix(opt.Out, filepath.Ext(opt.Out))
	mapFile, dbgFile = stem+".map", stem+".dbg"
	args := []string{"-m", mapFile, "--dbgfile", dbgFile, "-o", opt.Out, "-C", cfgPath,
		baseObj, filepath.Join(c.buildDir, "runtime_init.o"), filepath.Join(c.buildDir, "runtime.o")}
	if c.farcallAsm() != "" {
		args = append(args, filepath.Join(c.buildDir, "farcall.o"))
	}
	args = append(args, objs...)
	if extra, ok := opts.Get("link"); ok && extra.Kind == ir.OptStr {
		// options(link: "a.o b.o lib.lib"): 追加のオブジェクト / ライブラリ (空白区切り、Dir 相対)
		for _, f := range strings.Fields(extra.Str) {
			args = append(args, filepath.Join(c.dir, f))
		}
	}
	c.sh("ld65", args...)
	return mapFile, dbgFile
}

// writeLinkerConfig は fc の ld65.cfg (バンク構成は main の options(bank_count / char_banks) とモジュールの options(bank / org)) を書く。
func (c *compilation) writeLinkerConfig() {
	opts := c.prog.Options

	ineschr := 1
	if cb, ok := opts.Int("char_banks"); ok {
		ineschr = cb
	}

	var banks []*bankInfo
	if bc, ok := opts.Int("bank_count"); ok {
		for i := 0; i < bc; i++ {
			size := 0x2000
			if i == bc-1 {
				size -= 6 // vectors size
			}
			org := 0x8000 + (i%4)*0x2000
			banks = append(banks, &bankInfo{size: size, org: org})
		}
	} else {
		banks = []*bankInfo{{size: 0x8000 - 6, org: 0x8000}}
	}

	type segInfo struct {
		name string
		bank int
	}
	var segs []segInfo
	for _, m := range c.prog.Modules.List() {
		bank := 0
		if b, ok := m.Options.Int("bank"); ok && c.target == "nes" { // emu にバンクは無い (bank は far call の判定にだけ使う)
			bank = b
		}
		if bank < 0 {
			bank = len(banks) + bank
		}
		if bank < 0 || bank >= len(banks) {
			b, _ := m.Options.Int("bank")
			panic(&diag.Error{Msg: fmt.Sprintf("module %s: options(bank: %d) is out of range (bank_count is %d)", m.Id, b, len(banks)),
				Pos: syntax.Position{Filename: m.Path}})
		}
		if org, ok := m.Options.Int("org"); ok {
			banks[bank].org = org
		}
		segs = append(segs, segInfo{name: m.Id, bank: bank})
	}

	var cfg string
	if c.target == "nes" {
		var b strings.Builder
		b.WriteString("# memory config for ld65\n\nMEMORY {\n")
		b.WriteString(c.memoryMap().LinkerMemory())
		b.WriteString("  HEADER: start = $0000, size = $10, file = %O, fill = yes;\n")
		for i, bank := range banks {
			fmt.Fprintf(&b, "  ROM%d: start = $%x, size = $%x, file = %%O, fill = yes, define = yes, bank = %d;\n", i, bank.org, bank.size, i)
		}
		b.WriteString("  ROMV: start = $fffa, size = $0006, file = %O, fill = yes;\n")
		fmt.Fprintf(&b, "  ROMC: start = $0000, size = $%x, file = %%O, fill = yes;\n", ineschr*0x2000)
		b.WriteString("}\n\nSEGMENTS {\n")
		b.WriteString("  HEADER: load = HEADER, type = ro;\n")
		fmt.Fprintf(&b, "  CODE: load = ROM%d, type = ro, define = yes;\n", len(banks)-1)
		fmt.Fprintf(&b, "  FC_RUNTIME: load = ROM%d, type = ro, define = yes;\n", len(banks)-1)
		b.WriteString("  VECTORS: load = ROMV, type = rw;\n")
		b.WriteString("  CHARS: load = ROMC, type = rw, optional = yes;\n")
		b.WriteString("  BSS: load = SRAM, type= bss, define = yes;\n")
		b.WriteString("  ZEROPAGE: load = ZP, type = zp;\n")
		b.WriteString("  FC_ZEROPAGE: load = ZP, type = zp;\n")
		b.WriteString("  FC_STACK: load = ZP_STACK, type = zp;\n")
		for _, seg := range segs {
			fmt.Fprintf(&b, "  %s: load = ROM%d, type = ro;\n", seg.name, seg.bank)
		}
		b.WriteString("}\n")
		cfg = b.String()
	} else {
		var b strings.Builder
		b.WriteString("# memory config for ld65\n\nMEMORY {\n")
		b.WriteString(c.memoryMap().LinkerMemory())
		b.WriteString("  ROMV: start = $1000, size = 3, type = rw, define = yes;\n")
		b.WriteString("  ROM: start = $1003, size = $6FFD, file = %O, fill = no, define = yes, bank = 0;\n")
		b.WriteString("  SRAM_EX: start = $8000, size = $7F00, type = rw, define = yes;\n") // 大きな配列用 (options(segment: "BSS_EX"))
		b.WriteString("  CHARS: start = $0000, size = $10000, type = rw, fill = no, define = yes;\n")
		b.WriteString("}\n\nSEGMENTS {\n")
		b.WriteString("  CODE: load = ROM, type = ro, define = yes;\n")
		b.WriteString("  FC_RUNTIME: load = ROM, type = ro, define = yes;\n")
		b.WriteString("  VECTORS: load = ROMV, type = rw;\n")
		b.WriteString("  BSS: load = SRAM, type= bss, define = yes;\n")
		b.WriteString("  BSS_EX: load = SRAM_EX, type= bss, define = yes, optional = yes;\n")
		b.WriteString("  ZEROPAGE: load = ZP, type = zp;\n")
		b.WriteString("  FC_ZEROPAGE: load = ZP, type = zp;\n")
		b.WriteString("  FC_STACK: load = ZP_STACK, type = zp;\n")
		for _, seg := range segs {
			fmt.Fprintf(&b, "  %s: load = ROM, type = ro;\n", seg.name)
		}
		b.WriteString("  CHARS: load = CHARS, type = bss, optional = yes;\n")
		b.WriteString("}\n")
		cfg = b.String()
	}

	if err := writeIfChanged(filepath.Join(c.buildDir, "ld65.cfg"), []byte(cfg)); err != nil {
		panic(err)
	}
}

// globalInitTable は、初期値のある変数を持つモジュール (ir.Module.HasGlobalInit) の初期値の記録を並べた表
// __fc_init_table の asm (_fc_init.s) を書いてそのパスを返し、runtime.asm を FC_GLOBAL_INIT つきでアセンブルさせる
// (起動のときに fc_global_init が写す)。初期値のある変数が無ければ "" (runtime も今までと同じ)。
func (c *compilation) globalInitTable(mods []*ir.Module) (string, error) {
	var b strings.Builder
	var syms []string
	for _, m := range mods {
		if m.HasGlobalInit() {
			syms = append(syms, m.InitSymbol())
		}
	}
	c.globalInit = len(syms) > 0
	if !c.globalInit {
		return "", nil
	}
	b.WriteString("; グローバル変数の初期値の記録の表 (fcc が生成。share/runtime.asm の fc_global_init が読む)\n")
	b.WriteString("\t.export __fc_init_table\n")
	for _, s := range syms {
		fmt.Fprintf(&b, "\t.import %s\n", s)
	}
	b.WriteString(".segment \"FC_RUNTIME\"\n__fc_init_table:\n")
	for _, s := range syms {
		fmt.Fprintf(&b, "\t.word %s\n", s)
	}
	b.WriteString("\t.word 0\n")
	path := filepath.Join(c.buildDir, "_fc_init.s")
	return path, writeIfChanged(path, []byte(b.String()))
}

// defaultInterrupts は割り込みの入口 (_interrupt / _interrupt_irq。share/runtime.asm の NMI / IRQ が呼ぶ) を定義するものが
// 無ければ、何もしない入口の asm (_interrupts.s) を書いてそのパスを返す (全部あれば "")。fc の関数 (Id がその名前。
// options(symbol:) の extern も) か、include した asm がその名前を参照していれば (castle の ppu.asm の `.export _interrupt`)
// 定義があるとみなす。stdio を使わないプログラム (console だけ) が、入口が無いというリンクのエラーになっていた。
func (c *compilation) defaultInterrupts(mods []*ir.Module) (string, error) {
	defined := map[string]bool{}
	for _, m := range mods {
		for _, d := range m.Defs {
			if d.Kind == ir.DefCode && d.Lambda != nil {
				defined[d.Lambda.Id] = true
			}
		}
		for _, s := range m.AsmSymbols {
			defined[s] = true
		}
	}
	var b strings.Builder
	for _, sym := range []string{"_interrupt", "_interrupt_irq"} {
		if !defined[sym] {
			fmt.Fprintf(&b, "\t.export %s\n%s:\n", sym, sym)
		}
	}
	if b.Len() == 0 {
		return "", nil
	}
	src := "; fc が生成: 割り込みの入口を定義するモジュールが無いときの、何もしない入口\n.segment \"FC_RUNTIME\"\n" + b.String() + "\trts\n"
	path := filepath.Join(c.buildDir, "_interrupts.s")
	return path, writeIfChanged(path, []byte(src))
}

// farcallAsm は fc が用意する farcall トランポリン (Agent/wiki/design/farcall.md §3.4)。emu と、バンク切替の無い nes (MMC0) では
// 「そのまま飛ぶ」だけの fclib/<target>/farcall.asm を使う。バンク切替のあるマッパーはプロジェクトが farcall を用意する。
func (c *compilation) farcallAsm() string {
	if c.target == "nes" {
		if c.layout != nil && len(c.layout.Profile.Slots) > 0 {
			return "" // fc.toml のバンクの表でバンク切替のあるマッパー: トランポリンはプロジェクトが用意する
		}
		if m, ok := c.prog.Options.Get("mapper"); ok && c.layout == nil && !(m.Kind == ir.OptStr && m.Str == "MMC0" || m.Kind == ir.OptInt && m.Int == 0) {
			return ""
		}
	}
	p := filepath.Join(c.FCHome, "fclib", c.target, "farcall.asm")
	if !fileExists(p) {
		return ""
	}
	return p
}

// projectDefines は fc.toml (ソースの基準ディレクトリから親へ探す) と CLI の -D から @(build) の const の上書きを作る。
// fc.toml のバンクの表も読んで c.layout に入れる。
func (c *compilation) projectDefines(cli []string) (map[string]*sema.DefineUse, error) {
	cfg, err := project.FindConfig(c.dir)
	if err != nil {
		return nil, err
	}
	if c.layout, err = cfg.Layout(); err != nil {
		return nil, err
	}
	if c.macroServers, err = cfg.MacroServers(); err != nil {
		return nil, err
	}
	if c.macroScripts, err = cfg.MacroScripts(); err != nil {
		return nil, err
	}
	if c.layout != nil && c.layout.Fragment != "" && !filepath.IsAbs(c.layout.Fragment) {
		c.layout.Fragment = filepath.Join(filepath.Dir(cfg.Path), c.layout.Fragment)
	}
	// fc.toml の [lib.*] (git のものは取ってくる) を、BuildOptions.LibPath の後・fclib の前の探索先に
	libs, err := (&project.Resolver{Offline: c.offline, Log: project.StderrLog}).Resolve(cfg)
	if err != nil {
		return nil, err
	}
	dirs, err := project.LibDirs(libs, c.target)
	if err != nil {
		return nil, err
	}
	c.libs = libs
	c.libDirs = append([]string{}, c.optLibs...)
	for _, d := range dirs {
		c.libDirs = append(c.libDirs, filepath.ToSlash(d))
	}
	return cfg.Defines(cli)
}

// libSummary は fc.toml の [lib.*] の要約の行: ライブラリごとに場所と、使ったモジュール (fclib の同じ名前のモジュールを
// 置き換えたものには印)。
func (c *compilation) libSummary(prog *sema.Program) []string {
	var lines []string
	for _, l := range c.libs {
		src := l.Root
		if l.Git != "" {
			src = fmt.Sprintf("%s@%s (%s)", l.Git, shortCommit(l.Commit), l.Root)
		}
		lines = append(lines, fmt.Sprintf("  %s: %s", l.Name, src))
		root, _ := filepath.Abs(l.Root)
		var mods []string
		for _, m := range prog.Modules.List() {
			p, _ := filepath.Abs(m.Path)
			if rel, err := filepath.Rel(root, p); err != nil || strings.HasPrefix(rel, "..") {
				continue
			}
			name := m.Id
			for _, d := range []string{filepath.Join(c.FCHome, "fclib"), filepath.Join(c.FCHome, "fclib", c.target)} {
				if _, err := os.Stat(filepath.Join(d, filepath.Base(m.Path))); err == nil {
					name += " (replaces fclib)"
					break
				}
			}
			mods = append(mods, name)
		}
		if len(mods) > 0 {
			lines = append(lines, "    uses "+strings.Join(mods, ", "))
		}
	}
	return lines
}

func shortCommit(c string) string {
	if len(c) > 10 {
		return c[:10]
	}
	return c
}

// memoryMap は fc が生成する ld65.cfg と base.s の RAM の配置。
func (c *compilation) memoryMap() project.MemoryMap {
	return project.DefaultMemoryMap(c.target)
}

// banks は意味解析に渡すバンクの表 (fc.toml に無ければ nil)。
func (c *compilation) banks() map[string]sema.BankRef {
	if c.layout == nil {
		return nil
	}
	return c.layout.SemaBanks()
}

// checkDefines は上書きが宣言された @(build) の const に当たったかを検査する (ビルドに含まれないモジュールは警告)。
func (c *compilation) checkDefines(prog *sema.Program) error {
	return prog.CheckDefines(func(m string) bool { return project.ModuleExists(c.dir, c.libPath(), m) })
}

// sortedDefines は上書きの一覧 (キー順)。
func sortedDefines(m map[string]*sema.DefineUse) []sema.DefineUse {
	var r []sema.DefineUse
	for _, d := range m {
		r = append(r, *d)
	}
	sort.Slice(r, func(i, j int) bool { return r[i].Key < r[j].Key })
	return r
}

// debugFileFunc は -g のときの .dbg に書くファイル名 (sema のファイル参照 (Dir 相対) を ROM の隣から辿れる相対パスに)。
func (c *compilation) debugFileFunc(opt *BuildOptions) func(ref string) string {
	if !opt.Debug {
		return nil
	}
	outDir := filepath.Dir(opt.Out)
	return func(ref string) string {
		abs := ref
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(c.dir, ref)
		}
		if rel, err := filepath.Rel(outDir, abs); err == nil {
			return filepath.ToSlash(rel)
		}
		return filepath.ToSlash(abs)
	}
}

// baseAsmTemplate は base.s の雛形。
func (c *compilation) baseAsmTemplate(inesprg, ineschr, inesmir, inesmap int) string {
	header := "\t.exportzp FC_LOCAL\n" +
		"\t.exportzp FC_REG\n" +
		"\t.exportzp FC_STACK\n" +
		"\t.exportzp FC_FASTCALL_REG\n" +
		"\t.export FC_FARCALL\n" +
		"\t.export FC_FASTCALL_REG_SIZE : absolute\n" +
		fmt.Sprintf("FC_FASTCALL_REG_SIZE = %d\n", validated(fastcallRegSize(c.prog))) +
		"\t.exportzp FC_SZP\n" +
		"\t.exportzp FC_SP\n" +
		"\t.export FC_SRAM\n" +
		"\t.export FC_SZP_SIZE : absolute\n" +
		"\t.export FC_SRAM_SIZE : absolute\n" +
		fmt.Sprintf("FC_SZP_SIZE = %d\n", validated(staticZpSize(c.prog))) +
		fmt.Sprintf("FC_SRAM_SIZE = %d\n", validated(staticRamSize(c.prog))) +
		"\t.exportzp L \t\t\t\t\t; TODO: そのうち消すこと\n" +
		"\t.exportzp reg\n" +
		"\t.exportzp S\n" +
		"\t.import interrupt\n" +
		"\t.import start\n" +
		"\t.import interrupt_irq\n" +
		"\t\n"
	zeropage := ".segment \"FC_ZEROPAGE\": zeropage\n" +
		"\t\n" +
		"FC_LOCAL: .res $10\n" +
		"FC_REG: .res $10\n" +
		"FC_FASTCALL_REG: .res FC_FASTCALL_REG_SIZE\n" +
		"FC_SZP: .res FC_SZP_SIZE\n" + // 静的フレーム (ゼロページ側)。ここまでで $6F
		"FC_SP: .res 1\n" + // スタックの空き先頭 (S からのオフセット。X の代わり)。$70-$7E は ZEROPAGE セグメント用に空ける
		"\n" +
		".segment \"BSS\"\n" +
		"FC_FARCALL: .res 3\n" +
		"FC_SRAM: .res FC_SRAM_SIZE\n" + // 静的フレーム (RAM 側)
		"\n" +
		".segment \"FC_STACK\": zeropage\n" +
		"\t\n" +
		fmt.Sprintf("FC_STACK: .res $%02X\n", c.memoryMap().Stack.Size) +
		"\n" +
		"\tL = FC_LOCAL\n" +
		"\treg = FC_REG\n" +
		"\tS = FC_STACK\n" +
		"\t\n"
	if c.target == "nes" {
		return header +
			".segment \"HEADER\"\n\n" +
			fmt.Sprintf("    .byte   $4e,$45,$53,$1a ; \"NES\"^Z\n") +
			fmt.Sprintf("    .byte   %d       ; ines prg  - Specifies the number of 16k prg banks.\n", inesprg) +
			fmt.Sprintf("    .byte   %d       ; ines chr  - Specifies the number of 8k chr banks.\n", ineschr) +
			fmt.Sprintf("    .byte   %d   ; ines mir  - Specifies VRAM mirroring of the banks.\n", inesmir|((inesmap&15)<<4)) +
			fmt.Sprintf("    .byte   %d   ; ines map  - Specifies the NES mapper used.\n", inesmap>>4) +
			"    .byte   0,0,0,0,0,0,0,0 ; 8 zeroes\n\n" +
			zeropage +
			".segment \"VECTORS\"\n" +
			"\t.word interrupt\n" +
			"\t.word start\n" +
			"\t.word interrupt_irq\n"
	}
	return header +
		zeropage +
		".segment \"VECTORS\"\n" +
		"\tjmp start\n"
}

// assembleAll は複数の .s を c.jobs 並列でアセンブルする。
// 失敗したら最初 (sources 順) のエラーを返し、残りは ctx のキャンセルで止める。
func (c *compilation) assembleAll(sources []string) error {
	ctx, cancel := context.WithCancel(c.ctx)
	defer cancel()
	errs := make([]error, len(sources))
	sem := make(chan struct{}, c.jobs)
	var wg sync.WaitGroup
	for i, src := range sources {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, src string) {
			defer wg.Done()
			defer func() { <-sem }()
			if ctx.Err() != nil {
				return
			}
			if err := c.assembleCached(ctx, src); err != nil {
				errs[i] = err
				cancel()
			}
		}(i, src)
	}
	wg.Wait()
	// 最初に失敗したものが cancel で他を止めるので、止められた側 (出力なし) ではなく本当に失敗したものを返す
	var first error
	for _, err := range errs {
		if err == nil {
			continue
		}
		if ce, ok := err.(*CommandError); ok && strings.TrimSpace(ce.Result) != "" {
			return err
		}
		if first == nil {
			first = err
		}
	}
	return first
}

// ca65 はアセンブルを実行する (逐次。入力が前回と同じなら再利用する。失敗は CommandError を panic)。
func (c *compilation) ca65(path string) {
	if err := c.assembleCached(c.ctx, path); err != nil {
		panic(err)
	}
}

// ca65Args はアセンブルのコマンド引数を作る。
func (c *compilation) ca65Args(path string) []string {
	args := []string{
		"-g",
		"-o", c.objPath(path),
		"-I", filepath.Join(c.FCHome, "share"),
		"-I", c.buildDir,
		"-I", c.dir,
	}
	// use と同じ順: ソースのディレクトリ → ライブラリ → fclib (ライブラリが fclib の asm を置き換えられるように)
	for _, d := range c.libDirs {
		args = append(args, "-I", filepath.FromSlash(d))
	}
	args = append(args, "-I", filepath.Join(c.FCHome, "fclib"), "-I", filepath.Join(c.FCHome, "fclib", c.target))
	if c.globalInit && filepath.Base(path) == "runtime.asm" {
		args = append(args, "-D", "FC_GLOBAL_INIT=1") // 起動のときにグローバル変数の初期値を写す (globalInitTable)
	}
	if c.dir != "." {
		// .incbin は -I でなく --bin-include-dir で探す (既定は作業ディレクトリ)
		args = append(args, "--bin-include-dir", c.dir)
	}
	return append(args, path)
}

// sh は外部コマンドを実行する (失敗時は CommandError を panic)。
func (c *compilation) sh(name string, args ...string) {
	if err := c.run(c.ctx, name, args...); err != nil {
		panic(err)
	}
}

// run は外部コマンドを実行し、失敗なら *CommandError を返す。
func (c *compilation) run(ctx context.Context, name string, args ...string) error {
	select {
	case toolSlots <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-toolSlots }()
	cmd := exec.CommandContext(ctx, cc65.ToolPath(name), args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		code := -1
		if cmd.ProcessState != nil {
			code = cmd.ProcessState.ExitCode()
		} else {
			out = append(out, err.Error()...) // 起動できなかった理由 (出力が無く、終了コード -1 だけが残っていた)
		}
		return &CommandError{
			Msg:     fmt.Sprintf("%s returns %d", name, code),
			Command: append([]string{name}, args...),
			Result:  string(out),
		}
	}
	return nil
}

// checkAddressVars は @(address: N) の変数が、リンクした RAM のセグメント (fc の ZP・BSS・静的フレーム・スタック、[ram.*]、
// ほかの変数) と重ならないかを確かめる (fc の ZP の中に置くと、reg などと黙って重なっていた)。ROM・I/O の番地は RAM の
// セグメントでないので対象外。重なりを意図するなら storage alias を使う。
func (c *compilation) checkAddressVars(loadDbg func() (*cc65.DbgFile, error)) error {
	var vars []*ir.Def
	for _, m := range c.prog.Modules.List() {
		for _, d := range m.Defs {
			if d.AddressVar != "" && d.Equ != nil && d.Equ.IsInt {
				vars = append(vars, d)
			}
		}
	}
	if len(vars) == 0 {
		return nil
	}
	dbg, err := loadDbg()
	if err != nil {
		return err
	}
	ids := make([]int, 0, len(dbg.Segments))
	for id := range dbg.Segments {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	var errs diag.ErrorList
	for _, d := range vars {
		start, size := d.Equ.Int, d.Type.Size
		if size <= 0 {
			size = 1 // 長さ未定の配列 (`[]u8 @(address: …)`) は先頭だけ
		}
		for _, id := range ids {
			s := dbg.Segments[id]
			if s.RO || s.Ooffs >= 0 || s.Size <= 0 {
				continue // ROM に置かれるセグメント
			}
			if start < s.Start+s.Size && s.Start < start+size {
				errs = append(errs, &diag.Error{Pos: d.Pos, Msg: fmt.Sprintf("`%s` @(address: $%04X) overlaps segment %s ($%04X-$%04X; fc's registers, frames or other variables)",
					d.AddressVar, start, s.Name, s.Start, s.Start+s.Size-1)})
				break
			}
		}
	}
	if len(errs) > 0 {
		return errs
	}
	return nil
}

// projectMacros は fc.toml の [macro_server.*] と [macro_script.*] のマクロ。外部コマンドはマクロを最初に使ったときに起動し、
// 呼ぶ側が done で止める (ディスクのキャッシュは中間生成物ディレクトリ: internal/extmacro)。Starlark のスクリプトはここで実行して
// 一番上の関数を集める (internal/starmacro)。
func (c *compilation) projectMacros() (srcs []sema.MacroSource, done func(), err error) {
	done = func() {}
	if len(c.macroScripts) > 0 {
		set, err := starmacro.Load(c.macroScripts)
		if err != nil {
			return nil, done, err
		}
		srcs = append(srcs, set)
	}
	if len(c.macroServers) > 0 {
		cache := ""
		if c.buildDir != "" {
			cache = filepath.Join(c.buildDir, "macros")
		}
		pool, err := extmacro.NewPool(c.macroServers, cache)
		if err != nil {
			return nil, done, err
		}
		srcs, done = append(srcs, pool), pool.Close
	}
	return srcs, done, nil
}
