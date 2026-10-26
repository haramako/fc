package nes

// TestNesOamMetaRef: fclib/nes/oam.asm の spr / meta / meta_pal を、前の fc で書いた版 (下の ref.fc) と乱数の入力で比べる
// (戻り値・置いた枚数・OAM の写しの中身)。ずらしは i8 の全部 (反転で -dx - 8 が i8 に入らない 127 も)、座標は画面の外も、
// 8×8 / 8×16 のモード、満杯の近く。

import (
	"testing"
)

const oamRefSrc = `#fc 4
use nes;
use frame;

public var buf:[256]u8;
public var n:u8;

public function spr(x:u8, y:u8, tile:u8, attr:u8):bool
{
	if (n >= 64) {
		return false;
	}
	var i = n << 2;
	buf[i] = y;
	buf[i + 1] = tile;
	buf[i + 2] = attr;
	buf[i + 3] = x;
	n += 1;
	return true;
}

public function place(x:i16, y:i16, m:[]const u8, flip:u8, keep:u8, add:u8):bool
{
	var h:i16 = 8;
	if ((frame.ctrl & nes.CTRL_SPR_8X16) != 0) {
		h = 16;
	}
	var ok = true;
	for (var k:u8 = 0; k + 3 < @len(m); k += 4) {
		var dx = @bitcast(i8, m[k]) as i16;
		var dy = @bitcast(i8, m[k + 1]) as i16;
		if ((flip & nes.ATTR_FLIP_H) != 0) {
			dx = -dx - 8;
		}
		if ((flip & nes.ATTR_FLIP_V) != 0) {
			dy = -dy - h;
		}
		var sx = x + dx;
		var sy = y + dy;
		if (sx < 0 || sx > 255 || sy < 0 || sy > 239) {
			continue;
		}
		ok = spr(sx as u8, sy as u8, m[k + 2], ((m[k + 3] & keep) | add) ^ flip) && ok;
	}
	return ok;
}
`

const oamTestSrc = `#fc 4
use nes;
use frame;
use oam;
use ref;

public var cases:u16;
public var bad:u16;
public var done:bool;
var seed:u16 = 12345;
var m:[24]u8;

function rnd():u8
{
	seed = seed * 25173 + 13849;
	return (seed >> 8) as u8;
}

function rnd16(span:u16):i16
{
	var v = ((rnd() as u16) << 8) | rnd();
	return (v % span) as i16 - (span / 2) as i16;
}

function main():void
{
	frame.init();
	for (var c:u16 = 0; c < 3000; c += 1) {
		// 置く前の枚数 (満杯の近くも)
		var pre = rnd() & 63;
		if ((rnd() & 3) == 0) {
			pre = 60 + (rnd() & 3);
		}
		oam.begin();
		ref.n = 0;
		for (var i:u8 = 0; i < pre; i += 1) {
			oam.spr(i, i, i, i);
			ref.spr(i, i, i, i);
		}
		var len = (rnd() % 7) << 2;
		if ((rnd() & 7) == 0) {
			len += rnd() & 3;   // 4 の倍数でない長さ (残りは使わない)
		}
		for (var i:u8 = 0; i < len; i += 1) {
			m[i] = rnd();
		}
		frame.ctrl = (rnd() & 1) << 5;   // 8×8 / 8×16
		var x = rnd16(700);
		var y = rnd16(700);
		if ((rnd() & 1) == 0) {
			x = (rnd() as i16) - 16;      // 画面の近く
			y = (rnd() as i16) - 16;
		}
		var flip = rnd() & 0xc0;
		var a:bool;
		var b:bool;
		if ((rnd() & 1) == 0) {
			a = oam.meta(x, y, m[..len], flip);
			b = ref.place(x, y, m[..len], flip, 0xff, 0);
		} else {
			var pal = rnd();
			a = oam.meta_pal(x, y, m[..len], flip, pal);
			b = ref.place(x, y, m[..len], flip, 0xfc, pal & 3);
		}
		var same = a == b && oam.count() == ref.n;
		for (var j:u16 = 0; j < (ref.n as u16) << 2; j += 1) {
			if (oam.buf[j] != ref.buf[j]) {
				same = false;
			}
		}
		if (!same) {
			bad += 1;
		}
		cases += 1;
	}
	done = true;
	while (true) {
	}
}
`

func TestNesOamMetaRef(t *testing.T) {
	t.Parallel()
	p := buildNes(t, map[string]string{"t.fc": oamTestSrc, "ref.fc": oamRefSrc})
	p.play(t, 8000)
	cases, bad := p.peek16(t, "_t_cases"), p.peek16(t, "_t_bad")
	t.Logf("cases %d, 違い %d", cases, bad)
	if p.peek(t, "_t_done", 0) == 0 {
		t.Fatalf("終わっていない (cases %d)", cases)
	}
	if bad != 0 {
		t.Errorf("asm の oam と fc の版の結果が %d / %d 件で違う", bad, cases)
	}
}
