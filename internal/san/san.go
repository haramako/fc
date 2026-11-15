// Package san は実行のときの検査 (サニタイザ): 内蔵の 6502 でプログラムを走らせながら、出力には現れないかもしれない
// 「起きてはいけないこと」を静的フレームの影のメモリで捕まえる。差分テスト (-O 0 / -O 2 / インタプリタ) は出力が同じかしか
// 見ないので、壊れても出力に出ない誤り・生成器が出力に使わない値の誤りを見逃す (Agent/wiki/design/sanitizer.md)。
//
// 検査 (emu ターゲット。静的フレームの関数だけ):
//   - 重なり: 関数に入るとき、そのフレームが、実行中 (呼び出しの途中) のほかの関数のフレームと重なっていない。同じ関数に
//     入り直していない (frames.Place の配置・コールグラフの誤り、割り込みからの入り直し)
//   - 未初期化の値の使用: 関数に入るとフレーム (引数を除く) は未初期化、戻ると (戻り値を除いて) 未初期化 (ローカル変数は 0 で
//     始まらない。コンパイラが作った一時変数・asm の作業領域も同じ)。未初期化の印は値と一緒に伝わり (写す・計算する・積む)、
//     その値で分岐する・番地や添字に使う・出力の番地に書くときに誤りにする (読むだけ・写すだけでは誤りにしない: 条件つきで
//     代入する変数をレジスタに載せる、一部だけ初期化した struct を写す、は正しいコード)。
//     積んだ引数を、呼ぶまでの間にほかの関数 (引数の式の中の呼び出し。兄弟の関数のフレームは重ねて置く) が自分のフレームと
//     して使うと、その関数が戻ったときに未初期化になるので、呼び先がその引数を使う所で見つかる
package san

import (
	"fmt"

	"github.com/haramako/fc/internal/r6502"
)

// Range はフレームの中の範囲 (フレームの先頭からのバイト)。
type Range struct{ Off, Size int }

// Func は関数 1 つ。
type Func struct {
	Name    string  // アセンブラのシンボル
	Entries []int   // 入口の番地 (関数のシンボルと、ほかの入口)
	Frame   int     // 静的フレームの番地 (Size が 0 なら無い: stack の関数)
	Size    int     // 静的フレームの大きさ
	Params  []Range // 引数 (呼ぶ側が書く)
	Result  int     // 戻り値の大きさ (フレームの先頭。呼ぶ側が戻った後に読む)
}

// Info は検査に要る、ビルドしたプログラムの情報 (driver が作る)。
type Info struct {
	Funcs []*Func
	// OutLo〜OutHi は出力の番地の範囲 (未初期化の値を書くと誤り)。emu のホストとのやり取りの番地
	OutLo, OutHi int
}

// mark は未初期化の印: 0 は初期化済み、ほかは出どころ (関数の番号 + 1 を上位、フレームの中の位置を下位 16 ビット)。
type mark uint32

func or(a, b mark) mark {
	if a != 0 {
		return a
	}
	return b
}

type act struct {
	fn *Func
	s  int // 入ったときの S (戻ると S はこれより大きくなる)
}

// Checker は検査つきのバス。Cpu.Mem に入れ、命令ごとに Step を呼ぶ。最初の誤りを Err が返す。
type Checker struct {
	bus   r6502.Bus
	info  *Info
	entry map[int]*Func
	index map[*Func]int
	mem   [0x10000]mark  // 番地の値の印
	live  [0x10000]uint8 // 実行中の関数のフレームに入っている数
	stack []act
	// レジスタとフラグの値の印
	a, x, y        mark
	fn, fz, fc, fv mark
	wr             mark // 今の命令が書くバイトの印 (Set が使う)
	pc             int
	err            error
}

// NewChecker は bus を包む検査器。
func NewChecker(bus r6502.Bus, info *Info) *Checker {
	c := &Checker{bus: bus, info: info, entry: map[int]*Func{}, index: map[*Func]int{}}
	for i, f := range info.Funcs {
		c.index[f] = i
		for _, e := range f.Entries {
			c.entry[e] = f
		}
		c.poison(f, 0, f.Size)
	}
	return c
}

// poison は f のフレームの [lo, hi) を未初期化にする (ほかの実行中の関数のフレームと重なる所は除く)。
func (c *Checker) poison(f *Func, lo, hi int) {
	for i := lo; i < hi; i++ {
		if a := (f.Frame + i) & 0xffff; c.live[a] == 0 {
			c.mem[a] = mark(c.index[f]+1)<<16 | mark(i)
		}
	}
}

// Err は最初に見つけた誤り (無ければ nil)。
func (c *Checker) Err() error { return c.err }

