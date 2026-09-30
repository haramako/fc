package migrate

// fc 3 → fc 4 (Agent/wiki/plans/v4-plan.md §0)。fc 4 は意味を変えるので、書き換えは sema が fc 3 のソースをコンパイルしながら集める
// (sema.Rewrite。internal/sema/rewrite.go)。ここはそれをソースに当てて `#fc 4` にするだけ。プログラム単位の手順
// (fc 2 → 3 をメモリの上で済ませ、入口ごとにコンパイルして書き換えを集める) は driver の Compiler.Migrate。

import (
	"bytes"
	"fmt"
	"strings"

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
	var copies, terms []sema.Rewrite
	for _, r := range rewrites {
		switch {
		case r.CopyEnd > r.CopyStart:
			copies = append(copies, r)
		case r.Rule == "string-terminator":
			terms = append(terms, r)
		default:
			c.Replace(r.Start, r.End, r.Text)
		}
	}
	// 文字列に `\0` を足す書き換えは、ほかの書き換えがその文字列ごと置き換えていれば (printf の引数を書式に取り込む) 当てない
	for _, r := range terms {
		if !overlaps(c.Edits, r.Start, r.End) {
			c.Replace(r.Start, r.End, r.Text)
		}
	}
	// 範囲の写しを含む書き換え (複合代入の 2 つめの左辺): 写す範囲の中の書き換えを当ててから写す
	base := append([]Edit{}, c.Edits...)
	for _, r := range copies {
		var inner []Edit
		for _, e := range base {
			if e.Start >= r.CopyStart && e.End <= r.CopyEnd {
				inner = append(inner, Edit{Start: e.Start - r.CopyStart, End: e.End - r.CopyStart, Text: e.Text})
			}
		}
		text, err := apply(src[r.CopyStart:r.CopyEnd], inner)
		if err != nil {
			return nil, fmt.Errorf("%s: %v", filename, err)
		}
		c.Replace(r.Start, r.End, r.Text+string(text)+r.Suffix)
	}
	if err := abiRules(c); err != nil {
		return nil, fmt.Errorf("%s: %v", filename, err)
	}
	runTestsRule(c)
	elsifRule(c)
	quoteRule(c)
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

// abiRules は関数の呼び出し規約の書き換え (構文だけで決まる。Agent/wiki/plans/v4-plan.md §2): fc 4 は fastcall を廃止し、extern (本体の
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
// fc 3 はスコープの test_* の関数を集めて stdio に出していた (その動きを @run_tests_v3 に残してある。Agent/wiki/plans/v4-stdlib.md §7.1)。
func runTestsRule(c *Ctx) {
	syntax.Inspect(c.File, func(n syntax.Node) bool {
		if id, ok := n.(*syntax.Ident); ok && id.Name == "@run_tests" {
			c.Replace(id.NamePos.Offset, id.NamePos.Offset+len(id.Name), "@run_tests_v3")
		}
		return true
	})
}

// quoteRule は fc 3 の文字列の綴りを fc 4 の綴りに書き換える (値は変えない):
//   - `'...'` (エスケープを解釈しない文字列) は fc 4 では文字のリテラルなので `"..."` に
//   - `"..."` の中の `\n` / `\xNN` 以外の `\` は fc 3 ではそのままの `\` だったが、fc 4 ではエスケープ (`\t` は tab、ほかはエラー)
//     なので `\\` に
func quoteRule(c *Ctx) {
	syntax.Inspect(c.File, func(n syntax.Node) bool {
		s, ok := n.(*syntax.StringLit)
		if !ok || !strings.HasPrefix(s.Text, "'") && !hasV3OnlyBackslash(s.Text) {
			return true
		}
		if overlaps(c.Edits, s.ValuePos.Offset, s.EndPos.Offset) {
			return true // 意味の書き換え (printf の書式に取り込む、終端の \0 を足すなど) が既にこの文字列を置き換えている
		}
		c.Replace(s.ValuePos.Offset, s.EndPos.Offset, syntax.QuoteString(s.Value))
		return true
	})
}

// hasV3OnlyBackslash は fc 3 の `"..."` / `"""..."""` の綴りに、fc 4 では意味の変わる `\` (`\n` と `\xNN` 以外) があるか。
func hasV3OnlyBackslash(text string) bool {
	for i := 0; i < len(text); i++ {
		if text[i] != '\\' {
			continue
		}
		if i+1 < len(text) && (text[i+1] == 'n' || text[i+1] == 'x') {
			i++ // \xNN の NN は普通の文字
			continue
		}
		return true
	}
	return false
}

// overlaps は [start:end] が edits のどれかと重なるか。
func overlaps(edits []Edit, start, end int) bool {
	for _, e := range edits {
		if e.Start < end && start < e.End {
			return true
		}
	}
	return false
}

// elsifRule は `elsif` を `else if` にする (fc 4 で elsif をなくした。意味は同じで、構文木の違いは整形の印 IsElsif だけ)。
func elsifRule(c *Ctx) {
	syntax.Inspect(c.File, func(n syntax.Node) bool {
		if s, ok := n.(*syntax.IfStmt); ok && s.IsElsif {
			c.Replace(s.If.Offset, s.If.Offset+len("elsif"), "else if")
		}
		return true
	})
}
