package main

// fcc lib: fc.toml の [lib.*] のライブラリ (doc/v4_stdlib.md §9)。
//
//	fcc lib fetch              fc.lock のとおりに揃える (無いものは取ってきて fc.lock を書く。fcc build も自動でする)
//	fcc lib update [name...]   git のライブラリを rev の今のコミットに進めて fc.lock を書き直す (名前が無ければ全部)
//	fcc lib list               ライブラリと固定したコミットを並べる

import (
	"flag"
	"fmt"

	"github.com/haramako/fc/pkg/fc"
)

const libUsage = `Usage: fcc lib <fetch | update [name...] | list> [-C dir]
    fetch     fetch the libraries of fc.toml [lib.*] as fc.lock pins them (fcc build does this too)
    update    move git libraries to the current commit of their rev and rewrite fc.lock
    list      list the libraries and the pinned commits
    -C DIR    look for fc.toml from DIR (default: the current directory)
`

func runLib(args []string) int {
	if len(args) == 0 {
		fmt.Print(libUsage)
		return 0
	}
	fs := flag.NewFlagSet("fcc lib", flag.ExitOnError)
	fs.Usage = func() { fmt.Print(libUsage) }
	dir := fs.String("C", ".", "directory to look for fc.toml from")
	if err := fs.Parse(args[1:]); err != nil {
		return 1
	}
	var infos []fc.LibInfo
	var err error
	switch args[0] {
	case "fetch":
		infos, err = fc.LibFetch(*dir)
	case "update":
		infos, err = fc.LibUpdate(*dir, fs.Args())
	case "list":
		infos, err = fc.LibList(*dir)
	default:
		fmt.Print(libUsage)
		return 1
	}
	if err != nil {
		printErrors(err)
		return 1
	}
	if len(infos) == 0 {
		fmt.Println("no libraries in fc.toml ([lib.NAME] with path = \"folder\" or git = \"url\")")
		return 0
	}
	for _, l := range infos {
		switch {
		case l.Commit != "":
			fmt.Printf("%-16s %s @ %s (rev %q)\n", l.Name, l.Source, l.Commit[:min(12, len(l.Commit))], l.Rev)
		case l.Root == "" && args[0] == "list":
			fmt.Printf("%-16s %s (rev %q, not fetched)\n", l.Name, l.Source, l.Rev)
		default:
			fmt.Printf("%-16s %s\n", l.Name, l.Source)
		}
	}
	return 0
}
