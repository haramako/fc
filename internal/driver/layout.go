package driver

// fc.toml のバンクの表から配置を決める (doc/v3_plan.md §3)。マルチバンクのプログラムも fc だけで書け、ld65.cfg は
// 特殊な場合の脱出口 (options(linker_config:) で丸ごと、または [linker] extra で断片) にする。
//
//	[target]
//	mapper = "MMC3"      # マッパーのプロファイル (mapperProfiles)
//	prg = "64K"          # PRG ROM の大きさ
//	chr = "8K"           # CHR ROM の大きさ
//	[bank.en]            # 論理名。モジュールは @(bank: "en")、プログラムからは @bank("en") で番号
//	slot = 0xA000        # 切り替えのスロット (プロファイルの Slots のどれか)
//	index = 3            # 番号を固定するときだけ (省けば空いている番号を小さい順に)
//	segments = ["CODE"]  # 外部のオブジェクト (cc65 など) のセグメントもこのバンクに置く
//	[ram.save]           # 名前つきの RAM 領域。変数は @(segment: "save") で置く
//	start = 0x7E00
//	size = 0x200
//
// "fixed" は常に見えている領域の予約名 (位置と大きさはプロファイルが決める)。

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/sema"
)

// mapperProfile はマッパーごとの配置の型: 切り替えの単位、切り替えのスロット、常に見えている領域。
type mapperProfile struct {
	Name       string
	INES       int   // iNES のマッパー番号
	BankSize   int   // 切り替えの単位 (バイト)
	Slots      []int // 切り替えのスロットの番地
	FixedAddr  int   // 常に見えている領域の先頭 (PRG の最後の FixedBanks 個のバンク)
	FixedBanks int
}

// mapperProfiles は最初の版で扱うマッパー。
var mapperProfiles = map[string]*mapperProfile{
	"NROM": {Name: "NROM", INES: 0, BankSize: 0x4000, FixedAddr: 0x8000, FixedBanks: 2}, // 32K 全部 (16K なら $C000 だけ)
	"MMC3": {Name: "MMC3", INES: 4, BankSize: 0x2000, Slots: []int{0x8000, 0xA000}, FixedAddr: 0xC000, FixedBanks: 2},
	// UxROM / MMC1 (PRG モード 3): $8000 の 16KB を切り替え、$C000 は最後のバンクに固定。トランポリンは
	// fclib/nes/farcall_uxrom.asm / farcall_mmc1.asm
	"UXROM": {Name: "UxROM", INES: 2, BankSize: 0x4000, Slots: []int{0x8000}, FixedAddr: 0xC000, FixedBanks: 1},
	"MMC1":  {Name: "MMC1", INES: 1, BankSize: 0x4000, Slots: []int{0x8000}, FixedAddr: 0xC000, FixedBanks: 1},
}

// bankDef は [bank.<name>] 1 つ。
type bankDef struct {
	Name     string
	Slot     int
	Index    int // 決まった番号 (-1 なら未定)
	Segments []string
}

// ramDef は [ram.<name>] 1 つ。
type ramDef struct {
	Name        string
	Start, Size int
}

// bankLayout は fc.toml のバンクの表を解決したもの。
type bankLayout struct {
	Profile  *mapperProfile
	PRGSize  int
	CHRSize  int
	Banks    []*bankDef // 番号順
	RAM      []*ramDef
	Fragment string // [linker] extra (cfg の断片のパス。fc.toml からの相対は解決済み)
	Source   string // fc.toml のパス
}

// numBanks は PRG のバンクの数。
func (l *bankLayout) numBanks() int { return l.PRGSize / l.Profile.BankSize }

// firstFixed は常に見えている領域の最初のバンクの番号。
func (l *bankLayout) firstFixed() int {
	n := l.numBanks() - l.Profile.FixedBanks
	if n < 0 {
		return 0
	}
	return n
}

// semaBanks は意味解析に渡す名前 → 番号 / スロットの表。
func (l *bankLayout) semaBanks() map[string]sema.BankRef {
	m := map[string]sema.BankRef{"fixed": {Index: -1, Fixed: true}}
	for _, b := range l.Banks {
		m[b.Name] = sema.BankRef{Index: b.Index, Slot: b.Slot}
	}
	return m
}

