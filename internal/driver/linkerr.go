package driver

// ld65 の失敗を fc の言葉にする (Agent/discussions/2026-10-08-debug-games-findings.md)。区画があふれたときは、ld65 の設定 (ld65.cfg) から
// 区画の番地と大きさ、失敗しても ld65 が書く .map からモジュールごとの量を読んで、大きい順に示す。

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/syntax"
)

var (
	ldOverflow = regexp.MustCompile(`Segment\s+\S?([A-Za-z0-9_]+)\S?\s+overflows memory area\s+\S?([A-Za-z0-9_]+)\S?\s+by (\d+) bytes?`)
	ldMemory   = regexp.MustCompile(`(?m)^\s*([A-Za-z0-9_]+):\s*start\s*=\s*\$([0-9A-Fa-f]+),\s*size\s*=\s*\$([0-9A-Fa-f]+)`)
	ldSegment  = regexp.MustCompile(`(?m)^\s*([A-Za-z0-9_]+):\s*load\s*=\s*([A-Za-z0-9_]+)`)
	mapModule  = regexp.MustCompile(`^(\S+):$`)
	mapSegSize = regexp.MustCompile(`^\s+(\S+)\s+Offs=[0-9A-F]+\s+Size=([0-9A-F]+)`)
)

// areaUse は区画を使うモジュール 1 つ (.map のオブジェクトの名前と量)。
type areaUse struct {
	name string
	size int
}

// linkError は ld65 の失敗 err を、分かるものは説明を付けたエラーにする。ほかは「リンクで失敗」を頭に付けたまま返す。
func (c *compilation) linkError(err error, cfgPath, mapFile string) error {
	ce, ok := err.(*CommandError)
	if !ok {
		return err
	}
	m := ldOverflow.FindStringSubmatch(ce.Result)
	if m == nil {
		ce.Msg = "link failed: " + ce.Msg
		return ce
	}
	seg, area := m[1], m[2]
	over, _ := strconv.Atoi(m[3])
	cfg, _ := os.ReadFile(cfgPath)
	start, size := -1, -1
	for _, mm := range ldMemory.FindAllStringSubmatch(string(cfg), -1) {
		if mm[1] == area {
			s, _ := strconv.ParseInt(mm[2], 16, 32)
			n, _ := strconv.ParseInt(mm[3], 16, 32)
			start, size = int(s), int(n)
		}
	}
	segs := map[string]bool{seg: true}
	for _, sm := range ldSegment.FindAllStringSubmatch(string(cfg), -1) {
		if sm[2] == area {
			segs[sm[1]] = true
		}
	}
	users := areaUsers(mapFile, segs)
	var b strings.Builder
	where := area
	if start >= 0 {
		where = fmt.Sprintf("%s ($%04X-$%04X, %d bytes)", area, start, start+size-1, size)
	}
	kind := "the data"
	switch {
	case segs["BSS"] || segs["ZEROPAGE"] || segs["FC_ZEROPAGE"]:
		kind = "the variables"
	case segs["CODE"] || segs["FC_RUNTIME"]:
		kind = "the code and data"
	}
	fmt.Fprintf(&b, "%s do not fit in %s: %d bytes over", kind, where, over)
	if len(users) > 0 {
		b.WriteString(" (used by")
		for i, u := range users {
			if i == 8 {
				fmt.Fprintf(&b, ", … %d more", len(users)-i)
				break
			}
			sep := ","
			if i == 0 {
				sep = ""
			}
			fmt.Fprintf(&b, "%s %s %d", sep, c.describeObject(u.name, area), u.size)
		}
		b.WriteString(")")
	}
	switch {
	case segs["BSS"]:
		b.WriteString(". Make large arrays smaller, or put some variables in other RAM (fc.toml [ram.NAME] and @(segment: \"NAME\") on the variables)")
		if _, fixed := c.prog.Options.Int("static_ram"); fixed {
			b.WriteString(", or lower @(static_ram: N)")
		}
	case segs["FC_ZEROPAGE"] || segs["ZEROPAGE"]:
		b.WriteString(". Lower @(static_zp: N) or @(fastcall_reg: N), or move variables out of the zero page")
	default:
		b.WriteString(". Make the code or data smaller, or move modules to another bank (@(bank: …); fc.toml [target] prg / mapper for more banks)")
	}
	fmt.Fprintf(&b, " [ld65: segment %s overflows memory area %s]", seg, area)
	return &diag.Error{Msg: b.String(), Pos: syntax.Position{Filename: c.mainFile}}
}

// areaUsers は .map の "Modules list" から、segs のセグメントの量をオブジェクトごとに足して大きい順に返す (読めなければ nil)。
func areaUsers(mapFile string, segs map[string]bool) []areaUse {
	data, err := os.ReadFile(mapFile)
	if err != nil {
		return nil
	}
	sizes := map[string]int{}
	var order []string
	cur := ""
	for _, line := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		if strings.HasPrefix(line, "Segment list:") {
			break
		}
		if mm := mapModule.FindStringSubmatch(line); mm != nil {
			cur = mm[1]
			continue
		}
		if mm := mapSegSize.FindStringSubmatch(line); mm != nil && cur != "" && segs[mm[1]] {
			n, _ := strconv.ParseInt(mm[2], 16, 32)
			if _, seen := sizes[cur]; !seen {
				order = append(order, cur)
			}
			sizes[cur] += int(n)
		}
	}
	var r []areaUse
	for _, name := range order {
		if sizes[name] > 0 {
			r = append(r, areaUse{name, sizes[name]})
		}
	}
	sort.SliceStable(r, func(i, j int) bool { return r[i].size > r[j].size })
	return r
}

// describeObject はオブジェクトの名前を分かる名前にする (_main.o は module main、base.o は fc の作業域)。
func (c *compilation) describeObject(obj, area string) string {
	base := obj
	if i := strings.LastIndexAny(base, `/\`); i >= 0 {
		base = base[i+1:]
	}
	base = strings.TrimSuffix(base, ".o")
	switch {
	case strings.HasPrefix(base, "_"):
		return "module " + base[1:]
	case base == "base" && c.prog != nil:
		if area == "SRAM" || area == "RAM" {
			return fmt.Sprintf("fc work area (static frames %d)", c.staticRamReserve())
		}
		return "fc work area"
	}
	return base
}
