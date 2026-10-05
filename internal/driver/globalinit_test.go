package driver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// globalInitSrc はグローバル変数の初期値 (fc 4) の色々な形: 整数・struct・長い配列 (0 を写さない並びと 255 バイトを超える記録)・
// 名前付きの const・文字列・グローバル変数と配列定数と関数のアドレス・ほかのモジュールの public の変数・ゼロページ・
// 0 で埋めない領域 (emu の BSS_EX)。
var globalInitSrc = map[string]string{
	"gm.fc": "#fc 4\npublic var counter:u16 = 1234;\npublic function get():u8 { return 42; }\n",
	"t.fc": `#fc 4
use console;
use gm;
struct P { x:u8; y:i16; }
const ROW:[3]u8 = [7, 8, 9];
var a:u8 = 5;
var b:i16 = -300;
var c = 200;
var s:P = {3, -4};
var arr:[300]u8 = [1, 2, 0, 0, 0, 0, 0, 9` + strings.Repeat(", 0", 291) + `, 7];
var big:[40]u16 = [0x1234, 0, 0, 0, 0x5678];
var name = "hello";
var p:*u8 = &arr[7];
var q:*const u8 = ROW;
var z:u8 = 0;
var f:fn():u8 = gm.get;
var ps:[2]P = [{1, 2}, {3, 4}];
var zp:u8 = 77 @(segment: "ZEROPAGE");
var ex:[4]u8 = [0, 3] @(segment: "BSS_EX");
var row = ROW;
function main():void
{
	@printf("{} {} {} {} {} {} {} {} {} {}\n", a, b, c, s.x, s.y, arr[1], arr[7], arr[298], arr[299], @len(name));
	@printf("{} {} {} {} {} {} {} {} {}\n", *p, q[2], z, f(), ps[1].y, big[0], big[4], big[39], gm.counter);
	@printf("{} {} {} {} {}\n", zp, ex[0], ex[1], ex[3], row[2]);
	a += 1;
	@printf("{}\n", a);
	console.exit(0);
}
`}

// TestGlobalInit: fc 4 のモジュールの変数の初期値 (起動のときに runtime の fc_global_init が ROM の記録から写す)。
// emu の -O 0 / -O 2 (NES は TestGlobalInitBanked)。
func TestGlobalInit(t *testing.T) {
	t.Parallel()
	want := "5 -300 200 3 -4 2 9 0 7 5\n9 9 0 42 4 4660 22136 0 1234\n77 0 3 0 9\n6\n"
	out, err := buildBothLevels(t, globalInitSrc)
	if err != nil || out != want {
		t.Errorf("emu: got %q, %v, want %q", out, err, want)
	}
	r := testBuild(t, buildSpec{Files: globalInitSrc, CompileOnly: true})
	if r.Err != nil {
		t.Fatal(r.Err)
	}
	// 0 の並びは写さない (既定の BSS は起動のときに 0)。0 で埋めない領域 (BSS_EX) は全部写す
	asm := r.Built(t, "_t.s")
	if strings.Contains(asm, ".byte 255") {
		t.Errorf("arr の 0 の並びを写している:\n%s", asm)
	}
}

// TestGlobalInitBanked: 切り替えのバンクのモジュールの変数の初期値も起動のときに写す (記録は固定のバンク)。
func TestGlobalInitBanked(t *testing.T) {
	t.Parallel()
	files := map[string]string{
		"fc.toml": "[target]\nmapper = \"UxROM\"\nprg = \"64K\"\n[bank.a]\nslot = 0x8000\n",
		"main.fc": `#fc 4
@(farcall);
use uxrom;
use ma;
public var out:[8]u8;
public var done:u8;
var mine:u8 = 11;
struct P { x:u8; y:u16; }
var ps:[2]P = [{1, 0}, {0, 0x1234}];
function main():void
{
	uxrom.init();
	out[0] = mine;
	out[1] = ma.v;
	out[2] = ma.get(2);
	out[3] = ps[0].x;
	out[4] = (ps[1].y >> 8) as u8;
	out[5] = ps[1].y as u8;
	done = 1;
	while (true) {
	}
}
`,
		"ma.fc": "#fc 4\n@(bank: \"a\");\npublic var v:u8 = 33;\nvar t:[4]u8 = [5, 6, 7, 8];\npublic function get(i:u8):u8 @(noinline) { return t[i]; }\n",
	}
	for _, level := range []int{-1, 0} {
		out, done, _ := runNes(t, files, level, 6)
		if done != 1 || fmtInts(out) != "[11 33 7 1 18 52]" {
			t.Errorf("-O %d: out=%v done=%d", level, out, done)
		}
	}
}

// TestGlobalInitErrors: 定数でない初期値、固定番地の変数、fc 3 のモジュールはエラー。初期値の無いビルドは runtime に何も足さない。
func TestGlobalInitErrors(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ src, msg string }{
		{"#fc 4\nvar v:u8;\nvar w:u8 = v;\n", "the initial value of global variable w must be a constant"},
		{"#fc 4\nvar v:u8;\nvar w:[2]u8 = [1, v];\n", "`v` is a variable"},
		{"#fc 4\nvar io:u8 = 1 @(address: 0x6000);\n", "cannot have an initial value"},
		{"#fc 4\nvar w:u8 = 300;\n", "300"},
		{"#fc 3\nvar w:u8 = 1;\n", "can't init global variable w"},
	} {
		_, err := buildFiles(t, map[string]string{"t.fc": c.src + "function main():void {}\n"})
		if err == nil || !strings.Contains(err.Error(), c.msg) {
			t.Errorf("%s: got %v, want /%s/", c.src, err, c.msg)
		}
	}
	// 文字表の変換器 (_T) も定数
	out, err := buildFiles(t, map[string]string{"t.txt": "＿あかい゛", "t.fc": "#fc 4\nuse console;\nconst _T = @textmap(\"t.txt\");\n" +
		"var c:u8 = _T('か');\nvar m = _T(\"いあ\");\nfunction main():void { @printf(\"{} {} {} {}\\n\", c, m[0], m[1], @len(m)); console.exit(0); }\n"})
	if want := "2 3 1 3\n"; err != nil || out != want {
		t.Errorf("_T: got %q, %v, want %q", out, err, want)
	}
	r := testBuild(t, buildSpec{Files: map[string]string{"t.fc": "#fc 4\nvar w:u8;\nfunction main():void { w = 1; }\n"}})
	if r.Err != nil {
		t.Fatal(r.Err)
	}
	if _, err := os.Stat(filepath.Join(r.Res.BuildDir, "_fc_init.s")); err == nil {
		t.Errorf("初期値の無いビルドで _fc_init.s を書いた")
	}
}
