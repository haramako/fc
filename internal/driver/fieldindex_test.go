package driver

// opt.foldFieldIndex (struct の配列のフィールドの読み書きを `lda a+k,y` / `sta a+k,y` にする) のテスト。

import (
	"strings"
	"testing"
)

// TestFieldIndex: 要素の大きさ 2・3・5 (u16・i8 のフィールド)、先頭以外のフィールドの読み書き、for-each の要素のポインタ、
// i8 の添字、全体が 256 バイトを超える配列 (置き換えない)、要素全体のコピーで、-O 0 と -O 2 の結果が同じ。
func TestFieldIndex(t *testing.T) {
	t.Parallel()
	out, err := buildBothLevels(t, map[string]string{"t.fc": `#fc 3
use * from stdio;
struct E { hp:u8; x:u8; }
struct F { a:u8; w:u16; c:i8; }
struct G { a:u8; b:u8; c:u8; }
var es:[8]E;
var fs:[10]F;
var gs:[5]G;
var big:[100]F;
function main():void
{
	for (var i in 0..8) { es[i].hp = i + 10; es[i].x = i * 3; }
	for (var i in 0..8) { es[i].hp -= 1; es[i].x += es[i].hp; }
	for (var e in es) { printf(e.hp, ",", e.x, " "); }
	printf("\n");
	for (var i in 0..10) { fs[i].w = i as u16 * 1000; fs[i].c = -(i as i8); fs[i].a = i; }
	for (var p in &fs) { p.w += 7; p.c -= 1; }
	for (var f in fs) { printf(f.a, ",", f.w, ",", f.c, " "); }
	printf("\n");
	for (var i in 0..5) { gs[i].b = i + 1; gs[i].c = gs[i].b * 2; gs[i].a = gs[i].c + gs[i].b; }
	var k:i8 = 3;
	gs[k].a = 99;
	for (var g in gs) { printf(g.a, ",", g.b, ",", g.c, " "); }
	printf("\n");
	for (var i in 0..100) { big[i].w = i as u16; big[i].c = 1; }
	var s:u16 = 0;
	for (var f in big) { s += f.w + (f.c as u16); }
	var e0:E = es[2];
	es[3] = e0;
	printf(s, " ", es[3].hp, ",", es[3].x, "\n");
	exit(0);
}
`})
	want := "9,9 10,13 11,17 12,21 13,25 14,29 15,33 16,37 \n" +
		"0,7,-1 1,1007,-2 2,2007,-3 3,3007,-4 4,4007,-5 5,5007,-6 6,6007,-7 7,7007,-8 8,8007,-9 9,9007,-10 \n" +
		"3,1,2 6,2,4 9,3,6 99,4,8 15,5,10 \n5050 11,17\n"
	if err != nil || out != want {
		t.Errorf("got %q, %v\nwant %q", out, err, want)
	}
	// 生成コード: 先頭以外のフィールドの書き込みが `sta a+k,y` になる (ポインタを組み立てない)
	asm := compileAsmFiles(t, map[string]string{"t.fc": `#fc 3
struct G { a:u8; b:u8; c:u8; }
var gs:[40]G;
function f(n:u8):void @(noinline) { for (var p in &gs) { p.b -= n; } }
function main():void { f(1); }
`})
	body := asm[strings.Index(asm, ".proc _t_f\n"):]
	body = body[:strings.Index(body, ".endproc")]
	if !strings.Contains(body, "sta _t_gs+1,y") || strings.Contains(body, "(F_t_f") {
		t.Errorf("gs[i].b への書き込みが添字の形になっていない:\n%s", body)
	}
}

// TestFieldPtr: struct の配列フィールドをポインタ経由で引く (opt.fuseArrayField) と、add と index の 16 ビットの番地の
// 計算が消えて、ポインタ + 添字 + ずれの 1 命令 (`lda i; asl; clc; adc #k; tay; lda (p),y`) になる。要素 1・2・3 バイト、
// 読みと書き、添字の式、Y に常駐しうる添字のループで、-O 0 と -O 2 の結果が同じ。
func TestFieldPtr(t *testing.T) {
	t.Parallel()
	out, err := buildBothLevels(t, map[string]string{"t.fc": `#fc 3
use * from stdio;
struct P { id:u8; q:u8; }
struct T3 { a:u8; w:u16; }
struct D { pad:u8; items:[3]P; w:[4]u16; b:[5]u8; t3:[2]T3; tail:u8; }
var ds:[2]D;
function set(p:*D, i:u8, v:u8):void @(noinline) {
	p.items[i].q = v;
	p.items[i].id = v + 1;
	p.w[i] = v as u16 * 300;
	p.b[i + 1] = v;
	p.t3[i & 1].w = v as u16 + 1000;
}
function sum(p:*D, k:u8):u16 @(noinline) {
	var s:u16 = p.items[k].q;
	s += p.items[k].id;
	s += p.w[k];
	s += p.b[k];
	s += p.t3[k & 1].w;
	return s;
}
function walk(p:*D, n:u8):u8 @(noinline) {
	var s:u8 = 0;
	for (var i = 0; i < n; i++) { s += p.b[i]; p.b[i] = s; }
	return s;
}
function main():void {
	for (var i = 0; i < 3; i++) { set(&ds[0], i, i + 10); set(&ds[1], i, i * 7); }
	printf(sum(&ds[0], 0), " ", sum(&ds[0], 2), " ", sum(&ds[1], 1), "\n");
	printf(walk(&ds[0], 5), " ", walk(&ds[1], 4), "\n");
	for (var d in ds) { printf(d.pad, ",", d.items[0].id, ",", d.items[2].q, ",", d.w[2], ",", d.b[3], ",", d.b[4], ",", d.t3[0].w, ",", d.tail, " "); }
	printf("\n");
	exit(0);
}
`})
	want := "4033 4648 3122\n33 21\n0,11,12,3600,33,33,1012,0 0,1,14,4200,21,0,1014,0 \n"
	if err != nil || out != want {
		t.Errorf("got %q, %v\nwant %q", out, err, want)
	}
	asm := compileAsmFiles(t, map[string]string{"t.fc": `#fc 3
struct P { id:u8; q:u8; }
struct D { pad:u8; items:[3]P; b:[5]u8; }
var d0:D;
function g(p:*D, i:u8):u8 @(noinline) { return p.items[i].q + p.b[i]; }
function main():void { var x = g(&d0, 1); d0.pad = x; }
`})
	body := asm[strings.Index(asm, ".proc _t_g\n"):]
	body = body[:strings.Index(body, ".endproc")]
	if !strings.Contains(body, "adc #2") || !strings.Contains(body, "adc #7") || strings.Contains(body, "adc #0") {
		t.Errorf("p.items[i].q / p.b[i] が (p),y の 1 回の参照になっていない:\n%s", body)
	}
}
