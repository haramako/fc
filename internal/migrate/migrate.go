// Package migrate は fc 2 のソースを fc 3 に書き換える (`fcc migrate`)。
//
// 書き換えはソースのバイト列への置き換え (Edit) の列で、トークンの位置を使う。構文木を印字し直さないので、
// コメント・空行・書式はそのまま残る。fc 3 で文法を変えるときは、その変更の Rule をここに足し、
// internal/driver の TestMigrate* (castle / miku / bench / golden のソースを migrate して fc 3 としてビルドし、
// fc 2 のままのビルドと ROM・出力がバイト単位で一致する) で確かめる。
//
// 移行期間はコンパイラが fc 2 と fc 3 の両方を受け付ける (syntax.Version2 / Version3。モジュールごとに混ぜられる)。
package migrate

import (
	"bytes"
	"fmt"
	"sort"

	"github.com/haramako/fc/internal/syntax"
)

// Edit はソースの src[Start:End] を Text に置き換える。
type Edit struct {
	Start, End int
	Text       string
}

// Ctx は 1 ファイル分の書き換えの状態。Rule は File (fc 2 として解析した構文木) と Tokens を見て Edits を足す。
type Ctx struct {
	Src    []byte
	File   *syntax.File
	Tokens []syntax.Token
	Edits  []Edit
}

// Replace は src[start:end] を text に置き換える。
func (c *Ctx) Replace(start, end int, text string) {
	c.Edits = append(c.Edits, Edit{Start: start, End: end, Text: text})
}

// ReplaceToken はトークン t を text に置き換える。
func (c *Ctx) ReplaceToken(t syntax.Token, text string) {
	c.Replace(t.Pos.Offset, t.End.Offset, text)
}

// Rule は fc 2 → fc 3 の書き換え規則 1 つ。
type Rule struct {
	Name  string // 規則の名前 (`fcc migrate -rules` / エラーの表示用)
	Doc   string // 何をどう書き換えるか (1 行)
	Apply func(c *Ctx)
}

// Rules は適用する規則 (順に Apply する。どれも元のソースの位置に対する Edit を足すだけなので、順序は Edit の
// 重なりにしか影響しない)。fc 3 の文法変更と一緒に足す。
var Rules []Rule

// Migrate は fc 2 のソース src を fc 3 に書き換える。すでに fc 3 のソースはそのまま返す。書き換えた結果が
// fc 3 として解析できなければエラー (規則の不具合)。
func Migrate(src []byte, filename string) ([]byte, error) {
	f, err := syntax.Parse(src, filename)
	if err != nil {
		return nil, err
	}
	if f.Version == syntax.Version3 {
		return src, nil
	}
	toks, _, err := syntax.Tokenize(src, filename)
	if err != nil {
		return nil, err
	}
	c := &Ctx{Src: src, File: f, Tokens: toks}
	for _, r := range Rules {
		r.Apply(c)
	}
	// プラグマ: `#fc 2` の行を `#fc 3` に。無ければ先頭に足す
	if f.Pragma != "" {
		c.Replace(0, len(f.Pragma), fmt.Sprintf("#fc %d", syntax.Version3))
	} else {
		nl := "\n"
		if i := bytes.IndexByte(src, '\n'); i > 0 && src[i-1] == '\r' {
			nl = "\r\n" // 改行が CRLF のソース (Windows の作業ツリー) は揃える
		}
		c.Replace(0, 0, fmt.Sprintf("#fc %d%s", syntax.Version3, nl))
	}
	out, err := apply(src, c.Edits)
	if err != nil {
		return nil, fmt.Errorf("%s: %v", filename, err)
	}
	g, err := syntax.Parse(out, filename)
	if err != nil {
		return nil, fmt.Errorf("%s: migrated source does not parse as fc 3 (migrate rule bug): %v", filename, err)
	}
	if g.Version != syntax.Version3 {
		return nil, fmt.Errorf("%s: migrated source is not fc 3", filename)
	}
	return out, nil
}

// apply は edits を src に当てる (位置の順に並べ、重なりはエラー。同じ位置への挿入は足した順で、その位置からの
// 置き換えより前: 先頭のトークンを書き換えるソースに `#fc 3` を足すとき)。
func apply(src []byte, edits []Edit) ([]byte, error) {
	es := append([]Edit{}, edits...)
	sort.SliceStable(es, func(i, j int) bool {
		if es[i].Start != es[j].Start {
			return es[i].Start < es[j].Start
		}
		return es[i].End == es[i].Start && es[j].End > es[j].Start
	})
	var b bytes.Buffer
	at := 0
	for _, e := range es {
		if e.Start < at || e.End < e.Start || e.End > len(src) {
			return nil, fmt.Errorf("overlapping or invalid edit [%d:%d] %q", e.Start, e.End, e.Text)
		}
		b.Write(src[at:e.Start])
		b.WriteString(e.Text)
		at = e.End
	}
	b.Write(src[at:])
	return b.Bytes(), nil
}
