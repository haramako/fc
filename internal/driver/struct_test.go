package driver

// struct (doc/v2_types_struct.md §4) の実行テスト。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// compileErr は fc 2 のソースをコンパイルしてエラーメッセージを返す (成功なら "")。
func compileErr(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	src := "#fc 2\n" + body
	if err := os.WriteFile(filepath.Join(dir, "t.fc"), []byte(src), 0o666); err != nil {
		t.Fatal(err)
	}
	_, err := NewCompiler(absRepoRoot).Build("t.fc", &BuildOptions{Dir: dir, BuildDir: filepath.Join(dir, "b"), Out: filepath.Join(dir, "a.bin"), CompileOnly: true})
	if err == nil {
		return ""
	}
	return err.Error()
}

func TestStructBasic(t *testing.T) {
	t.Parallel()
	// グローバル / ローカルの struct 変数、フィールドの読み書き、ポインタ経由、リテラル、値コピー、sizeof
	out := runEmu(t, `struct Point { x:int; y:int16; }
struct Line { a:Point; b:Point; tag:int; }
var g:Point;
var ln:Line;
function set(p:*Point, x:int, y:int16):void
{
	p.x = x;
	p.y = y;
}
function sum(p:Point):int16
{
	return p.x as int16 + p.y;
}
function make(x:int):Point
{
	var r = Point{x: x, y: (x * 2) as int16};
	return r;
}
function make2(x:int):Point
{
	return {x, 1};
}
function main():void
{
	g.x = 3;
	g.y = 300;
	printf("g=", g.x, ",", g.y, " size=", sizeof(Point), ",", sizeof(Line), ",", sizeof(g), "\n");
	var l:Point = {10, 20};
	set(&l, 7, 700);
	printf("l=", l.x, ",", l.y, " sum=", sum(l), "\n");
	var m = make(5);
	printf("m=", m.x, ",", m.y, " ", make2(6).x, ",", make2(6).y, "\n");
	m = g;
	g.x = 9;
	printf("copy=", m.x, ",", m.y, " g.x=", g.x, "\n");
	ln.a = l;
	ln.b.x = 1;
	ln.b.y = 2;
	ln.tag = 4;
	var lp = &ln;
	lp.a.x += 1;
	printf("ln=", ln.a.x, ",", ln.a.y, ",", lp.b.x, ",", lp.b.y, ",", lp.tag, "\n");
	exit(0);
}
`)
	want := "g=3,300 size=3,7,3\nl=7,700 sum=707\nm=5,10 6,1\ncopy=3,300 g.x=9\nln=8,700,1,2,4\n"
	if out != want {
		t.Errorf("got %q\nwant %q", out, want)
	}
}

func TestStructArrayAndConst(t *testing.T) {
	t.Parallel()
	// struct の配列 (要素サイズ 3: index のシフト加算)、定数 struct / 定数配列、ポインタ + 添字、ローカル配列
	out := runEmu(t, `struct Point { x:int; y:int16; }
const origin = Point{x: 1, y: 2};
const pts:[3]Point = [{1, 100}, {2, 200}, Point{x: 3, y: 300}];
var arr:[4]Point;
function total(p:*Point, n:int):int16
{
	var s:int16 = 0;
	for (var i = 0; i < n; i++) {
		s += p[i].y;
	}
	return s;
}
function main():void
{
	printf("origin=", origin.x, ",", origin.y, "\n");
	for (var i = 0; i < 3; i++) {
		printf("pts[", i, "]=", pts[i].x, ",", pts[i].y, "\n");
	}
	for (var i = 0; i < 4; i++) {
		arr[i].x = i;
		arr[i].y = (i * 10) as int16;
	}
	arr[2] = pts[2];
	printf("arr=", arr[0].x, ",", arr[1].y, ",", arr[2].x, ",", arr[2].y, ",", arr[3].y, " total=", total(arr, 4), "\n");
	var loc:[2]Point;
	loc[0] = {5, 6};
	loc[1].x = 7;
	loc[1].y = 8;
	printf("loc=", loc[0].x, ",", loc[0].y, ",", loc[1].x, ",", loc[1].y, " ", total(loc, 2), "\n");
	exit(0);
}
`)
	want := "origin=1,2\npts[0]=1,100\npts[1]=2,200\npts[2]=3,300\narr=0,10,3,300,30 total=340\nloc=5,6,7,8 14\n"
	if out != want {
		t.Errorf("got %q\nwant %q", out, want)
	}
}

