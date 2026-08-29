// Package nes は fc が生成した NES ROM の「大雑把な動作確認」用ヘッドレスランナー。
//
// 目的はスモークテストであり、正確なNESエミュレーションではない:
//   - CPU: internal/r6502 (Accurateモード) + 概算サイクル
//   - PPU: レジスタ挙動の近似 (vblankフラグ / NMI / VRAM・パレット・OAMの記録)。
//     レンダリングパイプラインはなく、Screenshot() が nametable から静的に絵を起こす
//   - マッパー: NROM(0) と MMC3(4)。MMC3 の scanline IRQ はスキャンライン数の概算で駆動
//   - APU: 書き込みは無視、読み出しは0
//
// これで「ブートし、NMIが回り、描画が有効になり、画面が作られる」ことと、
// その画面のスクリーンショット(PNG)による目視確認ができる。
package nes

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"

	"github.com/haramako/fc/internal/r6502"
)

// ボタンビット (コントローラ1)
const (
	ButtonA = 1 << iota
	ButtonB
	ButtonSelect
	ButtonStart
	ButtonUp
	ButtonDown
	ButtonLeft
	ButtonRight
)

const (
	cyclesPerScanline = 114 // 113.67 の概算
	scanlinesPerFrame = 262
	vblankScanline    = 241
)

// Stats は動作確認用の統計。
type Stats struct {
	Frames               int
	Instructions         int64
	NmiCount             int
	IrqCount             int
	VramWrites           int
	OamDmaCount          int
	RenderingEverEnabled bool
}

type Machine struct {
	Cpu   *r6502.Cpu
	Stats Stats

	prg    []byte
	chr    []byte
	ram    [0x800]byte
	prgRam [0x2000]byte

	mapper         int
	mirrorVertical bool

	// MMC3
	mmc3BankSelect byte
	mmc3Regs       [8]byte
	prgOffsets     [4]int // $8000/$A000/$C000/$E000 の 8KB バンク先頭
	chrOffsets     [8]int // 1KB 単位
	irqLatch       byte
	irqCounter     byte
	irqReload      bool
	irqEnabled     bool
	irqPending     bool

	// PPU
	ctrl        byte
	mask        byte
	status      byte
	oamAddr     byte
	vramAddr    int
	writeToggle bool
	readBuffer  byte
	nt          [0x800]byte
	palette     [32]byte
	oam         [256]byte

	// コントローラ1
	buttons  byte
	strobe   bool
	shiftIdx int
}

// New は iNES 形式の ROM からマシンを作る。
func New(rom []byte) (*Machine, error) {
	if len(rom) < 16 || rom[0] != 'N' || rom[1] != 'E' || rom[2] != 'S' || rom[3] != 0x1a {
		return nil, fmt.Errorf("invalid iNES header")
	}
	prgSize := int(rom[4]) * 0x4000
	chrSize := int(rom[5]) * 0x2000
	mapper := int(rom[6]>>4) | int(rom[7]&0xf0)
	if mapper != 0 && mapper != 4 {
		return nil, fmt.Errorf("unsupported mapper %d", mapper)
	}
	if len(rom) < 16+prgSize+chrSize {
		return nil, fmt.Errorf("ROM too short")
	}
	m := &Machine{
		prg:            rom[16 : 16+prgSize],
		chr:            rom[16+prgSize : 16+prgSize+chrSize],
		mapper:         mapper,
		mirrorVertical: rom[6]&1 != 0,
	}
	if mapper == 4 {
		// 起動時のバンクは電源投入時不定だが、リセットベクタが最終バンクに
		// 固定でいるよう初期化しておく
		m.updateMmc3Banks()
	}
	m.Cpu = r6502.NewCpu(m)
	m.Cpu.Accurate = true
	return m, nil
}

// LoadFile はファイルから ROM を読み込む。
func LoadFile(path string) (*Machine, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return New(b)
}

// SetButtons はコントローラ1の状態を設定する。
func (m *Machine) SetButtons(b byte) { m.buttons = b }

func (m *Machine) renderingEnabled() bool { return m.mask&0x18 != 0 }

// ---------------------------------------------------------------
// r6502.Bus 実装 (CPUメモリマップ)
// ---------------------------------------------------------------

func (m *Machine) Get(addr int) int {
	addr &= 0xffff
	switch {
	case addr < 0x2000:
		return int(m.ram[addr&0x7ff])
	case addr < 0x4000:
		return m.readPpuReg(addr & 7)
	case addr == 0x4016:
		return m.readController()
	case addr < 0x4020:
		return 0 // APU等
	case addr >= 0x6000 && addr < 0x8000:
		return int(m.prgRam[addr-0x6000])
	case addr >= 0x8000:
		return int(m.readPrg(addr))
	}
	return 0
}

