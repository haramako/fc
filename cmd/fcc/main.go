// fcc は FC コンパイラの CLI。
//
//	Usage: fcc <command> [options] <src.fc> ...
//	  command: build(b) / compile(c) / run / fmt / check
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/haramako/fc/pkg/fc"
)

const usage = `NES Compiler
Usage: fcc <command> [options] <src.fc> ...
Commands:
    build, b         build ROM / binary
    compile, c       compile to object files only
    run              build and run by emulator
    fmt              format source files (see fcc fmt -h)
    check            compile without producing files and report errors / warnings
    version          show version
Options:
    -h, --help       show this message
    -o FILE          output file
    -e               run by interpreter
    -d, --debug      show debug info
    -t, --target     target platform ( nes, emu )
    -O LEVEL         optimize level (0-2)
`

func main() {
	os.Exit(run())
}

func run() int {
	// <command> が先頭に来る
	args := os.Args[1:]
	if len(args) == 0 {
		fmt.Print(usage)
		return 0
	}
	com := args[0]
	switch com {
	case "fmt":
		return runFmt(args[1:])
	case "version", "--version", "-v":
		return runVersion()
	case "check":
		return runCheck(args[1:])
	}
	fs := flag.NewFlagSet("fcc", flag.ExitOnError)
	fs.Usage = func() { fmt.Print(usage) }
	out := fs.String("o", "", "output file")
	runFlag := fs.Bool("e", false, "run by interpreter")
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
		OptimizeLevel: optimizeLevel(*optLevel),
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
		printErrors(err)
		return 1
	}
	printWarnings(res.Warnings)
	if *debugFlag {
		for _, line := range res.Frames {
			fmt.Fprintln(os.Stderr, line)
		}
	}
	if *debugFlag && len(res.FarCalls) > 0 {
		// far call (別バンクへの呼び出し) の一覧: 熱い経路が far になっていないかの確認用 (doc/v2_farcall.md §4)
		fmt.Fprintf(os.Stderr, "far calls: %d\n", len(res.FarCalls))
		for _, f := range res.FarCalls {
			fmt.Fprintf(os.Stderr, "  %s: %s -> %s\n", f.Pos, f.Caller, f.Callee)
		}
	}
	return res.ExitCode
}

// optimizeLevel は -O の値を driver の表現に (0 は「未指定」の意味なので、-O 0 は -1 で渡す)。
func optimizeLevel(o int) int {
	if o == 0 {
		return -1
	}
	return o
}
