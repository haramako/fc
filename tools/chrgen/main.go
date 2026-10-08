// chrgen はテキストで書いたドット絵から NES の CHR (8 KB: BG $0000・スプライト $1000) と、タイルの名前の fc のモジュールと、
// 確かめ用の PNG を作る。
//
//	go run ./tools/chrgen games/common/tiles.txt     // tiles.chr・tiles.fc・tiles.png を同じディレクトリに
//
// 元の書き方 (games/common/tiles.txt の頭にも):
//
//	# コメント
//	font <path> <from> <to> <color>   // font.txt (fclib/nes/font.txt の形) の文字を BG のその番号に、色 color で
//	at <bg|spr> <index>               // 次に置く場所を探し始める番号
//	maze <NAME> <index>               // 迷路の壁 16 枚 (index + 隣の向き: 1 上・2 右・4 下・8 左)
//	== <NAME> <bg|spr> <W>x<H> [pal=N] [= <op> ...]
//	<H 行の W 文字: . 1 2 3 は色の番号>
//
// op は flipx / flipy / rot (時計回りに 90 度。正方形だけ) / copy <NAME>、glyph <文字> <色> [on <NAME>] (8×8。font の文字を
// 重ねる)。
//
// 置き方: BG は左上から 16 タイル × 16 行の並びで、W×H の塊が縦横に並ぶ (16×16 なら n, n+1, n+16, n+17)。スプライトは
// 8×16 のモード (タイルの番号の下位 1 ビットが $1000 を選び、偶数・奇数の 2 枚が上下に並ぶ) でもそのまま使えるように、
// 列ごとに上下 2 枚を続けて置く: 16×16 は左上・左下・右上・右下 (n..n+3)、8×16 は上・下、8×8 は絵と空白の 2 枚。
package main

import (
	"bufio"
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// tile は 8×8 の色の番号 (0〜3)。
type tile [8][8]byte

// object は名前の付いた絵 1 つ。
type object struct {
	name  string
	spr   bool
	w, h  int // ドット
	pal   int
	pix   [][]byte // [y][x]
	index int      // 置いた最初のタイルの番号
	maze  bool
}

// sheet は作っている CHR。
type sheet struct {
	tiles  [2][256]tile
	used   [2][256]string // 置いたものの名前
	cursor [2]int
	objs   []*object
	byName map[string]*object
	font   map[byte][7]byte // font.txt の文字 (5 ドット × 7 行。ビット 4 が左)
	dir    string
}

func tableOf(s string) (int, error) {
	switch s {
	case "bg":
		return 0, nil
	case "spr":
		return 1, nil
	}
	return 0, fmt.Errorf("bg か spr: %q", s)
}

func (s *sheet) mark(t, i int, name string) error {
	if i < 0 || i > 255 {
		return fmt.Errorf("%s: 番号 %d が表の外", name, i)
	}
	if s.used[t][i] != "" {
		return fmt.Errorf("%s: %s の %#02x は %s が使っている", name, []string{"bg", "spr"}[t], i, s.used[t][i])
	}
	s.used[t][i] = name
	return nil
}

// place は o の場所を決めてタイルを書く。
func (s *sheet) place(o *object) error {
	tw, th := o.w/8, o.h/8
	t := 0
	if o.spr {
		t = 1
	}
	var at []int // 置く順のタイルの番号 (絵の列ごとに上から)
	if !o.spr {
		found := -1
		for p := s.cursor[0]; p < 256 && found < 0; p++ {
			col, row := p%16, p/16
			if col+tw > 16 || row%th != 0 || row+th > 16 {
				continue
			}
			free := true
			for y := 0; y < th && free; y++ {
				for x := 0; x < tw && free; x++ {
					free = s.used[0][p+y*16+x] == ""
				}
			}
			if free {
				found = p
			}
		}
		if found < 0 {
			return fmt.Errorf("%s: BG に場所が無い", o.name)
		}
		o.index = found
		for x := 0; x < tw; x++ {
			for y := 0; y < th; y++ {
				at = append(at, found+y*16+x)
			}
		}
	} else {
		if th > 2 {
			return fmt.Errorf("%s: スプライトの高さは 8 か 16", o.name)
		}
		p := s.cursor[1]
		p += p & 1
		o.index = p
		for x := 0; x < tw; x++ {
			at = append(at, p+2*x, p+2*x+1)
		}
		s.cursor[1] = p + 2*tw
	}
	k := 0
	for x := 0; x < tw; x++ {
		for y := 0; y < 2 && (y < th || o.spr); y++ {
			i := at[k]
			k++
			if err := s.mark(t, i, o.name); err != nil {
				return err
			}
			if y >= th {
				continue // 8×8 のスプライトの下の空白
			}
			var tl tile
			for yy := 0; yy < 8; yy++ {
				for xx := 0; xx < 8; xx++ {
					tl[yy][xx] = o.pix[y*8+yy][x*8+xx]
				}
			}
			s.tiles[t][i] = tl
		}
	}
	s.objs = append(s.objs, o)
	s.byName[o.name] = o
	return nil
}

// loadFont は font.txt (fclib/nes/font.txt の形) を読む。
func (s *sheet) loadFont(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	s.font = map[byte][7]byte{}
	sc := bufio.NewScanner(bytes.NewReader(b))
	code, row := -1, 0
	var g [7]byte
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case line == "" || strings.HasPrefix(line, "#") && code < 0:
			continue
		case code < 0 || row == 7:
			if code >= 0 {
				s.font[byte(code)] = g
			}
			c, err := strconv.ParseUint(strings.Fields(line)[0], 16, 8)
			if err != nil {
				return fmt.Errorf("%s: %v", path, err)
			}
			code, row, g = int(c), 0, [7]byte{}
		default:
			for i, ch := range line {
				if ch == '#' {
					g[row] |= 0x10 >> i
				}
			}
			row++
		}
	}
	if code >= 0 {
		s.font[byte(code)] = g
	}
	return sc.Err()
}

