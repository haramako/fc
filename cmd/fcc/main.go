// fcc は FC コンパイラの CLI (Ruby 版 bin/fcc 互換)。
//
//	Usage: fcc <command> [options] <src.fc> ...
//	  command: build(b) / compile(c) / run
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/haramako/fc/pkg/fc"
)

const usage = `NES Compiler
Usage: fcc <command> [options] <src.fc> ...
Options:
    -h, --help       show this message
    -o FILE          output file
    -e               run by interpreter
    -S               output asm file
    -d, --debug      show debug info
    -t, --target     target platform ( nes, emu )
    -O LEVEL         optimize level (0-2)
`

func main() {
	os.Exit(run())
}

func run() int {
	// bin/fcc と同じく <command> が先頭に来る
	args := os.Args[1:]
	if len(args) == 0 {
		fmt.Print(usage)
		return 0
	}
	com := args[0]
	fs := flag.NewFlagSet("fcc", flag.ExitOnError)
	fs.Usage = func() { fmt.Print(usage) }
	out := fs.String("o", "", "output file")
	runFlag := fs.Bool("e", false, "run by interpreter")
	// -S / -d は Ruby 版との互換のため受理する (出力には影響しない)
	fs.Bool("S", false, "output asm file")
	debugFlag := fs.Bool("d", false, "show debug info")
	fs.BoolVar(debugFlag, "debug", false, "show debug info")
	target := fs.String("t", "", "target platform ( nes, emu )")
	fs.StringVar(target, "target", "", "target platform ( nes, emu )")
	optLevel := fs.Int("O", 2, "optimize level (0-2)")
	if err := fs.Parse(args[1:]); err != nil {
		return 1
	}
	rest := fs.Args()

	opt := fc.Options{
		Target:        *target,
		Out:           *out,
		Run:           *runFlag,
		OptimizeLevel: *optLevel,
	}
	switch com {
	case "run":
		opt.Run = true
	case "build", "b":
	case "compile", "c":
		opt.CompileOnly = true
	default:
		fmt.Print(usage)
		return 0
	}
	if len(rest) == 0 {
		fmt.Print(usage)
		return 0
	}

	compiler, err := fc.New()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer compiler.Close()

	res, err := compiler.Build(context.Background(), rest[0], opt)
	if err != nil {
		var ce *fc.Error
		if errors.As(err, &ce) {
			fmt.Printf("%s: error: %s\n", ce.Pos, ce.Msg)
			return 1
		}
		fmt.Println(err)
		return 1
	}
	return res.ExitCode
}
