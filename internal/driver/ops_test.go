package driver

// v2 で足した演算子 (複合代入、~) と switch の case の規則。

import (
	"strings"
	"testing"
)

func TestCompoundAssignAndBitNot(t *testing.T) {
	t.Parallel()
	out := runEmu(t, `var g:int16;
struct P { v:int; }
var ps:[2]P;
function main():void
{
	var a = 0b1010;
	a |= 0b0101;
	var b = a;
	a &= 0b0110;
	var c = a;
	a ^= 0xff;
	var d = a;
	a <<= 2;
	var e = a;
	a >>= 5;
	var f = a;
	a = 7;
	a *= 6;
	var h = a;
	a /= 4;
	var i = a;
	a %= 4;
	var j = a;
	var k = ~a;
	printf(b, " ", c, " ", d, " ", e, " ", f, " ", h, " ", i, " ", j, " ", k, "\n");
	rest();
	exit(0);
}
function rest():void
{
	g = 1000;
	g -= 1;
	g <<= 1;
	g |= 1;
	var p = &ps[1];
	p.v = 3;
	p.v |= 4;
	ps[0].v = 1;
	ps[0].v <<= 3;
	printf(g, " ", p.v, " ", ps[0].v, " ", ~0x0f & 0xff, "\n");
}
`)
	want := "15 6 249 228 7 42 10 2 253\n1999 7 8 240\n"
	if out != want {
		t.Errorf("got %q\nwant %q", out, want)
	}
}

func TestSwitchCases(t *testing.T) {
	t.Parallel()
	// 空の case は「何もしない」(fall through しない)。default も空でよい
	out := runEmu(t, `function f(x:int):int
{
	switch (x) {
	case 0:
	case 1, 2:
		return 10;
	case 3:
		return 30;
	default:
	}
	return 99;
}
function main():void
{
	printf(f(0), " ", f(1), " ", f(2), " ", f(3), " ", f(4), "\n");
	exit(0);
}
`)
	if out != "99 10 10 30 99\n" {
		t.Errorf("got %q", out)
	}

	got := compileErr(t, "function f(x:int):void { switch (x) { case 1: break; case 2, 1: break; } }\n")
	if !strings.Contains(got, "duplicate case value 1") {
		t.Errorf("重複 case: %q", got)
	}
	got = compileErr(t, "const A = 5;\nfunction f(x:int):void { switch (x) { case A: break; case 5: break; } }\n")
	if !strings.Contains(got, "duplicate case value 5") {
		t.Errorf("重複 case (const): %q", got)
	}
}

// TestDivMod16: 16 ビットの除算・剰余 (__div_16s / __mod_16 / __mod_16s と 2 のべき乗の算術シフト)。
// 丸めは 8 ビットと同じ床除算 (商は負の無限大方向、余りの符号は除数に合わせる)。
// bench/math16.fc を書いたときに __mod_16 が rts だけの stub、符号付き 16 ビットが符号無しの __div_16 を呼んでいた、
// 2 のべき乗の符号付き除算が `cmp $80` (ゼロページ参照) を出していた、の 3 つが見つかった。
func TestDivMod16(t *testing.T) {
	t.Parallel()
	out := runEmu(t, `var u:int16;
var s:sint16;
var a:sint8;
function main():void
{
	u = 65236;
	printf(u / 7, " ", u % 7, " ", u / 4, " ", u % 8, "\n");
	s = -300;
	printf(s / 4, " ", s / 3, " ", s % 4, " ", s % 3, " ", s % -7, " ", s / -7, "\n");
	s = 300;
	printf(s / 4, " ", s / 7, " ", s % 7, " ", s / -7, " ", s % -7, "\n");
	s = -32768;
	printf(s / 3, " ", s % 3, "\n");
	a = -100;
	printf(a / 7, " ", a % 7, " ", a / 4, " ", a % 4, "\n");
	exit(0);
}
`)
	// printf は 8 ビット値も 16 ビットに符号拡張して表示する (a / 7 = -15 → 65521)
	want := "9319 3 16309 4\n65461 65436 0 0 65530 42\n75 42 6 65493 65535\n54613 1\n65521 5 65511 0\n"
	if out != want {
		t.Errorf("got %q\nwant %q", out, want)
	}
}

// TestCompoundAssignCallOnce: 複合代入の左辺に関数呼び出しがあっても 1 回しか呼ばない (`a[f()] += 8`、`g().v += 5`)。
// 左辺が単純なときは index を 2 回計算する脱糖のまま (index_pget / index_pset に融合される)。
func TestCompoundAssignCallOnce(t *testing.T) {
	t.Parallel()
	out := runEmu(t, `struct P { v:int; }
var a:[4]int;
var ps:[2]P;
var calls:int;
function f():int { calls++; return 1; }
function g():*P { calls++; return &ps[1]; }
function main():void
{
	a[f()] += 8;
	a[f()] |= 1;
	g().v += 5;
	printf(calls, " ", a[1], " ", ps[1].v, "\n");
	exit(0);
}
`)
	if want := "3 9 5\n"; out != want {
		t.Errorf("got %q\nwant %q", out, want)
	}
}

