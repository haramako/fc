package driver

// doc/v3_plan.md §10（一般的な用途で不便な仕様の調査、2026-09-26）で「修正する」にした項目のテスト。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestBlockScope: 素のブロック `{ }` はスコープを作る（中の宣言は外から見えず、外と同じ名前を宣言できる）。
func TestBlockScope(t *testing.T) {
	t.Parallel()
	out, err := buildBothLevels(t, map[string]string{"t.fc": `#fc 3
use * from stdio;
function main():void
{
	var x:u8 = 10;
	{ var x:u8 = 20; printf(x, " "); }
	{ var t:u8 = 1; printf(t, " "); }
	{ var t:u8 = 2; printf(t, " "); }
	printf(x, "\n");
	exit(0);
}
`})
	if err != nil || out != "20 1 2 10\n" {
		t.Errorf("got %q, %v", out, err)
	}
	_, err = buildFiles(t, map[string]string{"t.fc": "#fc 3\nfunction main():void { { var q:u8 = 2; } q = 3; }\n"})
	if err == nil || !strings.Contains(err.Error(), "q not found") {
		t.Errorf("ブロックの外から中の変数: %v", err)
	}
}

// TestMemZeroLength: mem.set / zero / compare の長さは u16 で、0 は何もしない・等しい (長さ u8 の `iny; cpy; bne` のループで
// 0 が 256 バイトになっていた。256 を渡すと 0 に切り詰められ、それに頼っていた)。256 バイトを超える長さも。
func TestMemZeroLength(t *testing.T) {
	t.Parallel()
	out, err := buildBothLevels(t, map[string]string{"t.fc": `#fc 3
use * from stdio;
use mem;
var buf:[8]u8;
var n:u8;
var big:[301]u8;
var big2:[301]u8;
function main():void
{
	for (var i:u8 = 0; i < 8; i += 1) { buf[i] = i + 1; }
	n = 0;
	mem.zero(&buf[2], n);
	printf(buf[1], buf[2], buf[7], " ");
	n = 3;
	mem.zero(&buf[2], n);
	printf(buf[1], buf[2], buf[4], buf[5], " ");
	n = 0;
	printf(mem.compare("abc", "xyz", n), mem.compare("abc", "abd", 2), mem.compare("abc", "abd", 3), " ");
	mem.set(&big[0], 7, 300);
	printf(big[0], big[255], big[299], big[300], " ");
	mem.copy(&big2[0], &big[0], 300);
	printf(mem.compare(&big[0], &big2[0], 300), " ");
	big2[299] = 1;
	printf(mem.compare(&big[0], &big2[0], 300), mem.compare(&big[0], &big2[0], 299), " ");
	mem.zero(&big[1], 298);
	printf(big[0], big[1], big[298], big[299], "\n");
	exit(0);
}
`})
	if err != nil || out != "238 2006 001 7770 0 10 7007\n" {
		t.Errorf("got %q, %v", out, err)
	}
}

// TestInlineLocalConst: インライン展開する関数の中の const の表・文字列 (その関数の .proc の中のラベル) を、展開先へ別の名前で
// 写す (ca65 の `Symbol 'T' is undefined` だった)。表が別の表を指す形、呼び出し側の同じ名前の表とも衝突しない。
func TestInlineLocalConst(t *testing.T) {
	t.Parallel()
	out, err := buildBothLevels(t, map[string]string{"t.fc": `#fc 3
use * from stdio;
function f(i:u8):u8 { const T = [5, 6, 7]; var p:*const u8 = "ab"; return T[i] + p[0]; }
function g(i:u8):u8 { const T = [50, 60, 70]; const PS:[?]*const u8 = ["xy", "zw"]; return T[i] + PS[i][1]; }
function main():void
{
	const T = [1, 2, 3];
	printf(f(1), " ", f(2), " ", g(0), " ", g(1), " ", T[2], "\n");
	exit(0);
}
`})
	if err != nil || out != "103 104 171 179 3\n" {
		t.Errorf("got %q, %v", out, err)
	}
}

