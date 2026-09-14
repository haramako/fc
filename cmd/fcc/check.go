package main

// fcc check: ファイルを生成せずにコンパイルし、エラーと警告を報告する。

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/haramako/fc/pkg/fc"
)

const checkUsage = `Usage: fcc check [-t target] <src.fc> ...
    -t, --target     target platform ( nes, emu )
`

func runCheck(args []string) int {
	fs := flag.NewFlagSet("fcc check", flag.ExitOnError)
	fs.Usage = func() { fmt.Print(checkUsage) }
	target := fs.String("t", "", "target platform")
	fs.StringVar(target, "target", "", "target platform")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if fs.NArg() == 0 {
		fmt.Print(checkUsage)
		return 0
	}
	compiler, err := fc.New()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer compiler.Close()

	rc := 0
	for _, src := range fs.Args() {
		ws, err := compiler.Check(src, fc.CheckOptions{Target: *target})
		if err != nil {
			var ce *fc.Error
			if errors.As(err, &ce) {
				fmt.Printf("%s: error: %s\n", ce.Pos, ce.Msg)
			} else {
				fmt.Println(err)
			}
			rc = 1
			continue
		}
		printWarnings(ws)
	}
	return rc
}

// printWarnings は警告を `file:line:col: warning: msg` で標準エラーに出す。
func printWarnings(ws []fc.Warning) {
	for _, w := range ws {
		fmt.Fprintf(os.Stderr, "%s: warning: %s\n", w.Pos, w.Msg)
	}
}