// TestPointerSelfRead: 読み先がポインタ自身 (`p = p.next`、`q = q[1]`、`pq = *pq`) でも壊れない
// (ゼロページのポインタは (p),y で直接読むが、下位バイトを書いてから上位バイトを読むと壊れるので reg 経由に戻す)。
func TestPointerSelfRead(t *testing.T) {
	t.Parallel()
	out := runEmu(t, `struct Node { value:int; next:*Node; }
var nodes:[3]Node;
var table:[3]*Node;
function main():void
{
	nodes[0] = {1, &nodes[1]};
	nodes[1] = {2, &nodes[2]};
	nodes[2] = {3, null};
	table[0] = &nodes[2];
	table[1] = &nodes[1];
	var s = 0;
	var p = &nodes[0];
	while (p != null) {
		s += p.value;
		p = p.next;
	}
	var q = table as **Node;
	q = bitcast<**Node>(q[1]);
	var pq = &table[0];
	pq = bitcast<**Node>(*pq);
	printf(s, " ", bitcast<*Node>(q).value, " ", bitcast<*Node>(pq).value, "\n");
	exit(0);
}
`)
	if want := "6 2 3\n"; out != want {
		t.Errorf("got %q\nwant %q", out, want)
	}
}

// TestGlobalResident: ループ内でレジスタに常駐するグローバル変数は、呼び出し・ポインタ経由の書き込みの前後で
// メモリと同期される (呼び先や別名経由の変更が反映され、ループを抜けた後の値も正しい)。
func TestGlobalResident(t *testing.T) {
	t.Parallel()
	out := runEmu(t, `var g:int;
var h:int;
var arr:[4]int;
function bump():void { g += 10; }
function main():void
{
	var p = &h;
	g = 0;
	h = 0;
	for (var i = 0; i < 4; i++) {
		g += 1;       // レジスタに常駐しうる
		bump();       // 呼び先が g を変える
		g += 1;
		h += 2;
		*p = h + 100; // ポインタ経由で h を変える
		h += 1;
		arr[i] = g;
	}
	printf(g, " ", h, " ", arr[0], " ", arr[3], "\n");
	exit(0);
}
`)
	// g: 毎周 +12 → 48、h: (2+100... 毎周 h = (h+2)+100+1) → 103, 206, 309, 412 → 412 - 256 = 156 (8 ビット)
	if want := "48 156 12 48\n"; out != want {
		t.Errorf("got %q\nwant %q", out, want)
	}
}

// TestChainSelfOperand: `x = (x op1 a) op2 x` の 2 つ目の x は演算前の値 (opt.chainInPlace が中間の一時変数を
// x 自身に置き換えるのは、後の演算が x を読まないときだけ)。
func TestChainSelfOperand(t *testing.T) {
	t.Parallel()
	out := runEmu(t, `function main():void
{
	var x:int16 = 5;
	var y:int16 = 6;
	var z:int16 = 7;
	x = (x << 1) ^ x;
	y = (y + 1) - y;
	z = (z + 1) + 2;
	printf(x, " ", y, " ", z, "\n");
	exit(0);
}
`)
	if want := "15 1 10\n"; out != want {
		t.Errorf("got %q\nwant %q", out, want)
	}
}

// TestScaledIndex: 要素 2 バイトの配列 / ポインタの添字は opt.scaleIndex がブロック内で 1 度だけ 2 倍する
// (添字が書き換わったら作り直す。ループ内では 2 倍した添字が Y に常駐する。ポインタ経由でも同じ)。
func TestScaledIndex(t *testing.T) {
	t.Parallel()
	out := runEmu(t, `var a:[8]int16;
var b:[8]sint16;
function fill(p:*int16, n:int):void
{
	for (var i = 0; i < n; i++) {
		p[i] = (i as int16) * 300;
	}
}
function main():void
{
	fill(a, 8);
	for (var i = 0; i < 8; i++) {
		b[i] = (a[i] as sint16) - 1000;
	}
	var s:int16 = 0;
	for (var i = 0; i < 8; i++) {
		s += a[i];
		i++;
		s += a[i];  // 添字が変わった後は 2 倍し直す
		s += b[i] as int16;
		a[i] = s;
	}
	var p = b as *sint16;
	var j = 3;
	p[j] = p[j] + p[j + 1];
	printf(s, " ", a[7], " ", b[3], " ", a[6], " ", b[7], "\n");
	exit(0);
}
`)
	// a = 0,300,...,2100; b = a - 1000
	// s: i=0: 0 → i=1: 300, +(300-1000=-700 → 64836) → s=300-700=-400 (65136); a[1]=65136
	//    i=2: +600 → 200; i=3: +900 → 1100; +(-100) → 1000; a[3]=1000
	//    i=4: +1200 → 2200; i=5: +1500 → 3700; +500 → 4200; a[5]=4200
	//    i=6: +1800 → 6000; i=7: +2100 → 8100; +1100 → 9200; a[7]=9200
	// p[3] = b[3] + b[4] = -100 + 200 = 100; a[6] = 1800; b[7] = 1100
	if want := "9200 9200 100 1800 1100\n"; out != want {
		t.Errorf("got %q\nwant %q", out, want)
	}
}

