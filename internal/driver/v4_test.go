package driver

// fc 4 の整数の規則 (Agent/wiki/plans/v4-plan.md §1.3) と、fc 3 → 4 の migrate の書き換え (sema の Rewrite) のテスト。

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
		{"F2: u8 << 8", "function f(hi:u8):void { var a = hi << 8; g = a as u8; }", "shifting u8 left by 8 always gives 0"},
		{"F2: 8 ビットの代入先", "function f(hi:u8, lo:u8):void { g = hi << 8 | lo; }", "shifting u8 left by 8 always gives 0"},
		{"F2: u8 >> 8", "function f(lo:u8):void { g = lo >> 8; }", "shifting u8 right by 8 always gives 0"},
		{"F2: u16 << 16", "function f(w:u16):void { w = w << 16; g = w as u8; }", "shifting u16 left by 16 always gives 0"},
		{"F4: 16 ビットに入らない", "const Z = [40000, -1];", "do not fit in one integer type"},
		{"F6: u8 と i8", "function f(x:u8, v:i8):void { if (x < v) { g = 1; } }", "ordered comparison of signed i8 and unsigned u8 would compare as i8"},
		{"F6: 座標 + 移動量と u8", "function f(x:u8, v:i8, lim:u8):void { if (x + v > lim) { g = 1; } }", "signed i8 and unsigned u8"},
		{"F6: u16 と i8", "function f(w:u16, v:i8):void { if (w >= v) { g = 1; } }", "would compare as u16"},
		{"F6: u16 と i16", "function f(w:u16, v:i16):void { if (w <= v) { g = 1; } }", "signed i16 and unsigned u16"},
		{"F6: 型付きの定数", "const A:u8 = 200;\nconst B:i8 = -1;\nfunction f():void { if (A > B) { g = 1; } }", "signed i8 and unsigned u8"},
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
	@printf("{} {} {} {} {} {} {}\n", B, none(), x, lo, c, u, n);
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

