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
	// 引数が式 (push_result と push_arg の間に計算がある): 計算が消えない (castle の梯子判定 abs(x % 16 - 8) が壊れた)
	var e = 0;
	if ((g & 1) && sum3(1, 2, 3) & 2 && abs8(x % 16 - 8) <= 4) {
		e = 1;
	}
	var f = abs8(g * 3 - 20) + abs8(x + 30);
	printf(a, " ", b, " ", g, " ", tab[0], " ", c, " ", d, " ", e, " ", f, "\n");
	exit(0);
}
`)
	// x = -7 (249) → x % 16 - 8 = 9 - 8 = 1 → e = 1。g = 3 → |9 - 20| + |249 + 30 = 23| = 34
	if want := "7 5 3 2 8 10 1 34\n"; out != want {
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

// TestResidentCondRestore: 比較の結果がフラグ (次の if が見る) のとき、常駐レジスタの復帰 (ldx / ldy) が N / Z を壊さない。
// sw の `on_idx == i` (i@X) は `cmp; ldx; bne` になって Z が消え、castle の踏むスイッチが効かなかった (i == 0 のときだけ動く)。
// 今は可換なので cpx on_idx。f の 16 ビットの比較 (可換の形にできない) は復帰を php / plp で挟む。
func TestResidentCondRestore(t *testing.T) {
	t.Parallel()
	out := runEmu(t, `var tab:[8]int;
var tab2:[8]int;
var tab3:[8]int;
var g:int16;
var on_idx:int;
var cnt:int;
var cnt2:int;
function sw(i:int):void
{
	var x = tab[i];
	var y = tab2[i];
	var z = tab3[i];
	if (g == 4 && on_idx == i) {
		if (y < tab3[i] + 8) {
			y += 1;
		}
		cnt += i + 1; // どの i で一致したか (壊れると i == 0 で一致する)
	} else {
		if (y > tab3[i]) {
			y -= 1;
		}
	}
	tab[i] = x;
	tab2[i] = y + z;
}
function f():void
{
	for (var i = 0; i < 8; i++) {
		tab[i] = i;
		tab2[i] = tab[i] + 1;
		tab3[i] = tab2[i] + 1;
		tab[i] = tab3[i] + 1;
		tab2[i] = tab[i] + 1;
		tab3[i] = tab2[i] + 1;
		if (g == i) {
			cnt2 += 1;
		}
		if (i == g) {
			cnt2 += 10;
		}
	}
}
function main():void
{
	on_idx = 5;
	g = 4;
	for (var i = 0; i < 8; i++) { sw(i); }
	g = 3;
	f();
	printf(cnt, " ", cnt2, "\n");
	exit(0);
}
`)
	if want := "6 11\n"; out != want {
		t.Errorf("got %q\nwant %q", out, want)
	}
}

// TestFuzzFound1: ランダムプログラムの差分テスト (randprog_test.go) で最初に見つかった 4 件 (2026-09-19)。-O 0 と -O 2 の両方で走らせる。
//
//	(1) `!((a < b) as int16)`: cast を挟んだ not の入力だけがコンディションレジスタになり codegen が panic
//	(2) `a16[i] = 4`: 融合した index_pset が値の幅 (1 バイト) しか書かず上位バイトが残る
//	(3) `(x8 as int16)` を splitWords がバイトに分けるとき、上位バイトとして隣の番地を読む (part3 の g1 が 46024 になる)
//	(4) 使われない書き込み (`l0 = ...`) の位置が live range に入らず、ループ変数と番地を共有して無限ループ (part4、-O 0)
// struct まわりの 2 件 (拡張した fuzz で発覚):
// (1) fusePointer: struct 配列の要素の先頭フィールドへの書き込み `sa[i].f0 = 4` を index_pset (書く幅 = 要素 2 バイト)
//     にして隣のフィールド f1 を壊していた
// (2) splitWords: 1 バイトのフィールドを 2 バイトに広げた cast `(s0.f1 as int16)` の下位バイトを、元の struct 変数の
//     0 バイト目 (別のフィールド) として読んでいた
func TestStructFieldWidths(t *testing.T) {
	t.Parallel()
	src := `struct S { f0:sint; f1:sint; }
