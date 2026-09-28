package driver

// fuzz のプログラムの migrate (fc 2 → fc 3) の検査: 生成した fc 2 のプログラムを internal/migrate で fc 3 に書き換えて
// ビルドした ROM が、fc 2 のままビルドした ROM とバイト単位で一致する (-O 0 / -O 2)。

import (
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/haramako/fc/internal/migrate"
	"github.com/haramako/fc/internal/sema"
)

// romBuild は files を emu でビルドして ROM (a.bin) を返す (走らせない)。コンパイラの panic もエラーにする。
func romBuild(t *testing.T, files map[string]string, level int) (rom []byte, err error) {
	t.Helper()
	r := testBuild(t, buildSpec{Files: files, Level: level, Recover: true})
	if r.Err != nil {
		return nil, r.Err
	}
	return os.ReadFile(r.Out)
}

// v3Breaking は fc 3 で意図してエラーにした fc 2 の書き方 (migrate は書き換えない) のエラーか。fc 4 への migrate が
// 自動では書き換えられない形 (左辺に呼び出しのある複合代入の縮小: sema.CompoundCallMsg) も同じ扱い。
func v3Breaking(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "visible only in that case") || // switch の case ごとのスコープ
		strings.Contains(err.Error(), sema.CompoundCallMsg))
}

func TestRandomMigrate(t *testing.T) {
	t.Parallel()
	n := 20
	if *randN > 30 {
		n = *randN / 10
	}
	base := *randSeed
	if base == 0 {
		base = 1
	}
	var breaking atomic.Int32
	t.Cleanup(func() {
		if b := breaking.Load(); b > 0 {
			t.Logf("fc 3 の非互換 (case ごとのスコープ) でビルドできない種: %d", b)
		}
	})
	for k := 0; k < n; k++ {
		seed := base + int64(k)
		t.Run(fmt.Sprintf("seed%d", seed), func(t *testing.T) {
			t.Parallel()
			g := &rpGen{r: rand.New(rand.NewSource(seed))}
			g.genProgram()
			v2 := g.sources()
			v3 := map[string]string{}
			for name, src := range v2 {
				out, err := migrate.Migrate([]byte(src), name)
				if err != nil {
					t.Fatalf("migrate が失敗 (seed %d, %s): %v\n%s", seed, name, err, g.allSource())
				}
				v3[name] = string(out)
			}
			for _, level := range []int{-1, 0} {
				want, err := romBuild(t, v2, level)
				if err != nil {
					t.Skipf("fc 2 のプログラムがビルドできない (seed %d): %+v", seed, err)
				}
				got, err := romBuild(t, v3, level)
				if v3Breaking(err) {
					breaking.Add(1)
					t.Skipf("fc 3 の非互換 (seed %d): %v", seed, err)
				}
				if err != nil {
					t.Fatalf("-O %d: migrate したプログラムがビルドできない (seed %d): %v\n%s\n// ---- migrate の後 ----\n%s", level, seed, err, g.allSource(), v3["t.fc"])
				}
				if !bytes.Equal(got, want) {
					t.Fatalf("-O %d: migrate で ROM が変わった (seed %d)\n%s\n// ---- migrate の後 ----\n%s", level, seed, g.allSource(), v3["t.fc"])
				}
			}
			// 最新の版 (fc 4) まで: Compiler.Migrate (fc 3 → 4 は型を見て、意味の変わる所に今の意味の `as` などを足す)。
			// ROM は元のままと同じ (fc 4 の整数の規則の実装と、書き換えの取りこぼしの両方を見る。doc/v4_plan.md §0)
			v4, err := migrateToLatest(t, v2)
			if v3Breaking(err) {
				breaking.Add(1)
				t.Skipf("fc 3 の非互換 (seed %d): %v", seed, err)
			}
			if err != nil {
				t.Fatalf("fc 4 への migrate が失敗 (seed %d): %v\n%s", seed, err, g.allSource())
			}
			for _, level := range []int{-1, 0} {
				want, err := romBuild(t, v2, level)
				if err != nil {
					t.Skipf("fc 2 のプログラムがビルドできない (seed %d): %+v", seed, err)
				}
				got, err := romBuild(t, v4, level)
				if err != nil {
					t.Fatalf("-O %d: fc 4 に migrate したプログラムがビルドできない (seed %d): %v\n%s\n// ---- migrate の後 ----\n%s", level, seed, err, g.allSource(), joinSources(v4))
				}
				if !bytes.Equal(got, want) {
					t.Fatalf("-O %d: fc 4 への migrate で ROM が変わった (seed %d)\n%s\n// ---- migrate の後 ----\n%s", level, seed, g.allSource(), joinSources(v4))
				}
			}
		})
	}
}

// migrateToLatest は files (ファイル名 → ソース) を一時ディレクトリに書き、Compiler.Migrate で最新の版に書き換えた内容を返す。
func migrateToLatest(t *testing.T, files map[string]string) (map[string]string, error) {
	t.Helper()
	dir := t.TempDir()
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	paths := make([]string, len(names))
	for i, name := range names {
		paths[i] = filepath.Join(dir, name)
		if err := os.WriteFile(paths[i], []byte(files[name]), 0o666); err != nil {
			t.Fatal(err)
		}
	}
	res, err := NewCompiler(absRepoRoot).Migrate(paths, &MigrateOptions{})
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for i, name := range names {
		out[name] = string(res[paths[i]])
	}
	return out, nil
}

// joinSources はファイルを名前の順に並べた 1 つのテキスト (失敗の報告用)。
func joinSources(files map[string]string) string {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	var b strings.Builder
	for _, name := range names {
		fmt.Fprintf(&b, "// ---- %s ----\n%s", name, files[name])
	}
	return b.String()
}