func (c *Checker) fail(format string, args ...any) {
	if c.err == nil {
		c.err = fmt.Errorf("sanitizer: "+format+" (pc=$%04x%s)", append(args, c.pc, c.trace())...)
	}
}

// trace は実行中の関数の並び (外側から)。
func (c *Checker) trace() string {
	s := ""
	for _, a := range c.stack {
		s += " > " + a.fn.Name
	}
	if s != "" {
		s = ", in" + s[2:]
	}
	return s
}

// origin は印 m の出どころの説明。
func (c *Checker) origin(m mark) string {
	f, off := c.info.Funcs[int(m>>16)-1], int(m&0xffff)
	s := fmt.Sprintf("uninitialized value from the frame of %s +%d ($%04x)", f.Name, off, f.Frame+off)
	for _, p := range f.Params {
		if off >= p.Off && off < p.Off+p.Size {
			s += ": an argument that was not stored, or was overwritten by a call made after it was stored"
		}
	}
	return s
}

// use は印 m の値を what に使う (未初期化なら誤り)。
func (c *Checker) use(m mark, what string) {
	if m != 0 {
		c.fail("%s an %s", what, c.origin(m))
	}
}

func (c *Checker) enter(f *Func, s int) {
	if f.Size > 0 {
		for a := f.Frame; a < f.Frame+f.Size; a++ {
			if c.live[a&0xffff] == 0 {
				continue
			}
			other := "?"
			for i := len(c.stack) - 1; i >= 0; i-- {
				if g := c.stack[i].fn; a >= g.Frame && a < g.Frame+g.Size {
					other = g.Name
					break
				}
			}
			if other == f.Name {
				c.fail("%s is entered again while it is running (its static frame is in use)", f.Name)
			} else {
				c.fail("the static frame of %s overlaps the live frame of %s at $%04x", f.Name, other, a)
			}
			break
		}
		// 引数は呼ぶ側が書いたまま (レジスタで受ける引数は入口が書く)。ほかは未初期化
		param := make([]bool, f.Size)
		for _, p := range f.Params {
			for i := p.Off; i < p.Off+p.Size && i < f.Size; i++ {
				param[i] = true
			}
		}
		for i := 0; i < f.Size; i++ {
			if !param[i] {
				c.poison(f, i, i+1)
			}
		}
		for i := 0; i < f.Size; i++ {
			c.live[(f.Frame+i)&0xffff]++
		}
	}
	c.stack = append(c.stack, act{f, s})
}

func (c *Checker) leave() {
	n := len(c.stack) - 1
	f := c.stack[n].fn
	c.stack = c.stack[:n]
	for i := 0; i < f.Size; i++ {
		c.live[(f.Frame+i)&0xffff]--
	}
	c.poison(f, f.Result, f.Size) // 戻った関数のローカル変数・引数 (戻り値は呼ぶ側が読む)
}

// Step は命令を実行する前に呼ぶ: 呼び出しの出入りを追い、命令の値の印を伝える。
func (c *Checker) Step(cpu *r6502.Cpu) {
	pc, s := cpu.Pc, cpu.S
	c.pc = pc
	for n := len(c.stack); n > 0 && c.stack[n-1].s < s; n = len(c.stack) {
		c.leave()
	}
	if f := c.entry[pc]; f != nil {
		n := len(c.stack)
		switch {
		case n > 0 && c.stack[n-1].s == s && c.stack[n-1].fn == f:
			// 同じ呼び出しの中で入口を通った (別の入口へ落ちた・関数の頭へ戻るループ)
		case n > 0 && c.stack[n-1].s == s:
			c.leave() // 末尾の jmp での呼び出し: 前の関数は終わった
			c.enter(f, s)
		default:
			c.enter(f, s)
		}
	}
	c.shadow(cpu)
}

