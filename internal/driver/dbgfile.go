package driver

// ld65 の --dbgfile (デバッグ情報) の読み込みと、それから作る生成物:
//   - Mesen 2 / MesenCE 向けのラベルファイル .mlb (ROM と同じ名前で置くと自動で読まれる)
//   - 関数ごとのコードサイズ (fcc build --size-report / fcc size)
// ソース行の対応 (fc の行 → アドレス) は codegen が `.dbg line` で .s に埋め、ld65 が dbgfile の line レコードにする。
// Mesen は ROM と同じ名前の .dbg も自動で読み、fc のソースをステップ実行できる (doc/development_notes.md)。

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// DbgSegment は dbgfile の seg レコード。
type DbgSegment struct {
	ID    int
	Name  string
	Start int // CPU アドレス
	Size  int
	Ooffs int  // 出力ファイル内のオフセット (ROM に置かれないセグメントは -1)
	RO    bool // type=ro
}

// DbgSymbol は dbgfile の sym レコード (ラベルと equ)。
type DbgSymbol struct {
	Name string
	Val  int
	Seg  int // -1 なら絶対値 (equ など)
	Lab  bool
}

// DbgFile は dbgfile の内容 (この用途で要るものだけ)。
type DbgFile struct {
	Segments map[int]*DbgSegment
	Symbols  []DbgSymbol
}

var (
	reDbgField = regexp.MustCompile(`(\w+)=("(?:[^"]*)"|[^,]*)`)
)

func dbgFields(line string) map[string]string {
	r := map[string]string{}
	for _, m := range reDbgField.FindAllStringSubmatch(line, -1) {
		r[m[1]] = strings.Trim(m[2], `"`)
	}
	return r
}

func dbgInt(s string) int {
	if strings.HasPrefix(s, "0x") {
		n, _ := strconv.ParseInt(s[2:], 16, 32)
		return int(n)
	}
	n, _ := strconv.Atoi(s)
	return n
}

// ParseDbgFile は ld65 の --dbgfile を読む。
func ParseDbgFile(path string) (*DbgFile, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	d := &DbgFile{Segments: map[int]*DbgSegment{}}
	for _, line := range strings.Split(string(b), "\n") {
		kind, rest, ok := strings.Cut(strings.TrimSpace(line), "\t")
		if !ok {
			continue
		}
		f := dbgFields(rest)
		switch kind {
		case "seg":
			s := &DbgSegment{ID: dbgInt(f["id"]), Name: f["name"], Start: dbgInt(f["start"]), Size: dbgInt(f["size"]), Ooffs: -1, RO: f["type"] == "ro"}
			if o, ok := f["ooffs"]; ok {
				s.Ooffs = dbgInt(o)
			}
			d.Segments[s.ID] = s
		case "sym":
			if f["type"] == "imp" { // 別モジュールからの import (定義側に同じ名前がある)
				continue
			}
			s := DbgSymbol{Name: f["name"], Val: dbgInt(f["val"]), Seg: -1, Lab: f["type"] == "lab"}
			if seg, ok := f["seg"]; ok {
				s.Seg = dbgInt(seg)
			}
			d.Symbols = append(d.Symbols, s)
		}
	}
	return d, nil
}

// WriteMlb は Mesen 2 のラベルファイルを書く。ROM 上のラベルは PRG ROM のオフセット、RAM 上のラベルは CPU アドレス。
// battery は iNES ヘッダの電池フラグ ($6000-$7FFF を SaveRam と呼ぶか WorkRam と呼ぶか)。
func (d *DbgFile) WriteMlb(path string, battery bool) error {
	var lines []string
	seen := map[string]bool{}
	for _, s := range d.Symbols {
		if !s.Lab || seen[s.Name] || !isMlbLabel(s.Name) {
			continue
		}
		seg := d.Segments[s.Seg]
		var kind string
		var addr int
		switch {
		case seg != nil && seg.Ooffs >= 0:
			kind, addr = "NesPrgRom", seg.Ooffs-16+(s.Val-seg.Start) // iNES ヘッダの 16 バイトを除く
		case s.Val < 0x2000:
			kind, addr = "NesInternalRam", s.Val&0x7ff
		case s.Val >= 0x6000 && s.Val < 0x8000:
			kind, addr = "NesWorkRam", s.Val-0x6000
			if battery {
				kind = "NesSaveRam"
			}
		default:
			kind, addr = "NesMemory", s.Val
		}
		if addr < 0 {
			continue
		}
		seen[s.Name] = true
		lines = append(lines, fmt.Sprintf("%s:%X:%s", kind, addr, s.Name))
	}
	sort.Strings(lines)
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o666)
}

