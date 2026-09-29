package fc

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/haramako/fc/internal/fcdoc"
	"github.com/haramako/fc/internal/syntax"
)

// DocModule はモジュールのドキュメント (fcc doc)。public の宣言と、その直前のコメント。
type DocModule = fcdoc.Module

// StdDocs は標準ライブラリ (FC_HOME の fclib/、fclib/nes/、fclib/emu/) の fc 4 のモジュールのドキュメント。
// fc 3 のまま残しているモジュール (古いプロジェクトのためのもの) は含めない。
func (c *Compiler) StdDocs() ([]*DocModule, error) {
	var mods []*DocModule
	for _, target := range []string{"", TargetNES, TargetEmu} {
		dir := filepath.Join(c.Home(), "fclib", target)
		files, err := filepath.Glob(filepath.Join(dir, "*.fc"))
		if err != nil {
			return nil, err
		}
		sort.Strings(files)
		for _, f := range files {
			m, err := DocFile(f)
			if err != nil {
				return nil, err
			}
			if m.Version < syntax.Version4 {
				continue
			}
			m.Target = target
			m.Path = strings.TrimPrefix(filepath.ToSlash(filepath.Join("fclib", target, filepath.Base(f))), "/")
			mods = append(mods, m)
		}
	}
	return mods, nil
}

// DocFile は fc のソースファイル 1 つのドキュメント。
func DocFile(path string) (*DocModule, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return fcdoc.Parse(path, src)
}

// DocIndex は標準ライブラリの一覧のページ (Markdown)。link はモジュールのページへのリンク。
func DocIndex(mods []*DocModule, link func(*DocModule) string) string { return fcdoc.Index(mods, link) }
