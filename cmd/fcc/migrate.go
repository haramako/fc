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
	"fmt"
	"os"

	"github.com/haramako/fc/pkg/fc"
)

type migrateCmd struct {
	targetFlags
	rewriteFlags
	Files []string `arg:"" name:"file" help:"Source files (pass the modules used by other files too: they are rewritten together)."`
}

func (c *migrateCmd) run() int {
	compiler, err := fc.New()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer compiler.Close()
	out, err := compiler.Migrate(c.Files, fc.MigrateOptions{Target: c.Target, Defines: c.Define})
	if err != nil {
		printErrors(err)
		return 1
	}
	rc := 0
	for _, path := range c.Files {
		src, err := os.ReadFile(path)
		if err == nil {
			err = c.emit(path, src, out[path], "migrated")
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			rc = 1
		}
	}
	return rc
}
