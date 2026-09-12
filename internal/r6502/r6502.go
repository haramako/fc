// Package r6502 は lib/r6502 (Ruby製6502エミュレータ) の厳密移植。
// 実機と異なる挙動 (zpx のページクロス非マスク、indx の二重参照など) も
// Ruby 版の演算子優先順位どおりに再現している。
package r6502

// Bus はCPUから見えるメモリ空間。Memory のほか、NESのメモリマップ実装
// (internal/nes) などを差し込める。
type Bus interface {
	Get(addr int) int
	Set(addr, val int)
}

// Memory は R6502::Memory 相当 (疎なメモリ、未設定は0)。
type Memory struct {
	m map[int]int
}

func NewMemory() *Memory {
	return &Memory{m: map[int]int{}}
}

func (m *Memory) Set(addr, val int) {
	m.m[addr] = val & 0xff
}

func (m *Memory) Get(addr int) int {
	return m.m[addr]
}

func (m *Memory) GetWord(addr int) int {
	return m.Get(addr) + (m.Get(addr+1) << 8)
}

// Mode はアドレッシングモード。
type Mode int

const (
	Imp Mode = iota
	Acc
	Imm
	Zp
	Zpx
	Zpy
	Rel
	Abs
	Absx
	Absy
	Ind
	Indx
	Indy
)

// Instr は命令の種類。
type Instr int

const (
	ADC Instr = iota
	AND
	ASL
	BIT
	BPL
	BMI
	BVC
	BVS
	BCC
	BCS
	BNE
	BEQ
	BRK
	CMP
	CPX
	CPY
	DEC
	EOR
	CLC
	SEC
	CLI
	SEI
	CLV
	CLD
	SED
	INC
	JMP
	JSR
	LDA
	LDX
	LDY
	LSR
	NOP
	ORA
	TAX
	TXA
	DEX
	INX
	TAY
	TYA
	DEY
	INY
	ROL
	ROR
	RTI
	RTS
	SBC
	STA
	TXS
	TSX
	PHA
	PLA
	PHP
	PLP
	STX
	STY
	INVALID
)

type instrMode struct {
	instr Instr
	mode  Mode
}

