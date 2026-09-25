package driver

// fc 3 の @log (doc/v3_plan.md §9) のリンク後の処理: codegen の地点 (LogSite) のラベルと番地の式を dbgfile で値にして、
//   - ROM の隣に <rom>.fclog.json (地点・書式・値の所在) と Mesen 2 用の <rom>.fclog.lua を書く
//   - emu ターゲットの実行 (fcc run、テスト) では、地点の PC に来たら値を読んで表示する (stdout に printf と同じ順で出る)
// どちらも NES 側の命令は増やさない (地点はラベル、値はメモリとレジスタから読むだけ)。

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/haramako/fc/internal/codegen"
	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/types"
)

// LogFile は <rom>.fclog.json の内容。
type LogFile struct {
	Version int            `json:"version"`
	Target  string         `json:"target"` // nes / emu
	Points  []LogFilePoint `json:"points"`
}

// LogFilePoint はソースの @log 1 つ。
type LogFilePoint struct {
	ID     int           `json:"id"`
	File   string        `json:"file"`
	Line   int           `json:"line"`
	Format string        `json:"format"`
	Parts  []LogFilePart `json:"parts"`
	Args   []LogFileArg  `json:"args"`
	Sites  []LogFileSite `json:"sites"`
}

// LogFilePart は書式の 1 片 (Arg < 0 なら Text)。
type LogFilePart struct {
	Text  string `json:"text,omitempty"`
	Arg   int    `json:"arg"`
	Verb  string `json:"verb,omitempty"` // "" / d / x / X / b / c
	Width int    `json:"width,omitempty"`
	Zero  bool   `json:"zero,omitempty"`
}

// LogFileArg は引数の表示の仕方。
type LogFileArg struct {
	Expr   string         `json:"expr"`
	Type   string         `json:"type"`
	Kind   string         `json:"kind"` // int / bool / enum / ptr
	Size   int            `json:"size"`
	Signed bool           `json:"signed,omitempty"`
	Enum   map[int]string `json:"enum,omitempty"`
}

// LogFileSite は地点 1 つ (実行アドレスと、そこでの値の所在)。
type LogFileSite struct {
	Func string `json:"func"`
	Seq  int    `json:"seq"` // 地点の通し番号 (同じ PC の @log はこの順に出す = ソースの順)
	PC   int    `json:"pc"`  // CPU アドレス
	Prg  int    `json:"prg"` // NES: PRG ROM 上のオフセット (バンクの照合用)。emu は -1
	// Prevs は地点が合流点のとき、この地点の経路の直前の命令の CPU アドレス。直前にそのどれかを実行したときだけ出す
	// (nil なら毎回。空なら出さない)
	Prevs  []int          `json:"prevs"`
	Values []LogFileValue `json:"values"`
}

// LogFileValue は引数 1 つの所在。
type LogFileValue struct {
	Loc   string         `json:"loc"` // const / mem / stack (Addr + X) / reg / bytes (Bytes を下位から) / none
	Addr  int            `json:"addr,omitempty"`
	Reg   string         `json:"reg,omitempty"`
	Value int            `json:"value,omitempty"`
	Why   string         `json:"why,omitempty"`
	Bytes []LogFileValue `json:"bytes,omitempty"`
}

