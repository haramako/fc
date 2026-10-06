// Package runner はビルドした ROM・バイナリを内蔵のエミュレータで走らせる (fcc run / fcc test、テスト)。emu は内蔵の 6502
// (internal/emu)、nes は内蔵の NES のランナー (internal/nes。画面は描かない: console の出力と終了コードだけ)。
//
// ビルド (internal/driver) と分けてある (2026-10-06。以前は driver の build が emu を、pkg/fc の Build が NES を走らせていた)。
// driver を使わない: internal/nes のテストが driver を使うので、driver が nes を使うと import が輪になる。
package runner

import (
	"fmt"
	"io"
	"os"

	"github.com/haramako/fc/internal/emu"
	"github.com/haramako/fc/internal/fclog"
	"github.com/haramako/fc/internal/nes"
	"github.com/haramako/fc/internal/r6502"
)

// DefaultFrames は nes で走らせるフレーム数の既定の上限 (60 フレームで 1 秒。1 分)。
const DefaultFrames = 3600

// Options は走らせる設定。
type Options struct {
	Stdout    io.Writer      // プログラムの出力 (console・printf。nil なら捨てる)
	LogOut    io.Writer      // @log の出力 (nil なら Stdout。printf と同じ順に混ざる)
	Log       *fclog.LogFile // @log の地点 (driver.Result.Log。nil なら出さない。emu だけ)
	MaxCycles int64          // emu: サイクル数の上限 (0 なら無制限)。超えたらエラー
	MaxFrames int            // nes: フレーム数の上限 (0 なら DefaultFrames)。console.exit まで走らせる
	TracePC   bool           // emu: invalid opcode などの panic のとき、直近の PC を stderr に出す (FC_TRACE=pc)
}

// Result は走らせた結果。
type Result struct {
	ExitCode int   // 終了コード (console.exit / exit)
	Cycles   int64 // emu: ベンチ区間 (stdio.bench_start / bench_end) の合計、無ければ全体のサイクル数
}

// Run は target ("emu" / "nes") の ROM・バイナリ path を走らせる。
func Run(target, path string, opt Options) (*Result, error) {
	out := opt.Stdout
	if out == nil {
		out = io.Discard
	}
	switch target {
	case "emu":
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		logOut := opt.LogOut
		if logOut == nil {
			logOut = out
		}
		o := emu.Options{Out: out, MaxCycles: opt.MaxCycles, TracePC: opt.TracePC}
		if step := fclog.Stepper(opt.Log, logOut); step != nil {
			o.OnStep = func(pc, prevPC int, cpu *r6502.Cpu, mem *r6502.Memory) {
				step(pc, prevPC, func() fclog.Reader { return fclog.Reader{Mem: mem.Get, A: cpu.A, X: cpu.X, Y: cpu.Y} })
			}
		}
		res, err := emu.Run(data, o)
		if err != nil {
			return nil, err
		}
		return &Result{ExitCode: res.Exit, Cycles: res.Cycles}, nil
	case "nes":
		m, err := nes.LoadFile(path)
		if err != nil {
			return nil, err
		}
		m.Output = out
		frames := opt.MaxFrames
		if frames == 0 {
			frames = DefaultFrames
		}
		if err := m.RunUntilExit(frames); err != nil {
			return nil, err
		}
		return &Result{ExitCode: m.ExitCode}, nil
	}
	return nil, fmt.Errorf("cannot run target %s (emu / nes)", target)
}
