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
	"regexp"
	"sort"
	"strconv"
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

func TestRandomProgramsV4(t *testing.T) { runRandomV4(t, false) }

// TestRandomV3ProgramsV4 は TestRandomV3Programs の生成器 (fc 3 のモジュール v3m: for-each・slice・範囲・負の添字・enum・
// struct の配列など、fc 3 以降の機能) のプログラムを fc 4 にして比べる。TestRandomProgramsV4 は fc 2 の生成器なのでこれらの
// 機能を含まない (fc 3 のまま回す TestRandomV3Programs は減らした。2026-09-30)。
func TestRandomV3ProgramsV4(t *testing.T) { runRandomV4(t, true) }

// runRandomV4 は生成したプログラム (v3 なら v3m のモジュールも) を fc 4 にして -O 0 / -O 2 / インタプリタで比べる。
func runRandomV4(t *testing.T, v3 bool) {
	t.Parallel()
	n := *randN / 2
	base := *randSeed
	if base == 0 {
		base = 1
	}
	var skipped atomic.Int32
	t.Cleanup(func() {
		s := skipped.Load()
		if s > 0 {
			t.Logf("fc 4 にできなかった種 (fc 3 の非互換・fc 4 の規則のエラー): %d", s)
		}
		// 見張り: 生成器や migrate の変更で fc 4 にできない種ばかりになると、何も比べずに通ってしまう
		if n >= 10 && int(s)*2 > n {
			t.Errorf("fc 4 にできなかった種が多すぎる: %d / %d", s, n)
		}
	})
	for k := 0; k < n; k++ {
		seed := base + int64(k)
		t.Run(fmt.Sprintf("seed%d", seed), func(t *testing.T) {
			t.Parallel()
			g := &rpGen{r: rand.New(rand.NewSource(seed))}
			if v3 {
				g.v3 = &rpV3{}
			}
			g.genProgram()
			files, err := migrateRules(t, g.sources(), v4Rules)
			if err != nil {
				skipped.Add(1)
				t.Skipf("fc 4 にできない (seed %d): %v", seed, err)
			}
			if seed%2 == 0 {
				files["t.fc"] = v4GlobalInits(files["t.fc"]) // 半分の種はグローバル変数の初期値の経路で
			}
			files = rpCondRewrite(files, seed) // 条件式と do-while を混ぜる (randcond_test.go)
			res := rpCheck(t, files)
			switch res.kind {
			case "ok":
			case "error":
				for _, s := range []string{"frame size over", "memory area overflow", "zero page index wrapped", "does not fit in the zero page", "static frames do not fit"} {
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

var (
	rpInitScalar = regexp.MustCompile(`^\s*(\w+) = (\(?-?\d+\)?);$`)
	rpInitElem   = regexp.MustCompile(`^\s*(\w+)\[(\d+)\] = (\(?-?\d+\)?);$`)
	rpInitOther  = regexp.MustCompile(`^\s*[\w.\[\]]+ = \(?-?\d+\)?;$`)
)

// v4GlobalInits は fc 4 にした t.fc の main の先頭の、グローバル変数・配列への定数の代入 (`g0 = 5;`、`a0[3] = 7;`) を宣言の
// 初期値 (`var g0:u8 = 5;`、`var a0:[16]u8 = [...];`) に移す。起動のときに写す初期値 (codegen の initRecords・runtime の
// fc_global_init) を、-O 0 / -O 2 / インタプリタで比べる。宣言の見つからないものはそのまま。
func v4GlobalInits(src string) string {
	lines := strings.Split(src, "\n")
	decl := map[string]int{} // 名前 → 宣言の行 (`var g0:u8;`)
	for i, l := range lines {
		if strings.HasPrefix(l, "var ") && strings.HasSuffix(l, ";") && !strings.Contains(l, "=") && !strings.Contains(l, "@(") {
			if name, _, ok := strings.Cut(strings.TrimSuffix(strings.TrimPrefix(l, "var "), ";"), ":"); ok {
				decl[name] = i
			}
		}
	}
	main := -1
	for i, l := range lines {
		if strings.HasPrefix(l, "function main()") {
			main = i
			break
		}
	}
	if main < 0 {
		return src
	}
	scalars := map[string]string{}
	elems := map[string]map[int]string{}
	drop := map[int]bool{}
	for i := main + 1; i < len(lines); i++ {
		l := lines[i]
		if t := strings.TrimSpace(l); t == "{" || strings.HasPrefix(t, "var ") {
			continue // main のローカル変数の宣言 (初期化の代入はその後)
		}
		if m := rpInitScalar.FindStringSubmatch(l); m != nil && decl[m[1]] > 0 {
			scalars[m[1]] = m[2]
			drop[i] = true
		} else if m := rpInitElem.FindStringSubmatch(l); m != nil && decl[m[1]] > 0 && strings.Contains(lines[decl[m[1]]], ":[16]") {
			if elems[m[1]] == nil {
				elems[m[1]] = map[int]string{}
			}
			k, _ := strconv.Atoi(m[2])
			elems[m[1]][k] = m[3]
			drop[i] = true
		} else if !rpInitOther.MatchString(l) {
			break
		}
	}
	for name, v := range scalars {
		lines[decl[name]] = strings.TrimSuffix(lines[decl[name]], ";") + " = " + v + ";"
	}
	for name, es := range elems {
		vals := make([]string, 16)
		for k := range vals {
			if v, ok := es[k]; ok {
				vals[k] = v
			} else {
				vals[k] = "0"
			}
		}
		lines[decl[name]] = strings.TrimSuffix(lines[decl[name]], ";") + " = [" + strings.Join(vals, ", ") + "];"
	}
	var out []string
	for i, l := range lines {
		if !drop[i] {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}
