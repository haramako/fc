package driver

// far call のレジスタ渡し (Agent/wiki/design/farcall.md §3.7): トランポリン farcall_ay は A / Y の引数をそのまま呼び先へ渡し、
// A の戻り値をそのまま返す。fclib の uxrom / mmc1 / mmc3 のトランポリンで、切り替えの要る呼び出し・要らない呼び出し・
// 同じスロットのバンクどうしの入れ子・stack の関数 (再帰) を呼ぶ・stack の関数から呼ぶ・ループの中で呼ぶ形を走らせる。

import (
	"strings"
	"testing"
)

// farRegMain は far call のレジスタ渡しを試す main (init は mapper ごとの初期化)。
func farRegMain(mod, init string) string {
	return `#fc 4
@(farcall);
use ` + mod + `;
use ma;
use mb;
public var out:[32]u8;
public var done:u8;

// 再帰する (stack の) 関数から far call (呼び出しの後で X = フレームの底を戻す)
function rec(n:u8):u8
{
	if (n == 0) {
		return 0;
	}
	return rec(n - 1) + mb.get(n & 3);
}

function main():void
{
	` + init + `
	out[0] = ma.add2(3, 4);        // A / Y の引数 (a は入っているので切り替え無し)
	out[1] = mb.add2(5, 6);        // 切り替えあり
	var w:u16 = ma.wide(200, 100); // 2 バイトの戻り値 (フレームから)
	out[2] = w as u8;
	out[3] = (w >> 8) as u8;
	out[4] = ma.nest(1);           // a → b (同じスロットの入れ子)
	out[5] = mb.rec(3);            // 再帰する (stack の) 呼び先: X = FC_SP で入る
	var s:u8 = 0;
	for (var i:u8 = 0; i < 4; i += 1) {
		s += ma.add2(i, s);
	}
	out[6] = s;
	out[7] = rec(3);
	out[8] = ma.add2(mb.get(2), ma.add2(1, 2));
`
}

const farRegA = `#fc 4
@(bank: "a");
use mb;
const T:[4]u8 = [10, 20, 30, 40];
public function add2(x:u8, y:u8):u8 @(noinline) { return x + y + T[0]; }
public function wide(x:u8, y:u8):u16 @(noinline) { return (x as u16) * 3 + y; }
public function nest(i:u8):u8 @(noinline) { return mb.get(i) + T[i]; }
`

const farRegB = `#fc 4
@(bank: "b");
const T:[4]u8 = [50, 60, 70, 80];
public function get(i:u8):u8 @(noinline) { return T[i]; }
public function add2(x:u8, y:u8):u8 @(noinline) { return x * 2 + y + T[1]; }
public function rec(n:u8):u8
{
	if (n == 0) {
		return T[0];
	}
	return rec(n - 1) + 1;
}
`

// farRegWant は out[0..8] の期待値: 17, 76, 700 (188, 2), 60 + 20, 50 + 3, ループ (10, 21, 43, 87 の和)、60 + 70 + 80、
// add2(70, 13) = 93。
const farRegWant = "17 76 188 2 80 53 161 210 93"

func TestFarCallRegisters16K(t *testing.T) {
	t.Parallel()
	for _, m := range []struct{ mapper, mod string }{{"UxROM", "uxrom"}, {"MMC1", "mmc1"}} {
		files := map[string]string{
			"fc.toml": "[target]\nmapper = \"" + m.mapper + "\"\nprg = \"64K\"\n[bank.a]\nslot = 0x8000\n[bank.b]\nslot = 0x8000\n",
			"main.fc": farRegMain(m.mod, m.mod+".init();\n\t"+m.mod+".prg(@bank(\"a\"));") +
				"\tout[9] = " + m.mod + ".bank;\n\tdone = 1;\n\twhile (true) {\n\t}\n}\n",
			"ma.fc": farRegA,
			"mb.fc": farRegB,
		}
		for _, level := range []int{-1, 0} {
			out, done, asm := runNes(t, files, level, 10)
			if got := strings.Trim(fmtInts(out), "[]"); done != 1 || got != farRegWant+" 0" {
				t.Errorf("%s -O %d: out=%s done=%d, want %s 0", m.mapper, level, got, done, farRegWant)
			}
			// A / Y の引数を置いたまま呼ぶ (呼び先の入口は A / Y で受け取る _ma_add2)
			if !strings.Contains(asm, "ldx #<.bank(_ma_add2)\n\tstx FC_FARCALL+2\n\tjsr farcall_ay") {
				t.Errorf("%s -O %d: far call がレジスタ渡しでない\n%s", m.mapper, level, asm)
			}
		}
	}
}

