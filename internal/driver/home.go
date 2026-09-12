package driver

import (
	"os"
	"path/filepath"

	fcdata "github.com/haramako/fc"
)

// ResolveFCHome は fclib/ share/ を含むディレクトリ (FC_HOME) を探す。
//  1. 環境変数 FC_HOME
//  2. 実行ファイルの場所から上方向に探索
//  3. カレントディレクトリから上方向に探索
//  4. バイナリに同梱した embed.FS を一時ディレクトリに展開 (cleanup で削除する)
//
// cleanup は不要なとき nil。
func ResolveFCHome() (home string, cleanup func(), err error) {
	if h := os.Getenv("FC_HOME"); h != "" {
		return h, nil, nil
	}
	isHome := func(dir string) bool {
		fi1, err1 := os.Stat(filepath.Join(dir, "fclib"))
		fi2, err2 := os.Stat(filepath.Join(dir, "share"))
		return err1 == nil && err2 == nil && fi1.IsDir() && fi2.IsDir()
	}
	var starts []string
	if exe, err := os.Executable(); err == nil {
		starts = append(starts, filepath.Dir(exe))
	}
	if wd, err := os.Getwd(); err == nil {
		starts = append(starts, wd)
	}
	for _, start := range starts {
		dir := start
		for {
			if isHome(dir) {
				return dir, nil, nil
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	// embed.FS を展開する
	tmp, err := os.MkdirTemp("", "fc-home-")
	if err != nil {
		return "", nil, err
	}
	home, err = fcdata.Materialize(tmp)
	if err != nil {
		os.RemoveAll(tmp)
		return "", nil, err
	}
	return home, func() { os.RemoveAll(tmp) }, nil
}
