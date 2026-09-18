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

// TestResidentBranchRestore: A に常駐する変数を壊して検査する分岐 (2 バイトの `if (flag)`) は、飛ぶ側の経路でも
// A を復帰する (以前は落ちてくる側にしか復帰が無く、else 側で A = flag の上位バイトのまま計算していた)。
func TestResidentBranchRestore(t *testing.T) {
	t.Parallel()
	out := runEmu(t, `var tab:[8]int;
var flag:int16;
function calc():int
{
	var s = 0;
	for (var i = 0; i < 8; i++) {
		s += tab[i];
		s ^= 5;
		s += 1;
		s = s << 1;
		s -= 3;
		s ^= tab[i];
		if (flag) {
			s += 2;
		} else {
			s += 3;
		}
		s += tab[i];
		s ^= 1;
	}
	return s;
}
function main():void
{
	for (var i = 0; i < 8; i++) { tab[i] = i * 3; }
	flag = 0;
	var a = calc();
	flag = 256;
	var b = calc();
	printf(a, " ", b, "\n");
	exit(0);
}
`)
	if want := "189 140\n"; out != want {
		t.Errorf("got %q\nwant %q", out, want)
	}
}

// TestConstPointerArray: ポインタの配列の const (`[N]*T`): 要素は文字列リテラル、配列定数の名前、null。
// 二重配列の const (`[2][3]int`) と合わせて、定数添字・変数添字の両方で読める。
func TestConstPointerArray(t *testing.T) {
	t.Parallel()
	out := runEmu(t, `const A:[2][3]int = [[1, 2, 3], [4, 5, 6]];
const W:[2][2]int16 = [[1000, 2], [3, 40000]];
const S1:*int = "ab";
const T2:[]int = [7, 8, 9];
const PS:[3]*int = [S1, T2, "xyz"];
const NAMES:[2]*int = ["hello", "hi"];
function len(p:*int):int { var n = 0; while (p[n]) { n++; } return n; }
function main():void
{
	var i = 1;
	var j = 2;
	printf(A[1][2], " ", A[i][0], " ", W[1][1], " ", W[i][0], "\n");
	printf(PS[0][1], " ", PS[i][2], " ", PS[j][0], " ", len(NAMES[0]), " ", len(NAMES[i]), "\n");
	exit(0);
}
`)
	if want := "6 4 40000 3\n98 9 120 5 2\n"; out != want {
		t.Errorf("got %q\nwant %q", out, want)
	}
	got := compileErr(t, "var v:int;\nconst P:[1]*int = [&v];\n")
	if !strings.Contains(got, "constant address required") && !strings.Contains(got, "constant value required") {
		t.Errorf("非定数の要素: %q", got)
	}
}

// TestSwitchJumpTable: 10 個以上の整数の case が密に並ぶ switch はジャンプテーブル (`switch` 命令) になる。
// 隙間・範囲外は default、複数の値の case、最小値が 0 でない / 負の値、ループ内 (常駐レジスタと共存)、
// case 内の break、default 無し。比較の連鎖 (case が少ない) と結果が一致する。
func TestSwitchJumpTable(t *testing.T) {
	t.Parallel()
	out := runEmu(t, `var tab:[16]int;
function f(x:int):int
{
	switch (x) {
	case 0: return 10;
	case 1: return 11;
	case 2, 3: return 12;
	case 4: return 14;
	case 6: return 16;
	case 7: return 17;
	case 8: return 18;
	case 9: return 19;
	case 10: return 20;
	case 11: return 21;
	case 12: return 22;
	default: return 99;
	}
}
function g(x:sint):int
{
	var r = 0;
	switch (x) {
	case -3: r = 1;
	case -2: r = 2;
	case -1: r = 3;
	case 0: r = 4;
	case 1: r = 5;
	case 2: r = 6;
	case 3: r = 7;
	case 4: r = 8;
	case 5: r = 9; break;
	case 6: r = 10;
	case 7: r = 11;
	}
	return r + 100;
}
function main():void
{
	var s:int16 = 0;
	for (var i = 0; i < 16; i++) {
		tab[i] = i;
		switch (tab[i]) {
		case 1: s += 1;
		case 2: s += 2;
		case 3: s += 3;
		case 4: s += 4;
		case 5: s += 5;
		case 6: s += 6;
		case 7: s += 7;
		case 8: s += 8;
		case 9: s += 9;
		case 10: s += 10;
		case 11: s += 11;
		default: s += 100;
		}
		s += tab[i];
	}
	printf(f(0), " ", f(3), " ", f(5), " ", f(12), " ", f(13), " ", f(200), "\n");
	printf(g(-3), " ", g(0), " ", g(5), " ", g(7), " ", g(8), " ", g(-4), "\n");
	printf(s, "\n");
	exit(0);
}
`)
	// s: 1..11 の和 66 + default 5 回 (0, 12..15) × 100 + 0..15 の和 120 = 686
	if want := "10 12 99 22 99 99\n101 104 109 111 100 100\n686\n"; out != want {
		t.Errorf("got %q\nwant %q", out, want)
	}
}

