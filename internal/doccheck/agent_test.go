package doccheck

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// Agent/ の索引と大きさの検査 (Agent/AGENTS.md の「大きさの予算」)。

// agentIndex は索引 1 つ: dir の下の記録のファイルが、files の索引のどれかにちょうど 1 回載る。
type agentIndex struct {
	dir     string   // 記録を置くディレクトリ
	files   []string // 索引のファイル (dir からの glob)。AGENTS.md は section の見出しの後だけを見る
	section string   // AGENTS.md の索引の見出し
	nested  bool     // dir の下のフォルダの中のファイルも記録
}

var agentIndexes = []agentIndex{
	{dir: "Agent/wiki", files: []string{"AGENTS.md"}, section: "## Articles", nested: true},
	{dir: "Agent/discussions", files: []string{"AGENTS.md", "log-*.md"}, section: "## Log"},
	{dir: "Agent/issues", files: []string{"AGENTS.md"}, section: "## Index"},
}

// 索引そのもので、記録ではないファイル
func isIndexFile(name string) bool {
	return name == "AGENTS.md" || name == "CLAUDE.md" || (strings.HasPrefix(name, "log-") && strings.HasSuffix(name, ".md"))
}

// TestAgentIndex: 記録のファイルと索引の行が 1 対 1 か (索引に無い記録・2 回載る記録・先が無い行)。
func TestAgentIndex(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	for _, ix := range agentIndexes {
		dir := filepath.Join(root, filepath.FromSlash(ix.dir))
		var records []string
		err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if p != dir && !ix.nested {
					return filepath.SkipDir
				}
				return nil
			}
			rel, _ := filepath.Rel(dir, p)
			if !isIndexFile(d.Name()) {
				records = append(records, filepath.ToSlash(rel))
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		count := map[string]int{}
		for _, pat := range ix.files {
			matches, _ := filepath.Glob(filepath.Join(dir, pat))
			for _, m := range matches {
				b, err := os.ReadFile(m)
				if err != nil {
					t.Fatal(err)
				}
				src := strings.ReplaceAll(string(b), "\r\n", "\n")
				name := ix.dir + "/" + filepath.Base(m)
				if filepath.Base(m) == "AGENTS.md" {
					i := strings.Index(src, "\n"+ix.section+"\n")
					if i < 0 {
						t.Errorf("%s: 索引の見出し %q が無い", name, ix.section)
						continue
					}
					src = src[i:]
				}
				for _, l := range linkRe.FindAllStringSubmatch(fencedRe.ReplaceAllString(src, ""), -1) {
					link := strings.SplitN(l[1], "#", 2)[0]
					if strings.Contains(link, "://") || link == "" {
						continue
					}
					target := path.Clean(link)
					if strings.HasPrefix(target, "../") {
						continue // ほかの領域への言及
					}
					if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(target))); err != nil {
						t.Errorf("%s: 索引の行の先 %s が無い", name, link)
						continue
					}
					count[target]++
				}
			}
		}
		for _, r := range records {
			switch n := count[r]; {
			case n == 0:
				t.Errorf("%s/%s が索引 (%s) に無い", ix.dir, r, strings.Join(ix.files, "・"))
			case n > 1:
				t.Errorf("%s/%s が索引に %d 回載る (1 回だけにする)", ix.dir, r, n)
			}
		}
	}
}

// agentBudget は大きさの予算 1 つ (バイト)。目安を超えたら掃除する、上限を超えたら落ちる。
type agentBudget struct {
	pattern       string // リポジトリのルートからの path.Match のパターン
	target, limit int
	why           string // 決まりと違う予算にした理由
}

const kb = 1024

// agentBudgets は上から順に、最初に合ったものを使う。毎回読むもの (AGENTS.md・索引・TODO.md) は小さく、wiki の記事は作業ごとに 1 本読む大きさに。
var agentBudgets = []agentBudget{
	{pattern: "AGENTS.md", target: 12 * kb, limit: 16 * kb},
	{pattern: "Agent/AGENTS.md", target: 6 * kb, limit: 8 * kb},
	{pattern: "Agent/TODO.md", target: 2 * kb, limit: 4 * kb},
	{pattern: "Agent/discussions/AGENTS.md", target: 10 * kb, limit: 14 * kb}, // 古い行は log-YYYY-MM.md へ
	{pattern: "Agent/wiki/AGENTS.md", target: 10 * kb, limit: 14 * kb},
	{pattern: "Agent/issues/AGENTS.md", target: 3 * kb, limit: 5 * kb},
	{pattern: "Agent/scripts/AGENTS.md", target: 2 * kb, limit: 4 * kb},
	{pattern: "Agent/wiki/plans/roadmap.md", target: 20 * kb, limit: 30 * kb}, // 済んだ項目は消す
	// 移行中: 仕様の部分を design/ に抜き出して小さくするまで (Agent/TODO.md)
	{pattern: "Agent/wiki/plans/v4-stdlib.md", target: 24 * kb, limit: 58 * kb, why: "design/ へ抜き出すまで"},
	{pattern: "Agent/wiki/plans/v4-plan.md", target: 24 * kb, limit: 50 * kb, why: "design/ へ抜き出すまで"},
	{pattern: "Agent/wiki/testing-and-fuzzing.md", target: 24 * kb, limit: 48 * kb, why: "回し方と事例に分けるまで"},
	{pattern: "Agent/wiki/*.md", target: 24 * kb, limit: 40 * kb},
	{pattern: "Agent/wiki/*/*.md", target: 24 * kb, limit: 40 * kb},
}

// TestAgentBudget: 予算の表に合うファイルが上限の内か。-v で全部の大きさと目安を並べる。
func TestAgentBudget(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	var files []string
	seen := map[string]bool{}
	for _, pat := range []string{"AGENTS.md", "Agent/*.md", "Agent/*/AGENTS.md", "Agent/wiki/*.md", "Agent/wiki/*/*.md"} {
		m, _ := filepath.Glob(filepath.Join(root, filepath.FromSlash(pat)))
		for _, p := range m {
			rel, _ := filepath.Rel(root, p)
			rel = filepath.ToSlash(rel)
			if !seen[rel] && filepath.Base(rel) != "CLAUDE.md" {
				seen[rel] = true
				files = append(files, rel)
			}
		}
	}
	sort.Strings(files)
	var report []string
	for _, f := range files {
		var bg *agentBudget
		for i := range agentBudgets {
			if ok, _ := path.Match(agentBudgets[i].pattern, f); ok {
				bg = &agentBudgets[i]
				break
			}
		}
		if bg == nil {
			continue
		}
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f)))
		if err != nil {
			t.Fatal(err)
		}
		size := len(b) - strings.Count(string(b), "\r") // Windows の改行でも同じ数に
		mark := ""
		switch {
		case size > bg.limit:
			mark = "上限超"
			t.Errorf("%s: %d バイトが上限 %d を超えた。掃除する (済んだ項目を消す・経緯を discussions へ・索引の古い行を月の索引へ・記事を分ける。Agent/AGENTS.md の「大きさの予算」)",
				f, size, bg.limit)
		case size > bg.target:
			mark = "目安超"
		}
		note := bg.why
		if note != "" {
			note = "(" + note + ")"
		}
		report = append(report, fmt.Sprintf("%7.1f KB  目安 %3d  上限 %3d  %-6s %s %s", float64(size)/kb, bg.target/kb, bg.limit/kb, mark, f, note))
	}
	t.Log("Agent/ の大きさ:\n" + strings.Join(report, "\n"))
}