func TestStructModules(t *testing.T) {
	t.Parallel()
	// 他モジュールの struct: mod.T と use T from mod、非公開の struct は見えない
	dir := t.TempDir()
	files := map[string]string{
		"geo.fc": "#fc 2\npublic struct Point { x:int; y:int; }\nstruct Hidden { a:int; }\npublic function len1(p:Point):int { return p.x + p.y; }\n",
		"t.fc": `#fc 2
use * from stdio;
use geo;
use Point from geo;
var a:geo.Point;
var b:Point;
function main():void
{
	a.x = 1; a.y = 2;
	b = a;
	printf("len=", geo.len1(b), "\n");
	exit(0);
}
`,
	}
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o666); err != nil {
			t.Fatal(err)
		}
	}
	var out strings.Builder
	code, err := NewCompiler(absRepoRoot).Build("t.fc", &BuildOptions{Dir: dir, BuildDir: filepath.Join(dir, "b"), Out: filepath.Join(dir, "a.bin"), Run: true, Stdout: &out})
	if err != nil {
		t.Fatalf("ビルド失敗: %v", err)
	}
	if code != 0 || out.String() != "len=3\n" {
		t.Errorf("code=%d out=%q", code, out.String())
	}

	if err := os.WriteFile(filepath.Join(dir, "u.fc"), []byte("#fc 2\nuse geo;\nvar h:geo.Hidden;\n"), 0o666); err != nil {
		t.Fatal(err)
	}
	_, err = NewCompiler(absRepoRoot).Build("u.fc", &BuildOptions{Dir: dir, BuildDir: filepath.Join(dir, "b2"), Out: filepath.Join(dir, "u.bin"), CompileOnly: true})
	if err == nil || !strings.Contains(err.Error(), "Hidden") {
		t.Errorf("非公開の struct が見えている: %v", err)
	}
}

func TestStructErrors(t *testing.T) {
	t.Parallel()
	cases := []struct{ src, want string }{
		{"struct P { x:int; }\nvar p:P;\nfunction main():void { p.z = 1; }\n", "has no field z"},
		{"struct P { x:int; }\nvar p:P;\nfunction main():void { p.x.y = 1; }\n", "is not a struct"},
		{"struct P { x:int; }\nfunction main():void { var q = P{1, 2}; }\n", "has 1 fields but 2 values"},
		{"struct P { x:int; y:int; }\nfunction main():void { var q = P{x: 1, 2}; }\n", "mixes named and positional"},
		{"struct P { x:int; y:int; }\nfunction main():void { var q = P{x: 1, x: 2}; }\n", "given twice"},
		{"struct P { x:int; y:int; }\nfunction main():void { var n:int; n = {1, 2}; }\n", "struct literal cannot be used as u8"},
		{"struct P { x:int; y:int; }\nfunction main():void { var q = [{1, 2}]; }\n", "needs a declared type"},
		{"struct P { x:int; p:P; }\n", "not complete yet"},
		{"struct P { x:int; }\nstruct P { y:int; }\n", "already defined"},
		{"struct P { x:int; x:int; }\n", "already defined in struct"},
		{"struct P { x:int; }\nfunction main():void { var v = P; }\n", "is a type, not a value"},
		{"var p:Q;\n", "unknown type Q"},
		{"struct P { x:int; }\nfunction f():void { struct Q { a:int; } }\n", "must be at module level"},
	}
	for _, c := range cases {
		got := compileErr(t, c.src)
		if !strings.Contains(got, c.want) {
			t.Errorf("%q:\n  got  %q\n  want /%s/", c.src, got, c.want)
		}
	}
}

