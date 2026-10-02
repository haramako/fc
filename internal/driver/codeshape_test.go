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
	@printf("{}\n", cnt);
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
	@printf("{} {} {}\n", adv(T, 1), adv(T, 3), adv(T[1..], 5));
	@printf("{} {}\n", shrink(T[..5]), shrink(T[..0]));
	@printf("{} {}\n", chain(T, 0), chain(T, 4));
	@printf("{} {}\n", wide(T), glob());
	@printf("{}\n", write(3));
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

// TestResultArg: 関数の戻り値の slice をそのまま引数にする形 (`sum(get())`) は、戻り値を一時変数に受けずに呼び先のフレームから
// 引数の場所へ写す (codegen.markResultArg)。兄弟の関数のフレームは重なるので全部読んでから書く。stack の関数 (再帰) の中、
// ループの中 (常駐)、4 バイトの [:u16]、最後の引数がレジスタ渡しの関数、前に引数がある形 (写さない) も -O 0 と同じ結果。
// あわせて、グローバルの slice への `@format` の結果は一時変数を通さずに直に書く (opt.assembleInPlace)。
func TestResultArg(t *testing.T) {
	t.Parallel()
	src := `#fc 4
use console;
const T = "abcdefghijklmnop";
var k:u8;
var buf:[8]u8;
var gw:[]u8;
function get(n:u8):[]const u8 @(noinline)
{
	return T[n..];
}
function wget(n:u8):[:u16]const u8 @(noinline)
{
	return T[n..];
}
function sum(s:[]const u8):u16 @(noinline)
{
	var r:u16 = 0;
	for (var c in s) {
		r += c;
	}
	return r;
}
function wsum(s:[:u16]const u8):u16 @(noinline)
{
	var r:u16 = 0;
	for (var c in s) {
		r += c;
	}
	return r;
}
function sum2(s:[]const u8, m:u8):u16 @(noinline)
{
	return sum(s) * m;
}
function msum(m:u8, s:[]const u8):u16 @(noinline)
{
	return sum(s) * m;
}
function rec(n:u8):u16
{
	if (n == 0) {
		return 0;
	}
	return sum(get(n)) + rec(n - 1);
}
function lp():u16 @(noinline)
{
	var r:u16 = 0;
	for (var i:u8 = 0; i < 5; i++) {
		r += sum(get(i)) + i;
	}
	return r;
}
function fmtg(v:u8):u8 @(noinline)
{
	gw = @format(buf, "v{}", v);
	return @len(gw);
}
function main():void
{
	k = 3;
	@printf("{} {} {}\n", sum(get(k)), wsum(wget(2)), sum2(get(1), 2));
	@printf("{} {} {}\n", msum(3, get(4)), rec(4), lp());
	@printf("{} ", fmtg(k * 40));
	console.write(gw);
	console.newline();
	console.exit(0);
}
`
	out, asm := buildShape(t, src)
	tab := "abcdefghijklmnop"
	sum := func(s string) int {
		r := 0
		for _, c := range []byte(s) {
			r += int(c)
		}
		return r
	}
	rec, loop := 0, 0
	for n := 1; n <= 4; n++ {
		rec += sum(tab[n:])
	}
	for i := 0; i < 5; i++ {
		loop += sum(tab[i:]) + i
	}
	want := fmt.Sprintf("%d %d %d\n%d %d %d\n4 v120\n", sum(tab[3:]), sum(tab[2:]), sum(tab[1:])*2, sum(tab[4:])*3, rec&0xffff, loop)
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
	// main の sum(get(k)) は get のフレームから sum のフレームへ直に写す (main のフレームに受けない)
	body := strings.Join(procBody(t, asm, "_main"), "\n")
	if !strings.Contains(body, "jsr _t_get\nldy <F_t_get+1\nldx <F_t_get+2\nlda <F_t_get+0\nsta <F_t_sum+2\nsty <F_t_sum+3\nstx <F_t_sum+4\njsr _t_sum") {
		t.Errorf("sum(get(k)) を直に写していない:\n%s", body)
	}
	// fmtg の gw は組み立ての一時の値を通さない (gw のバイトに直に書く: 写しの 3 組の lda / sta が無い)
	r := testBuild(t, buildSpec{Files: map[string]string{"t.fc": src}, CompileOnly: true, Config: ir.NewConfig("aggbuild")})
	if r.Err != nil {
		t.Fatal(r.Err)
	}
	if on, off := procBody(t, asm, "_t_fmtg"), procBody(t, r.Built(t, "_t.s"), "_t_fmtg"); len(off)-len(on) < 6 {
		t.Errorf("fmtg が縮んでいない (%d 行、aggbuild を切ると %d 行):\n%s", len(on), len(off), strings.Join(on, "\n"))
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
		fmt.Fprintf(&src, "\tacc = 0;\n\tfor (var i:u16 = 0; i < 256; i += 1) {\n\t\tacc = (acc << 1 | acc >> 15) + sh%d(i as u8);\n\t}\n\t@printf(\"{}\\n\", acc);\n", k)
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
	@printf("{} {} {} {}\n", k, out[0], out[3], out[5]);
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
	@printf("{} {}\n", cnt, f(7));
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
	@printf("{} {} {} {} {} {} {} {}\n", p1.held, p1.pressed, p1.released, p2.held, p2.pressed, p2.released, high(0x1234, 200), @len(s));
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

// TestWideForEachPointer: 要素 1 バイトで添字が 16 ビットの for-each ([:u16] の slice、256 要素を超える配列) は要素を指す
// ポインタを進めるループ (sema の forInElems)。空・256 を超える長さ・continue / break・const の表で、値は Go で計算したもの。
func TestWideForEachPointer(t *testing.T) {
	t.Parallel()
	out, asm := buildShape(t, `#fc 4
use console;
var big:[300]u8;
function sum(s:[:u16]const u8):u16 @(noinline)
{
	var r:u16 = 0;
	for (var c in s) {
		if (c == 7) {
			continue;
		}
		if (c == 250) {
			break;
		}
		r += c;
	}
	return r;
}
function fill():u16 @(noinline)
{
	var k:u8 = 0;
	for (var i:u16 = 0; i < 300; i += 1) {
		big[i] = k;
		k += 3;
	}
	var r:u16 = 0;
	for (var c in big) {
		r += c;
	}
	return r;
}
function main():void
{
	var f = fill();
	@printf("{} {} {} {}\n", f, sum(big), sum(big[..0]), sum(big[10..20]));
	console.exit(0);
}
`)
	big := make([]byte, 300)
	k := byte(0)
	for i := range big {
		big[i] = k
		k += 3
	}
	sum := func(s []byte) int {
		r := 0
		for _, c := range s {
			if c == 7 {
				continue
			}
			if c == 250 {
				break
			}
			r += int(c)
		}
		return r & 0xffff
	}
	all := 0
	for _, c := range big {
		all += int(c)
	}
	want := fmt.Sprintf("%d %d %d %d\n", all, sum(big), 0, sum(big[10:20]))
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
	// 添字の 16 ビットの足し算 (adc <reg+1) が無い
	if body := strings.Join(procBody(t, asm, "_t_sum"), "\n"); strings.Contains(body, "reg+1") {
		t.Errorf("sum が 16 ビットの添字で読んでいる:\n%s", body)
	}
}

// TestMul8x8To16: 0〜255 どうしの 16 ビットの掛け算 (`(a as u16) * b`、定数の 3 も) は __mul_8t16 (表引き) を呼び、全部の組で
// 正しい (codegen.byteValue、share/runtime.asm)。
func TestMul8x8To16(t *testing.T) {
	t.Parallel()
	out, asm := buildShape(t, `#fc 4
use console;
function mul(a:u8, b:u8):u16 @(noinline) { return (a as u16) * b; }
function mul3(a:u8):u16 @(noinline) { return (a as u16) * 201; }
function main():void
{
	var bad:u16 = 0;
	var e3:u16 = 0;
	for (var a:u16 = 0; a < 256; a += 1) {
		if (mul3(a as u8) != e3) {
			bad += 1;
		}
		e3 += 201;
		var e:u16 = 0; // a * b を足し算で (-O 0 の __mul_16 は 1 回 400 サイクルほどで遅い)
		for (var b:u16 = 0; b < 256; b += 1) {
			if (mul(a as u8, b as u8) != e) {
				bad += 1;
			}
			e += a;
		}
	}
	@printf("{} {}\n", bad, mul(255, 255));
	console.exit(0);
}
`)
	if out != "0 65025\n" {
		t.Errorf("got %q", out)
	}
	for _, f := range []string{"_t_mul", "_t_mul3"} {
		if body := strings.Join(procBody(t, asm, f), "\n"); !strings.Contains(body, "jsr __mul_8t16") {
			t.Errorf("%s が __mul_8t16 を呼ばない:\n%s", f, body)
		}
	}
}

// TestAverageBytes: 1 バイトどうしの平均 (`((a as u16 + b) / 2) as u8`・`>> 1`) は 8 ビットの足し算と ror a (opt.averageBytes)。
// 全部の組で -O 0 と同じ値。
func TestAverageBytes(t *testing.T) {
	t.Parallel()
	out, asm := buildShape(t, `#fc 4
use console;
function avg(a:u8, b:u8):u8 @(noinline) { return ((a as u16 + b) / 2) as u8; }
function avg2(a:u8, b:u8):u8 @(noinline) { return ((a as u16 + b) >> 1) as u8; }
function main():void
{
	var bad:u16 = 0;
	for (var a:u16 = 0; a < 256; a += 1) {
		for (var b:u16 = 0; b < 256; b += 1) {
			var e = ((a + b) >> 1) as u8;
			if (avg(a as u8, b as u8) != e || avg2(a as u8, b as u8) != e) {
				bad += 1;
			}
		}
	}
	@printf("{} {} {}\n", bad, avg(255, 255), avg2(200, 101));
	console.exit(0);
}
`)
	if out != "0 255 150\n" {
		t.Errorf("got %q", out)
	}
	for _, f := range []string{"_t_avg", "_t_avg2"} {
		if body := strings.Join(procBody(t, asm, f), "\n"); !strings.Contains(body, "ror a") || strings.Contains(body, "lsr") {
			t.Errorf("%s が adc + ror a でない:\n%s", f, body)
		}
	}
}

// TestLiteralLoadsResident: 常駐の割付の後のリテラルの伝播 (regalloc.PropagateLiteralLoads) は、常駐の一時変数への書き込みを
// 消さない (関数の出口の書き戻し `sty g` は codegen が出すので IR の上では読まれていない。castle の anchor_throw の形)。
// ループの変数の初期値の書き込み (`lda #0; sta i`) は消える。
func TestLiteralLoadsResident(t *testing.T) {
	t.Parallel()
	out, asm := buildShape(t, `#fc 4
use console;
var g:i8;
var d:u8;
var h:u8;
var buf:[8]u8;
var k:u8;
function throw():void @(noinline)
{
	h = 3;
	if (d == 0) {
		g = 8;
	} else {
		g = -8;
	}
	h += 1;
}
function copy(n:u8):void @(noinline)
{
	for (var i:u8 = 0; i < n; i += 1) {
		buf[k] = i;
		k += 1;
	}
}
function main():void
{
	throw();
	var a = g;
	d = 1;
	throw();
	copy(5);
	@printf("{} {} {} {} {}\n", a, g, h, k, buf[4]);
	console.exit(0);
}
`)
	if out != "8 -8 4 5 4\n" {
		t.Errorf("got %q", out)
	}
	if body := strings.Join(procBody(t, asm, "_t_copy"), "\n"); strings.Contains(body, "lda #0\n\tsta 0+<F_t_copy") || strings.Contains(body, "lda #0\nsta 0+<F_t_copy") {
		t.Errorf("ループの変数の初期値を書いている:\n%s", body)
	}
}
