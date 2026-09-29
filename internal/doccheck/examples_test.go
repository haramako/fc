package doccheck

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/driver"
	"github.com/haramako/fc/internal/syntax"
)

// codeBlock は Markdown のフェンスで囲まれたコードのブロック。
type codeBlock struct {
	file  string   // リポジトリのルートからのパス
	line  int      // 開きのフェンスの行 (1 から)
	lang  string   // info string の最初の語 (```fc run なら "fc")
	attrs []string // 残りの語 (```fc run なら ["run"])
	code  string
}

func (b *codeBlock) name() string { return b.file + ":" + strconv.Itoa(b.line) }

// codeOrEmpty は中身 (b が nil なら "")。
func (b *codeBlock) codeOrEmpty() string {
	if b == nil {
		return ""
	}
	return b.code
}

// cliErrors は fcc と同じ形のエラーの行 (`file:line:col: error: msg`。cmd/fcc の printErrors)。
func cliErrors(err error) string {
	es := diag.Errors(err)
	if len(es) == 0 {
		return err.Error()
	}
	var b strings.Builder
	for _, e := range es {
		fmt.Fprintf(&b, "%s: error: %s\n", e.Pos, e.Msg)
	}
	return b.String()
}

// fcFileRe は行の頭の「ファイル名.fc:」(エラーの位置)。
var fcFileRe = regexp.MustCompile(`(?m)^[\w./-]+\.fc:`)

var fenceRe = regexp.MustCompile("^([ \t]*)(`{3,}|~{3,})[ \t]*([^`]*)$")

// parseBlocks は Markdown からフェンスのブロックを順に取り出す。
func parseBlocks(file, text string) []*codeBlock {
	var blocks []*codeBlock
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	for i := 0; i < len(lines); i++ {
		m := fenceRe.FindStringSubmatch(lines[i])
		if m == nil {
			continue
		}
		indent, fence, info := m[1], m[2], strings.TrimSpace(m[3])
		b := &codeBlock{file: file, line: i + 1}
		if f := strings.FieldsFunc(info, func(r rune) bool { return r == ' ' || r == '\t' }); len(f) > 0 {
			// VitePress の ```fc{1,3} / ```fc:line-numbers も言語は fc
			b.lang = strings.FieldsFunc(f[0], func(r rune) bool { return r == '{' || r == ':' })[0]
			b.attrs = f[1:]
		}
		var body []string
		for i++; i < len(lines); i++ {
			l := lines[i]
			if t := strings.TrimLeft(l, " \t"); strings.HasPrefix(t, fence) && strings.TrimSpace(strings.TrimLeft(t, fence[:1])) == "" {
				break
			}
			body = append(body, strings.TrimPrefix(l, indent))
		}
		if len(body) > 0 {
			b.code = strings.Join(body, "\n") + "\n"
		}
		blocks = append(blocks, b)
	}
	return blocks
}

