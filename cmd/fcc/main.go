// fcc は FCコンパイラの Go 実装 (bin/fcc 互換 CLI)。
//
//	Usage: fcc <command> [options] <src.fc> ...
//	  command: build(b) / compile(c) / run
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	fcdata "github.com/haramako/fc"
	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/driver"
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
	asmFlag := fs.Bool("S", false, "output asm file")
	debugFlag := fs.Bool("d", false, "show debug info")
	fs.BoolVar(debugFlag, "debug", *debugFlag, "show debug info")
	target := fs.String("t", "", "target platform ( nes, emu )")
	fs.StringVar(target, "target", "", "target platform ( nes, emu )")
	optLevel := fs.Int("O", 2, "optimize level (0-2)")
	if err := fs.Parse(args[1:]); err != nil {
		return 1
	}
	rest := fs.Args()

	opt := &driver.BuildOptions{
		Target:        *target,
		Out:           *out,
		Run:           *runFlag,
		Asm:           *asmFlag,
		DebugInfo:     *debugFlag,
		OptimizeLevel: *optLevel,
	}

	fcHome, cleanup, err := resolveFCHome()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if cleanup != nil {
		defer cleanup()
	}
	compiler := driver.NewCompiler(fcHome)

	buildOne := func(src string) int {
		code, err := compiler.Build(src, opt)
		if err != nil {
			if ce, ok := err.(*diag.Error); ok {
				fmt.Printf("%s: error: %s\n", ce.Pos, ce.Msg)
				return 1
			}
			fmt.Println(err)
			return 1
		}
		return code
	}

	switch com {
	case "run":
		if len(rest) == 0 {
			fmt.Print(usage)
			return 0
		}
		opt.Run = true
		return buildOne(rest[0])
	case "build", "b":
		if len(rest) == 0 {
			fmt.Print(usage)
			return 0
		}
		return buildOne(rest[0])
	case "compile", "c":
		if len(rest) == 0 {
			fmt.Print(usage)
			return 0
		}
		opt.CompileOnly = true
		return buildOne(rest[0])
	default:
		fmt.Print(usage)
		return 0
	}
}

// resolveFCHome は fclib/ share/ を含むディレクトリを探す。
// 1. 環境変数 FC_HOME
// 2. 実行ファイルの場所から上方向に探索
// 3. カレントディレクトリから上方向に探索
// 4. embed.FS を一時ディレクトリに展開
func resolveFCHome() (string, func(), error) {
	if h := os.Getenv("FC_HOME"); h != "" {
		return h, nil, nil
	}
	isHome := func(dir string) bool {
		fi1, err1 := os.Stat(filepath.Join(dir, "fclib"))
		fi2, err2 := os.Stat(filepath.Join(dir, "share"))
		return err1 == nil && err2 == nil && fi1.IsDir() && fi2.IsDir()
	}
	var starts []string
	if exe, err := os.Executable(); err == nil {
		starts = append(starts, filepath.Dir(exe))
	}
	if wd, err := os.Getwd(); err == nil {
		starts = append(starts, wd)
	}
	for _, start := range starts {
		dir := start
		for {
			if isHome(dir) {
				return dir, nil, nil
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	// embed.FS を展開する
	tmp, err := os.MkdirTemp("", "fc-home-")
	if err != nil {
		return "", nil, err
	}
	home, err := fcdata.Materialize(tmp)
	if err != nil {
		os.RemoveAll(tmp)
		return "", nil, err
	}
	return home, func() { os.RemoveAll(tmp) }, nil
}
