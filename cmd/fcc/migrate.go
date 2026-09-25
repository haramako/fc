package main

// fcc migrate: fc 2 のソースを fc 3 に書き換える (internal/migrate)。
//
//	fcc migrate [-l] [-w] [-d] <file.fc> ...
//	  (フラグなし)  書き換えた結果を標準出力に書く
//	  -l           書き換わるファイル名を列挙する
//	  -w           ファイルを上書きする
//	  -d           差分を表示する
//
// fc 3 のソースはそのまま (何度かけても同じ)。入力が CRLF なら出力も CRLF にする。

import (
	"bytes"
	"flag"
	"fmt"
	"os"

	"github.com/haramako/fc/internal/migrate"
)

const migrateUsage = `Usage: fcc migrate [-l] [-w] [-d] <file.fc> ...
    -l    list files that would be rewritten
    -w    write result to (source) file instead of stdout
    -d    display diffs instead of rewriting files
`

func runMigrate(args []string) int {
	fs := flag.NewFlagSet("fcc migrate", flag.ExitOnError)
	fs.Usage = func() { fmt.Print(migrateUsage) }
	list := fs.Bool("l", false, "list files that would be rewritten")
	write := fs.Bool("w", false, "write result to (source) file instead of stdout")
	diff := fs.Bool("d", false, "display diffs instead of rewriting files")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if fs.NArg() == 0 {
		fmt.Print(migrateUsage)
		return 0
	}
	rc := 0
	for _, path := range fs.Args() {
		if err := migrateFile(path, *list, *write, *diff); err != nil {
			fmt.Fprintln(os.Stderr, err)
			rc = 1
		}
	}
	return rc
}

func migrateFile(path string, list, write, diff bool) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	crlf := bytes.Contains(src, []byte("\r\n"))
	out, err := migrate.Migrate(bytes.ReplaceAll(src, []byte("\r\n"), []byte("\n")), path)
	if err != nil {
		return err
	}
	if crlf {
		out = bytes.ReplaceAll(out, []byte("\n"), []byte("\r\n"))
	}
	changed := !bytes.Equal(src, out)
	switch {
	case list:
		if changed {
			fmt.Println(path)
		}
	case diff:
		if changed {
			fmt.Printf("--- %s (fc 2)\n+++ %s (fc 3)\n", path, path)
			fmt.Print(lineDiff(string(src), string(out)))
		}
	case write:
		if changed {
			return os.WriteFile(path, out, 0o666)
		}
	default:
		os.Stdout.Write(out)
	}
	return nil
}
