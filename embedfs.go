// Package fcdata は fclib/ と share/ を単一バイナリに同梱するための embed.FS を提供する。
// FC_HOME が見つからない環境では、この FS を一時ディレクトリに展開して使う
// (ca65/ld65 が実ファイルを必要とするため)。
package fcdata

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

//go:embed fclib share
var FS embed.FS

var (
	digestOnce sync.Once
	digest     string
)

// Digest は同梱した fclib/ share/ の中身のハッシュ (展開先のキャッシュの名前に使う。中身が同じ fcc は同じ展開先を使い回す)。
func Digest() string {
	digestOnce.Do(func() {
		h := sha256.New()
		fs.WalkDir(FS, ".", func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			data, err := FS.ReadFile(path)
			if err != nil {
				return err
			}
			h.Write([]byte(path))
			h.Write([]byte{0})
			h.Write(data)
			h.Write([]byte{0})
			return nil
		})
		digest = hex.EncodeToString(h.Sum(nil))
	})
	return digest
}

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