// shadow は今の命令の値の印を伝える (命令の実行の前。書き込みの印は wr に置いて Set が番地に入れる)。
func (c *Checker) shadow(cpu *r6502.Cpu) {
	get := c.bus.Get
	sec, thd := get(c.pc+1), get(c.pc+2)
	instr, mode := r6502.InstrMode(get(c.pc))
	// 番地と、番地を作るのに使った値の印
	var ea int
	var at, mt mark
	hasMem := true
	switch mode {
	case r6502.Zp:
		ea = sec
	case r6502.Zpx:
		ea, at = (sec+cpu.X)&0xff, c.x
	case r6502.Zpy:
		ea, at = (sec+cpu.Y)&0xff, c.y
	case r6502.Abs:
		ea = thd<<8 | sec
	case r6502.Absx:
		ea, at = (thd<<8+sec+cpu.X)&0xffff, c.x
	case r6502.Absy:
		ea, at = (thd<<8+sec+cpu.Y)&0xffff, c.y
	case r6502.Ind:
		p := thd<<8 | sec
		at, hasMem = or(c.mem[p], c.mem[thd<<8|(sec+1)&0xff]), false
	case r6502.Indx:
		p := (cpu.X + sec) & 0xff
		ea, at = get(p)|get((p+1)&0xff)<<8, or(c.x, or(c.mem[p], c.mem[(p+1)&0xff]))
	case r6502.Indy:
		ea, at = (get(sec)|get((sec+1)&0xff)<<8+cpu.Y)&0xffff, or(c.y, or(c.mem[sec], c.mem[(sec+1)&0xff]))
	default:
		hasMem = false
	}
	if instr == r6502.JSR || instr == r6502.JMP {
		hasMem = false // 飛び先 (値を読まない)
	}
	c.use(at, "an address or index is made from")
	if hasMem {
		mt = c.mem[ea]
	}
	if mode == r6502.Acc {
		mt = c.a
	}
	nz := func(m mark) { c.fn, c.fz = m, m }
	c.wr = 0
	stackTop := c.mem[0x100+(cpu.S+1)&0xff]
	switch instr {
	case r6502.LDA:
		c.a = mt
		nz(mt)
	case r6502.LDX:
		c.x = mt
		nz(mt)
	case r6502.LDY:
		c.y = mt
		nz(mt)
	case r6502.STA:
		c.wr = c.a
	case r6502.STX:
		c.wr = c.x
	case r6502.STY:
		c.wr = c.y
	case r6502.TAX:
		c.x = c.a
		nz(c.a)
	case r6502.TAY:
		c.y = c.a
		nz(c.a)
	case r6502.TXA:
		c.a = c.x
		nz(c.x)
	case r6502.TYA:
		c.a = c.y
		nz(c.y)
	case r6502.TSX:
		c.x = 0
		nz(0)
	case r6502.ADC, r6502.SBC:
		c.a = or(c.a, or(mt, c.fc))
		nz(c.a)
		c.fc, c.fv = c.a, c.a
	case r6502.AND, r6502.ORA, r6502.EOR:
		c.a = or(c.a, mt)
		nz(c.a)
	case r6502.CMP:
		nz(or(c.a, mt))
		c.fc = c.fn
	case r6502.CPX:
		nz(or(c.x, mt))
		c.fc = c.fn
	case r6502.CPY:
		nz(or(c.y, mt))
		c.fc = c.fn
	case r6502.BIT:
		c.fn, c.fv, c.fz = mt, mt, or(c.a, mt)
	case r6502.INC, r6502.DEC:
		c.wr = mt
		nz(mt)
	case r6502.INX, r6502.DEX:
		nz(c.x)
	case r6502.INY, r6502.DEY:
		nz(c.y)
	case r6502.ASL, r6502.LSR, r6502.ROL, r6502.ROR:
		m := mt
		if instr == r6502.ROL || instr == r6502.ROR {
			m = or(m, c.fc)
		}
		if mode == r6502.Acc {
			c.a = m
		} else {
			c.wr = m
		}
		nz(m)
		c.fc = m
	case r6502.BPL, r6502.BMI:
		c.use(c.fn, "a branch depends on")
	case r6502.BVC, r6502.BVS:
		c.use(c.fv, "a branch depends on")
	case r6502.BCC, r6502.BCS:
		c.use(c.fc, "a branch depends on")
	case r6502.BNE, r6502.BEQ:
		c.use(c.fz, "a branch depends on")
	case r6502.CLC, r6502.SEC:
		c.fc = 0
	case r6502.CLV:
		c.fv = 0
	case r6502.PHA:
		c.wr = c.a
	case r6502.PHP:
		c.wr = or(or(c.fn, c.fz), or(c.fc, c.fv))
	case r6502.PLA:
		c.a = stackTop
		nz(c.a)
	case r6502.PLP, r6502.RTI:
		c.fn, c.fz, c.fc, c.fv = stackTop, stackTop, stackTop, stackTop
	}
}

// Get は読み出し (印は Step が命令ごとに伝える)。
func (c *Checker) Get(addr int) int { return c.bus.Get(addr) }

// Set は書き込み: 今の命令が書く値の印を番地に入れる。
func (c *Checker) Set(addr, val int) {
	addr &= 0xffff
	if c.wr != 0 && addr >= c.info.OutLo && addr < c.info.OutHi {
		c.use(c.wr, "the program outputs")
	}
	c.mem[addr] = c.wr
	c.bus.Set(addr, val)
}