// glyph は文字 c を色 col で 8×8 に書く (2〜6 列目・1〜7 行目。内蔵のフォントと同じ位置)。
func (s *sheet) glyph(c byte, col byte, base [][]byte) ([][]byte, error) {
	g, ok := s.font[c]
	if !ok {
		return nil, fmt.Errorf("font に文字 %q が無い", c)
	}
	pix := blank(8, 8)
	if base != nil {
		pix = clone(base)
	}
	for y := 0; y < 7; y++ {
		for x := 0; x < 5; x++ {
			if g[y]&(0x10>>x) != 0 {
				pix[1+y][1+x] = col
			}
		}
	}
	return pix, nil
}

func blank(w, h int) [][]byte {
	p := make([][]byte, h)
	for y := range p {
		p[y] = make([]byte, w)
	}
	return p
}

func clone(p [][]byte) [][]byte {
	c := make([][]byte, len(p))
	for y := range p {
		c[y] = append([]byte(nil), p[y]...)
	}
	return c
}

// mazeTile は迷路の壁の 1 枚 (幅 4 の管。隣とつながる向きへ伸ばす。縁は色 3、中は 2)。
func mazeTile(mask int) [][]byte {
	in := func(x, y int) bool {
		cx, cy := x >= 2 && x <= 5, y >= 2 && y <= 5
		switch {
		case cx && cy:
			return true
		case cx && y < 2:
			return mask&1 != 0
		case cx && y > 5:
			return mask&4 != 0
		case cy && x > 5:
			return mask&2 != 0
		case cy && x < 2:
			return mask&8 != 0
		}
		return false
	}
	p := blank(8, 8)
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			if !in(x, y) {
				continue
			}
			edge := false
			for _, d := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
				nx, ny := x+d[0], y+d[1]
				if nx >= 0 && nx < 8 && ny >= 0 && ny < 8 && !in(nx, ny) {
					edge = true
				}
			}
			p[y][x] = 2
			if edge {
				p[y][x] = 3
			}
		}
	}
	return p
}

