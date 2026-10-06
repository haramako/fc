package cc65

// ld65 のリンカ設定 (.cfg) の MEMORY と SEGMENTS を、バンクの使用量の表示に要る分だけ読む (fcc build --size-report /
// fcc size --cfg)。fcc が書く設定も、自前の設定 (castle の ld65.cfg) も同じに読む。値は `$hex` と 10 進だけ (式は 0)。

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// MemArea は MEMORY の 1 つ。
type MemArea struct {
	Name  string
	Start int
	Size  int
	File  bool // 出力ファイルに置く (file = %O): ROM の領域
}

// LinkConfig は読んだリンカ設定。
type LinkConfig struct {
	Memory []*MemArea        // 書いた順
	Load   map[string]string // セグメント → load の MEMORY
}

// ReadLinkConfig はリンカ設定のファイルを読む。
func ReadLinkConfig(path string) (*LinkConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseLinkConfig(string(data)), nil
}

// ParseLinkConfig はリンカ設定の MEMORY と SEGMENTS を読む (ほかのブロックと読めない項目は飛ばす)。
func ParseLinkConfig(src string) *LinkConfig {
	cfg := &LinkConfig{Load: map[string]string{}}
	var lines []string
	for _, l := range strings.Split(src, "\n") {
		if i := strings.IndexByte(l, '#'); i >= 0 {
			l = l[:i]
		}
		lines = append(lines, l)
	}
	text := strings.Join(lines, "\n")
	for _, block := range []string{"MEMORY", "SEGMENTS"} {
		loc := regexp.MustCompile(`\b` + block + `\s*\{([^}]*)\}`).FindStringSubmatch(text)
		if loc == nil {
			continue
		}
		for _, item := range strings.Split(loc[1], ";") {
			name, attrs, ok := strings.Cut(item, ":")
			name = strings.TrimSpace(name)
			if !ok || name == "" {
				continue
			}
			kv := map[string]string{}
			for _, a := range strings.Split(attrs, ",") {
				if k, v, ok := strings.Cut(a, "="); ok {
					kv[strings.ToLower(strings.TrimSpace(k))] = strings.TrimSpace(v)
				}
			}
			if block == "MEMORY" {
				_, file := kv["file"]
				cfg.Memory = append(cfg.Memory, &MemArea{Name: name, Start: cfgInt(kv["start"]), Size: cfgInt(kv["size"]), File: file && kv["file"] != `""`})
			} else if m := kv["load"]; m != "" {
				cfg.Load[name] = m
			}
		}
	}
	return cfg
}

func cfgInt(s string) int {
	var n int64
	var err error
	if strings.HasPrefix(s, "$") {
		n, err = strconv.ParseInt(s[1:], 16, 32)
	} else {
		n, err = strconv.ParseInt(s, 0, 32)
	}
	if err != nil {
		return 0
	}
	return int(n)
}

// BankReport は ROM の領域 (出力ファイルに置く MEMORY。16 バイト以下のヘッダ・ベクタは除く) ごとの使用量・空きと、置いた
// セグメント (fc のモジュール) の大きさ (バンクとモジュールの組み合わせを考えるときの表)。
func (d *DbgFile) BankReport(cfg *LinkConfig) []string {
	used := map[string]int{}
	segs := map[string][]*DbgSegment{}
	for _, s := range d.Segments {
		if s.Ooffs < 0 || s.Size == 0 {
			continue
		}
		m := cfg.Load[s.Name]
		used[m] += s.Size
		segs[m] = append(segs[m], s)
	}
	r := []string{"banks (ROM areas of the linker config):", fmt.Sprintf("  %-10s %6s %7s %7s %7s %5s  %s", "area", "start", "size", "used", "free", "use", "segments")}
	for _, m := range cfg.Memory {
		if !m.File || m.Size <= 16 {
			continue
		}
		ss := segs[m.Name]
		sort.Slice(ss, func(i, j int) bool {
			if ss[i].Size != ss[j].Size {
				return ss[i].Size > ss[j].Size
			}
			return ss[i].Name < ss[j].Name
		})
		var names []string
		for _, s := range ss {
			names = append(names, fmt.Sprintf("%s %d", s.Name, s.Size))
		}
		u := used[m.Name]
		line := fmt.Sprintf("  %-10s %6s %7d %7d %7d %4d%%  %s", m.Name, fmt.Sprintf("$%04X", m.Start), m.Size, u, m.Size-u, u*100/m.Size, strings.Join(names, ", "))
		r = append(r, strings.TrimRight(line, " "))
	}
	return r
}
