package driver

// レジスタ割付 (doc/v2_frame_alloc.md §3): バイト単位の詰め込み、フレームへのあふれ、fastcall 領域の大きさ。

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFrameSpill(t *testing.T) {
	t.Parallel()
	// 同時に生きる変数が 16 バイトを超える普通の関数: レジスタから外れた分はフレームに置かれ、正しく動く
	out := runEmu(t, `function many(a:int):int16
{
	var v0:int16 = a as int16 + 1;
	var v1:int16 = a as int16 + 2;
	var v2:int16 = a as int16 + 3;
	var v3:int16 = a as int16 + 4;
	var v4:int16 = a as int16 + 5;
	var v5:int16 = a as int16 + 6;
	var v6:int16 = a as int16 + 7;
	var v7:int16 = a as int16 + 8;
	var v8:int16 = a as int16 + 9;
	var v9:int16 = a as int16 + 10;
	var v10:int16 = a as int16 + 11;
	var v11:int16 = a as int16 + 12;
	return v0 + v1 + v2 + v3 + v4 + v5 + v6 + v7 + v8 + v9 + v10 + v11;
}
function bytes(a:int):int
{
	var b0 = a + 1; var b1 = a + 2; var b2 = a + 3; var b3 = a + 4; var b4 = a + 5; var b5 = a + 6;
	var b6 = a + 7; var b7 = a + 8; var b8 = a + 9; var b9 = a + 10; var b10 = a + 11; var b11 = a + 12;
	var b12 = a + 13; var b13 = a + 14; var b14 = a + 15; var b15 = a + 16; var b16 = a + 17; var b17 = a + 18;
	return b0 + b1 + b2 + b3 + b4 + b5 + b6 + b7 + b8 + b9 + b10 + b11 + b12 + b13 + b14 + b15 + b16 + b17;
}
function fc(a:int, b:int16, c:int):int16 options(fastcall: true)
{
	var v0:int16 = b + 1;
	var v1:int16 = b + 2;
	var v2:int16 = b + 3;
	var v3:int16 = b + 4;
	var v4:int16 = b + 5;
	var v5:int16 = b + 6;
	var v6:int16 = b + 7;
	return v0 + v1 + v2 + v3 + v4 + v5 + v6 + a as int16 + c as int16;
}
function main():void
{
	printf(many(1), " ", bytes(1), " ", fc(1, 100, 2), "\n");
	exit(0);
}
`)
	// many: 12 + 78 = 90、bytes: 18 + 171 = 189、fc: 700 + 28 + 3 = 731
	if out != "90 189 731\n" {
		t.Errorf("got %q", out)
	}
}

// TestFastcallStatic: fc で本体を持つ fastcall 関数は静的フレーム (FC_FASTCALL_REG の 16 バイト制限は extern だけ) なので、
// ローカルが多くてもコンパイルでき、呼び出しは呼び先のフレーム F_<sym> に直接書く。
func TestFastcallStatic(t *testing.T) {
	t.Parallel()
	src := `#fc 2
options(fastcall_reg: 16);
function fc(a:int, b:int16, c:int):int16 options(fastcall: true)
{
	var v0:int16 = b + 1;
	var v1:int16 = b + 2;
	var v2:int16 = b + 3;
	var v3:int16 = b + 4;
	var v4:int16 = b + 5;
	var v5:int16 = b + 6;
	var v6:int16 = b + 7;
	return v0 + v1 + v2 + v3 + v4 + v5 + v6 + a as int16 + c as int16;
}
function main():void { fc(1, 2, 3); }
`
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "t.fc"), []byte(src), 0o666); err != nil {
		t.Fatal(err)
	}
	if _, err := NewCompiler(absRepoRoot).Build("t.fc", &BuildOptions{Dir: dir, BuildDir: filepath.Join(dir, "b"), Out: filepath.Join(dir, "a.bin"), CompileOnly: true}); err != nil {
		t.Fatalf("got %v", err)
	}
	mod, _ := os.ReadFile(filepath.Join(dir, "b", "_t.s"))
	frames, _ := os.ReadFile(filepath.Join(dir, "b", "_frames.inc"))
	if !strings.Contains(string(mod), "sta <F_t_fc+3") || !strings.Contains(string(frames), "F_t_fc = FC_SZP+") {
		t.Errorf("_t.s / _frames.inc:\n%s\n%s", mod, frames)
	}
	// base.s に静的フレームの領域と大きさが出る
	base, _ := os.ReadFile(filepath.Join(dir, "b", "base.s"))
	if len(base) > 0 && !strings.Contains(string(base), "FC_SZP_SIZE = 64") {
		t.Errorf("base.s:\n%s", base)
	}
}

