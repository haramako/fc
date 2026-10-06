package main

// fcc watch: ソースが変わるたびにビルドし直す (エディタの隣で回しておく用)。ファイルの監視は外部ライブラリを使わず、
// ソースディレクトリ (と fclib) の .fc / .asm / .inc / .chr / .txt / .cfg の更新時刻を 0.5 秒ごとに見る。

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/haramako/fc/pkg/fc"
)

var watchExts = map[string]bool{".fc": true, ".asm": true, ".inc": true, ".chr": true, ".txt": true, ".cfg": true}

type watchCmd struct {
	buildFlags
	Compile bool   `short:"c" help:"Compile only (no link)."`
	Src     string `arg:"" help:"The main source file."`
}

func (c *watchCmd) run() int {
	src := c.Src
	compiler, err := fc.New()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer compiler.Close()

	dirs := []string{filepath.Dir(src)}
	if home := compiler.Home(); home != "" {
		dirs = append(dirs, filepath.Join(home, "fclib"))
	}
	opt := c.options()
	opt.CompileOnly = c.Compile
	dir, file := splitSrc(src)
	opt.Dir, posDir = dir, dir
	build := func() {
		start := time.Now()
		res, err := compiler.Build(context.Background(), file, opt)
		stamp := time.Now().Format("15:04:05")
		if err != nil {
			printErrors(err)
			fmt.Printf("[%s] failed\n", stamp)
			return
		}
		printWarnings(res.Warnings)
		fmt.Printf("[%s] ok (%d warnings, %.1fs)\n", stamp, len(res.Warnings), time.Since(start).Seconds())
	}
	build()
	last := snapshotSources(dirs)
	for {
		time.Sleep(500 * time.Millisecond)
		cur := snapshotSources(dirs)
		if changed := changedFiles(last, cur); len(changed) > 0 {
			fmt.Printf("changed: %s\n", strings.Join(changed, " "))
			last = cur
			build()
		}
	}
}

// snapshotSources は dirs の下のソースファイルの更新時刻と大きさ。
func snapshotSources(dirs []string) map[string]string {
	r := map[string]string{}
	for _, dir := range dirs {
		_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if name := d.Name(); name == ".fc-build" || name == ".git" || name == "node_modules" {
					return filepath.SkipDir
				}
				return nil
			}
			if !watchExts[strings.ToLower(filepath.Ext(path))] {
				return nil
			}
			if info, err := d.Info(); err == nil {
				r[path] = fmt.Sprintf("%d:%d", info.ModTime().UnixNano(), info.Size())
			}
			return nil
		})
	}
	return r
}

func changedFiles(old, cur map[string]string) []string {
	var r []string
	for p, v := range cur {
		if old[p] != v {
			r = append(r, filepath.Base(p))
		}
	}
	for p := range old {
		if _, ok := cur[p]; !ok {
			r = append(r, filepath.Base(p)+" (deleted)")
		}
	}
	return r
}
