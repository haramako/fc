// fcc は FC コンパイラの CLI。
//
//	Usage: fcc <command> [options] <src.fc> ...
//	  command: build(b) / compile(c) / run / fmt / migrate / check
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/haramako/fc/pkg/fc"
)

const usage = `NES Compiler
Usage: fcc <command> [options] <src.fc> ...
Commands:
    build, b         build ROM / binary
    compile, c       compile to object files only
    run              build and run by emulator
    fmt              format source files (see fcc fmt -h)
    migrate          rewrite fc 2 sources as fc 3 (see fcc migrate -h)
    check            compile without producing files and report errors / warnings
    size             show code size per function from an ld65 --dbgfile (see fcc size -h)
    watch            rebuild whenever a source file changes (see fcc watch -h)
    version          show version
Options:
    -h, --help       show this message
    -o FILE          output file
    -e               run by interpreter
    -d, --debug      show debug info (frames, far calls)
    -g               emit debug info for Mesen (.dbg with fc source lines, .mlb labels next to the ROM)
    --size-report    show code size per segment / function (needs linking)
    -t, --target     target platform ( nes, emu )
    -O LEVEL         optimize level (0-2)
    -D MOD.NAME=VAL  override a @(build) const (repeatable; applied after fc.toml [define.MOD])
`

func main() {
	os.Exit(run())
}

func run() int {
	posDir = ""
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
	case "migrate":
		return runMigrate(args[1:])
	case "version", "--version", "-v":
		return runVersion()
	case "check":
		return runCheck(args[1:])
	case "size":
		return runSize(args[1:])
	case "watch":
		return runWatch(args[1:])
	}
	fs := flag.NewFlagSet("fcc", flag.ExitOnError)
	fs.Usage = func() { fmt.Print(usage) }
	out := fs.String("o", "", "output file")
	runFlag := fs.Bool("e", false, "run by interpreter")
	debugFlag := fs.Bool("d", false, "show debug info")
	fs.BoolVar(debugFlag, "debug", false, "show debug info")
	gFlag := fs.Bool("g", false, "emit debug info for Mesen")
	sizeFlag := fs.Bool("size-report", false, "show code size per segment / function")
	target := fs.String("t", "", "target platform ( nes, emu )")
	fs.StringVar(target, "target", "", "target platform ( nes, emu )")
	optLevel := fs.Int("O", 2, "optimize level (0-2)")
	var defines stringList
	fs.Var(&defines, "D", "override a @(build) const: module.NAME=value (repeatable)")
	if err := fs.Parse(args[1:]); err != nil {
		return 1
	}
	rest := fs.Args()

	opt := fc.Options{
		Target:        *target,
		Out:           *out,
		Run:           *runFlag,
		OptimizeLevel: optimizeLevel(*optLevel),
		Debug:         *gFlag,
		SizeReport:    *sizeFlag,
		Defines:       defines,
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

	src := rest[0]
	opt.Dir, posDir = splitSrc(src)
	_, file := splitSrc(src)
	res, err := compiler.Build(context.Background(), file, opt)
	if err != nil {
		printErrors(err)
		return 1
	}
	printWarnings(res.Warnings)
	if *debugFlag {
		for _, line := range res.Frames {
			fmt.Fprintln(os.Stderr, line)
		}
		if len(res.Defines) > 0 {
			// @(build) の const の上書き (値と出所)
			fmt.Fprintf(os.Stderr, "defines: %d\n", len(res.Defines))
			for _, d := range res.Defines {
				used := ""
				if !d.Used {
					used = " (not used)"
				}
				fmt.Fprintf(os.Stderr, "  %s = %s (%s)%s\n", d.Key, d.Value, d.Source, used)
			}
		}
	}
	for _, line := range res.SizeReport {
		fmt.Println(line)
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

// stringList は繰り返せる文字列のフラグ (-D)。
type stringList []string

func (s *stringList) String() string     { return strings.Join(*s, ",") }
func (s *stringList) Set(v string) error { *s = append(*s, v); return nil }

// optimizeLevel は -O の値を driver の表現に (0 は「未指定」の意味なので、-O 0 は -1 で渡す)。
func optimizeLevel(o int) int {
	if o == 0 {
		return -1
	}
	return o
}
