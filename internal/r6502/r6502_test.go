package r6502

// 実機の 6502 との突き合わせ (bug hunt 2026-10-18 の Phase 3 の 1)。r6502 は emu も内蔵の NES ランナーも使う唯一の実行系で、
// 差分 fuzz の -O 0 / -O 2 は同じ r6502 の上で比べるので、r6502 の誤りは codegen の誤りを隠すか、codegen の誤りに見える。
// ここの期待値は r6502 のコードを引かずに、公式の命令の表と命令の定義から書いている。

import (
	"fmt"
	"strings"
	"testing"
)

// officialOp は公式の命令 1 つ (オペコード・ニーモニック・モード・基本サイクル数・ページをまたぐ読み出しの +1)。
type officialOp struct {
	op    int
	mnem  string
	mode  Mode
	cyc   int
	cross bool
}

// officialOps は公式の 151 命令 (MOS の資料の表)。
var officialOps = func() []officialOp {
	var ops []officialOp
	add := func(mnem string, list string) {
		for _, f := range strings.Fields(list) {
			var op, cyc int
			var mode string
			if _, err := fmt.Sscanf(f, "%x:%3s:%d", &op, &mode, &cyc); err != nil {
				panic(f)
			}
			cross := strings.HasSuffix(f, "+")
			m := map[string]Mode{"imp": Imp, "acc": Acc, "imm": Imm, "zp_": Zp, "zpx": Zpx, "zpy": Zpy, "rel": Rel,
				"abs": Abs, "abx": Absx, "aby": Absy, "ind": Ind, "inx": Indx, "iny": Indy}[mode]
			ops = append(ops, officialOp{op, mnem, m, cyc, cross})
		}
	}
	alu := func(mnem string, imm, zp, zpx, abs, abx, aby, inx, iny int) {
		add(mnem, fmt.Sprintf("%02x:imm:2 %02x:zp_:3 %02x:zpx:4 %02x:abs:4 %02x:abx:4+ %02x:aby:4+ %02x:inx:6 %02x:iny:5+",
			imm, zp, zpx, abs, abx, aby, inx, iny))
	}
	rmw := func(mnem string, acc, zp, zpx, abs, abx int) {
		s := fmt.Sprintf("%02x:zp_:5 %02x:zpx:6 %02x:abs:6 %02x:abx:7", zp, zpx, abs, abx)
		if acc >= 0 {
			s = fmt.Sprintf("%02x:acc:2 ", acc) + s
		}
		add(mnem, s)
	}
	alu("adc", 0x69, 0x65, 0x75, 0x6d, 0x7d, 0x79, 0x61, 0x71)
	alu("and", 0x29, 0x25, 0x35, 0x2d, 0x3d, 0x39, 0x21, 0x31)
	alu("cmp", 0xc9, 0xc5, 0xd5, 0xcd, 0xdd, 0xd9, 0xc1, 0xd1)
	alu("eor", 0x49, 0x45, 0x55, 0x4d, 0x5d, 0x59, 0x41, 0x51)
	alu("lda", 0xa9, 0xa5, 0xb5, 0xad, 0xbd, 0xb9, 0xa1, 0xb1)
	alu("ora", 0x09, 0x05, 0x15, 0x0d, 0x1d, 0x19, 0x01, 0x11)
	alu("sbc", 0xe9, 0xe5, 0xf5, 0xed, 0xfd, 0xf9, 0xe1, 0xf1)
	rmw("asl", 0x0a, 0x06, 0x16, 0x0e, 0x1e)
	rmw("lsr", 0x4a, 0x46, 0x56, 0x4e, 0x5e)
	rmw("rol", 0x2a, 0x26, 0x36, 0x2e, 0x3e)
	rmw("ror", 0x6a, 0x66, 0x76, 0x6e, 0x7e)
	rmw("inc", -1, 0xe6, 0xf6, 0xee, 0xfe)
	rmw("dec", -1, 0xc6, 0xd6, 0xce, 0xde)
	add("sta", "85:zp_:3 95:zpx:4 8d:abs:4 9d:abx:5 99:aby:5 81:inx:6 91:iny:6")
	add("stx", "86:zp_:3 96:zpy:4 8e:abs:4")
	add("sty", "84:zp_:3 94:zpx:4 8c:abs:4")
	add("ldx", "a2:imm:2 a6:zp_:3 b6:zpy:4 ae:abs:4 be:aby:4+")
	add("ldy", "a0:imm:2 a4:zp_:3 b4:zpx:4 ac:abs:4 bc:abx:4+")
	add("cpx", "e0:imm:2 e4:zp_:3 ec:abs:4")
	add("cpy", "c0:imm:2 c4:zp_:3 cc:abs:4")
	add("bit", "24:zp_:3 2c:abs:4")
	add("jmp", "4c:abs:3 6c:ind:5")
	add("jsr", "20:abs:6")
	for _, b := range []string{"10:bpl", "30:bmi", "50:bvc", "70:bvs", "90:bcc", "b0:bcs", "d0:bne", "f0:beq"} {
		p := strings.Split(b, ":")
		add(p[1], p[0]+":rel:2")
	}
	for _, b := range []string{"00:brk:7", "40:rti:6", "60:rts:6", "08:php:3", "28:plp:4", "48:pha:3", "68:pla:4",
		"18:clc:2", "38:sec:2", "58:cli:2", "78:sei:2", "b8:clv:2", "d8:cld:2", "f8:sed:2", "ea:nop:2",
		"aa:tax:2", "8a:txa:2", "a8:tay:2", "98:tya:2", "ba:tsx:2", "9a:txs:2",
		"ca:dex:2", "e8:inx:2", "88:dey:2", "c8:iny:2"} {
		p := strings.Split(b, ":")
		add(p[1], p[0]+":imp:"+p[2])
	}
	return ops
}()

