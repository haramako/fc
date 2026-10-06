package main

// fcc size: ld65 の --dbgfile から関数ごとのコードサイズを表示する (自前で ld65 を呼ぶプロジェクト向け。
// fcc がリンクするなら `fcc build --size-report`)。

import (
	"fmt"
	"os"

	"github.com/haramako/fc/pkg/fc"
)

type sizeCmd struct {
	N    int    `short:"n" default:"40" help:"Show the N largest functions (default ${default}, 0 for all)."`
	Cfg  string `placeholder:"FILE" help:"The linker config used for the link: also show the used / free bytes of each ROM area (bank)."`
	HTML string `name:"html" placeholder:"FILE" help:"Write the same information as an HTML page."`
	Dbg  string `arg:"" name:"file.dbg" help:"The debug file written by ld65 --dbgfile."`
}

func (c *sizeCmd) run() int {
	if c.HTML != "" {
		if err := fc.SizeHTML(c.Dbg, c.Cfg, c.HTML); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0
	}
	lines, err := fc.SizeReport(c.Dbg, c.Cfg, c.N)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	for _, l := range lines {
		fmt.Println(l)
	}
	return 0
}
