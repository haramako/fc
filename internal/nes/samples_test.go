package nes

// examples/ の fc 4 の小さなサンプル (life / jump / statusbar / wave) のテスト。動きは内蔵のランナーで、見た目 (スクロールの
// 分割などの描画の途中の変化) は internal/quicknes (libretro の QuickNES) の画面で確かめる。FC_SAMPLE_PNG_DIR を与えると
// QuickNES の画面を <名前>.png に書く。

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haramako/fc/internal/driver"
	"github.com/haramako/fc/internal/quicknes"
)

// exampleFiles は examples/<name> のファイル (main は t.fc に名前を変える)。
func exampleFiles(t *testing.T, name, main string) map[string]string {
	t.Helper()
	dir := filepath.Join("..", "..", "examples", name)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		n := e.Name()
		if n == main {
			n = "t.fc"
		}
		files[n] = string(b)
	}
	return files
}

// romOf はビルドした ROM (buildNes の後で)。
func romOf(t *testing.T, p *nesProg) []byte {
	t.Helper()
	b, err := os.ReadFile(p.rom)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// openQuickNES は rom を QuickNES で開く (コアが無ければ Skip)。
func openQuickNES(t *testing.T, rom []byte) *quicknes.Machine {
	t.Helper()
	m, err := quicknes.Open(rom)
	if errors.Is(err, quicknes.ErrUnavailable) {
		t.Skip(err)
	} else if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Close)
	return m
}

// saveSample は FC_SAMPLE_PNG_DIR があれば画面を <name>.png に書く。
func saveSample(t *testing.T, name string, img image.Image) {
	t.Helper()
	dir := os.Getenv("FC_SAMPLE_PNG_DIR")
	if dir == "" {
		return
	}
	f, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

// lifeRef は examples/life と同じライフゲームを Go で走らせた出力。
func lifeRef() string {
	const w, h, gens = 32, 20, 8
	start := []int{1, 0, 2, 1, 0, 2, 1, 2, 2, 2, 20, 4, 21, 4, 22, 4, 10, 12, 11, 12, 12, 12, 9, 13, 10, 13, 11, 13}
	var cur [h][w]bool
	for i := 0; i < len(start); i += 2 {
		cur[start[i+1]][start[i]] = true
	}
	var b strings.Builder
	for g := 0; g <= gens; g++ {
		fmt.Fprintf(&b, "gen %d\n", g)
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				if cur[y][x] {
					b.WriteByte('#')
				} else {
					b.WriteByte('.')
				}
			}
			b.WriteByte('\n')
		}
		var nxt [h][w]bool
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				n := 0
				for dy := -1; dy <= 1; dy++ {
					for dx := -1; dx <= 1; dx++ {
						if (dx != 0 || dy != 0) && cur[(y+dy+h)%h][(x+dx+w)%w] {
							n++
						}
					}
				}
				nxt[y][x] = n == 3 || n == 2 && cur[y][x]
			}
		}
		cur = nxt
	}
	return b.String()
}

// TestExampleLife: examples/life (console のライフゲーム) を内蔵のエミュレータと NES のランナー (console の出力を $4018 で
// 受け取る) で走らせ、Go で走らせた同じライフゲームと比べる。
func TestExampleLife(t *testing.T) {
	t.Parallel()
	want := lifeRef()
	files := exampleFiles(t, "life", "life.fc")
	// emu
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for n, s := range files {
		if err := os.WriteFile(filepath.Join(dir, n), []byte(s), 0o666); err != nil {
			t.Fatal(err)
		}
	}
	var out strings.Builder
	if _, err := driver.NewCompiler(repoRoot).BuildContext(context.Background(), "t.fc", &driver.BuildOptions{
		Target: "emu", Dir: dir, BuildDir: filepath.Join(dir, "b"), Out: filepath.Join(dir, "t.bin"), Run: true, Stdout: &out,
	}); err != nil {
		t.Fatal(err)
	}
	if out.String() != want {
		t.Errorf("emu の出力が Go のライフゲームと違う:\n%s", out.String())
	}
	// NES
	p := buildNes(t, files)
	var nesOut strings.Builder
	p.Output = &nesOut
	if err := p.RunUntilExit(600); err != nil {
		t.Fatal(err)
	}
	t.Logf("NES: %d フレームで 9 世代", p.Stats.Frames)
	if nesOut.String() != want || p.ExitCode != 0 {
		t.Errorf("NES の出力が違う (exit %d):\n%s", p.ExitCode, nesOut.String())
	}
}

// TestExampleJump: examples/jump を動かす。床から右へ歩いて跳ぶと 1 つ目の足場に乗り、そこのコインを取る (COIN 1/3)。
// 足場から歩いて外れると床へ落ちる。同じ入力で QuickNES の画面にも自分と文字が出る。
func TestExampleJump(t *testing.T) {
	t.Parallel()
	p := buildNes(t, exampleFiles(t, "jump", "jump.fc"))
	type step struct {
		buttons byte
		frames  int
	}
	script := []step{{0, 20}, {ButtonRight, 32}, {ButtonA | ButtonRight, 4}, {ButtonA, 40}, {0, 10}}
	play := func(set func(byte), run func(int)) {
		for _, s := range script {
			set(s.buttons)
			run(s.frames)
		}
		set(0)
	}
	play(p.SetButtons, func(n int) { p.run(t, n) })
	y := func() int { return (p.peek(t, "_t_py", 0) | p.peek(t, "_t_py", 1)<<8) >> 4 }
	if px, py := p.peek(t, "_t_px", 0), y(); py != 21*8-8 || px < 48 {
		t.Fatalf("1 つ目の足場に乗っていない: (%d, %d)", px, py)
	}
	if p.peek(t, "_t_score", 0) != 1 || !strings.HasPrefix(p.ntRow(0x2000, 2), "\x00\x00COIN 1/3") {
		t.Errorf("コイン: score %d, %q", p.peek(t, "_t_score", 0), p.ntRow(0x2000, 2))
	}
	// 右へ歩いて足場から外れると床に落ちる
	p.SetButtons(ButtonRight)
	p.run(t, 40)
	p.SetButtons(0)
	p.run(t, 30)
	if py := y(); py != 26*8-8 {
		t.Errorf("床に落ちていない: y %d", py)
	}
	p.checkVblank(t)

	q := openQuickNES(t, romOf(t, p))
	play(func(b byte) { q.SetButtons(0, b) }, q.RunFrames)
	img := q.Image()
	saveSample(t, "jump", img)
	// 自分 (オレンジ) が足場の上 (y 152〜159) にいる
	found := false
	for yy := 150; yy < 162 && !found; yy++ {
		for xx := 40; xx < 100; xx++ {
			if c := img.RGBAAt(xx, yy); c.R > 180 && c.G > 60 && c.G < 180 && c.B < 80 {
				found = true
				break
			}
		}
	}
	if !found {
		t.Errorf("QuickNES の画面で、足場の上に自分が見えない")
	}
}