var instrTable = map[int]instrMode{
	// adc
	0x69: {ADC, Imm}, 0x65: {ADC, Zp}, 0x75: {ADC, Zpx}, 0x6d: {ADC, Abs},
	0x7d: {ADC, Absx}, 0x79: {ADC, Absy}, 0x61: {ADC, Indx}, 0x71: {ADC, Indy},
	// and
	0x29: {AND, Imm}, 0x25: {AND, Zp}, 0x35: {AND, Zpx}, 0x2d: {AND, Abs},
	0x3d: {AND, Absx}, 0x39: {AND, Absy}, 0x21: {AND, Indx}, 0x31: {AND, Indy},
	// asl
	0x0a: {ASL, Acc}, 0x06: {ASL, Zp}, 0x16: {ASL, Zpx}, 0x0e: {ASL, Abs}, 0x1e: {ASL, Absx},
	// bit
	0x24: {BIT, Zp}, 0x2c: {BIT, Abs},
	// branches
	0x10: {BPL, Rel}, 0x30: {BMI, Rel}, 0x50: {BVC, Rel}, 0x70: {BVS, Rel},
	0x90: {BCC, Rel}, 0xb0: {BCS, Rel}, 0xd0: {BNE, Rel}, 0xf0: {BEQ, Rel},
	// brk
	0x00: {BRK, Imp},
	// cmp
	0xc9: {CMP, Imm}, 0xc5: {CMP, Zp}, 0xd5: {CMP, Zpx}, 0xcd: {CMP, Abs},
	0xdd: {CMP, Absx}, 0xd9: {CMP, Absy}, 0xc1: {CMP, Indx}, 0xd1: {CMP, Indy},
	// cpx
	0xe0: {CPX, Imm}, 0xe4: {CPX, Zp}, 0xec: {CPX, Abs},
	// cpy
	0xc0: {CPY, Imm}, 0xc4: {CPY, Zp}, 0xcc: {CPY, Abs},
	// dec
	0xc6: {DEC, Zp}, 0xd6: {DEC, Zpx}, 0xce: {DEC, Abs}, 0xde: {DEC, Absx},
	// eor
	0x49: {EOR, Imm}, 0x45: {EOR, Zp}, 0x55: {EOR, Zpx}, 0x4d: {EOR, Abs},
	0x5d: {EOR, Absx}, 0x59: {EOR, Absy}, 0x41: {EOR, Indx}, 0x51: {EOR, Indy},
	// flag ops
	0x18: {CLC, Imp}, 0x38: {SEC, Imp}, 0x58: {CLI, Imp}, 0x78: {SEI, Imp},
	0xb8: {CLV, Imp}, 0xd8: {CLD, Imp}, 0xf8: {SED, Imp},
	// inc
	0xe6: {INC, Zp}, 0xf6: {INC, Zpx}, 0xee: {INC, Abs}, 0xfe: {INC, Absx},
	// jmp
	0x4c: {JMP, Abs}, 0x6c: {JMP, Ind},
	// jsr
	0x20: {JSR, Abs},
	// lda
	0xa9: {LDA, Imm}, 0xa5: {LDA, Zp}, 0xb5: {LDA, Zpx}, 0xad: {LDA, Abs},
	0xbd: {LDA, Absx}, 0xb9: {LDA, Absy}, 0xa1: {LDA, Indx}, 0xb1: {LDA, Indy},
	// ldx
	0xa2: {LDX, Imm}, 0xa6: {LDX, Zp}, 0xb6: {LDX, Zpy}, 0xae: {LDX, Abs}, 0xbe: {LDX, Absy},
	// ldy
	0xa0: {LDY, Imm}, 0xa4: {LDY, Zp}, 0xb4: {LDY, Zpx}, 0xac: {LDY, Abs}, 0xbc: {LDY, Absx},
	// lsr
	0x4a: {LSR, Acc}, 0x46: {LSR, Zp}, 0x56: {LSR, Zpx}, 0x4e: {LSR, Abs}, 0x5e: {LSR, Absx},
	// nop
	0xea: {NOP, Imp},
	// ora
	0x09: {ORA, Imm}, 0x05: {ORA, Zp}, 0x15: {ORA, Zpx}, 0x0d: {ORA, Abs},
	0x1d: {ORA, Absx}, 0x19: {ORA, Absy}, 0x01: {ORA, Indx}, 0x11: {ORA, Indy},
	// transfers etc
	0xaa: {TAX, Imp}, 0x8a: {TXA, Imp}, 0xca: {DEX, Imp}, 0xe8: {INX, Imp},
	0xa8: {TAY, Imp}, 0x98: {TYA, Imp}, 0x88: {DEY, Imp}, 0xc8: {INY, Imp},
	// rol
	0x2a: {ROL, Acc}, 0x26: {ROL, Zp}, 0x36: {ROL, Zpx}, 0x2e: {ROL, Abs}, 0x3e: {ROL, Absx},
	// ror
	0x6a: {ROR, Acc}, 0x66: {ROR, Zp}, 0x76: {ROR, Zpx}, 0x6e: {ROR, Abs}, 0x7e: {ROR, Absx},
	// rti / rts
	0x40: {RTI, Imp}, 0x60: {RTS, Imp},
	// sbc
	0xe9: {SBC, Imm}, 0xe5: {SBC, Zp}, 0xf5: {SBC, Zpx}, 0xed: {SBC, Abs},
	0xfd: {SBC, Absx}, 0xf9: {SBC, Absy}, 0xe1: {SBC, Indx}, 0xf1: {SBC, Indy},
	// sta
	0x85: {STA, Zp}, 0x95: {STA, Zpx}, 0x8d: {STA, Abs}, 0x9d: {STA, Absx},
	0x99: {STA, Absy}, 0x81: {STA, Indx}, 0x91: {STA, Indy},
	// stack
	0x9a: {TXS, Imp}, 0xba: {TSX, Imp}, 0x48: {PHA, Imp}, 0x68: {PLA, Imp},
	0x08: {PHP, Imp}, 0x28: {PLP, Imp},
	// stx
	0x86: {STX, Zp}, 0x96: {STX, Zpy}, 0x8e: {STX, Abs},
	// sty
	0x84: {STY, Zp}, 0x94: {STY, Zpx}, 0x8c: {STY, Abs},
}