// layout は fc.toml から配置を作る ([target] も [bank.*] も無ければ nil: 今までどおり options(bank_count / bank) で配置する)。
func (cfg *ProjectConfig) layout() (*bankLayout, error) {
	target, hasTarget := cfg.Sections["target"]
	hasBanks := false
	for s := range cfg.Sections {
		if strings.HasPrefix(s, "bank.") || strings.HasPrefix(s, "ram.") {
			hasBanks = true
		}
	}
	if !hasTarget && !hasBanks {
		return nil, nil
	}
	fail := func(format string, args ...any) error {
		return &diag.Error{Msg: fmt.Sprintf("%s: %s", cfg.Path, fmt.Sprintf(format, args...))}
	}
	if !hasTarget {
		return nil, fail("[bank.*] / [ram.*] need a [target] section (mapper, prg, chr)")
	}
	prof := mapperProfiles[strings.ToUpper(target["mapper"])]
	if prof == nil {
		var names []string
		for n := range mapperProfiles {
			names = append(names, n)
		}
		sort.Strings(names)
		return nil, fail("[target] mapper = %q is not supported (known: %s; use options(linker_config:) for others)", target["mapper"], strings.Join(names, ", "))
	}
	l := &bankLayout{Profile: prof, Source: cfg.Path}
	var err error
	if l.PRGSize, err = parseSize(target["prg"], "32K"); err != nil {
		return nil, fail("[target] prg: %v", err)
	}
	if l.CHRSize, err = parseSize(target["chr"], "8K"); err != nil {
		return nil, fail("[target] chr: %v", err)
	}
	if prof.Name == "NROM" && l.PRGSize == 0x4000 {
		l.Profile = &mapperProfile{Name: "NROM", INES: 0, BankSize: 0x4000, FixedAddr: 0xC000, FixedBanks: 1} // 16K は $C000 (ミラー)
		prof = l.Profile
	}
	if l.PRGSize%prof.BankSize != 0 || l.numBanks() < prof.FixedBanks {
		return nil, fail("[target] prg = %s must be a multiple of %dK and hold the fixed banks", target["prg"], prof.BankSize/1024)
	}
	used := map[int]string{}
	for i := l.firstFixed(); i < l.numBanks(); i++ {
		used[i] = "fixed"
	}
	var names []string
	for s := range cfg.Sections {
		if n, ok := strings.CutPrefix(s, "bank."); ok {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	var pending []*bankDef
	for _, n := range names {
		sec := cfg.Sections["bank."+n]
		if n == "fixed" {
			return nil, fail("[bank.fixed]: fixed is reserved (the always-mapped area of the mapper)")
		}
		b := &bankDef{Name: n, Index: -1}
		if len(prof.Slots) == 0 {
			return nil, fail("[bank.%s]: mapper %s has no switchable banks", n, prof.Name)
		}
		if b.Slot, err = parseInt(sec["slot"]); err != nil {
			return nil, fail("[bank.%s] slot: %v", n, err)
		}
		okSlot := false
		for _, s := range prof.Slots {
			okSlot = okSlot || s == b.Slot
		}
		if !okSlot {
			return nil, fail("[bank.%s] slot = $%04X is not a switchable slot of %s (%s)", n, b.Slot, prof.Name, hexList(prof.Slots))
		}
		if v, ok := sec["size"]; ok {
			size, err := parseSize(v, "")
			if err != nil {
				return nil, fail("[bank.%s] size: %v", n, err)
			}
			if size != prof.BankSize {
				return nil, fail("[bank.%s] size = %s: only %dK banks are supported (put larger areas in a [linker] extra cfg fragment)", n, v, prof.BankSize/1024)
			}
		}
		if v, ok := sec["index"]; ok {
			if b.Index, err = parseInt(v); err != nil {
				return nil, fail("[bank.%s] index: %v", n, err)
			}
			if b.Index < 0 || b.Index >= l.numBanks() {
				return nil, fail("[bank.%s] index = %d is out of range (0..%d)", n, b.Index, l.numBanks()-1)
			}
			if o, dup := used[b.Index]; dup {
				return nil, fail("[bank.%s] index = %d is already used by %s", n, b.Index, o)
			}
			used[b.Index] = n
		} else {
			pending = append(pending, b)
		}
		if v, ok := sec["segments"]; ok {
			if b.Segments, err = parseStringList(v); err != nil {
				return nil, fail("[bank.%s] segments: %v", n, err)
			}
		}
		for k := range sec {
			switch k {
			case "slot", "size", "index", "segments":
			default:
				return nil, fail("[bank.%s]: unknown key %s", n, k)
			}
		}
		l.Banks = append(l.Banks, b)
	}
	next := 0
	for _, b := range pending {
		for used[next] != "" {
			next++
		}
		if next >= l.firstFixed() {
			return nil, fail("[bank.%s]: no free bank (prg = %s has %d switchable banks)", b.Name, target["prg"], l.firstFixed())
		}
		b.Index = next
		used[next] = b.Name
	}
	sort.Slice(l.Banks, func(i, j int) bool { return l.Banks[i].Index < l.Banks[j].Index })
	var rams []string
	for s := range cfg.Sections {
		if n, ok := strings.CutPrefix(s, "ram."); ok {
			rams = append(rams, n)
		}
	}
	sort.Strings(rams)
	for _, n := range rams {
		sec := cfg.Sections["ram."+n]
		r := &ramDef{Name: n}
		if r.Start, err = parseInt(sec["start"]); err != nil {
			return nil, fail("[ram.%s] start: %v", n, err)
		}
		if r.Size, err = parseInt(sec["size"]); err != nil {
			return nil, fail("[ram.%s] size: %v", n, err)
		}
		// fc 自身の領域 (writeLayoutConfig の ZP / ZP_STACK / SRAM と CPU スタック) と重ならないこと。OAM を $0200 に置くと
		// FC_FARCALL・BSS と同じ場所になり、黙って壊れていた
		for _, own := range []struct {
			what       string
			start, end int
		}{
			{"the zero page ($00-$FF: fc's registers, static frames and stack)", 0x0000, 0x0100},
			{"the CPU stack ($0100-$01FF)", 0x0100, 0x0200},
			{"fc's RAM ($0200-$06FF: BSS and static frames)", 0x0200, 0x0700},
		} {
			if r.Start < own.end && own.start < r.Start+r.Size {
				return nil, fail("[ram.%s] $%04X-$%04X overlaps %s; use $0700-$07FF or cartridge RAM ($6000-$7FFF)", n, r.Start, r.Start+r.Size-1, own.what)
			}
		}
		for _, o := range l.RAM {
			if r.Start < o.Start+o.Size && o.Start < r.Start+r.Size {
				return nil, fail("[ram.%s] overlaps [ram.%s]", n, o.Name)
			}
		}
		l.RAM = append(l.RAM, r)
	}
	if lk, ok := cfg.Sections["linker"]; ok {
		if ex, ok := lk["extra"]; ok {
			l.Fragment = ex
		}
	}
	return l, nil
}

// parseSize は "32K" / "8192" / "0x2000" を読む (空なら def)。
func parseSize(s, def string) (int, error) {
	if s == "" {
		s = def
	}
	if s == "" {
		return 0, fmt.Errorf("missing")
	}
	mul := 1
	if strings.HasSuffix(strings.ToUpper(s), "K") {
		mul, s = 1024, s[:len(s)-1]
	}
	n, err := strconv.ParseInt(s, 0, 32)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("%q is not a size (e.g. 32K)", s)
	}
	return int(n) * mul, nil
}

func parseInt(s string) (int, error) {
	if s == "" {
		return 0, fmt.Errorf("missing")
	}
	n, err := strconv.ParseInt(s, 0, 32)
	if err != nil {
		return 0, fmt.Errorf("%q is not an integer", s)
	}
	return int(n), nil
}

// parseStringList は `["a", "b"]` を読む (fc.toml の値の綴り)。
func parseStringList(s string) ([]string, error) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "[") || !strings.HasSuffix(s, "]") {
		return nil, fmt.Errorf("expected [\"name\", ...]")
	}
	var r []string
	for _, e := range strings.Split(s[1:len(s)-1], ",") {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		if len(e) < 2 || e[0] != '"' || e[len(e)-1] != '"' {
			return nil, fmt.Errorf("expected [\"name\", ...]")
		}
		r = append(r, e[1:len(e)-1])
	}
	return r, nil
}

