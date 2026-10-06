// fcc は FC コンパイラの CLI。
//
//	Usage: fcc <command> [flags] <src.fc> ...
//	  command: build(b) / compile(c) / run / check / test / fmt / doc / lib / watch / size / migrate / version
//
// コマンドラインは kong (github.com/alecthomas/kong) で読む: コマンドは cli の構造体のフィールド、オプションはタグ。
// GNU の getopt の形 (`-O1`、`--offline`。長い名前は `--`) で、オプションはソースの後ろにも書ける。
package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/alecthomas/kong"

	"github.com/haramako/fc/pkg/fc"
)

// cli はコマンドの木。選ばれたコマンドの構造体の run を呼ぶ。
type cli struct {
	Version versionFlag `help:"Show version and exit." short:"v"`

	Build      buildCmd      `cmd:"" aliases:"b" help:"Build a ROM / binary."`
	Compile    compileCmd    `cmd:"" aliases:"c" help:"Compile to object files only (no link)."`
	Run        runCmd        `cmd:"" help:"Build and run (emu: the built-in emulator; nes: console output on the built-in NES runner)."`
	Check      checkCmd      `cmd:"" help:"Compile without producing files and report errors / warnings."`
	Test       testCmd       `cmd:"" help:"Run the @(test) functions of modules."`
	Fmt        fmtCmd        `cmd:"" help:"Format source files."`
	Doc        docCmd        `cmd:"" help:"Show the public declarations of a module and their comments."`
	Lib        libCmd        `cmd:"" help:"Fetch / update / list the libraries of fc.toml [lib.*]."`
	Watch      watchCmd      `cmd:"" help:"Rebuild whenever a source file under the source directory (or fclib) changes. Ctrl-C to stop."`
	Size       sizeCmd       `cmd:"" help:"Show code size per segment and function from an ld65 --dbgfile."`
	Migrate    migrateCmd    `cmd:"" help:"Rewrite older sources as the latest fc."`
	VersionCmd versionCmd    `cmd:"" name:"version" help:"Show version and the ca65 / ld65 in use."`
	Completion completionCmd `cmd:"" help:"Print a shell completion script (zsh / bash)."`
}

// runner は選ばれたコマンド。終了コードを返す (run / build のプログラムの終了コードもそのまま)。
type runner interface{ run() int }

// exitPanic は kong が --help などで終わるときの終了コード (run() で recover して返す。os.Exit はテストの邪魔)。
type exitPanic int

func main() {
	os.Exit(run())
}

func run() (code int) {
	posDir = ""
	args := os.Args[1:]
	if len(args) == 0 {
		args = []string{"--help"}
	}
	var c cli
	parser, err := kong.New(&c,
		kong.Name("fcc"),
		kong.Description("NES Compiler"),
		kong.Writers(os.Stdout, os.Stderr),
		kong.Exit(func(code int) { panic(exitPanic(code)) }),
		kong.ConfigureHelp(kong.HelpOptions{Compact: true}),
	)
	if err != nil {
		panic(err) // タグの書き間違い
	}
	defer func() {
		if r := recover(); r != nil {
			e, ok := r.(exitPanic)
			if !ok {
				panic(r)
			}
			code = int(e)
		}
	}()
	if args[0] == "__complete" { // 補完のスクリプトから (completion.go)
		printCompletions(parser.Model.Node, args[1:])
		return 0
	}
	if err := checkSingleDash(parser, args); err != nil {
		fmt.Fprintf(os.Stderr, "fcc: error: %v\n", err)
		return 2
	}
	ctx, err := parser.Parse(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fcc: error: %v (see %s --help)\n", err, commandPath(err))
		return 2
	}
	return ctx.Selected().Target.Addr().Interface().(runner).run()
}

// commandPath はエラーの起きたコマンド (`fcc lib add`。コマンドの前なら `fcc`)。
func commandPath(err error) string {
	if pe, ok := err.(*kong.ParseError); ok && pe.Context != nil {
		path := "fcc"
		for n := pe.Context.Selected(); n != nil && n.Parent != nil; n = n.Parent {
			path = strings.Replace(path, "fcc", "fcc "+n.Name, 1)
		}
		return path
	}
	return "fcc"
}

// checkSingleDash は `-offline` のように長い名前に `-` を 1 つしか付けないものをエラーにする。getopt の形では短いオプションの
// 並び (`-o ffline`) と読めてしまい、黙って別の意味になる。
func checkSingleDash(parser *kong.Kong, args []string) error {
	long := map[string]bool{}
	var walk func(n *kong.Node)
	walk = func(n *kong.Node) {
		for _, f := range n.Flags {
			if len(f.Name) > 1 {
				long[f.Name] = true
			}
		}
		for _, ch := range n.Children {
			walk(ch)
		}
	}
	walk(parser.Model.Node)
	for _, a := range args {
		if a == "--" {
			break
		}
		if name, _, _ := strings.Cut(strings.TrimPrefix(a, "-"), "="); len(a) > 2 && a[0] == '-' && a[1] != '-' && long[name] {
			return fmt.Errorf("unknown flag %s (long flags take two dashes: -%s)", a, a)
		}
	}
	return nil
}