struct T { f0:int; f1:int; f2:int16; }
var sa:[4]S;
var s0:T;
var g3:sint16;
var g4:int;
function main():void
{
	var i:int;
	sa[0].f1 = 7;
	for (i = 0; i < 2; i++) {
		sa[((sa[0].f1 as int) - g4) & 3].f0 = 4;
	}
	s0.f1 = 202;
	for (var j:int = 5; j; j--) {
		g3 = (s0.f1 as int16) as sint16;
	}
	var w:sint16 = 0;
	if (((s0.f1 as sint16) & (sa[0].f1 as sint16))) { w = 1; } else { w = 2; }
	printf(sa[0].f0 as int, " ", sa[0].f1 as int, " ", sa[3].f0 as int, " ", sa[3].f1 as int, " ", g3, " ", w, "\n");
	exit(0);
}
`
	for _, level := range []int{-1, 0} {
		if got := runEmuLevel(t, src, level); got != "0 7 4 0 202 1\n" {
			t.Errorf("level %d: got %q", level, got)
		}
	}
}

// 符号付き 1 バイトの変数シフト: 左シフトは C を 0 にして回す (`cmp #128; rol` は右の算術シフトだけ。
// `-109 << 1` が 39 になっていた。SSA の定数畳み込みとの差分で発覚)
func TestShiftVarSigned(t *testing.T) {
	t.Parallel()
	src := `var g:sint;
var n:int;
function main():void
{
	g = (-109); n = 1;
	var a:sint = g << n;
	n = 2;
	var b:sint = g >> n;
	var u:int = 200;
	var c:int = u << n;
	var d:int = u >> n;
	printf(a as int, " ", b as int, " ", c, " ", d, "\n");
	exit(0);
}
`
	for _, level := range []int{-1, 0} {
		if got := runEmuLevel(t, src, level); got != "38 228 32 50\n" {
			t.Errorf("level %d: got %q", level, got)
		}
	}
}

// A に常駐した変数と、cast を挟んだ一時変数の `if` (`if ((!x) as int16)`): regalloc が「コンディションの一時変数」と
// 見なして A を壊さない扱いにしていたが、実際はメモリから A に読むので、常駐していた g3 を壊して飛んでいた (fuzz で発覚)
func TestResidentIfCast(t *testing.T) {
	t.Parallel()
	src := `var g3:sint;
var g4:int;
var a3:[16]int16;
function main():void
{
	var l3:int = 0;
	while ((((!((g4 as sint16) + a3[((g4) & 7)])) as sint16)) && l3 < 4) {
		l3++;
		for (var l4:sint = 0; l4 < 5; l4++) {
			g3 <<= 3; g3 ^= 66;
		}
	}
	printf(g3 as int, " ", l3, "\n");
	exit(0);
}
`
	for _, level := range []int{-1, 0} {
		if got := runEmuLevel(t, src, level); got != "210 4\n" {
			t.Errorf("level %d: got %q", level, got)
		}
	}
}

// `if (A || 定数の真)` の片側が畳まれて `if_true c goto next` (飛び先 = 落ちる先 = ループの入口) が残ると、
// 常駐レジスタの入口の写しが落ちる辺にしか付かず、飛ぶ辺から入ったときに X が未設定だった (fuzz で発覚)。
// simplifyJumps が直後への条件分岐を消し、regalloc は両方の辺に写しを置く
func TestResidentEntryBothEdges(t *testing.T) {
	t.Parallel()
	src := `var g1:int16;
var g2:int;
var a0:[16]int;
var a2:[16]int16;
function f0():int
{
	var l1:sint16 = 5;
	var q0:*int16 = &a2[2];
	var l5:int = 0;
	if ((((g1) as int) < (g2 / 2)) || (((l1 || (~((*q0) as sint16))) as int))) {
		while ((((((q0[1] as sint) << 2) && (-l1)) as int16)) && l5 < 5) {
			l5++;
			a0[2] = 2;
		}
	}
	return l5;
}
function main():void
{
	a2[2] = 1397; a2[3] = 7; g2 = 9;
	printf(f0(), " ", a0[2], "\n");
	exit(0);
}
`
	for _, level := range []int{-1, 0} {
		if got := runEmuLevel(t, src, level); got != "5 2\n" {
			t.Errorf("level %d: got %q", level, got)
		}
	}
}

