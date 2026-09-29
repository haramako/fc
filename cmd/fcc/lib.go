package main

// fcc lib: fc.toml の [lib.*] のライブラリ (doc/v4_stdlib.md §9)。
//
//	fcc lib fetch              fc.lock のとおりに揃える (無いものは取ってきて fc.lock を書く。fcc build も自動でする)
//	fcc lib update [name...]   git のライブラリを rev の今のコミットに進めて fc.lock を書き直す (名前が無ければ全部)
//	fcc lib list               ライブラリと固定したコミットを並べる
//	fcc lib add name src [-rev r] [-dir d]   fc.toml に [lib.name] を書き足して取ってくる (src はフォルダか git の URL)

import (
	"flag"
	"fmt"
	"strings"

	"github.com/haramako/fc/pkg/fc"
)

const libUsage = `Usage: fcc lib <fetch | update [name...] | list | add name src> [-C dir]
    fetch     fetch the libraries of fc.toml [lib.*] as fc.lock pins them (fcc build does this too)
    update    move git libraries to the current commit of their rev and rewrite fc.lock
    list      list the libraries and the pinned commits
    add       add [lib.name] to fc.toml (src: a folder, or a git URL with -rev REV / -dir DIR) and fetch it
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
	rev := fs.String("rev", "", "git: tag / branch / commit (add)")
	sub := fs.String("dir", "", "git: the folder of the modules in the repository (add)")
	rest := args[1:]
	var pos []string
	if args[0] == "add" {
		// fcc lib add name src [-rev r] (名前と場所を先に書く)
		for len(rest) > 0 && !strings.HasPrefix(rest[0], "-") && len(pos) < 2 {
			pos, rest = append(pos, rest[0]), rest[1:]
		}
	}
	if err := fs.Parse(rest); err != nil {
		return 1
	}
	pos = append(pos, fs.Args()...)
	var infos []fc.LibInfo
	var err error
	switch args[0] {
	case "fetch":
		infos, err = fc.LibFetch(*dir)
	case "update":
		infos, err = fc.LibUpdate(*dir, pos)
	case "list":
		infos, err = fc.LibList(*dir)
	case "add":
		if len(pos) != 2 {
			fmt.Print(libUsage)
			return 1
		}
		infos, err = fc.LibAdd(*dir, pos[0], pos[1], *rev, *sub)
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
