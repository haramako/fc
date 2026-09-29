package main

// fcc migrate: 古い版のソースを最新の版 (fc 4) に書き換える。
//
//	fcc migrate [-t target] [-D module.NAME=value] [-l] [-w] [-d] <file.fc> ...
//	  (フラグなし)  書き換えた結果を標準出力に書く
//	  -l           書き換わるファイル名を列挙する
//	  -w           ファイルを上書きする
//	  -d           差分を表示する
//	  -t / -D      fc 3 → 4 で各ファイルを入口にコンパイルするときのターゲットと @(build) の上書き (fcc check と同じ)
//
// fc 2 → 3 は構文の書き換え (internal/migrate)、fc 3 → 4 は型を見る意味の書き換え (sema の Rewrite。Agent/wiki/plans/v4-plan.md §0) で、
// 渡したファイルをまとめて書き換える (ほかのファイルが use するモジュールも渡す)。最新の版のソースはそのまま (何度かけても
// 同じ)。入力が CRLF なら出力も CRLF にする。

import (
	"bytes"
	"flag"
	"fmt"
	"os"

	"github.com/haramako/fc/pkg/fc"
)

const migrateUsage = `Usage: fcc migrate [-t target] [-D module.NAME=value] [-l] [-w] [-d] <file.fc> ...
    -t    target platform used to compile the sources (emu (default) / nes)
    -D    override a @(build) const: module.NAME=value (repeatable)
    -l    list files that would be rewritten
    -w    write result to (source) file instead of stdout
    -d    display diffs instead of rewriting files
`

func runMigrate(args []string) int {
	fs := flag.NewFlagSet("fcc migrate", flag.ExitOnError)
	fs.Usage = func() { fmt.Print(migrateUsage) }
	target := fs.String("t", "", "target platform")
	fs.StringVar(target, "target", "", "target platform")
	var defines stringList
	fs.Var(&defines, "D", "override a @(build) const: module.NAME=value (repeatable)")
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
	compiler, err := fc.New()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer compiler.Close()
	out, err := compiler.Migrate(fs.Args(), fc.MigrateOptions{Target: *target, Defines: defines})
	if err != nil {
		printErrors(err)
		return 1
	}
	rc := 0
	for _, path := range fs.Args() {
		if err := emitMigrated(path, out[path], *list, *write, *diff); err != nil {
			fmt.Fprintln(os.Stderr, err)
			rc = 1
		}
	}
	return rc
}

// emitMigrated は path の書き換えの結果 res (LF) を、フラグに従って出す。
func emitMigrated(path string, res []byte, list, write, diff bool) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if bytes.Contains(src, []byte("\r\n")) {
		res = bytes.ReplaceAll(res, []byte("\n"), []byte("\r\n"))
	}
	changed := !bytes.Equal(src, res)
	switch {
	case list:
		if changed {
			fmt.Println(path)
		}
	case diff:
		if changed {
			fmt.Printf("--- %s\n+++ %s (migrated)\n", path, path)
			fmt.Print(lineDiff(string(src), string(res)))
		}
	case write:
		if changed {
			return os.WriteFile(path, res, 0o666)
		}
	default:
		os.Stdout.Write(res)
	}
	return nil
}