// TestUnusedFunctions: main / 割り込み / options(symbol:) / 関数ポインタ (代入・const 表) / asm から辿れない関数は
// 出力しない (frames.Analyze の tree shaking)。届く関数 (使われない関数からだけ呼ばれるものは届かない) だけが .s に残る。
func TestUnusedFunctions(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	src := `#fc 2
use * from stdio;
var fp:fn():void;
function dead():void { dead2(); }
function dead2():void { }
public function dead_public():void { }
function live():void { }
function by_pointer():void { }
function by_table():void { }
const TAB:[1]fn():void = [by_table];
function by_asm():void { }
function by_symbol():void options(symbol: "_from_asm") { }
function main():void
{
	live();
	fp = by_pointer;
	fp();
	TAB[0]();
	asm("jsr _t_by_asm");
	exit(0);
}
`
	if err := os.WriteFile(filepath.Join(dir, "t.fc"), []byte(src), 0o666); err != nil {
		t.Fatal(err)
	}
	res, err := NewCompiler(absRepoRoot).BuildContext(context.Background(), "t.fc", &BuildOptions{Dir: dir, BuildDir: filepath.Join(dir, "b"), Out: filepath.Join(dir, "a.bin"), CompileOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	mod, _ := os.ReadFile(filepath.Join(dir, "b", "_t.s"))
	for _, want := range []string{"_t_live", "_t_by_pointer", "_t_by_table", "_t_by_asm", "_from_asm", "_main"} {
		if !strings.Contains(string(mod), ".proc "+want) {
			t.Errorf("%s が出力されていない", want)
		}
	}
	for _, dead := range []string{"_t_dead", "_t_dead2", "_t_dead_public"} {
		if strings.Contains(string(mod), dead) {
			t.Errorf("%s が出力されている (使われない)", dead)
		}
	}
	found := false
	for _, line := range res.Frames {
		if strings.Contains(line, "unused (not emitted)") && strings.Contains(line, "_t_dead2") && !strings.Contains(line, "_t_live") {
			found = true
		}
	}
	if !found {
		t.Errorf("-d の要約に unused が無い: %q", res.Frames)
	}
}

// TestCc65Abi: options(abi: "cc65") の extern 関数 (cc65 の __fastcall__ 規約): 引数 0〜1 個を A (1 バイト) / A,X (2 バイト) で
// 渡し、戻り値を A / A,X で受ける。呼び先は X / Y を壊す (static / stack の両方の呼び出し側から)。
func TestCc65Abi(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	asm := `; cc65 __fastcall__ 規約のテスト用 (fc の t モジュールに include される)
.export _t_dbl, _t_add16, _t_seven, _t_store
_t_dbl:      ; int dbl(int x): A = x → x * 2
	asl a
	ldx #77      ; X / Y を壊す
	ldy #66
	rts
_t_add16:    ; int16 add16(int16 v): A/X = v → v + 0x0101
	clc
	adc #1
	pha
	txa
	adc #1
	tax
	pla
	ldy #66
	rts
_t_seven:    ; int seven(): 7
	lda #7
	ldx #77
	rts
_t_store:    ; void store(int v)
	sta _t_g
	ldx #77
	ldy #66
	rts
`
	src := `#fc 2
use * from stdio;
include("cc65.asm");
var g:int;
function dbl(x:int):int options(abi: "cc65");
function add16(v:int16):int16 options(abi: "cc65");
function seven():int options(abi: "cc65");
function store(v:int):void options(abi: "cc65");
function rec(n:int):int16 options(abi: "stack")
{
	if (n == 0) { return 0; }
	var d = dbl(n);
	return add16(rec(n - 1)) + d;
}
function main():void
{
	var a:[4]int;
	for (var i = 0; i < 4; i++) { a[i] = dbl(i + 1) + seven(); }
	store(a[3]);
	var w = add16(0x1234);
	printf(a[0], " ", a[3], " ", g, " ", w, " ", rec(3), "\n");
	exit(0);
}
`
	if err := os.WriteFile(filepath.Join(dir, "cc65.asm"), []byte(asm), 0o666); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "t.fc"), []byte(src), 0o666); err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	code, err := NewCompiler(absRepoRoot).Build("t.fc", &BuildOptions{Dir: dir, BuildDir: filepath.Join(dir, "b"), Out: filepath.Join(dir, "a.bin"), Run: true, Stdout: &out})
	if err != nil {
		t.Fatalf("ビルド失敗: %v", err)
	}
	if code != 0 {
		t.Fatalf("終了コード %d: %s", code, out.String())
	}
	// dbl(i+1)+7: 9, 15; g = 15; 0x1234 + 0x101 = 4917; rec(3) = ((0+257+2)+257+4)+257+6 = 783
	if want := "9 15 15 4917 783\n"; out.String() != want {
		t.Errorf("got %q\nwant %q", out.String(), want)
	}

	for _, c := range []struct{ src, want string }{
		{"function f(a:int, b:int):void options(abi: \"cc65\");\nfunction main():void { f(1, 2); }\n", "at most one argument"},
		{"function f(a:int):void options(abi: \"cc65\") { }\nfunction main():void { f(1); }\n", "is for extern functions"},
		{"var p:fn(int):void;\nfunction f(a:int):void options(abi: \"cc65\");\nfunction main():void { p = f; }\n", "cannot take the address"},
	} {
		if got := compileErr(t, c.src); !strings.Contains(got, c.want) {
			t.Errorf("%q: got %q, want %q", c.src, got, c.want)
		}
	}
}
