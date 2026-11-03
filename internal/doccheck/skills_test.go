package doccheck

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestSkills: skills/ のエージェント向けのスキルと docs/public/llms.txt が、今のリポジトリと文書を指しているか (fc を直して
// ページ・見出し・道具の名前が変わったときに、スキルが黙って古くならないように)。
//   - SKILL.md の頭 (Agent Skills の name・description。name はディレクトリと同じ、小文字・数字・ハイフンで 64 字まで、description は 1024 字まで)
//   - 本文の「reference/<ページ> の「言葉」」の言葉が docs/reference/ のそのページに在る
//   - haramako.github.io/fc/ のページの URL が docs/ の .md に、raw.githubusercontent.com と github.com の blob / tree の
//     リポジトリの中のパスが在る
func TestSkills(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	nameRe := regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	skills, _ := filepath.Glob(filepath.Join(root, "skills", "*", "SKILL.md"))
	if len(skills) == 0 {
		t.Fatal("skills/*/SKILL.md が無い")
	}
	files := append([]string(nil), skills...)
	files = append(files, filepath.Join(root, "docs", "public", "llms.txt"), filepath.Join(root, "docs", "start", "agents.md"))
	for _, p := range skills {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		src := strings.ReplaceAll(string(b), "\r\n", "\n")
		rel, _ := filepath.Rel(root, p)
		parts := strings.SplitN(src, "\n---\n", 2)
		if !strings.HasPrefix(src, "---\n") || len(parts) != 2 {
			t.Errorf("%s: 頭 (--- で囲んだ name と description) が無い", rel)
			continue
		}
		head := map[string]string{}
		for _, l := range strings.Split(strings.TrimPrefix(parts[0], "---\n"), "\n") {
			if k, v, ok := strings.Cut(l, ":"); ok {
				head[k] = strings.TrimSpace(v)
			}
		}
		dir := filepath.Base(filepath.Dir(p))
		if n := head["name"]; n != dir || len(n) > 64 || !nameRe.MatchString(n) {
			t.Errorf("%s: name %q (ディレクトリと同じ、小文字・数字・ハイフン、64 字まで)", rel, n)
		}
		if d := head["description"]; d == "" || len([]rune(d)) > 1024 {
			t.Errorf("%s: description が無いか 1024 字を超える (%d 字)", rel, len([]rune(d)))
		}
		for _, m := range regexp.MustCompile(`reference/([a-z-]+) の「([^」]+)」`).FindAllStringSubmatch(parts[1], -1) {
			doc, err := os.ReadFile(filepath.Join(root, "docs", "reference", m[1]+".md"))
			if err != nil || !strings.Contains(string(doc), m[2]) {
				t.Errorf("%s: docs/reference/%s.md に「%s」が無い", rel, m[1], m[2])
			}
		}
	}
	pageRe := regexp.MustCompile(`https://haramako\.github\.io/fc/([A-Za-z0-9_/.-]*)`)
	repoRe := regexp.MustCompile(`https://(?:raw\.githubusercontent\.com/haramako/fc/main|github\.com/haramako/fc/(?:blob|tree)/main)/([A-Za-z0-9_/.$-]+)`)
	for _, p := range files {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		rel, _ := filepath.Rel(root, p)
		for _, m := range pageRe.FindAllStringSubmatch(string(b), -1) {
			page := strings.TrimSuffix(m[1], "/")
			var cands []string
			switch {
			case page == "":
				cands = []string{"index.md"}
			case page == "llms.txt":
				cands = []string{"public/llms.txt"}
			default:
				cands = []string{page + ".md", page + "/index.md"}
			}
			if !anyExists(filepath.Join(root, "docs"), cands) && !strings.HasPrefix(page, "reference/std") {
				t.Errorf("%s: %s のページが docs/ に無い", rel, m[0])
			}
		}
		for _, m := range repoRe.FindAllStringSubmatch(string(b), -1) {
			path := m[1]
			if strings.Contains(path, "$f") {
				// `for f in a b c; do curl …/$f` の形: ループの名前を一つずつ
				loop := regexp.MustCompile(`for f in ([^;]+);`).FindStringSubmatch(string(b))
				if loop == nil {
					t.Errorf("%s: %s の $f の並びが無い", rel, path)
					continue
				}
				for _, f := range strings.Fields(loop[1]) {
					if !anyExists(root, []string{strings.ReplaceAll(path, "$f", f)}) {
						t.Errorf("%s: %s が無い", rel, strings.ReplaceAll(path, "$f", f))
					}
				}
				continue
			}
			if !anyExists(root, []string{path}) {
				t.Errorf("%s: %s がリポジトリに無い", rel, path)
			}
		}
	}
}

func anyExists(dir string, names []string) bool {
	for _, n := range names {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(n))); err == nil {
			return true
		}
	}
	return false
}
