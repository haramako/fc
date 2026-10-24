package driver

// castle の doc/memo.md「FC BUG」由来の回帰テスト (Agent/discussions/2026-08-29-go-evolution-plan.md R4 の表)。
// 小さなプログラムを emu で実行して出力を見る。

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runEmu は fc 2 のソース (use * from stdio 済み) を emu でビルド・実行し、標準出力を返す。
func runEmu(t *testing.T, body string) string {
	t.Helper()
	return runEmuLevel(t, body, 0)
}

// runEmuLevel は最適化レベルを指定する runEmu (0 は既定の 2、-1 は -O 0)。
func runEmuLevel(t *testing.T, body string, level int) string {
	t.Helper()
	r := testBuild(t, buildSpec{Files: map[string]string{"t.fc": "#fc 2\nuse * from stdio;\n" + body}, Level: level, Run: true})
	if r.Err != nil {
		t.Fatalf("ビルド失敗: %v", r.Err)
	}
	if r.Run.ExitCode != 0 {
		t.Fatalf("終了コード %d:\n%s", r.Run.ExitCode, r.Stdout)
	}
	return r.Stdout
}

func TestBugGlobalPointerIndex(t *testing.T) {
	t.Parallel()
	// グローバルのポインタ変数への添字代入 (index+pset の融合がポインタを配列扱いしていた)
	out := runEmu(t, `var buf:*int;
var arr:[8]int;
function main():void
{
	buf = arr;
	buf[2] = 5;
	var v = buf[2];
	var w = arr[2];
	printf("v=", v, " w=", w, "\n");
	exit(0);
}
`)
	if out != "v=5 w=5\n" {
		t.Errorf("got %q", out)
	}
}

func TestBugUnusedExpressionStatement(t *testing.T) {
	t.Parallel()
	// 結果を使わない演算の式文 (一時変数に場所が割り付かずコード生成で落ちていた)
	out := runEmu(t, `function main():void
{
	var c = 32;
	c == 32;
	c + 1;
	-c;
	printf("ok\n");
	exit(0);
}
`)
	if out != "ok\n" {
		t.Errorf("got %q", out)
	}
}

func TestBugVoidValue(t *testing.T) {
	t.Parallel()
	// void 関数の呼び出しを値として使うとエラー (クラッシュしない)
	dir := t.TempDir()
	src := "#fc 2\nfunction f():void {}\nfunction main():void\n{\n\tif (f()) {}\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "t.fc"), []byte(src), 0o666); err != nil {
		t.Fatal(err)
	}
	_, err := buildCode(t, NewCompiler(absRepoRoot), "t.fc", &BuildOptions{Dir: dir, BuildDir: filepath.Join(dir, "b"), Out: filepath.Join(dir, "a.bin"), CompileOnly: true}, false, nil, 0)
	if err == nil || !strings.Contains(err.Error(), "expression has no value") {
		t.Errorf("got %v", err)
	}
}

func TestBugLocalArray(t *testing.T) {
	t.Parallel()
	// ローカル配列 (レジスタ領域に 2 バイトで置かれて隣の一時変数と重なっていた → フレームに置く)。
	// 定数添字・変数添字・2 バイト添字・ポインタ経由・関数への受け渡し
	out := runEmu(t, `use mem;
function fill(p:*int, n:int):void
{
	var i:int;
	for (i = 0; i < n; i++) {
		p[i] = i + 40;
	}
}
function sum(p:*int, n:int):int
{
	var s = 0;
	var i:int;
	for (i = 0; i < n; i++) {
		s += p[i];
	}
	return s;
}
function main():void
{
	var la:[4]int;
	la[0] = 7;
	la[1] = 8;
	la[2] = 9;
	la[3] = 10;
	var l0 = la[0];
	var l3 = la[3];
	printf("const ", l0, " ", l3, "\n");
	var i:int;
	for (i = 0; i < 4; i++) {
		la[i] = i + 20;
	}
	var l2 = la[2];
	l3 = la[3];
	printf("var ", l2, " ", l3, "\n");
	var i16:int16 = 1;
	la[i16] = 99;
	var l1 = la[1];
	printf("idx16 ", l1, "\n");
	var p:*int = la;
	p[0] = 1;
	l0 = la[0];
	printf("ptr ", l0, "\n");
	mem.fill(la, 3);
	var s1 = sum(la, 4);
	fill(la, 4);
	var s2 = sum(la, 4);
	printf("call ", s1, " ", s2, "\n");
	var big:[20]int;
	big[19] = 77;
	var b19 = big[19];
	printf("big ", b19, "\n");
	exit(0);
}
`)
	want := "const 7 10\nvar 22 23\nidx16 99\nptr 1\ncall 12 166\nbig 77\n"
	if out != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}
}

