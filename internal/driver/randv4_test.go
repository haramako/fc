package driver

// fuzz の fc 4 版 (TestRandomProgramsV4): 生成した fc 2 のプログラムを、fc 4 でエラーになる所 (範囲外の定数・大きさが減る
// 暗黙の変換) にだけ `as` を足して fc 4 にし (Compiler.Migrate の MigrateOptions.Rules。A1・F1 の fc 4 の意味がそのまま効く)、
// -O 0 / -O 2 / 最適化前の IR のインタプリタで出力を比べる。A1 で広げた IR (大きさの違うオペランド・符号拡張を足した部分木) を
// 最適化と codegen が正しく扱うかを見る (Agent/wiki/plans/v4-plan.md §1.3 A)。sema の広げ方そのものの正しさは、型付きの定数と変数の差分
// (TestRandomConstFoldV4) と migrate の ROM の一致 (TestRandomMigrate) が見る。

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
)

// v4Rules は fc 4 でエラーになる所だけの書き換え (sema/convert.go の E・D、sema/intrules.go の F6、fc 4 の printf の書式文字列:
// sema/format.go、0 終端でない文字列をポインタにする所・文字列の定数と配列: sema/strconst.go、符号の混ざった演算の結果を解釈する所: sema/mixedarith.go)。
var v4Rules = []string{"constant-range", "narrowing", "sign-compare", "printf-format", "string-terminator", "string-length", "string-rows", "mixed-sign-arith"}

// migrateRules は files を一時ディレクトリに書き、Compiler.Migrate で rules の書き換えだけを当てて最新の版にした内容を返す。
func migrateRules(t *testing.T, files map[string]string, rules []string) (map[string]string, error) {
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
	res, err := NewCompiler(absRepoRoot).Migrate(paths, &MigrateOptions{Rules: rules})
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for i, name := range names {
		out[name] = string(res[paths[i]])
	}
	return out, nil
}

func TestRandomProgramsV4(t *testing.T) {
	t.Parallel()
	n := *randN / 2
	base := *randSeed
	if base == 0 {
		base = 1
	}
	var skipped atomic.Int32
	t.Cleanup(func() {
		if s := skipped.Load(); s > 0 {
			t.Logf("fc 4 にできなかった種 (fc 3 の非互換・fc 4 の規則のエラー): %d", s)
		}
	})
	for k := 0; k < n; k++ {
		seed := base + int64(k)
		t.Run(fmt.Sprintf("seed%d", seed), func(t *testing.T) {
			t.Parallel()
			g := &rpGen{r: rand.New(rand.NewSource(seed))}
			g.genProgram()
			files, err := migrateRules(t, g.sources(), v4Rules)
			if err != nil {
				skipped.Add(1)
				t.Skipf("fc 4 にできない (seed %d): %v", seed, err)
			}
			res := rpCheck(t, files)
			switch res.kind {
			case "ok":
			case "error":
				for _, s := range []string{"frame size over", "memory area overflow", "zero page index wrapped"} {
					if strings.Contains(res.detail, s) {
						t.Skipf("プログラムが大きすぎる (seed %d): %s", seed, s)
					}
				}
				if strings.Contains(res.detail, "does not fit in") || strings.Contains(res.detail, "implicitly (narrowing") {
					// fc 3 で集めた書き換えでは直らない fc 4 の規則のエラー (fc 4 の型が fc 3 と違う所)
					skipped.Add(1)
					t.Skipf("fc 4 の規則のエラー (seed %d): %s", seed, res.detail)
				}
				t.Fatalf("ビルド失敗 (seed %d):\n%s\n%s", seed, joinSources(files), res.detail)
			case "slow", "hang":
				t.Skipf("サイクルの上限 (seed %d)", seed)
			default:
				t.Errorf("%s (seed %d):\n%s\n%s", res.kind, seed, joinSources(files), res.detail)
			}
		})
	}
}
