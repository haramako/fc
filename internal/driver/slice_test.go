package driver

// fc 3 の slice (`[]T` / `[]const T`、a[lo..hi]、@len / @slice / @ptr / @copy) のテスト。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// buildBothLevels は files を -O 0 と -O 2 でビルドして走らせ、出力が同じならそれを返す (違えばテストの失敗)。
func buildBothLevels(t *testing.T, files map[string]string) (string, error) {
	t.Helper()
	var outs [2]string
	for i, level := range []int{-1, 0} {
		dir := t.TempDir()
		for name, src := range files {
			if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o666); err != nil {
				t.Fatal(err)
			}
		}
		var out strings.Builder
		_, err := NewCompiler(absRepoRoot).Build("t.fc", &BuildOptions{Dir: dir, BuildDir: filepath.Join(dir, "b"), Out: filepath.Join(dir, "a.bin"), Run: true, Stdout: &out, MaxCycles: 10_000_000, OptimizeLevel: level})
		if err != nil {
			return "", err
		}
		outs[i] = out.String()
	}
	if outs[0] != outs[1] {
		t.Errorf("-O 0 と -O 2 で出力が違う:\n-O 0: %q\n-O 2: %q", outs[0], outs[1])
	}
	return outs[1], nil
}

// TestSliceBasics: 配列 → slice の暗黙の変換、添字の読み書き、範囲、長さ、引数・戻り値・struct のフィールド。
func TestSliceBasics(t *testing.T) {
	t.Parallel()
	out, err := buildBothLevels(t, map[string]string{
		"t.fc": `#fc 3
use * from stdio;
use mem;

struct Buf { data:[]u8; tag:u8; }
struct P { x:u8; y:u16; }

const TAB = [10, 20, 30, 40, 50];
const PS:[3]P = [{1, 100}, {2, 200}, {3, 300}];
var gbuf:[8]u8;
var gs:[]u8;

function sum(s:[]const u8):u16
{
	var n:u16 = 0;
	for (var i = 0; i < @len(s); i += 1) {
		n += s[i];
	}
	return n;
}

function fill(s:[]u8, v:u8):void
{
	for (var i = 0; i < @len(s); i += 1) {
		s[i] = v + i;
	}
}

function tail(s:[]const u8):[]const u8
{
	return s[1..];
}

function ysum(ps:[]const P):u16
{
	var n:u16 = 0;
	for (var i = 0; i < @len(ps); i += 1) {
		n += ps[i].y;
	}
	return n;
}

function main():void
{
	var local:[6]u8;
	printf(sum(TAB), " ", @len(TAB), " ", sum(TAB[1..3]), " ", sum(TAB[..2]), " ", sum(TAB[3..]), "\n");
	fill(local, 5);
	printf(sum(local), " ", local[5], "\n");
	fill(local[2..4], 100);
	printf(local[1], " ", local[2], " ", local[3], " ", local[4], "\n");
	var s:[]const u8 = "hello";
	printf(@len(s), " ", @len("abc"), " ", s[4], " ", sum(tail(tail(s))), "\n");
	var lo = 1;
	var hi = 4;
	var t = TAB[lo..hi];
	printf(@len(t), " ", t[0], " ", sum(t), "\n");
	var u = t[lo..];
	printf(@len(u), " ", u[0], "\n");
	gs = gbuf;
	fill(gs, 1);
	printf(gbuf[7], " ", @len(gs), "\n");
	var b:Buf = {gbuf[2..5], 9};
	printf(@len(b.data), " ", b.data[0], " ", b.tag, "\n");
	printf(ysum(PS), " ", ysum(PS[1..]), "\n");
	var p = @ptr(t);
	printf(p[0], " ", *p, "\n");
	var w = @slice(&local[1], 3);
	printf(@len(w), " ", sum(w), "\n");
	var n = @copy(local, TAB);
	printf(n, " ", local[0], " ", local[4], " ", local[5], "\n");
	n = @copy(gbuf[..2], t);
	printf(n, " ", gbuf[0], " ", gbuf[1], " ", gbuf[2], "\n");
	exit(0);
}
`,
	})
	want := "150 5 50 30 90\n" +
		"45 10\n" +
		"6 100 101 9\n" +
		"5 3 111 327\n" +
		"3 20 90\n" +
		"2 30\n" +
		"8 8\n" +
		"3 3 9\n" +
		"600 500\n" +
		"20 20\n" +
		"3 207\n" +
		"5 10 50 10\n" +
		"2 20 30 3\n"
	if err != nil || out != want {
		t.Errorf("got %q, %v\nwant %q", out, err, want)
	}
}