// Cpu は R6502::Cpu 相当。
type Cpu struct {
	Mem Bus
	Pc  int
	S   int
	X   int
	Y   int
	A   int
	// フラグ (0/1)
	C, Z, I, D, B, V, N int

	// Accurate を true にすると、Ruby版の再現である誤った挙動
	// (zpx/zpy のページクロス非マスク、indx の二重参照) を実機準拠に直す。
	// emu ターゲットの golden テストは false のまま使う。
	Accurate bool

	// Cycles は概算の消費サイクル数 (フレームタイミング用。正確ではない)。
	Cycles int64
}

func NewCpu(mem Bus) *Cpu {
	return &Cpu{
		Mem: mem,
		Pc:  mem.Get(0xfffc) + (mem.Get(0xfffd) << 8),
		S:   0xff,
	}
}

// InstrMode はオペコードから命令とモードを引く。
func InstrMode(opcode int) (Instr, Mode) {
	im, ok := instrTable[opcode]
	if !ok {
		return INVALID, Imp
	}
	return im.instr, im.mode
}

// decodeArg は Cpu#decode_arg 相当 (Ruby版の演算子優先順位の癖も再現)。
// arg なし(imp/acc)は hasArg=false。
func (c *Cpu) decodeArg(mode Mode, secWord, thdWord int) int {
	switch mode {
	case Imp:
		return 0
	case Imm:
		return secWord
	case Zp:
		return secWord
	case Zpx:
		if c.Accurate {
			return 0xff & (secWord + c.X)
		}
		return secWord + c.X // マスクなし (Ruby版と同じ)
	case Zpy:
		if c.Accurate {
			return 0xff & (secWord + c.Y)
		}
		return secWord + c.Y
	case Rel:
		if secWord <= 127 {
			return secWord
		}
		return secWord - 256
	case Abs:
		return (thdWord << 8) + secWord
	case Absx:
		return (thdWord << 8) + secWord + c.X
	case Absy:
		return (thdWord << 8) + secWord + c.Y
	case Ind:
		lb := c.Mem.Get((thdWord << 8) + secWord)
		hb := c.Mem.Get((thdWord << 8) + secWord + 1)
		return (hb << 8) + lb
	case Indx:
		lb := c.Mem.Get(0xff & (c.X + secWord))
		hb := c.Mem.Get(0xff & (c.X + secWord + 1))
		if c.Accurate {
			return (hb << 8) + lb
		}
		// Ruby版は最後にもう一度 mem.get する (実機と異なる挙動の再現)
		return c.Mem.Get((hb << 8) + lb)
	case Indy:
		lb := c.Mem.Get(0xff & secWord)
		hb := c.Mem.Get(0xff & (secWord + 1))
		addr := (hb << 8) + lb
		return addr + c.Y
	}
	return 0
}

func (c *Cpu) incPcByMode(mode Mode) {
	switch mode {
	case Imp, Acc:
		c.Pc += 1
	case Imm, Zp, Zpx, Zpy, Indx, Indy, Rel:
		c.Pc += 2
	case Abs, Absx, Absy, Ind:
		c.Pc += 3
	}
}

// StepSilent は step_silent 相当 (デバッグ出力なしの1ステップ実行)。
func (c *Cpu) StepSilent() {
	instr, mode := InstrMode(c.Mem.Get(c.Pc))
	arg := c.decodeArg(mode, c.Mem.Get(c.Pc+1), c.Mem.Get(c.Pc+2))
	c.Cycles += approxCycles(instr, mode)
	c.exec(instr, arg, mode)
}

