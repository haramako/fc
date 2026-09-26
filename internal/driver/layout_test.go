package driver

// fc.toml のバンクの表から配置を作るテスト (doc/v3_plan.md §3、layout.go)。MMC3 で名前つきのバンクに表を置き、固定の
// 領域の main から far call で読む。内蔵 NES ランナーで走らせ、値・呼び出しの後のバンクの復帰・@bank の番号・名前つきの
// RAM 領域・cfg の断片を確かめる。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func layoutFiles() map[string]string {
	mod := func(bank, vals string) string {
		return "#fc 3\n@(bank: \"" + bank + "\");\nconst T:[4]u8 = [" + vals + "];\npublic function get(i:u8):u8 @(noinline) { return T[i]; }\n"
	}
	return map[string]string{
		"fc.toml": `# layout test
[target]
mapper = "MMC3"
prg = "64K"
chr = "8K"
[bank.a]
slot = 0x8000
[bank.b]
slot = 0x8000
[bank.c]
slot = 0xA000
index = 4
[ram.save]
start = 0x6000
size = 0x100
[linker]
extra = "extra.cfg"
`,
		"extra.cfg": "MEMORY {\n  XRAM: start = $6100, size = $100, type = rw;\n}\nSEGMENTS {\n  xram: load = XRAM, type = bss;  # 断片のセグメント\n}\n",
		"main.fc": `#fc 3
@(farcall);
use nes;
use ma;
use mb;
use mc;
@include("farcall_mmc3.asm");
var pbank_bak:[2]u8 @(symbol: "_mmc3_pbank_bak");
var BANK_SELECT:u8 @(address: 0x8000);
var BANK_DATA:u8 @(address: 0x8001);
public var out:[32]u8;
public var done:u8;
var saved:u8 @(segment: "save");
var extra:u8 @(segment: "xram");
function nmi():void @(symbol: "_interrupt") { }
function irq():void @(symbol: "_interrupt_irq") { }
function main():void
{
	BANK_SELECT = 6;
	BANK_DATA = @bank("a");
	BANK_SELECT = 7;
	BANK_DATA = @bank("c");
	pbank_bak[0] = @bank("a");
	pbank_bak[1] = @bank("c");
	out[0] = ma.get(1);
	out[1] = mb.get(1);
	out[2] = mc.get(1);
	out[3] = ma.get(2);
	out[4] = pbank_bak[0];
	out[5] = pbank_bak[1];
	out[6] = @bank("a");
	out[7] = @bank("b");
	out[8] = @bank("c");
	saved = 9;
	extra = 11;
	out[9] = saved;
	out[10] = extra;
	done = 1;
	while (true) {
	}
}
`,
		"ma.fc": mod("a", "10, 20, 30, 40"),
		"mb.fc": mod("b", "50, 60, 70, 80"),
		"mc.fc": mod("c", "1, 2, 3, 4"),
	}
}

func TestLayoutMMC3(t *testing.T) {
	t.Parallel()
	want := "[20 60 2 30 0 4 0 1 4 9 11]"
	for _, level := range []int{-1, 0} {
		out, done, _ := runNes(t, layoutFiles(), level, 11)
		if done != 1 {
			t.Fatalf("-O %d: main が終わらない", level)
		}
		if got := strings.ReplaceAll(strings.Trim(strings.ReplaceAll(fmtInts(out), ",", ""), " "), "  ", " "); got != want {
			t.Errorf("-O %d: got %s want %s", level, got, want)
		}
	}
}

func fmtInts(xs []int) string {
	var b strings.Builder
	b.WriteString("[")
	for i, x := range xs {
		if i > 0 {
			b.WriteString(" ")
		}
		b.WriteString(itoa(x))
	}
	b.WriteString("]")
	return b.String()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	if neg {
		s = "-" + s
	}
	return s
}

// TestLayoutErrors: fc.toml のバンクの表の誤り。
func TestLayoutErrors(t *testing.T) {
	t.Parallel()
	base := "[target]\nmapper = \"MMC3\"\nprg = \"64K\"\n"
	for _, c := range []struct{ toml, src, msg string }{
		{"[target]\nmapper = \"MMC9\"\n", "", `mapper = "MMC9" is not supported`},
		{base + "[bank.a]\nslot = 0xC000\n", "", "slot = $C000 is not a switchable slot of MMC3"},
		{base + "[bank.a]\nslot = 0x8000\nindex = 6\n", "", "index = 6 is already used by fixed"},
		{base + "[bank.a]\nslot = 0x8000\nsize = \"16K\"\n", "", "only 8K banks are supported"},
		{base + "[bank.fixed]\nslot = 0x8000\n", "", "fixed is reserved"},
		{base + "[bank.a]\nslot = 0x8000\ncolor = 1\n", "", "unknown key color"},
		{base + "[ram.oam]\nstart = 0x0200\nsize = 0x100\n", "", "[ram.oam] $0200-$02FF overlaps fc's RAM"},
		{base + "[ram.z]\nstart = 0x00f0\nsize = 0x10\n", "", "overlaps the zero page"},
		{base + "[ram.a]\nstart = 0x6000\nsize = 0x100\n[ram.b]\nstart = 0x60f0\nsize = 0x10\n", "", "[ram.b] overlaps [ram.a]"},
		{base, "@(bank: \"nope\");\n", `unknown bank "nope"`},
		{"", "@(bank: \"a\");\n", `bank names need a bank table in fc.toml`},
		{base, "function f():u8 { return @bank(\"fixed\"); }\n", "the fixed area has no bank number"},
	} {
		dir := t.TempDir()
		if c.toml != "" {
			os.WriteFile(filepath.Join(dir, "fc.toml"), []byte(c.toml), 0o666)
		}
		os.WriteFile(filepath.Join(dir, "t.fc"), []byte("#fc 3\n"+c.src+"function main():void { }\n"), 0o666)
		_, err := NewCompiler(absRepoRoot).Build("t.fc", &BuildOptions{Target: "nes", Dir: dir, BuildDir: filepath.Join(dir, "b"), Out: filepath.Join(dir, "a.nes"), CompileOnly: true})
		if err == nil || !strings.Contains(err.Error(), c.msg) {
			t.Errorf("%q / %q: got %v, want /%s/", c.toml, c.src, err, c.msg)
		}
	}
}