func (m *Machine) Set(addr, val int) {
	addr &= 0xffff
	v := byte(val)
	switch {
	case addr < 0x2000:
		m.ram[addr&0x7ff] = v
	case addr < 0x4000:
		m.writePpuReg(addr&7, v)
	case addr == 0x4014:
		m.oamDma(v)
	case addr == 0x4016:
		m.strobe = v&1 != 0
		if m.strobe {
			m.shiftIdx = 0
		}
	case addr < 0x4020:
		// APU等: 無視
	case addr >= 0x6000 && addr < 0x8000:
		m.prgRam[addr-0x6000] = v
	case addr >= 0x8000:
		m.writeMapper(addr, v)
	}
}

func (m *Machine) readPrg(addr int) byte {
	if m.mapper == 4 {
		bank := (addr - 0x8000) / 0x2000
		return m.prg[m.prgOffsets[bank]+(addr-0x8000)&0x1fff]
	}
	// NROM: 16KB×1 はミラー、16KB×2 はそのまま
	return m.prg[(addr-0x8000)%len(m.prg)]
}

func (m *Machine) readController() int {
	if m.strobe {
		return int(m.buttons & 1)
	}
	var bit byte = 1
	if m.shiftIdx < 8 {
		bit = (m.buttons >> m.shiftIdx) & 1
	}
	m.shiftIdx++
	return int(bit)
}

func (m *Machine) oamDma(page byte) {
	base := int(page) << 8
	for i := 0; i < 256; i++ {
		m.oam[(int(m.oamAddr)+i)&0xff] = byte(m.Get(base + i))
	}
	m.Stats.OamDmaCount++
	m.Cpu.Cycles += 513
}

// ---------------------------------------------------------------
// PPU レジスタ
// ---------------------------------------------------------------

func (m *Machine) readPpuReg(reg int) int {
	switch reg {
	case 2: // PPUSTATUS
		r := m.status
		m.status &= 0x7f
		m.writeToggle = false
		return int(r)
	case 4: // OAMDATA
		return int(m.oam[m.oamAddr])
	case 7: // PPUDATA
		addr := m.vramAddr & 0x3fff
		var r byte
		if addr >= 0x3f00 {
			r = m.readVram(addr)
		} else {
			r = m.readBuffer
			m.readBuffer = m.readVram(addr)
		}
		m.incVramAddr()
		return int(r)
	}
	return 0
}

func (m *Machine) writePpuReg(reg int, v byte) {
	switch reg {
	case 0:
		m.ctrl = v
	case 1:
		m.mask = v
		if m.renderingEnabled() {
			m.Stats.RenderingEverEnabled = true
		}
	case 3:
		m.oamAddr = v
	case 4:
		m.oam[m.oamAddr] = v
		m.oamAddr++
	case 5: // PPUSCROLL (値は使わない)
		m.writeToggle = !m.writeToggle
	case 6: // PPUADDR
		if !m.writeToggle {
			m.vramAddr = (m.vramAddr & 0x00ff) | (int(v) << 8)
		} else {
			m.vramAddr = (m.vramAddr & 0xff00) | int(v)
		}
		m.writeToggle = !m.writeToggle
	case 7: // PPUDATA
		m.writeVram(m.vramAddr&0x3fff, v)
		m.Stats.VramWrites++
		m.incVramAddr()
	}
}

func (m *Machine) incVramAddr() {
	if m.ctrl&0x04 != 0 {
		m.vramAddr += 32
	} else {
		m.vramAddr++
	}
}

func (m *Machine) ntIndex(addr int) int {
	addr &= 0x0fff
	if m.mirrorVertical {
		return addr & 0x7ff
	}
	// 水平ミラー
	return ((addr >> 1) & 0x400) | (addr & 0x3ff)
}

func (m *Machine) paletteIndex(addr int) int {
	i := addr & 0x1f
	// $3F10/$14/$18/$1C は $3F00/... のミラー
	if i >= 0x10 && i%4 == 0 {
		i -= 0x10
	}
	return i
}

func (m *Machine) readVram(addr int) byte {
	switch {
	case addr < 0x2000:
		return m.chrAt(addr)
	case addr < 0x3f00:
		return m.nt[m.ntIndex(addr)]
	default:
		return m.palette[m.paletteIndex(addr)]
	}
}

func (m *Machine) writeVram(addr int, v byte) {
	switch {
	case addr < 0x2000:
		// CHR ROM: 書き込み無視
	case addr < 0x3f00:
		m.nt[m.ntIndex(addr)] = v
	default:
		m.palette[m.paletteIndex(addr)] = v
	}
}