// approxCycles は命令の概算サイクル数 (ページクロス・分岐成立ペナルティは無視)。
func approxCycles(instr Instr, mode Mode) int64 {
	switch instr {
	case BRK:
		return 7
	case JSR, RTS, RTI:
		return 6
	case PHA, PHP:
		return 3
	case PLA, PLP:
		return 4
	case JMP:
		if mode == Ind {
			return 5
		}
		return 3
	case ASL, LSR, ROL, ROR, INC, DEC:
		if mode == Acc {
			return 2
		}
		switch mode {
		case Zp:
			return 5
		case Zpx, Abs:
			return 6
		default:
			return 7
		}
	}
	switch mode {
	case Imp, Acc, Imm, Rel:
		return 2
	case Zp:
		return 3
	case Zpx, Zpy, Abs, Absx, Absy:
		return 4
	case Indx:
		return 6
	case Indy:
		return 5
	}
	return 2
}

// NMI は NMI 割り込みを発生させる (実機準拠。NESランナー用)。
func (c *Cpu) NMI() {
	c.interrupt(0xfffa)
}

// IRQ は IRQ 割り込みを発生させる (Iフラグは呼び出し側で確認すること)。
func (c *Cpu) IRQ() {
	c.interrupt(0xfffe)
}

func (c *Cpu) interrupt(vector int) {
	c.Mem.Set(0x0100+(0xff&c.S), (c.Pc>>8)&0xff)
	c.S--
	c.Mem.Set(0x0100+(0xff&c.S), c.Pc&0xff)
	c.S--
	val := c.N             // bit 7
	val = (val << 1) + c.V // bit 6
	val = (val << 1) + 1   // bit 5
	val = (val << 1) + 0   // bit 4 (B=0: 割り込み)
	val = (val << 1) + c.D // bit 3
	val = (val << 1) + c.I // bit 2
	val = (val << 1) + c.Z // bit 1
	val = (val << 1) + c.C // bit 0
	c.Mem.Set(0x0100+(0xff&c.S), val)
	c.S--
	c.I = 1
	c.Cycles += 7
	c.Pc = c.Mem.Get(vector) + (c.Mem.Get(vector+1) << 8)
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (c *Cpu) exec(instr Instr, arg int, mode Mode) {
	m := c.Mem
	switch instr {

	case ADC:
		x := c.A
		y := arg
		if mode != Imm {
			y = m.Get(arg)
		}
		if c.D == 0 { // normal binary mode
			r := x + y + c.C
			c.A = 0xff & r
			c.V = (((0x7f & x) + (0x7f & y) + c.C) >> 7) ^ ((x + y + c.C) >> 8)
			c.Z = b2i(r&0xff == 0)
			c.C = b2i(r > 255)
			c.N = (0x80 & r) >> 7
		} else { // BCD mode
			ones := (0xf & x) + (0xf & y)
			tens := ((0xf0 & x) >> 4) + ((0xf0 & y) >> 4)
			r0 := ones + 10*tens + c.C
			c.C = b2i(r0 > 99)
			r := r0 % 100
			c.Z = b2i(r == 0)
			c.A = r + 6*(r/10)
			c.N = (0x80 & c.A) >> 7
		}
		c.incPcByMode(mode)

	case SBC:
		x := c.A
		y := arg
		if mode != Imm {
			y = m.Get(arg)
		}
		if c.D == 0 {
			y = y ^ 0xff
			r := x + y + c.C
			c.A = 0xff & r
			c.V = (((0x7f & x) + (0x7f & y) + c.C) >> 7) ^ ((x + y + c.C) >> 8)
			c.Z = b2i(0xff&r == 0)
			c.C = b2i(r > 255)
			c.N = (0x80 & r) >> 7
		} else {
			ones := (0xf & x) - (0xf & y)
			tens := ((0xf0 & x) >> 4) - ((0xf0 & y) >> 4)
			r0 := ones + 10*tens - (1 - c.C)
			c.C = b2i(r0 >= 0)
			r := rubyIntMod(r0, 100)
			c.Z = b2i(r == 0)
			c.A = r + 6*(r/10)
			c.N = (0x80 & c.A) >> 7
		}
		c.incPcByMode(mode)

	case AND:
		if mode == Imm {
			c.A = c.A & arg
		} else {
			c.A = c.A & m.Get(arg)
		}
		c.Z = b2i(c.A == 0)
		c.N = c.A >> 7
		c.incPcByMode(mode)

	case ASL:
		if mode == Acc {
			r := c.A << 1
			c.A = r & 0xff
			c.Z = b2i(c.A == 0)
			c.N = c.A >> 7
			c.C = b2i(r > 0xff)
		} else {
			r := m.Get(arg) << 1
			m.Set(arg, r&0xff)
			c.Z = b2i(r == 0)
			c.N = r >> 7
			c.C = b2i(r > 0xff)
		}
		c.incPcByMode(mode)

	case BIT:
		mv := m.Get(arg)
		result := c.A & mv
		c.Z = b2i(result == 0)
		c.V = (mv & 0x40) >> 6
		c.N = (mv & 0x80) >> 7
		c.incPcByMode(mode)

	case DEC:
		r := m.Get(arg) - 1
		m.Set(arg, r&0xff)
		c.Z = b2i(r == 0)
		c.N = (r & 0x80) >> 7
		c.incPcByMode(mode)

	case DEX:
		r := c.X - 1
		c.X = r & 0xff
		c.Z = b2i(c.X == 0)
		c.N = (c.X & 0x80) >> 7
		c.incPcByMode(mode)

	case DEY:
		r := c.Y - 1
		c.Y = r & 0xff
		c.Z = b2i(c.Y == 0)
		c.N = (c.Y & 0x80) >> 7
		c.incPcByMode(mode)

	case EOR:
		if mode == Imm {
			c.A = c.A ^ arg
		} else {
			c.A = c.A ^ m.Get(arg)
		}
		c.Z = b2i(c.A == 0)
		c.N = (c.A & 0x80) >> 7
		c.incPcByMode(mode)

	case INC:
		r := (m.Get(arg) + 1) & 0xff
		m.Set(arg, r)
		c.Z = b2i(r == 0)
		c.N = (r & 0x80) >> 7
		c.incPcByMode(mode)

	case INX:
		c.X = (c.X + 1) & 0xff
		c.Z = b2i(c.X == 0)
		c.N = (c.X & 0x80) >> 7
		c.incPcByMode(mode)

	case INY:
		c.Y = (c.Y + 1) & 0xff
		c.Z = b2i(c.Y == 0)
		c.N = (c.Y & 0x80) >> 7
		c.incPcByMode(mode)

	case LSR:
		if mode == Acc {
			c.C = 0x01 & c.A
			c.A = c.A >> 1
			c.Z = b2i(c.A == 0)
			c.N = (c.A & 0x80) >> 7
		} else {
			v := m.Get(arg)
			c.C = 0x01 & v
			r := v >> 1
			c.Z = b2i(r == 0)
			c.N = (r & 0x80) >> 7
			m.Set(arg, r)
		}
		c.incPcByMode(mode)

	case ORA:
		if mode == Imm {
			c.A = c.A | arg
		} else {
			c.A = c.A | m.Get(arg)
		}
		c.Z = b2i(c.A == 0)
		c.N = (c.A & 0x80) >> 7
		c.incPcByMode(mode)

	case ROL:
		if mode == Acc {
			cc := c.C
			c.C = (c.A & 0x80) >> 7
			c.A = 0xff&(c.A<<1) | cc
			c.Z = b2i(c.A == 0)
			c.N = (c.A & 0x80) >> 7
		} else {
			val := m.Get(arg)
			cc := c.C
			c.C = (val & 0x80) >> 7
			r := 0xff&(val<<1) | cc
			c.Z = b2i(r == 0)
			c.N = (r & 0x80) >> 7
			m.Set(arg, r)
		}
		c.incPcByMode(mode)

	case ROR:
		if mode == Acc {
			cc := c.C
			c.C = c.A & 0x01
			c.A = (c.A >> 1) | (cc << 7)
			c.Z = b2i(c.A == 0)
			c.N = (c.A & 0x80) >> 7
		} else {
			val := m.Get(arg)
			cc := c.C
			c.C = val & 0x01
			r := (val >> 1) | (cc << 7)
			m.Set(arg, r)
			c.Z = b2i(r == 0)
			c.N = (r & 0x80) >> 7
		}
		c.incPcByMode(mode)

	case NOP:
		c.incPcByMode(mode)

	case SEC:
		c.C = 1
		c.incPcByMode(mode)
	case SED:
		c.D = 1
		c.incPcByMode(mode)
	case SEI:
		c.I = 1
		c.incPcByMode(mode)
	case CLC:
		c.C = 0
		c.incPcByMode(mode)
	case CLD:
		c.D = 0
		c.incPcByMode(mode)
	case CLI:
		c.I = 0
		c.incPcByMode(mode)
	case CLV:
		c.V = 0
		c.incPcByMode(mode)

	case BCC:
		c.incPcByMode(Rel)
		if c.C == 0 {
			c.Pc += arg
		}
	case BCS:
		c.incPcByMode(Rel)
		if c.C == 1 {
			c.Pc += arg
		}
	case BEQ:
		c.incPcByMode(Rel)
		if c.Z == 1 {
			c.Pc += arg
		}
	case BMI:
		c.incPcByMode(Rel)
		if c.N == 1 {
			c.Pc += arg
		}
	case BNE:
		c.incPcByMode(Rel)
		if c.Z == 0 {
			c.Pc += arg
		}
	case BPL:
		c.incPcByMode(Rel)
		if c.N == 0 {
			c.Pc += arg
		}
	case BVC:
		c.incPcByMode(Rel)
		if c.V == 0 {
			c.Pc += arg
		}
	case BVS:
		c.incPcByMode(Rel)
		if c.V == 1 {
			c.Pc += arg
		}

	case CMP:
		v := arg
		if mode != Imm {
			v = m.Get(arg)
		}
		result := c.A - v
		c.C = b2i(result >= 0)
		c.Z = b2i(result == 0)
		c.N = (0xff & result) >> 7
		c.incPcByMode(mode)

	case CPX:
		v := arg
		if mode != Imm {
			v = m.Get(arg)
		}
		result := c.X - v
		c.C = b2i(result >= 0)
		c.Z = b2i(result == 0)
		c.N = (0xff & result) >> 7
		c.incPcByMode(mode)

	case CPY:
		v := arg
		if mode != Imm {
			v = m.Get(arg)
		}
		result := c.Y - v
		c.C = b2i(result >= 0)
		c.Z = b2i(result == 0)
		c.N = (0xff & result) >> 7
		c.incPcByMode(mode)

	case JMP:
		c.Pc = arg

	case LDA:
		if mode == Imm {
			c.A = arg
		} else {
			c.A = m.Get(arg)
		}
		c.Z = b2i(c.A == 0)
		c.N = (0x80 & c.A) >> 7
		c.incPcByMode(mode)

	case LDX:
		if mode == Imm {
			c.X = arg
		} else {
			c.X = m.Get(arg)
		}
		c.Z = b2i(c.X == 0)
		c.N = (0x80 & c.X) >> 7
		c.incPcByMode(mode)

	case LDY:
		if mode == Imm {
			c.Y = arg
		} else {
			c.Y = m.Get(arg)
		}
		c.Z = b2i(c.Y == 0)
		c.N = (0x80 & c.Y) >> 7
		c.incPcByMode(mode)

	case PHA:
		addr := 0x0100 + (0xff & c.S)
		m.Set(addr, c.A)
		c.S -= 1
		c.incPcByMode(mode)

	case PLA:
		addr := 0x0100 + (0xff & (c.S + 1))
		c.A = m.Get(addr)
		c.S += 1
		c.Z = b2i(c.A == 0)
		c.N = (0x80 & c.A) >> 7
		c.incPcByMode(mode)

	case PHP:
		addr := 0x0100 + (0xff & c.S)
		val := c.N             // bit 7
		val = (val << 1) + c.V // bit 6
		val = (val << 1) + 1   // bit 5
		val = (val << 1) + c.B // bit 4
		val = (val << 1) + c.D // bit 3
		val = (val << 1) + c.I // bit 2
		val = (val << 1) + c.Z // bit 1
		val = (val << 1) + c.C // bit 0
		m.Set(addr, val)
		c.S -= 1
		c.incPcByMode(mode)

	case PLP:
		addr := 0x0100 + (0xff & (c.S + 1))
		val := m.Get(addr)
		c.C = 0x1 & val
		c.Z = 0x1 & (val >> 1)
		c.I = 0x1 & (val >> 2)
		c.D = 0x1 & (val >> 3)
		c.B = 0x1 & (val >> 4)
		// bit 5
		c.V = 0x1 & (val >> 6)
		c.N = 0x1 & (val >> 7)
		c.incPcByMode(mode)

	case STA:
		m.Set(arg, c.A)
		c.incPcByMode(mode)
	case STX:
		m.Set(arg, c.X)
		c.incPcByMode(mode)
	case STY:
		m.Set(arg, c.Y)
		c.incPcByMode(mode)

	case TAX:
		c.X = c.A
		c.Z = b2i(c.X == 0)
		c.N = (0x80 & c.X) >> 7
		c.incPcByMode(mode)
	case TAY:
		c.Y = c.A
		c.Z = b2i(c.Y == 0)
		c.N = (0x80 & c.Y) >> 7
		c.incPcByMode(mode)
	case TSX:
		c.X = c.S
		c.Z = b2i(c.X == 0)
		c.N = (0x80 & c.X) >> 7
		c.incPcByMode(mode)
	case TXA:
		c.A = c.X
		c.Z = b2i(c.A == 0)
		c.N = (0x80 & c.A) >> 7
		c.incPcByMode(mode)
	case TXS:
		c.S = c.X
		c.incPcByMode(mode)
	case TYA:
		c.A = c.Y
		c.Z = b2i(c.A == 0)
		c.N = (0x80 & c.A) >> 7
		c.incPcByMode(mode)

	case BRK:
		m.Set(0x0100+c.S, c.Pc>>8)
		c.S -= 1
		m.Set(0x0100+c.S, 0xff&c.Pc)
		c.S -= 1
		val := c.N             // bit 7
		val = (val << 1) + c.V // bit 6
		val = (val << 1) + 1   // bit 5
		val = (val << 1) + 1   // bit 4 break flag
		val = (val << 1) + c.D // bit 3
		val = (val << 1) + c.I // bit 2
		val = (val << 1) + c.Z // bit 1
		val = (val << 1) + c.C // bit 0
		m.Set(0x0100+c.S, val)
		c.S -= 1
		lo := m.Get(0xfffe)
		hi := m.Get(0xffff)
		c.Pc = (hi << 8) + lo

	case RTI:
		flags := m.Get(0x0100 + c.S + 1)
		c.S += 1
		c.C = 0x1 & flags
		c.Z = 0x1 & (flags >> 1)
		c.I = 0x1 & (flags >> 2)
		c.D = 0x1 & (flags >> 3)
		c.B = 0x1 & (flags >> 4)
		// bit 5
		c.V = 0x1 & (flags >> 6)
		c.N = 0x1 & (flags >> 7)
		// 注: Ruby版は hi/lo を逆順に取り出すバグがあった (fc の emuターゲットでは
		// rti は一度も実行されないため露見しない)。実機準拠 (flags→lo→hi) に修正。
		lo := m.Get(0x0100 + c.S + 1)
		c.S += 1
		hi := m.Get(0x0100 + c.S + 1)
		c.S += 1
		c.Pc = 0xffff & ((hi << 8) + lo)

	case JSR:
		m.Set(0x0100+c.S, (c.Pc+2)>>8)
		c.S -= 1
		m.Set(0x0100+c.S, 0xff&(c.Pc+2))
		c.S -= 1
		c.Pc = arg

	case RTS:
		lo := m.Get(0x0100 + c.S + 1)
		c.S += 1
		hi := m.Get(0x0100 + c.S + 1)
		c.S += 1
		c.Pc = 0xffff & ((hi << 8) + lo + 1)

	default:
		panic("invalid opcode")
	}
}

// rubyIntMod は Ruby の % (floor剰余)。
func rubyIntMod(a, b int) int {
	m := a % b
	if m != 0 && (m < 0) != (b < 0) {
		m += b
	}
	return m
}
