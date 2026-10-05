package driver

// 新しい fclib (Agent/wiki/plans/v4-stdlib.md) のテスト: console (emu)、sys、fmt。

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
	console.write_z("z\n\0");
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

// TestMainReturnExitsEmu: emu では main から戻ると終了コード 0 で終わる (console.exit を呼ばなくても fcc run が終わる。
// fclib/emu/runtime_main_return.inc)。前は runtime の jmp * で止まったままだった。
func TestMainReturnExitsEmu(t *testing.T) {
	t.Parallel()
	r := testBuild(t, buildSpec{Files: map[string]string{"t.fc": `#fc 4
use console;
function main():void
{
	console.init();
	@printf("hi\n");
}
`}, Run: true, MaxCycles: 1_000_000})
	if r.Err != nil {
		t.Fatal(r.Err)
	}
	if r.Res.ExitCode != 0 || r.Stdout != "hi\n" {
		t.Errorf("exit %d, out %q; want 0, %q", r.Res.ExitCode, r.Stdout, "hi\n")
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

// TestLz4Random: 乱数で作ったデータ (圧縮の効き方の違うもの) を @lz4(@incbin(...)) でコンパイル時に圧縮し、lz4.unpack で
// 展開して元のデータと比べる (-O 0 / -O 2)。1 バイトの展開のサイクル数も出す。
func TestLz4Random(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewSource(1))
	for k, n := range []int{1, 13, 200, 1000, 4000} {
		data := make([]byte, n)
		switch k % 3 {
		case 0:
			r.Read(data)
		case 1:
			for i := range data {
				data[i] = byte(r.Intn(3))
			}
		default:
			for i := range data {
				data[i] = "fc lz4 fc lz4 "[i%14] ^ byte(r.Intn(8)/7)
			}
		}
		src := `#fc 4
use console;
use lz4;
const RAW = @incbin("d.bin");
const PACKED = @lz4(@incbin("d.bin"));
var buf:[4100]u8 @(segment: "BSS_EX");
function main():void
{
	console.bench_start();
	var n = lz4.unpack(buf, PACKED);
	console.bench_end();
	var bad:u16 = 0;
	for (var i:u16 = 0; i < @len(RAW); i += 1) {
		if (buf[i] != RAW[i]) {
			bad += 1;
		}
	}
	@printf("{} {} {}\n", n, bad, @len(PACKED));
	console.exit(0);
}
`
		for _, level := range []int{-1, 0} {
			res := testBuild(t, buildSpec{Files: map[string]string{"t.fc": src, "d.bin": string(data)}, Run: true, Level: level})
			if res.Err != nil {
				t.Fatalf("n=%d: %v", n, res.Err)
			}
			var gotN, bad, packed int
			fmt.Sscan(res.Stdout, &gotN, &bad, &packed)
			if gotN != n || bad != 0 {
				t.Errorf("n=%d (-O %d): %q", n, level, res.Stdout)
			}
			if level == 0 {
				t.Logf("%d バイト → %d バイト、展開 %d サイクル (1 バイト %.1f)", n, packed, res.Res.Cycles, float64(res.Res.Cycles)/float64(n))
			}
		}
	}
}

// lzwPack は fclib/lzw.asm の形式に圧縮する (テスト用。fc に LZW の圧縮は無い): 3 バイト以上の一致は距離・長さ (255 まで) に。
func lzwPack(data []byte) []byte {
	var out []byte
	nbits := 0
	put := func(v, n int) {
		for i := n - 1; i >= 0; i-- {
			if nbits%8 == 0 {
				out = append(out, 0)
			}
			if v>>i&1 != 0 {
				out[len(out)-1] |= 0x80 >> (nbits % 8)
			}
			nbits++
		}
	}
	vln := func(v, short, long int) {
		if v < 1<<short {
			put(0, 1)
			put(v, short)
		} else {
			put(1, 1)
			put(v, long)
		}
	}
	vln(len(data), 8, 16)
	for i := 0; i < len(data); {
		best, dist := 0, 0
		for d := 1; d <= 255 && d <= i; d++ {
			n := 0
			for n < 255 && i+n < len(data) && data[i+n] == data[i+n-d] {
				n++
			}
			if n > best {
				best, dist = n, d
			}
		}
		if best >= 3 {
			put(0, 1)
			vln(dist, 4, 8)
			vln(best, 4, 8)
			i += best
		} else {
			put(1, 1)
			put(int(data[i]), 8)
			i++
		}
	}
	return out
}

// TestLzwRandom: lzw.unpack を乱数のデータ (lzwPack で圧縮) で確かめる。書き先がちょうどの長さでも、1 バイト足りなければ
// try_unpack が失敗を返すことも。
func TestLzwRandom(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewSource(1))
	for k, n := range []int{1, 13, 200, 1000, 3000} {
		data := make([]byte, n)
		switch k % 3 {
		case 0:
			r.Read(data)
		case 1:
			for i := range data {
				data[i] = byte(r.Intn(3))
			}
		default:
			for i := range data {
				data[i] = "fc lzw fc lzw "[i%14] ^ byte(r.Intn(8)/7)
			}
		}
		src := fmt.Sprintf(`#fc 4
use console;
use lzw;
const RAW = @incbin("d.bin");
const PACKED = @incbin("p.bin");
var buf:[%d]u8 @(segment: "BSS_EX");
function main():void
{
	console.bench_start();
	var n = lzw.unpack(buf, PACKED);
	console.bench_end();
	var bad:u16 = 0;
	for (var i:u16 = 0; i < @len(RAW); i += 1) {
		if (buf[i] != RAW[i]) {
			bad += 1;
		}
	}
	@printf("{} {} {}\n", n, bad, lzw.try_unpack(buf[..@len(RAW) - 1], PACKED));
	console.exit(0);
}
`, n)
		packed := lzwPack(data)
		for _, level := range []int{-1, 0} {
			res := testBuild(t, buildSpec{Files: map[string]string{"t.fc": src, "d.bin": string(data), "p.bin": string(packed)}, Run: true, Level: level})
			if res.Err != nil {
				t.Fatalf("n=%d: %v", n, res.Err)
			}
			if want := fmt.Sprintf("%d 0 65535\n", n); res.Stdout != want {
				t.Errorf("n=%d (-O %d): got %q, want %q", n, level, res.Stdout, want)
			}
			if level == 0 {
				t.Logf("%d バイト → %d バイト、展開 %d サイクル (1 バイト %.1f)", n, len(packed), res.Res.Cycles, float64(res.Res.Cycles)/float64(n))
			}
		}
	}
}