func hexList(xs []int) string {
	var r []string
	for _, x := range xs {
		r = append(r, fmt.Sprintf("$%04X", x))
	}
	return strings.Join(r, ", ")
}

// writeLayoutConfig は fc.toml のバンクの表から ld65.cfg を書く。切り替えのバンクは番号順に 1 つずつ (名前の無い番号は
// 最初のスロットに置いた空きのバンク)、常に見えている領域は 1 つにまとめて最後の 6 バイトをベクタにする (iNES の
// ファイルの並び: ヘッダ、バンク 0 から順、固定、CHR)。[ram.<name>] は同じ名前のセグメントと一緒に作る。
func (c *Compiler) writeLayoutConfig() {
	l := c.layout
	prof := l.Profile
	named := map[int]*bankDef{}
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
	for i := 0; i < l.firstFixed(); i++ {
		slot := prof.Slots[0]
		if nb := named[i]; nb != nil {
			slot = nb.Slot
		}
		fmt.Fprintf(&b, "  ROM%d: start = $%04X, size = $%04X, file = %%O, fill = yes, define = yes, bank = %d;\n", i, slot, prof.BankSize, i)
	}
	fmt.Fprintf(&b, "  FIXED: start = $%04X, size = $%04X, file = %%O, fill = yes, define = yes, bank = %d;\n", prof.FixedAddr, prof.FixedBanks*prof.BankSize-6, l.firstFixed())
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
			if bank >= l.firstFixed() {
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
