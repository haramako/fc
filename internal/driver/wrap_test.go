package driver

// fc 4 の左の項の型で折り返す演算 `+%` / `-%` / `*%` (sema/wrap.go)。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haramako/fc/internal/syntax"
)

// TestWrapOps: 結果は左の項の型で、その幅で折り返す。u8 の座標 + i8 の移動量を割る・比べる・シフトする形が符号なしになる
// (`+` は同じ大きさなら符号付きが勝つ)。広い代入先でも広げない (A1 の区切り)。右の項が狭ければ自分の符号で広げる。
func TestWrapOps(t *testing.T) {
	t.Parallel()
	out, err := buildBothLevels(t, map[string]string{"t.fc": `#fc 4
use console;
var y:u8;
var dy:i8;
var vy:i8;
var giff:u8;
var big:u16;
function main():void
{
	y = 200;
	dy = 1;
	vy = -20;
	giff = 3;
	big = 1000;
	@printf("{} {}\n", ((y + dy) as i8) / 16, (y +% dy) / 16);
	@printf("{} {}\n", (y + dy) as i8 > 100, y +% dy > 100);
	@printf("{} {}\n", (vy +% giff) / 16, (y -% dy) >> 4);
	var n = y +% dy;
	var d:u16 = y +% 100;
	@printf("{} {} {} {} {}\n", n, d, big +% dy, big -% 1001, y *% 3);
	@printf("{} {}\n", y +% -1, (250 as u8) +% 10);
	const K:u8 = 250;
	const K2 = K +% 10;
	@printf("{}\n", K2);
	console.exit(0);
}
`})
	if want := "-4 12\nfalse true\n-2 12\n201 44 1001 65535 88\n199 4\n4\n"; err != nil || out != want {
		t.Errorf("got %q, %v\nwant %q", out, err, want)
	}
	for src, want := range map[string]string{
		"var a:u8; var w:u16; var x = a +% w;": "the right operand (u16) is wider than the left (u8)",
		"var a:u8; var x = a +% 300;":          "300 does not fit in u8",
		"var a:u8; var x = 3 +% a;":            "the left operand decides the type",
		// 項が式なら実行時の経路 (lval) を通る。型のない定数を見ていなくて黙って通っていた (TestRandomConstFoldV4)
		"var a:u16; var x = (a + 1) -% 65543;": "65543 does not fit in u16",
		"var a:u8; var x = 3 +% (a + 1);":      "the left operand decides the type",
		"var p:*u8; var x = p +% 1;":           "the left operand must be an integer",
	} {
		_, err := buildFiles(t, map[string]string{"t.fc": "#fc 4\nfunction main():void\n{\n\t" + src + "\n}\n"})
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: got %v, want /%s/", src, err, want)
		}
	}
	if _, err := buildFiles(t, map[string]string{"t.fc": "#fc 3\nfunction main():void\n{\n\tvar a:u8 = 1;\n\tvar x = a +% 1;\n}\n"}); err == nil || !strings.Contains(err.Error(), "`+%` is fc 4") {
		t.Errorf("fc 3: got %v", err)
	}
	src := "#fc 4\nvar x = a +% b *% c -% d;\n"
	if got, err := syntax.Format([]byte(src), "t.fc"); err != nil || string(got) != src {
		t.Errorf("fmt: got %q, %v", got, err)
	}
}

