// Package sizehtml はビルドしたプログラムの大きさを 1 つの HTML ファイルにする (fcc build --size-html / fcc size --html)。
// --size-report と同じ情報 (ROM の領域 (バンク) ごとの使用量と置いたモジュール、モジュールの間の呼び出しの数、関数の大きさ) を、
// バンクの帯と並べ替え・絞り込みのできる表で見せる。データは JSON で埋め込み、外のファイル・ライブラリを読まない (オフラインで開ける)。
package sizehtml

import (
	_ "embed"
	"encoding/json"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/haramako/fc/internal/cc65"
)

// Report は HTML に埋め込むデータ。
type Report struct {
	Title     string `json:"title"`
	Total     int    `json:"total"` // ROM に置くセグメントの合計
	Areas     []Area `json:"areas"`
	Functions []Func `json:"functions"`
	Calls     []Call `json:"calls"` // 無ければ空 (fcc size はビルドの中身を知らない)
	HasCalls  bool   `json:"hasCalls"`
}

// Area はリンカ設定の ROM の領域 (バンク)。
type Area struct {
	Name     string `json:"name"`
	Start    int    `json:"start"`
	Size     int    `json:"size"`
	Used     int    `json:"used"`
	Segments []Seg  `json:"segments"`
}

// Seg はセグメント (fc のモジュール) と大きさ。
type Seg struct {
	Name string `json:"name"`
	Size int    `json:"size"`
}

// Func は関数 (ラベル) の大きさ。
type Func struct {
	Name    string `json:"name"`
	Segment string `json:"segment"`
	Size    int    `json:"size"`
}

// Call はモジュールの組の呼び出しの数。
type Call struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Count int    `json:"count"`
	Far   int    `json:"far"`
}

// FromDbg は dbgfile とリンカ設定 (nil ならリンカ設定の無い表: セグメントを 1 つの領域に) からデータを作る。
func FromDbg(title string, d *cc65.DbgFile, lc *cc65.LinkConfig) *Report {
	r := &Report{Title: title}
	segs := map[string][]Seg{}
	used := map[string]int{}
	var all []Seg
	for _, s := range d.Segments {
		if s.Ooffs < 0 || s.Size == 0 {
			continue
		}
		r.Total += s.Size
		area := "ROM"
		if lc != nil {
			area = lc.Load[s.Name]
		}
		segs[area] = append(segs[area], Seg{s.Name, s.Size})
		used[area] += s.Size
		all = append(all, Seg{s.Name, s.Size})
	}
	bySize := func(ss []Seg) {
		sort.Slice(ss, func(i, j int) bool {
			if ss[i].Size != ss[j].Size {
				return ss[i].Size > ss[j].Size
			}
			return ss[i].Name < ss[j].Name
		})
	}
	if lc != nil {
		for _, m := range lc.Memory {
			if !m.File || m.Size <= 16 {
				continue
			}
			ss := segs[m.Name]
			if ss == nil {
				ss = []Seg{} // 空きのバンク (JSON で null にしない)
			}
			bySize(ss)
			r.Areas = append(r.Areas, Area{Name: m.Name, Start: m.Start, Size: m.Size, Used: used[m.Name], Segments: ss})
		}
	} else {
		bySize(all)
		r.Areas = []Area{{Name: "ROM", Size: r.Total, Used: r.Total, Segments: all}}
	}
	for _, e := range d.Sizes() {
		r.Functions = append(r.Functions, Func{Name: e.Name, Segment: e.Segment, Size: e.Size})
	}
	return r
}

//go:embed page.html
var page string

// Write は r を埋め込んだ HTML を書く。
func Write(w io.Writer, r *Report) error {
	if r.Calls == nil {
		r.Calls = []Call{}
	}
	if r.Functions == nil {
		r.Functions = []Func{}
	}
	data, err := json.Marshal(r) // json は < > & を \u003c などにするので、名前に </script> があっても切れない
	if err != nil {
		return err
	}
	html := strings.Replace(page, "/*DATA*/null", string(data), 1)
	html = strings.ReplaceAll(html, "<!--TITLE-->", htmlEscape(r.Title))
	_, err = io.WriteString(w, html)
	return err
}

func htmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(s)
}

// WriteFile は r を埋め込んだ HTML をファイル path に書く。
func WriteFile(path string, r *Report) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := Write(f, r); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