// 定数との乗算のシフト・加減算への展開 (opt.expandMul): 1 バイト / 2 バイト、符号付き、2^n - 1、Dst が入力と同じ
func TestExpandMulRun(t *testing.T) {
	t.Parallel()
	src := `var a:int;
var b:sint;
var c:int16;
var d:sint16;
function main():void
{
	a = 13; b = (-7); c = 1234; d = (-300);
	var x:int = a * 10;
	var y:sint = b * 7;
	var z:int16 = c * 11;
	var w:sint16 = d * 3;
	a *= 3;
	c *= 100;
	printf(x, " ", y as int, " ", z, " ", w as int16, " ", a, " ", c, "\n");
	exit(0);
}
`
	for _, level := range []int{-1, 0} {
		if got := runEmuLevel(t, src, level); got != "130 207 13574 64636 39 57864\n" {
			t.Errorf("level %d: got %q", level, got)
		}
	}
}

// ポインタの添字参照 (`(p),y`) の codegen 2 件 (拡張した fuzz で発覚):
// (1) 要素 2 バイトの `iny` の後、添字が Y に常駐していれば Y を戻す (`q0[i] += 1` の index_pset が 1 バイトずれていた)
// (2) フレームがゼロページでない関数では pointerBase がポインタを reg に写すが、添字が A にあると A を壊すので、
//     先に添字を Y に置く。main の t2 はフレームを大きくして (印字用の一時変数) ゼロページから追い出した形
func TestPointerIndexY(t *testing.T) {
	t.Parallel()
	src := `var a2:[16]int16;
var a1:[16]sint;
var g0:int;
var r:int16;
function t1(i:int):void
{
	var q0:*int16 = &a2[3];
	q0[i] += 1;
	r = a2[8];
}
function main():void
{
	var q0:*sint = &a1[3];
	a2[8] = 2;
	t1(5);
	a1[3] = 1;
	a1[0] = (((q0[(g0 & 7)] as int) as sint) * 1);
	printf(r, " ", a1[0], " ", a1[1], " ", a1[2], " ", a1[3], " ", a1[4], " ", a1[5], " ", a1[6], " ", a1[7], " ", a1[8], " ", a1[9], " ", a1[10], " ", a1[11], " ", a1[12], " ", a1[13], " ", a1[14], " ", a1[15], " ", a2[0], " ", a2[1], " ", a2[2], " ", a2[3], " ", a2[4], " ", a2[5], " ", a2[6], " ", a2[7], " ", a2[8], " ", a2[9], " ", a2[10], " ", a2[11], "\n");
	exit(0);
}
`
	want := "3 1 0 0 1 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 3 0 0 0\n"
	for _, level := range []int{-1, 0} {
		if got := runEmuLevel(t, src, level); got != want {
			t.Errorf("level %d: got %q", level, got)
		}
	}
}

// 常駐レジスタと「メモリ上の定数シフト」: 2 バイトの `>> 8` (バイトの移動) は A を使うので、A に常駐している変数
// (x.hi@A) を先に退避しなければならない (regalloc.isMemShift が A-free と見なしていた。TestSplitWords の mix が
// SSA のコピー伝播で形が変わって発覚)
func TestResidentMemShift(t *testing.T) {
	t.Parallel()
	src := `function mix(a:int16):int16
{
	var x:int16 = a;
	x |= 0x8001;
	x &= 0xf7ff;
	return x + ((x >> 8) & 0x00ff);
}
function main():void
{
	printf(mix(0x1234), " ", mix(0), "\n");
	exit(0);
}
`
	for _, level := range []int{-1, 0} {
		if got := runEmuLevel(t, src, level); got != "37575 32897\n" {
			t.Errorf("level %d: got %q", level, got)
		}
	}
}

// 小さなループの完全展開 (opt.unrollLoops): crc8 の形、continue / break のある形、カウンタを本体で使う形、
// 入れ子。-O 0 と結果を比べる
func TestUnrollRun(t *testing.T) {
	t.Parallel()
	src := `var acc:int16;
function crc8(p:*int, length:int):int
{
	var crc = 0;
	var i:int;
	for (i = 0; i < length; i++) {
		crc ^= *p;
		for (var j = 8; j; j--) {
			if (crc & 0x80) {
				crc = (crc << 1) ^ 0x1d;
			} else {
				crc = crc << 1;
			}
		}
		p += 1;
	}
	return crc;
}
function tricky(n:int):int16
{
	var s:int16 = 0;
	var j:int;
	for (j = 0; j < 6; j++) {
		if (j == 2) { continue; }
		if (j == n) { break; }
		s += (j as int16) * 10;
		var k:int;
		for (k = 3; k > 0; k--) { s += k; }
	}
	return s;
}
function main():void
{
	var data:[5]int;
	data[0] = 1; data[1] = 2; data[2] = 3; data[3] = 250; data[4] = 7;
	printf(crc8(&data[0], 5), " ", tricky(9), " ", tricky(4), " ", tricky(0), "\n");
	exit(0);
}
`
	want := runEmuLevel(t, src, -1)
	if got := runEmuLevel(t, src, 0); got != want || len(want) < 5 {
		t.Errorf("-O 0: %q, -O 2: %q", want, got)
	}
}

