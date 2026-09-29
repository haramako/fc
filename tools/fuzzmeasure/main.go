// fuzzmeasure は fuzz と自動テストの効果を測る (Agent/wiki/testing-and-fuzzing.md「fuzz の効果の測定」)。
//
//	go run ./tools/fuzzmeasure zoo   [-bugs a,b] [-randn 200] [-foldn 100] [-seed 1] [-off feat,..] [-run REGEXP]
//	go run ./tools/fuzzmeasure cover [-randn 100] [-seed 1] [-feats a,b]
//
// zoo: testdata/bugzoo/*.patch (直したバグをわざと戻すパッチ) を 1 つずつ、HEAD の一時的な作業ツリー (git worktree) に
// 当ててテストを回し、テストごとに失敗した数を表にする (コミットしていない変更は入らない)。失敗した種で使っていた生成器の
// 機能 (FUZZ_STATS) も数える。-off で生成器の機能を切って回すと、その機能が無いと見つからないバグが分かる。
//
// cover: 全部の機能で回したときと、機能を 1 つずつ切ったときのコンパイラ (internal/...) の通過を比べ、その機能を切ると
// 通らなくなる block の数 (その機能だけが通す経路) を表にする。
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: fuzzmeasure zoo|cover [flags]")
		os.Exit(2)
	}
	switch os.Args[1] {
	case "zoo":
		zoo(os.Args[2:])
	case "cover":
		cover(os.Args[2:])
	default:
		fmt.Fprintln(os.Stderr, "usage: fuzzmeasure zoo|cover [flags]")
		os.Exit(2)
	}
}

// repoRoot はリポジトリの根 (git rev-parse)。
func repoRoot() string {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		fatal("git rev-parse: %v", err)
	}
	return strings.TrimSpace(string(out))
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

// testEvent は go test -json の 1 行。
type testEvent struct {
	Action string
	Test   string
}

// stat は FUZZ_STATS の 1 行 (internal/driver/randfeat_test.go の rpStat)。
type stat struct {
	Test     string   `json:"test"`
	Seed     int64    `json:"seed"`
	Kind     string   `json:"kind"`
	Features []string `json:"features"`
}

// runTests は dir で go test を回し、トップのテストごとの失敗したサブテストの数と、失敗した種の機能の数を返す。
func runTests(dir string, args []string, statsPath string) (map[string]int, map[string]int, time.Duration) {
	start := time.Now()
	cmd := exec.Command("go", append([]string{"test", "./internal/driver", "-count=1", "-json", "-timeout", "180m"}, args...)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "FUZZ_STATS="+statsPath)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = os.Stderr
	_ = cmd.Run() // 失敗するのが目的 (結果は -json から数える)
	fails := map[string]int{}
	topFail := map[string]bool{}
	sc := bufio.NewScanner(&out)
	sc.Buffer(make([]byte, 1<<20), 1<<26)
	for sc.Scan() {
		var e testEvent
		if json.Unmarshal(sc.Bytes(), &e) != nil || e.Action != "fail" || e.Test == "" {
			continue
		}
		if top, _, sub := strings.Cut(e.Test, "/"); sub {
			fails[top]++
		} else {
			topFail[e.Test] = true
		}
	}
	for t := range topFail {
		if fails[t] == 0 {
			fails[t] = 1 // サブテストの無いテスト (TestConstFoldCases など) の失敗
		}
	}
	feats := map[string]int{}
	if b, err := os.ReadFile(statsPath); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			var s stat
			if json.Unmarshal([]byte(line), &s) != nil || s.Kind == "ok" || s.Kind == "error" || s.Kind == "hang" {
				continue
			}
			for _, f := range s.Features {
				feats[f]++
			}
		}
	}
	return fails, feats, time.Since(start)
}