// TestLayoutNROM: 切り替えの無い NROM も fc.toml で書ける (32K は $8000 から、16K は $C000)。名前つきのバンクは作れない。
func TestLayoutNROM(t *testing.T) {
	t.Parallel()
	for _, prg := range []string{"32K", "16K"} {
		files := map[string]string{
			"fc.toml": "[target]\nmapper = \"NROM\"\nprg = \"" + prg + "\"\n",
			"main.fc": `#fc 3
use nes;
public var out:[32]u8;
public var done:u8;
const T:[3]u8 = [7, 8, 9];
function nmi():void @(symbol: "_interrupt") { }
function irq():void @(symbol: "_interrupt_irq") { }
function main():void
{
	out[0] = T[2];
	done = 1;
	while (true) {
	}
}
`,
		}
		out, done, _ := runNes(t, files, 0, 1)
		if done != 1 || out[0] != 9 {
			t.Errorf("NROM %s: out=%v done=%d", prg, out, done)
		}
	}
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "fc.toml"), []byte("[target]\nmapper = \"NROM\"\n[bank.a]\nslot = 0x8000\n"), 0o666)
	os.WriteFile(filepath.Join(dir, "t.fc"), []byte("#fc 3\nfunction main():void { }\n"), 0o666)
	_, err := NewCompiler(absRepoRoot).Build("t.fc", &BuildOptions{Target: "nes", Dir: dir, BuildDir: filepath.Join(dir, "b"), Out: filepath.Join(dir, "a.nes"), CompileOnly: true})
	if err == nil || !strings.Contains(err.Error(), "mapper NROM has no switchable banks") {
		t.Errorf("NROM のバンク: %v", err)
	}
}

// TestLayout16K: UxROM / MMC1 (16KB のバンクを $8000 で切り替え、$C000 は固定)。fclib/nes/farcall_uxrom.asm /
// farcall_mmc1.asm のトランポリンで far call し、戻ったらバンクが元に戻る。
func TestLayout16K(t *testing.T) {
	t.Parallel()
	mod := func(bank, vals string) string {
		return "#fc 3\n@(bank: \"" + bank + "\");\nconst T:[4]u8 = [" + vals + "];\npublic function get(i:u8):u8 @(noinline) { return T[i]; }\n"
	}
	for _, c := range []struct{ mapper, include, bankVar, init string }{
		{"UxROM", "farcall_uxrom.asm", "_uxrom_bank", "UXROM = @bank(\"a\");\n\tcur = @bank(\"a\");"},
		{"MMC1", "farcall_mmc1.asm", "_mmc1_bank", "@asm(\"jsr mmc1_reset\", \"lda #0\", \"jsr mmc1_set\"); // a は 0 番"},
	} {
		files := map[string]string{
			"fc.toml": "[target]\nmapper = \"" + c.mapper + "\"\nprg = \"64K\"\n[bank.a]\nslot = 0x8000\n[bank.b]\nslot = 0x8000\n",
			"main.fc": `#fc 3
@(farcall);
use nes;
use ma;
use mb;
@include("` + c.include + `");
var cur:u8 @(symbol: "` + c.bankVar + `");
var UXROM:u8 @(address: 0x8000);
public var out:[32]u8;
public var done:u8;
function nmi():void @(symbol: "_interrupt") { }
function irq():void @(symbol: "_interrupt_irq") { }
function main():void
{
	` + c.init + `
	out[0] = ma.get(1);
	out[1] = mb.get(2);
	out[2] = ma.get(3);
	out[3] = cur;
	out[4] = @bank("b");
	done = 1;
	while (true) {
	}
}
`,
			"ma.fc": mod("a", "10, 20, 30, 40"),
			"mb.fc": mod("b", "50, 60, 70, 80"),
		}
		for _, level := range []int{-1, 0} {
			out, done, _ := runNes(t, files, level, 5)
			if done != 1 || fmtInts(out) != "[20 70 40 0 1]" {
				t.Errorf("%s -O %d: out=%v done=%d", c.mapper, level, out, done)
			}
		}
	}
}
