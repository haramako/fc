package main

// fcc check: ファイルを生成せずにコンパイルし、エラーと警告を報告する。

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/haramako/fc/pkg/fc"
)

type checkCmd struct {
	targetFlags
	JSON bool     `name:"json" help:"Print diagnostics as JSON lines ({\"file\",\"line\",\"col\",\"severity\",\"message\"}) for editors."`
	Srcs []string `arg:"" name:"src" help:"Source files (each one is compiled as a program)."`
}

func (c *checkCmd) run() int {
	compiler, err := fc.New()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer compiler.Close()

	rc := 0
	for _, src := range c.Srcs {
		dir, file := splitSrc(src)
		posDir = dir
		ws, err := compiler.Check(file, fc.CheckOptions{Target: c.Target, Dir: dir, Defines: c.Define})
		if c.JSON {
			if err != nil {
				rc = 1
				es := fc.Errors(err)
				if len(es) == 0 {
					printJSONDiag(fc.Position{Filename: file}, "error", err.Error())
				}
				for _, e := range es {
					printJSONDiag(e.Pos, "error", e.Msg)
				}
			}
			for _, w := range ws {
				printJSONDiag(w.Pos, "warning", w.Msg)
			}
			continue
		}
		if err != nil {
			printErrors(err)
			rc = 1
			continue
		}
		printWarnings(ws)
	}
	return rc
}

// printJSONDiag は診断 1 件を JSON 1 行で出す (エディタ連携。tools/vscode-fc)。
func printJSONDiag(pos fc.Position, severity, msg string) {
	pos = displayPos(pos)
	b, _ := json.Marshal(map[string]any{"file": pos.Filename, "line": pos.Line, "col": pos.Col, "severity": severity, "message": msg})
	fmt.Println(string(b))
}

// printErrors はコンパイルエラーを `file:line:col: error: msg` で全部出す (それ以外のエラーはそのまま)。
func printErrors(err error) {
	es := fc.Errors(err)
	if len(es) == 0 {
		fmt.Println(err)
		return
	}
	for _, e := range es {
		fmt.Printf("%s: error: %s\n", displayPos(e.Pos), e.Msg)
	}
	if len(es) > 1 {
		fmt.Printf("%d errors\n", len(es))
	}
}

// printWarnings は警告を `file:line:col: warning: msg` で標準エラーに出す。
func printWarnings(ws []fc.Warning) {
	for _, w := range ws {
		fmt.Fprintf(os.Stderr, "%s: warning: %s\n", displayPos(w.Pos), w.Msg)
	}
}
