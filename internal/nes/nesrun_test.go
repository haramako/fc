package nes

// tools/nesrun の fcmesen.py (Mesen のヘッドレス実行で @log・console の出力・panic) と fcretro.py (libretro の QuickNES で画面・
// 入力・RAM) を実際に動かす。Python と Mesen / QuickNES のコアがあるときだけ (FC_MESEN・FC_QUICKNES。無ければ Skip)。

import (
	"context"
	"errors"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haramako/fc/internal/driver"
)

// nesrunSrc: 右を押すたびに @log、3 回で sys.panic
const nesrunSrc = `#fc 4
use sys;
use pad;
use frame;
use vram;
public var n:u8;
function main():void
{
	frame.init();
	vram.write_now(vram.addr(0, 1, 2), "HELLO");
	frame.render_on();
	while (true) {
		frame.wait();
		pad.poll();
		if ((pad.p1.pressed & pad.RIGHT) != 0) {
			@log("right n={}", n);
			n += 1;
		}
		if (n == 3) {
			sys.panic("three");
		}
	}
}
`

func python(t *testing.T) string {
	t.Helper()
	for _, p := range []string{"python3", "python"} {
		if path, err := exec.LookPath(p); err == nil {
			return path
		}
	}
	t.Skip("python が無い")
	return ""
}

// nesrunBuild は nesrunSrc を -g の NES の ROM にする (ROM の隣に .dbg と .fclog.lua)。
func nesrunBuild(t *testing.T) (repoRoot, rom string) {
	t.Helper()
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"t.fc": nesrunSrc})
	rom = filepath.Join(dir, "t.nes")
	if _, err := driver.NewCompiler(repoRoot).BuildContext(context.Background(), "t.fc", &driver.BuildOptions{
		Target: "nes", Dir: dir, BuildDir: filepath.Join(dir, "b"), Out: rom, Debug: true,
	}); err != nil {
		t.Fatal(err)
	}
	return repoRoot, rom
}

func runNesrun(t *testing.T, py, script string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(py, append([]string{script}, args...)...)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	code := 0
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return strings.ReplaceAll(string(out), "\r\n", "\n"), code
}

func TestNesrunMesen(t *testing.T) {
	py := python(t)
	mesen := findMesen()
	if mesen == "" {
		t.Skip("MesenCE が見つからない (FC_MESEN)")
	}
	ensureMesenSettings(t, mesen)
	repoRoot, rom := nesrunBuild(t)
	tool := filepath.Join(repoRoot, "tools", "nesrun", "fcmesen.py")
	out, code := runNesrun(t, py, tool, rom, "--mesen", mesen, "--input", "10:right,20:right,30:right", "--frames", "120", "--peek", "t.n")
	want := "right n=0\nright n=1\nright n=2\npanic: three\n[exit 1]\n[peek] t.n = 3 (0x03)\n"
	if out != want || code != 1 {
		t.Errorf("panic まで: code %d\n%s", code, out)
	}
	out, code = runNesrun(t, py, tool, rom, "--mesen", mesen, "--input", "10:right", "--frames", "60", "--until", "n=0")
	if out != "right n=0\n" || code != 0 {
		t.Errorf("--until: code %d\n%s", code, out)
	}
	out, code = runNesrun(t, py, tool, rom, "--mesen", mesen, "--frames", "30", "--until", "never")
	if out != "[frames 30]\n" || code != 2 {
		t.Errorf("フレームが尽きる: code %d\n%s", code, out)
	}
	// ネームテーブルとパレット (PPU を副作用なしに読む)
	out, _ = runNesrun(t, py, tool, rom, "--mesen", mesen, "--frames", "5", "--nametable", "0", "--nametable", "0:hex", "--palette")
	for _, w := range []string{"[nt0 02] |.HELLO" + strings.Repeat(".", 26) + "|\n", "[nt0 02] 00 48 45 4c 4c 4f 00 ", "[palette] "} {
		if !strings.Contains(out, w) {
			t.Errorf("--nametable / --palette に %q が無い:\n%s", w, out)
		}
	}
}

func TestNesrunRetro(t *testing.T) {
	py := python(t)
	core := os.Getenv("FC_QUICKNES")
	if core == "" {
		for _, p := range []string{`C:\Applications\libretro\quicknes_libretro.dll`, filepath.Join(os.Getenv("HOME"), ".local/lib/libretro/quicknes_libretro.so")} {
			if _, err := os.Stat(p); err == nil {
				core = p
			}
		}
	}
	if core == "" {
		t.Skip("QuickNES のコアが見つからない (FC_QUICKNES)")
	}
	repoRoot, rom := nesrunBuild(t)
	shot := filepath.Join(t.TempDir(), "s.png")
	out, code := runNesrun(t, py, filepath.Join(repoRoot, "tools", "nesrun", "fcretro.py"), rom, "--core", core,
		"--input", "10:right,20:right", "--frames", "40", "--watch", "t.n", "--png", shot)
	// n は 0 で始まり (グローバルの初期化)、右を押した次のフレームから増える。始めの RAM はコアの初期値
	if code != 0 || !strings.Contains(out, "] t.n = 1 (0x01)\n") || !strings.HasSuffix(out, "] t.n = 2 (0x02)\n") {
		t.Errorf("code %d\n%s", code, out)
	}
	f, err := os.Open(shot)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 256 || b.Dy() != 240 {
		t.Errorf("画面の大きさ %v", b)
	}
}
