package driver

import (
	"fmt"
	"strings"
	"testing"

	"github.com/haramako/fc/internal/ir"
)

// procBody は asm の中の関数 name (.proc name から .endproc まで) の命令の行 (注釈を除く)。
func procBody(t *testing.T, asm, name string) []string {
	t.Helper()
	i := strings.Index(asm, ".proc "+name+"\n")
	if i < 0 {
		i = strings.Index(asm, ".proc "+name+"\r\n")
	}
	if i < 0 {
		t.Fatalf("%s が無い", name)
	}
	body := asm[i:]
	body = body[:strings.Index(body, ".endproc")]
	var lines []string
	for _, l := range strings.Split(body, "\n")[1:] {
		l = strings.TrimSpace(l)
		if l != "" && !strings.HasPrefix(l, ";") {
			lines = append(lines, l)
		}
	}
	return lines
}

// buildShape は fc 4 のプログラムを -O 0 と -O 2 で走らせて出力を比べ、-O 2 の出力とアセンブリ (_t.s) を返す。
func buildShape(t *testing.T, src string) (string, string) {
	t.Helper()
	files := map[string]string{"t.fc": src}
	out, err := buildBothLevels(t, files)
	if err != nil {
		t.Fatal(err)
	}
	r := testBuild(t, buildSpec{Files: files, CompileOnly: true})
	if r.Err != nil {
		t.Fatal(r.Err)
	}
	return out, r.Built(t, "_t.s")
}

// TestInlineBoolBranch: インライン展開した bool の関数の結果 (`!on()`) は 0 / 1 を作らずにフラグのまま分岐する
// (regalloc.allocateCond が $result を計算の一時の値より先に見ていた)。
func TestInlineBoolBranch(t *testing.T) {
	t.Parallel()
	out, asm := buildShape(t, `#fc 4
use console;
var mask:u8;
var cnt:u8;
function on():bool
{
	return (mask & 0x18) != 0;
}
function f():void @(noinline)
{
	if (!on()) {
		cnt += 1;
		return;
	}
	cnt += 2;
}
function main():void
{
	f();
	mask = 8;
	f();
	mask = 0x40;
	f();
	printf("{}\n", cnt);
	console.exit(0);
}
`)
	if out != "4\n" {
		t.Errorf("got %q", out)
	}
	body := strings.Join(procBody(t, asm, "_t_f"), "\n")
	if strings.Contains(body, "lda #1") || strings.Contains(body, "jmp") {
		t.Errorf("bool を 0 / 1 にしている:\n%s", body)
	}
}

