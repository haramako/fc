package main

// fcc size: ld65 の --dbgfile から関数ごとのコードサイズを表示する (自前で ld65 を呼ぶプロジェクト向け。
// fcc がリンクするなら `fcc build --size-report`)。

import (
	"flag"
	"fmt"
	"os"

	"github.com/haramako/fc/pkg/fc"
)

const sizeUsage = `Usage: fcc size [-n N] <file.dbg>
    -n N             show the N largest functions (default 40, 0 for all)
Reads the debug file written by ld65 --dbgfile and prints the size of each segment and function.
`

func runSize(args []string) int {
	fs := flag.NewFlagSet("fcc size", flag.ExitOnError)
	fs.Usage = func() { fmt.Print(sizeUsage) }
	n := fs.Int("n", 40, "number of functions")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if fs.NArg() != 1 {
		fmt.Print(sizeUsage)
		return 0
	}
	lines, err := fc.SizeReport(fs.Arg(0), *n)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	for _, l := range lines {
		fmt.Println(l)
	}
	return 0
}