// TestMemSpeed: mem.fill / copy / move (重なりを後ろから) / zero の 1000 バイトのサイクル数を出し、中身を確かめる (-O 0 / -O 2)。
// asm の版の速さの退行を見張る (上限は 1 バイトあたり)。
func TestMemSpeed(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name, call string
		limit      float64 // 1 バイトあたりのサイクル数の上限
	}{
		{"fill", "mem.fill(a, 7);", 12},
		{"copy", "mem.copy(b, a);", 20},
		{"move", "mem.move(a[1..], a);", 22},
		{"zero", "mem.zero(b);", 12},
	} {
		src := `#fc 4
use console;
use mem;
var a:[1000]u8 @(segment: "BSS_EX");
var b:[1000]u8 @(segment: "BSS_EX");
function main():void
{
	for (var i:u16 = 0; i < 1000; i += 1) {
		a[i] = i as u8;
		b[i] = 0x55;
	}
	console.bench_start();
	` + c.call + `
	console.bench_end();
	var sa:u16 = 0;
	var sb:u16 = 0;
	for (var i:u16 = 0; i < 1000; i += 1) {
		sa = (sa << 1 | sa >> 15) ^ a[i];
		sb = (sb << 1 | sb >> 15) ^ b[i];
	}
	@printf("{} {}\n", sa, sb);
	console.exit(0);
}
`
		// 参照 (Go)
		a, b := make([]byte, 1000), make([]byte, 1000)
		for i := range a {
			a[i], b[i] = byte(i), 0x55
		}
		switch c.name {
		case "fill":
			for i := range a {
				a[i] = 7
			}
		case "copy":
			copy(b, a)
		case "move":
			copy(a[1:], a)
		case "zero":
			clear(b)
		}
		sum := func(x []byte) uint16 {
			s := uint16(0)
			for _, v := range x {
				s = (s<<1 | s>>15) ^ uint16(v)
			}
			return s
		}
		want := fmt.Sprintf("%d %d\n", sum(a), sum(b))
		for _, level := range []int{-1, 0} {
			res := testBuild(t, buildSpec{Files: map[string]string{"t.fc": src}, Run: true, Level: level})
			if res.Err != nil {
				t.Fatalf("%s: %v", c.name, res.Err)
			}
			if res.Stdout != want {
				t.Errorf("%s (-O %d): %q, want %q", c.name, level, res.Stdout, want)
			}
			if level == 0 {
				per := float64(res.Res.Cycles) / 1000
				t.Logf("%s: 1000 バイトで %d サイクル (1 バイト %.1f)", c.name, res.Res.Cycles, per)
				if per > c.limit {
					t.Errorf("%s: 1 バイト %.1f サイクル (上限 %.0f)", c.name, per, c.limit)
				}
			}
		}
	}
}