// modeBytes はモードの命令の長さ。
func modeBytes(m Mode) int {
	switch m {
	case Imp, Acc:
		return 1
	case Abs, Absx, Absy, Ind:
		return 3
	}
	return 2
}

// cpuAt は $0200 に命令 code を置いた CPU (ベクタは $0200)。
func cpuAt(code ...int) (*Cpu, *Memory) {
	m := NewMemory()
	for i, b := range code {
		m.Set(0x200+i, b)
	}
	m.Set(0xfffc, 0x00)
	m.Set(0xfffd, 0x02)
	return NewCpu(m), m
}

func TestOfficialOpcodes(t *testing.T) {
	if len(officialOps) != 151 {
		t.Fatalf("表の命令の数 %d (151 のはず)", len(officialOps))
	}
	known := map[int]bool{}
	for _, o := range officialOps {
		known[o.op] = true
		instr, mode := InstrMode(o.op)
		if instr.String() != o.mnem || mode != o.mode {
			t.Errorf("$%02x: %s mode %d、want %s mode %d", o.op, instr.String(), mode, o.mnem, o.mode)
		}
	}
	for op := 0; op < 256; op++ {
		if instr, _ := InstrMode(op); !known[op] && instr != INVALID {
			t.Errorf("$%02x: 非公式の命令が %s として解釈される", op, instr.String())
		}
	}
}

// TestCyclesAndLength は命令の長さと、ページをまたがない・またぐときのサイクル数。
func TestCyclesAndLength(t *testing.T) {
	for _, o := range officialOps {
		switch o.mnem {
		case "jmp", "jsr", "rts", "rti", "brk":
			continue // 飛ぶ (長さは個別のテスト)
		}
		// オペランドは $10 / $0310 (X = Y = 1 でもページをまたがない)。(zp),y の指す先は $0310
		c, m := cpuAt(o.op, 0x10, 0x03)
		m.Set(0x10, 0x10)
		m.Set(0x11, 0x03)
		c.X, c.Y = 1, 1
		c.Z, c.C, c.N, c.V = 0, 0, 0, 0
		if o.mode == Rel {
			// 成立しない分岐 (bne は Z=1、beq は Z=0 …)
			c.Z, c.C, c.N, c.V = 1, 1, 1, 1
			if o.mnem == "beq" || o.mnem == "bcs" || o.mnem == "bmi" || o.mnem == "bvs" {
				c.Z, c.C, c.N, c.V = 0, 0, 0, 0
			}
		}
		c.StepSilent()
		if got := c.Pc - 0x200; got != modeBytes(o.mode) {
			t.Errorf("$%02x %s: 長さ %d、want %d", o.op, o.mnem, got, modeBytes(o.mode))
		}
		if int(c.Cycles) != o.cyc {
			t.Errorf("$%02x %s: サイクル %d、want %d", o.op, o.mnem, c.Cycles, o.cyc)
		}
		if o.mode != Absx && o.mode != Absy && o.mode != Indy {
			continue
		}
		// ページをまたぐ: 基準 $03F0 + $20
		c, m = cpuAt(o.op, 0xf0, 0x03)
		m.Set(0xf0, 0xf0)
		m.Set(0xf1, 0x03)
		c.X, c.Y = 0x20, 0x20
		c.StepSilent()
		want := o.cyc
		if o.cross {
			want++
		}
		if int(c.Cycles) != want {
			t.Errorf("$%02x %s: ページをまたぐとサイクル %d、want %d", o.op, o.mnem, c.Cycles, want)
		}
	}
}

