package nes

// TestNesFarCallSameBank: MMC3 で、同じ切替バンクの別のモジュールへの呼び出し (near になる) と、同じスロットの別のバンクへの
// far call (切り替えて戻す) を混ぜて走らせる。near にした呼び出しの後も、呼ぶ側のバンクの表を正しく読む。

import "testing"

func TestNesFarCallSameBank(t *testing.T) {
	t.Parallel()
	p := buildNes(t, map[string]string{
		"fc.toml": "[target]\nmapper = \"MMC3\"\nprg = \"64K\"\n\n[bank.code]\nslot = 0x8000\n\n[bank.other]\nslot = 0x8000\n",
		"t.fc": `#fc 4
@(farcall);
use nes;
use frame;
use mmc3;
use codea;

public var result:u16;
public var done:bool;

function main():void
{
	mmc3.init();
	frame.init();
	for (var i:u8 = 0; i < 8; i += 1) {
		result += codea.f(i);
	}
	done = true;
	while (true) {
		frame.wait();
	}
}
`,
		"codea.fc": `#fc 4
@(bank: "code");
use codeb;
use other;
const TA = [3, 1, 4, 1, 5, 9, 2, 6];
public function f(i:u8):u16 @(noinline)
{
	var a = codeb.g(i);       // 同じバンク: near
	var b = other.h(i);       // 同じスロットの別のバンク: far (戻ると code に戻す)
	return (a + TA[i] + b) as u16;
}
`,
		"codeb.fc": `#fc 4
@(bank: "code");
const TB = [10, 20, 30, 40, 50, 60, 70, 80];
public function g(i:u8):u8 @(noinline) { return TB[i]; }
`,
		"other.fc": `#fc 4
@(bank: "other");
const TO = [100, 101, 102, 103, 104, 105, 106, 107];
public function h(i:u8):u8 @(noinline) { return TO[i]; }
`,
	})
	p.play(t, 10)
	if p.peek(t, "_t_done", 0) == 0 {
		t.Fatal("終わっていない")
	}
	want := 0
	ta := []int{3, 1, 4, 1, 5, 9, 2, 6}
	for i := 0; i < 8; i++ {
		want += (i+1)*10 + ta[i] + 100 + i
	}
	if got := p.peek16(t, "_t_result"); got != want {
		t.Errorf("result %d (%d のはず)", got, want)
	}
}
