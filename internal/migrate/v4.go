package migrate

// fc 3 → fc 4 (doc/v4_plan.md §0)。fc 4 は意味を変えるので、書き換えは sema が fc 3 のソースをコンパイルしながら集める
// (sema.Rewrite。internal/sema/rewrite.go)。ここはそれをソースに当てて `#fc 4` にするだけ。プログラム単位の手順
// (fc 2 → 3 をメモリの上で済ませ、入口ごとにコンパイルして書き換えを集める) は driver の Compiler.Migrate。

import (
	"bytes"
	"fmt"

	"github.com/haramako/fc/internal/sema"
	"github.com/haramako/fc/internal/syntax"
)

// ToV4 は fc 3 のソース src (LF) に書き換え rewrites を当て、`#fc 4` にする。fc 4 のソースは rewrites が無ければそのまま返す。
// 書き換えた結果が fc 4 として解析できなければエラー (書き換えの不具合)。
func ToV4(src []byte, filename string, rewrites []sema.Rewrite) ([]byte, error) {
	f, err := syntax.Parse(src, filename)
	if err != nil {
		return nil, err
	}
	switch {
	case f.Version == syntax.Version4 && len(rewrites) == 0:
		return src, nil
	case f.Version != syntax.Version3:
		return nil, fmt.Errorf("%s: fc %d source cannot be rewritten as fc 4 directly (migrate it to fc 3 first)", filename, f.Version)
	}
	c := &Ctx{Src: src, File: f}
	for _, r := range rewrites {
		c.Replace(r.Start, r.End, r.Text)
	}
	if err := abiRules(c); err != nil {
		return nil, fmt.Errorf("%s: %v", filename, err)
	}
	runTestsRule(c)
	c.Replace(0, len(f.Pragma), fmt.Sprintf("#fc %d", syntax.Version4))
	out, err := apply(src, c.Edits)
	if err != nil {
		return nil, fmt.Errorf("%s: %v", filename, err)
	}
	g, err := syntax.Parse(out, filename)
	if err != nil {
		return nil, fmt.Errorf("%s: migrated source does not parse as fc 4 (migrate rewrite bug): %v", filename, err)
	}
	if g.Version != syntax.Version4 {
		return nil, fmt.Errorf("%s: migrated source is not fc 4", filename)
	}
	return out, nil
}

// abiRules は関数の呼び出し規約の書き換え (構文だけで決まる。doc/v4_plan.md §2): fc 4 は fastcall を廃止し、extern (本体の
// 無い関数) は規約の明示が必須。
//   - 本体のある関数の fastcall は消す (コンパイラが規約を決める。生成コードは変わるが意味は同じ)
//   - 規約を書かない extern は `abi: "stack"` を足す (fc 3 の extern の既定。asm はそのまま)
//   - fastcall の extern はエラー (引数の置き場所 FC_FASTCALL_REG が無くなるので、asm を abi: "frame" に直す必要がある)
func abiRules(c *Ctx) error {
	var err error
	syntax.Inspect(c.File, func(n syntax.Node) bool {
		fd, ok := n.(*syntax.FuncDecl)
		if !ok || err != nil {
			return err == nil
		}
		var fast *syntax.OptionEntry
		hasABI := false
		if fd.Options != nil {
			for _, e := range fd.Options.Entries {
				switch e.Key.Name {
				case "fastcall":
					fast = e
				case "abi":
					hasABI = true
				}
			}
		}
		switch {
		case fast != nil && fd.Body == nil:
			err = fmt.Errorf("%d:%d: %s is an assembler function with fastcall, which fc 4 removes: rewrite the assembler to take the arguments in its static frame and declare @(abi: \"frame\") (F_<sym>+k instead of FC_FASTCALL_REG+k)", fd.Keyword.Line, fd.Keyword.Col, fd.Name.Name)
		case fast != nil:
			removeEntry(c, fd.Options, fast)
		case fd.Body == nil && !hasABI:
			if fd.Options != nil && len(fd.Options.Entries) > 0 {
				c.Replace(fd.Options.Entries[0].Pos().Offset, fd.Options.Entries[0].Pos().Offset, `abi: "stack", `)
			} else if fd.Options != nil {
				c.Replace(fd.Options.Rparen.Offset, fd.Options.Rparen.Offset, `abi: "stack"`)
			} else {
				c.Replace(fd.Semi.Offset, fd.Semi.Offset, ` @(abi: "stack")`)
			}
		}
		return true
	})
	return err
}

// removeEntry は属性の並び o から e を消す (最後の 1 つなら `@(...)` ごと、前の空白も)。
func removeEntry(c *Ctx, o *syntax.Options, e *syntax.OptionEntry) {
	if len(o.Entries) == 1 {
		start := o.Pos().Offset
		for start > 0 && (c.Src[start-1] == ' ' || c.Src[start-1] == '\t') {
			start--
		}
		c.Replace(start, o.End().Offset, "")
		return
	}
	for i, x := range o.Entries {
		if x != e {
			continue
		}
		if i+1 < len(o.Entries) {
			c.Replace(e.Pos().Offset, o.Entries[i+1].Pos().Offset, "") // `fastcall, inline` → `inline`
		} else {
			end := e.End().Offset
			start := o.Entries[i-1].End().Offset // `inline, fastcall` → `inline`
			if bytes.IndexByte(c.Src[start:end], ',') < 0 {
				start = e.Pos().Offset
			}
			c.Replace(start, end, "")
		}
		return
	}
}

// runTestsRule は `@run_tests()` を `@run_tests_v3()` にする (構文だけで決まる): fc 4 の @run_tests は @(test) の関数を集めるが、
// fc 3 はスコープの test_* の関数を集めて stdio に出していた (その動きを @run_tests_v3 に残してある。doc/v4_stdlib.md §7.1)。
func runTestsRule(c *Ctx) {
	syntax.Inspect(c.File, func(n syntax.Node) bool {
		if id, ok := n.(*syntax.Ident); ok && id.Name == "@run_tests" {
			c.Replace(id.NamePos.Offset, id.NamePos.Offset+len(id.Name), "@run_tests_v3")
		}
		return true
	})
}