// TestV2StructProgram は test/test_struct.fc (unittest 形式) を emu で実行する。
func TestV2StructProgram(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	var out strings.Builder
	code, err := NewCompiler(absRepoRoot).Build("test_struct.fc", &BuildOptions{
		Target: "emu", Out: filepath.Join(tmp, "a.bin"), Run: true, Stdout: &out,
		Dir: testDir(), BuildDir: filepath.Join(tmp, "build"),
	})
	if err != nil {
		t.Fatalf("ビルド失敗: %v", err)
	}
	if code != 0 || strings.Contains(out.String(), "ERROR") {
		t.Errorf("code=%d\n%s", code, out.String())
	}
}

// compileAsm は body (stdio 付き) をコンパイルして、生成された t モジュールのアセンブリ (_t.s) を返す。
func compileAsm(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	src := "#fc 2\nuse * from stdio;\n" + body
	if err := os.WriteFile(filepath.Join(dir, "t.fc"), []byte(src), 0o666); err != nil {
		t.Fatal(err)
	}
	if _, err := NewCompiler(absRepoRoot).Build("t.fc", &BuildOptions{Dir: dir, BuildDir: filepath.Join(dir, "b"), Out: filepath.Join(dir, "a.bin"), CompileOnly: true}); err != nil {
		t.Fatalf("コンパイル失敗: %v", err)
	}
	asm, err := os.ReadFile(filepath.Join(dir, "b", "_t.s"))
	if err != nil {
		t.Fatal(err)
	}
	return string(asm)
}

// stack 系 (再帰) の関数のフレームは `<S+k,x` で触るので、FC_STACK (128 バイト) を超えたら ld65 / ca65 の範囲エラーでなく
// frame size over にする (-O 2 のインライン展開で再帰関数のフレームが 223 / 260 バイトになり、fuzz で発覚)。
func TestStackFrameTooLarge(t *testing.T) {
	t.Parallel()
	msg := compileErr(t, `function r(n:int):int
{
	var big:[140]int;
	big[n & 7] = n;
	if (n == 0) { return big[0]; }
	return r(n - 1) + big[n & 7];
}
function main():void
{
	r(3);
}
`)
	if !strings.Contains(msg, "frame size over") || !strings.Contains(msg, "FC_STACK") {
		t.Errorf("want frame size over, got %q", msg)
	}
}

// TestFrameOverNoGrow: -O 2 の展開でフレームが上限を超えた関数は、その関数だけ展開を止めてコンパイルし直す
// (100 バイトのローカル配列を持つ inline 関数を 3 回展開すると main の静的フレームが 256 バイトを超える。-O 0 は通るのに
// -O 2 だけ frame size over だった。fuzz では種の 0.1〜0.2% がこれで飛ばされていた)。fcc check も同じくやり直す。
func TestFrameOverNoGrow(t *testing.T) {
	t.Parallel()
	src := `function f(n:int):int options(inline: true)
{
	var a:[100]int;
	a[n & 7] = n;
	return a[n & 7] + 1;
}
function main():void
{
	printf(f(1), " ", f(2), " ", f(3), "\n");
	exit(0);
}
`
	want := "2 3 4\n"
	for _, level := range []int{-1, 0} {
		if out := runEmuLevel(t, src, level); out != want {
			t.Errorf("-O %d: got %q want %q", level, out, want)
		}
	}
	if asm := compileAsm(t, src); !strings.Contains(asm, "jsr _t_f") {
		t.Errorf("main に f を展開したまま (呼び出しが無い):\n%s", asm)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "t.fc"), []byte("#fc 2\nuse * from stdio;\n"+src), 0o666); err != nil {
		t.Fatal(err)
	}
	if _, err := NewCompiler(absRepoRoot).Check("t.fc", &CheckOptions{Dir: dir}); err != nil {
		t.Errorf("fcc check: %v", err)
	}
}

