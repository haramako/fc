package nes

import (
	"io"
	"os"
	"testing"

	"github.com/haramako/fc/internal/driver"
	"github.com/haramako/fc/internal/emu"
	"github.com/haramako/fc/internal/fclog"
	"github.com/haramako/fc/internal/r6502"
)

// runEmuBuilt はビルドした emu のバイナリ (res) を内蔵の 6502 で走らせ、終了コードを返す。internal/runner と同じ手順 (runner は
// nes を使うので、nes のテストからは使えない: import が輪になる)。@log は res.Log の地点で logOut に出す。
func runEmuBuilt(t *testing.T, res *driver.Result, out, logOut io.Writer, maxCycles int64) int {
	t.Helper()
	data, err := os.ReadFile(res.Out)
	if err != nil {
		t.Fatal(err)
	}
	o := emu.Options{Out: out, MaxCycles: maxCycles}
	if step := fclog.Stepper(res.Log, logOut); step != nil {
		o.OnStep = func(pc, prevPC int, cpu *r6502.Cpu, mem *r6502.Memory) {
			step(pc, prevPC, func() fclog.Reader { return fclog.Reader{Mem: mem.Get, A: cpu.A, X: cpu.X, Y: cpu.Y} })
		}
	}
	r, err := emu.Run(data, o)
	if err != nil {
		t.Fatal(err)
	}
	return r.Exit
}
