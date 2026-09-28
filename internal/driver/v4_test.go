package driver

// fc 4 の整数の規則 (doc/v4_plan.md §1.3) と、fc 3 → 4 の migrate の書き換え (sema の Rewrite) のテスト。

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestV4Errors: fc 4 でエラーになる形 (E: 範囲外の定数、D: 大きさが減る暗黙の変換)。
func TestV4Errors(t *testing.T) {
	t.Parallel()
	pre := "#fc 4\nvar g:u8;\nfunction take(n:u8):void { g = n; }\n"
	cases := []struct{ name, src, want string }{
		{"E: 初期化", "function f():void { var c:u8 = 300; g = c; }", "300 does not fit in u8"},
		{"E: 符号", "function f():void { var y:i8 = 200; g = y; }", "200 does not fit in i8"},
		{"E: 負の数を符号なしに", "function f():void { var u:u16 = -1; g = u as u8; }", "-1 does not fit in u16"},
		{"E: 戻り値", "function f():u8 { return -1; }", "-1 does not fit in u8"},
		{"E: 引数", "function f():void { take(256); }", "256 does not fit in u8"},
		{"E: 代入", "function f():void { g = 999; }", "999 does not fit in u8"},
		{"E: const の注釈", "const B:u8 = 300;", "const B: 300 does not fit in u8"},
		{"E: const の注釈 (符号)", "const F:u8 = -1;", "const F: -1 does not fit in u8"},
		{"E: 型のない const", "const N = 300;\nfunction f():void { g = N; }", "300 does not fit in u8"},
		{"D: 初期化", "function f(w:u16):void { var lo:u8 = w; g = lo; }", "cannot convert u16 to u8 implicitly"},
		{"D: 代入", "function f(w:u16):void { g = w + 1; }", "cannot convert u16 to u8 implicitly"},
		{"D: 戻り値", "function f(w:u16):u8 { return w; }", "cannot convert u16 to u8 implicitly"},
		{"D: 引数", "function f(w:u16):void { take(w); }", "cannot convert u16 to u8 implicitly"},
		{"D: i16 → i8", "function f(s:i16):i8 { return s; }", "cannot convert i16 to i8 implicitly"},
	}
	for i, c := range cases {
		c := c
		t.Run(fmt.Sprintf("err%02d", i+1), func(t *testing.T) {
			t.Parallel()
			_, err := buildFiles(t, map[string]string{"t.fc": pre + c.src + "\nfunction main():void { }\n"})
			if err == nil {
				t.Fatalf("%s: エラーにならない: %s", c.name, c.src)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("%s: %s: got %v, want %q", c.name, c.src, err, c.want)
			}
		})
	}
}

// TestV4Accepts: fc 4 で通る形。明示の `as` は切り詰め、同じ大きさで符号だけ違う変換 (i8 → u8。座標 + 移動量) は暗黙に通る。
func TestV4Accepts(t *testing.T) {
	t.Parallel()
	out, err := buildBothLevels(t, map[string]string{"t.fc": `#fc 4
use * from stdio;
const B:u16 = 300;
const NONE = 255;
function none():u8 { return -1 as u8; }
function main():void
{
	var w:u16 = 0x1234;
	var x:u8 = 200;
	var vx:i8 = -1;
	x = x + vx;
	x += vx;
	var lo:u8 = w as u8;
	var c:u8 = 300 as u8;
	var s:i8 = -3;
	var u:u8 = s;
	var n:u8 = NONE;
	printf(B, " ", none(), " ", x, " ", lo, " ", c, " ", u, " ", n, "\n");
	exit(0);
}
`})
	if err != nil {
		t.Fatal(err)
	}
	if want := "300 255 198 52 44 253 255\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// TestV4MigrateRewrites: fc 3 のソースを migrate すると、fc 4 で意味が変わる所 (E / D) に `as` を足し、const の注釈を今の型に
// 直す。書き換えた fc 4 のソースは fc 3 と同じ出力になる。
func TestV4MigrateRewrites(t *testing.T) {
	t.Parallel()
	src := `#fc 3
use * from stdio;
const B:u8 = 300;
const F:u8 = -1;
function none():u8 { return -1; }
function put(n:u8):void { printf(n, "\n"); }
function main():void
{
	var w:u16 = 0x1234;
	var c:u8 = 300;
	var y:i8 = 200;
	var lo:u8 = w;
	lo = w + 1;
	put(256);
	put(w);
	var x:u8 = 200;
	var vx:i8 = -1;
	x = x + vx;
	printf(B, " ", F, " ", none(), " ", c, " ", y, " ", lo, " ", x, "\n");
	exit(0);
}
`
	want := `#fc 4
use * from stdio;
const B:u16 = 300;
const F:i8 = -1;
function none():u8 { return -1 as u8; }
function put(n:u8):void { printf(n, "\n"); }
function main():void
{
	var w:u16 = 0x1234;
	var c:u8 = 300 as u8;
	var y:i8 = 200 as i8;
	var lo:u8 = w as u8;
	lo = (w + 1) as u8;
	put(256 as u8);
	put(w as u8);
	var x:u8 = 200;
	var vx:i8 = -1;
	x = x + vx;
	printf(B, " ", F, " ", none(), " ", c, " ", y, " ", lo, " ", x, "\n");
	exit(0);
}
`
	dir := t.TempDir()
	path := filepath.Join(dir, "t.fc")
	if err := os.WriteFile(path, []byte(src), 0o666); err != nil {
		t.Fatal(err)
	}
	res, err := NewCompiler(absRepoRoot).Migrate([]string{path}, &MigrateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got := string(res[path]); got != want {
		t.Fatalf("migrate:\n%s\nwant:\n%s", got, want)
	}
	before, err := buildBothLevels(t, map[string]string{"t.fc": src})
	if err != nil {
		t.Fatal(err)
	}
	after, err := buildBothLevels(t, map[string]string{"t.fc": want})
	if err != nil {
		t.Fatal(err)
	}
	if before != after || before != "0\n52\n300 -1 255 44 -56 53 199\n" {
		t.Errorf("fc 3: %q, fc 4: %q", before, after)
	}
}