// TestResidentSignedCompare: A に常駐する符号付きの変数を比較 (sec; sbc; bvc; eor で A が壊れる) した後も使う
// (math.sin の形。関数全体の領域で x が A に置かれ、比較の後の tab[x] が壊れた値を添字にしていた)。
func TestResidentSignedCompare(t *testing.T) {
	t.Parallel()
	out := runEmu(t, `var tab:[4]int;
function f(x:sint8):int
{
	if (x < 0) {
		if (x < -2) {
			return tab[x + 4] + 200;
		}
		return tab[x + 3] + 100;
	}
	if (x < 2) {
		return tab[x];
	}
	return tab[x - 2] + 50;
}
function main():void
{
	tab[0] = 10; tab[1] = 20; tab[2] = 30; tab[3] = 40;
	printf(f(-4), " ", f(-1), " ", f(1), " ", f(3), "\n");
	exit(0);
}
`)
	if want := "210 130 20 70\n"; out != want {
		t.Errorf("got %q\nwant %q", out, want)
	}
}

// TestFunctionResident: ループの外 (関数の直線部分) でも引数などをレジスタに常駐させる。中のループが同じレジスタを
// 使っても、境界の写しの順序 (外側の退避 → 内側の復帰、内側の退避 → 外側の復帰) が保たれる。
// 領域内で書き換えられない変数 (k) は書き戻し無し (Clean)。
func TestFunctionResident(t *testing.T) {
	t.Parallel()
	out := runEmu(t, `var a:[16]int;
var b:[16]int;
var c:[16]int;
var g:int;
function f(k:int, n:int):int
{
	b[k] = a[k] + 1;
	c[k] = a[k] + b[k];
	var s = 0;
	for (var i = 0; i < n; i++) {
		s += a[i];
	}
	c[k] = c[k] + s;
	a[k] = b[k] + c[k];
	g += a[k];
	if (b[k] == 7) {
		return b[k];
	}
	return a[k];
}
function main():void
{
	for (var i = 0; i < 16; i++) { a[i] = i * 2; }
	g = 0;
	var r1 = f(3, 4);
	var r2 = f(6, 8);
	printf(r1, " ", r2, " ", g, " ", a[3], " ", a[6], " ", c[6], "\n");
	exit(0);
}
`)
	// f(3,4): b[3]=7, c[3]=13, s=12, c[3]=25, a[3]=32, g=32 → 7。f(6,8): b[6]=13, c[6]=25, s=0+2+4+32+8+10+12+14=82,
	// c[6]=107, a[6]=120, g=152 → 120
	if want := "7 120 152 32 120 107\n"; out != want {
		t.Errorf("got %q\nwant %q", out, want)
	}
}

// TestIndexSmallStructArray: 配列全体が 256 バイト以内なら `&objs[i]` (要素が 3 バイト以上) の積を 8 ビットで計算する
// (末尾の要素、要素 6 バイト / 5 バイト、256 バイトちょうどの配列)。
func TestIndexSmallStructArray(t *testing.T) {
	t.Parallel()
	out := runEmu(t, `struct Obj { x:int; y:int; tile:int; flip:int; pal:int; visible:int; }
struct P5 { a:int; b:int16; c:int16; }
var objs:[24]Obj;
var ps:[51]P5;
struct Q { a:int16; b:int16; }
var exact:[64]Q;   // 256 バイトちょうど
function main():void
{
	for (var i = 0; i < 24; i++) {
		var o = &objs[i];
		o.x = i * 3;
		o.visible = i & 1;
	}
	for (var j = 0; j < 51; j++) {
		var p = &ps[j];
		p.b = (j as int16) * 100;
	}
	for (var k = 0; k < 64; k++) {
		var q = &exact[k];
		q.a = (k as int16) * 1000;
		q.b = k as int16;
	}
	var s:int16 = 0;
	for (var i = 0; i < 24; i++) {
		var o = &objs[i];
		s += o.x + o.visible;
	}
	printf(s, " ", objs[23].x, " ", ps[50].b, " ", ps[1].b, " ", exact[63].a, " ", exact[62].b, "\n");
	exit(0);
}
`)
	// s = 3*(0+..+23) + 12 = 828 + 12 = 840、objs[23].x = 69、ps[50].b = 5000、exact[63].a = 63000、exact[62].b = 62
	if want := "840 69 5000 100 63000 62\n"; out != want {
		t.Errorf("got %q\nwant %q", out, want)
	}
}

