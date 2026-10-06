package main

// fcc doc: モジュールのドキュメント (public の宣言と、その直前のコメント) を出す。--md は利用者向けのサイトのページ (docs/reference/std)。

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/haramako/fc/pkg/fc"
)

type docCmd struct {
	Target string `short:"t" placeholder:"nes|emu" complete:"nes,emu" help:"Only the modules of the target."`
	MD     string `name:"md" placeholder:"DIR" help:"Write the pages of the standard library for the documentation site (Markdown) into DIR."`
	Arg    string `arg:"" optional:"" name:"name" help:"module (vram, nes/vram), module.name (vram.put), or file.fc[:name] of your own. None: list the standard library modules."`
}

// sourceBase はサイトのページから標準ライブラリのソースへのリンクの頭。
const sourceBase = "https://github.com/haramako/fc/blob/main/"

func (c *docCmd) run() int {
	arg := c.Arg

	// 自分のファイル (file.fc か file.fc:name)
	if file, name, _ := strings.Cut(arg, ":"); strings.HasSuffix(file, ".fc") {
		m, err := fc.DocFile(file)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return printDoc([]*fc.DocModule{m}, name)
	}

	compiler, err := fc.New()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer compiler.Close()
	mods, err := compiler.StdDocs()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if c.MD != "" {
		return writeDocSite(mods, c.MD)
	}
	if c.Target != "" {
		var ms []*fc.DocModule
		for _, m := range mods {
			if m.Target == "" || m.Target == c.Target {
				ms = append(ms, m)
			}
		}
		mods = ms
	}
	if arg == "" {
		for _, m := range mods {
			name := m.Name
			if m.Target != "" {
				name = m.Target + "/" + m.Name
			}
			fmt.Printf("%-14s %s\n", name, m.Summary())
		}
		return 0
	}
	// module / target/module / module.name
	modName, name, _ := strings.Cut(arg, ".")
	tgt, modName, hasTarget := strings.Cut(modName, "/")
	if !hasTarget {
		modName, tgt = tgt, ""
	}
	var found []*fc.DocModule
	for _, m := range mods {
		if m.Name == modName && (tgt == "" || m.Target == tgt) {
			found = append(found, m)
		}
	}
	if len(found) == 0 {
		fmt.Fprintf(os.Stderr, "fcc doc: no module %s in the standard library (fcc doc lists them)\n", arg)
		return 1
	}
	return printDoc(found, name)
}

// printDoc は mods のドキュメント (name があればその宣言だけ) を出す。
func printDoc(mods []*fc.DocModule, name string) int {
	for i, m := range mods {
		if i > 0 {
			fmt.Println()
		}
		if name == "" {
			fmt.Print(m.Text())
			continue
		}
		it := m.Find(name)
		if it == nil {
			fmt.Fprintf(os.Stderr, "fcc doc: %s has no public %s\n", m.Path, name)
			return 1
		}
		fmt.Print(it.Text())
	}
	return 0
}

// writeDocSite は dir に一覧 (index.md) とモジュールごとのページ (<module>.md、nes/<module>.md、emu/<module>.md) を書く。
func writeDocSite(mods []*fc.DocModule, dir string) int {
	page := func(m *fc.DocModule) string {
		if m.Target == "" {
			return m.Name
		}
		return m.Target + "/" + m.Name
	}
	if err := os.RemoveAll(dir); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	write := func(rel, text string) error {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o777); err != nil {
			return err
		}
		return os.WriteFile(p, []byte(text), 0o666)
	}
	for _, m := range mods {
		if err := write(page(m)+".md", m.Markdown(sourceBase+m.Path)); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}
	index := fc.DocIndex(mods, func(m *fc.DocModule) string { return "./" + page(m) })
	if err := write("index.md", index); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Printf("fcc doc: wrote %d modules into %s\n", len(mods), dir)
	return 0
}
