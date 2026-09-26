package main

import (
	"path/filepath"

	"github.com/haramako/fc/pkg/fc"
)

// splitSrc はコマンドラインのソース (`fcc build m1/main.fc`) を、基準ディレクトリとファイル名に分ける。use / include は
// ソースのディレクトリから探す (language_reference §1.4。作業ディレクトリから探していて、親ディレクトリで実行すると
// `file a.fc not found` だった)。ディレクトリが付いていなければ ("main.fc") 今までどおり作業ディレクトリ。
// 出力 (-o) は作業ディレクトリ基準のまま。
func splitSrc(src string) (dir, file string) {
	if d := filepath.Dir(src); d != "." {
		return d, filepath.Base(src)
	}
	return "", src
}

// posDir は診断の位置の相対パスの基準 (splitSrc の dir)。位置はソースのディレクトリ基準なので、表示は作業ディレクトリ
// 基準に戻す (`m1/a.fc:3:4`)。
var posDir string

func displayPos(p fc.Position) fc.Position {
	if posDir != "" && p.Filename != "" && !filepath.IsAbs(p.Filename) {
		p.Filename = filepath.Join(posDir, p.Filename)
	}
	return p
}
