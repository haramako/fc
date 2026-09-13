package main

// fcc migrate: v1 ソースを文法 v2 に移行する (doc/v2_grammar.md §5)。
//
//	fcc migrate [-t target] [--lib DIR]... [--visibility minimal|preserve] [--textmap NAME=PATH]... [-w] [--force] <main.fc>...
//
// main から use で届く全モジュール (作業ディレクトリと --lib の下にあるもの) を書き換える。
// -w が無ければ書き換え対象を列挙するだけ。-w なら書き込んだ後に再コンパイルして
// asm が移行前と一致することを確かめ、一致しなければ元に戻す (--force で受け入れる)。

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/haramako/fc/pkg/fc"
)

const migrateUsage = `Usage: fcc migrate [options] <main.fc> ...
    -t, --target       target platform ( nes, emu )
    --lib DIR          library directory (migrated with visibility=preserve; repeatable)
    --visibility V     minimal (default: public only what other modules use) | preserve
    --textmap N=PATH   replace include("macro.rb") with 'const N = textmap("PATH");' (repeatable)
    -w                 write files (otherwise just list them)
    --force            keep the result even if the generated asm differs
`

type multiFlag []string

func (m *multiFlag) String() string     { return strings.Join(*m, ",") }
func (m *multiFlag) Set(s string) error { *m = append(*m, s); return nil }

func runMigrate(args []string) int {
	fs := flag.NewFlagSet("fcc migrate", flag.ExitOnError)
	fs.Usage = func() { fmt.Print(migrateUsage) }
	target := fs.String("t", "", "target platform")
	fs.StringVar(target, "target", "", "target platform")
	var libs, textmaps multiFlag
	fs.Var(&libs, "lib", "library directory")
	fs.Var(&textmaps, "textmap", "NAME=PATH")
	vis := fs.String("visibility", "minimal", "minimal | preserve")
	write := fs.Bool("w", false, "write files")
	force := fs.Bool("force", false, "keep result even if asm differs")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if fs.NArg() == 0 {
		fmt.Print(migrateUsage)
		return 0
	}
	opt := fc.MigrateOptions{Target: *target, Mains: fs.Args(), Libs: libs, Write: *write, Force: *force, Textmaps: map[string]string{}}
	switch *vis {
	case "minimal":
		opt.Visibility = fc.VisibilityMinimal
	case "preserve":
		opt.Visibility = fc.VisibilityPreserve
	default:
		fmt.Fprintf(os.Stderr, "unknown visibility %q\n", *vis)
		return 1
	}
	for _, tm := range textmaps {
		name, path, ok := strings.Cut(tm, "=")
		if !ok {
			fmt.Fprintf(os.Stderr, "--textmap %q: expected NAME=PATH\n", tm)
			return 1
		}
		opt.Textmaps[name] = path
	}

	compiler, err := fc.New()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer compiler.Close()

	res, err := compiler.Migrate(opt)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if res.Written {
		fmt.Printf("migrated %d files\n", len(res.Files))
		for _, f := range res.Files {
			fmt.Println(" ", f)
		}
		if len(res.Diffs) > 0 {
			fmt.Printf("asm changed in: %s\n", strings.Join(res.Diffs, ", "))
		}
	}
	return 0
}
