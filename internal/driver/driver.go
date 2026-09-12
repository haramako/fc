package driver

// パイプライン統括 (ソース → sema → codegen → ca65 → ld65 → emu 実行)。lib/fc/compiler.rb 由来。
// base.asm / ld65.cfg のテンプレートは小さく静的なので text/template を使わず文字列生成している。

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/haramako/fc/internal/codegen"
	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/r6502"
	"github.com/haramako/fc/internal/sema"
)

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
	Target        string // emu / nes (デフォルト emu)
	Out           string // 出力ファイル (デフォルト a.bin / a.nes。作業ディレクトリ相対)
	Run           bool   // -e
	OptimizeLevel int    // -O (デフォルト 2)
	CompileOnly   bool
	Stdout        io.Writer

	// Dir はソースの基準ディレクトリ (use / include / incbin の相対パスの起点)。"" なら作業ディレクトリ。
	// BuildDir は中間生成物 (.s / .inc / .o / base.o / ld65.cfg) の置き場所。"" なら <Dir>/.fc-build。
	// CLI はどちらも既定のままなので外部挙動は従来どおり (doc/v2_plan.md G6)。
	Dir      string
	BuildDir string

	// Jobs は ca65 を同時に走らせる数。0 なら CPU 数。1 で逐次。
	Jobs int
}

// Result はビルドの結果。
type Result struct {
	ExitCode int      // Run 指定時のプログラムの終了コード (それ以外は 0)
	Out      string   // 出力ファイル (CompileOnly なら "")
	MapFile  string   // ld65 のマップファイル (CompileOnly なら "")
	Objects  []string // fc ソースから生成したオブジェクトファイル (モジュール順 = リンク順)
	BuildDir string   // 中間生成物ディレクトリ
}

type Compiler struct {
	FCHome   string // fclib/ share/ を含むディレクトリ
	ctx      context.Context
	jobs     int // ca65 の並列数
	target   string
	dir      string // ソースの基準ディレクトリ (BuildOptions.Dir)
	buildDir string // 中間生成物ディレクトリ (BuildOptions.BuildDir)
	prog     *sema.Program
}

func NewCompiler(fcHome string) *Compiler {
	return &Compiler{FCHome: fcHome}
}

