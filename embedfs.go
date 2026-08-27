// Package fcdata は fclib/ と share/ を単一バイナリに同梱するための embed.FS を提供する。
// FC_HOME が見つからない環境では、この FS を一時ディレクトリに展開して使う
// (ca65/ld65 が実ファイルを必要とするため)。
package fcdata

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed fclib share
var FS embed.FS

// Materialize は fclib/ share/ を dir に展開し、FC_HOME として使えるパスを返す。
func Materialize(dir string) (string, error) {
	err := fs.WalkDir(FS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		dst := filepath.Join(dir, path)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o777)
		}
		data, err := FS.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, data, 0o666)
	})
	if err != nil {
		return "", err
	}
	return dir, nil
}
