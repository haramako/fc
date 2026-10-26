package nes

// TestNesPalSet: pal.set は 1 色だけ、pal.set_range は続いた色を 1 つの項目でキューに積み (GitHub の Issue #13)、どちらも明るさを掛けて
// PPU のパレットに届く。16 / 20 / 24 / 28 番 (0 / 4 / 8 / 12 番の写し) に入れても背景の色は変わらない。32 番を超える分は捨てる。

import "testing"

const palTestSrc = `#fc 4
use frame;
use pal;

public var qlen:u8;
public var got16:u8;
public var got31:u8;
public var done:bool;
var base:[32]u8;

function main():void
{
	frame.init();
	for (var i:u8 = 0; i < 32; i += 1) {
		base[i] = (i * 5 + 0x11) & 0x3f;
	}
	pal.set_all(base);
	pal.bright(3);
	frame.render_on();
	frame.wait();
	pal.set(5, 0x21);
	pal.set_range(9, [0x16, 0x27, 0x38]);
	pal.set(16, 0x2a);
	pal.set_range(30, [0x11, 0x12, 0x13, 0x14]);
	qlen = frame.queue_len;
	got16 = pal.get(16);
	got31 = pal.get(31);
	frame.wait();
	done = true;
	while (true) {
		frame.wait();
	}
}
`

func TestNesPalSet(t *testing.T) {
	t.Parallel()
	p := buildNes(t, map[string]string{"t.fc": palTestSrc})
	p.play(t, 20)
	if p.peek(t, "_t_done", 0) == 0 {
		t.Fatal("終わっていない")
	}
	colors := make([]byte, 32)
	for i := range colors {
		colors[i] = byte((i*5 + 0x11) & 0x3f)
	}
	colors[5] = 0x21
	copy(colors[9:], []byte{0x16, 0x27, 0x38})
	colors[16] = 0x2a
	copy(colors[30:], []byte{0x11, 0x12})
	dark := func(c byte) byte { // 明るさ 3 (pal.shade の d = -1)
		if c&0x0f >= 0x0e || c>>4 < 1 {
			return 0x0f
		}
		if c -= 0x10; c == 0x0d {
			return 0x0f
		}
		return c
	}
	for i := 0; i < 32; i++ {
		if i&0x13 == 0x10 {
			continue // 0 / 4 / 8 / 12 番の写し
		}
		if want := dark(colors[i]); p.palette[i] != want {
			t.Errorf("パレットの %d 番が %#02x (%#02x のはず)", i, p.palette[i], want)
		}
	}
	if want := dark(colors[0]); p.palette[0] != want {
		t.Errorf("背景の色が %#02x (%#02x のはず。16 番に入れても変わらない)", p.palette[0], want)
	}
	// 1 色の項目 2 つ (頭 3 + 1) と、3 色・2 色の項目
	if got, want := p.peek(t, "_t_qlen", 0), 2*4+(3+3)+(3+2); got != want {
		t.Errorf("キューの長さ %d (%d のはず)", got, want)
	}
	if p.peek(t, "_t_got16", 0) != 0x2a || p.peek(t, "_t_got31", 0) != 0x12 {
		t.Errorf("pal.get: 16 番 %#02x、31 番 %#02x", p.peek(t, "_t_got16", 0), p.peek(t, "_t_got31", 0))
	}
}