// TestMixedSignArith: 符号の混ざった `+ - *` の結果を解釈する所 (比較・`/ % >>`・`as` で広げる・型を省いた変数・@printf) は fc 4 で
// エラー。同じ大きさの代入先に入れるだけ・`& | ^`・A1 が部分木ごと広げる所 (16 ビットの代入先・相手) は通る。
func TestMixedSignArith(t *testing.T) {
	t.Parallel()
	out, err := buildBothLevels(t, map[string]string{"t.fc": `#fc 4
use console;
var y:u8;
var dy:i8;
var w:u16;
function main():void
{
	y = 200;
	dy = 1;
	w = 150;
	var y2:u8 = y + dy;
	y += dy;
	var m:u8 = (y + dy) & 0xf0;
	var t:u16 = (y + dy) / 16;
	var t2:u16 = y + dy + w;
	@printf("{} {} {} {} {}\n", y2, y, m, t, t2);
	console.exit(0);
}
`})
	if want := "201 201 192 12 352\n"; err != nil || out != want {
		t.Errorf("got %q, %v\nwant %q", out, err, want)
	}
	for _, src := range []string{
		"var b = y + dy > 100;",
		"var b = (y + dy) / 16;",
		"var b = (y - dy) % 3;",
		"var b = (y * dy) >> 1;",
		"var b = ((y + dy) & 0xf0) >> 4;",
		"var b = (y + dy) as u16;",
		"var b = y + dy;",
		"@printf(\"{}\", y + dy);",
	} {
		_, err := buildFiles(t, map[string]string{"t.fc": "#fc 4\nuse console;\nvar y:u8;\nvar dy:i8;\nfunction main():void\n{\n\t" + src + "\n}\n"})
		if err == nil || !strings.Contains(err.Error(), "mixes u8 and i8") {
			t.Errorf("%s: got %v", src, err)
		}
	}
}

// TestV4MigrateMixedSignArith: fc 3 の符号の混ざった演算の結果を解釈する所は、migrate が起点の演算を、符号付きの項が左なら
// `+%` などに、右なら `(…) as i8` にして fc 3 と同じ結果にする。
func TestV4MigrateMixedSignArith(t *testing.T) {
	t.Parallel()
	src := `#fc 3
use * from stdio;
function main():void
{
	var vy:i8 = -20;
	var giff:u8 = 3;
	var x:u8 = 250;
	var vx:i8 = -1;
	var a:i8 = (vy + giff) / 16;
	var b:i8 = (giff + vy) / 16;
	var c:i8 = (vy - giff) >> 1;
	var d:bool = x + vx > 100;
	x += vx;
	printf(a, " ", b, " ", c, " ", d, " ", x, "\n");
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
		"var a:i8 = (vy +% giff) / 16;", "var b:i8 = (giff + vy) as i8 / 16;", "var c:i8 = (vy -% giff) >> 1;",
		"var d:bool = (x + vx) as i8 > 100;", "x += vx;",
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
	if before != after || before != "-2 -2 -12 0 249\n" {
		t.Errorf("fc 3: %q, fc 4: %q", before, after)
	}
}

// TestV4MigrateWidenThenConvert: 8 ビットで折り返す fc 3 の計算を保つ `as i8` (widen) と、代入先への変換の `as u16`
// (constant-range) が同じ式に付くとき、`as i8` が内側 (`... as i8 as u16`)。E・D を評価の前に判定するようにしてから、外側の
// 変換を先に足して `... as u16 as i8` になり fc 4 でエラーになっていた (fuzz の TestRandomMigrate)。
func TestV4MigrateWidenThenConvert(t *testing.T) {
	t.Parallel()
	src := `#fc 3
use * from stdio;
var g:u16;
function main():void
{
	var l:u16 = ((15932 as i8) << 2);
	g = ((100 as i8) << 2);
	printf(l, " ", g, "\n");
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
	for _, want := range []string{"((15932 as i8) << 2) as i8 as u16;", "((100 as i8) << 2) as i8 as u16;"} {
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
	if before != after || before != "65520 65424\n" {
		t.Errorf("fc 3: %q, fc 4: %q", before, after)
	}
}

// TestUntypedConstCast: 組み込みの結果の型のない定数への同じ型の `as` は型付きの定数にする (`(@min(0, -6) as i8) + x` (x:u8) は
// i8 の -6 と u8 の和で i8。値そのものを返していて、型のない -6 として u8 に合わせられ 253 になっていた。sema/typing.go の
// 型の照合で発覚)。
func TestUntypedConstCast(t *testing.T) {
	t.Parallel()
	out, err := buildBothLevels(t, map[string]string{"t.fc": `#fc 3
use * from stdio;
var x:u8;
function main():void
{
	x = 3;
	var y:i16 = (@min(0, -6) as i8) + x;
	printf(y, "\n");
	exit(0);
}
`})
	if err != nil || out != "-3\n" {
		t.Errorf("got %q, %v", out, err)
	}
}
