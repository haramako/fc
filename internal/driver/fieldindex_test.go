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
	if !strings.Contains(body, "sta _t_gs+1+0,y") || strings.Contains(body, "(F_t_f") {
		t.Errorf("gs[i].b への書き込みが添字の形になっていない:\n%s", body)
	}
}
