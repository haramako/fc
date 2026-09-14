package driver

// castle の doc/memo.md「FC BUG」由来の回帰テスト (doc/go_evolution_plan.md R4 の表)。
// 小さなプログラムを emu で実行して出力を見る。

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runEmu は fc 2 のソース (use * from stdio 済み) を emu でビルド・実行し、標準出力を返す。
func runEmu(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	src := "#fc 2\nuse * from stdio;\n" + body
	if err := os.WriteFile(filepath.Join(dir, "t.fc"), []byte(src), 0o666); err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	code, err := NewCompiler(absRepoRoot).Build("t.fc", &BuildOptions{Dir: dir, BuildDir: filepath.Join(dir, "b"), Out: filepath.Join(dir, "a.bin"), Run: true, Stdout: &out})
	if err != nil {
		t.Fatalf("ビルド失敗: %v", err)
	}
	if code != 0 {
		t.Fatalf("終了コード %d:\n%s", code, out.String())
	}
	return out.String()
}

func TestBugGlobalPointerIndex(t *testing.T) {
	t.Parallel()
	// グローバルのポインタ変数への添字代入 (index+pset の融合がポインタを配列扱いしていた)
	out := runEmu(t, `var buf:int*;
var arr:int[8];
function main():void
{
	buf = arr;
	buf[2] = 5;
	var v = buf[2];
	var w = arr[2];
	printf("v=", v, " w=", w, "\n");
	exit(0);
}
`)
	if out != "v=5 w=5\n" {
		t.Errorf("got %q", out)
	}
}

func TestBugUnusedExpressionStatement(t *testing.T) {
	t.Parallel()
	// 結果を使わない演算の式文 (一時変数に場所が割り付かずコード生成で落ちていた)
	out := runEmu(t, `function main():void
{
	var c = 32;
	c == 32;
	c + 1;
	-c;
	printf("ok\n");
	exit(0);
}
`)
	if out != "ok\n" {
		t.Errorf("got %q", out)
	}
}

func TestBugVoidValue(t *testing.T) {
	t.Parallel()
	// void 関数の呼び出しを値として使うとエラー (クラッシュしない)
	dir := t.TempDir()
	src := "#fc 2\nfunction f():void {}\nfunction main():void\n{\n\tif (f()) {}\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "t.fc"), []byte(src), 0o666); err != nil {
		t.Fatal(err)
	}
	_, err := NewCompiler(absRepoRoot).Build("t.fc", &BuildOptions{Dir: dir, BuildDir: filepath.Join(dir, "b"), Out: filepath.Join(dir, "a.bin"), CompileOnly: true})
	if err == nil || !strings.Contains(err.Error(), "expression has no value") {
		t.Errorf("got %v", err)
	}
}

func TestBugLocalArray(t *testing.T) {
	t.Parallel()
	// ローカル配列 (レジスタ領域に 2 バイトで置かれて隣の一時変数と重なっていた → フレームに置く)。
	// 定数添字・変数添字・2 バイト添字・ポインタ経由・関数への受け渡し
	out := runEmu(t, `use mem;
function fill(p:int*, n:int):void
{
	var i:int;
	for (i = 0; i < n; i++) {
		p[i] = i + 40;
	}
}
function sum(p:int*, n:int):int
{
	var s = 0;
	var i:int;
	for (i = 0; i < n; i++) {
		s += p[i];
	}
	return s;
}
function main():void
{
	var la:int[4];
	la[0] = 7;
	la[1] = 8;
	la[2] = 9;
	la[3] = 10;
	var l0 = la[0];
	var l3 = la[3];
	printf("const ", l0, " ", l3, "\n");
	var i:int;
	for (i = 0; i < 4; i++) {
		la[i] = i + 20;
	}
	var l2 = la[2];
	l3 = la[3];
	printf("var ", l2, " ", l3, "\n");
	var i16:int16 = 1;
	la[i16] = 99;
	var l1 = la[1];
	printf("idx16 ", l1, "\n");
	var p:int* = la;
	p[0] = 1;
	l0 = la[0];
	printf("ptr ", l0, "\n");
	mem.set(la, 3, 4);
	var s1 = sum(la, 4);
	fill(la, 4);
	var s2 = sum(la, 4);
	printf("call ", s1, " ", s2, "\n");
	var big:int[20];
	big[19] = 77;
	var b19 = big[19];
	printf("big ", b19, "\n");
	exit(0);
}
`)
	want := "const 7 10\nvar 22 23\nidx16 99\nptr 1\ncall 12 166\nbig 77\n"
	if out != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}
}

// TestCheckWarnings: fcc check / ビルド結果の警告 (構文検査 + v1 の include("*.rb"))。
func TestCheckWarnings(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	src := "use * from stdio;\ninclude(\"stdio.rb\");\nfunction main():void\n{\n\tvar a = 1;\n\tif (a & 2 == 0) {}\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "t.fc"), []byte(src), 0o666); err != nil {
		t.Fatal(err)
	}
	c := NewCompiler(absRepoRoot)
	ws, err := c.Check("t.fc", &CheckOptions{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(ws) != 2 || !strings.Contains(ws[0].Msg, "no longer needed") || ws[0].Pos.Line != 2 ||
		!strings.Contains(ws[1].Msg, "binds looser") || ws[1].Pos.Line != 6 {
		t.Errorf("warnings: %+v", ws)
	}
	// Build の結果にも同じ警告が付く
	res, err := c.BuildContext(context.Background(), "t.fc", &BuildOptions{Dir: dir, BuildDir: filepath.Join(dir, "b"), Out: filepath.Join(dir, "a.bin"), CompileOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Warnings) != 2 {
		t.Errorf("build warnings: %+v", res.Warnings)
	}
}

func TestBugSignedCompare(t *testing.T) {
	t.Parallel()
	// 比較のコード生成 (v1: 左辺の符号しか見ない、符号付きのオーバーフロー、16 ビットの上位/下位の取り違え)
	out := runEmu(t, `function main():void
{
	var a:int16 = 0x0201;
	var b:int16 = 0x0102;
	var r1 = a < b;
	var r2 = b < a;
	printf("u16 ", r1, " ", r2, "\n");
	var s:sint = -100;
	var t:sint = 100;
	var r3 = s < t;
	var r4 = t < s;
	var u:sint = -1;
	var r5 = u < 0;
	var r6 = u > 0;
	var r7 = 0 < u;
	printf("s8 ", r3, " ", r4, " ", r5, " ", r6, " ", r7, "\n");
	var s16:sint16 = -300;
	var t16:sint16 = 300;
	var r8 = s16 < t16;
	var r9 = t16 < s16;
	var r10 = s16 < 0;
	printf("s16 ", r8, " ", r9, " ", r10, "\n");
	if (s < t) { printf("c1 "); }
	if (t < s) { printf("WRONG "); }
	if (a < b) { printf("WRONG "); }
	if (u >= 0) { printf("WRONG "); }
	if (s16 < t16) { printf("c2\n"); }
	exit(0);
}
`)
	want := "u16 0 1\ns8 1 0 1 0 0\ns16 1 0 1\nc1 c2\n"
	if out != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}
}
