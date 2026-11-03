package nes

// fclib/nes/raster (MMC3 の走査線の IRQ で画面を分ける) のテスト。
//   - TestRasterIrqChain: 内蔵のランナーで、境目の数だけ毎フレーム IRQ が来る (NMI の仕掛けと IRQ の中の連鎖)、off で減る、
//     主の側が処理落ちしても仕掛け続ける
//   - TestRasterSplit: Mesen (FC_MESEN) と QuickNES (FC_QUICKNES) の画面で、分割した画面が、分割しない 2 枚 (上の帯のスクロールと
//     境目から下のスクロール) を走査線で継いだものと画素まで同じ (内蔵のランナーは描画の途中の変化を描かないので、外のエミュレータで)。
//     raster.asm の待ちの長さと仕掛けの数はこれで合わせた

import (
	"context"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/haramako/fc/internal/driver"
)

const rasterToml = "[target]\nmapper = \"MMC3\"\nprg = \"32K\"\nchr = \"8K\"\n"

// rasterSrc は、2 面のネームテーブルを行ごとに違う模様で埋め、毎フレーム top のスクロールと splits の境目を決めるプログラム。
// work なら主の側で長い命令 (inc abs,x) を回して、1 フレームに収まらない (処理落ちする) ようにする。
func rasterSrc(top [3]int, splits [][4]int, work bool, extra string) string {
	set := fmt.Sprintf("\t\tframe.scroll(%d, %d, %d);\n", top[0], top[1], top[2])
	for i, s := range splits {
		set += fmt.Sprintf("\t\traster.split(%d, %d, %d, %d, %d);\n", i, s[0], s[1], s[2], s[3])
	}
	if work {
		set += "\t\tfor (var k:u16 = 0; k < 1500; k += 1) {\n\t\t\tcnt[(k as u8) & 15] += 1;\n\t\t\tcnt[((k as u8) + 3) & 15] += 1;\n\t\t}\n"
	}
	return `#fc 4
use nes;
use frame;
use vram;
use pal;
use raster;
@include("font.chr");

const PALETTE = [0x0f, 0x30, 0x16, 0x2a, 0x0f, 0x30, 0x16, 0x2a, 0x0f, 0x30, 0x16, 0x2a, 0x0f, 0x30, 0x16, 0x2a,
	0x0f, 0x30, 0x16, 0x2a, 0x0f, 0x30, 0x16, 0x2a, 0x0f, 0x30, 0x16, 0x2a, 0x0f, 0x30, 0x16, 0x2a];
var row:[32]u8;
var cnt:[16]u8;
public var frames:u8;

function main():void
{
	frame.init();
	frame.ctrl |= nes.CTRL_SPR_1000;
	for (var nt:u8 = 0; nt < 2; nt += 1) {
		for (var r:u8 = 0; r < 30; r += 1) {
			var v:u8 = r * 7 + nt * 11;
			for (var c:u8 = 0; c < 32; c += 1) {
				row[c] = 0x21 + (v & 63);
				v += 3;
			}
			vram.write_now(vram.addr(nt, 0, r), row);
		}
	}
	pal.set_all(PALETTE);
	frame.render_on();
	while (true) {
` + set + extra + `		frames += 1;
		frame.wait();
	}
}
`
}

func TestRasterIrqChain(t *testing.T) {
	t.Parallel()
	// 3 つの境目。60 フレームの後は 2 番目から下を消す
	p := buildNes(t, map[string]string{"fc.toml": rasterToml, "t.fc": rasterSrc([3]int{0, 0, 0}, [][4]int{{40, 1, 2, 1}, {100, 3, 4, 0}, {180, 5, 6, 1}}, false,
		"\t\tif (frames >= 60) {\n\t\t\traster.off(1);\n\t\t}\n")})
	p.run(t, 30)
	irq0 := p.Stats.IrqCount
	p.run(t, 20)
	if d := p.Stats.IrqCount - irq0; d < 58 || d > 62 {
		t.Errorf("3 つの境目の 20 フレームで IRQ が %d 回 (60 回のはず)", d)
	}
	p.run(t, 40) // 主の側は起動に約 10 フレームかかるので、off(1) が効くのは 70 フレームほどから
	irq0 = p.Stats.IrqCount
	p.run(t, 20)
	if d := p.Stats.IrqCount - irq0; d < 19 || d > 21 {
		t.Errorf("off(1) の後の 20 フレームで IRQ が %d 回 (20 回のはず)", d)
	}

	// 処理落ちする主の側: 主の側の 1 周は 1 フレームより長いが、NMI は毎フレーム仕掛ける
	q := buildNes(t, map[string]string{"fc.toml": rasterToml, "t.fc": rasterSrc([3]int{0, 0, 0}, [][4]int{{40, 1, 2, 1}, {100, 3, 4, 0}}, true, "")})
	q.run(t, 30)
	irq0, f0 := q.Stats.IrqCount, q.peek(t, "_t_frames", 0)
	q.run(t, 20)
	if d, loops := q.Stats.IrqCount-irq0, (q.peek(t, "_t_frames", 0)-f0+256)%256; d < 38 || d > 42 || loops >= 20 {
		t.Errorf("処理落ちする 20 フレーム (主の側は %d 周) で IRQ が %d 回 (40 回のはず)", loops, d)
	}
}