func TestBranchCycles(t *testing.T) {
	// bne が成立: 同じページ +1、またぐ +2
	c, _ := cpuAt(0xd0, 0x10)
	c.StepSilent()
	if c.Pc != 0x212 || c.Cycles != 3 {
		t.Errorf("同じページの成立: pc $%04x サイクル %d", c.Pc, c.Cycles)
	}
	c, _ = cpuAt(0xd0, 0x80) // $0202 - 128 = $0182
	c.StepSilent()
	if c.Pc != 0x182 || c.Cycles != 4 {
		t.Errorf("ページをまたぐ成立: pc $%04x サイクル %d", c.Pc, c.Cycles)
	}
}

// flags は N V - - D I Z C を 1 バイトにする (比べる用)。
func flags(c *Cpu) int { return c.N<<7 | c.V<<6 | c.D<<3 | c.I<<2 | c.Z<<1 | c.C }

func nz(v int) (n, z int) {
	v &= 0xff
	return v >> 7, b2i(v == 0)
}

// TestALUExhaustive は値を扱う命令の結果とフラグを、全部の入力で命令の定義と比べる (2 進モード)。
func TestALUExhaustive(t *testing.T) {
	type res struct{ a, n, z, c, v int }
	check := func(name string, a, m, cin int, got, want res) bool {
		if got != want {
			t.Errorf("%s a=$%02x m=$%02x c=%d: got %+v want %+v", name, a, m, cin, got, want)
			return false
		}
		return true
	}
	imm := map[string]int{"adc": 0x69, "sbc": 0xe9, "and": 0x29, "ora": 0x09, "eor": 0x49, "cmp": 0xc9}
	for _, name := range []string{"adc", "sbc", "and", "ora", "eor", "cmp"} {
	loop:
		for a := 0; a < 256; a++ {
			for m := 0; m < 256; m++ {
				for cin := 0; cin < 2; cin++ {
					c, _ := cpuAt(imm[name], m)
					c.A, c.C, c.V = a, cin, 1
					c.StepSilent()
					got := res{c.A, c.N, c.Z, c.C, c.V}
					var want res
					switch name {
					case "adc", "sbc":
						mm := m
						if name == "sbc" {
							mm = m ^ 0xff
						}
						s := a + mm + cin
						r := s & 0xff
						want.a, want.c = r, b2i(s > 0xff)
						want.v = b2i((a^r)&(mm^r)&0x80 != 0)
						want.n, want.z = nz(r)
					case "and", "ora", "eor":
						r := map[string]int{"and": a & m, "ora": a | m, "eor": a ^ m}[name]
						want = res{a: r, c: cin, v: 1}
						want.n, want.z = nz(r)
					case "cmp":
						want = res{a: a, c: b2i(a >= m), v: 1}
						want.n, want.z = nz(a - m)
					}
					if !check(name, a, m, cin, got, want) {
						break loop
					}
				}
			}
		}
	}
}