// docsFiles は docs/ のサイトのページ (VitePress の srcExclude と生成物・依存を除く。reference/std は fcc doc が標準ライブラリの
// コメントから作るページで、コメントの中の例は断片なので見ない)。
func docsFiles(t *testing.T) []string {
	t.Helper()
	var files []string
	root := filepath.Join(repoRoot(t), "docs")
	std := filepath.Join(root, "reference", "std")
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case "node_modules", ".vitepress", "public":
				return filepath.SkipDir
			}
			if p == std {
				return filepath.SkipDir
			}
			return nil
		}
		switch d.Name() {
		case "AGENTS.md", "CLAUDE.md", "language_reference.md":
			return nil
		}
		if strings.HasSuffix(p, ".md") {
			files = append(files, p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// TestDocsExamples: docs/ の ```fc のブロックを確かめる。印 (info string の fc の後の語) ごとに:
//
//	(無し)  断片。構文が通り、fcc fmt の書式と同じ (#fc の行が無ければ fc 4。トップレベルで読めなければ関数の本体として読む)
//	run     emu でビルドして走らせ、終了コード 0 で、次の ```text のブロック (次の ```fc より前) と出力が同じ。警告も無いこと
//	test    fcc test と同じく @(test) の関数を走らせて通る (次に ```text があれば出力も比べる)
//	nes     -t nes でビルドが通る (警告無し)
//	error   ビルドがエラーになり、次の ```text のブロックがあればその文言を含む
//	ignore  確かめない (使うときは理由を書く)
//
// file=名前.fc を付けたブロックは、そのページの後のブロックが一緒にビルドするファイルになる (同じ名前がもうあれば後ろに足す:
// 「score.fc の最後に足します」)。run / test / nes のブロックに付ければ、そのファイルを入口 (test ならテストするモジュール) にする。
func TestDocsExamples(t *testing.T) {
	root := repoRoot(t)
	for _, path := range docsFiles(t) {
		text, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		rel, _ := filepath.Rel(root, path)
		blocks := parseBlocks(filepath.ToSlash(rel), string(text))
		page := map[string]string{} // file= で覚えたファイル
		for i, b := range blocks {
			if b.lang != "fc" {
				continue
			}
			var next *codeBlock // 出力の ```text (次の ```fc より前の最初のもの。間の ```bash などは飛ばす)
			for _, n := range blocks[i+1:] {
				if n.lang == "fc" {
					break
				}
				if n.lang == "text" {
					next = n
					break
				}
			}
			mode, file := b.mode()
			if file != "" {
				if prev, ok := page[file]; ok {
					page[file] = prev + "\n" + b.code
				} else {
					page[file] = b.code
				}
			}
			files := map[string]string{}
			for k, v := range page {
				files[k] = v
			}
			b := b
			t.Run(b.name(), func(t *testing.T) {
				t.Parallel()
				checkBlock(t, root, b, next, mode, file, files)
			})
		}
	}
}

// mode は印 (= を含まない最初の語) と file= の値。
func (b *codeBlock) mode() (mode, file string) {
	for _, a := range b.attrs {
		if k, v, ok := strings.Cut(a, "="); ok {
			if k == "file" {
				file = v
			}
		} else if mode == "" {
			mode = a
		}
	}
	return mode, file
}

func checkBlock(t *testing.T, root string, b, next *codeBlock, mode, file string, files map[string]string) {
	entry := file
	if entry == "" {
		entry = "t.fc"
		files[entry] = b.code
	}
	src := files[entry] // 入口のファイル (file= で足した分を含む)
	if !strings.HasPrefix(src, "#fc") && mode != "ignore" && mode != "error" && mode != "" {
		t.Fatalf("%s: ```fc %s のブロック (か file= のファイル) は #fc 4 から始まる 1 つのプログラムにする", b.name(), mode)
	}
	fragment := b.code
	if !strings.HasPrefix(fragment, "#fc") {
		fragment = "#fc 4\n" + fragment
	}
	switch mode {
	case "ignore":
		return
	case "":
		checkFormat(t, b, fragment)
	case "run":
		checkFormat(t, b, fragment)
		if next == nil {
			t.Fatalf("%s: ```fc run の後に出力の ```text のブロックが要る", b.name())
		}
		r := build(t, root, files, "emu", true, entry)
		if r.err != nil {
			t.Fatalf("%s: %v", b.name(), r.err)
		}
		noWarnings(t, b, r)
		if r.res.ExitCode != 0 {
			t.Errorf("%s: 終了コード %d", b.name(), r.res.ExitCode)
		}
		if r.stdout != next.code {
			t.Errorf("%s: 出力が %s の ```text と違う\ngot:\n%s\nwant:\n%s", b.name(), next.name(), r.stdout, next.code)
		}
	case "test":
		checkFormat(t, b, fragment)
		const runner = "doccheck_main.fc"
		files[runner] = "#fc 4\nuse " + strings.TrimSuffix(entry, ".fc") + ";\nfunction main():void { @run_tests(); }\n"
		r := build(t, root, files, "emu", true, runner)
		if r.err != nil {
			t.Fatalf("%s: %v", b.name(), r.err)
		}
		noWarnings(t, b, r)
		if r.res.ExitCode != 0 {
			t.Errorf("%s: テストが通らない (終了コード %d)\n%s", b.name(), r.res.ExitCode, r.stdout)
		}
		if next != nil && r.stdout != next.code {
			t.Errorf("%s: 出力が %s の ```text と違う\ngot:\n%s\nwant:\n%s", b.name(), next.name(), r.stdout, next.code)
		}
	case "nes":
		checkFormat(t, b, fragment)
		r := build(t, root, files, "nes", false, entry)
		if r.err != nil {
			t.Fatalf("%s: %v", b.name(), r.err)
		}
		noWarnings(t, b, r)
	case "error":
		r := build(t, root, files, "emu", false, entry)
		if r.err == nil {
			t.Fatalf("%s: ```fc error なのにエラーにならない", b.name())
		}
		// ドキュメントには利用者が見る形 (`over.fc:7:2: error: ...`) で書くので、行の頭のファイル名はビルドした入口の名前と読み替える
		if want := fcFileRe.ReplaceAllString(strings.TrimSpace(next.codeOrEmpty()), entry+":"); !strings.Contains(cliErrors(r.err), want) {
			t.Errorf("%s: エラーの文言が %s と違う\ngot:  %s\nwant: %s", b.name(), next.name(), cliErrors(r.err), want)
		}
	default:
		t.Fatalf("%s: 知らない印 %q (run / test / nes / error / ignore)", b.name(), mode)
	}
}

// checkFormat は src が fcc fmt の書式どおりかを見る。トップレベルで読めなければ関数の本体として読む (文の断片)。
func checkFormat(t *testing.T, b *codeBlock, src string) {
	t.Helper()
	got, err := syntax.Format([]byte(src), b.file)
	if err == nil {
		if string(got) != src {
			t.Errorf("%s: fcc fmt の書式と違う\ngot:\n%s\nwant (fmt):\n%s", b.name(), src, got)
		}
		return
	}
	body := strings.TrimPrefix(src, "#fc 4\n")
	var w strings.Builder
	w.WriteString("#fc 4\nfunction f():void\n{\n")
	for _, l := range strings.SplitAfter(body, "\n") {
		if strings.TrimSpace(l) != "" {
			w.WriteString("\t")
		}
		w.WriteString(l)
	}
	w.WriteString("}\n")
	wrapped := w.String()
	got2, err2 := syntax.Format([]byte(wrapped), b.file)
	if err2 != nil {
		t.Errorf("%s: 構文エラー: %v", b.name(), err)
		return
	}
	if string(got2) != wrapped {
		t.Errorf("%s: fcc fmt の書式と違う (関数の本体として整形)\ngot:\n%s\nwant (fmt):\n%s", b.name(), wrapped, got2)
	}
}

func noWarnings(t *testing.T, b *codeBlock, r buildResult) {
	t.Helper()
	for _, w := range r.res.Warnings {
		t.Errorf("%s: 警告: %v", b.name(), w)
	}
}

type buildResult struct {
	res    *driver.Result
	stdout string
	err    error
}

// build は files を一時ディレクトリに置いてビルドする (run なら emu で走らせる)。main は入口 (既定 t.fc)。
func build(t *testing.T, root string, files map[string]string, target string, run bool, main ...string) buildResult {
	t.Helper()
	dir := t.TempDir()
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o666); err != nil {
			t.Fatal(err)
		}
	}
	entry := "t.fc"
	if len(main) > 0 {
		entry = main[0]
	}
	out := "a.bin"
	if target == "nes" {
		out = "a.nes"
	}
	for retry := 0; ; retry++ {
		var stdout strings.Builder
		opt := &driver.BuildOptions{Target: target, Dir: dir, BuildDir: filepath.Join(dir, "b"), Out: filepath.Join(dir, out),
			Run: run, Stdout: &stdout, MaxCycles: 50_000_000}
		res, err := driver.NewCompiler(root).BuildContext(t.Context(), entry, opt)
		// ca65 がまれに何も出さずに失敗する (Windows)。internal/driver の testBuild と同じくやり直す
		var ce *driver.CommandError
		if retry < 2 && errors.As(err, &ce) && strings.TrimSpace(ce.Result) == "" {
			continue
		}
		return buildResult{res: res, stdout: stdout.String(), err: err}
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}
