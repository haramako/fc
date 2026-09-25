package driver

// fuzz のプログラムの migrate (fc 2 → fc 3) の検査: 生成した fc 2 のプログラムを internal/migrate で fc 3 に書き換えて
// ビルドした ROM が、fc 2 のままビルドした ROM とバイト単位で一致する (-O 0 / -O 2)。

import (
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/haramako/fc/internal/migrate"
)

// romBuild は files を emu でビルドして ROM (a.bin) を返す (走らせない)。コンパイラの panic もエラーにする。
func romBuild(t *testing.T, files map[string]string, level int) (rom []byte, err error) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	dir := t.TempDir()
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o666); err != nil {
			t.Fatal(err)
		}
	}
	out := filepath.Join(dir, "a.bin")
	if _, err := NewCompiler(absRepoRoot).Build("t.fc", &BuildOptions{Dir: dir, BuildDir: filepath.Join(dir, "b"), Out: out, OptimizeLevel: level}); err != nil {
		return nil, err
	}
	return os.ReadFile(out)
}

// v3Breaking は fc 3 で意図してエラーにした fc 2 の書き方 (migrate は書き換えない) のエラーか。
func v3Breaking(err error) bool {
	return err != nil && strings.Contains(err.Error(), "visible only in that case") // switch の case ごとのスコープ
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
		})
	}
}