func zoo(argv []string) {
	fs := flag.NewFlagSet("zoo", flag.ExitOnError)
	bugs := fs.String("bugs", "", "回すバグ (カンマ区切り。空なら全部と none)")
	randn := fs.Int("randn", 200, "TestRandomPrograms / TestRandomV3Programs の本数")
	foldn := fs.Int("foldn", 100, "TestRandomConstFold の本数")
	seed := fs.Int64("seed", 1, "乱数の種")
	off := fs.String("off", "", "切る生成器の機能 (-fuzzoff)")
	metan := fs.Int("metan", 60, "TestRandomMetamorphic の本数")
	mutn := fs.Int("mutn", 60, "TestRandomMutate の本数")
	run := fs.String("run", "TestRandomPrograms|TestRandomV3Programs|TestRandomConstFold|TestRandomMetamorphic|TestRandomMutate|TestMustError|TestMustWarn|TestConstFoldCases|TestTypeRuleMatrix", "回すテスト")
	all := fs.Bool("all", true, "internal/driver の全テストを回し、-run 以外で失敗したテストを「その他」に出す (専用の単体テストや go の fuzz の入力があるか)")
	fs.Parse(argv)

	root := repoRoot()
	patches, _ := filepath.Glob(filepath.Join(root, "testdata", "bugzoo", "*.patch"))
	var names []string
	for _, p := range patches {
		names = append(names, strings.TrimSuffix(filepath.Base(p), ".patch"))
	}
	if *bugs != "" {
		names = strings.Split(*bugs, ",")
	} else {
		names = append([]string{"none"}, names...) // パッチを当てない (誤検出が 0 のはず)
	}
	goRun := *run
	if *all {
		goRun = "."
	}
	args := []string{"-run", goRun, "-randn", fmt.Sprint(*randn), "-foldn", fmt.Sprint(*foldn), "-metan", fmt.Sprint(*metan), "-mutn", fmt.Sprint(*mutn), "-randseed", fmt.Sprint(*seed)}
	if *off != "" {
		args = append(args, "-fuzzoff", *off)
	}
	tests := strings.Split(*run, "|")
	fmt.Printf("# bugzoo (randn %d, foldn %d, metan %d, mutn %d, seed %d, off %q)\n\n", *randn, *foldn, *metan, *mutn, *seed, *off)
	fmt.Printf("| バグ | %s | その他 | 時間 | 失敗した種で使っていた機能 (多い順) |\n|---|%s---|---|---|\n", strings.Join(tests, " | "), strings.Repeat("---|", len(tests)))
	for _, name := range names {
		dir, err := os.MkdirTemp("", "fuzzmeasure-")
		if err != nil {
			fatal("%v", err)
		}
		wt := filepath.Join(dir, "wt")
		if out, err := exec.Command("git", "-C", root, "worktree", "add", "--detach", wt, "HEAD").CombinedOutput(); err != nil {
			fatal("git worktree add: %v\n%s", err, out)
		}
		if name != "none" {
			patch := filepath.Join(root, "testdata", "bugzoo", name+".patch")
			if out, err := exec.Command("git", "-C", wt, "apply", patch).CombinedOutput(); err != nil {
				fmt.Printf("| %s | (パッチが当たらない: %s) |\n", name, strings.TrimSpace(string(out)))
				exec.Command("git", "-C", root, "worktree", "remove", "--force", wt).Run()
				os.RemoveAll(dir)
				continue
			}
		}
		fails, feats, took := runTests(wt, args, filepath.Join(dir, "stats.jsonl"))
		var cells []string
		listed := map[string]bool{}
		for _, t := range tests {
			cells = append(cells, fmt.Sprint(fails[t]))
			listed[t] = true
		}
		var others []string
		for t := range fails {
			if !listed[t] {
				others = append(others, t)
			}
		}
		sort.Strings(others)
		fmt.Printf("| %s | %s | %s | %s | %s |\n", name, strings.Join(cells, " | "), strings.Join(others, " "), took.Round(time.Second), topFeatures(feats, 6))
		exec.Command("git", "-C", root, "worktree", "remove", "--force", wt).Run()
		os.RemoveAll(dir)
	}
}

// topFeatures は数の多い順に n 個 (`名前×数`)。
func topFeatures(m map[string]int, n int) string {
	type kv struct {
		k string
		v int
	}
	var l []kv
	for k, v := range m {
		l = append(l, kv{k, v})
	}
	sort.Slice(l, func(i, j int) bool { return l[i].v > l[j].v || l[i].v == l[j].v && l[i].k < l[j].k })
	var r []string
	for i, e := range l {
		if i >= n {
			break
		}
		r = append(r, fmt.Sprintf("%s×%d", e.k, e.v))
	}
	return strings.Join(r, " ")
}

// coverBlocks は coverprofile の、通った block の集合 (`file:start,end`)。
func coverBlocks(path string) map[string]bool {
	b, err := os.ReadFile(path)
	if err != nil {
		fatal("%v", err)
	}
	r := map[string]bool{}
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) != 3 || f[2] == "0" || strings.HasPrefix(line, "mode:") {
			continue
		}
		r[f[0]] = true
	}
	return r
}

var featName = regexp.MustCompile(`^\t"([a-z0-9]+)":`)

// features は internal/driver/randfeat_test.go の rpFeatures の名前。
func features(root string) []string {
	b, err := os.ReadFile(filepath.Join(root, "internal", "driver", "randfeat_test.go"))
	if err != nil {
		fatal("%v", err)
	}
	var r []string
	for _, line := range strings.Split(string(b), "\n") {
		if m := featName.FindStringSubmatch(strings.TrimRight(line, "\r")); m != nil {
			r = append(r, m[1])
		}
	}
	return r
}

func cover(argv []string) {
	fs := flag.NewFlagSet("cover", flag.ExitOnError)
	randn := fs.Int("randn", 100, "TestRandomPrograms / TestRandomV3Programs の本数")
	seed := fs.Int64("seed", 1, "乱数の種")
	only := fs.String("feats", "", "測る機能 (カンマ区切り。空なら全部)")
	fs.Parse(argv)
	root := repoRoot()
	dir, err := os.MkdirTemp("", "fuzzcover-")
	if err != nil {
		fatal("%v", err)
	}
	defer os.RemoveAll(dir)
	profile := func(off string) (map[string]bool, time.Duration) {
		out := filepath.Join(dir, "c"+off+".out")
		args := []string{"test", "./internal/driver", "-count=1", "-run", "TestRandomPrograms|TestRandomV3Programs", "-randn", fmt.Sprint(*randn),
			"-randseed", fmt.Sprint(*seed), "-coverpkg=./internal/...", "-coverprofile=" + out, "-timeout", "180m"}
		if off != "" {
			args = append(args, "-fuzzoff", off)
		}
		cmd := exec.Command("go", args...)
		cmd.Dir = root
		cmd.Stderr = os.Stderr
		start := time.Now()
		_ = cmd.Run()
		return coverBlocks(out), time.Since(start)
	}
	all, took := profile("")
	fmt.Printf("# 機能ごとの独自の経路 (randn %d, seed %d。全部の機能で %d block、%s)\n\n", *randn, *seed, len(all), took.Round(time.Second))
	fmt.Println("| 機能 | 切ると通らなくなる block | 切ったときの時間 |\n|---|---|---|")
	names := features(root)
	if *only != "" {
		names = strings.Split(*only, ",")
	}
	for _, f := range names {
		without, t := profile(f)
		n := 0
		for b := range all {
			if !without[b] {
				n++
			}
		}
		fmt.Printf("| %s | %d | %s |\n", f, n, t.Round(time.Second))
	}
}