// rasterShot は src を作り、tool (fcmesen.py / fcretro.py) で frames フレーム動かした最後の画面。
func rasterShot(t *testing.T, py, tool string, args []string, src string, frames int) image.Image {
	t.Helper()
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"fc.toml": rasterToml, "t.fc": src})
	rom := filepath.Join(dir, "t.nes")
	if _, err := driver.NewCompiler(repoRoot).BuildContext(context.Background(), "t.fc", &driver.BuildOptions{
		Target: "nes", Dir: dir, BuildDir: filepath.Join(dir, "b"), Out: rom,
	}); err != nil {
		t.Fatal(err)
	}
	shot := filepath.Join(dir, "s.png")
	out, _ := runNesrun(t, py, filepath.Join(repoRoot, "tools", "nesrun", tool), append([]string{rom, "--frames", fmt.Sprint(frames), "--png", shot}, args...)...)
	f, err := os.Open(shot)
	if err != nil {
		t.Fatalf("画面が無い: %v\n%s", err, out)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func rowsEqual(a image.Image, ya int, b image.Image, yb int) bool {
	for x := 0; x < 256; x++ {
		r1, g1, b1, _ := a.At(x, ya).RGBA()
		r2, g2, b2, _ := b.At(x, yb).RGBA()
		if r1 != r2 || g1 != g2 || b1 != b2 {
			return false
		}
	}
	return true
}

func TestRasterSplit(t *testing.T) {
	py := python(t)
	type emu struct {
		name, tool string
		args       []string
	}
	var emus []emu
	if mesen := findMesen(); mesen != "" {
		ensureMesenSettings(t, mesen)
		emus = append(emus, emu{"Mesen", "fcmesen.py", []string{"--mesen", mesen}})
	}
	if core := os.Getenv("FC_QUICKNES"); core != "" {
		emus = append(emus, emu{"QuickNES", "fcretro.py", []string{"--core", core}})
	}
	if len(emus) == 0 {
		t.Skip("Mesen (FC_MESEN) も QuickNES (FC_QUICKNES) も無い")
	}
	cases := []struct {
		top    [3]int
		splits [][4]int // 走査線, x, y, ネームテーブル
		work   bool
	}{
		{[3]int{0, 0, 0}, [][4]int{{100, 13, 37, 1}}, false},
		{[3]int{0, 0, 0}, [][4]int{{2, 255, 239, 3}}, false},                                              // 最初の境目の下限、x / y の上限、ネームテーブル 3
		{[3]int{9, 3, 2}, [][4]int{{239, 1, 0, 1}}, false},                                                // 最後の走査線
		{[3]int{5, 9, 1}, [][4]int{{50, 250, 200, 0}, {120, 200, 5, 2}}, true},                            // 2 段 (IRQ の中で次を仕掛ける)、処理落ちする主の側
		{[3]int{0, 0, 0}, [][4]int{{30, 7, 100, 1}, {33, 0, 3, 0}}, false},                                // 境目の間が 3 走査線
		{[3]int{1, 2, 0}, [][4]int{{20, 3, 4, 1}, {80, 5, 6, 0}, {140, 7, 8, 1}, {200, 9, 10, 0}}, false}, // 4 段
	}
	for _, e := range emus {
		for ci, c := range cases {
			got := rasterShot(t, py, e.tool, e.args, rasterSrc(c.top, c.splits, c.work, ""), 40)
			// 期待: 上の帯は top のスクロールの画面、境目 B から下の k 行目は、その境目のスクロールの画面の k 行目
			refs := []image.Image{rasterShot(t, py, e.tool, e.args, rasterSrc(c.top, nil, false, ""), 40)}
			for _, s := range c.splits {
				refs = append(refs, rasterShot(t, py, e.tool, e.args, rasterSrc([3]int{s[1], s[2], s[3]}, nil, false, ""), 40))
			}
			var bad []int
			for y := 0; y < 240; y++ {
				band, from := 0, 0
				for i, s := range c.splits {
					if y >= s[0] {
						band, from = i+1, s[0]
					}
				}
				if !rowsEqual(got, y, refs[band], y-from) {
					bad = append(bad, y)
				}
			}
			if len(bad) > 0 {
				t.Errorf("%s: 例 %d (%v): 合わない走査線 %d 本 %v", e.name, ci, c.splits, len(bad), bad[:min(len(bad), 8)])
			}
		}
	}
}