// TestOptimizeLevel0: `-O 0` (BuildOptions.OptimizeLevel = -1) でも正しいコードが出る (以前は「未指定」と区別されず、
// 区別した途端に -O 0 用の簡易な割付器が静的フレームに対応していなくて壊れていた)。
func TestOptimizeLevel0(t *testing.T) {
	t.Parallel()
	src := `var a:[4]int;
var g:int16;
function f(k:int):int16
{
	var s:int16 = 0;
	for (var i = 0; i < 4; i++) {
		a[i] = i + k;
		s += a[i];
	}
	g = s * 3;
	return s;
}
function main():void
{
	printf(f(2), " ", g, " ", a[3], "\n");
	exit(0);
}
`
	want := "14 42 5\n"
	if out := runEmuLevel(t, src, -1); out != want {
		t.Errorf("-O 0: got %q want %q", out, want)
	}
	if out := runEmuLevel(t, src, 1); out != want {
		t.Errorf("-O 1: got %q want %q", out, want)
	}
}

// TestBoolCompareResult: 比較・論理演算の結果は bool (0 / 1)。uint8 と互換なので整数の引数・戻り値・代入・算術に
// そのまま使え、`var f = a < b` の f (bool) も if / & / 加算に使える。定数畳み込みも同じ。
func TestBoolCompareResult(t *testing.T) {
	t.Parallel()
	out := runEmu(t, `var tab:[4]int;
var g:int;
function isBig(x:int):int { return x > 100; }
function count(b:bool):void { g += b; }
function main():void
{
	var a = 5;
	var f = a < 7;          // bool
	var h = a == 5 && a != 6;
	var n = !f;
	var k:int = (a > 1) + (a > 2) + (a > 9); // 2
	tab[f] = 9;             // 添字にも使える (1)
	count(a < 3);
	count(f);
	count(true);
	const C = 3 < 4;
	printf(f, " ", h, " ", n, " ", k, " ", tab[1], " ", isBig(200), " ", isBig(2), " ", g, " ", C, " ", (f == h), " ", f & 1, "\n");
	exit(0);
}
`)
	if want := "1 1 0 2 9 1 0 2 1 1 1\n"; out != want {
		t.Errorf("got %q\nwant %q", out, want)
	}
}

// TestInlineFunction: options(inline: true) の関数は呼び出し側に展開される (引数・途中の return・void・入れ子・
// 別モジュールの葉関数)。展開しない形 (別の呼び出しの引数の中) も結果は同じ。
func TestInlineFunction(t *testing.T) {
	t.Parallel()
	out := runEmu(t, `var g:int;
var tab:[4]int;
function abs8(i:sint):int options(inline: true)
{
	if (i < 0) {
		return -i;
	}
	return i;
}
function bump(n:int):void options(inline: true)
{
	g += n;
	if (g > 100) {
		g = 0;
		return;
	}
	tab[0] += 1;
}
function twice(i:sint):int options(inline: true)
{
	return abs8(i) + abs8(i);   // inline の中の inline
}
function sum3(a:int, b:int, c:int):int { return a + b + c; }
function main():void
{
	var x:sint = -7;
	var a = abs8(x);
	var b = abs8(5);
	bump(50);
	bump(60);       // ここで g = 0 に
	bump(3);
	var c = twice(-4);
	var d = sum3(abs8(-1), abs8(-2), abs8(x)); // 引数の中 (評価順が保たれる)
	printf(a, " ", b, " ", g, " ", tab[0], " ", c, " ", d, "\n");
	exit(0);
}
`)
	if want := "7 5 3 2 8 10\n"; out != want {
		t.Errorf("got %q\nwant %q", out, want)
	}

	got := compileErr(t, "function f(i:int):int options(inline: true) { if (i) { return f(i - 1); } return 0; }\nfunction main():void { f(1); }\n")
	if !strings.Contains(got, "is recursive") {
		t.Errorf("再帰の inline: %q", got)
	}
	got = compileErr(t, "function f():void options(inline: true, symbol: \"_ext\");\nfunction main():void { f(); }\n")
	if !strings.Contains(got, "has no body") {
		t.Errorf("extern の inline: %q", got)
	}
}
