package driver

// fc 4 の fclib の mapper のモジュール (fclib/nes/uxrom.fc / mmc1.fc / mmc3.fc。Agent/wiki/plans/v4-stdlib.md §3.2)。

import "testing"

// mapperBank は bank の切替の領域に置く、表を引く関数のモジュール。
func mapperBank(bank, vals string) string {
	return "#fc 4\n@(bank: \"" + bank + "\");\nconst T:[4]u8 = [" + vals + "];\npublic function get(i:u8):u8 @(noinline) { return T[i]; }\n"
}

// TestMapperModules16K: uxrom / mmc1 のモジュールのトランポリンで far call し (戻ったらバンクが元に戻る)、prg は切り替えて
// 前のバンクを返す。
func TestMapperModules16K(t *testing.T) {
	t.Parallel()
	for _, m := range []struct{ mapper, mod string }{{"UxROM", "uxrom"}, {"MMC1", "mmc1"}} {
		files := map[string]string{
			"fc.toml": "[target]\nmapper = \"" + m.mapper + "\"\nprg = \"64K\"\n[bank.a]\nslot = 0x8000\n[bank.b]\nslot = 0x8000\n",
			"main.fc": `#fc 4
@(farcall);
use ` + m.mod + `;
use ma;
use mb;
public var out:[32]u8;
public var done:u8;
function main():void
{
	` + m.mod + `.init();
	out[0] = ` + m.mod + `.prg(@bank("a"));
	out[1] = ma.get(1);
	out[2] = mb.get(2);
	out[3] = ` + m.mod + `.bank;
	out[4] = ` + m.mod + `.prg(@bank("b"));
	out[5] = mb.get(3);
	out[6] = ` + m.mod + `.bank;
	done = 1;
	while (true) {
	}
}
`,
			"ma.fc": mapperBank("a", "10, 20, 30, 40"),
			"mb.fc": mapperBank("b", "50, 60, 70, 80"),
		}
		for _, level := range []int{-1, 0} {
			out, done, _ := runNes(t, files, level, 7)
			// a = 0、b = 1
			if done != 1 || fmtInts(out) != "[0 20 70 0 0 80 1]" {
				t.Errorf("%s -O %d: out=%v done=%d", m.mapper, level, out, done)
			}
		}
	}
}

// TestMapperMMC3: mmc3 のトランポリンで 2 つのスロットの far call (BANK_SELECT の写し mmc3.select も合わせる)、prg / chr が前の
// バンクを返し、走査線の IRQ が呼び出し口を呼ぶ。
func TestMapperMMC3(t *testing.T) {
	t.Parallel()
	files := map[string]string{
		"fc.toml": "[target]\nmapper = \"MMC3\"\nprg = \"64K\"\nchr = \"8K\"\n[bank.a]\nslot = 0x8000\n[bank.b]\nslot = 0x8000\n[bank.c]\nslot = 0xA000\nindex = 4\n",
		"main.fc": `#fc 4
@(farcall);
use nes;
use mmc3;
use ma;
use mb;
use mc;
public var out:[32]u8;
public var done:u8;
var hits:u8 @(volatile);
function on_irq():void @(interrupt)
{
	hits += 1;
}
function main():void
{
	mmc3.init();
	mmc3.prg(0, @bank("a"));
	mmc3.prg(1, @bank("c"));
	out[0] = ma.get(1);
	out[1] = mb.get(1);        // スロット 0 を b にして戻す
	out[2] = mc.get(1);
	out[3] = mmc3.prg_banks[0];
	out[4] = mmc3.prg(0, @bank("b"));
	out[5] = mb.get(2);
	out[6] = mmc3.chr(2, 9);
	out[7] = mmc3.chr(2, 4);
	out[8] = mmc3.select;
	mmc3.irq_hook = on_irq;
	nes.PPUMASK = nes.MASK_BG;   // (ランナーは描画中だけ走査線を数える)
	mmc3.irq(100);
	var seen:u8 = 0;
	while (hits < 3) {
		if (hits != seen) {
			seen = hits;
			mmc3.irq(100);   // 受け取ると止まるので、また仕掛ける
		}
	}
	out[9] = hits;
	done = 1;
	while (true) {
	}
}
`,
		"ma.fc": mapperBank("a", "10, 20, 30, 40"),
		"mb.fc": mapperBank("b", "50, 60, 70, 80"),
		"mc.fc": mapperBank("c", "90, 100, 110, 120"),
	}
	for _, level := range []int{-1, 0} {
		out, done, _ := runNes(t, files, level, 10)
		// prg は前のバンク (a = 0)、chr(2, …) は前の値 (init の 4、次に 9)、select は最後の chr の R2
		if done != 1 || fmtInts(out[:10]) != "[20 60 100 0 0 70 4 9 2 3]" {
			t.Errorf("-O %d: out=%v done=%d", level, out[:10], done)
		}
	}
}
