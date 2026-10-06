package main

// fcc lib: fc.toml の [lib.*] のライブラリ (Agent/wiki/plans/v4-stdlib.md §9)。
//
//	fcc lib fetch              fc.lock のとおりに揃える (無いものは取ってきて fc.lock を書く。fcc build も自動でする)
//	fcc lib update [name...]   git のライブラリを rev の今のコミットに進めて fc.lock を書き直す (名前が無ければ全部)
//	fcc lib list               ライブラリと固定したコミットを並べる
//	fcc lib add name src [--rev r] [--dir d]   fc.toml に [lib.name] を書き足して取ってくる (src はフォルダか git の URL)

import (
	"fmt"

	"github.com/haramako/fc/pkg/fc"
)

type libCmd struct {
	Dir string `short:"C" name:"directory" default:"." placeholder:"DIR" help:"Look for fc.toml from DIR."`

	Fetch  libFetchCmd  `cmd:"" help:"Fetch the libraries of fc.toml [lib.*] as fc.lock pins them (fcc build does this too)."`
	Update libUpdateCmd `cmd:"" help:"Move git libraries to the current commit of their rev and rewrite fc.lock."`
	List   libListCmd   `cmd:"" help:"List the libraries and the pinned commits."`
	Add    libAddCmd    `cmd:"" help:"Add [lib.NAME] to fc.toml and fetch it."`
}

// AfterApply は kong が解析の後に呼ぶ: サブコマンドから親の -C を見えるようにする。
func (c *libCmd) AfterApply() error {
	c.Fetch.lib, c.Update.lib, c.List.lib, c.Add.lib = c, c, c, c
	return nil
}

type libFetchCmd struct{ lib *libCmd }
type libUpdateCmd struct {
	lib   *libCmd
	Names []string `arg:"" optional:"" name:"name" help:"Libraries to update (default: all)."`
}
type libListCmd struct{ lib *libCmd }
type libAddCmd struct {
	lib  *libCmd
	Name string `arg:"" help:"The library name."`
	Src  string `arg:"" help:"A folder, or a git URL."`
	Rev  string `placeholder:"REV" help:"git: tag / branch / commit."`
	Sub  string `name:"dir" placeholder:"DIR" help:"git: the folder of the modules in the repository."`
}

func (c *libFetchCmd) run() int {
	infos, err := fc.LibFetch(c.lib.Dir)
	return printLibs(infos, err, false)
}

func (c *libUpdateCmd) run() int {
	infos, err := fc.LibUpdate(c.lib.Dir, c.Names)
	return printLibs(infos, err, false)
}

func (c *libListCmd) run() int {
	infos, err := fc.LibList(c.lib.Dir)
	return printLibs(infos, err, true)
}

func (c *libAddCmd) run() int {
	infos, err := fc.LibAdd(c.lib.Dir, c.Name, c.Src, c.Rev, c.Sub)
	return printLibs(infos, err, false)
}

// printLibs はライブラリの一覧を出す (list は、まだ取ってきていないものにそう書く)。
func printLibs(infos []fc.LibInfo, err error, list bool) int {
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
		case l.Root == "" && list:
			fmt.Printf("%-16s %s (rev %q, not fetched)\n", l.Name, l.Source, l.Rev)
		default:
			fmt.Printf("%-16s %s\n", l.Name, l.Source)
		}
	}
	return 0
}
