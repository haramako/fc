package migrate

// fc 3 → fc 4 (doc/v4_plan.md §0)。fc 4 は意味を変えるので、書き換えは sema が fc 3 のソースをコンパイルしながら集める
// (sema.Rewrite。internal/sema/rewrite.go)。ここはそれをソースに当てて `#fc 4` にするだけ。プログラム単位の手順
// (fc 2 → 3 をメモリの上で済ませ、入口ごとにコンパイルして書き換えを集める) は driver の Compiler.Migrate。

import (
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
