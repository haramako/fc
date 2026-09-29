package doccheck

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// 見に行かないディレクトリ (生成物・依存・作業用のコピー)
var skipDirs = map[string]bool{".git": true, "node_modules": true, ".fc-build": true, "dist": true, "worktrees": true, "golden": true}

// 参照を集めるファイル
var refExts = map[string]bool{".go": true, ".fc": true, ".asm": true, ".inc": true, ".md": true, ".yml": true, ".yaml": true, ".mts": true}

// repoFiles はリポジトリの中の refExts のファイル (Agent/discussions は過去の記録で、今は無いものを指してよいので除く)。
func repoFiles(t *testing.T, root string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if skipDirs[d.Name()] || rel == "Agent/discussions" || rel == "docs/.vitepress/cache" {
				return filepath.SkipDir
			}
			return nil
		}
		if refExts[filepath.Ext(p)] {
			files = append(files, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

var (
	// Agent/ と docs/ の下の .md への参照と、その後の「§ 番号」か「の「言葉」」
	docRefRe = regexp.MustCompile(`(?:^|[^A-Za-z0-9_./-])((?:Agent|docs)/[A-Za-z0-9_./-]+\.md)\)?(?:\s?§\s?([0-9][0-9A-Za-z.\-]*)|\s?の?「([^」]+)」)?`)
	// Markdown の相対リンク [text](path)
	linkRe       = regexp.MustCompile(`\]\(([^)\s]+)\)`)
	inlineCodeRe = regexp.MustCompile("`[^`\n]*`")
	fencedRe     = regexp.MustCompile("(?s)```.*?```")
)

// TestRepoDocRefs: 文書への参照が実在するものを指しているか。
//   - コメントや文書に書いた Agent/ と docs/ の下の .md のパスが在る
//   - その後の「§N」は N で始まる見出しが在る、「の「言葉」」は言葉が本文に在る (Agent/wiki/AGENTS.md の「参照の書き方」)
//   - Markdown の相対リンク (docs/ のサイトのページは VitePress のビルドが確かめるので除く) の先が在る
func TestRepoDocRefs(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	texts := map[string]string{} // 参照先の中身の読み込みの使い回し
	read := func(rel string) (string, bool) {
		if s, ok := texts[rel]; ok {
			return s, true
		}
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			return "", false
		}
		texts[rel] = strings.ReplaceAll(string(b), "\r\n", "\n")
		return texts[rel], true
	}
	for _, rel := range repoFiles(t, root) {
		src, _ := read(rel)
		for _, line := range lines(src) {
			for _, m := range docRefRe.FindAllStringSubmatch(line.text, -1) {
				target, sec, word := strings.TrimRight(m[1], "."), m[2], m[3]
				body, ok := read(target)
				if !ok {
					t.Errorf("%s:%d: %s は無い", rel, line.n, target)
					continue
				}
				if sec != "" && !hasSection(body, strings.TrimRight(sec, ".")) {
					t.Errorf("%s:%d: %s に §%s の見出しが無い", rel, line.n, target, sec)
				}
				if word != "" && !strings.Contains(body, word) {
					t.Errorf("%s:%d: %s に「%s」が無い", rel, line.n, target, word)
				}
			}
		}
		if !strings.HasSuffix(rel, ".md") || strings.HasPrefix(rel, "docs/") {
			continue
		}
		prose := inlineCodeRe.ReplaceAllString(fencedRe.ReplaceAllString(src, ""), "")
		for _, line := range lines(prose) {
			for _, m := range linkRe.FindAllStringSubmatch(line.text, -1) {
				link := m[1]
				if strings.Contains(link, "://") || strings.HasPrefix(link, "#") || strings.HasPrefix(link, "mailto:") {
					continue
				}
				p := strings.SplitN(link, "#", 2)[0]
				if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(filepath.ToSlash(filepath.Join(filepath.Dir(rel), p))))); err != nil {
					t.Errorf("%s:%d: リンク先 %s が無い", rel, line.n, link)
				}
			}
		}
	}
}

type numberedLine struct {
	n    int
	text string
}

func lines(s string) []numberedLine {
	var out []numberedLine
	for i, l := range strings.Split(s, "\n") {
		out = append(out, numberedLine{i + 1, l})
	}
	return out
}

// hasSection は body に「## N. …」「### N …」「## §N」のような N で始まる見出しがあるか。
func hasSection(body, sec string) bool {
	re := regexp.MustCompile(`^#+\s*(?:§\s*)?` + regexp.QuoteMeta(sec) + `(?:[.\s:）)．、]|$)`)
	for _, l := range strings.Split(body, "\n") {
		if strings.HasPrefix(l, "#") && re.MatchString(l) {
			return true
		}
	}
	return false
}
