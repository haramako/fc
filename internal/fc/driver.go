package fc

// lib/fc/compiler.rb の Compiler (パイプライン統括・ca65/ld65起動・リンク) の移植。
// ERBテンプレートは text/template ではなく直接文字列生成で 1:1 に再現している
// (テンプレートが小さく静的なため)。

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/haramako/fc/internal/r6502"
)

const BuildPath = ".fc-build"

// CommandError は外部コマンドの失敗 (Fc::CommandError 相当)。
type CommandError struct {
	Msg     string
	Command []string
	Result  string
}

func (e *CommandError) Error() string { return e.Msg }

type BuildOptions struct {
	Target        string // emu / nes (デフォルト emu)
	Out           string // 出力ファイル (デフォルト a.bin / a.nes)
	Run           bool   // -e
	Asm           bool   // -S (Ruby版でも実質未使用)
	DebugInfo     bool   // -d (Go版はデバッグログのみ)
	OptimizeLevel int    // -O (デフォルト 2)
	CompileOnly   bool
	Stdout        io.Writer
}

type Compiler struct {
	FCHome string // fclib/ share/ を含むディレクトリ
	target string
	hlc    *Hlc
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

// Build は Compiler#build 相当。Run 指定時は実行結果 (終了コード) を返す。
func (c *Compiler) Build(filename string, opt *BuildOptions) (result int, err error) {
	defer func() {
		if r := recover(); r != nil {
			if ce, ok := r.(*CommandError); ok {
				err = &CompileError{Msg: ce.Msg + "\n" + strings.Join(ce.Command, " ") + "\n" + ce.Result}
				return
			}
			if ce, ok := r.(*CompileError); ok {
				err = ce
				return
			}
			panic(r)
		}
	}()

	if opt.Target == "" {
		opt.Target = "emu"
	}
	if opt.Target == "x6502" {
		return 0, &CompileError{Msg: "target x6502 is not supported by go port"}
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

	if err := os.MkdirAll(BuildPath, 0o777); err != nil {
		return 0, err
	}

	// compile (ソースコード -> 中間コード)
	hlc := NewHlc([]string{".", filepath.ToSlash(filepath.Join(c.FCHome, "fclib")), filepath.ToSlash(filepath.Join(c.FCHome, "fclib", opt.Target))})
	if cerr := hlc.Compile(filename); cerr != nil {
		return 0, cerr
	}
	c.hlc = hlc

	// compile2 (中間コード -> アセンブラファイル)
	llc := NewLlc(opt.OptimizeLevel)
	for _, me := range hlc.Modules.Entries() {
		mod := me.Val.(*Module)
		if mod.FromFcm {
			continue
		}
		asm, inc := llc.Compile(mod)
		if err := os.WriteFile(filepath.Join(BuildPath, fmt.Sprintf("_%s.inc", ToS(mod.Id))), []byte(strings.Join(inc, "\n")), 0o666); err != nil {
			return 0, err
		}
		if err := os.WriteFile(filepath.Join(BuildPath, fmt.Sprintf("_%s.s", ToS(mod.Id))), []byte(strings.Join(asm, "\n")), 0o666); err != nil {
			return 0, err
		}
	}

	// assemble (アセンブラ -> オブジェクトファイル)
	var objs []string
	for _, me := range hlc.Modules.Entries() {
		mod := me.Val.(*Module)
		objs = append(objs, filepath.Join(BuildPath, fmt.Sprintf("_%s.o", ToS(mod.Id))))
		if mod.FromFcm {
			continue
		}
		c.ca65(filepath.Join(BuildPath, fmt.Sprintf("_%s.s", ToS(mod.Id))))
	}

	c.makeRuntime(opt.Target)
	if opt.CompileOnly {
		return 0, nil
	}

	c.makeBase()

	c.link(objs, opt)

	if opt.Run {
		return c.execute(opt.Out, opt.Stdout)
	}
	return 0, nil
}

func (c *Compiler) makeRuntime(target string) {
	c.ca65(c.findShare("runtime.asm"))
	c.ca65(filepath.Join(c.FCHome, "fclib", target, "runtime_init.asm"))
}

// makeBase は base.asm.erb 相当の base.s を生成してアセンブルする。
func (c *Compiler) makeBase() {
	opts := c.hlc.Options
	inesmap := 0
	switch m := opts.GetOr(Sym("mapper")).(type) {
	case nil:
		inesmap = 0
	case string:
		switch m {
		case "MMC0":
			inesmap = 0
		case "MMC3":
			inesmap = 4
		}
	case int:
		inesmap = m
	}

	bankCount := 4
	if bc, ok := opts.GetOr(Sym("bank_count")).(int); ok {
		bankCount = bc
	}
	inesprg := bankCount / 2
	ineschr := 1
	if cb, ok := opts.GetOr(Sym("char_banks")).(int); ok {
		ineschr = cb
	}

	str := c.baseAsmTemplate(inesprg, ineschr, 1, inesmap)
	if err := os.WriteFile(filepath.Join(BuildPath, "base.s"), []byte(str), 0o666); err != nil {
		panic(err)
	}
	c.ca65(filepath.Join(BuildPath, "base.s"))
}

type bankInfo struct {
	size, org int
}

// link はオブジェクトファイルをリンクする。
func (c *Compiler) link(objs []string, opt *BuildOptions) {
	opts := c.hlc.Options

	ineschr := 1
	if cb, ok := opts.GetOr(Sym("char_banks")).(int); ok {
		ineschr = cb
	}

	var banks []*bankInfo
	if bc, ok := opts.GetOr(Sym("bank_count")).(int); ok {
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
	for _, me := range c.hlc.Modules.Entries() {
		m := me.Val.(*Module)
		bank := 0
		if b, ok := m.Options.GetOr(Sym("bank")).(int); ok {
			bank = b
		}
		if bank < 0 {
			bank = len(banks) + bank
		}
		if org, ok := m.Options.GetOr(Sym("org")).(int); ok {
			banks[bank].org = org
		}
		segs = append(segs, segInfo{name: ToS(m.Id), bank: bank})
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

	if err := os.WriteFile(filepath.Join(BuildPath, "ld65.cfg"), []byte(cfg), 0o666); err != nil {
		panic(err)
	}

	mapFile := strings.TrimSuffix(opt.Out, filepath.Ext(opt.Out)) + ".map"
	args := []string{"-m", mapFile, "-o", opt.Out, "-C", filepath.Join(BuildPath, "ld65.cfg"),
		filepath.Join(BuildPath, "base.o"), filepath.Join(BuildPath, "runtime_init.o"), filepath.Join(BuildPath, "runtime.o")}
	args = append(args, objs...)
	c.sh("ld65", args...)
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

// ca65 はアセンブルを実行する。
func (c *Compiler) ca65(path string) {
	base := filepath.Base(path)
	obj := filepath.Join(BuildPath, strings.TrimSuffix(base, filepath.Ext(base))+".o")
	c.sh("ca65",
		"-g",
		"-o", obj,
		"-I", filepath.Join(c.FCHome, "share"),
		"-I", BuildPath,
		"-I", filepath.Join(c.FCHome, "fclib"),
		"-I", ".",
		"-I", filepath.Join(c.FCHome, "fclib", c.target),
		path)
}

// sh は外部コマンドを実行する (失敗時は CommandError を panic)。
func (c *Compiler) sh(name string, args ...string) {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		code := -1
		if cmd.ProcessState != nil {
			code = cmd.ProcessState.ExitCode()
		}
		panic(&CommandError{
			Msg:     fmt.Sprintf("%s returns %d", name, code),
			Command: append([]string{name}, args...),
			Result:  string(out),
		})
	}
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
