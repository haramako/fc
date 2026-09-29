package nes

// fclib/nes/font.chr (fc の内蔵のフォント) を fclib/nes/font.txt から作り、一致を確かめる。

import (
	"bufio"
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

var updateFont = flag.Bool("update-font", false, "fclib/nes/font.chr を font.txt から書き直す")

// fontChr は font.txt から 128 タイル (2 KB) の CHR を作る (タイルの番号 = 文字のコード。色 1)。
func fontChr(src []byte) ([]byte, error) {
	chr := make([]byte, 128*16)
	sc := bufio.NewScanner(bytes.NewReader(src))
	code := -1
	row := 0
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case line == "" || strings.HasPrefix(line, "#") && code < 0:
			continue
		case code < 0 || row == 7:
			f := strings.Fields(line)
			c, err := strconv.ParseUint(f[0], 16, 8)
			if err != nil || c >= 128 {
				return nil, err
			}
			code, row = int(c), 0
		default:
			var b byte
			for i, ch := range line {
				if ch == '#' {
					b |= 0x40 >> i // 5 ドットを 2〜6 列目に
				}
			}
			chr[code*16+1+row] = b // 上に 1 行あける (プレーン 0 だけ: 色 1)
			row++
		}
	}
	return chr, sc.Err()
}

func TestFontChr(t *testing.T) {
	dir := filepath.Join("..", "..", "fclib", "nes")
	src, err := os.ReadFile(filepath.Join(dir, "font.txt"))
	if err != nil {
		t.Fatal(err)
	}
	chr, err := fontChr(src)
	if err != nil {
		t.Fatal(err)
	}
	if chr['A'*16+1] != 0x38 || chr['A'*16+7] != 0x44 {
		t.Errorf("A の形: % x", chr['A'*16:'A'*16+16])
	}
	path := filepath.Join(dir, "font.chr")
	if *updateFont {
		if err := os.WriteFile(path, chr, 0o666); err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, chr) {
		t.Errorf("font.chr が font.txt と違う (go test ./internal/nes -run TestFontChr -update-font)")
	}
}
