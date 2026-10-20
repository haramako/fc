package driver

import "testing"

// TestCompareWidenedConst: 8 ビットの式と型のない定数の比較は、変数と同じ規則 (相手の広げる前の型で決める)。A1 で式を
// 16 ビットに広げたあと、広げた型 (i16 / u16) で定数を合わせていて、`(x << 4) != -176` (x:u8) が「-176 does not fit in
// u16」、`(x + 0) == 37440` (x:i8) が「37440 does not fit in i16」になっていた (変数の `x != -176`・`x == 37440` は通る)。
// エラーにならなくしたあと、広げた型と u16 の定数を同じ大きさの符号付きで比べると 65535 を -1 と読むので、比べる型も
// 広げる前の型で決める (seed 61340040 / TestRandomConstFoldV4 の種 61400024 など)。
func TestCompareWidenedConst(t *testing.T) {
	t.Parallel()
	out, _ := buildShape(t, `#fc 4
use console;
var p:i8 = 127;
var n:i8 = -1;
var u:u8 = 137;
function b(c:bool):u8 { if (c) { return 1; } return 0; }
function main():void
{
	@printf("{} {} {} {}\n", b(u != -176), b((u << 4) != -176), b((u + 0) < 300), b((u << 4) == 2192));
	@printf("{} {} {} {}\n", b(p >= 65535), b((p ^ 0) >= 65535), b(p == 37440), b((p + 0) == 37440));
	@printf("{} {} {} {}\n", b(n >= 65535), b((n ^ 0) >= 65535), b(n > 40000), b((n + 0) > 40000));
	console.exit(0);
}
`)
	// 32768 以上の定数は u16 で比べる (TestConstFoldCases の `(65533 > ((-81 as i8) >> 9))` と同じ): -1 は 65535
	want := "1 1 1 1\n0 0 0 0\n1 1 1 1\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}
