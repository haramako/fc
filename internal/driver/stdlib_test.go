package driver

// 新しい fclib (doc/v4_stdlib.md) のテスト: console (emu)、sys、fmt。

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
)

// TestConsoleEmu: console.write は長さの分だけ出す (途中の 0 も)。write_z は終端 0 まで。stdio を使わないので割り込みの入口は
// fc が生成した空のもの。
func TestConsoleEmu(t *testing.T) {
	t.Parallel()
	out, err := buildBothLevels(t, map[string]string{"t.fc": `#fc 4
use console;
function main():void
{
	console.write("ab");
	var a = [65, 0, 66];
	console.write(a);
	console.newline();
	console.write_z("z\n");
	console.exit(0);
}
`})
	if err != nil {
		t.Fatal(err)
	}
	if want := "abA\x00B\nz\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// TestFmtReference: fmt の数の変換を Go の fmt と比べる (乱数の値・幅・埋め)。
func TestFmtReference(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewSource(1))
	type fn struct {
		name, verb string
		lo, hi     int64
		maxWidth   int
		flags      int // spec に足すビット (0x40 = 16 進の小文字)
	}
	fns := []fn{
		{"dec_u8", "d", 0, 255, 6, 0}, {"dec_u16", "d", 0, 65535, 8, 0}, {"dec_i8", "d", -128, 127, 6, 0}, {"dec_i16", "d", -32768, 32767, 8, 0},
		{"hex_u8", "X", 0, 255, 5, 0}, {"hex_u16", "x", 0, 65535, 6, 0x40}, {"bin_u8", "b", 0, 255, 10, 0}, {"bin_u16", "b", 0, 65535, 18, 0},
	}
	var src, want strings.Builder
	src.WriteString("#fc 4\nuse console;\nuse fmt;\nvar buf:[20]u8;\nfunction show():void { console.write(buf[..fmt.at]); console.newline(); }\n")
	const perFunc = 20 // 呼び出しの引数の一時変数で静的フレームが 256 バイトを超えないように、関数を分ける
	var lines []string
	for i := 0; i < 240; i++ {
		if i%perFunc == 0 {
			if i > 0 {
				src.WriteString("}\n")
			}
			fmt.Fprintf(&src, "function part%d():void\n{\n", i/perFunc)
		}
		f := fns[i%len(fns)]
		v := f.lo + r.Int63n(f.hi-f.lo+1)
		switch r.Intn(6) { // 端の値も混ぜる
		case 0:
			v = f.lo
		case 1:
			v = f.hi
		}
		width, zero := r.Intn(f.maxWidth+1), r.Intn(2) == 0
		spec := width | f.flags
		if zero {
			spec |= 0x20
		}
		line := fmt.Sprintf("\tfmt.begin(buf); fmt.%s(%d, %d); show();", f.name, v, spec)
		lines = append(lines, line)
		src.WriteString(line + "\n")
		gs := "%"
		if zero {
			gs += "0"
		}
		if width > 0 {
			gs += fmt.Sprint(width)
		}
		fmt.Fprintf(&want, gs+f.verb+"\n", v)
	}
	src.WriteString("}\nfunction main():void\n{\n")
	for k := 0; k < 240/perFunc; k++ {
		fmt.Fprintf(&src, "\tpart%d();\n", k)
	}
	src.WriteString("\tconsole.exit(0);\n}\n")
	out, err := buildBothLevels(t, map[string]string{"t.fc": src.String()})
	if err != nil {
		t.Fatal(err)
	}
	gl, wl := strings.Split(out, "\n"), strings.Split(want.String(), "\n")
	for i := range wl {
		if i >= len(gl) || gl[i] != wl[i] {
			got := "(無い)"
			if i < len(gl) {
				got = gl[i]
			}
			t.Fatalf("%d 行目: got %q, want %q\n%s", i+1, got, wl[i], lines[min(i, len(lines)-1)])
		}
	}
}

// TestFmtTooSmall: 書き先が足りなければ sys.panic で止まる (終了コード 1)。
func TestFmtTooSmall(t *testing.T) {
	t.Parallel()
	r := testBuild(t, buildSpec{Run: true, Files: map[string]string{"t.fc": `#fc 4
use console;
use fmt;
var buf:[4]u8;
function main():void
{
	fmt.begin(buf[..2]);
	fmt.dec_u16(12345, 0);
	console.write(buf[..fmt.at]);
	console.exit(0);
}
`}})
	if r.Res == nil || r.Res.ExitCode != 1 || r.Stdout != "panic: fmt: the buffer is too small\n" {
		t.Errorf("err = %v, stdout = %q", r.Err, r.Stdout)
	}
}
