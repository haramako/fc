package nes

// NES の console (fclib/nes/console.fc) を内蔵ランナーで走らせ、ネームテーブルに書かれた文字と終了コードを見る。

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haramako/fc/internal/driver"
)

// TestNesConsole: write / newline / write_z がネームテーブル 0 に並び ('\n' で次の行の頭へ)、exit が終了コードを残す。
// stdio を使わないので、割り込みの入口は fc が生成した空のもの。
func TestNesConsole(t *testing.T) {
	t.Parallel()
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	src := `#fc 4
use console;
function main():void
{
	console.init();
	console.write("HELLO");
	console.newline();
	console.write_z("WORLD\0");
	console.exit(3);
}
`
	if err := os.WriteFile(filepath.Join(dir, "t.fc"), []byte(src), 0o666); err != nil {
		t.Fatal(err)
	}
	rom := filepath.Join(dir, "t.nes")
	if _, err := driver.NewCompiler(repoRoot).BuildContext(context.Background(), "t.fc", &driver.BuildOptions{
		Target: "nes", Dir: dir, BuildDir: filepath.Join(dir, "b"), Out: rom,
	}); err != nil {
		t.Fatalf("ビルド失敗: %v", err)
	}
	m, err := LoadFile(rom)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.RunFrames(10); err != nil {
		t.Fatal(err)
	}
	row := func(y int) []byte {
		var b []byte
		for a := 0x2000 + y*32; a < 0x2000+(y+1)*32; a++ {
			b = append(b, m.readVram(a))
		}
		return bytes.TrimRight(b, "\x00")
	}
	if got := string(row(0)); got != "HELLO" {
		t.Errorf("1 行目: %q", got)
	}
	if got := string(row(1)); got != "WORLD" {
		t.Errorf("2 行目: %q", got)
	}
	syms := parseLd65Map(t, strings.TrimSuffix(rom, ".nes")+".dbg", "_console_exited", "_console_exit_code")
	if m.Get(syms["_console_exited"]) != 1 || m.Get(syms["_console_exit_code"]) != 3 {
		t.Errorf("exited = %d, exit_code = %d", m.Get(syms["_console_exited"]), m.Get(syms["_console_exit_code"]))
	}
}

// TestNesPanic: NES の sys.panic は console を使わない (固定の所に console が入らない。GitHub の Issue #19)。理由を sys.msg に
// 残し、NMI を止めてその場で止まる (描画中のゲームの画面はそのまま)。ランナーには $4018 で "panic: 理由" を、$4019 で終了コード 1 を渡す。
func TestNesPanic(t *testing.T) {
	t.Parallel()
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	src := `#fc 4
use frame;
use vram;
use sys;
function main():void
{
	frame.init();
	vram.write_now(vram.addr(0, 0, 5), "GAME SCREEN");
	frame.render_on();
	for (var i:u8 = 0; i < 3; i += 1) {
		frame.wait();
	}
	sys.panic("boom");
}
`
	if err := os.WriteFile(filepath.Join(dir, "t.fc"), []byte(src), 0o666); err != nil {
		t.Fatal(err)
	}
	rom := filepath.Join(dir, "t.nes")
	if _, err := driver.NewCompiler(repoRoot).BuildContext(context.Background(), "t.fc", &driver.BuildOptions{
		Target: "nes", Dir: dir, BuildDir: filepath.Join(dir, "b"), Out: rom,
	}); err != nil {
		t.Fatalf("ビルド失敗: %v", err)
	}
	m, err := LoadFile(rom)
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	m.Output = &out
	if err := m.RunFrames(20); err != nil {
		t.Fatal(err)
	}
	if out.String() != "panic: boom\n" || !m.Exited || m.ExitCode != 1 {
		t.Errorf("ランナーへの出力 %q、exited = %v、exit code = %d", out.String(), m.Exited, m.ExitCode)
	}
	if m.ctrl&0x80 != 0 {
		t.Errorf("NMI を止めていない: ctrl = %#x", m.ctrl)
	}
	var row []byte
	for a := 0x2000 + 5*32; a < 0x2000+6*32; a++ {
		row = append(row, m.readVram(a))
	}
	if !strings.HasPrefix(string(row), "GAME SCREEN") {
		t.Errorf("ゲームの画面が消えた: %q", row)
	}
	syms := parseLd65Map(t, strings.TrimSuffix(rom, ".nes")+".dbg", "_sys_msg")
	if n := m.Get(syms["_sys_msg"]+2) + m.Get(syms["_sys_msg"]+3)<<8; n != 4 {
		t.Errorf("sys.msg の長さ = %d", n)
	}
	mp, err := os.ReadFile(strings.TrimSuffix(rom, ".nes") + ".map")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(mp, []byte("\nconsole ")) {
		t.Errorf("console が入っている")
	}
}
