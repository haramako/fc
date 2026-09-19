package driver

// far call (doc/v2_farcall.md): 別バンクのモジュールの関数への呼び出しを farcall トランポリン経由にする。
// emu の farcall はそのまま飛ぶだけなので、結果が正しいこと・生成コードが farcall を経由すること・
// near になるべき呼び出しが直接 jsr のままであることを見る。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const farMain = `#fc 2
options(farcall: true);
use * from stdio;
use far1;
use fixed1;
public function fixed_add(a:int, b:int):int { return a + b; }
function main():void
{
	var s = far1.add(1, 2);
	var t = far1.fadd(3, 4);
	var u = far1.nearf(5);
	var v = far1.nested(10);
	var w = fixed1.twice(21);
	printf(s, " ", t, " ", u, " ", v, " ", w, " ", far1.add(far1.fadd(1, 1), 1), "\n");
	exit(0);
}
`

const far1 = `#fc 2
options(bank: 1);
use fixed1;
use main;
public function add(a:int, b:int):int { return a + b; }
public function fadd(a:int, b:int):int options(fastcall: true) { return a + b; }
public function nearf(a:int):int options(near: true) { return a * 2; }
public function nested(a:int):int
{
	// 同じモジュール内 (add) と固定バンク (fixed1.twice, main.fixed_add) は near、
	// far1 → main.fixed_add → (fixed から far1.add へ far) の入れ子は main 側で far
	return add(a, 1) + fixed1.twice(a) + main.fixed_add(a, 2);
}
`

const fixed1 = `#fc 2
public function twice(a:int):int { return a * 2; }
`

func TestFarCall(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"main.fc": farMain, "far1.fc": far1, "fixed1.fc": fixed1})
	var out strings.Builder
	c := NewCompiler(absRepoRoot)
	res, err := c.BuildContext(t.Context(), "main.fc", &BuildOptions{Dir: dir, BuildDir: filepath.Join(dir, "b"), Out: filepath.Join(dir, "a.bin"), Run: true, Stdout: &out})
	if err != nil {
		t.Fatalf("ビルド失敗: %v", err)
	}
	// nested(10) = 11 + 20 + 12 = 43
	if out.String() != "3 7 10 43 42 3\n" {
		t.Errorf("out=%q", out.String())
	}

	mainAsm, _ := os.ReadFile(filepath.Join(dir, "b", "_main.s"))
	far1Asm, _ := os.ReadFile(filepath.Join(dir, "b", "_far1.s"))
	m, f := string(mainAsm), string(far1Asm)
	// main → far1.add / far1.nested / far1.fadd: jsr farcall (main は静的フレームなので X を進める call マクロは使わない)、
	// far1.nearf: 直接、fixed1.twice: 直接
	if strings.Count(m, "call farcall") != 0 || strings.Count(m, "jsr farcall") != 5 {
		t.Errorf("main.s: call farcall=%d jsr farcall=%d\n%s", strings.Count(m, "call farcall"), strings.Count(m, "jsr farcall"), m)
	}
	if !strings.Contains(m, "jsr _far1_nearf") || !strings.Contains(m, "jsr _fixed1_twice") {
		t.Errorf("main.s: nearf / twice は直接呼ぶべき")
	}
	// far call の飛び先は入口の sta を飛ばす `_far1_add__frame` (レジスタ渡し。doc/v2_frame_alloc.md §7)
	if !strings.Contains(m, "lda #<.bank(_far1_add__frame)") || !strings.Contains(m, "sta FC_FARCALL+2") || !strings.Contains(m, ".global farcall") {
		t.Errorf("main.s: FC_FARCALL の設定が無い")
	}
	// far1 の中: 同じモジュール・固定バンクへの呼び出しは near
	if strings.Contains(f, "farcall") && strings.Count(f, "farcall") > 1 { // .import farcall の 1 行だけ
		t.Errorf("far1.s: far call があってはいけない\n%s", f)
	}
	// 一覧
	var names []string
	for _, fc := range res.FarCalls {
		names = append(names, fc.Caller+"->"+fc.Callee)
	}
	if got := strings.Join(names, " "); got != "_main->_far1_add _main->_far1_fadd _main->_far1_nested _main->_far1_fadd _main->_far1_add" {
		t.Errorf("FarCalls: %s", got)
	}
}

func TestFarCallDisabled(t *testing.T) {
	t.Parallel()
	// options(farcall: true) が無ければ何も変わらない (farcall の参照も出ない)
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"main.fc":   strings.Replace(farMain, "options(farcall: true);\n", "", 1),
		"far1.fc":   far1,
		"fixed1.fc": fixed1,
	})
	var out strings.Builder
	if _, err := NewCompiler(absRepoRoot).Build("main.fc", &BuildOptions{Dir: dir, BuildDir: filepath.Join(dir, "b"), Out: filepath.Join(dir, "a.bin"), Run: true, Stdout: &out}); err != nil {
		t.Fatal(err)
	}
	mainAsm, _ := os.ReadFile(filepath.Join(dir, "b", "_main.s"))
	if strings.Contains(string(mainAsm), "farcall") || strings.Contains(string(mainAsm), "FC_FARCALL") {
		t.Errorf("farcall が使われている")
	}
	if out.String() != "3 7 10 43 42 3\n" {
		t.Errorf("out=%q", out.String())
	}
}

func TestFarCallNesConfig(t *testing.T) {
	t.Parallel()
	// nes ターゲットで fc が生成する ld65.cfg には bank = N が付く
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"main.fc": "#fc 2\noptions(bank_count: 4, farcall: true);\nuse * from stdio;\nuse far1;\nfunction main():void { far1.add(1, 2); }\n",
		"far1.fc": "#fc 2\noptions(bank: 1);\npublic function add(a:int, b:int):int { return a + b; }\n",
	})
	if _, err := NewCompiler(absRepoRoot).Build("main.fc", &BuildOptions{Target: "nes", Dir: dir, BuildDir: filepath.Join(dir, "b"), Out: filepath.Join(dir, "a.nes")}); err != nil {
		t.Fatal(err)
	}
	cfg, _ := os.ReadFile(filepath.Join(dir, "b", "ld65.cfg"))
	for _, want := range []string{"ROM0: start = $8000, size = $2000, file = %O, fill = yes, define = yes, bank = 0;", "bank = 1;", "bank = 3;"} {
		if !strings.Contains(string(cfg), want) {
			t.Errorf("ld65.cfg に %q が無い:\n%s", want, cfg)
		}
	}
}

// TestFarcallMMC3Assembles: 参考実装 fclib/nes/farcall_mmc3.asm がアセンブル・リンクできる (中身は castle で確認する)。
func TestFarcallMMC3Assembles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"stub.s": "\t.export FC_FARCALL, _mmc3_pbank_bak\n.segment \"BSS\"\nFC_FARCALL: .res 3\n_mmc3_pbank_bak: .res 2\n.segment \"CODE\"\n\t.import farcall\n\tjsr farcall\n",
		"l.cfg":  "MEMORY { RAM: start = $0200, size = $100, type = rw; ROM: start = $8000, size = $2000, file = %O, fill = yes; }\nSEGMENTS { BSS: load = RAM, type = bss; CODE: load = ROM, type = ro; }\n",
	})
	runTool(t, dir, "ca65", filepath.Join(absRepoRoot, "fclib", "nes", "farcall_mmc3.asm"), "-o", "farcall.o")
	runTool(t, dir, "ca65", "stub.s", "-o", "stub.o")
	runTool(t, dir, "ld65", "-C", "l.cfg", "-o", "out.bin", "farcall.o", "stub.o")
}

// TestFarCallSegmentOption: options(segment: X) で別モジュールのセグメントに置いた関数は、そのモジュールの置き場所として判定する。
func TestFarCallSegmentOption(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"main.fc": "#fc 2\noptions(farcall: true);\nuse * from stdio;\nuse far1;\nuse far2;\n" +
			"function main():void { printf(far1.in_main(1), \" \", far1.in_far2(2), \" \", far2.g(3), \"\n\"); exit(0); }\n",
		"far1.fc": "#fc 2\noptions(bank: 1);\nuse far2;\n" +
			"public function in_main(a:int):int options(segment: main) { return a + far2.g(a); }\n" + // main (固定) に置く: near で呼ばれ、far2.g への呼び出しは far
			"public function in_far2(a:int):int options(segment: far2) { return far2.g(a) + 1; }\n", // far2 に置く: far で呼ばれ、far2.g は near
		"far2.fc": "#fc 2\noptions(bank: 2);\npublic function g(a:int):int { return a * 10; }\n",
	})
	var out strings.Builder
	res, err := NewCompiler(absRepoRoot).BuildContext(t.Context(), "main.fc", &BuildOptions{Dir: dir, BuildDir: filepath.Join(dir, "b"), Out: filepath.Join(dir, "a.bin"), Run: true, Stdout: &out})
	if err != nil {
		t.Fatal(err)
	}
	if out.String() != "11 21 30\n" {
		t.Errorf("out=%q", out.String())
	}
	var names []string
	for _, fc := range res.FarCalls {
		names = append(names, fc.Caller+"->"+fc.Callee)
	}
	// main → in_main: near、main → in_far2: far、main → far2.g: far、in_main → far2.g: far、in_far2 → far2.g: near
	if got := strings.Join(names, " "); got != "_main->_far1_in_far2 _main->_far2_g _far1_in_main->_far2_g" {
		t.Errorf("FarCalls: %s", got)
	}
}

// writeFiles は複数のファイルを dir に書く。
func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, src := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o777); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(src), 0o666); err != nil {
			t.Fatal(err)
		}
	}
}