// parse は元のテキストを読んで sheet を作る。
func parse(path string) (*sheet, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	s := &sheet{byName: map[string]*object{}, dir: filepath.Dir(path)}
	lines := strings.Split(strings.ReplaceAll(string(src), "\r\n", "\n"), "\n")
	for n := 0; n < len(lines); n++ {
		line := strings.TrimSpace(lines[n])
		errf := func(format string, a ...any) error {
			return fmt.Errorf("%s:%d: %s", path, n+1, fmt.Sprintf(format, a...))
		}
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Fields(line)
		switch f[0] {
		case "font":
			if len(f) != 5 {
				return nil, errf("font <path> <from> <to> <color>")
			}
			if err := s.loadFont(filepath.Join(s.dir, f[1])); err != nil {
				return nil, errf("%v", err)
			}
			from, _ := strconv.ParseUint(f[2], 0, 8)
			to, _ := strconv.ParseUint(f[3], 0, 8)
			col, _ := strconv.Atoi(f[4])
			for c := from; c <= to; c++ {
				pix, err := s.glyph(byte(c), byte(col), nil)
				if err != nil {
					return nil, errf("%v", err)
				}
				if err := s.mark(0, int(c), "font"); err != nil {
					return nil, errf("%v", err)
				}
				var tl tile
				for y := 0; y < 8; y++ {
					copy(tl[y][:], pix[y])
				}
				s.tiles[0][c] = tl
			}
		case "at":
			if len(f) != 3 {
				return nil, errf("at <bg|spr> <index>")
			}
			t, err := tableOf(f[1])
			if err != nil {
				return nil, errf("%v", err)
			}
			i, err := strconv.ParseUint(f[2], 0, 8)
			if err != nil {
				return nil, errf("%v", err)
			}
			s.cursor[t] = int(i)
		case "maze":
			if len(f) != 3 {
				return nil, errf("maze <NAME> <index>")
			}
			i, err := strconv.ParseUint(f[2], 0, 8)
			if err != nil || i+16 > 256 {
				return nil, errf("maze の番号 %s", f[2])
			}
			o := &object{name: f[1], w: 8, h: 8, index: int(i), maze: true, pix: mazeTile(15)}
			for m := 0; m < 16; m++ {
				if err := s.mark(0, int(i)+m, f[1]); err != nil {
					return nil, errf("%v", err)
				}
				var tl tile
				for y, row := range mazeTile(m) {
					copy(tl[y][:], row)
				}
				s.tiles[0][int(i)+m] = tl
			}
			s.objs = append(s.objs, o)
			s.byName[o.name] = o
		case "==":
			o, used, err := s.entry(f[1:], lines[n+1:])
			if err != nil {
				return nil, errf("%v", err)
			}
			n += used
			if err := s.place(o); err != nil {
				return nil, errf("%v", err)
			}
		default:
			return nil, errf("知らない行: %s", line)
		}
	}
	return s, nil
}

