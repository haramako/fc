package nes

// NES の stdio (fclib/nes/stdio.fc / stdio.asm) を内蔵ランナーで走らせ、ネームテーブルに書かれた文字を見る。

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/haramako/fc/internal/driver"
)

// TestNesStdioPrint: print と print_int16 (16 進 4 桁) がネームテーブルに並ぶ。print_int16 が文字列を古い渡し方
// (S+0,x) で fastcall の print に渡していて、数値でなく前の文字列が出ていた。16 進の表の D も C になっていた。
func TestNesStdioPrint(t *testing.T) {
	t.Parallel()
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	src := `#fc 3
use * from stdio;
var n:u16;
function main():void
{
	init();
	n = 0xABCD;
	print("HI ");
	print_int16(n);
	print(" ");
	print_int16(0x0F09);
	while (true) { }
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
	if err := m.RunFrames(5); err != nil {
		t.Fatal(err)
	}
	var nt []byte
	for a := 0x2000; a < 0x23c0; a++ {
		nt = append(nt, m.readVram(a))
	}
	if want := []byte("HI ABCD 0F09"); !bytes.Contains(nt, want) {
		t.Errorf("ネームテーブルに %q が無い: %q", want, bytes.Trim(nt, "\x00"))
	}
}