// TestStackPushOverNoGrow: stack 系 (再帰) の関数が大きいフレームの後ろに引数を積むとき、積む位置 (`<S+k,x`) が
// FC_STACK を超えたら frame size over にし、-O 2 なら展開を止めてやり直す。以前はフレームだけを検査していて、inline 関数を
// 展開した再帰関数で `<S+128,x` になり ld65 の範囲エラーだった (fuzz の種 3572709)。
func TestStackPushOverNoGrow(t *testing.T) {
	t.Parallel()
	src := `function f(n:int):int options(inline: true)
{
	var a:[62]int;
	a[n & 7] = n;
	return a[n & 7] + 1;
}
function h(a:int16, b:int16, c:int16, d:int16, e:int16, g:int16, i:int16, j:int16):int16
{
	return a + b + c + d + e + g + i + j;
}
function r(n:int):int16
{
	if (n == 0) { return 0; }
	var x = f(n) + f(n + 1);
	return r(n - 1) + h(x, 1, 2, 3, 4, 5, 6, n);
}
function main():void
{
	printf(r(2), "\n");
	exit(0);
}
`
	want := "57\n" // r(1) = (2 + 3) + 21 + 1 = 27、r(2) = 27 + (3 + 4) + 21 + 2
	for _, level := range []int{-1, 0} {
		if out := runEmuLevel(t, src, level); out != want {
			t.Errorf("-O %d: got %q want %q", level, out, want)
		}
	}
}

// ポインタ経由の配列フィールド (`ta[i].arr[j]`、`p.arr[j]`) は、配列の中身でなく番地を添字の基にする。以前は rval が
// 配列フィールドを pget して、その中身を番地として添字を足し、別の場所に書いていた (-O 0 / -O 2 とも同じ値なので
// 差分の fuzz では見えず、生成器を広げるときの手計算で発覚)。
func TestArrayFieldViaPointer(t *testing.T) {
	t.Parallel()
	src := `struct T {
	x:int;
	arr:[4]int16;
}
var ta:[2]T;
var g0:int;
function setp(p:*T, i:int, v:int16):void
{
	p.arr[i & 3] = v;
}
function sum(q:*int16, n:int):int16
{
	var s:int16 = 0;
	for (var i:int = 0; i < n; i++) {
		s += q[i];
	}
	return s;
}
function main():void
{
	ta[1].arr[(g0 & 3)] = 300;
	ta[1].arr[2] = 500;
	setp(&ta[0], 1, 1000);
	var pp:*int16 = &ta[0].arr[3];
	*pp = 7;
	printf(ta[1].arr[0], " ", ta[1].arr[2], " ", ta[0].arr[1], " ", ta[0].arr[3], " ", sum(ta[1].arr, 4), " ", ta[0].x, " ", ta[1].x, "\n");
	exit(0);
}
`
	for _, level := range []int{-1, 0} {
		if got := runEmuLevel(t, src, level); got != "300 500 1000 7 800 0 0\n" {
			t.Errorf("level %d: got %q", level, got)
		}
	}
}

// 関数でない値の呼び出しはエラー (名前が同じ変数に取られて関数の宣言がエラーになったとき、以前は sema が panic した)。
func TestCallNonFunction(t *testing.T) {
	t.Parallel()
	es := buildErrors(t, map[string]string{"t.fc": `#fc 2
struct T { x:int; }
var t0:T;
function t0(p:int):int
{
	return p;
}
function main():void
{
	t0(1);
}
`}, "t.fc")
	var msgs []string
	for _, e := range es {
		msgs = append(msgs, e.Msg)
	}
	all := strings.Join(msgs, "\n")
	if !strings.Contains(all, "already defined") || !strings.Contains(all, "is not a function") {
		t.Errorf("want both errors, got %q", all)
	}
}