func TestFarCallRegistersMMC3(t *testing.T) {
	t.Parallel()
	files := map[string]string{
		"fc.toml": "[target]\nmapper = \"MMC3\"\nprg = \"64K\"\nchr = \"8K\"\n[bank.a]\nslot = 0x8000\n[bank.b]\nslot = 0x8000\n[bank.c]\nslot = 0xA000\nindex = 4\n[bank.d]\nslot = 0xA000\nindex = 5\n",
		"main.fc": strings.Replace(farRegMain("mmc3", "mmc3.init();\n\tmmc3.prg(0, @bank(\"a\"));\n\tmmc3.prg(1, @bank(\"c\"));"), "use mb;\n", "use mb;\nuse mc;\nuse md;\n", 1) +
			"\tout[9] = mc.sub2(100, 3);     // スロット 1 (切り替え無し)\n" +
			"\tout[10] = mc.cross(2);        // スロット 1 → スロット 0 の b (切り替えあり)\n" +
			"\tout[14] = md.add2(7, 8);      // スロット 1 を d に切り替えて戻す\n" +
			"\tout[11] = mmc3.prg_banks[0];\n\tout[12] = mmc3.prg_banks[1];\n\tout[13] = mmc3.select;\n" +
			"\tdone = 1;\n\twhile (true) {\n\t}\n}\n",
		"ma.fc": farRegA,
		"mb.fc": farRegB,
		"mc.fc": `#fc 4
@(bank: "c");
use mb;
const T:[4]u8 = [90, 100, 110, 120];
public function sub2(x:u8, y:u8):u8 @(noinline) { return x - y + T[0]; }
public function cross(i:u8):u8 @(noinline) { return mb.add2(i, T[i]); }
`,
		"md.fc": `#fc 4
@(bank: "d");
const T:[4]u8 = [1, 2, 3, 4];
public function add2(x:u8, y:u8):u8 @(noinline) { return x * y + T[3]; }
`,
	}
	for _, level := range []int{-1, 0} {
		out, done, _ := runNes(t, files, level, 15)
		// sub2(100, 3) = 187、cross(2) = mb.add2(2, 110) = 4 + 110 + 60 = 174、バンクは a (0) と c (4) に戻り、select は 6 か 7、
		// md.add2(7, 8) = 56 + 4
		got := strings.Trim(fmtInts(out), "[]")
		if want := farRegWant + " 187 174 0 4"; done != 1 || !strings.HasPrefix(got, want) || (out[13] != 6 && out[13] != 7) || out[14] != 60 {
			t.Errorf("-O %d: out=%s done=%d, want %s 6|7 60", level, got, done, want)
		}
	}
}

// TestFarCallInInterruptWarns: 割り込みから届く関数の far call は警告 (FC_FARCALL とバンクの写しは主の処理と共有。
// Agent/wiki/design/farcall.md §3.5)。割り込みが固定のバンクの関数だけを呼ぶなら警告しない。
func TestFarCallInInterruptWarns(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		nmi  string
		warn bool
	}{
		{"x = ma.get(1);", true},
		{"x = fixed(1);", false},
	} {
		r := testBuild(t, buildSpec{Target: "nes", Main: "main.fc", Files: map[string]string{
			"fc.toml": "[target]\nmapper = \"UxROM\"\nprg = \"64K\"\n[bank.a]\nslot = 0x8000\n",
			"main.fc": "#fc 4\n@(farcall);\nuse uxrom;\nuse ma;\npublic var x:u8;\n" +
				"function fixed(i:u8):u8 @(noinline) { return i + 1; }\n" +
				"function nmi():void @(interrupt, symbol: \"_interrupt\")\n{\n\t" + c.nmi + "\n}\n" +
				"function main():void\n{\n\tuxrom.init();\n\twhile (true) {\n\t\tx += ma.get(0);\n\t}\n}\n",
			"ma.fc": "#fc 4\n@(bank: \"a\");\nconst T:[4]u8 = [1, 2, 3, 4];\npublic function get(i:u8):u8 @(noinline) { return T[i]; }\n",
		}})
		if r.Err != nil {
			t.Fatal(r.Err)
		}
		found := false
		for _, w := range r.Res.Warnings {
			found = found || strings.Contains(w.Msg, "_interrupt makes a far call in an interrupt handler")
		}
		if found != c.warn {
			t.Errorf("%s: 警告 %v, want %v (%v)", c.nmi, found, c.warn, r.Res.Warnings)
		}
	}
}