// entry は `== NAME bg|spr WxH [pal=N] [= op ...]` と、続く絵の行を読む (読んだ行の数を返す)。
func (s *sheet) entry(f []string, rest []string) (*object, int, error) {
	if len(f) < 3 {
		return nil, 0, fmt.Errorf("== <NAME> <bg|spr> <W>x<H> [pal=N] [= op ...]")
	}
	o := &object{name: f[0]}
	if _, dup := s.byName[o.name]; dup {
		return nil, 0, fmt.Errorf("%s が 2 つある", o.name)
	}
	t, err := tableOf(f[1])
	if err != nil {
		return nil, 0, err
	}
	o.spr = t == 1
	if _, err := fmt.Sscanf(f[2], "%dx%d", &o.w, &o.h); err != nil || o.w%8 != 0 || o.h%8 != 0 || o.w == 0 || o.h == 0 {
		return nil, 0, fmt.Errorf("%s: 大きさ %q (8 の倍数の WxH)", o.name, f[2])
	}
	f = f[3:]
	if len(f) > 0 && strings.HasPrefix(f[0], "pal=") {
		o.pal, _ = strconv.Atoi(strings.TrimPrefix(f[0], "pal="))
		f = f[1:]
	}
	if len(f) > 0 {
		if f[0] != "=" || len(f) < 2 {
			return nil, 0, fmt.Errorf("%s: `= op ...`", o.name)
		}
		pix, err := s.op(o, f[1:])
		if err != nil {
			return nil, 0, fmt.Errorf("%s: %v", o.name, err)
		}
		o.pix = pix
		return o, 0, nil
	}
	used := 0
	for len(o.pix) < o.h {
		if used >= len(rest) {
			return nil, 0, fmt.Errorf("%s: 絵の行が %d 行足りない", o.name, o.h-len(o.pix))
		}
		line := strings.TrimSpace(rest[used])
		used++
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if len(line) != o.w {
			return nil, 0, fmt.Errorf("%s: %d 行目の幅が %d (%d のはず): %s", o.name, len(o.pix)+1, len(line), o.w, line)
		}
		row := make([]byte, o.w)
		for x, ch := range []byte(line) {
			switch ch {
			case '.':
			case '1', '2', '3':
				row[x] = ch - '0'
			default:
				return nil, 0, fmt.Errorf("%s: 知らない文字 %q (. 1 2 3)", o.name, ch)
			}
		}
		o.pix = append(o.pix, row)
	}
	return o, used, nil
}

// op は `= op ...` の絵を作る。
func (s *sheet) op(o *object, f []string) ([][]byte, error) {
	src := func(name string) ([][]byte, error) {
		b, ok := s.byName[name]
		if !ok {
			return nil, fmt.Errorf("%s が無い (前に書く)", name)
		}
		if b.w != o.w || b.h != o.h {
			if !(f[0] == "rot" && b.w == o.h && b.h == o.w) {
				return nil, fmt.Errorf("%s と大きさが違う", name)
			}
		}
		return b.pix, nil
	}
	switch f[0] {
	case "copy", "flipx", "flipy", "rot":
		if len(f) != 2 {
			return nil, fmt.Errorf("%s <NAME>", f[0])
		}
		p, err := src(f[1])
		if err != nil {
			return nil, err
		}
		out := blank(o.w, o.h)
		for y := 0; y < o.h; y++ {
			for x := 0; x < o.w; x++ {
				switch f[0] {
				case "copy":
					out[y][x] = p[y][x]
				case "flipx":
					out[y][x] = p[y][o.w-1-x]
				case "flipy":
					out[y][x] = p[o.h-1-y][x]
				case "rot":
					out[y][x] = p[o.w-1-x][y]
				}
			}
		}
		return out, nil
	case "glyph":
		if o.w != 8 || o.h != 8 || (len(f) != 3 && len(f) != 5) {
			return nil, fmt.Errorf("glyph <文字> <色> [on <NAME>] (8×8)")
		}
		col, _ := strconv.Atoi(f[2])
		var base [][]byte
		if len(f) == 5 {
			p, err := src(f[4])
			if err != nil {
				return nil, err
			}
			base = p
		}
		return s.glyph(f[1][0], byte(col), base)
	}
	return nil, fmt.Errorf("知らない op %q", f[0])
}

// chr は 8 KB の CHR (2 プレーンの 16 バイト × 512 タイル)。
func (s *sheet) chr() []byte {
	out := make([]byte, 0, 8192)
	for t := 0; t < 2; t++ {
		for i := 0; i < 256; i++ {
			var lo, hi [8]byte
			for y := 0; y < 8; y++ {
				for x := 0; x < 8; x++ {
					c := s.tiles[t][i][y][x]
					lo[y] |= (c & 1) << (7 - x)
					hi[y] |= (c >> 1 & 1) << (7 - x)
				}
			}
			out = append(out, lo[:]...)
			out = append(out, hi[:]...)
		}
	}
	return out
}

