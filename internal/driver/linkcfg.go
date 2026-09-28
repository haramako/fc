package driver

// fc.toml のバンクの表 (project.BankLayout) から ld65.cfg を書く。

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/project"
)

// writeLayoutConfig は fc.toml のバンクの表から ld65.cfg を書く。切り替えのバンクは番号順に 1 つずつ (名前の無い番号は
// 最初のスロットに置いた空きのバンク)、常に見えている領域は 1 つにまとめて最後の 6 バイトをベクタにする (iNES の
// ファイルの並び: ヘッダ、バンク 0 から順、固定、CHR)。[ram.<name>] は同じ名前のセグメントと一緒に作る。
func (c *Compiler) writeLayoutConfig() {
	l := c.layout
	prof := l.Profile
	named := map[int]*project.BankDef{}
	for _, b := range l.Banks {
		named[b.Index] = b
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# memory config for ld65 (generated from %s)\n\nMEMORY {\n", filepath.Base(l.Source))
	b.WriteString("  ZP: start = $00, size = $80, type = rw, define = yes;\n")
	b.WriteString("  ZP_STACK: start = $80, size = $80, type = rw, define = yes;\n")
	b.WriteString("  SRAM: start = $0200, size = $0500, type = rw, define = yes;\n")
	for _, r := range l.RAM {
		fmt.Fprintf(&b, "  RAM_%s: start = $%04X, size = $%04X, type = rw, define = yes;\n", r.Name, r.Start, r.Size)
	}
	b.WriteString("  HEADER: start = $0000, size = $10, file = %O, fill = yes;\n")
	for i := 0; i < l.FirstFixed(); i++ {
		slot := prof.Slots[0]
		if nb := named[i]; nb != nil {
			slot = nb.Slot
		}
		fmt.Fprintf(&b, "  ROM%d: start = $%04X, size = $%04X, file = %%O, fill = yes, define = yes, bank = %d;\n", i, slot, prof.BankSize, i)
	}
	fmt.Fprintf(&b, "  FIXED: start = $%04X, size = $%04X, file = %%O, fill = yes, define = yes, bank = %d;\n", prof.FixedAddr, prof.FixedBanks*prof.BankSize-6, l.FirstFixed())
	b.WriteString("  ROMV: start = $fffa, size = $0006, file = %O, fill = yes;\n")
	if l.CHRSize > 0 {
		fmt.Fprintf(&b, "  ROMC: start = $0000, size = $%x, file = %%O, fill = yes;\n", l.CHRSize)
	}
	if l.Fragment != "" {
		c.appendFragment(&b, "MEMORY")
	}
	b.WriteString("}\n\nSEGMENTS {\n")
	b.WriteString("  HEADER: load = HEADER, type = ro;\n")
	b.WriteString("  CODE: load = FIXED, type = ro, define = yes;\n")
	b.WriteString("  FC_RUNTIME: load = FIXED, type = ro, define = yes;\n")
	b.WriteString("  VECTORS: load = ROMV, type = rw;\n")
	if l.CHRSize > 0 {
		b.WriteString("  CHARS: load = ROMC, type = rw, optional = yes;\n")
	}
	b.WriteString("  BSS: load = SRAM, type= bss, define = yes;\n")
	b.WriteString("  ZEROPAGE: load = ZP, type = zp;\n")
	b.WriteString("  FC_ZEROPAGE: load = ZP, type = zp;\n")
	b.WriteString("  FC_STACK: load = ZP_STACK, type = zp;\n")
	for _, r := range l.RAM {
		fmt.Fprintf(&b, "  %s: load = RAM_%s, type = bss, define = yes;\n", r.Name, r.Name)
	}
	for _, m := range c.prog.Modules.List() {
		mem := "FIXED"
		if bank, ok := m.Options.Int("bank"); ok && bank >= 0 {
			if bank >= l.FirstFixed() {
				panic(&diag.Error{Msg: fmt.Sprintf("module %s: bank %d is in the fixed area (use @(bank: \"fixed\"))", m.Id, bank)})
			}
			mem = fmt.Sprintf("ROM%d", bank)
		}
		fmt.Fprintf(&b, "  %s: load = %s, type = ro;\n", m.Id, mem)
	}
	for _, nb := range l.Banks {
		for _, seg := range nb.Segments {
			fmt.Fprintf(&b, "  %s: load = ROM%d, type = ro;\n", seg, nb.Index)
		}
	}
	if l.Fragment != "" {
		c.appendFragment(&b, "SEGMENTS")
	}
	b.WriteString("}\n")
	if err := writeIfChanged(filepath.Join(c.buildDir, "ld65.cfg"), []byte(b.String())); err != nil {
		panic(err)
	}
}

// appendFragment は [linker] extra の cfg の断片から、block (MEMORY / SEGMENTS) の中身を足す。
func (c *Compiler) appendFragment(b *strings.Builder, block string) {
	data, err := os.ReadFile(c.layout.Fragment)
	if err != nil {
		panic(&diag.Error{Msg: fmt.Sprintf("[linker] extra: %v", err)})
	}
	body, ok := cfgBlock(string(data), block)
	if !ok {
		return
	}
	fmt.Fprintf(b, "  # from %s\n", filepath.Base(c.layout.Fragment))
	for _, line := range strings.Split(body, "\n") {
		if strings.TrimSpace(line) != "" {
			b.WriteString("  " + strings.TrimSpace(line) + "\n")
		}
	}
}

// cfgBlock は ld65.cfg の `NAME { ... }` の中身を返す (# のコメントは落とす)。
func cfgBlock(src, name string) (string, bool) {
	var lines []string
	for _, line := range strings.Split(src, "\n") {
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		lines = append(lines, line)
	}
	s := strings.Join(lines, "\n")
	i := strings.Index(s, name)
	for i >= 0 {
		rest := strings.TrimLeft(s[i+len(name):], " \t\r\n")
		if strings.HasPrefix(rest, "{") {
			start := len(s) - len(rest) + 1
			end := strings.IndexByte(s[start:], '}')
			if end < 0 {
				return "", false
			}
			return s[start : start+end], true
		}
		j := strings.Index(s[i+len(name):], name)
		if j < 0 {
			break
		}
		i += len(name) + j
	}
	return "", false
}
