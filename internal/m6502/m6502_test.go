package m6502_test

// m6502 の表 (codegen のピープホール・レジスタの書き込みの検査・regalloc の見積もりが引く) を、r6502 で実際に走らせた結果と
// 突き合わせる (bug hunt 2026-10-18 の Phase 3 の 1)。r6502 は r6502_test.go で公式の命令の表と比べてある。

import (
	"math/rand"
	"testing"

	"github.com/haramako/fc/internal/m6502"
	"github.com/haramako/fc/internal/r6502"
)

var modeOf = map[r6502.Mode]m6502.Mode{
	r6502.Imp: m6502.Imp, r6502.Acc: m6502.Imp, r6502.Imm: m6502.Imm, r6502.Zp: m6502.ZP, r6502.Zpx: m6502.ZPX,
	r6502.Zpy: m6502.ZPY, r6502.Rel: m6502.Rel, r6502.Abs: m6502.Abs, r6502.Absx: m6502.AbsX, r6502.Absy: m6502.AbsY,
	r6502.Ind: m6502.Ind, r6502.Indx: m6502.IndX, r6502.Indy: m6502.IndY,
}

// notTaken は分岐が成立しないフラグ (Z C N V を全部同じ値にする)。
var notTaken = map[string]int{"bpl": 1, "bvc": 1, "bcc": 1, "bne": 1, "bmi": 0, "bvs": 0, "bcs": 0, "beq": 0}

type step struct {
	a, x, y, n, z int
	cycles        int64
}

// run は $0200 に命令を置いて 1 つ走らせる。オペランドは $10 / $0310 (ページをまたがない)。
func run(op int, a, x, y, flags int, mem func(m *r6502.Memory)) (before, after step) {
	m := r6502.NewMemory()
	m.Set(0x200, op)
	m.Set(0x201, 0x10)
	m.Set(0x202, 0x03)
	m.Set(0x10, 0x10)
	m.Set(0x11, 0x03)
	m.Set(0xfffc, 0x00)
	m.Set(0xfffd, 0x02)
	if mem != nil {
		mem(m)
	}
	c := r6502.NewCpu(m)
	c.A, c.X, c.Y = a, x, y
	c.Z, c.C, c.N, c.V = flags, flags, flags, flags
	before = step{c.A, c.X, c.Y, c.N, c.Z, 0}
	c.StepSilent()
	return before, step{c.A, c.X, c.Y, c.N, c.Z, c.Cycles}
}

func official() map[string]map[m6502.Mode]int {
	ops := map[string]map[m6502.Mode]int{}
	for op := 0; op < 256; op++ {
		instr, mode := r6502.InstrMode(op)
		if instr == r6502.INVALID {
			continue
		}
		name := instr.String()
		if ops[name] == nil {
			ops[name] = map[m6502.Mode]int{}
		}
		ops[name][modeOf[mode]] = op
	}
	return ops
}

// TestCyclesMatchR6502 は表のモードとサイクル数が、実際の命令 (r6502) と一致すること。表にあって実機に無いモードも誤り
// (存在しない命令のサイクルを見積もりに使う・検査を通してしまう)。
func TestCyclesMatchR6502(t *testing.T) {
	ops := official()
	for name, modes := range ops {
		if _, ok := m6502.Table[name]; !ok {
			t.Errorf("%s が表に無い", name)
			continue
		}
		for mode, op := range modes {
			flags := 0
			if f, ok := notTaken[name]; ok {
				flags = f
			}
			_, after := run(op, 0, 1, 1, flags, nil)
			if got := m6502.Cycles(name, mode); got != int(after.cycles) {
				t.Errorf("%s (mode %d, $%02x): 表 %d サイクル、r6502 %d", name, mode, op, got, after.cycles)
			}
		}
	}
	for name := range m6502.Table {
		for mode := m6502.Imp; mode <= m6502.Rel; mode++ {
			if m6502.Cycles(name, mode) == 0 {
				continue
			}
			if _, ok := ops[name][mode]; !ok {
				t.Errorf("%s: 表に実機に無いモード %d がある", name, mode)
			}
		}
	}
}

// TestWritesAndFlags は、表が「書かない」と言うレジスタを命令が書かないこと、KeepsNZ の命令が N / Z を変えないこと、
// NZ の命令が N / Z を結果で決めること (少なくとも 1 つの入力で変わる) を、ランダムな値で確かめる。
func TestWritesAndFlags(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	for name, modes := range official() {
		info := m6502.Table[name]
		for mode, op := range modes {
			wa, wx, wy := m6502.Writes(name, mode)
			nzChanged := false
			for k := 0; k < 200; k++ {
				a, x, y, v := r.Intn(256), r.Intn(4), r.Intn(4), r.Intn(256)
				flags := r.Intn(2)
				if name == "brk" || name == "rti" || name == "jsr" || name == "rts" || name == "plp" {
					continue // フラグ・PC をスタックから: 表は何も約束しない
				}
				before, after := run(op, a, x, y, flags, func(m *r6502.Memory) {
					for addr := 0x10; addr < 0x18; addr++ {
						m.Set(addr, v)
					}
					m.Set(0x10, 0x10) // (zp) の指す先は $0310 のまま
					m.Set(0x11, 0x03)
					for addr := 0x310; addr < 0x318; addr++ {
						m.Set(addr, v)
					}
					m.Set(0x1ff, v) // pla
				})
				if !wa && after.a != before.a || !wx && after.x != before.x || !wy && after.y != before.y {
					t.Errorf("%s (mode %d): 表は (A %v X %v Y %v) を書くと言うが、%+v → %+v", name, mode, wa, wx, wy, before, after)
					break
				}
				if info.KeepsNZ && (after.n != before.n || after.z != before.z) {
					t.Errorf("%s (mode %d): KeepsNZ なのに N / Z が変わった %+v → %+v", name, mode, before, after)
					break
				}
				if after.n != before.n || after.z != before.z {
					nzChanged = true
				}
			}
			if info.NZ && !nzChanged {
				t.Errorf("%s (mode %d): NZ と言うが、200 の入力で N / Z が一度も変わらなかった", name, mode)
			}
			if !info.NZ && !info.KeepsNZ && nzChanged && name != "plp" {
				t.Logf("%s (mode %d): NZ でも KeepsNZ でもないが N / Z が変わる", name, mode)
			}
			if info.Implied != (mode == m6502.Imp && name != "asl" && name != "lsr" && name != "rol" && name != "ror") {
				t.Errorf("%s (mode %d): Implied=%v", name, mode, info.Implied)
			}
		}
	}
}
