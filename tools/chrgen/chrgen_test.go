package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haramako/fc/pkg/fc"
)

var update = flag.Bool("update", false, "games/common/tiles.{chr,fc,png} を tiles.txt から書き直す")

const commonTiles = "../../games/common/tiles.txt"

// TestCommonTiles: games/common の生成物が tiles.txt と合っている (go test ./tools/chrgen -update で書き直す)。
func TestCommonTiles(t *testing.T) {
	out, err := outputs(commonTiles)
	if err != nil {
		t.Fatal(err)
	}
	for path, want := range out {
		if *update {
			if err := os.WriteFile(path, want, 0o666); err != nil {
				t.Fatal(err)
			}
			continue
		}
		got, err := os.ReadFile(path)
		same := err == nil && bytes.Equal(got, want)
		if err == nil && !same && filepath.Ext(path) == ".png" {
			same = samePixels(got, want) // 圧縮のバイト列は Go の版で変わる (CI は go.mod の 1.24、手元は新しい版)
		}
		if !same {
			t.Errorf("%s が tiles.txt と違う (go test ./tools/chrgen -update か go run ./tools/chrgen games/common/tiles.txt)", path)
		}
	}
}

// samePixels は 2 つの PNG が同じ大きさで画素が全部同じか。
func samePixels(a, b []byte) bool {
	ia, err1 := png.Decode(bytes.NewReader(a))
	ib, err2 := png.Decode(bytes.NewReader(b))
	if err1 != nil || err2 != nil || ia.Bounds() != ib.Bounds() {
		return false
	}
	r := ia.Bounds()
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if color.NRGBAModel.Convert(ia.At(x, y)) != color.NRGBAModel.Convert(ib.At(x, y)) {
				return false
			}
		}
	}
	return true
}

// TestCommonTilesBuild: tiles.fc と tiles.chr を使うプログラムが NES の ROM にビルドできる (定数・メタスプライトの表)。
func TestCommonTilesBuild(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"tiles.fc", "tiles.chr"} {
		b, err := os.ReadFile(filepath.Join("../../games/common", name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o666); err != nil {
			t.Fatal(err)
		}
	}
	main := `#fc 4
use nes;
use frame;
use oam;
use tiles;
function main():void
{
	frame.init();
	frame.ctrl |= nes.CTRL_SPR_1000;
	oam.begin();
	oam.meta(100, 100, tiles.HERO_R0_M8, 0);
	oam.spr(20, 20, tiles.BALL, 0);
	oam.end();
	frame.wait();
}
`
	if err := os.WriteFile(filepath.Join(dir, "main.fc"), []byte(main), 0o666); err != nil {
		t.Fatal(err)
	}
	root, _ := filepath.Abs("../..")
	c := fc.NewWithHome(root)
	defer c.Close()
	if _, err := c.Build(context.Background(), "main.fc", fc.Options{Target: fc.TargetNES, Dir: dir, Out: filepath.Join(dir, "a.nes")}); err != nil {
		t.Fatal(err)
	}
}

// TestTallBG: 高さ 24 以上の BG の絵 (列ごとに 3 枚以上) を続けて置いても重ならず、どのタイルも絵のその所 (GitHub の Issue #6)。
// 色 0 は . でも 0 でも書ける (Issue #7)。
func TestTallBG(t *testing.T) {
	var src strings.Builder
	src.WriteString("at bg 0x80\n")
	pic := func(name string, w, h, seed int) [][]byte {
		fmt.Fprintf(&src, "== %s bg %dx%d\n", name, w, h)
		pix := make([][]byte, h)
		for y := range pix {
			pix[y] = make([]byte, w)
			for x := range pix[y] {
				c := byte((x*7 + y*13 + seed) % 4)
				pix[y][x] = c
				ch := ".123"[c]
				if c == 0 && (x+y)%2 == 0 {
					ch = '0'
				}
				src.WriteByte(ch)
			}
			src.WriteByte('\n')
		}
		return pix
	}
	want := map[string][][]byte{
		"ROCK_A": pic("ROCK_A", 32, 32, 0),
		"ROCK_B": pic("ROCK_B", 32, 32, 1),
		"TALL":   pic("TALL", 16, 24, 2),
	}
	path := filepath.Join(t.TempDir(), "t.txt")
	if err := os.WriteFile(path, []byte(src.String()), 0o666); err != nil {
		t.Fatal(err)
	}
	s, err := parse(path)
	if err != nil {
		t.Fatal(err)
	}
	for name, pix := range want {
		o := s.byName[name]
		for ty := 0; ty < len(pix)/8; ty++ {
			for tx := 0; tx < len(pix[0])/8; tx++ {
				i := o.index + ty*16 + tx
				if s.used[0][i] != name {
					t.Fatalf("%s のタイル (%d, %d) = %#02x の印が %q", name, tx, ty, i, s.used[0][i])
				}
				for y := 0; y < 8; y++ {
					for x := 0; x < 8; x++ {
						if got := s.tiles[0][i][y][x]; got != pix[ty*8+y][tx*8+x] {
							t.Fatalf("%s のタイル (%d, %d) の (%d, %d) が %d (%d のはず)", name, tx, ty, x, y, got, pix[ty*8+y][tx*8+x])
						}
					}
				}
			}
		}
	}
}
