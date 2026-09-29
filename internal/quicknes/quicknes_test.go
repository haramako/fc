package quicknes

import (
	"context"
	"errors"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/haramako/fc/internal/driver"
)

// buildHello は examples/hello を NES の ROM にする。
func buildHello(t *testing.T) []byte {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	out := filepath.Join(dir, "hello.nes")
	if _, err := driver.NewCompiler(root).BuildContext(context.Background(), "hello.fc", &driver.BuildOptions{
		Target: "nes", Dir: filepath.Join(root, "examples", "hello"), BuildDir: filepath.Join(dir, "b"), Out: out,
	}); err != nil {
		t.Fatal(err)
	}
	rom, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	return rom
}

// TestHello: examples/hello を QuickNES で動かすと文字が描かれ (10 行目 = y 80〜87 に明るい点がある)、右を押すとスプライトが動く。
// 同じプロセスで 2 回開ける (コアを止めて起こし直せる)。
func TestHello(t *testing.T) {
	rom := buildHello(t)
	for round := 0; round < 2; round++ {
		m, err := Open(rom)
		if errors.Is(err, ErrUnavailable) {
			t.Skip(err)
		} else if err != nil {
			t.Fatal(err)
		}
		m.RunFrames(60)
		img := m.Image()
		bright := 0
		for y := 80; y < 88; y++ {
			for x := 0; x < Width; x++ {
				if c := img.RGBAAt(x, y); int(c.R)+int(c.G)+int(c.B) > 300 {
					bright++
				}
			}
		}
		if bright < 50 {
			t.Errorf("round %d: 文字の行に明るい点が %d しか無い", round, bright)
		}
		if ram := m.RAM(); len(ram) != 0x800 {
			t.Errorf("RAM の大きさ: %d", len(ram))
		}
		// 右を押すと、スプライト (オレンジの塗りつぶし) が右へ動く
		spriteX := func() int {
			img := m.Image()
			for y := 0; y < Height; y++ {
				for x := 0; x < Width; x++ {
					if c := img.RGBAAt(x, y); c.R > 180 && c.G > 60 && c.G < 180 && c.B < 80 {
						return x
					}
				}
			}
			return -1
		}
		x0 := spriteX()
		m.SetButtons(0, ButtonRight)
		m.RunFrames(10)
		m.SetButtons(0, 0)
		m.RunFrames(2)
		if x1 := spriteX(); x0 < 0 || x1 < x0+8 {
			t.Errorf("round %d: スプライトの x: %d → %d", round, x0, x1)
		}
		if round == 0 {
			if p := os.Getenv("FC_QUICKNES_PNG"); p != "" {
				f, _ := os.Create(p)
				png.Encode(f, m.Image())
				f.Close()
			}
		}
		m.Close()
	}
}
