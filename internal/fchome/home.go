// Package fchome は fclib/ share/ を含むディレクトリ (FC_HOME) の解決 (無ければ同梱のものをキャッシュに展開する)。
// Package fchome は fclib/ share/ を含むディレクトリ (FC_HOME) の解決 (無ければ同梱のものをキャッシュに展開する)。
package fchome

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	fcdata "github.com/haramako/fc"
)

// Resolve は fclib/ share/ を含むディレクトリ (FC_HOME) を探す。
//  1. 環境変数 FC_HOME
//  2. 実行ファイルの場所から上方向に探索
//  3. カレントディレクトリから上方向に探索
//  4. バイナリに同梱した embed.FS をユーザーのキャッシュ (FC_CACHE_DIR、無ければ os.UserCacheDir()/fc) の
//     home-<中身のハッシュ> に展開して使い回す (消さない)
//  5. 4 ができなければ一時ディレクトリに展開 (cleanup で削除する)
//
// 4 は 2026-09-28 から。以前は実行のたびに別の一時ディレクトリに展開していて、-g の .s に入る fclib のパス
// (`.dbg file`) が毎回変わり、fclib のコードを含むモジュールを毎回アセンブルし直していた (castle で 45 個中 18 個)。
// ビルドの後に一時ディレクトリを消すので、Mesen から fclib のソースも開けなかった。
//
// cleanup は不要なとき nil。
func Resolve() (home string, cleanup func(), err error) {
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
	// embed.FS をキャッシュに展開する (できなければ一時ディレクトリへ)
	if home, err := cachedHome(); err == nil {
		return home, nil, nil
	}
	tmp, err := os.MkdirTemp("", "fc-home-")
	if err != nil {
		tempEnv := "TMPDIR"
		if runtime.GOOS == "windows" {
			tempEnv = "TMP and TEMP"
		}
		return "", nil, fmt.Errorf("failed to create a temporary directory for bundled FC libraries\n"+
			"  parent: %s\n  cause: %w\n"+
			"Set %s to a writable directory, or set FC_HOME to a directory containing fclib/ and share/.\n"+
			"When running through an agent/shell tool, pass these variables to the fcc process.", os.TempDir(), err, tempEnv)
	}
	home, err = fcdata.Materialize(tmp)
	if err != nil {
		os.RemoveAll(tmp)
		return "", nil, err
	}
	return home, func() { os.RemoveAll(tmp) }, nil
}

// homeComplete は展開し終えたキャッシュの印 (途中で止まった展開を使わない)。
const homeComplete = ".fc-home-complete"

// cachedHome は同梱の fclib/ share/ を、中身のハッシュで名前を付けたキャッシュのディレクトリに展開して返す (展開済みなら
// そのまま)。同時に走る fcc は一時的な名前に展開してから名前を変え、先に置かれていればそれを使う。
func cachedHome() (string, error) {
	base := os.Getenv("FC_CACHE_DIR")
	if base == "" {
		d, err := os.UserCacheDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(d, "fc")
	}
	dir := filepath.Join(base, "home-"+fcdata.Digest()[:16])
	if _, err := os.Stat(filepath.Join(dir, homeComplete)); err == nil {
		return dir, nil
	}
	if err := os.MkdirAll(base, 0o777); err != nil {
		return "", err
	}
	tmp, err := os.MkdirTemp(base, "tmp-home-")
	if err != nil {
		return "", err
	}
	if _, err := fcdata.Materialize(tmp); err != nil {
		os.RemoveAll(tmp)
		return "", err
	}
	if err := os.WriteFile(filepath.Join(tmp, homeComplete), nil, 0o666); err != nil {
		os.RemoveAll(tmp)
		return "", err
	}
	if err := os.Rename(tmp, dir); err != nil {
		os.RemoveAll(tmp)
		// ほかの fcc が先に置いた
		if _, serr := os.Stat(filepath.Join(dir, homeComplete)); serr != nil {
			return "", err
		}
	}
	return dir, nil
}
