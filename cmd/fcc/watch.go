package main

// fcc watch: ソースが変わるたびにビルドし直す (エディタの隣で回しておく用)。ファイルの監視は外部ライブラリを使わず、
// ソースディレクトリ (と fclib) の .fc / .asm / .inc / .chr / .txt の更新時刻を 0.5 秒ごとに見る。

import (
	"context"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/haramako/fc/pkg/fc"
)

const watchUsage = `Usage: fcc watch [-t target] [-o FILE] [-O LEVEL] [-g] [-c] <src.fc>
    -t, --target     target platform ( nes, emu )
    -o FILE          output file
    -O LEVEL         optimize level (0-2)
    -g               emit debug info for Mesen
    -c               compile only (no link)
Rebuilds whenever a source file under the source directory (or fclib) changes. Ctrl-C to stop.
`

var watchExts = map[string]bool{".fc": true, ".asm": true, ".inc": true, ".chr": true, ".txt": true, ".cfg": true}

func runWatch(args []string) int {
	fs := flag.NewFlagSet("fcc watch", flag.ExitOnError)
	fs.Usage = func() { fmt.Print(watchUsage) }
	target := fs.String("t", "", "target platform")
	fs.StringVar(target, "target", "", "target platform")
	out := fs.String("o", "", "output file")
	optLevel := fs.Int("O", 2, "optimize level")
	gFlag := fs.Bool("g", false, "emit debug info")
	cFlag := fs.Bool("c", false, "compile only")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if fs.NArg() != 1 {
		fmt.Print(watchUsage)
		return 0
	}
	src := fs.Arg(0)
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
	opt := fc.Options{Target: *target, Out: *out, OptimizeLevel: optimizeLevel(*optLevel), Debug: *gFlag, CompileOnly: *cFlag}
	build := func() {
		start := time.Now()
		res, err := compiler.Build(context.Background(), src, opt)
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