// TestV4IntRulesF: fc 4 で通る F2・F4・F6 の形。A1 で広がるシフト、符号付きの `>>`、配列リテラルは定数の値を変えない型
// (宣言の型があればそちら)、符号なしが狭い比較・`==`・型のない定数との比較。
func TestV4IntRulesF(t *testing.T) {
	t.Parallel()
	out, err := buildBothLevels(t, map[string]string{"t.fc": `#fc 4
use * from stdio;
const T = [128, -1];
const U = [1000, -1];
const S = [200, 100];
const D:[3]i16 = [128, -1, 5];
function main():void
{
	var hi:u8 = 0x12;
	var lo:u8 = 0x34;
	var s:i8 = -100;
	var x:u8 = 250;
	var vx:i8 = -1;
	var lim:u8 = 200;
	var s16:i16 = -5;
	var h:u16 = hi << 8 | lo;
	var r = [vx, 200];
	@printf("{} {}\n", h, s >> 8);
	@printf("{} {} {} {} {} {} {} {} {}\n", T[0], T[1], @sizeof(T), U[1], @sizeof(U), @sizeof(S), D[0], r[1], @sizeof(r));
	@printf("{:d} {:d} {:d} {:d}\n", x < s16, x == (-6 as i8), (x + vx) as u8 > lim, x + vx > 100);
	exit(0);
}
`})
	if err != nil {
		t.Fatal(err)
	}
	if want := "4660 -1\n128 -1 4 -1 4 2 128 200 4\n0 1 1 0\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// TestV4MigrateIntRulesF: fc 3 の配列リテラルの型 (F4) と符号の混ざった比較 (F6) は、migrate が今の型の `as` を足して fc 3 と
// 同じ結果にする。
func TestV4MigrateIntRulesF(t *testing.T) {
	t.Parallel()
	src := `#fc 3
use * from stdio;
const T = [128, -1];
const U = [1000, -1];
const A:u8 = 200;
const B:i8 = -1;
function main():void
{
	var x:u8 = 250;
	var vx:i8 = -1;
	var lim:u8 = 200;
	var w:u16 = 100;
	var r = [vx, 200];
	printf(T[0], " ", U[1], " ", r[1], " ", x < vx, " ", w > vx, " ", x + vx > lim, " ", A > B, "\n");
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
	got := string(res[path])
	for _, want := range []string{
		"const T = [128 as i8, -1];", "const U = [1000, -1 as u16];", "var r = [vx, 200 as i8];",
		"x as i8 < vx", "w > vx as u16", "x + vx > lim as i8", "A as i8 > B",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("migrate の結果に %q が無い:\n%s", want, got)
		}
	}
	before, err := buildBothLevels(t, map[string]string{"t.fc": src})
	if err != nil {
		t.Fatal(err)
	}
	if want := "-128 65535 -56 1 0 1 0\n"; before != want {
		t.Errorf("fc 3: got %q, want %q", before, want)
	}
	after, err := buildBothLevels(t, map[string]string{"t.fc": got})
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Errorf("fc 3: %q, fc 4: %q", before, after)
	}
}

// versionSrc は fc 3 と fc 4 で共有する雛形 tmpl (版は %d) の、版 ver のソース。fc 4 の printf は書式文字列なので、fc 4 版は
// fc 3 版を printf の書き換え (printf-format) だけで migrate したもの (ほかは fc 4 の意味のまま)。
func versionSrc(t *testing.T, tmpl string, ver int) string {
	t.Helper()
	src := fmt.Sprintf(tmpl, 3)
	if ver < 4 {
		return src
	}
	out, err := migrateRules(t, map[string]string{"t.fc": src}, []string{"printf-format"})
	if err != nil {
		t.Fatal(err)
	}
	return out["t.fc"]
}

// a1Src は A1 (代入先と式の中の一番広い型で計算) と F1 (シフトは左辺の型) の例 (版は %d)。
const a1Src = `#fc %d
use * from stdio;
function put16(n:u16):void { printf(n, "\n"); }
function ret16(a:u8, b:u8):u16 { return a + b; }
function main():void
{
	var a:u8 = 200;
	var b:u8 = 100;
	var y:u8 = 5;
	var x:u8 = 200;
	var dx:i8 = -1;
	var hi:u8 = 0x12;
	var lo:u8 = 0x34;
	var pts:u8 = 30;
	var score:u16 = 1000;
	var n7:u16 = 7;
	var s1:i8 = 1;
	var d:u16 = a + b;
	var avg:u16 = (a + b) / 2;
	var addr = 0x2000 + y * 64;
	score += pts * 10;
	var h:u16 = hi << 8 | lo;
	var w:i16 = x + dx;
	var m:i16 = -y;
	var n:u16 = x + -1;
	var c:u8 = a + b;
	printf(d, " ", avg, " ", addr, " ", score, " ", h, " ", w, " ", m, " ", n, " ", c, "\n");
	put16(a + b);
	printf(ret16(a, b), " ", a + b > 250, " ", (a + b) as u16, "\n");
	var r1:u16 = y << n7;
	var r2 = a << s1;
	var r2w:i16 = r2;
	printf(r1, " ", r2w, "\n");
	exit(0);
}
`

// TestV4Widen: fc 4 は式を代入先・引数・戻り値の型と式の中の一番広い型の広いほうで計算する (A1)。代入先の無い比較、`as` の
// 中は今の幅のまま。シフトの結果は左辺の型 (F1)。fc 3 は今までどおり 8 ビットで折り返す。
func TestV4Widen(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		ver  int
		want string
	}{
		{4, "300 150 8512 1300 4660 199 -5 199 44\n300\n300 0 44\n640 144\n"},
		{3, "44 22 8256 1044 52 -57 251 199 44\n44\n44 0 44\n640 -112\n"},
	} {
		out, err := buildBothLevels(t, map[string]string{"t.fc": versionSrc(t, a1Src, c.ver)})
		if err != nil {
			t.Fatal(err)
		}
		if out != c.want {
			t.Errorf("fc %d: got %q, want %q", c.ver, out, c.want)
		}
	}
}

// typedConstSrc は型付きの定数どうしの演算と A1 の例 (版は %d)。
const typedConstSrc = `#fc %d
use * from stdio;
const A:u8 = 200;
const B:u8 = 100;
const C = A + B;
function main():void
{
	var d:u16 = A + B;
	var e:u16 = (200 as u8) + (100 as u8);
	var f:u8 = A + B;
	var g:u16 = ((A + B) / 2) as u8;
	var h:u16 = (A + B) / 2;
	var k:bool = (A + B) == (300 as u16);
	var m:u16 = -(5 as u8);
	var n:u16 = (A + B) as u8;
	var w:u16 = 1;
	w = w + ((253 as u8) | (127 as i8));
	printf(d, " ", e, " ", f, " ", g, " ", h, " ", k, " ", m, " ", n, " ", C, " ", w, "\n");
	exit(0);
}
`

// TestV4TypedConst: 型付きの定数どうしの演算も、fc 4 では広がる式の中なら広い幅で計算する (畳み込みが折り返した値に元の式の
// 印を付け、広い型と出会ったら元の式をその幅で畳み込み直す: widen.go)。`as` の中・const の宣言・代入先が 8 ビットなら折り返す。
// fc 3 から migrate すると `as` が足されて fc 3 と同じ結果になる。
func TestV4TypedConst(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		ver  int
		want string
	}{
		{4, "300 300 44 22 150 1 65531 44 44 256\n"},
		{3, "44 44 44 22 22 0 251 44 44 0\n"},
	} {
		out, err := buildBothLevels(t, map[string]string{"t.fc": versionSrc(t, typedConstSrc, c.ver)})
		if err != nil {
			t.Fatal(err)
		}
		if out != c.want {
			t.Errorf("fc %d: got %q, want %q", c.ver, out, c.want)
		}
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "t.fc")
	if err := os.WriteFile(path, []byte(fmt.Sprintf(typedConstSrc, 3)), 0o666); err != nil {
		t.Fatal(err)
	}
	res, err := NewCompiler(absRepoRoot).Migrate([]string{path}, &MigrateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	out, err := buildBothLevels(t, map[string]string{"t.fc": string(res[path])})
	if err != nil {
		t.Fatal(err)
	}
	if want := "44 44 44 22 22 0 251 44 44 0\n"; out != want {
		t.Errorf("migrate した fc 4: got %q, want %q\n%s", out, want, res[path])
	}
}

// strConstSrc は名前付きの文字列定数の長さの例 (版は %d)。
const strConstSrc = `#fc %d
use * from stdio;
const NM = "joe";
const NM2:[?]u8 = "ab";
const NM3:[4]u8 = "xyz";
function n(s:[]const u8):u8 { return @len(s); }
function main():void
{
	printf(@len(NM), " ", @sizeof(NM), " ", n(NM), " ", n("joe"), " ", n(NM2), " ", @len(NM2), " ", @len(NM3), "\n");
	var k:u8 = 0;
	for (var c in NM) { k++; }
	var s:[]const u8 = NM;
	var s2 = NM[1..];
	printf(k, " ", @len(s), " ", @len(s2), " ", NM3[3], "\n");
	exit(0);
}
`

// TestV4StringConst: fc 4 の文字列は 0 終端にしない (名前付きの定数も長さ・@sizeof が文字数。長さを書いた配列は余りが 0)。
// fc 3 から migrate すると、名前付きの文字列定数の宣言が長さつき (終端の 0 の分を含む) になって fc 3 と同じ結果。
func TestV4StringConst(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		ver  int
		want string
	}{
		{4, "3 3 3 3 2 2 4\n3 3 2 0\n"},
		{3, "4 4 4 3 3 3 4\n4 4 3 0\n"},
	} {
		out, err := buildBothLevels(t, map[string]string{"t.fc": versionSrc(t, strConstSrc, c.ver)})
		if err != nil {
			t.Fatal(err)
		}
		if out != c.want {
			t.Errorf("fc %d: got %q, want %q", c.ver, out, c.want)
		}
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "t.fc")
	if err := os.WriteFile(path, []byte(fmt.Sprintf(strConstSrc, 3)), 0o666); err != nil {
		t.Fatal(err)
	}
	res, err := NewCompiler(absRepoRoot).Migrate([]string{path}, &MigrateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	got := string(res[path])
	for _, want := range []string{`const NM:[4]u8 = "joe";`, `const NM2:[3]u8 = "ab";`, `const NM3:[4]u8 = "xyz";`} {
		if !strings.Contains(got, want) {
			t.Errorf("migrate の結果に %q が無い:\n%s", want, got)
		}
	}
	out, err := buildBothLevels(t, map[string]string{"t.fc": got})
	if err != nil {
		t.Fatal(err)
	}
	if want := "4 4 4 3 3 3 4\n4 4 3 0\n"; out != want {
		t.Errorf("migrate した fc 4: got %q, want %q", out, want)
	}
}

// TestV4MigrateWiden: A1・F1 で意味が変わる所は、migrate が今の型の `as` を足して fc 3 と同じ結果にする。fc 3 の `hi << 8`
// (値が必ず 0) は、fc 3 と同じ意味の書き方が fc 4 の F2 のエラーになるので、migrate がエラーにする。
func TestV4MigrateWiden(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "t.fc")
	if err := os.WriteFile(path, []byte(fmt.Sprintf(a1Src, 3)), 0o666); err != nil {
		t.Fatal(err)
	}
	if _, err := NewCompiler(absRepoRoot).Migrate([]string{path}, &MigrateOptions{}); err == nil || !strings.Contains(err.Error(), "shift-zero") {
		t.Errorf("hi << 8: err = %v, want shift-zero", err)
	}
	src := strings.Replace(fmt.Sprintf(a1Src, 3), "hi << 8 | lo", "hi << 7 | lo", 1)
	if err := os.WriteFile(path, []byte(src), 0o666); err != nil {
		t.Fatal(err)
	}
	res, err := NewCompiler(absRepoRoot).Migrate([]string{path}, &MigrateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	got := string(res[path])
	for _, want := range []string{
		"return (a + b) as u8;", "var d:u16 = (a + b) as u8;", "var avg:u16 = ((a + b) / 2) as u8;",
		"var addr = 0x2000 + (y * 64) as u8;", "score += (pts * 10) as u8;", "var h:u16 = (hi << 7 | lo) as u8;",
		"var w:i16 = (x + dx) as i8;", "var m:i16 = -y as u8;", "var n:u16 = (x + -1) as u8;", "var c:u8 = a + b;",
		"put16((a + b) as u8);", "var r1:u16 = y as u16 << n7;", "var r2 = a as i8 << s1;",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("migrate の結果に %q が無い:\n%s", want, got)
		}
	}
	before, err := buildBothLevels(t, map[string]string{"t.fc": src})
	if err != nil {
		t.Fatal(err)
	}
	after, err := buildBothLevels(t, map[string]string{"t.fc": got})
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Errorf("fc 3: %q, fc 4: %q", before, after)
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
function put(n:u8):void { @printf("{}\n", n); }
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
	@printf("{} {} {} {} {} {} {}\n", B, F, none(), c, y, lo, x);
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

// TestV4MigrateCompoundNested: 複合代入の書き換え (`x op= y` → `x = (x op y) as T`) の右辺の中にも書き換え (`-l0` の
// `as i8`) があると、右辺を含む置き換えと重なって "overlapping or invalid edit" になっていた (fuzz の TestRandomMigrate)。
// 複合代入は前と後ろだけを書き換える。
func TestV4MigrateCompoundNested(t *testing.T) {
	t.Parallel()
	src := `#fc 3
use * from stdio;
function f(l0:i8):u8
{
	var l2:u8 = 1;
	l2 -= ((~204) ^ (-l0));
	return l2;
}
function main():void
{
	printf(f(3), "\n");
	exit(0);
}
`
	want := `#fc 4
use * from stdio;
function f(l0:i8):u8
{
	var l2:u8 = 1;
	l2 = (l2 - ((~204) ^ (-l0) as i8)) as u8;
	return l2;
}
function main():void
{
	@printf("{}\n", f(3));
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
	if before != after {
		t.Errorf("fc 3: %q, fc 4: %q", before, after)
	}
}

// TestUnusedPrivateGlobals: fc 4 のモジュールの private な変数 (既定の BSS) と配列定数 (public でも) は、出力する関数から
// 参照されなければ領域を取らない (どこからも呼ばれない関数・@(test) の関数だけが使う大きなバッファ・表など)。public の変数・
// @(symbol:)・置き場所を指定したもの (@(segment:)。整列の詰め物のように並びを当てにしうる)・fc 3 のモジュールの変数は残す。
func TestUnusedPrivateGlobals(t *testing.T) {
	t.Parallel()
	src := `#fc 4
use console;
use old;
var used_v:u8;
var only_helper:[300]u8;
var never:u8;
var placed:[4]u8 @(segment: "BSS");
public var pub:u8;
var named:u8 @(symbol: "_named_v");
const TABLE_USED = [1, 2, 3, 4, 5];
const TABLE_TEST = [6, 7, 8, 9, 10];
public const TABLE_PUB = [11, 12, 13];
const TABLE_SYM:[3]u8 = [14, 15, 16] @(symbol: "_t_sym");
function helper():void { only_helper[0] = 1; }
function test_x():void @(test) { only_helper[1] = TABLE_TEST[1]; }
function main():void
{
	used_v = TABLE_USED[2];
	@printf("{} {}\n", used_v, old.get());
	console.exit(0);
}
`
	old := "#fc 3\nvar unused_old:u8;\npublic function get():u8 { return 5; }\n"
	files := map[string]string{"t.fc": src, "old.fc": old}
	s := compileAsmFiles(t, files)
	for _, sym := range []string{"_t_used_v:", "_t_placed:", "_t_pub:", "_named_v:", "_t_TABLE_USED:", "_t_sym:"} {
		if !strings.Contains(s, sym) {
			t.Errorf("%s が無い:\n%s", sym, s)
		}
	}
	for _, sym := range []string{"_t_only_helper", "_t_never", "_t_TABLE_TEST", "_t_TABLE_PUB"} {
		if strings.Contains(s, sym) {
			t.Errorf("%s が残っている", sym)
		}
	}
	r := testBuild(t, buildSpec{Files: files, CompileOnly: true})
	if o := r.Built(t, "_old.s"); !strings.Contains(o, "_old_unused_old:") {
		t.Errorf("fc 3 の変数が消えた:\n%s", o)
	}
	if out, err := buildFiles(t, files); err != nil || out != "3 5\n" {
		t.Errorf("got %q, %v", out, err)
	}
}

// TestConstIndex: fc 4 の const の初期値では、定数の配列 (文字列・名前付きの配列定数) を定数の添字で引く式を畳む
// (`const C = "#"[0];`。fc には文字のリテラルが無いので文字のコードに使う)。範囲の外はエラー。fc 3 は今までどおり定数にならない。
func TestConstIndex(t *testing.T) {
	t.Parallel()
	out, err := buildFiles(t, map[string]string{"t.fc": `#fc 4
use console;
const HASH = "#"[0];
const T = [5, 6, 300];
const X = T[1] + 1;
const Y = T[2];
function main():void
{
	@printf("{} {} {}\n", HASH, X, Y);
	console.exit(0);
}
`})
	if err != nil || out != "35 7 300\n" {
		t.Errorf("got %q, %v", out, err)
	}
	for src, want := range map[string]string{
		"#fc 4\nconst T = [1, 2];\nconst Z = T[2];\nfunction main():void { }\n": "index 2 is out of the constant array (length 2)",
		"#fc 3\nconst C = \"#\"[0];\nfunction main():void { }\n":                "const C must be constant",
	} {
		if _, err := buildFiles(t, map[string]string{"t.fc": src}); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: got %v, want /%s/", src, err, want)
		}
	}
}

// TestCharLiteral: fc 4 の文字のリテラル 'A' は文字のコードの定数 (エスケープは \n \t \0 \ \' \xNN)。ASCII 以外の文字は
// @textmap の変換器に渡したときだけ使える (_T('あ') は表で引いた 1 つのコード。2 つ以上のコードになる文字はエラー)。
// fc 3 の '...' は今までどおり文字列。
func TestCharLiteral(t *testing.T) {
	t.Parallel()
	out, err := buildFiles(t, map[string]string{
		"t.txt": "＿あかい゛",
		"t.fc": `#fc 4
use console;
const _T = @textmap("t.txt");
const HASH = '#';
const TAB = ['a', 'b', '\n'];
function main():void
{
	var c:u8 = 'A';
	c += 1;
	@printf("{} {} {} {} {} {} {}\n", c, HASH, TAB[2], '\'', '\\', '\x7f', '\0');
	@printf("{} {}\n", _T('あ'), _T('い'));
	switch (c) {
	case 'B':
		@printf("B\n");
	default:
		@printf("?\n");
	}
	console.exit(0);
}
`})
	if err != nil || out != "66 35 10 39 92 127 0\n1 3\nB\n" {
		t.Errorf("got %q, %v", out, err)
	}
	for src, want := range map[string]string{
		"#fc 4\nvar x:u8 = 'あ';\nfunction main():void { }\n":   "'あ' is not an ASCII character; convert it with a textmap converter (_T('あ'))",
		"#fc 4\nvar x:u8 = 'ab';\nfunction main():void { }\n":   "'ab' is not one character (in fc 4 '...' is a character literal; write strings with \"...\")",
		"#fc 4\nvar x:u8 = 'a;\nfunction main():void { }\n":    "unterminated character literal",
		"#fc 4\nvar x:u8 = '\\q';\nfunction main():void { }\n": "invalid escape in character literal",
		"#fc 4\nconst _T = @textmap(\"t.txt\");\nvar x:u8 = _T('が');\nfunction main():void { }\n": "'が' converts to 2 codes",
	} {
		if _, err := buildFiles(t, map[string]string{"t.fc": src, "t.txt": "＿あかい゛"}); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: got %v, want /%s/", src, err, want)
		}
	}
	// fc 3 の '...' は文字列 (エスケープを解釈しない)
	out, err = buildFiles(t, map[string]string{"t.fc": "#fc 3\nuse * from stdio;\nfunction main():void { printf('ab', \"\\n\"); exit(0); }\n"})
	if err != nil || out != "ab\n" {
		t.Errorf("fc 3: got %q, %v", out, err)
	}
}