// TestShiftsExhaustive は asl / lsr / rol / ror / inc / dec を、A とメモリ (zp) の両方で全部の値について比べる。
func TestShiftsExhaustive(t *testing.T) {
	ops := map[string][2]int{"asl": {0x0a, 0x06}, "lsr": {0x4a, 0x46}, "rol": {0x2a, 0x26}, "ror": {0x6a, 0x66},
		"inc": {-1, 0xe6}, "dec": {-1, 0xc6}}
	for _, name := range []string{"asl", "lsr", "rol", "ror", "inc", "dec"} {
		for _, onMem := range []bool{false, true} {
			op := ops[name][b2i(onMem)]
			if op < 0 {
				continue
			}
		loop:
			for v := 0; v < 256; v++ {
				for cin := 0; cin < 2; cin++ {
					c, m := cpuAt(op, 0x10)
					c.C = cin
					if onMem {
						m.Set(0x10, v)
						c.A = 0x55
					} else {
						c.A = v
					}
					c.StepSilent()
					r, cout := 0, cin
					switch name {
					case "asl":
						r, cout = (v<<1)&0xff, v>>7
					case "lsr":
						r, cout = v>>1, v&1
					case "rol":
						r, cout = (v<<1)&0xff|cin, v>>7
					case "ror":
						r, cout = v>>1|cin<<7, v&1
					case "inc":
						r = (v + 1) & 0xff
					case "dec":
						r = (v - 1) & 0xff
					}
					got := c.A
					if onMem {
						got = m.Get(0x10)
						if c.A != 0x55 {
							t.Errorf("%s (zp) が A を変えた", name)
						}
					}
					n, z := nz(r)
					if got != r || c.C != cout || c.N != n || c.Z != z {
						t.Errorf("%s mem=%v v=$%02x c=%d: 結果 $%02x C=%d N=%d Z=%d、want $%02x C=%d N=%d Z=%d",
							name, onMem, v, cin, got, c.C, c.N, c.Z, r, cout, n, z)
						break loop
					}
				}
			}
		}
	}
}

// TestLoadsCompareBit は ldx / ldy / cpx / cpy / bit と、レジスタの移動・増減。
func TestLoadsCompareBit(t *testing.T) {
	for v := 0; v < 256; v++ {
		n, z := nz(v)
		for _, op := range []int{0xa9, 0xa2, 0xa0} { // lda / ldx / ldy #v
			c, _ := cpuAt(op, v)
			c.StepSilent()
			if c.N != n || c.Z != z {
				t.Errorf("$%02x #$%02x: N=%d Z=%d", op, v, c.N, c.Z)
			}
		}
		for r := 0; r < 256; r += 17 {
			for _, op := range []int{0xe0, 0xc0} { // cpx / cpy #v
				c, _ := cpuAt(op, v)
				c.X, c.Y = r, r
				c.StepSilent()
				wn, wz := nz(r - v)
				if c.C != b2i(r >= v) || c.N != wn || c.Z != wz {
					t.Errorf("$%02x r=$%02x #$%02x: C=%d N=%d Z=%d", op, r, v, c.C, c.N, c.Z)
				}
			}
			c, m := cpuAt(0x24, 0x10) // bit $10
			m.Set(0x10, v)
			c.A = r
			c.StepSilent()
			if c.N != v>>7 || c.V != (v>>6)&1 || c.Z != b2i(r&v == 0) || c.A != r {
				t.Errorf("bit a=$%02x m=$%02x: N=%d V=%d Z=%d", r, v, c.N, c.V, c.Z)
			}
		}
		for _, tc := range []struct {
			op   int
			name string
		}{{0xaa, "tax"}, {0xa8, "tay"}, {0x8a, "txa"}, {0x98, "tya"}, {0xe8, "inx"}, {0xc8, "iny"}, {0xca, "dex"}, {0x88, "dey"}} {
			c, _ := cpuAt(tc.op)
			c.A, c.X, c.Y = v, v, v
			c.StepSilent()
			want := v
			switch tc.name {
			case "inx", "iny":
				want = (v + 1) & 0xff
			case "dex", "dey":
				want = (v - 1) & 0xff
			}
			got := map[string]int{"tax": c.X, "tay": c.Y, "txa": c.A, "tya": c.A, "inx": c.X, "iny": c.Y, "dex": c.X, "dey": c.Y}[tc.name]
			wn, wz := nz(want)
			if got != want || c.N != wn || c.Z != wz {
				t.Errorf("%s v=$%02x: $%02x N=%d Z=%d", tc.name, v, got, c.N, c.Z)
			}
		}
	}
}

