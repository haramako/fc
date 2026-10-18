package driver

import "testing"

// TestMinMaxWidth: @min / @max / @clamp は A1 の区切りではない (`a < b ? a : b` と同じ。上から来た幅で引数を計算して比べる)。
// 8 ビットで折り返していた (`var v:i16 = @max(a + a, 0)` が 144。Agent/issues/local-2026-10-18-minmax-a1-width.md)。
// 実行時の式と定数の式 (畳み込み) が同じ値になることも見る。
func TestMinMaxWidth(t *testing.T) {
	t.Parallel()
	out, _ := buildShape(t, `#fc 4
use console;
function id_u8(v:u8):u8 @(noinline) { return v; }
function id_i8(v:i8):i8 @(noinline) { return v; }
function id_u16(v:u16):u16 @(noinline) { return v; }
var a:u8 = 200;
var s:i8 = -100;
const CA:u8 = 200;
function main():void
{
	// 代入先の幅 (16 ビット) で計算する
	var v1:i16 = @max(a + a, 0);
	var v2:i16 = @min(a + a, 500);
	var v3:i16 = @clamp(a + a, 0, 500);
	var v4:i16 = @min(a + a, 150);
	var v5:i16 = @min(s + s, 0);
	var v6:u8 = @max(a + a, 0);
	@printf("{} {} {} {} {} {}\n", v1, v2, v3, v4, v5, v6);
	// 定数の式も同じ値
	const k1:i16 = @max(CA + CA, 0);
	const k2:i16 = @clamp(CA + CA, 0, 500);
	var k3:i16 = @min((200 as u8) + (200 as u8), 150) + 0;
	@printf("{} {} {}\n", k1, k2, k3);
	// 16 ビットの相手 (w) と足す式の中の @max: 引数の ~ も 16 ビットで
	var w:u16 = 1;
	var c1 = w + @max((13 << 3), (~(105 as u8)));
	var c2 = w + @max((13 << 3), (~id_u8(105)));
	@printf("{} {}\n", c1, c2);
	// 揃える型 (u16) の符号で比べる (広い幅で計算した i8 の引数 1 ^ 7 も u16 として)
	var m1 = (@max((65535 as u16), ((1 as u8) ^ (7 as i8)))) as i16;
	var m2 = (@max(id_u16(65535), (id_u8(1) ^ id_i8(7)))) as i16;
	@printf("{} {}\n", m1, m2);
	console.exit(0);
}
`)
	want := "400 400 400 150 -200 144\n400 400 150\n65431 65431\n-1 -1\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}
