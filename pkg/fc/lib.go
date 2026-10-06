package fc

// fcc lib: fc.toml の [lib.*] のライブラリ (Agent/wiki/plans/v4-stdlib.md §9)。

import (
	"os"
	"path/filepath"

	"github.com/haramako/fc/internal/project"
)

// LibInfo はライブラリ 1 つの状態。
type LibInfo struct {
	Name   string
	Source string // path (フォルダ) か git の URL
	Rev    string // git の rev (fc.toml)
	Commit string // fc.lock のコミット (git のもの。取ってきていなければ "")
	Root   string // モジュールのあるフォルダ (取ってきていなければ "")
}

// LibFetch は dir から親へ探した fc.toml の [lib.*] を fc.lock のとおりに揃える (無いものは取ってきて fc.lock を書く)。
func LibFetch(dir string) ([]LibInfo, error) {
	return resolveLibs(dir, nil)
}

// LibUpdate は names (空なら全部) の git のライブラリを rev の今のコミットに進めて fc.lock を書き直す。
func LibUpdate(dir string, names []string) ([]LibInfo, error) {
	if len(names) == 0 {
		names = []string{"*"}
	}
	return resolveLibs(dir, names)
}

func resolveLibs(dir string, update []string) ([]LibInfo, error) {
	cfg, err := project.FindConfig(dir)
	if err != nil {
		return nil, err
	}
	r := &project.Resolver{Update: update, Log: project.StderrLog}
	libs, err := r.Resolve(cfg)
	if err != nil {
		return nil, err
	}
	var infos []LibInfo
	for _, l := range libs {
		infos = append(infos, LibInfo{Name: l.Name, Source: source(l.Lib), Rev: l.Rev, Commit: l.Commit, Root: l.Root})
	}
	return infos, nil
}

// LibAdd は fc.toml (dir から親へ探す。無ければ dir に作る) に [lib.name] を書き足して取ってくる。src はフォルダか git の URL。
// 取ってくるのに失敗したら fc.toml を元に戻す。
func LibAdd(dir, name, src, rev, sub string) ([]LibInfo, error) {
	path, old, existed, err := project.AddLib(dir, name, src, rev, sub)
	if err != nil {
		return nil, err
	}
	infos, err := resolveLibs(dir, nil)
	if err != nil {
		if existed {
			os.WriteFile(path, old, 0o666)
		} else {
			os.Remove(path)
		}
		return nil, err
	}
	return infos, nil
}

// LibList は fc.toml の [lib.*] と fc.lock を読んで並べる (取ってこない)。
func LibList(dir string) ([]LibInfo, error) {
	cfg, err := project.FindConfig(dir)
	if err != nil {
		return nil, err
	}
	libs, err := cfg.Libs()
	if err != nil {
		return nil, err
	}
	lock, err := cfg.ReadLock()
	if err != nil {
		return nil, err
	}
	var infos []LibInfo
	for _, l := range libs {
		info := LibInfo{Name: l.Name, Source: source(l), Rev: l.Rev}
		if l.Git == "" {
			info.Root = l.Path
		} else if e, ok := lock[l.Name]; ok && e.Git == l.Git && e.Rev == l.Rev {
			info.Commit = e.Commit
			if cache, err := project.CacheDir(); err == nil {
				p := filepath.Join(project.CachedSource(cache, l.Git, e.Commit), filepath.FromSlash(l.Dir))
				if _, err := os.Stat(p); err == nil {
					info.Root = p
				}
			}
		}
		infos = append(infos, info)
	}
	return infos, nil
}

func source(l project.Lib) string {
	if l.Git != "" {
		return l.Git
	}
	return l.Path
}