// TestCheckWarnings: fcc check / ビルド結果の警告 (構文検査)。
func TestCheckWarnings(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	src := "use * from stdio;\nfunction main():void\n{\n\tvar a = 1;\n\tif (a & 2 == 0) {}\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "t.fc"), []byte(src), 0o666); err != nil {
		t.Fatal(err)
	}
	c := NewCompiler(absRepoRoot)
	ws, err := c.Check("t.fc", &CheckOptions{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(ws) != 1 || !strings.Contains(ws[0].Msg, "binds looser") || ws[0].Pos.Line != 5 {
		t.Errorf("warnings: %+v", ws)
	}
	// Build の結果にも同じ警告が付く
	res, err := c.BuildContext(context.Background(), "t.fc", &BuildOptions{Dir: dir, BuildDir: filepath.Join(dir, "b"), Out: filepath.Join(dir, "a.bin"), CompileOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Warnings) != 1 {
		t.Errorf("build warnings: %+v", res.Warnings)
	}
}

// TestBugConstWidenA1 は、型付きの定数の式を広い型に初期化するときの A1 の広げ (fc 4)。
// 実行時の式は代入先まで含めて広い幅で計算するが、定数は畳み込んだ時点で幅が決まるので、初期化・引数・配列や struct の要素で
// 広い型と出会ったときに畳み込み直す必要がある。const 宣言・グローバルの変数の初期値・配列と struct リテラルの要素では
// 広げ直しておらず、var と const で値が変わっていた (bug hunt 2026-10-18。TestRandomArithV4 の種 1 が見つけた)。
func TestBugConstWidenA1(t *testing.T) {
	t.Parallel()
	src := `#fc 4
use console;
struct P { x:u16; }
const E = (128 as u8) + (200 as u8);
const C1:u16 = (128 as u8) + (200 as u8);
const C2:u16 = (50 as u8) - (128 as u8);
const CARR:[2]u16 = [(128 as u8) + (200 as u8), 5];
var GV:u16 = (128 as u8) + (200 as u8);
var ARR:[2]u16 = [(128 as u8) + (200 as u8), 5];
var SP:P = P{x: (128 as u8) + (200 as u8)};
function f(v:u16):u16 { return v; }
function h():u16 { return (128 as u8) + (200 as u8); }
function main():void
{
	var v:u16 = (128 as u8) + (200 as u8);
	var lp:P = P{x: (128 as u8) + (200 as u8)};
	var larr:[2]u16 = [(128 as u8) + (200 as u8), 5];
	// as は区切り (内側は u8 のまま)
	var sep:u16 = ((128 as u8) + (200 as u8)) as u8;
	// 型のない定数だけの式は任意精度で、代入先の型に入ることを確かめる
	var exact:u16 = 200 + 100;
	@printf("{}\n{}\n{}\n{}\n{} {}\n{}\n{}\n{}\n{} {}\n{}\n{}\n{}\n{}\n",
		E, C1, C2, GV, ARR[0], ARR[1], SP.x, v, lp.x, larr[0], larr[1], sep, exact, f((128 as u8) + (200 as u8)), h());
	console.exit(0);
}
`
	want := "72\n328\n65458\n328\n328 5\n328\n328\n328\n328 5\n72\n300\n328\n328\n"
	for _, level := range []int{0, -1} {
		r := testBuild(t, buildSpec{Files: map[string]string{"t.fc": src}, Level: level, Run: true})
		if r.Err != nil {
			t.Fatalf("level %d: ビルド失敗: %v", level, r.Err)
		}
		if r.Stdout != want {
			t.Errorf("level %d: 出力\n%s\nwant\n%s", level, r.Stdout, want)
		}
	}
}

// TestBugConstMinMax は組み込みの @min / @max / @clamp を定数式で畳む (fc 3 と fc 4)。
// 純粋マクロ (展開が IR を出さない) は constEval が展開しなかったので、`const D = @min(3, 5);` が
// 「const D must be constant」、配列の大きさ `[@min(4, 8)]u8` が「array size must be constant」になっていた
// (bug hunt 2026-10-18。TestRandomArithV4 の @min / @max で発覚)。
func TestBugConstMinMax(t *testing.T) {
	t.Parallel()
	src := `#fc 4
use console;
const D = @min(3, 5);
const E:u16 = @max((200 as u8), (100 as u8));
const F = @clamp((300 as u8), (20 as u8), (170 as u8));
var arr:[@min(4, 8)]u8;
function main():void
{
	var v:u8 = 200;
	arr[0] = D + E + F;
	@printf("{} {} {} {}\n", D, E, F, @sizeof(arr));
	console.exit(0);
}
`
	for _, level := range []int{0, -1} {
		r := testBuild(t, buildSpec{Files: map[string]string{"t.fc": src}, Level: level, Run: true})
		if r.Err != nil {
			t.Fatalf("level %d: ビルド失敗: %v", level, r.Err)
		}
		if want := "3 200 44 4\n"; r.Stdout != want {
			t.Errorf("level %d: 出力 %q、want %q", level, r.Stdout, want)
		}
	}
}

// TestBugMixedSignNarrowExpr は、符号の混ざり (F6) の印を、A1 で広げた後の IR の型でなく式の型で付けること。
// 16 ビットの命令にした狭い式の項 (`b >> 2` (b:u8) は u16 の命令) を同じ大きさの符号違いと見て、`a - (b >> 2)` (a:i16) を
// 誤ってエラーにしていた (変数の `a - b` は通る。Agent/issues/local-2026-10-18-mixed-sign-false-error.md。2026-10-19 に直した)。
// 本当に同じ大きさで符号が違う項 (`as i16`・同じ 8 ビット) は今までどおりエラー。
func TestBugMixedSignNarrowExpr(t *testing.T) {
	t.Parallel()
	const head = `#fc 4
use console;
var a:i16 = 100;
var b:u8 = 200;
var c:u8 = 3;
var gi80:i8 = -128;
var gi84:i8 = 0;
var gi87:i8 = 127;
var gu:u16 = 40000;
function id_u16(v:u16):u16 @(noinline) { return v; }
function main():void
{
`
	src := head + `	@printf("{} {} {} {}\n", a - (b >> 2), a - (b + c), gu + (gi84 ^ gi84), (284 * gi87) + (gi84 ^ gi84));
	@printf("{}\n", (gi80 << 3) * id_u16(293));
	console.exit(0);
}
`
	for _, level := range []int{0, -1} {
		r := testBuild(t, buildSpec{Files: map[string]string{"t.fc": src}, Level: level, Run: true})
		if r.Err != nil {
			t.Fatalf("level %d: ビルド失敗: %v", level, r.Err)
		}
		if want := "50 -103 40000 36068\n27648\n"; r.Stdout != want {
			t.Errorf("level %d: 出力 %q、want %q", level, r.Stdout, want)
		}
	}
	for _, e := range []string{"gu + (gi84 as i16)", "b + gi84", "(b + c) + gi84"} {
		r := testBuild(t, buildSpec{Files: map[string]string{"t.fc": head + "\t@printf(\"{}\\n\", " + e + ");\n}\n"}})
		if r.Err == nil || !strings.Contains(r.Err.Error(), "mixes") {
			t.Errorf("%s: 符号の混ざりのエラーにならない (err %v)", e, r.Err)
		}
	}
}

// TestBugAndZeroDeadCompare は、片方だけで結果が決まる `x & 0` を畳み、使われなくなった比較を残さないこと。関数ポインタの表を
// `(a <= b) & 0` で引くと、比較・真偽値・`and #0`・ジャンプテーブルがそのまま出ていた (fuzz の -O 2 で 4000 本に 37 本の
// 使われない cmp。Agent/issues/local-2026-10-18-dead-compare.md。2026-10-19 に直した)。テストでは codegen のフラグの検査が
// 使われない比較をエラーにする。
func TestBugAndZeroDeadCompare(t *testing.T) {
	t.Parallel()
	out, asm := buildShape(t, `#fc 4
use console;
var a0:[16]u8;
var n:u8;
function f():void @(noinline) { n = n + 1; }
function g():void @(noinline) { n = n + 2; }
const fp0:[2]fn():void = [f, g];
function t(l1:u8):void @(noinline)
{
	fp0[((a0[7] <= l1) as u8) & 0]();
}
function main():void
{
	a0[7] = 3;
	t(5);
	t(1);
	@printf("{}\n", n);
	console.exit(0);
}
`)
	if out != "2\n" {
		t.Errorf("出力 %q、want %q", out, "2\n")
	}
	body := asm[strings.Index(asm, ".proc _t_t\n"):]
	body = body[:strings.Index(body, ".endproc")]
	for _, bad := range []string{"\tcmp ", "\tand #0", "\tpha"} {
		if strings.Contains(body, bad) {
			t.Errorf("t に %q が残っている:\n%s", strings.TrimSpace(bad), body)
		}
	}
}

// TestBugEmptyCastDeadCompare は、ssa の畳み込みが作った「元の値のどのバイトも読まない cast」で、使われなくなった比較が残らない
// こと。`((x != 40) as u8 as u16 >> 8) as i8` は `cast<i8, 1>(真偽値)` (1 バイトの値の 2 バイト目 = いつも 0) に畳まれるが、cast の
// ままだと真偽値を使っている扱いで、比較 (`cmp #40`) が残っていた。0 にするのは後の pipeline の ZeroEmptyCasts で、その後は
// 消す段が無い (2026-10-23 に nomad-lab の fuzz の TestRandomConstFoldV4 -randseed 20261049578000 (種 20261049579941) が見つけた)。
// テストではフラグの検査が使われない比較をエラーにする。
func TestBugEmptyCastDeadCompare(t *testing.T) {
	t.Parallel()
	out, asm := buildShape(t, `#fc 4
use console;
function id_i8(x:i8):i8 @(noinline) { return x; }
var ga_i8:[3]i8;
function main():void
{
	ga_i8[1] = (((((((id_i8(127) != 40) as u8)) as u16) >> 8)) as i8);
	var w:i16 = ga_i8[1];
	@printf("{}\n", w);
	console.exit(0);
}
`)
	if out != "0\n" {
		t.Errorf("出力 %q、want %q", out, "0\n")
	}
	body := asm[strings.Index(asm, ".proc _main\n"):]
	body = body[:strings.Index(body, ".endproc")]
	if strings.Contains(body, "\tcmp #40") {
		t.Errorf("main に使われない cmp #40 が残っている:\n%s", body)
	}
}

// TestBugDeadCompareO0 は、使われない比較を消す段 (ssa の畳み込み・jumps の dropDeadTemps) が走らない -O 0 では、使われない
// 比較をフラグの検査がエラーにしないこと。`65536 == (u8 の値)` を sema が偽に畳み、中の `242 <= id_u8(255)` の比較だけが
// `cmp #242` のまま残って `asl a` に上書きされていた (出力は正しい。2026-10-20 に fuzz-loop の TestRandomConstFold
// -randseed 2026102012040002 が見つけた)。
func TestBugDeadCompareO0(t *testing.T) {
	t.Parallel()
	out, err := buildBothLevels(t, map[string]string{"t.fc": `#fc 3
use * from stdio;
function id_u8(x:u8):u8 @(noinline) { return x; }
var gv_u8:u8;
function main():void
{
	gv_u8 = (((65536 == ((242 <= id_u8(255)) as u8)) as u8) << 6);
	printf(gv_u8 as i16, "\n");
	exit(0);
}
`})
	if err != nil {
		t.Fatal(err)
	}
	if out != "0\n" {
		t.Errorf("出力 %q、want %q", out, "0\n")
	}
}

// TestBugSignedDivByMinPow2 は、符号付きの割り算・余りで型の符号付きの範囲を超える 2 のべきの定数 (i8 の 128、i24 の 8388608) を
// 正の 2 のべきとしてシフト・マスクにしないこと。型のない定数は相手の型に合わせるので `x / 128` (x:i8) は x / -128 で、定数の
// 畳み込みはそう計算するが、codegen は 128 を正の数としてシフトしていた (`id8(-128) / 128` が 1 でなく -1、`% 128` が -28 でなく
// 100。2026-10-24 に TestRandomArithV4 -randseed 20261024000 (種 20261024045) が i24 の形で見つけた。1 バイトの形は前からあった)。
func TestBugSignedDivByMinPow2(t *testing.T) {
	t.Parallel()
	out, err := buildBothLevels(t, map[string]string{"t.fc": `#fc 4
use console;
function id8(x:i8):i8 @(noinline) { return x; }
function id24(x:i24):i24 @(noinline) { return x; }
function main():void
{
	@printf("{} {}\n", id8(-128) / 128, (-128 as i8) / 128);
	@printf("{} {}\n", id8(100) / 128, id8(100) % 128);
	@printf("{} {}\n", id24(32768) / 8388608, id24(32768) % 8388608);
	console.exit(0);
}
`})
	if err != nil {
		t.Fatal(err)
	}
	if want := "1 1\n-1 -28\n-1 -8355840\n"; out != want {
		t.Errorf("出力 %q、want %q", out, want)
	}
}
