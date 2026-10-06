// Package fcdoc は fc のモジュールのドキュメント (fcc doc) を作る。public の宣言と、その直前のコメント (Go の doc comment と
// 同じ形) を集め、端末向けの文字と、利用者向けのサイト (docs/reference/std) の Markdown にする。
//
// 約束:
//   - モジュールの説明は、ファイルの頭 (#fc の行の次) から続く // の行
//   - 宣言の説明は、宣言の直前に空行を挟まずに続く // の行。同じ行の後ろのコメントは宣言と一緒に出す
//   - 説明の中で // の後に 3 つ以上の空白 (かタブ) で始まる行はコードの例 (等幅で出す)
//   - 空行を挟まずに並んだ const / var は 1 つのまとまりとして出す
//   - 長い初期値 (複数行か 60 文字を超える) は … にする
package fcdoc

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/haramako/fc/internal/syntax"
)

// Module は 1 つのモジュールのドキュメント。
type Module struct {
	Name    string // use の名前 (ファイル名から .fc を除いたもの)
	Target  string // "" (どのターゲットでも) / "nes" / "emu"
	Path    string // 表示用のパス (fclib/nes/vram.fc)
	Version int    // #fc の版
	Doc     string // モジュールの説明 (// を除いた行を \n でつないだもの)
	Items   []*Item
}

// Item は public の宣言 (か、並んだ const / var のまとまり) 1 つ。
type Item struct {
	Names []string // 宣言した名前
	Kind  string   // "function" / "const" / "var" / "struct" / "enum" / "soa"
	Code  string   // 宣言の原文 (関数は本体の前まで、同じ行の後ろのコメントを含む)
	Doc   string   // 直前のコメント (// を除いた行を \n でつないだもの)
	Line  int      // 宣言の行
	end   int      // 宣言の最後の行 (まとめるときに使う)
}

// Parse は src (path のファイル) のドキュメントを作る。
func Parse(path string, src []byte) (*Module, error) {
	text := strings.ReplaceAll(string(src), "\r\n", "\n")
	f, err := syntax.Parse([]byte(text), path)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSuffix(filepath.Base(path), ".fc")
	m := &Module{Name: name, Path: filepath.ToSlash(path), Version: f.Version}
	lines := strings.Split(text, "\n")

	// 自分の行にある // のコメント (行 → 本文)
	own := map[int]string{}
	trailing := map[int]bool{} // 宣言の後ろに続くコメントのある行
	for _, c := range f.Comments {
		if !c.IsLine() {
			continue
		}
		if strings.TrimSpace(lines[c.Pos.Line-1][:c.Pos.Col-1]) == "" {
			own[c.Pos.Line] = strings.TrimPrefix(strings.TrimPrefix(c.Text, "//"), " ")
		} else {
			trailing[c.Pos.Line] = true
		}
	}
	// モジュールの説明: 頭から続くコメントの行
	first := 1
	if f.Pragma != "" {
		first = 2
	}
	var head []string
	moduleEnd := first - 1
	for l := first; ; l++ {
		s, ok := own[l]
		if !ok {
			break
		}
		head = append(head, s)
		moduleEnd = l
	}
	m.Doc = strings.Join(head, "\n")

	// 宣言の直前の説明 (モジュールの説明の行は使わない)
	docAbove := func(line int) string {
		var doc []string
		for l := line - 1; l > moduleEnd; l-- {
			s, ok := own[l]
			if !ok {
				break
			}
			doc = append([]string{s}, doc...)
		}
		return strings.Join(doc, "\n")
	}
	src2 := text
	for _, st := range f.Stmts {
		it := item(st, src2)
		if it == nil {
			continue
		}
		it.Doc = docAbove(it.Line)
		// 並んだ const / var は前のまとまりに足す (説明が無く、空行を挟まないもの)
		if n := len(m.Items); n > 0 && it.Doc == "" && isData(it.Kind) {
			prev := m.Items[n-1]
			if isData(prev.Kind) && prev.end+1 == it.Line {
				prev.Names = append(prev.Names, it.Names...)
				prev.Code += "\n" + it.Code
				if prev.Kind != it.Kind {
					prev.Kind = "var"
				}
				prev.end = it.end
				continue
			}
		}
		m.Items = append(m.Items, it)
	}
	return m, nil
}

func isData(kind string) bool { return kind == "const" || kind == "var" }