// TestSliceErrors: 型と定数の範囲の検査。
func TestSliceErrors(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ src, msg string }{
		{`const A = [1, 2, 3]; function main():void { var s:[]u8 = A[1..4]; }`, "hi (4) is out of range"},
		{`const A = [1, 2, 3]; function main():void { var s:[]u8 = A[2..1]; }`, "lo (2) > hi (1)"},
		{`const A = [1, 2, 3]; function main():void { var s:[]u16 = A; }`, "element types differ"},
		{`var A:[300]u8; function main():void { var s:[]u8 = A; }`, "at most 255 elements"},
		{`function main():void { var p:*u8; var s:[]u8 = p; }`, "not compatible"},
		{`function main():void { var p:*u8; var s = p[0..2]; }`, "not an array or a slice"},
		{`const A = [1, 2]; function f(s:[]u8):void {} function main():void { var s:[]const u8 = A; s[0] = 1; }`, "read-only"},
		{`const S:[]u8 = [1, 2];`, "a slice is a run-time value"},
		{`const S:[]u8 @(symbol: "_s");`, "a slice is a run-time value"},
		{`use mem; const A = [1, 2]; function main():void { var s:[]const u8 = A; @copy(s, A); }`, "destination is read-only"},
		{`function main():void { var a:[2]u8; var s:[]u8 = a; @copy(s, a); }`, "requires the mem module"},
	} {
		_, err := buildFiles(t, map[string]string{"t.fc": "#fc 3\n" + c.src + "\n"})
		if err == nil || !strings.Contains(err.Error(), c.msg) {
			t.Errorf("%s: got %v, want %q", c.src, err, c.msg)
		}
	}
}

// TestSliceMore: @len は定数の文脈 (配列の長さ) でも使える、enum のメンバーの数、ポインタ経由の配列・slice のフィールド、
// 読み取り専用のデータを []T に入れると警告。
func TestSliceMore(t *testing.T) {
	t.Parallel()
	out, res, err := buildFilesDefs(t, map[string]string{
		"t.fc": `#fc 3
use * from stdio;

enum Color { Red, Green, Blue }
struct Box { vals:[4]u8; s:[]u8; }
const TAB = [1, 2, 3];
var buf:[@len(TAB) + @len(Color)]u8;
var box:Box;

function total(s:[]u8):u8
{
	var n = 0;
	for (var i = 0; i < @len(s); i += 1) {
		n += s[i];
	}
	return n;
}

function main():void
{
	printf(@len(buf), " ", @len(Color), "\n");
	var p = &box;
	p.vals[0] = 7;
	p.vals[3] = 8;
	p.s = p.vals;
	p.s[1] = 1;
	printf(total(p.vals), " ", @len(p.s), " ", total(p.s[1..]), " ", p.s[1..][2], "\n");
	var w:[]u8 = TAB;
	printf(total(w), "\n");
	exit(0);
}
`,
	}, nil)
	if err != nil || out != "6 3\n16 4 9 8\n6\n" {
		t.Errorf("got %q, %v", out, err)
	}
	found := false
	for _, w := range res.Warnings {
		found = found || strings.Contains(w.Msg, "passes read-only data as []u8 (use []const u8)")
	}
	if !found {
		t.Errorf("const の配列を []u8 に入れた警告が無い: %v", res.Warnings)
	}
}

// TestWideSlice: 長さ u16 の広い slice `[:u16]T`。256 要素以上の配列、普通 → 広いの暗黙の変換、u16 の範囲、@copy。
func TestWideSlice(t *testing.T) {
	t.Parallel()
	out, err := buildBothLevels(t, map[string]string{
		"t.fc": `#fc 3
use * from stdio;
use mem;

var big:[600]u8;
var dst:[600]u8;
const SMALL = [1, 2, 3];

function sum(s:[:u16]const u8):u16
{
	var n:u16 = 0;
	for (var i:u16 = 0; i < @len(s); i += 1) {
		n += s[i];
	}
	return n;
}

function main():void
{
	for (var i:u16 = 0; i < 600; i += 1) {
		big[i] = i as u8;
	}
	var w:[:u16]u8 = big;
	printf(@len(w), " ", sum(w), " ", sum(SMALL), " ", @len(big[10..]), "\n");
	var lo:u16 = 250;
	var hi:u16 = 520;
	var part = w[lo..hi];
	printf(@len(part), " ", part[0], " ", part[269], " ", sum(big[256..260]), "\n");
	var n = @copy(dst, w[100..]);
	printf(n, " ", dst[0], " ", dst[499], " ", dst[500], "\n");
	var s:[]u8 = SMALL;
	var ws:[:u16]u8 = s;
	printf(@len(ws), " ", ws[2], " ", @len(@slice(&big[0], 300 as u16)), "\n");
	exit(0);
}
`,
	})
	// sum(big) = 0..255 + 0..255 + 0..87 = 2*32640 + 3828 = 69108 → u16 で 3572
	want := "600 3572 6 590\n" +
		"270 250 7 6\n" +
		"500 100 87 0\n" +
		"3 3 300\n"
	if err != nil || out != want {
		t.Errorf("got %q, %v\nwant %q", out, err, want)
	}
	for _, c := range []struct{ src, msg string }{
		{`var w:[:u16]u8; function main():void { var s:[]u8 = w; }`, "cannot use [:u16]u8 as []u8 implicitly"},
		{`var a:[300]u8; function main():void { var s:[]u8 = a; }`, "use [:u16]u8 for longer ones"},
		{`var a:[300]u8; function main():void { var s:[]u8 = a[..]; }`, "cannot use [:u16]u8 as []u8 implicitly"},
		{`function f(s:[:i16]u8):void {} function main():void {}`, "must be u8 or u16"},
	} {
		_, err := buildFiles(t, map[string]string{"t.fc": "#fc 3\n" + c.src + "\n"})
		if err == nil || !strings.Contains(err.Error(), c.msg) {
			t.Errorf("%s: got %v, want %q", c.src, err, c.msg)
		}
	}
}