// buildLogFile は地点のラベルと番地を dbgfile で解決する。dir は ROM のある場所 (ソースのパスをそこからの相対にする)。
func buildLogFile(sites []*codegen.LogSite, dbg *DbgFile, target string, rel func(string) string) (*LogFile, error) {
	syms := map[string]DbgSymbol{}
	for _, s := range dbg.Symbols {
		if strings.HasPrefix(s.Name, "__fclog_") {
			syms[s.Name] = s
		}
	}
	lf := &LogFile{Version: 1, Target: target}
	byID := map[int]*LogFilePoint{}
	var points []*LogFilePoint
	for seq, s := range sites {
		p := byID[s.Point.ID]
		if p == nil {
			fp := logFilePoint(s.Point, rel)
			p = &fp
			points = append(points, p)
			byID[s.Point.ID] = p
		}
		lab, ok := syms[s.Label]
		if !ok {
			continue // 出力されなかった関数 (どこからも呼ばれない)
		}
		site := LogFileSite{Func: s.Func, Seq: seq, PC: lab.Val, Prg: -1}
		if s.Prevs != nil {
			site.Prevs = []int{}
			for _, name := range s.Prevs {
				if pl, ok := syms[name]; ok {
					site.Prevs = append(site.Prevs, pl.Val)
				}
			}
		}
		if seg := dbg.Segments[lab.Seg]; seg != nil && seg.Ooffs >= 0 && target == "nes" {
			site.Prg = seg.Ooffs + lab.Val - seg.Start - 16 // iNES ヘッダの 16 バイト
		}
		var resolve func(loc codegen.LogLoc) (LogFileValue, error)
		resolve = func(loc codegen.LogLoc) (LogFileValue, error) {
			v := LogFileValue{Loc: loc.Kind, Reg: loc.Reg, Value: loc.Value, Why: loc.Why}
			if loc.Sym != "" {
				sym, ok := syms[loc.Sym]
				if !ok {
					return v, fmt.Errorf("@log: symbol %s not found in the debug file", loc.Sym)
				}
				v.Addr = sym.Val
				if loc.Kind == "addr" {
					v.Loc, v.Value, v.Addr = "const", sym.Val, 0
				}
			}
			for _, p := range loc.Parts {
				pv, err := resolve(p)
				if err != nil {
					return v, err
				}
				v.Bytes = append(v.Bytes, pv)
			}
			return v, nil
		}
		for _, loc := range s.Locs {
			v, err := resolve(loc)
			if err != nil {
				return nil, err
			}
			site.Values = append(site.Values, v)
		}
		p.Sites = append(p.Sites, site)
	}
	// ID 順・PC 順にそろえる (json の差分が安定するように)
	sort.Slice(points, func(i, j int) bool { return points[i].ID < points[j].ID })
	for _, p := range points {
		sort.SliceStable(p.Sites, func(a, b int) bool { return p.Sites[a].PC < p.Sites[b].PC })
		lf.Points = append(lf.Points, *p)
	}
	return lf, nil
}

func logFilePoint(p *ir.LogPoint, rel func(string) string) LogFilePoint {
	fp := LogFilePoint{ID: p.ID, File: rel(p.Pos.Filename), Line: p.Pos.Line, Format: p.Format}
	for _, part := range p.Parts {
		lp := LogFilePart{Text: part.Text, Arg: part.Arg, Width: part.Spec.Width, Zero: part.Spec.Zero}
		if part.Spec.Verb != 0 {
			lp.Verb = string(part.Spec.Verb)
		}
		fp.Parts = append(fp.Parts, lp)
	}
	for _, a := range p.Args {
		fp.Args = append(fp.Args, logFileArg(a))
	}
	return fp
}

func logFileArg(a *ir.LogArg) LogFileArg {
	t := a.Type
	r := LogFileArg{Expr: a.Expr, Type: t.String(), Size: t.Size, Signed: t.Signed}
	switch {
	case t.Enum != nil:
		r.Kind = "enum"
		r.Enum = map[int]string{}
		for _, m := range t.Enum.Members {
			if _, dup := r.Enum[m.Value]; !dup {
				r.Enum[m.Value] = m.Name
			}
		}
	case t.Kind == types.Bool:
		r.Kind = "bool"
	case t.Kind == types.Pointer || t.Kind == types.Func:
		r.Kind, r.Size = "ptr", 2
	default:
		r.Kind = "int"
	}
	return r
}

// logWarnings は値が取れない地点の警告 (同じ @log の同じ引数は 1 回)。
func logWarnings(sites []*codegen.LogSite) []diag.Warning {
	var ws []diag.Warning
	seen := map[string]bool{}
	for _, s := range sites {
		for k, loc := range s.Locs {
			if loc.Kind == "bytes" {
				for _, p := range loc.Parts {
					if p.Kind == "none" {
						loc = p
					}
				}
			}
			if loc.Kind != "none" {
				continue
			}
			key := fmt.Sprintf("%d/%d", s.Point.ID, k)
			if seen[key] {
				continue
			}
			seen[key] = true
			ws = append(ws, diag.Warning{Msg: fmt.Sprintf("@log: %s is not available here (%s); it is shown as ?", s.Point.Args[k].Expr, loc.Why), Pos: s.Point.Pos})
		}
	}
	return ws
}

