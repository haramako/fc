package driver

// バンクをまたぐインライン化と far call の実機に近いテスト: nes ターゲット (MMC3) で、同じ CPU アドレス ($8000〜) に
// 違う内容の const 表を持つ 2 つの切替バンクのモジュールを置き、固定バンクの main から呼んで内蔵 NES ランナーで走らせる。
// emu ターゲットにはバンクが無いので、そちらの fuzz では「表は元のバンクに残ってコードだけ移る」バグは見えない。

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haramako/fc/internal/nes"
)

// bankMainHeader は MMC3 の main モジュールの頭: 参考実装のトランポリンを include し、PRG バンク 0 / 1 を $8000 / $A000 に
// マップして pbank_bak を合わせる (fc の生成する ld65.cfg はバンク i を $8000 + (i % 4) * $2000 に置くので、
// bank_count 8 ならバンク 0 と 4 が同じ $8000 に来る)。
const bankMainHeader = `#fc 2
options(mapper: "MMC3", bank_count: 8, farcall: true);
options(bank: -1);
use nes;
include("farcall_mmc3.asm");
var pbank_bak:[2]int options(symbol: "_mmc3_pbank_bak");
var BANK_SELECT:int options(address: 0x8000);
var BANK_DATA:int options(address: 0x8001);
public var out:[32]int;
public var done:int;
function nmi():void options(symbol: "_interrupt") { }
function irq():void options(symbol: "_interrupt_irq") { }
function bank_init():void
{
	BANK_SELECT = 6;
	BANK_DATA = 0;
	BANK_SELECT = 7;
	BANK_DATA = 1;
	pbank_bak[0] = 0;
	pbank_bak[1] = 1;
}
`

// runNes は files をビルド (nes、level) して内蔵 NES ランナーで frames フレーム走らせ、`_main_out` の n バイトと
// `_main_done` を返す。asm には _main.s の中身を返す。
func runNes(t *testing.T, files map[string]string, level int, n int) (out []int, done int, asm string) {
	t.Helper()
	dir := t.TempDir()
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o666); err != nil {
			t.Fatal(err)
		}
	}
	rom := filepath.Join(dir, "a.nes")
	res, err := NewCompiler(absRepoRoot).BuildContext(context.Background(), "main.fc", &BuildOptions{Target: "nes", Dir: dir, BuildDir: filepath.Join(dir, "b"), Out: rom, OptimizeLevel: level})
	if err != nil {
		t.Fatalf("ビルド失敗 (-O %d): %v", level, err)
	}
	d, err := ParseDbgFile(res.DbgFile)
	if err != nil {
		t.Fatal(err)
	}
	syms := map[string]int{}
	for _, s := range d.Symbols {
		if _, ok := syms[s.Name]; !ok {
			syms[s.Name] = s.Val
		}
	}
	outAddr, ok := syms["_main_out"]
	if !ok {
		t.Fatalf("_main_out がシンボルに無い")
	}
	doneAddr := syms["_main_done"]
	data, err := os.ReadFile(rom)
	if err != nil {
		t.Fatal(err)
	}
	m, err := nes.New(data)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.RunFrames(6); err != nil {
		t.Fatalf("実行失敗 (-O %d): %v", level, err)
	}
	for i := 0; i < n; i++ {
		out = append(out, m.Get(outAddr+i))
	}
	b, _ := os.ReadFile(filepath.Join(dir, "b", "_main.s"))
	return out, m.Get(doneAddr), string(b)
}