// TestV4MigrateCompoundLhs: 複合代入の左辺の中にも書き換え (シフトの左辺の `156 as u8`) があると、左辺を含む置き換えと重なって
// "overlapping or invalid edit" になっていた (fuzz の TestRandomMigrate)。2 つめの左辺は中の書き換えを当てた写し。
func TestV4MigrateCompoundLhs(t *testing.T) {
	t.Parallel()
	src := `#fc 3
use * from stdio;
var a1:[16]u8;
var k:u8;
function main():void
{
	k = 2;
	a1[((156 >> ((6 & 7) as u8)) & 7)] += (((-66) << 6) + (5 as i8));
	printf(a1[7], "\n");
	exit(0);
}
`
	want := `#fc 4
use * from stdio;
var a1:[16]u8;
var k:u8;
function main():void
{
	k = 2;
	a1[((156 as u8 >> ((6 & 7) as u8)) & 7)] = (a1[((156 as u8 >> ((6 & 7) as u8)) & 7)] + (((-66) << 6) + (5 as i8))) as u8;
	@printf("{}\n", a1[7]);
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
	if before != after {
		t.Errorf("fc 3: %q, fc 4: %q", before, after)
	}
}

// TestConstCompareWidenedSign: 型付きの定数の大小の比較で、A1 で広げたオペランド (`(255 as u8) << 3` は i16 と出会うと u16 の
// 2040) の符号の混在 (F6) は広げる前の型で見る (実行時と同じ。定数の形だけエラーになっていた。TestRandomConstFoldV4 の種
// 700002)。値も実行時の形と同じ。
func TestConstCompareWidenedSign(t *testing.T) {
	t.Parallel()
	out, err := buildBothLevels(t, map[string]string{"t.fc": `#fc 4
use console;
function id_u8(x:u8):u8 @(noinline) { return x; }
function id_i16(x:i16):i16 @(noinline) { return x; }
function main():void
{
	var a = (100 as i16) >= ((255 as u8) << 3);
	var b = id_i16(100) >= (id_u8(255) << 3);
	var c = (3000 as i16) >= ((255 as u8) << 3);
	var d = id_i16(3000) >= (id_u8(255) << 3);
	@printf("{} {} {} {}\n", a, b, c, d);
	console.exit(0);
}
`})
	if err != nil {
		t.Fatal(err)
	}
	if out != "false false true true\n" {
		t.Errorf("got %q", out)
	}
}