func (m *Machine) chrAt(addr int) byte {
	if len(m.chr) == 0 {
		return 0
	}
	if m.mapper == 4 {
		bank := addr / 0x400
		return m.chr[(m.chrOffsets[bank]+(addr&0x3ff))%len(m.chr)]
	}
	return m.chr[addr%len(m.chr)]
}

// ---------------------------------------------------------------
// マッパー (MMC3)
// ---------------------------------------------------------------

func (m *Machine) writeMapper(addr int, v byte) {
	if m.mapper != 4 {
		return
	}
	even := addr&1 == 0
	switch {
	case addr < 0xa000:
		if even {
			m.mmc3BankSelect = v
		} else {
			m.mmc3Regs[m.mmc3BankSelect&7] = v
		}
		m.updateMmc3Banks()
	case addr < 0xc000:
		if even {
			m.mirrorVertical = v&1 == 0
		}
		// $A001 (PRG RAM protect) は無視
	case addr < 0xe000:
		if even {
			m.irqLatch = v
		} else {
			m.irqReload = true
		}
	default:
		if even {
			m.irqEnabled = false
			m.irqPending = false
		} else {
			m.irqEnabled = true
		}
	}
}

func (m *Machine) updateMmc3Banks() {
	prgBanks := len(m.prg) / 0x2000
	bank := func(n byte) int { return (int(n) % prgBanks) * 0x2000 }
	r6, r7 := m.mmc3Regs[6], m.mmc3Regs[7]
	if m.mmc3BankSelect&0x40 == 0 {
		m.prgOffsets = [4]int{bank(r6), bank(r7), (prgBanks - 2) * 0x2000, (prgBanks - 1) * 0x2000}
	} else {
		m.prgOffsets = [4]int{(prgBanks - 2) * 0x2000, bank(r7), bank(r6), (prgBanks - 1) * 0x2000}
	}

	r := m.mmc3Regs
	chr := func(n byte) int { return int(n) * 0x400 }
	if m.mmc3BankSelect&0x80 == 0 {
		m.chrOffsets = [8]int{chr(r[0] &^ 1), chr(r[0] | 1), chr(r[1] &^ 1), chr(r[1] | 1),
			chr(r[2]), chr(r[3]), chr(r[4]), chr(r[5])}
	} else {
		m.chrOffsets = [8]int{chr(r[2]), chr(r[3]), chr(r[4]), chr(r[5]),
			chr(r[0] &^ 1), chr(r[0] | 1), chr(r[1] &^ 1), chr(r[1] | 1)}
	}
}

func (m *Machine) mmc3ClockScanline() {
	if m.irqCounter == 0 || m.irqReload {
		m.irqCounter = m.irqLatch
		m.irqReload = false
	} else {
		m.irqCounter--
	}
	if m.irqCounter == 0 && m.irqEnabled {
		m.irqPending = true
	}
}

// ---------------------------------------------------------------
// 実行
// ---------------------------------------------------------------

// RunFrames は n フレーム実行する。CPUが不正オペコードに当たった場合は
// エラー (クラッシュ検出) を返す。
func (m *Machine) RunFrames(n int) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("crashed at frame %d pc=$%04x: %v", m.Stats.Frames, m.Cpu.Pc, r)
		}
	}()
	for i := 0; i < n; i++ {
		m.runFrame()
	}
	return nil
}

func (m *Machine) runFrame() {
	frameStart := m.Cpu.Cycles
	for scanline := 0; scanline < scanlinesPerFrame; scanline++ {
		target := frameStart + int64((scanline+1)*cyclesPerScanline)
		for m.Cpu.Cycles < target {
			if m.irqPending && m.Cpu.I == 0 {
				m.irqPending = false
				m.Stats.IrqCount++
				m.Cpu.IRQ()
			}
			m.Cpu.StepSilent()
			m.Stats.Instructions++
		}
		if scanline < 240 && m.renderingEnabled() && m.mapper == 4 {
			m.mmc3ClockScanline()
		}
		if scanline == vblankScanline-1 {
			m.status |= 0x80
			m.Stats.Frames++
			if m.ctrl&0x80 != 0 {
				m.Stats.NmiCount++
				m.Cpu.NMI()
			}
		}
	}
	m.status &= 0x7f
}

// ---------------------------------------------------------------
// スクリーンショット (BGのみの静的レンダリング)
// ---------------------------------------------------------------