// TestInlineAcrossBanks: 別バンクの const 表を読む小関数 (明示 inline / 自動 inline の両方) は呼び出し側に写さず far call の
// まま (写すとコードだけ移って表は元のバンクに残り、別の表を読む)。private な BSS を読み書きするだけの小関数は写す。
// asm は sei / cli のようなフラグだけの命令なら写し (castle の mmc3.set_pbank)、それ以外 (pha など) なら far call のまま。
func TestInlineAcrossBanks(t *testing.T) {
	t.Parallel()
	files := map[string]string{
		"main.fc": bankMainHeader + `use bank0;
use bank4;
function main():void
{
	bank_init();
	out[0] = bank0.get(2);
	out[1] = bank4.get(2);
	out[2] = bank0.getauto(1);
	out[3] = bank4.getauto(1);
	out[4] = bank0.text(1);
	bank0.bump();
	bank0.crit();
	bank0.opaque();
	bank4.bump();
	bank4.bump();
	out[5] = bank0.cnt;
	out[6] = bank4.cnt;
	out[7] = pbank_bak[0];
	out[8] = pbank_bak[1];
	done = 1;
	while (1) {
	}
}
`,
		"bank0.fc": `#fc 2
options(bank: 0);
const T:[4]int = [10, 20, 30, 40];
public var cnt:int;
public function get(i:int):int options(inline: true) { return T[i]; }
public function getauto(i:int):int { return T[i] ^ 1; }
public function text(i:int):int options(inline: true) { var s:*int = "ab"; return s[i]; }
public function bump():void options(inline: true) { cnt += 1; }
public function crit():void options(inline: true) { asm("sei"); cnt += 2; asm("cli"); }
public function opaque():void options(inline: true) { asm("nop"); asm("pha"); asm("pla"); cnt += 4; }
`,
		"bank4.fc": `#fc 2
options(bank: 4);
const T:[4]int = [50, 60, 70, 80];
public var cnt:int;
public function get(i:int):int options(inline: true) { return T[i]; }
public function getauto(i:int):int { return T[i] ^ 1; }
public function bump():void options(inline: true) { cnt += 1; }
`,
	}
	want := []int{30, 70, 21, 61, 'b', 7, 2, 0, 1}
	for _, level := range []int{-1, 0} {
		out, done, asm := runNes(t, files, level, len(want))
		if done != 1 {
			t.Errorf("-O %d: main が終わらない", level)
		}
		if fmt.Sprint(out) != fmt.Sprint(want) {
			t.Errorf("-O %d: got %v want %v", level, out, want)
		}
		if level == 0 {
			// 表を読む関数は far call のまま、BSS だけの bump は写されて呼び出しが無い
			for _, sym := range []string{"_bank0_get", "_bank4_get", "_bank0_getauto", "_bank0_text", "_bank0_opaque"} {
				if !strings.Contains(asm, ".bank("+sym) {
					t.Errorf("%s が far call になっていない", sym)
				}
			}
			if strings.Contains(asm, "_bank0_bump") || strings.Contains(asm, "_bank4_bump") {
				t.Errorf("bump (BSS だけ) が写されていない:\n%s", asm)
			}
			if strings.Contains(asm, "_bank0_crit") || !strings.Contains(asm, "sei") {
				t.Errorf("crit (sei / cli だけの asm) が写されていない:\n%s", asm)
			}
		}
	}
}

// TestRandomBankPrograms: 小さなバンク切替 fuzz。切替バンク 0 と 4 (同じ $8000) にランダムな表と、それを読む小関数
// (inline / 自動 inline 候補 / ループ入り) を置き、固定バンクの main から呼んだ結果を -O 0 / -O 2 の両方で走らせて
// 生成器が計算した期待値と比べる。呼び出しの後で pbank_bak (トランポリンの復帰) も見る。
func TestRandomBankPrograms(t *testing.T) {
	t.Parallel()
	n := 8
	if *randN > 30 {
		n = *randN / 4
	}
	for k := 0; k < n; k++ {
		seed := int64(k + 1)
		if *randSeed != 0 {
			seed = *randSeed + int64(k)
		}
		t.Run(fmt.Sprintf("seed%d", seed), func(t *testing.T) {
			t.Parallel()
			files, want := genBankProgram(rand.New(rand.NewSource(seed)))
			for _, level := range []int{-1, 0} {
				out, done, _ := runNes(t, files, level, len(want))
				if done != 1 {
					t.Fatalf("-O %d: main が終わらない\n%s", level, files["main.fc"])
				}
				if fmt.Sprint(out) != fmt.Sprint(want) {
					t.Errorf("-O %d: got %v\nwant %v\n%s\n// ---- bank0.fc ----\n%s\n// ---- bank4.fc ----\n%s", level, out, want, files["main.fc"], files["bank0.fc"], files["bank4.fc"])
				}
			}
		})
	}
}

// bankFunc は生成した表の関数 (1 バイト引数 1 つ、1 バイト戻り値) と、その Go での評価。
type bankFunc struct {
	name string
	decl string
	eval func(tab []int, i int) int
}