// module はタイルの名前の fc のモジュール。
func (s *sheet) module(srcName, chrName string) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "#fc 4\n// %s から tools/chrgen が作るタイルの番号 (手で書き換えない: go run ./tools/chrgen <%s のパス>)。\n", srcName, srcName)
	b.WriteString(`//
// BG ($0000): 16×16 の絵は n, n+1, n+16, n+17 (左上・右上・左下・右下)。$20〜$7F は ASCII の文字。
// スプライト ($1000。8×8 のモードなら frame.ctrl に nes.CTRL_SPR_1000): NAME は最初のタイル。
//   NAME_M8 は 8×8 のモードの oam.meta の表、NAME_M16 は 8×16 のモード (frame.ctrl に nes.CTRL_SPR_8X16) の表。
//   NAME_T16 は 8×16 のモードのタイルの番号 (下位 1 ビットが $1000 を選ぶ。8×8 の絵は下が空白)。
//   表の attr はパレットの番号だけ。
// use すると CHR (同じディレクトリの .chr) も ROM に入る (CHR RAM のゲームは fc.toml の [define.tiles] で CHR_ROM = false にして
// 定数だけ使い、絵は @incbin で読んで送る)。
`)
	fmt.Fprintf(&b, "public const CHR_ROM = true @(build);\n@if (CHR_ROM) {\n\t@include(%q);\n}\n", chrName)
	objs := append([]*object(nil), s.objs...)
	sort.SliceStable(objs, func(i, j int) bool {
		if objs[i].spr != objs[j].spr {
			return !objs[i].spr
		}
		return objs[i].index < objs[j].index
	})
	meta := func(name string, es [][4]int) {
		var parts []string
		for _, e := range es {
			parts = append(parts, fmt.Sprintf("%d, %d, 0x%02x, %d", e[0], e[1], e[2], e[3]))
		}
		fmt.Fprintf(&b, "public const %s:[?]u8 = [%s];\n", name, strings.Join(parts, ",  "))
	}
	last := -1
	for _, o := range objs {
		t := 0
		if o.spr {
			t = 1
		}
		if t != last {
			b.WriteString([]string{"\n// ---- BG ----\n", "\n// ---- スプライト ----\n"}[t])
			last = t
		}
		kind := fmt.Sprintf("%dx%d", o.w, o.h)
		if o.maze {
			kind = "迷路の壁 16 枚 (+ 隣の向き: 1 上・2 右・4 下・8 左)"
		}
		fmt.Fprintf(&b, "public const %s = 0x%02x; // %s\n", o.name, o.index, kind)
		if !o.spr {
			continue
		}
		tw := o.w / 8
		if o.w == 8 {
			fmt.Fprintf(&b, "public const %s_T16 = 0x%02x;\n", o.name, o.index|1)
		}
		if o.h == 16 {
			var m8, m16 [][4]int
			for x := 0; x < tw; x++ {
				n := o.index + 2*x
				m8 = append(m8, [4]int{8 * x, 0, n, o.pal}, [4]int{8 * x, 8, n + 1, o.pal})
				m16 = append(m16, [4]int{8 * x, 0, n | 1, o.pal})
			}
			meta(o.name+"_M8", m8)
			if tw > 1 {
				meta(o.name+"_M16", m16)
			}
		}
	}
	return []byte(b.String())
}

// ---- PNG ----

var palettes = [2][4]color.RGBA{
	{{0x10, 0x10, 0x18, 255}, {0x7c, 0x50, 0x20, 255}, {0xd8, 0x80, 0x30, 255}, {0xfc, 0xf4, 0xd8, 255}}, // BG
	{{0x68, 0x88, 0xfc, 255}, {0x10, 0x10, 0x10, 255}, {0xd8, 0x28, 0x00, 255}, {0xfc, 0xd8, 0xa8, 255}}, // スプライト
}

type canvas struct{ img *image.RGBA }

