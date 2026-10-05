package project

import "fmt"

// MemoryMap は fc が生成するリンカ設定 (ld65.cfg の MEMORY) と土台 (base.s) の RAM の配置。fc の ld65.cfg の 3 つの形
// (options(bank_count) の NES・emu・fc.toml のバンクの表) と base.s の FC_STACK の大きさ、[ram.*] の重なりの検査が
// ここから番地を取る。自前の base / リンカ設定 (options(base:) / options(linker_config:)) を持つプロジェクトは使わない。
type MemoryMap struct {
	ZP    Region // ZP: fc のレジスタ・静的フレームのゼロページ側 (FC_ZEROPAGE) と ZEROPAGE の変数
	Stack Region // ZP_STACK: stack 系の関数のフレーム (FC_STACK。regalloc.StackSize と同じ大きさ)
	RAM   Region // SRAM: FC_FARCALL・BSS・静的フレームの RAM 側 (FC_SRAM)
}

// Region は番地の範囲。
type Region struct {
	Start, Size int
}

// End は範囲の終わり (含まない)。
func (r Region) End() int { return r.Start + r.Size }

// DefaultMemoryMap は target (nes / emu) の配置。ゼロページは前半が fc のレジスタと静的フレーム、後半がスタック。
// NES の RAM は $0200-$06FF ($0700-$07FF は [ram.*] 用に空ける)、emu は $0200-$0FFF ($1000 からプログラム)。
func DefaultMemoryMap(target string) MemoryMap {
	m := MemoryMap{
		ZP:    Region{0x00, 0x80},
		Stack: Region{0x80, 0x80},
		RAM:   Region{0x0200, 0x0500},
	}
	if target != "nes" {
		m.RAM.Size = 0x0E00
	}
	return m
}

// LinkerMemory は ld65.cfg の MEMORY の ZP / ZP_STACK / SRAM の行。
func (m MemoryMap) LinkerMemory() string {
	return fmt.Sprintf("  ZP: start = $%02X, size = $%02X, type = rw, define = yes;\n", m.ZP.Start, m.ZP.Size) +
		fmt.Sprintf("  ZP_STACK: start = $%02X, size = $%02X, type = rw, define = yes;\n", m.Stack.Start, m.Stack.Size) +
		fmt.Sprintf("  SRAM: start = $%04X, size = $%04X, type = rw, define = yes;\n", m.RAM.Start, m.RAM.Size)
}

// ownRegions は fc 自身が使う領域 ([ram.*] と重なってはいけない所): ゼロページ全体、CPU のスタック、fc の RAM。
func (m MemoryMap) ownRegions() []struct {
	what       string
	start, end int
} {
	zp := Region{m.ZP.Start, m.Stack.End() - m.ZP.Start}
	return []struct {
		what       string
		start, end int
	}{
		{fmt.Sprintf("the zero page ($%02X-$%02X: fc's registers, static frames and stack)", zp.Start, zp.End()-1), zp.Start, zp.End()},
		{"the CPU stack ($0100-$01FF)", 0x0100, 0x0200},
		{fmt.Sprintf("fc's RAM ($%04X-$%04X: BSS and static frames)", m.RAM.Start, m.RAM.End()-1), m.RAM.Start, m.RAM.End()},
	}
}