// 誘導変数の統合 (opt.eliminateInduction): 比較にしか使われないカウンタをポインタの比較に置き換える。
// 初期値が上限以上でループに入らない場合、歩幅が変数 (外側のループの比較で上限が分かる) の場合、初期値がリテラルの場合
func TestInductionRun(t *testing.T) {
	t.Parallel()
	src := `var buf:[64]int;
function fill(k0:int16, lim_unused:int):int16
{
	var p = &buf[0];
	var k:int16 = k0;
	var n:int16 = 0;
	while (k < 40) {
		*p = 7;
		p += 1;
		k += 1;
		n += 1;
	}
	return n;
}
function sieve():int16
{
	var i:int16 = 0;
	var p = &buf[0];
	var count:int16 = 0;
	for (i = 0; i < 64; i++) {
		*p = 1;
		p += 1;
	}
	p = &buf[0];
	for (i = 0; i < 64; i++) {
		if (*p) {
			var prime:int16 = i + i + 3;
			var k:int16 = i + prime;
			var q = p;
			q += prime;
			while (k < 64) {
				*q = 0;
				q += prime;
				k += prime;
			}
			count++;
		}
		p += 1;
	}
	return count;
}
function main():void
{
	var a = fill(38, 0);
	var b = fill(45, 0);
	var c = fill(0, 0);
	var s:int16 = 0;
	var i:int;
	for (i = 0; i < 64; i++) { s += buf[i]; }
	printf(a, " ", b, " ", c, " ", s, " ", sieve(), " ", buf[0], buf[1], buf[2], buf[3], buf[4], buf[5], buf[6], buf[7], buf[8], buf[9], "\n");
	exit(0);
}
`
	for _, level := range []int{-1, 0} {
		if got := runEmuLevel(t, src, level); got != "2 0 40 280 30 1110110110\n" {
			t.Errorf("level %d: got %q", level, got)
		}
	}
}

// `!` の 2 バイトの入力: 全バイトが 0 のときだけ 1 (バイトごとに beq していて「どれかが 0」になっていた。
// SSA の定数畳み込みとの差分テストで発覚)
func TestNot16(t *testing.T) {
	t.Parallel()
	src := `function main():void
{
	var l1:int16 = 1;
	var l2:int16 = 256;
	var l3:int16 = 0;
	var s:sint = 0 - ((!l1) as sint);
	printf((!l1) as int, " ", (!l2) as int, " ", (!l3) as int, " ", s as int, "\n");
	exit(0);
}
`
	for _, level := range []int{-1, 0} {
		if got := runEmuLevel(t, src, level); got != "0 0 1 0\n" {
			t.Errorf("level %d: got %q", level, got)
		}
	}
}