// item は public の宣言の Item (public でなければ nil)。
func item(st syntax.Stmt, src string) *Item {
	var it *Item
	var start, end int // 原文の範囲
	switch d := st.(type) {
	case *syntax.FuncDecl:
		if !d.PublicPos.IsValid() {
			return nil
		}
		name := d.Name.Name
		if d.Recv != nil {
			name = d.Recv.Name + "." + name // メソッド
		}
		it = &Item{Names: []string{name}, Kind: "function"}
		start = d.Pos().Offset
		if d.Body != nil {
			end = d.Body.Lbrace.Offset
		} else {
			end = d.Semi.Offset + 1
		}
		it.Code = strings.TrimSpace(src[start:end])
		it.Line, it.end = d.Pos().Line, d.End().Line
		return it
	case *syntax.VarDecl:
		if !d.PublicPos.IsValid() {
			return nil
		}
		kind := "var"
		if d.Const {
			kind = "const"
		}
		it = &Item{Kind: kind}
		for _, sp := range d.Specs {
			it.Names = append(it.Names, sp.Name.Name)
		}
		start, end = d.Pos().Offset, d.End().Offset
		code := src[start:lineEnd(src, end)]
		// 長い初期値は … に (後ろから置き換える)
		for i := len(d.Specs) - 1; i >= 0; i-- {
			sp := d.Specs[i]
			if sp.Init == nil {
				continue
			}
			a, b := sp.Init.Pos().Offset-start, sp.Init.End().Offset-start
			if init := code[a:b]; strings.Contains(init, "\n") || utf8.RuneCountInString(init) > 60 {
				code = code[:a] + "…" + code[b:]
			}
		}
		it.Code = strings.TrimRight(code, " \t")
		it.Line, it.end = d.Pos().Line, d.End().Line
		return it
	case *syntax.StructDecl:
		if !d.PublicPos.IsValid() {
			return nil
		}
		it = &Item{Names: []string{d.Name.Name}, Kind: "struct"}
		start, end = d.Pos().Offset, d.End().Offset
		it.Line, it.end = d.Pos().Line, d.End().Line
	case *syntax.EnumDecl:
		if !d.PublicPos.IsValid() {
			return nil
		}
		it = &Item{Names: []string{d.Name.Name}, Kind: "enum"}
		start, end = d.Pos().Offset, d.End().Offset
		it.Line, it.end = d.Pos().Line, d.End().Line
	case *syntax.SoaDecl:
		if !d.PublicPos.IsValid() {
			return nil
		}
		it = &Item{Names: []string{d.Name.Name}, Kind: "soa"}
		start, end = d.Pos().Offset, d.End().Offset
		it.Line, it.end = d.Pos().Line, d.End().Line
	default:
		return nil
	}
	it.Code = strings.TrimRight(src[start:lineEnd(src, end)], " \t")
	return it
}

// lineEnd は off を含む行の終わり (改行の位置)。
func lineEnd(src string, off int) int {
	if off > len(src) {
		return len(src)
	}
	if i := strings.IndexByte(src[off:], '\n'); i >= 0 {
		return off + i
	}
	return len(src)
}

// Summary はモジュールの説明の最初の文 (「。」まで)。
func (m *Module) Summary() string {
	s := joinLines(strings.Split(m.Doc, "\n"))
	depth := 0 // 括弧の中の「。」では切らない
	for i, r := range s {
		switch r {
		case '(', '（':
			depth++
		case ')', '）':
			depth--
		case '。':
			if depth <= 0 {
				return s[:i+len("。")]
			}
		}
	}
	return strings.TrimSpace(s)
}

// Heading は Item の見出し (名前。まとまりなら並べて、長ければ … で切る)。
func (it *Item) Heading() string {
	h := strings.Join(it.Names, ", ")
	if utf8.RuneCountInString(h) > 40 {
		var b strings.Builder
		for i, n := range it.Names {
			if b.Len()+len(n) > 36 {
				b.WriteString(", …")
				break
			}
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(n)
		}
		h = b.String()
	}
	return h
}

// Find は名前 name の Item (無ければ nil)。
func (m *Module) Find(name string) *Item {
	for _, it := range m.Items {
		for _, n := range it.Names {
			if n == name {
				return it
			}
		}
	}
	return nil
}

// TargetLabel はターゲットの表示名。
func TargetLabel(target string) string {
	switch target {
	case "nes":
		return "NES"
	case "emu":
		return "emu"
	}
	return "どのターゲットでも"
}

// Text は端末向けの文字 (fcc doc)。
func (m *Module) Text() string {
	var b strings.Builder
	fmt.Fprintf(&b, "module %s (%s)\n", m.Name, m.Path)
	if m.Doc != "" {
		b.WriteString("\n" + indent(m.Doc, "") + "\n")
	}
	for _, it := range m.Items {
		b.WriteString("\n" + it.Text())
	}
	return b.String()
}

// Text は Item 1 つの端末向けの文字。
func (it *Item) Text() string {
	var b strings.Builder
	b.WriteString(it.Code + "\n")
	if it.Doc != "" {
		b.WriteString(indent(it.Doc, "    ") + "\n")
	}
	return b.String()
}

func indent(s, pre string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if l != "" {
			lines[i] = pre + l
		}
	}
	return strings.Join(lines, "\n")
}