// TestLocalInferredArray: ローカルの `var a:[?]T = [...]` は初期値から長さを決める (長さ未定のまま領域が取られず、ほかの
// ローカルを壊していた。-O 0 では ca65 の Range error)。
func TestLocalInferredArray(t *testing.T) {
	t.Parallel()
	out, err := buildBothLevels(t, map[string]string{"t.fc": `#fc 3
use * from stdio;
struct P { x:u8; y:u8; }
function main():void
{
	var a:[?]u8 = [1, 2, 3];
	var z:u8 = 50;
	var ps:[?]P = [{1, 2}, {3, 4}];
	a[0] = 9;
	ps[1].y = 7;
	printf(a[0], a[1], a[2], " ", z, " ", @len(a), " ", @sizeof(ps), " ", ps[0].x, ps[1].y, "\n");
	exit(0);
}
`})
	if err != nil || out != "923 50 3 4 17\n" {
		t.Errorf("got %q, %v", out, err)
	}
}

// TestConstArrayDeclaredLength: `const X:[4]u8 = [1, 2];` は宣言の長さまで 0 で埋める (ローカルの var・C と同じ。長さが
// 無視されて X[2] が隣のデータを読んでいた)。struct・文字列・u16 は 0、ポインタは null。要素が多すぎればエラー。
func TestConstArrayDeclaredLength(t *testing.T) {
	t.Parallel()
	out, err := buildBothLevels(t, map[string]string{"t.fc": `#fc 3
use * from stdio;
struct P { x:u8; y:u8; }
const X:[4]u8 = [1, 2];
const Y:[2]u8 = [7, 8];
const PS:[3]P = [{1, 2}];
const S:[6]u8 = "abc";
const Q:[3]*const u8 = ["hi"];
const W:[3]u16 = [1000];
function main():void
{
	printf(@len(X), X[2], X[3], Y[0], " ", PS[0].y, PS[2].y, " ", S[4], @len(S), " ", Q[0][1], Q[1] == null, " ", W[0], " ", W[2], "\n");
	exit(0);
}
`})
	if err != nil || out != "4007 20 06 1051 1000 0\n" {
		t.Errorf("got %q, %v", out, err)
	}
	_, err = buildFiles(t, map[string]string{"t.fc": "#fc 3\nconst Z:[2]u8 = [1, 2, 3];\nfunction main():void { }\n"})
	if err == nil || !strings.Contains(err.Error(), "3 elements given for [2]u8") {
		t.Errorf("多すぎる要素: %v", err)
	}
}

// TestRuntimeArrayLiteral: 実行時の値 (変数・式) を要素に持つ配列リテラルは実行時に組み立てる。変数の名前がそのアドレスの
// 定数になり (`var a:[2]u8 = [n, m]` が n と m のアドレスの表で 34 が 28)、式なら constant value required、代入・引数・
// struct のフィールドの中では panic だった。関数の中・戻り値・グローバルへの代入・struct のフィールド・slice の引数・
// 足りない要素の 0 埋め・要素の型の推論 (u16)・再帰関数の中。
func TestRuntimeArrayLiteral(t *testing.T) {
	t.Parallel()
	out, err := buildBothLevels(t, map[string]string{"t.fc": `#fc 3
use * from stdio;
struct Inv { items:[2]u8; n:u8; }
var gx:u8;
var gw:u16;
var g:Inv;
var pts:[2]u8;
function g2(n:u8, m:u8):u8 @(noinline) { var a:[2]u8 = [n, m]; return a[0] * 10 + a[1]; }
function pair(n:u8, m:u8):[2]u8 @(noinline) { return [n, m]; }
function sum(s:[]const u8):u8 { var t:u8 = 0; for (var i:u8 = 0; i < @len(s); i += 1) { t += s[i]; } return t; }
function rec(d:u8):u8 { if (d == 0) { return 0; } var a:[2]u8 = [d, d + 1]; return a[1] + rec(d - 1); }
function main():void
{
	gx = 7; gw = 1000;
	var b:[3]u8 = [gx, gx + 1, gx * 2];
	var w:[2]u16 = [gw, 5];
	var p = pair(4, 9);
	pts = [gx, 3];
	g = {items: [gx, 2], n: 1};
	var inv:Inv = {items: [gx, 6], n: gx};
	var pad:[4]u8 = [gx];
	var q = [gx, 300];
	printf(g2(3, 4), " ", g2(5, 6), " ", b[0], b[1], b[2], " ", w[0], " ", p[0], p[1], " ", pts[0], pts[1], " ", g.items[0], g.items[1], " ", inv.items[1], inv.n, " ", sum([gx, 1, 2]), " ", pad[0], pad[3], " ", q[1], " ", rec(3), "\n");
	exit(0);
}
`})
	if err != nil || out != "34 56 7814 1000 49 73 72 67 10 70 300 9\n" {
		t.Errorf("got %q, %v", out, err)
	}
}