func TestFuzzFound1(t *testing.T) {
	t.Parallel()
	src := `var g0:sint16;
var g1:int16;
var g2:sint;
var g5:int;
var a0:[8]sint16;
var a1:[8]int;
var b0:[8]sint;
var b:sint;
function f1(p0:int):int
{
	return ((a0[0] as int) & (g5 | 94));
}
function f0(p0:sint):int16 options(fastcall: true, inline: true)
{
	return (a1[((g1 as int) & 7)] as int16);
}
function f2(p0:sint16):int16 options(inline: true)
{
	return (((p0 as int) << 7) as int16);
}
function part3():int16
{
	var l0:int16 = 45981;
	g0 = (-5613);
	a1[3] = 200;
	g1 = 0x0303;
	switch ((((g0 as int) ^ ((l0 as int) >> 5)) & 7)) {
	case 7, 5:
		g1 = f0(g2);
	default:
	}
	return g1;
}
function part4():int
{
	var l0:int = 3;
	var l1:int16 = 4;
	var cnt = 0;
	if ((~(l1 as sint))) {
		switch ((((l0 >> (l0 & 7)) % 209) & 7)) {
		case 6:
		case 3:
			l0 = (f2((-(g2 >> 3))) as int);
		}
	}
	for (var l6:int = 0; l6 < 7; l6++) {
		for (var l7:int = 0; l7 < 4; l7++) {
			var l8:int16 = l1;
			l0 = (b0[6] as int); // 使われない書き込みが l6 / l7 の番地を壊していた
			cnt++;
		}
	}
	return cnt;
}
function main():void
{
	// (1)
	g1 = 3;
	var n1 = (!((g1 < 5) as int16)) as int;
	var n2 = (!((g1 == 3) as int16)) as int;
	var n3 = (!((g1 > 5) as int)) as int;
	// (2)
	b = -3;
	a0[3] = 3525;
	a0[4] = 3525;
	if (a1[f1(0)] == 0) { a0[3] = 4; }
	var i = 4;
	a0[i] = b;
	printf(n1, " ", n2, " ", n3, " ", a0[3], " ", a0[4], " ", part3(), " ", part4(), "\n");
	exit(0);
}
`
	for _, level := range []int{-1, 0} {
		if out, want := runEmuLevel(t, src, level), "0 0 1 4 65533 200 28\n"; out != want {
			t.Errorf("level %d: got %q\nwant %q", level, out, want)
		}
	}
}

// TestFuzzFound2: 差分テストで見つかった 2 巡目の 3 件 (2026-09-19)。
//
//	part1 (-O 0): `l ^ l` の l を A に置く (allocateA) と 2 つ目の入力がメモリから読めず panic
//	part2 (-O 0): `cast<sint16>(cast<uint16>(b8))` の 1 バイト目が b8 の隣の番地を読む (codegen の byte)
//	part3: 関数全体の常駐 (l1@X) の退避が、内側のループの入口の写し (l0@X の tax) の後に出て l1 が壊れる
//	part4: cast を挟んだ使用 `~(l1 as int16)` は常駐 (l1@Y) に置き換わらずメモリを読むのに、退避が出ていなかった
func TestFuzzFound2(t *testing.T) {
	t.Parallel()
	src := `var g0:int16;
var g1:sint;
var g3:int;
var g4:int16;
var a0:[8]sint16;
var b1:[8]int16;
var c1:[8]sint16;
function f1(p0:sint, p1:int16):int options(inline: true)
{
	var l0:int = 241;
	for (var l1:int = 0; l1 < 2; l1++) {
		l0++;
		b1[0] <<= 0; b1[((0 & l1) & 7)] ^= 61667;
	}
	return (((!p1) as int) / 39);
}
function part1():int
{
	var l6:int = g3;
	g3 = (l6 ^ l6);
	var l7:int = g3;
	return (l7 & l7) + (l6 - l6);
}
function part2():sint16
{
	c1[5] = 4; // 下位 4 は >> 4 で 0 になり、!0 = 1。壊れると隣の番地 (この 4) が上位バイトに入って 1025 になる
	a0[2] = (((!((c1[5] as int) >> 4)) as int16) as sint16);
	return a0[2];
}
function part3():int16
{
	var l0:sint = 0;
	var l1:int16 = 59596;
	var l4:int = 0;
	b1[1] = 1891;
	for (var l2:sint = 0; l2 < 1; l2++) {
	}
	b1[((((b1[1] as int) || l0) as int) & 7)] = (((((a0[0] / (l1 | 1)) > ((b1[(((a0[(f1(0, g0) & 7)] as int) | 3) & 7)] as sint16) as int16)) as sint16) != ((f1(g1, 4) as sint16) ^ 0)) as int16);
	l4++;
	return b1[0] + l4;
}
function part4():void
{
	var l1:int = 3;
	g3 = 27;
	l1 = g3;
	g4 = (~(l1 as int16));
	printf(g4, "\n");
}
function main():void
{
	g3 = 5;
	printf(part1(), " ", part2(), " ", part3(), " ");
	part4();
	exit(0);
}
`
	for _, level := range []int{-1, 0} {
		if out, want := runEmuLevel(t, src, level), "0 1 1 65508\n"; out != want {
			t.Errorf("level %d: got %q\nwant %q", level, out, want)
		}
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
