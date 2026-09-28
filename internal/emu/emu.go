// Package emu は emu ターゲットの実行 (fcc run とテスト): ROM を LoadAddr に置いて 6502 (r6502) で走らせ、ホストとの
// やり取りの取り決め ($fff0〜$ffff) で出力・ベンチ区間・終了コードを受ける (fclib/emu/stdio.fc がこの番地に書く)。
//
//	PortAddr  ($fff0, 2 バイト): 文字列の番地 (PortPrint = 1 / 6)
//	PortData  ($fff2, 2 バイト): 数 (PortPrint = 2 / 3)、バイト数 (PortPrint = 6)
//	PortPrint ($fffe): 1 = 終端 0 の文字列を出す、2 = 数を出す、3 = 数と空白を出す、4 = ベンチ区間の開始、5 = 終了、
//	                   6 = PortAddr から PortData バイトを出す (途中の 0 もそのまま: fclib/emu/console.fc)。処理したら 255 に戻す
//	PortExit  ($ffff): 255 以外が書かれたら終了 (その値が終了コード)
//
// internal/interp (IR のインタプリタ) も同じ取り決めで stdio を実行する。
package emu

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/haramako/fc/internal/r6502"
)

const (
	LoadAddr  = 0x1000
	PortAddr  = 0xfff0
	PortData  = 0xfff2
	PortPrint = 0xfffe
	PortExit  = 0xffff
)

// Options は実行の設定。
type Options struct {
	Out       io.Writer // print の出力 (nil なら捨てる)
	MaxCycles int64     // サイクル数の上限 (0 なら無制限)。超えたらエラー (差分テストの無限ループ対策)
	TracePC   bool      // invalid opcode などの panic のとき、直近の PC を stderr に出す (調査用。FC_TRACE_PC)
	// OnStep は命令を実行する前に呼ぶ (nil なら呼ばない。@log の地点の出力)。prevPC は直前に実行した命令の番地
	// (rts の後は戻り先の直前の jsr: 呼び出しの直後の合流点の地点の Prevs は jsr を指す)
	OnStep func(pc, prevPC int, cpu *r6502.Cpu, mem *r6502.Memory)
}

// Result は実行の結果。
type Result struct {
	Exit   int   // PortExit に書かれた終了コード
	Cycles int64 // ベンチ区間 (PortPrint = 4 / 5) の合計、無ければ全体のサイクル数
}

// Run は rom を実行する。
func Run(rom []byte, o Options) (Result, error) {
	out := o.Out
	if out == nil {
		out = io.Discard
	}
	mem := r6502.NewMemory()
	for i, b := range rom {
		mem.Set(LoadAddr+i, int(b))
	}
	cpu := r6502.NewCpu(mem)
	cpu.Pc = LoadAddr
	cpu.TrapZpWrap = true // FC_STACK (ZP 128 バイト) のあふれは S+k,x のページ越えとして見える
	mem.Set(PortExit, 255)
	mem.Set(PortPrint, 255)
	var benchStart, benchCycles int64
	benchUsed := false
	prevPC := -1
	var trace []int // 直近の PC (invalid opcode の panic で表示する)
	if o.TracePC {
		defer func() {
			if r := recover(); r != nil {
				var b strings.Builder
				for _, pc := range trace {
					fmt.Fprintf(&b, " $%04x", pc)
				}
				fmt.Fprintf(os.Stderr, "FC_TRACE_PC (last %d):%s\n", len(trace), b.String())
				panic(r)
			}
		}()
	}
	for mem.Get(PortExit) == 255 {
		if o.TracePC {
			trace = append(trace, cpu.Pc)
			if len(trace) > 48 {
				trace = trace[1:]
			}
		}
		if o.OnStep != nil {
			o.OnStep(cpu.Pc, prevPC, cpu, mem)
			prevPC = cpu.Pc
			if mem.Get(cpu.Pc) == 0x60 {
				// rts の後は、戻り先の直前の jsr を直前の命令とみなす
				ret := mem.Get(0x100+(cpu.S+1)&0xff) | mem.Get(0x100+(cpu.S+2)&0xff)<<8
				prevPC = (ret - 2) & 0xffff
			}
		}
		cpu.StepSilent()
		if o.MaxCycles > 0 && cpu.Cycles > o.MaxCycles {
			return Result{}, fmt.Errorf("cycle limit exceeded (%d cycles, pc=$%04x)", o.MaxCycles, cpu.Pc)
		}
		if mem.Get(PortPrint) != 255 {
			switch mem.Get(PortPrint) {
			case 1:
				addr := mem.Get(PortAddr) + (mem.Get(PortAddr+1) << 8)
				var sb []byte
				for mem.Get(addr) != 0 {
					sb = append(sb, byte(mem.Get(addr)))
					addr++
				}
				fmt.Fprint(out, string(sb))
			case 2:
				fmt.Fprint(out, mem.Get(PortData)+(mem.Get(PortData+1)<<8))
			case 3:
				fmt.Fprint(out, mem.Get(PortData)+(mem.Get(PortData+1)<<8), " ")
			case 6:
				addr := mem.Get(PortAddr) + (mem.Get(PortAddr+1) << 8)
				n := mem.Get(PortData) + (mem.Get(PortData+1) << 8)
				sb := make([]byte, n)
				for i := range sb {
					sb[i] = byte(mem.Get((addr + i) & 0xffff))
				}
				out.Write(sb)
			case 4:
				benchStart = cpu.Cycles
				benchUsed = true
			case 5:
				benchCycles += cpu.Cycles - benchStart
			}
			mem.Set(PortPrint, 255)
		}
	}
	if !benchUsed {
		benchCycles = cpu.Cycles
	}
	return Result{Exit: mem.Get(PortExit), Cycles: benchCycles}, nil
}
