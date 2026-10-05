package driver

import (
	"strings"
	"testing"
)

// TestFusedIndexLoad: 添字付きの読み出しを直後の演算の第 2 入力に融合する (regalloc.MarkFusedLoads / FusedLoad、
// codegen.genFusedLoad)。A に常駐する変数との演算 (`c ^= p[k]` が `eor (p),y`)、添字が X / Y に常駐、
// ゼロページに無いポインタ (グローバル)、stack 関数 (再帰)、グローバルの配列、交換できない sub。-O 0 と結果を比べる。
func TestFusedIndexLoad(t *testing.T) {
	t.Parallel()
	src := `#fc 4
use console;
var buf:[64]u8;
var gp:*u8;
function crc8(p:*u8, n:u8):u8
{
	var crc:u8 = 0;
	for (var k:u8 = 0; k < n; k++) {
		crc ^= p[k];
		for (var j:u8 = 8; j != 0; j--) {
			if ((crc & 0x80) != 0) { crc = (crc << 1) ^ 0x1d; } else { crc = crc << 1; }
		}
	}
	return crc;
}
function mix(n:u8):u8
{
	var a:u8 = 1;
	var s:u8 = 7;
	for (var i:u8 = 0; i < n; i++) {
		a += gp[i];
		s -= buf[i];
		a |= buf[n - i] & 3;
		s = s - gp[i + 1];
	}
	return a ^ s;
}
function xs(p:*u8, n:u8):u8
{
	var c:u8 = 0;
	for (var k:u8 = 0; k < n; k++) { c ^= p[k]; c = c << 1; }
	return c;
}
function rec(p:*u8, n:u8):u8
{
	if (n == 0) { return 0; }
	var x:u8 = 0;
	for (var i:u8 = 0; i < n; i++) { x += p[i]; x ^= p[i + 1]; }
	return x + rec(p, n - 1);
}
function main():void
{
	for (var i:u8 = 0; i < 64; i++) { buf[i] = i * 37 + 11; }
	gp = &buf[3];
	@printf("{} {} {} {}\n", crc8(&buf[0], 64), mix(40), rec(&buf[1], 10), xs(&buf[2], 50));
	console.exit(0);
}
`
	out, err := buildBothLevels(t, map[string]string{"t.fc": src})
	if want := "163 31 249 240\n"; err != nil || out != want { // Python で同じ計算をした値
		t.Fatalf("got %q, %v, want %q", out, err, want)
	}
	asm := compileAsmFiles(t, map[string]string{"t.fc": src})
	body := asm[strings.Index(asm, ".proc _t_xs\n"):]
	body = body[:strings.Index(body, ".endproc")]
	if !strings.Contains(body, "eor (") {
		t.Errorf("A に常駐する c の `c ^= p[k]` が eor (p),y にならない:\n%s", body)
	}
}