// TestAssembleInPlace: 部分ごとに組み立てた slice を丸ごと写さずに直に書く (opt.assembleInPlace)。組み立ての間に写し先を
// 読む形 (長さを縮める・範囲の端に写し先を使う・入れ替え) も -O 0 と同じ結果。
func TestAssembleInPlace(t *testing.T) {
	t.Parallel()
	src := `#fc 4
use console;
const T = "abcdefghijklmnop";
var gs:[]const u8;
var buf:[8]u8;
function sum(s:[]const u8):u16 @(noinline)
{
	var r:u16 = 0;
	for (var c in s) {
		r += c;
	}
	return r;
}
function adv(data:[]const u8, n:u8):u16 @(noinline)
{
	var s = data;
	var r:u16 = 0;
	while (@len(s) >= n) {
		r += s[0];
		s = s[n..];
	}
	return r + @len(s);
}
function shrink(s:[]const u8):u16 @(noinline)
{
	var r:u16 = 0;
	while (@len(s) != 0) {
		r += s[@len(s) - 1];
		s = s[..@len(s) - 1];
	}
	return r;
}
function chain(s:[]const u8, k:u8):u16 @(noinline)
{
	var t = s[1..];
	s = t[k..k + 2];
	t = s[1..];
	return sum(s) * 3 + sum(t);
}
function wide(w:[:u16]const u8):u16 @(noinline)
{
	var r:u16 = 0;
	while (@len(w) > 3) {
		w = w[3..];
		r += w[0];
	}
	return r;
}
function glob():u16 @(noinline)
{
	gs = T[2..];
	var r:u16 = 0;
	while (@len(gs) > 5) {
		gs = gs[2..];
		r += gs[0];
	}
	return r;
}
function write(n:u8):u16 @(noinline)
{
	var s:[]u8 = buf;
	var k:u8 = 0;
	while (@len(s) != 0) {
		s[0] = k;
		k += n;
		s = s[1..];
	}
	return sum(buf);
}
function main():void
{
	printf("{} {} {}\n", adv(T, 1), adv(T, 3), adv(T[1..], 5));
	printf("{} {}\n", shrink(T[..5]), shrink(T[..0]));
	printf("{} {}\n", chain(T, 0), chain(T, 4));
	printf("{} {}\n", wide(T), glob());
	printf("{}\n", write(3));
	console.exit(0);
}
`
	out, asm := buildShape(t, src)
	// 参照の値 (Go で計算)
	tab := "abcdefghijklmnop"
	sum := func(s string) int {
		r := 0
		for _, c := range []byte(s) {
			r += int(c)
		}
		return r
	}
	adv := func(s string, n int) int {
		r := 0
		for len(s) >= n {
			r += int(s[0])
			s = s[n:]
		}
		return r + len(s)
	}
	chain := func(s string, k int) int {
		t := s[1:]
		s = t[k : k+2]
		t = s[1:]
		return sum(s)*3 + sum(t)
	}
	wide := func(w string) int {
		r := 0
		for len(w) > 3 {
			w = w[3:]
			r += int(w[0])
		}
		return r
	}
	glob := func() int {
		g := tab[2:]
		r := 0
		for len(g) > 5 {
			g = g[2:]
			r += int(g[0])
		}
		return r
	}
	want := fmt.Sprintf("%d %d %d\n%d %d\n%d %d\n%d %d\n%d\n", adv(tab, 1), adv(tab, 3), adv(tab[1:], 5), sum(tab[:5]), 0,
		chain(tab, 0), chain(tab, 4), wide(tab), glob(), 0+3+6+9+12+15+18+21)
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
	// adv の `s = s[n..]` は s のポインタと長さを直に書き換える (組み立ての一時の値 3 バイトを写さない: 切ると 8 行以上長い)
	body := procBody(t, asm, "_t_adv")
	r := testBuild(t, buildSpec{Files: map[string]string{"t.fc": src}, CompileOnly: true, Config: ir.NewConfig("aggbuild")})
	if r.Err != nil {
		t.Fatal(r.Err)
	}
	if off := procBody(t, r.Built(t, "_t.s"), "_t_adv"); len(off)-len(body) < 8 {
		t.Errorf("adv が縮んでいない (%d 行、aggbuild を切ると %d 行):\n%s", len(body), len(off), strings.Join(body, "\n"))
	}
}

// TestShiftZeroExt: 1 バイトをゼロ拡張した値の 1〜7 の左シフト (`(n as u16) << k`) を上位と下位に分けて作る
// (codegen.shiftZeroExt)。すべての入力で値を確かめる。
func TestShiftZeroExt(t *testing.T) {
	t.Parallel()
	var src strings.Builder
	src.WriteString("#fc 4\nuse console;\nvar acc:u16;\n")
	for k := 1; k <= 7; k++ {
		fmt.Fprintf(&src, "function sh%d(n:u8):u16 @(noinline)\n{\n\treturn ((n as u16) << %d) ^ 0x5a5a;\n}\n", k, k)
	}
	src.WriteString("function main():void\n{\n")
	for k := 1; k <= 7; k++ {
		fmt.Fprintf(&src, "\tacc = 0;\n\tfor (var i:u16 = 0; i < 256; i += 1) {\n\t\tacc = (acc << 1 | acc >> 15) + sh%d(i as u8);\n\t}\n\tprintf(\"{}\\n\", acc);\n", k)
	}
	src.WriteString("\tconsole.exit(0);\n}\n")
	out, asm := buildShape(t, src.String())
	var want strings.Builder
	for k := 1; k <= 7; k++ {
		acc := uint16(0)
		for i := 0; i < 256; i++ {
			acc = (acc<<1 | acc>>15) + (uint16(i)<<k ^ 0x5a5a)
		}
		fmt.Fprintf(&want, "%d\n", acc)
	}
	if out != want.String() {
		t.Errorf("got %q, want %q", out, want.String())
	}
	if body := strings.Join(procBody(t, asm, "_t_sh5"), "\n"); strings.Contains(body, "rol") {
		t.Errorf("sh5 が 1 ビットずつ回している:\n%s", body)
	}
}