// genBankProgram は main + bank0 + bank4 を生成し、main の out[] に入るはずの値を返す。
func genBankProgram(r *rand.Rand) (map[string]string, []int) {
	kinds := []func(name string, k int) bankFunc{
		func(name string, k int) bankFunc { // 表の 1 要素
			return bankFunc{name, fmt.Sprintf("public function %s(i:int):int options(inline: true) { return T[(i & 7)] + %d; }", name, k),
				func(tab []int, i int) int { return (tab[i&7] + k) & 255 }}
		},
		func(name string, k int) bankFunc { // 自動インラインの候補 (印なし、小さい)
			return bankFunc{name, fmt.Sprintf("public function %s(i:int):int { return T[(i & 7)] ^ T[%d]; }", name, k&7),
				func(tab []int, i int) int { return tab[i&7] ^ tab[k&7] }}
		},
		func(name string, k int) bankFunc { // 2 要素の和
			return bankFunc{name, fmt.Sprintf("public function %s(i:int):int options(inline: true) { return T[(i & 7)] + T[((i + 1) & 7)]; }", name),
				func(tab []int, i int) int { return (tab[i&7] + tab[(i+1)&7]) & 255 }}
		},
		func(name string, k int) bankFunc { // ループで合計 (展開される)
			return bankFunc{name, fmt.Sprintf("public function %s(i:int):int { var s:int = i; for (var j:int = 0; j < %d; j++) { s += T[j]; } return s; }", name, 2+k%4),
				func(tab []int, i int) int {
					s := i
					for j := 0; j < 2+k%4; j++ {
						s += tab[j]
					}
					return s & 255
				}}
		},
		func(name string, k int) bankFunc { // BSS だけ (写される)
			return bankFunc{name, fmt.Sprintf("public function %s(i:int):int options(inline: true) { cnt += i; return cnt; }", name),
				nil}
		},
	}
	type mod struct {
		name  string
		bank  int
		tab   []int
		funcs []bankFunc
		cnt   int
	}
	mods := []*mod{{name: "bank0", bank: 0}, {name: "bank4", bank: 4}}
	files := map[string]string{}
	for _, m := range mods {
		for i := 0; i < 8; i++ {
			m.tab = append(m.tab, r.Intn(256))
		}
		var b strings.Builder
		fmt.Fprintf(&b, "#fc 2\noptions(bank: %d);\nconst T:[8]int = [", m.bank)
		for i, v := range m.tab {
			if i > 0 {
				b.WriteString(", ")
			}
			fmt.Fprint(&b, v)
		}
		b.WriteString("];\npublic var cnt:int;\n")
		nf := 1 + r.Intn(4)
		for i := 0; i < nf; i++ {
			f := kinds[r.Intn(len(kinds))](fmt.Sprintf("f%d", i), r.Intn(8))
			m.funcs = append(m.funcs, f)
			b.WriteString(f.decl + "\n")
		}
		files[m.name+".fc"] = b.String()
	}
	var main strings.Builder
	main.WriteString(bankMainHeader + "use bank0;\nuse bank4;\nfunction main():void\n{\n\tbank_init();\n")
	var want []int
	slot := 0
	nCalls := 4 + r.Intn(8)
	for c := 0; c < nCalls && slot < 30; c++ {
		m := mods[r.Intn(len(mods))]
		f := m.funcs[r.Intn(len(m.funcs))]
		arg := r.Intn(16)
		fmt.Fprintf(&main, "\tout[%d] = %s.%s(%d);\n", slot, m.name, f.name, arg)
		if f.eval != nil {
			want = append(want, f.eval(m.tab, arg))
		} else {
			m.cnt = (m.cnt + arg) & 255
			want = append(want, m.cnt)
		}
		slot++
		if r.Intn(3) == 0 {
			// 呼び出しの後でバンクが戻っているか (トランポリンの復帰)
			fmt.Fprintf(&main, "\tout[%d] = pbank_bak[0] + (pbank_bak[1] << 4);\n", slot)
			want = append(want, 0+1<<4)
			slot++
		}
	}
	main.WriteString("\tdone = 1;\n\twhile (1) {\n\t}\n}\n")
	files["main.fc"] = main.String()
	return files, want
}