// targetFlags はターゲットと @(build) の定数の上書き (コンパイルするコマンドに共通)。
type targetFlags struct {
	Target string   `short:"t" placeholder:"nes|emu" complete:"nes,emu" help:"Target platform (default: nes if fc.toml has [target], otherwise emu)."`
	Define []string `short:"D" sep:"none" placeholder:"MOD.NAME=VAL" help:"Override a @(build) const (repeatable; applied after fc.toml [define.MOD])."`
}

// buildFlags は build / compile / run / watch に共通のオプション。
type buildFlags struct {
	targetFlags
	Out     string `short:"o" placeholder:"FILE" help:"Output file."`
	Opt     int    `short:"O" default:"2" placeholder:"LEVEL" complete:"0,1,2" help:"Optimize level (0-2, default ${default})."`
	G       bool   `short:"g" name:"debug-info" help:"Emit debug info for Mesen (.dbg with fc source lines, .mlb labels next to the ROM)."`
	Offline bool   `help:"Do not fetch git libraries of fc.toml [lib.*] (use the cache only)."`
}

func (f *buildFlags) options() fc.Options {
	return fc.Options{
		Target:        f.Target,
		Out:           f.Out,
		OptimizeLevel: optimizeLevel(f.Opt),
		Debug:         f.G,
		Defines:       f.Define,
		Offline:       f.Offline,
	}
}

// buildArgs は build / compile / run のオプションとソース。
type buildArgs struct {
	buildFlags
	Debug      bool   `short:"d" help:"Show debug info (frames, far calls, libraries, defines)."`
	SizeReport bool   `help:"Show code size per segment / function, bank usage and calls between modules (needs linking)."`
	SizeHTML   string `name:"size-html" placeholder:"FILE" help:"Write the same information as --size-report as an HTML page."`
	Src        string `arg:"" help:"The main source file."`
}

type buildCmd struct{ buildArgs }
type compileCmd struct{ buildArgs }
type runCmd struct{ buildArgs }

func (c *buildCmd) run() int { return c.build(c.options()) }

func (c *compileCmd) run() int {
	opt := c.options()
	opt.CompileOnly = true
	return c.build(opt)
}

func (c *runCmd) run() int {
	opt := c.options()
	opt.Run = true
	return c.build(opt)
}

func (c *buildArgs) build(opt fc.Options) int {
	opt.SizeReport = c.SizeReport
	opt.SizeHTML = c.SizeHTML
	compiler, err := fc.New()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer compiler.Close()

	dir, file := splitSrc(c.Src)
	opt.Dir, posDir = dir, dir
	res, err := compiler.Build(context.Background(), file, opt)
	if err != nil {
		printErrors(err)
		return 1
	}
	printWarnings(res.Warnings)
	if c.Debug {
		for _, line := range res.Frames {
			fmt.Fprintln(os.Stderr, line)
		}
		if len(res.Libs) > 0 {
			// fc.toml の [lib.*] と、そこから使ったモジュール (fclib を置き換えたもの)
			fmt.Fprintln(os.Stderr, "libs:")
			for _, line := range res.Libs {
				fmt.Fprintln(os.Stderr, line)
			}
		}
		if len(res.Defines) > 0 {
			// @(build) の const の上書き (値と出所)
			fmt.Fprintf(os.Stderr, "defines: %d\n", len(res.Defines))
			for _, d := range res.Defines {
				used := ""
				if !d.Used {
					used = " (not used)"
				}
				fmt.Fprintf(os.Stderr, "  %s = %s (%s)%s\n", d.Key, d.Value, d.Source, used)
			}
		}
	}
	for _, line := range res.SizeReport {
		fmt.Println(line)
	}
	if c.Debug && len(res.FarCalls) > 0 {
		// far call (別バンクへの呼び出し) の一覧: 熱い経路が far になっていないかの確認用 (Agent/wiki/design/farcall.md §4)
		fmt.Fprintf(os.Stderr, "far calls: %d\n", len(res.FarCalls))
		for _, f := range res.FarCalls {
			fmt.Fprintf(os.Stderr, "  %s: %s -> %s\n", f.Pos, f.Caller, f.Callee)
		}
	}
	return res.ExitCode
}

// optimizeLevel は -O の値を driver の表現に (0 は「未指定」の意味なので、-O 0 は -1 で渡す)。
func optimizeLevel(o int) int {
	if o == 0 {
		return -1
	}
	return o
}