// TestResidentXIndexKeepsY: Y に常駐する添字のループで、X に常駐する添字でグローバルの配列に書く命令は Y を退避しない
// (regalloc.needsY が X の添字を知らず、毎周 sty していた)。
func TestResidentXIndexKeepsY(t *testing.T) {
	t.Parallel()
	out, asm := buildShape(t, `#fc 4
use console;
var out:[16]u8;
var k:u8;
function f(data:[]const u8, n:u8):void @(noinline)
{
	for (var i:u8 = 0; i < n; i += 1) {
		out[k] = data[i];
		k += 1;
	}
}
function main():void
{
	f("abcdef", 4);
	f("xyz", 2);
	printf("{} {} {} {}\n", k, out[0], out[3], out[5]);
	console.exit(0);
}
`)
	if out != "6 97 100 121\n" {
		t.Errorf("got %q", out)
	}
	body := procBody(t, asm, "_t_f")
	for _, l := range body {
		if strings.HasPrefix(l, "sty") {
			t.Errorf("ループの中で Y を退避している:\n%s", strings.Join(body, "\n"))
			break
		}
	}
}

// TestFoldAddChainLoop: 定数の加減算の連鎖を畳む (opt の simplify) とき、写しを辿ってループを回り自分の命令に着いても
// 畳まない (`while (n != 0) { tick(); n -= 1; }` の `n -= 1` を n + 0 にしていた)。
func TestFoldAddChainLoop(t *testing.T) {
	t.Parallel()
	out, _ := buildShape(t, `#fc 4
use console;
var cnt:u8;
var base:u8;
function tick():void @(noinline)
{
	cnt += 1;
}
function wait_n(n:u8):void @(noinline)
{
	while (n != 0) {
		tick();
		n -= 1;
	}
}
function room():u8
{
	return 128 - base;
}
function f(n:u8):u8 @(noinline)
{
	var r = room() - 3;
	var k = (n + 3) - 1;
	var j = 100 - n;
	j -= 5;
	base += 1;
	return r + room() - 1 + k * 2 + j;
}
function main():void
{
	wait_n(5);
	wait_n(0);
	wait_n(3);
	base = 10;
	printf("{} {}\n", cnt, f(7));
	console.exit(0);
}
`)
	// r = 128-10-3 = 115、room() は base+1 の後で 117、k = 9、j = 88: 115 + 117 - 1 + 18 + 88 = 337 → 81 (u8)
	if out != "8 81\n" {
		t.Errorf("got %q", out)
	}
}

// TestSSAByteAndPointerRules: `(a >> 8) as u8` は a の上位を直に読む、`(i + n) - i` は n、inline した関数に渡した `&g` を
// 通す読み書きは g の直の読み書き (opt の ssa)。-O 0 と同じ結果で、pad.update の形は (p),y を使わない。
func TestSSAByteAndPointerRules(t *testing.T) {
	t.Parallel()
	out, asm := buildShape(t, `#fc 4
use console;
struct Pad { held:u8; pressed:u8; released:u8; }
var p1:Pad;
var p2:Pad;
var q:[16]u8;
var hi:u8;
function update(p:*Pad, now:u8):void
{
	p.pressed = now & ~p.held;
	p.released = p.held & ~now;
	p.held = now;
}
function poll(a:u8, b:u8):void @(noinline)
{
	update(&p1, a);
	update(&p2, b);
}
function high(a:u16, b:u8):u8 @(noinline)
{
	var c = b as u16;
	hi = (c >> 8) as u8;
	return ((a >> 8) as u8) + hi;
}
function part(i:u8, n:u8):[]u8 @(noinline)
{
	return q[i..i + n];
}
function main():void
{
	poll(3, 5);
	poll(6, 1);
	var s = part(2, 5);
	printf("{} {} {} {} {} {} {} {}\n", p1.held, p1.pressed, p1.released, p2.held, p2.pressed, p2.released, high(0x1234, 200), @len(s));
	console.exit(0);
}
`)
	if out != "6 4 1 1 0 4 18 5\n" {
		t.Errorf("got %q", out)
	}
	if body := strings.Join(procBody(t, asm, "_t_poll"), "\n"); strings.Contains(body, "),y") {
		t.Errorf("poll がポインタで読み書きしている:\n%s", body)
	}
}
