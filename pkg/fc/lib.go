package fc

// fcc lib: fc.toml の [lib.*] のライブラリ (doc/v4_stdlib.md §9)。

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
	r := &project.Resolver{Update: update, Log: func(f string, a ...any) { logf(f, a...) }}
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

var logf = func(f string, a ...any) {
	os.Stderr.WriteString("fcc: " + strings.TrimSuffix(fmt.Sprintf(f, a...), "\n") + "\n")
}
