package main

// fcc test: モジュールの @(test) の関数を走らせる (Agent/wiki/plans/v4-stdlib.md §7.1)。
//
//	fcc test [-t emu|nes] [-O level] <module.fc> ...
//
// 渡したモジュールを use して @run_tests() を呼ぶ main を作り、emu (既定) か内蔵の NES のランナーで実行する。@(test) の関数を「モジュール.名前: 」と出してから
// 呼び、@assert / @assert_eq が落ちればそこで止まる (終了コード 1)。全部通れば「N tests ok」と出して終了コード 0。

import (
	"context"
	"fmt"
	"os"

	"github.com/haramako/fc/pkg/fc"
)

type testCmd struct {
	Target  string   `short:"t" default:"emu" enum:"emu,nes" placeholder:"emu|nes" help:"Target (emu: the built-in 6502 emulator, nes: the built-in NES runner; default ${default})."`
	Opt     int      `short:"O" default:"2" placeholder:"LEVEL" complete:"0,1,2" help:"Optimize level (0-2, default ${default})."`
	Modules []string `arg:"" name:"module" help:"Module source files (.fc)."`
}

func (c *testCmd) run() int {
	compiler, err := fc.New()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer compiler.Close()
	opt := fc.Options{Target: c.Target, Stdout: os.Stdout, OptimizeLevel: optimizeLevel(c.Opt)}
	res, err := compiler.Test(context.Background(), c.Modules, opt)
	if res != nil {
		printWarnings(res.Warnings)
	}
	if err != nil {
		printErrors(err)
		return 1
	}
	return res.ExitCode
}