// Markdown はサイトのページ (VitePress)。source はソースへのリンク (空なら出さない)。
func (m *Module) Markdown(source string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", m.Name)
	// 説明の中の {{ }} を Vue が式として読まないように v-pre で囲む
	b.WriteString("::: v-pre\n\n")
	fmt.Fprintf(&b, "`use %s;` ・ ターゲット: %s", m.Name, TargetLabel(m.Target))
	if source != "" {
		fmt.Fprintf(&b, " ・ ソース: [%s](%s)", m.Path, source)
	}
	b.WriteString("\n\n")
	if m.Doc != "" {
		b.WriteString(docMarkdown(m.Doc) + "\n")
	}
	for _, it := range m.Items {
		fmt.Fprintf(&b, "## %s\n\n", it.Heading())
		fmt.Fprintf(&b, "```fc\n%s\n```\n\n", it.Code)
		if it.Doc != "" {
			b.WriteString(docMarkdown(it.Doc) + "\n")
		}
	}
	b.WriteString(":::\n")
	return b.String()
}

// docMarkdown はコメントの本文を Markdown にする。空行は段落の区切り、3 つ以上の空白かタブで始まる行はコードの例。
func docMarkdown(doc string) string {
	var b strings.Builder
	var para, code []string
	flushPara := func() {
		if len(para) > 0 {
			b.WriteString(escape(joinLines(para)) + "\n\n")
			para = nil
		}
	}
	flushCode := func() {
		if len(code) > 0 {
			b.WriteString("```fc\n" + strings.Join(dedent(code), "\n") + "\n```\n\n")
			code = nil
		}
	}
	for _, l := range strings.Split(doc, "\n") {
		switch {
		case strings.TrimSpace(l) == "":
			flushPara()
			flushCode()
		case strings.HasPrefix(l, "  ") || strings.HasPrefix(l, "\t"):
			flushPara()
			code = append(code, l)
		default:
			flushCode()
			para = append(para, strings.TrimSpace(l))
		}
	}
	flushPara()
	flushCode()
	return strings.TrimRight(b.String(), "\n") + "\n"
}

// joinLines は段落の行をつなぐ。日本語どうしの間には空白を入れず、英数字との間には入れる (「、」「。」などの約物の隣には入れない)。
func joinLines(lines []string) string {
	var b strings.Builder
	for i, l := range lines {
		if i > 0 {
			prev, _ := utf8.DecodeLastRuneInString(lines[i-1])
			next, _ := utf8.DecodeRuneInString(l)
			wide := func(r rune) bool { return r >= utf8.RuneSelf }
			if !(wide(prev) && wide(next)) && !isPunct(prev) && !isPunct(next) {
				b.WriteByte(' ')
			}
		}
		b.WriteString(l)
	}
	return b.String()
}

// isPunct は隣に空白を入れない日本語の約物か。
func isPunct(r rune) bool {
	return strings.ContainsRune("、。，．・「」『』（）【】：；！？〜", r)
}

// escape は `…` の外の < > を文字参照にする (VitePress が HTML のタグとして読まないように)。
func escape(s string) string {
	parts := strings.Split(s, "`")
	for i := 0; i < len(parts); i += 2 {
		parts[i] = strings.NewReplacer("<", "&lt;", ">", "&gt;").Replace(parts[i])
	}
	return strings.Join(parts, "`")
}

// dedent は行に共通の頭の空白を除く。
func dedent(lines []string) []string {
	min := -1
	for _, l := range lines {
		n := len(l) - len(strings.TrimLeft(l, " \t"))
		if min < 0 || n < min {
			min = n
		}
	}
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = l[min:]
	}
	return out
}

// Index は標準ライブラリの一覧のページ。link はモジュールのページへのリンク。
func Index(mods []*Module, link func(*Module) string) string {
	var b strings.Builder
	b.WriteString("# 標準ライブラリ\n\n")
	b.WriteString("fc に付いてくるモジュールです。`use 名前;` で使えます。このページは標準ライブラリのソースのコメントから `fcc doc` が作っています" +
		"（端末では `fcc doc 名前` で同じものを見られます）。\n\n")
	b.WriteString("::: v-pre\n\n")
	for _, target := range []string{"", "nes", "emu"} {
		var ms []*Module
		for _, m := range mods {
			if m.Target == target {
				ms = append(ms, m)
			}
		}
		if len(ms) == 0 {
			continue
		}
		sort.Slice(ms, func(i, j int) bool { return ms[i].Name < ms[j].Name })
		switch target {
		case "":
			b.WriteString("## どのターゲットでも使えるもの\n\n")
		case "nes":
			b.WriteString("## NES\n\n")
		case "emu":
			b.WriteString("## emu\n\n")
		}
		b.WriteString("| モジュール | 内容 |\n|---|---|\n")
		for _, m := range ms {
			fmt.Fprintf(&b, "| [%s](%s) | %s |\n", m.Name, link(m), strings.ReplaceAll(escape(m.Summary()), "|", "\\|"))
		}
		b.WriteString("\n")
	}
	b.WriteString(":::\n")
	return b.String()
}