func isMlbLabel(name string) bool {
	if name == "" || strings.HasPrefix(name, "@") || strings.HasPrefix(name, ".") {
		return false
	}
	for _, c := range name {
		if !(c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z') {
			return false
		}
	}
	return true
}

// SizeEntry は関数 (ラベル) ごとのコードサイズ。
type SizeEntry struct {
	Name    string
	Segment string
	Size    int
}

// Sizes は ROM に置かれるセグメントのラベルを、次のラベルまで (最後は セグメントの末尾まで) の大きさで数える。
// fc の関数はモジュール名のセグメントに `_mod_name:` で並ぶので、関数 + その後ろの定数表が 1 つの大きさになる
// (ラベルの無い定数表は直前の関数に含まれる)。`@` のローカルラベルと `__direct` / `__a` / `__frame` (関数の途中の入口) は飛ばす。
func (d *DbgFile) Sizes() []SizeEntry {
	bySeg := map[int][]DbgSymbol{}
	for _, s := range d.Symbols {
		seg := d.Segments[s.Seg]
		if !s.Lab || seg == nil || seg.Ooffs < 0 || !isMlbLabel(s.Name) || strings.HasSuffix(s.Name, "__direct") || strings.HasSuffix(s.Name, "__frame") || strings.HasSuffix(s.Name, "__a") {
			continue
		}
		bySeg[s.Seg] = append(bySeg[s.Seg], s)
	}
	var r []SizeEntry
	for id, syms := range bySeg {
		seg := d.Segments[id]
		sort.Slice(syms, func(i, j int) bool { return syms[i].Val < syms[j].Val })
		for i, s := range syms {
			if i+1 < len(syms) && syms[i+1].Val == s.Val {
				continue // 同じ番地の別名 (options(symbol:) など): 後の名前に任せる
			}
			end := seg.Start + seg.Size
			if i+1 < len(syms) {
				end = syms[i+1].Val
			}
			r = append(r, SizeEntry{Name: s.Name, Segment: seg.Name, Size: end - s.Val})
		}
	}
	sort.Slice(r, func(i, j int) bool {
		if r[i].Size != r[j].Size {
			return r[i].Size > r[j].Size
		}
		return r[i].Name < r[j].Name
	})
	return r
}

// SizeReport は --size-report の表示 (セグメント別の合計と、大きい順の関数の一覧)。
func (d *DbgFile) SizeReport(top int) []string {
	type segTotal struct {
		name string
		size int
	}
	var segs []segTotal
	total := 0
	for _, s := range d.Segments {
		if s.Ooffs >= 0 && s.Size > 0 {
			segs = append(segs, segTotal{s.Name, s.Size})
			total += s.Size
		}
	}
	sort.Slice(segs, func(i, j int) bool { return segs[i].size > segs[j].size })
	var r []string
	r = append(r, fmt.Sprintf("size: %d bytes in ROM", total))
	for _, s := range segs {
		r = append(r, fmt.Sprintf("  %-16s %6d", s.name, s.size))
	}
	sizes := d.Sizes()
	if top > 0 && len(sizes) > top {
		sizes = sizes[:top]
	}
	r = append(r, fmt.Sprintf("functions (largest %d):", len(sizes)))
	for _, e := range sizes {
		r = append(r, fmt.Sprintf("  %-40s %6d  (%s)", e.Name, e.Size, e.Segment))
	}
	return r
}