// findShare は share以下のファイルを検索する (target優先)。
func (c *Compiler) findShare(filename string) string {
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

// Build はソースをビルドする。Run 指定時は実行結果 (終了コード) を返す。
func (c *Compiler) Build(filename string, opt *BuildOptions) (int, error) {
	r, err := c.BuildContext(context.Background(), filename, opt)
	if err != nil {
		return 0, err
	}
	return r.ExitCode, nil
}

// BuildContext はソースをビルドし、生成物の情報を返す。ctx のキャンセルは外部コマンド (ca65 / ld65) に伝わる。
// エラーは *diag.Error (コンパイルエラー) または *CommandError (外部コマンドの失敗)。
func (c *Compiler) BuildContext(ctx context.Context, filename string, opt *BuildOptions) (result *Result, err error) {
	defer func() {
		if r := recover(); r != nil {
			if ce, ok := r.(*CommandError); ok {
				err = ce
				return
			}
			if ce, ok := r.(*diag.Error); ok {
				err = ce
				return
			}
			panic(r)
		}
	}()
	c.ctx = ctx

	if opt.Target == "" {
		opt.Target = "emu"
	}
	if opt.Target == "x6502" {
		return nil, &diag.Error{Msg: "target x6502 is not supported by go port"}
	}
	if opt.Out == "" {
		if opt.Target == "nes" {
			opt.Out = "a.nes"
		} else {
			opt.Out = "a.bin"
		}
	}
	if opt.OptimizeLevel == 0 {
		opt.OptimizeLevel = 2
	}
	if opt.Stdout == nil {
		opt.Stdout = os.Stdout
	}
	c.target = opt.Target
	c.dir = opt.Dir
	if c.dir == "" {
		c.dir = "."
	}
	c.buildDir = opt.BuildDir
	if c.buildDir == "" {
		c.buildDir = filepath.Join(c.dir, DefaultBuildDirName)
	}
	c.jobs = opt.Jobs
	if c.jobs <= 0 {
		c.jobs = runtime.NumCPU()
	}

	if err := os.MkdirAll(c.buildDir, 0o777); err != nil {
		return nil, err
	}
	result = &Result{BuildDir: c.buildDir}

	// compile (ソースコード -> 中間コード)
	prog, cerr := sema.Compile(opt.Dir, c.libPath(opt.Target), filename)
	if cerr != nil {
		return nil, cerr
	}
	c.prog = prog

	// compile2 (中間コード -> アセンブラファイル)
	llc := codegen.NewLlc(opt.OptimizeLevel, prog.Types)
	for _, mod := range prog.Modules.List() {
		if mod.FromFcm {
			continue
		}
		asm, inc, lerr := llc.Compile(mod)
		if lerr != nil {
			return nil, lerr
		}
		if err := os.WriteFile(filepath.Join(c.buildDir, fmt.Sprintf("_%s.inc", mod.Id)), []byte(strings.Join(inc, "\n")), 0o666); err != nil {
			return nil, err
		}
		if err := os.WriteFile(filepath.Join(c.buildDir, fmt.Sprintf("_%s.s", mod.Id)), []byte(strings.Join(asm, "\n")), 0o666); err != nil {
			return nil, err
		}
	}

	// assemble (アセンブラ -> オブジェクトファイル)。各 .s は独立なので並列にアセンブルする
	// (.inc は上で全部書き終えている)。objs の並び = リンク順はモジュール順のまま
	var objs, sources []string
	for _, mod := range prog.Modules.List() {
		objs = append(objs, filepath.Join(c.buildDir, fmt.Sprintf("_%s.o", mod.Id)))
		if mod.FromFcm {
			continue
		}
		sources = append(sources, filepath.Join(c.buildDir, fmt.Sprintf("_%s.s", mod.Id)))
	}
	sources = append(sources, c.findShare("runtime.asm"), filepath.Join(c.FCHome, "fclib", opt.Target, "runtime_init.asm"))
	if err := c.assembleAll(sources); err != nil {
		return nil, err
	}
	result.Objects = objs
	if opt.CompileOnly {
		return result, nil
	}

	c.makeBase()

	result.Out = opt.Out
	result.MapFile = c.link(objs, opt)

	if opt.Run {
		code, err := c.execute(opt.Out, opt.Stdout)
		if err != nil {
			return nil, err
		}
		result.ExitCode = code
	}
	return result, nil
}

// libPath は use / include の検索パス (カレント → fclib → fclib/<target>)。
func (c *Compiler) libPath(target string) []string {
	return []string{".", filepath.ToSlash(filepath.Join(c.FCHome, "fclib")), filepath.ToSlash(filepath.Join(c.FCHome, "fclib", target))}
}

// makeBase は base.asm.erb 相当の base.s を生成してアセンブルする。
func (c *Compiler) makeBase() {
	opts := c.prog.Options
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

	str := c.baseAsmTemplate(inesprg, ineschr, 1, inesmap)
	if err := os.WriteFile(filepath.Join(c.buildDir, "base.s"), []byte(str), 0o666); err != nil {
		panic(err)
	}
	c.ca65(filepath.Join(c.buildDir, "base.s"))
}

type bankInfo struct {
	size, org int
}

// link はオブジェクトファイルをリンクし、マップファイルのパスを返す。
func (c *Compiler) link(objs []string, opt *BuildOptions) string {
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
		if b, ok := m.Options.Int("bank"); ok {
			bank = b
		}
		if bank < 0 {
			bank = len(banks) + bank
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
		b.WriteString("  ZP: start = $00, size = $80, type = rw, define = yes;\n")
		b.WriteString("  ZP_STACK: start = $80, size = $80, type = rw, define = yes;\n")
		b.WriteString("  SRAM: start = $0200, size = $0500, type = rw, define = yes;\n")
		b.WriteString("  HEADER: start = $0000, size = $10, file = %O, fill = yes;\n")
		for i, bank := range banks {
			fmt.Fprintf(&b, "  ROM%d: start = $%x, size = $%x, file = %%O, fill = yes, define = yes;\n", i, bank.org, bank.size)
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
		b.WriteString("  ZP: start = $00, size = $80, type = rw, define = yes;\n")
		b.WriteString("  ZP_STACK: start = $80, size = $80, type = rw, define = yes;\n")
		b.WriteString("  SRAM: start = $0200, size = $0500, type = rw, define = yes;\n")
		b.WriteString("  ROMV: start = $1000, size = 3, type = rw, define = yes;\n")
		b.WriteString("  ROM: start = $1003, size = $DFFD, file = %O, fill = no, define = yes;\n")
		b.WriteString("  CHARS: start = $0000, size = $10000, type = rw, fill = no, define = yes;\n")
		b.WriteString("}\n\nSEGMENTS {\n")
		b.WriteString("  CODE: load = ROM, type = ro, define = yes;\n")
		b.WriteString("  FC_RUNTIME: load = ROM, type = ro, define = yes;\n")
		b.WriteString("  VECTORS: load = ROMV, type = rw;\n")
		b.WriteString("  BSS: load = SRAM, type= bss, define = yes;\n")
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

	if err := os.WriteFile(filepath.Join(c.buildDir, "ld65.cfg"), []byte(cfg), 0o666); err != nil {
		panic(err)
	}

	mapFile := strings.TrimSuffix(opt.Out, filepath.Ext(opt.Out)) + ".map"
	args := []string{"-m", mapFile, "-o", opt.Out, "-C", filepath.Join(c.buildDir, "ld65.cfg"),
		filepath.Join(c.buildDir, "base.o"), filepath.Join(c.buildDir, "runtime_init.o"), filepath.Join(c.buildDir, "runtime.o")}
	args = append(args, objs...)
	c.sh("ld65", args...)
	return mapFile
}

// baseAsmTemplate は share/<target>/base.asm.erb 相当。
func (c *Compiler) baseAsmTemplate(inesprg, ineschr, inesmir, inesmap int) string {
	header := "\t.exportzp FC_LOCAL\n" +
		"\t.exportzp FC_REG\n" +
		"\t.exportzp FC_STACK\n" +
		"\t.exportzp FC_FASTCALL_REG\n" +
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
		"FC_FASTCALL_REG: .res $10\n" +
		"\n" +
		".segment \"FC_STACK\": zeropage\n" +
		"\t\n" +
		"FC_STACK: .res $80\n" +
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
func (c *Compiler) assembleAll(sources []string) error {
	ctx, cancel := context.WithCancel(c.ctxOrBackground())
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
			if err := c.run(ctx, "ca65", c.ca65Args(src)...); err != nil {
				errs[i] = err
				cancel()
			}
		}(i, src)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

// ca65 はアセンブルを実行する (逐次。失敗は CommandError を panic)。
func (c *Compiler) ca65(path string) {
	c.sh("ca65", c.ca65Args(path)...)
}

// ca65Args はアセンブルのコマンド引数を作る。
func (c *Compiler) ca65Args(path string) []string {
	base := filepath.Base(path)
	obj := filepath.Join(c.buildDir, strings.TrimSuffix(base, filepath.Ext(base))+".o")
	args := []string{
		"-g",
		"-o", obj,
		"-I", filepath.Join(c.FCHome, "share"),
		"-I", c.buildDir,
		"-I", filepath.Join(c.FCHome, "fclib"),
		"-I", c.dir,
		"-I", filepath.Join(c.FCHome, "fclib", c.target),
	}
	if c.dir != "." {
		// .incbin は -I でなく --bin-include-dir で探す (既定は作業ディレクトリ)
		args = append(args, "--bin-include-dir", c.dir)
	}
	return append(args, path)
}

func (c *Compiler) ctxOrBackground() context.Context {
	if c.ctx == nil {
		return context.Background()
	}
	return c.ctx
}

// sh は外部コマンドを実行する (失敗時は CommandError を panic)。
func (c *Compiler) sh(name string, args ...string) {
	if err := c.run(c.ctxOrBackground(), name, args...); err != nil {
		panic(err)
	}
}

// run は外部コマンドを実行し、失敗なら *CommandError を返す。
func (c *Compiler) run(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		code := -1
		if cmd.ProcessState != nil {
			code = cmd.ProcessState.ExitCode()
		}
		return &CommandError{
			Msg:     fmt.Sprintf("%s returns %d", name, code),
			Command: append([]string{name}, args...),
			Result:  string(out),
		}
	}
	return nil
}

// execute はビルド済みバイナリをエミュレータで実行する (Compiler#execute 相当)。
// ホスト呼び出し規約 ($fff0〜$ffff): 1=print / 2=print_int / 3=print_int_sp、
// $ffff が 255 以外になったら終了 (その値が終了コード)。
func (c *Compiler) execute(filename string, out io.Writer) (int, error) {
	if c.target != "emu" {
		return 0, nil // x6502 はスコープ外、nes は実行不可
	}
	data, err := os.ReadFile(filename)
	if err != nil {
		return 0, err
	}
	const startAddr = 0x1000
	mem := r6502.NewMemory()
	for i, b := range data {
		mem.Set(startAddr+i, int(b))
	}
	cpu := r6502.NewCpu(mem)
	cpu.Pc = startAddr
	mem.Set(0xffff, 255)
	mem.Set(0xfffe, 255)
	for mem.Get(0xffff) == 255 {
		cpu.StepSilent()
		if mem.Get(0xfffe) != 255 {
			switch mem.Get(0xfffe) {
			case 1:
				addr := mem.Get(0xfff0) + (mem.Get(0xfff1) << 8)
				var sb []byte
				for mem.Get(addr) != 0 {
					sb = append(sb, byte(mem.Get(addr)))
					addr++
				}
				fmt.Fprint(out, string(sb))
			case 2:
				num := mem.Get(0xfff2) + (mem.Get(0xfff3) << 8)
				fmt.Fprint(out, num)
			case 3:
				num := mem.Get(0xfff2) + (mem.Get(0xfff3) << 8)
				fmt.Fprint(out, num, " ")
			}
			mem.Set(0xfffe, 255)
		}
	}
	return mem.Get(0xffff), nil
}