func (c canvas) rect(x, y, w, h int, col color.RGBA) {
	for yy := y; yy < y+h; yy++ {
		for xx := x; xx < x+w; xx++ {
			c.img.SetRGBA(xx, yy, col)
		}
	}
}

// text はフォントの文字で s を書く (1 文字 6 ドット)。
func (c canvas) text(s *sheet, x, y int, str string, col color.RGBA) {
	for i := 0; i < len(str); i++ {
		g := s.font[str[i]]
		for yy := 0; yy < 7; yy++ {
			for xx := 0; xx < 5; xx++ {
				if g[yy]&(0x10>>xx) != 0 {
					c.img.SetRGBA(x+i*6+xx, y+yy, col)
				}
			}
		}
	}
}

// png は確かめ用の絵: 上に BG とスプライトの表 (2 倍)、下に名前の付いた絵 (2 倍) と名前。
func (s *sheet) png() []byte {
	const z = 2             // 拡大
	const cell = 8*z + 1    // 表の 1 タイル (線 1 ドット)
	const tab = 16*cell + 1 // 表の幅
	const margin = 8
	const cw = 84 // 名前の付いた絵の 1 つの幅
	gray := color.RGBA{0x50, 0x50, 0x50, 255}
	white := color.RGBA{0xff, 0xff, 0xff, 255}
	var objs []*object
	for _, o := range s.objs {
		if !o.maze {
			objs = append(objs, o)
		}
	}
	cols := (2*tab + margin) / cw
	rows := (len(objs) + cols - 1) / cols
	w := 2*tab + 3*margin
	top := margin + 10
	h := top + tab + margin + rows*(16*z+14) + margin
	c := canvas{image.NewRGBA(image.Rect(0, 0, w, h))}
	c.rect(0, 0, w, h, color.RGBA{0x28, 0x28, 0x30, 255})
	for t := 0; t < 2; t++ {
		x0 := margin + t*(tab+margin)
		c.text(s, x0, margin, []string{"BG $0000", "SPRITES $1000"}[t], white)
		c.rect(x0, top, tab, tab, gray)
		for i := 0; i < 256; i++ {
			tx, ty := x0+1+(i%16)*cell, top+1+(i/16)*cell
			for y := 0; y < 8; y++ {
				for x := 0; x < 8; x++ {
					c.rect(tx+x*z, ty+y*z, z, z, palettes[t][s.tiles[t][i][y][x]])
				}
			}
		}
	}
	y0 := top + tab + margin
	for k, o := range objs {
		x, y := margin+(k%cols)*cw, y0+(k/cols)*(16*z+14)
		t := 0
		if o.spr {
			t = 1
		}
		for yy := 0; yy < o.h; yy++ {
			for xx := 0; xx < o.w; xx++ {
				c.rect(x+xx*z, y+yy*z, z, z, palettes[t][o.pix[yy][xx]])
			}
		}
		c.text(s, x, y+o.h*z+3, o.name, white)
	}
	// 全体を 2 倍にする (表は 4 倍、名前の文字は 2 倍で見える)
	big := image.NewRGBA(image.Rect(0, 0, 2*w, 2*h))
	for y := 0; y < 2*h; y++ {
		for x := 0; x < 2*w; x++ {
			big.SetRGBA(x, y, c.img.RGBAAt(x/2, y/2))
		}
	}
	var buf bytes.Buffer
	png.Encode(&buf, big)
	return buf.Bytes()
}

// outputs は src から作るファイル (パス → 中身)。
func outputs(src string) (map[string][]byte, error) {
	s, err := parse(src)
	if err != nil {
		return nil, err
	}
	base := strings.TrimSuffix(src, filepath.Ext(src))
	return map[string][]byte{
		base + ".chr": s.chr(),
		base + ".fc":  s.module(filepath.Base(src), filepath.Base(base)+".chr"),
		base + ".png": s.png(),
	}, nil
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: chrgen <tiles.txt>")
		os.Exit(2)
	}
	out, err := outputs(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for path, b := range out {
		if err := os.WriteFile(path, b, 0o666); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
}
