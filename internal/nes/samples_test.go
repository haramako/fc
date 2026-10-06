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
	res, err := driver.NewCompiler(repoRoot).BuildContext(context.Background(), "t.fc", &driver.BuildOptions{
		Target: "emu", Dir: dir, BuildDir: filepath.Join(dir, "b"), Out: filepath.Join(dir, "t.bin"),
	})
	if err != nil {
		t.Fatal(err)
	}
	runEmuBuilt(t, res, &out, &out, 0)
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

// regionEqual は a と b の y0〜y1 の行が同じか。
func regionEqual(a, b *image.RGBA, y0, y1 int) bool {
	for y := y0; y < y1; y++ {
		for x := 0; x < a.Bounds().Dx(); x++ {
			if a.RGBAAt(x, y) != b.RGBAAt(x, y) {
				return false
			}
		}
	}
	return true
}

// TestExampleStatusbar: examples/statusbar (MMC3 の IRQ で画面を分割)。内蔵のランナーでは毎フレーム IRQ が来てスクロールが
// 進む。QuickNES の画面では、ステータスバー (上の 4 行) は時間がたっても同じで、下の画面だけが動いている。
func TestExampleStatusbar(t *testing.T) {
	t.Parallel()
	p := buildNes(t, exampleFiles(t, "statusbar", "statusbar.fc"))
	p.run(t, 60)
	irq0 := p.Stats.IrqCount
	x0 := p.peek(t, "split_x", 0)
	p.run(t, 10)
	if d := p.Stats.IrqCount - irq0; d < 9 || d > 11 {
		t.Errorf("10 フレームで IRQ が %d 回", d)
	}
	if d := (p.peek(t, "split_x", 0) - x0 + 256) % 256; d != 10 {
		t.Errorf("10 フレームでスクロールが %d 進んだ", d)
	}
	p.checkVblank(t)

	q := openQuickNES(t, romOf(t, p))
	q.RunFrames(60)
	a := q.Image()
	q.RunFrames(20)
	b := q.Image()
	saveSample(t, "statusbar", b)
	// FRAME の数の行 (y 16〜23) は変わるので、見出しの行 (y 8〜15) と区切りの行 (y 24〜31) だけ比べる
	if !regionEqual(a, b, 8, 16) || !regionEqual(a, b, 25, 31) {
		t.Errorf("ステータスバーが動いている")
	}
	if regionEqual(a, b, 40, 232) {
		t.Errorf("下の画面が動いていない")
	}
}

// barX は img の y の行で、最初に明るい点 (縦の線) がある x (無ければ -1)。
func barX(img *image.RGBA, y int) int {
	for x := 0; x < img.Bounds().Dx(); x++ {
		if c := img.RGBAAt(x, y); int(c.R)+int(c.G)+int(c.B) > 200 {
			return x
		}
	}
	return -1
}

// TestExampleWave: examples/wave (8 ラインごとの IRQ で横のスクロールをずらす)。内蔵のランナーでは 1 フレームに約 30 回 IRQ が
// 来る。QuickNES の画面では、縦の線の位置が帯ごとに違い (波打ち)、時間で変わる。
func TestExampleWave(t *testing.T) {
	t.Parallel()
	p := buildNes(t, exampleFiles(t, "wave", "wave.fc"))
	p.run(t, 30)
	irq0 := p.Stats.IrqCount
	p.run(t, 1)
	if d := p.Stats.IrqCount - irq0; d < 27 || d > 31 {
		t.Errorf("1 フレームの IRQ が %d 回", d)
	}
	p.checkVblank(t)

	q := openQuickNES(t, romOf(t, p))
	q.RunFrames(40)
	a := q.Image()
	saveSample(t, "wave", a)
	xs := map[int]bool{}
	for y := 4; y < 232; y += 8 {
		xs[barX(a, y)%16] = true // 縦の線は 32 ドットごと: 線の間隔の中の位置
	}
	if len(xs) < 4 {
		t.Errorf("帯ごとの縦の線の位置が %d 通りしかない (波打っていない)", len(xs))
	}
	q.RunFrames(5)
	if regionEqual(a, q.Image(), 0, 232) {
		t.Errorf("波が動いていない")
	}
}

// TestSampleScreens: 利用者向けのドキュメントのサンプル集 (docs/samples) の画面のうち、ほかのテストが撮らないもの (hello・miku4) を
// QuickNES で撮る。FC_SAMPLE_PNG_DIR を与えたときだけ走る (jump・statusbar・wave はそれぞれのテストが同じ場所に書く):
//
//	FC_SAMPLE_PNG_DIR=docs/public/samples go test ./internal/nes -run 'TestExample(Jump|Statusbar|Wave)|TestSampleScreens'
func TestSampleScreens(t *testing.T) {
	if os.Getenv("FC_SAMPLE_PNG_DIR") == "" {
		t.Skip("FC_SAMPLE_PNG_DIR が無い")
	}
	t.Parallel()
	shots := []struct {
		name, dir, main string
		script          []struct{ buttons, frames int }
	}{
		// フェードが済んでから右下へ少し動かす
		{"hello", "hello", "hello.fc", []struct{ buttons, frames int }{{0, 60}, {ButtonRight | ButtonDown, 12}, {0, 2}}},
		// 少し進んで敵と弾が出たところ
		{"miku4", "miku4", "miku.fc", []struct{ buttons, frames int }{{0, 30}, {ButtonLeft | ButtonA, 20}, {ButtonA, 120}}},
	}
	for _, s := range shots {
		t.Run(s.name, func(t *testing.T) {
			t.Parallel()
			p := buildNes(t, exampleFiles(t, s.dir, s.main))
			q := openQuickNES(t, romOf(t, p))
			for _, st := range s.script {
				q.SetButtons(0, byte(st.buttons))
				q.RunFrames(st.frames)
			}
			saveSample(t, s.name, q.Image())
		})
	}
}