// TestStack はスタックの命令 (S はページ 1 の中で回る、php は B と bit 5 を 1 で積む、plp は B を持たない)。
func TestStack(t *testing.T) {
	// S = 0 で pha → $0100 に積んで S は $FF
	c, m := cpuAt(0x48, 0xba) // pha; tsx
	c.S, c.A = 0, 0x42
	c.StepSilent()
	c.StepSilent()
	if m.Get(0x100) != 0x42 || c.S != 0xff || c.X != 0xff {
		t.Errorf("S=0 の pha: $0100=$%02x S=%d X=%d (S=$FF, X=$FF のはず)", m.Get(0x100), c.S, c.X)
	}
	// S = $FF で pla → $0100 から読んで S は 0
	c, m = cpuAt(0x68, 0xba) // pla; tsx
	c.S = 0xff
	m.Set(0x100, 0x99)
	c.StepSilent()
	c.StepSilent()
	if c.A != 0x99 || c.S != 0 || c.X != 0 {
		t.Errorf("S=$FF の pla: A=$%02x S=%d X=%d", c.A, c.S, c.X)
	}
	// jsr / rts がページ 1 の中で回る (S = 0 で jsr)
	c, m = cpuAt(0x20, 0x00, 0x03) // jsr $0300
	m.Set(0x300, 0x60)             // rts
	c.S = 0
	c.StepSilent()
	if m.Get(0x100) != 0x02 || m.Get(0x1ff) != 0x02 || c.S != 0xfe {
		t.Errorf("S=0 の jsr: $0100=$%02x $01FF=$%02x S=$%02x", m.Get(0x100), m.Get(0x1ff), c.S)
	}
	c.StepSilent()
	if c.Pc != 0x203 || c.S != 0 {
		t.Errorf("rts: pc=$%04x S=$%02x", c.Pc, c.S)
	}
	// php は B (bit 4) と bit 5 を 1 で積む
	c, m = cpuAt(0x08) // php
	c.N, c.V, c.D, c.I, c.Z, c.C = 1, 0, 0, 1, 0, 1
	c.StepSilent()
	if got := m.Get(0x1ff); got != 0xb5 {
		t.Errorf("php が $%02x を積んだ ($B5 のはず)", got)
	}
}

// TestJmpIndirectPageBug は jmp ($xxFF) が上位を $xx00 から読む実機の不具合 (fc のランタイムは避けて書いている:
// fclib/nes/frame.asm)。エミュレータが正しい番地へ飛ぶと、置き場所が $xxFF になったときの誤りが隠れる。
func TestJmpIndirectPageBug(t *testing.T) {
	c, m := cpuAt(0x6c, 0xff, 0x03) // jmp ($03FF)
	m.Set(0x3ff, 0x34)
	m.Set(0x400, 0x12) // 正しくはここ
	m.Set(0x300, 0x56) // 実機はここ
	c.StepSilent()
	if c.Pc != 0x5634 {
		t.Errorf("jmp ($03FF) が $%04x へ飛んだ (実機は $5634)", c.Pc)
	}
}

// TestBrkAndRti は brk が brk の番地 + 2 を積み (B を立てて)、I を立てて $FFFE へ飛ぶこと、rti で戻ること。
func TestBrkAndRti(t *testing.T) {
	c, m := cpuAt(0x00, 0xea) // brk; (パディング)
	m.Set(0xfffe, 0x00)
	m.Set(0xffff, 0x03)
	m.Set(0x300, 0x40) // rti
	c.StepSilent()
	if c.Pc != 0x300 || c.I != 1 || m.Get(0x1ff) != 0x02 || m.Get(0x1fe) != 0x02 || m.Get(0x1fd)&0x30 != 0x30 {
		t.Errorf("brk: pc=$%04x I=%d 積んだ $%02x $%02x $%02x", c.Pc, c.I, m.Get(0x1ff), m.Get(0x1fe), m.Get(0x1fd))
	}
	c.StepSilent()
	if c.Pc != 0x202 || c.S != 0xff {
		t.Errorf("rti: pc=$%04x S=$%02x", c.Pc, c.S)
	}
}

// TestAddressWrap は 16 ビットを超える番地が $0000 から回ること (abs,x / (zp),y)。
func TestAddressWrap(t *testing.T) {
	c, m := cpuAt(0xbd, 0xff, 0xff) // lda $FFFF,x
	c.X = 2
	m.Set(0x0001, 0x77)
	c.StepSilent()
	if c.A != 0x77 {
		t.Errorf("lda $FFFF,x (x=2) が $%02x ($0001 の $77 のはず)", c.A)
	}
	// zp,x はゼロページの中で回る
	c, m = cpuAt(0xb5, 0xf0) // lda $F0,x
	c.X = 0x20
	m.Set(0x10, 0x66)
	c.StepSilent()
	if c.A != 0x66 {
		t.Errorf("lda $F0,x (x=$20) が $%02x ($0010 の $66 のはず)", c.A)
	}
}
