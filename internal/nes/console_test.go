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

// TestNesPanicWhileRendering: console.init を呼ばずに描画中のゲームが panic すると、描画と NMI を止めてネームテーブル 0 を
// 空白にしてから文言を書く (console.claim。前は描画中の PPU に書いて画面に何も出なかった)。
func TestNesPanicWhileRendering(t *testing.T) {
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
	if err := m.RunFrames(20); err != nil {
		t.Fatal(err)
	}
	row := func(y int) string {
		var b []byte
		for a := 0x2000 + y*32; a < 0x2000+(y+1)*32; a++ {
			b = append(b, m.readVram(a))
		}
		return string(b)
	}
	if got := strings.TrimRight(row(0), " "); got != "panic: boom" {
		t.Errorf("1 行目: %q", got)
	}
	if got := row(5); got != strings.Repeat(" ", 32) {
		t.Errorf("ゲームの画面が残っている: %q", got)
	}
	if m.mask&0x08 == 0 {
		t.Errorf("背景を出していない: mask = %#x", m.mask)
	}
	syms := parseLd65Map(t, strings.TrimSuffix(rom, ".nes")+".dbg", "_console_exited", "_console_exit_code")
	if m.Get(syms["_console_exited"]) != 1 || m.Get(syms["_console_exit_code"]) != 1 {
		t.Errorf("exited = %d, exit_code = %d", m.Get(syms["_console_exited"]), m.Get(syms["_console_exit_code"]))
	}
}