// Screenshot は現在の nametable / パレット / CHRバンクから背景画面を描画する。
// スクロール・スプライトは無視する (大雑把な確認用)。
func (m *Machine) Screenshot() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 256, 240))
	ntBase := 0x2000 + 0x400*int(m.ctrl&3)
	patBase := 0
	if m.ctrl&0x10 != 0 {
		patBase = 0x1000
	}
	for ty := 0; ty < 30; ty++ {
		for tx := 0; tx < 32; tx++ {
			tile := int(m.readVram(ntBase + ty*32 + tx))
			attr := m.readVram(ntBase + 0x3c0 + (ty/4)*8 + tx/4)
			shift := uint(((ty%4)/2)*4 + ((tx%4)/2)*2)
			palGroup := int((attr >> shift) & 3)
			for py := 0; py < 8; py++ {
				lo := m.chrAt(patBase + tile*16 + py)
				hi := m.chrAt(patBase + tile*16 + 8 + py)
				for px := 0; px < 8; px++ {
					bit := uint(7 - px)
					ci := int((lo>>bit)&1 | ((hi>>bit)&1)<<1)
					var col byte
					if ci == 0 {
						col = m.palette[0]
					} else {
						col = m.palette[palGroup*4+ci]
					}
					img.Set(tx*8+px, ty*8+py, nesPalette[col&0x3f])
				}
			}
		}
	}
	return img
}

// WriteScreenshot は背景画面を PNG として保存する。
func (m *Machine) WriteScreenshot(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, m.Screenshot())
}

// nesPalette は 2C02 の標準的な64色パレット。
var nesPalette = [64]color.RGBA{
	{0x66, 0x66, 0x66, 0xff}, {0x00, 0x2a, 0x88, 0xff}, {0x14, 0x12, 0xa7, 0xff}, {0x3b, 0x00, 0xa4, 0xff},
	{0x5c, 0x00, 0x7e, 0xff}, {0x6e, 0x00, 0x40, 0xff}, {0x6c, 0x06, 0x00, 0xff}, {0x56, 0x1d, 0x00, 0xff},
	{0x33, 0x35, 0x00, 0xff}, {0x0b, 0x48, 0x00, 0xff}, {0x00, 0x52, 0x00, 0xff}, {0x00, 0x4f, 0x08, 0xff},
	{0x00, 0x40, 0x4d, 0xff}, {0x00, 0x00, 0x00, 0xff}, {0x00, 0x00, 0x00, 0xff}, {0x00, 0x00, 0x00, 0xff},
	{0xad, 0xad, 0xad, 0xff}, {0x15, 0x5f, 0xd9, 0xff}, {0x42, 0x40, 0xff, 0xff}, {0x75, 0x27, 0xfe, 0xff},
	{0xa0, 0x1a, 0xcc, 0xff}, {0xb7, 0x1e, 0x7b, 0xff}, {0xb5, 0x31, 0x20, 0xff}, {0x99, 0x4e, 0x00, 0xff},
	{0x6b, 0x6d, 0x00, 0xff}, {0x38, 0x87, 0x00, 0xff}, {0x0c, 0x93, 0x00, 0xff}, {0x00, 0x8f, 0x32, 0xff},
	{0x00, 0x7c, 0x8d, 0xff}, {0x00, 0x00, 0x00, 0xff}, {0x00, 0x00, 0x00, 0xff}, {0x00, 0x00, 0x00, 0xff},
	{0xff, 0xfe, 0xff, 0xff}, {0x64, 0xb0, 0xff, 0xff}, {0x92, 0x90, 0xff, 0xff}, {0xc6, 0x76, 0xff, 0xff},
	{0xf3, 0x6a, 0xff, 0xff}, {0xfe, 0x6e, 0xcc, 0xff}, {0xfe, 0x81, 0x70, 0xff}, {0xea, 0x9e, 0x22, 0xff},
	{0xbc, 0xbe, 0x00, 0xff}, {0x88, 0xd8, 0x00, 0xff}, {0x5c, 0xe4, 0x30, 0xff}, {0x45, 0xe0, 0x82, 0xff},
	{0x48, 0xcd, 0xde, 0xff}, {0x4f, 0x4f, 0x4f, 0xff}, {0x00, 0x00, 0x00, 0xff}, {0x00, 0x00, 0x00, 0xff},
	{0xff, 0xfe, 0xff, 0xff}, {0xc0, 0xdf, 0xff, 0xff}, {0xd3, 0xd2, 0xff, 0xff}, {0xe8, 0xc8, 0xff, 0xff},
	{0xfb, 0xc2, 0xff, 0xff}, {0xfe, 0xc4, 0xea, 0xff}, {0xfe, 0xcc, 0xc5, 0xff}, {0xf7, 0xd8, 0xa5, 0xff},
	{0xe4, 0xe5, 0x94, 0xff}, {0xcf, 0xef, 0x96, 0xff}, {0xbd, 0xf4, 0xab, 0xff}, {0xb3, 0xf3, 0xcc, 0xff},
	{0xb5, 0xeb, 0xf2, 0xff}, {0xb8, 0xb8, 0xb8, 0xff}, {0x00, 0x00, 0x00, 0xff}, {0x00, 0x00, 0x00, 0xff},
}