// TestStepLoop: Y / X に常駐する添字の `i += k` (k ≤ 4) は iny × k、ループの出口のラベルが外側の if の終端と同じでも
// ループは回転する (castle の ppu.wait_vsync_with_flag のスプライト消去ループ)。
func TestStepLoop(t *testing.T) {
	t.Parallel()
	out := runEmu(t, `var buf:[64]int;
var flag:int;
function clear(start:int, f:int):void
{
	if (f != 2) {
		if (!flag) {
			for (var i = start; i < 64; i += 4) {
				buf[i] = 255;
			}
		} else {
			for (var i = 0; i <= start; i += 3) {
				buf[i] = 7;
			}
		}
	}
	for (var j = 62; j > 40; j -= 2) {
		buf[j] += 1;
	}
}
function main():void
{
	var s:int16 = 0;
	clear(20, 0);
	flag = 1;
	clear(12, 0);
	clear(0, 2);
	for (var i = 0; i < 64; i++) { s += buf[i]; }
	printf(buf[20], " ", buf[21], " ", buf[24], " ", buf[12], " ", buf[13], " ", buf[62], " ", buf[41], " ", s, "\n");
	exit(0);
}
`)
	// 期待値は Python で同じ計算を再現して求めた (development_notes.md)
	if want := "255 0 255 7 0 3 0 1593\n"; out != want {
		t.Errorf("got %q\nwant %q", out, want)
	}
}

// TestShiftByte: 2 バイト値の 8 以上の定数シフトはバイトの移動 (符号付き >> 8 は符号を埋める。9 以上、16 以上、その場)。
func TestShiftByte(t *testing.T) {
	t.Parallel()
	out := runEmu(t, `var u:int16;
var s:sint16;
function main():void
{
	u = 0xa5c3;
	s = -300;   // 0xfed4
	var a = u >> 8;
	var b = u << 8;
	var c = s >> 8;      // -2 (0xfffe)
	var d = u >> 10;
	var e = u << 9;
	var f = u >> 16;
	var g = (s >> 8) as sint8;
	var h = (u >> 8) as int + 1;
	s = s >> 8;          // その場
	u = u << 8;
	printf(a, " ", b, " ", c, " ", d, " ", e, " ", f, " ", g, " ", h, " ", s, " ", u, "\n");
	exit(0);
}
`)
	// a = 0xa5 = 165, b = 0xc300 = 49920, c = 0xfffe = 65534, d = 0x29 = 41, e = 0x8600 = 34304, f = 0,
	// g = -2 → 65534 (8 ビットも符号拡張して表示), h = 166, s = 65534, u = 49920
	if want := "165 49920 65534 41 34304 0 65534 166 65534 49920\n"; out != want {
		t.Errorf("got %q\nwant %q", out, want)
	}
}

// TestSplitWords: 2 バイトの変数を上位 / 下位に分ける最適化 (opt.splitWords) の実行結果: シフト (rolc / rorc で C を通す)、
// xor / and / or、<< 8 / >> 8、if、分解できない使用 (加算・引数・戻り値) の前後の実体化。CRC-16 の形と、混ぜた形。
func TestSplitWords(t *testing.T) {
	t.Parallel()
	out := runEmu(t, `var data:[4]int;
function crc16(n:int):int16
{
	var crc:int16 = 0xffff;
	for (var i = 0; i < n; i++) {
		crc ^= (data[i] as int16) << 8;
		for (var j = 8; j; j--) {
			if (crc & 0x8000) {
				crc = (crc << 1) ^ 0x1021;
			} else {
				crc = crc << 1;
			}
		}
	}
	return crc;
}
function mix(a:int16):int16
{
	var x:int16 = a;
	for (var k = 0; k < 3; k++) {
		x = x >> 1;          // 符号なしの右回転
		x |= 0x8001;
		x &= 0xf7ff;
		x += 3;              // 分解できない: 実体化
		if (x) { x ^= 0x0100; }
	}
	return x + ((x >> 8) & 0x00ff);
}
function main():void
{
	data[0] = 0x31; data[1] = 0x32; data[2] = 0x33; data[3] = 0x34;
	printf(crc16(4), " ", crc16(0), " ", mix(0x1234), " ", mix(0), "\n");
	exit(0);
}
`)
	// 期待値は Python で同じ計算を再現して求めた
	if want := "21321 65535 57965 58023\n"; out != want {
		t.Errorf("got %q\nwant %q", out, want)
	}
}

// TestFuseIndexedOperand: `x - tab[i]` / `x < tab[i]` の第 2 入力を直前の index_pget と融合する (sta t; lda x; sbc t →
// lda x; sbc tab,y)。可換な演算は opt.commuteTemp で第 1 入力になるので対象外。結果が他でも使われるなら融合しない。
func TestFuseIndexedOperand(t *testing.T) {
	t.Parallel()
	out := runEmu(t, `var tab:[8]int;
var stab:[8]sint8;
function f(x:int, i:int):int
{
	var d = x - tab[i];
	var lt = 0;
	if (x < tab[i]) { lt = 1; }
	var t = tab[i];
	var e = x - t + t;   // t は 2 回使う: 融合しない
	return d + lt * 10 + e;
}
function g(x:sint8, i:int):int
{
	if (x < stab[i]) { return 1; }   // 符号付き
	return 0;
}
function main():void
{
	for (var i = 0; i < 8; i++) { tab[i] = i * 3; stab[i] = (i as sint8) - 4; }
	printf(f(10, 2), " ", f(5, 3), " ", g(-2, 1), " ", g(-2, 6), " ", g(3, 7), "\n");
	exit(0);
}
`)
	// f(10,2): d = 4, lt = 0, e = 10 → 14。f(5,3): d = 5-9 = 252, lt = 1, e = 5 → 252+10+5 = 267 → 11 (8 ビット)
	// g(-2,1): stab[1] = -3 → -2 < -3 は偽 → 0。g(-2,6): stab[6] = 2 → 1。g(3,7): stab[7] = 3 → 0
	if want := "14 11 0 1 0\n"; out != want {
		t.Errorf("got %q\nwant %q", out, want)
	}
}