// writeLogFiles は <base>.fclog.json と <base>.fclog.lua を書く。
func writeLogFiles(base string, lf *LogFile) error {
	b, err := json.MarshalIndent(lf, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(base+".fclog.json", append(b, '\n'), 0o666); err != nil {
		return err
	}
	return os.WriteFile(base+".fclog.lua", []byte(mesenLogScript(lf)), 0o666)
}

// logReader は実行中の値の読み出し (emu の実行、テスト)。
type logReader struct {
	mem     func(addr int) int
	a, x, y int
}

// formatLog は地点 site での @log の 1 行。
func formatLog(p *LogFilePoint, site *LogFileSite, r logReader) string {
	var b strings.Builder
	for _, part := range p.Parts {
		if part.Arg < 0 {
			b.WriteString(part.Text)
			continue
		}
		v, ok := readLogValue(site.Values[part.Arg], p.Args[part.Arg].Size, r)
		if !ok {
			b.WriteString("?")
			continue
		}
		b.WriteString(formatLogValue(p.Args[part.Arg], part, v))
	}
	return b.String()
}

func readLogValue(v LogFileValue, size int, r logReader) (int, bool) {
	switch v.Loc {
	case "const":
		return v.Value, true
	case "reg":
		switch v.Reg {
		case "a":
			return r.a, true
		case "x":
			return r.x, true
		case "y":
			return r.y, true
		}
	case "bytes":
		n := 0
		for i, b := range v.Bytes {
			x, ok := readLogValue(b, 1, r)
			if !ok {
				return 0, false
			}
			n |= (x & 0xff) << (8 * i)
		}
		return n, true
	case "mem", "stack":
		addr := v.Addr
		if v.Loc == "stack" {
			addr = (addr + r.x) & 0xff // ゼロページのスタックフレーム (S + X)
		}
		n := 0
		for i := 0; i < size; i++ {
			n |= r.mem((addr+i)&0xffff) << (8 * i)
		}
		return n, true
	}
	return 0, false
}

// formatLogValue は値 1 つを書式どおりに。Lua 版 (mesenLogScript) と同じ規則。
func formatLogValue(a LogFileArg, part LogFilePart, v int) string {
	bits := 8 * a.Size
	v &= 1<<bits - 1
	signedV := v
	if a.Signed && v >= 1<<(bits-1) {
		signedV = v - 1<<bits
	}
	var s string
	switch part.Verb {
	case "x":
		s = strconv.FormatInt(int64(v), 16)
	case "X":
		s = strings.ToUpper(strconv.FormatInt(int64(v), 16))
	case "b":
		s = strconv.FormatInt(int64(v), 2)
	case "c":
		s = string(rune(v))
	case "d":
		s = strconv.Itoa(signedV)
	default:
		switch a.Kind {
		case "bool":
			s = "false"
			if v != 0 {
				s = "true"
			}
		case "enum":
			if name, ok := a.Enum[signedV]; ok {
				s = name
			} else {
				s = strconv.Itoa(signedV)
			}
		case "ptr":
			s = fmt.Sprintf("$%04X", v)
		default:
			s = strconv.Itoa(signedV)
		}
	}
	if part.Width > len(s) {
		pad := " "
		if part.Zero {
			pad = "0"
		}
		if part.Zero && strings.HasPrefix(s, "-") {
			s = "-" + strings.Repeat(pad, part.Width-len(s)) + s[1:]
		} else {
			s = strings.Repeat(pad, part.Width-len(s)) + s
		}
	}
	return s
}

// logHooks は emu の実行用に、PC → (地点, @log) の表を作る。
type logHook struct {
	point *LogFilePoint
	site  *LogFileSite
}

func logHooks(lf *LogFile) map[int][]logHook {
	if lf == nil {
		return nil
	}
	h := map[int][]logHook{}
	for i := range lf.Points {
		p := &lf.Points[i]
		for j := range p.Sites {
			s := &p.Sites[j]
			h[s.PC] = append(h[s.PC], logHook{p, s})
		}
	}
	// 同じ PC の地点は注釈の順 (ソースの順)
	for pc := range h {
		sort.SliceStable(h[pc], func(a, b int) bool { return h[pc][a].site.Seq < h[pc][b].site.Seq })
	}
	return h
}
