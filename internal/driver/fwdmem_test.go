package driver

import (
	"strings"
	"testing"
)

// TestForwardFields: struct を指すポインタのフィールドの読み直しを、直前に読んだ値・書いた定数で置き換える (opt の fwdmem)。
// 置き換えてはいけない形 (同じ所を指す別のポインタ・グローバル変数・アドレスを取った局所変数への書き込み、呼び出し、ポインタの
// 付け替え、合流) で値が正しいことと、pad.update の形で 2 回目の p.held を読まないこと。
func TestForwardFields(t *testing.T) {
	t.Parallel()
	out, asm := buildShape(t, `#fc 4
use console;
struct P {
	held:u8;
	pressed:u8;
	released:u8;
	timer:u8;
}
var g:P;
var h:P;
function poke():void @(noinline)
{
	g.held = 40;
}
// pad.update の形: 2 回目の p.held は 1 回目の値
function update(p:*P, now:u8):void @(noinline)
{
	var pressed = now & ~p.held;
	p.pressed = pressed;
	p.released = p.held & ~now;
	p.held = now;
	p.timer = 16;
	if (p.timer == 0) {
		p.timer = 1;
	}
}
// 同じ所を指す別のポインタ
function alias(p:*P, q:*P):u8 @(noinline)
{
	var a = p.held;
	q.held = a + 1;
	return p.held;
}
// グローバル変数への書き込み・呼び出し
function global(p:*P):u8 @(noinline)
{
	var a = p.held;
	g.held = a + 2;
	var b = p.held;
	poke();
	return b + p.held;
}
// ポインタの付け替え
function swap(p:*P):u8 @(noinline)
{
	var a = p.held;
	p = &h;
	return a + p.held;
}
// アドレスを取った局所変数
function local():u8 @(noinline)
{
	var s:P;
	s.held = 3;
	var p = &s;
	var a = p.held;
	s.held = 9;
	return a + p.held;
}
// 2 つのフィールドを 2 回ずつ読む: 2 回目の p.held は置き換えた後の命令の disp (0) で覚えてはいけない (seed 60443000)
function twice(p:*P):u8 @(noinline)
{
	var a = p.pressed + p.held;
	return a + (p.pressed - p.held);
}
function main():void
{
	g.held = 5;
	h.held = 100;
	update(&g, 6);
	@printf("{} {} {} {}\n", g.held, g.pressed, g.released, g.timer);
	g.held = 7;
	var x = alias(&g, &g);
	g.held = 10;
	var y = global(&g);
	g.held = 1;
	var z = swap(&g);
	@printf("{} {} {} {}\n", x, y, z, local());
	g.held = 3;
	g.pressed = 20;
	@printf("{}\n", twice(&g));
	console.exit(0);
}
`)
	// update: pressed = 6 & ~5 = 2、released = 5 & ~6 = 1。alias: 8。global: b = 12 (g.held を書いた後に読み直す)、poke の後の 40 で 52。
	// swap: 1 + 100。local: 3 + 9。twice: 23 + 17
	if out != "6 2 1 16\n8 52 101 12\n40\n" {
		t.Errorf("got %q", out)
	}
	body := strings.Join(procBody(t, asm, "_t_update"), "\n")
	if n := strings.Count(body, "ldy #0\n\tlda (") + strings.Count(body, "ldy #0\nlda ("); n > 1 {
		t.Errorf("update: p.held を %d 回読んでいる:\n%s", n, body)
	}
	if strings.Contains(body, "lda #1\n") {
		t.Errorf("update: p.timer = 16 の後の p.timer == 0 を畳んでいない:\n%s", body)
	}
}