// TestPointerArithmeticScaled: ポインタの加減算は要素 n 個分 (C と同じ)。バイト単位で、u16 の配列を p++ でたどると 1 バイト
// ずれ、p[1] と *(p + 1) が違っていた。変数・符号付きの n、struct の配列、ポインタの差 (要素数。要素 3 バイトは割り算)。
func TestPointerArithmeticScaled(t *testing.T) {
	t.Parallel()
	out, err := buildBothLevels(t, map[string]string{"t.fc": `#fc 3
use * from stdio;
struct O { hp:u8; x:u16; }
var w:[4]u16;
var objs:[4]O;
var b:[4]u8;
function main():void
{
	w[0] = 1000; w[1] = 2000; w[2] = 3000; w[3] = 4000;
	for (var i:u8 = 0; i < 4; i += 1) { objs[i].hp = i * 10; b[i] = i + 1; }
	var p = &w[0];
	p += 1;
	var q = &w[0] + 2;
	var r = &w[3];
	r--;
	var k:u8 = 3;
	var s = &w[0] + k;
	var m:i8 = -1;
	var t = &w[3] + m;
	var o = &objs[0];
	o++;
	o += 1;
	var bp = &b[0];
	bp += 2;
	printf(*p, " ", *q, " ", *r, " ", *s, " ", *t, " ", o.hp, " ", *bp, " ", s - p, " ", &objs[3] - &objs[0], " ", (p + 1)[0] == p[1], "\n");
	exit(0);
}
`})
	if err != nil || out != "2000 3000 3000 4000 3000 20 3 2 3 1\n" {
		t.Errorf("got %q, %v", out, err)
	}
}

// TestShift16Variable: 2 バイトの値を変数の回数でシフトする (-O 0 では「not supported」、-O 2 では回数が定数伝播されたときだけ
// 通っていた)。左・右・算術右、回数 0 / 15、回数が代入先と同じ変数 (`x = 0x0100 >> x`)。
func TestShift16Variable(t *testing.T) {
	t.Parallel()
	out, err := buildBothLevels(t, map[string]string{"t.fc": `#fc 3
use * from stdio;
function shl(w:u16, n:u8):u16 @(noinline) { return w << n; }
function shr(w:u16, n:u8):u16 @(noinline) { return w >> n; }
function sar(w:i16, n:u8):i16 @(noinline) { return w >> n; }
function mask(n:u8):u16 @(noinline) { return (1 as u16) << n; }
function main():void
{
	var w:u16 = 0x1234;
	var n:u8 = 4;
	w <<= n;
	var x:u16 = 3;
	x = 0x0100 >> x;
	printf(shl(0x1234, 4), " ", shl(1, 15), " ", shl(0x1234, 0), " ", shr(0x8000, 15), " ", shr(0x1234, 8), " ", sar(-256, 4), " ", mask(10), " ", w, " ", x, "\n");
	exit(0);
}
`})
	if err != nil || out != "9024 32768 4660 1 18 -16 1024 9024 32\n" {
		t.Errorf("got %q, %v", out, err)
	}
}

