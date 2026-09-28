package main

// fcc test: モジュールの @(test) の関数を走らせる (doc/v4_stdlib.md §7.1)。
//
//	fcc test [-t target] [-O level] <module.fc> ...
//
// 渡したモジュールを use して @run_tests() を呼ぶ main を作り、emu で実行する。@(test) の関数を「モジュール.名前: 」と出してから
// 呼び、@assert / @assert_eq が落ちればそこで止まる (終了コード 1)。全部通れば「N tests ok」と出して終了コード 0。

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/haramako/fc/pkg/fc"
)

const testUsage = `Usage: fcc test [-O level] <module.fc> ...
    -O    optimize level (0-2)
`

func runTest(args []string) int {
	fs := flag.NewFlagSet("fcc test", flag.ExitOnError)
	fs.Usage = func() { fmt.Print(testUsage) }
	level := fs.Int("O", 2, "optimize level")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if fs.NArg() == 0 {
		fmt.Print(testUsage)
		return 0
	}
	compiler, err := fc.New()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer compiler.Close()
	opt := fc.Options{Target: fc.TargetEmu, Stdout: os.Stdout}
	if *level == 0 {
		opt.OptimizeLevel = -1
	} else {
		opt.OptimizeLevel = *level
	}
	res, err := compiler.Test(context.Background(), fs.Args(), opt)
	if err != nil {
		printErrors(err)
		return 1
	}
	return res.ExitCode
}