// TestPrintfTypes: printf は slice を長さの分だけ、符号付きの整数を符号付きで、enum を値で出す (slice と struct は黙って捨て、
// 符号付きは符号なしで 65531 と出し、enum は print_int16 の引数のエラーだった)。struct はエラー。
func TestPrintfTypes(t *testing.T) {
	t.Parallel()
	out, err := buildBothLevels(t, map[string]string{"t.fc": `#fc 3
use * from stdio;
enum Color { Red, Green, Blue }
enum Delta:i8 { Back = -1, Stay = 0 }
function main():void
{
	var s:[]const u8 = "bob";
	var n8:i8 = -5;
	var n16:i16 = -300;
	var big:i16 = -32768;
	var c = Color.Blue;
	var d = Delta.Back;
	var t:u16 = 65535;
	printf("[", s, "] ", n8, " ", n16, " ", big, " ", c, " ", d, " ", t, " ", true, " ", s[1..], "\n");
	exit(0);
}
`})
	if err != nil || out != "[bob] -5 -300 -32768 2 -1 65535 1 ob\n" {
		t.Errorf("got %q, %v", out, err)
	}
	_, err = buildFiles(t, map[string]string{"t.fc": "#fc 3\nuse * from stdio;\nstruct P { x:u8; }\nfunction main():void { var p:P; printf(p); }\n"})
	if err == nil || !strings.Contains(err.Error(), "printf cannot print a value of type") {
		t.Errorf("struct: %v", err)
	}
}

// TestAddressVarOverlap: @(address:) の変数がリンクした RAM のセグメント (fc の ZP・BSS など) と重なればエラー (fc の ZP の
// 中に置くと reg などと黙って重なっていた)。I/O・カートリッジの RAM の番地はよい。
func TestAddressVarOverlap(t *testing.T) {
	t.Parallel()
	_, err := buildFiles(t, map[string]string{"t.fc": "#fc 3\nuse * from stdio;\nvar zp:u8 @(address: 0x10);\nfunction main():void { zp = 1; exit(0); }\n"})
	if err == nil || !strings.Contains(err.Error(), "`t.zp` @(address: $0010) overlaps segment") {
		t.Errorf("ZP: %v", err)
	}
	out, err := buildFiles(t, map[string]string{"t.fc": "#fc 3\nuse * from stdio;\nvar sram:u8 @(address: 0x6000);\nfunction main():void { sram = 1; printf(sram, \"\n\"); exit(0); }\n"})
	if err != nil || out != "1\n" {
		t.Errorf("$6000: %q, %v", out, err)
	}
}

// TestInterruptSymbolFrames: @(symbol: "_interrupt") の関数は @(interrupt) が無くても割り込み (フレームを main の呼び出しと
// 重ねない)。割り込みと main の両方から呼ぶ関数は、静的フレームを共有するので警告。
func TestInterruptSymbolFrames(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	src := `#fc 3
use nes;
var frame:u8;
var work:[4]u8;
function helper(a:u8, b:u8):u8 @(noinline) { var t = a + b; return t * 2; }
function sub(x:u8):u8 @(noinline) { var y = x + 1; var z = y * 3; return z; }
function nmi():void @(symbol: "_interrupt") { var k = sub(frame); frame = k; }
function irq():void @(symbol: "_interrupt_irq") { }
function main():void { while (true) { work[0] = helper(work[1], work[2]); work[3] = sub(work[0]); } }
`
	if err := os.WriteFile(filepath.Join(dir, "t.fc"), []byte(src), 0o666); err != nil {
		t.Fatal(err)
	}
	res, err := NewCompiler(absRepoRoot).BuildContext(t.Context(), "t.fc", &BuildOptions{Target: "nes", Dir: dir, BuildDir: filepath.Join(dir, "b"), Out: filepath.Join(dir, "t.nes")})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, w := range res.Warnings {
		found = found || strings.Contains(w.Msg, "_t_sub is called both from the interrupt handler _interrupt and from _main")
	}
	if !found {
		t.Errorf("共有するフレームの警告が無い: %v", res.Warnings)
	}
	inc, err := os.ReadFile(filepath.Join(dir, "b", "_frames.inc"))
	if err != nil {
		t.Fatal(err)
	}
	// 割り込みから届く sub と main から呼ぶ helper のフレームが重ならない (前は両方 FC_SZP+0)
	if strings.Contains(string(inc), "F_t_sub = FC_SZP+0") && strings.Contains(string(inc), "F_t_helper = FC_SZP+0") {
		t.Errorf("フレームが重なっている:\n%s", inc)
	}
}
