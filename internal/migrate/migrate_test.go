package migrate

import (
	"strings"
	"testing"

	"github.com/haramako/fc/internal/syntax"
)

// TestMigratePragma: `#fc 2` の行は `#fc 3` に、プラグマの無いソースには先頭に足す。fc 3 のソースはそのまま。
// コメントと書式は残る。
func TestMigratePragma(t *testing.T) {
	cases := []struct{ in, want string }{
		{"#fc 2\n// c\nvar a:bool;  // x\n", "#fc 3\n// c\nvar a:bool;  // x\n"},
		{"var a:bool;\n", "#fc 3\nvar a:bool;\n"},
		{"#fc 3\nvar a:u8;\n", "#fc 3\nvar a:u8;\n"},
		{"", "#fc 3\n"},
	}
	for _, c := range cases {
		got, err := Migrate([]byte(c.in), "t.fc")
		if err != nil {
			t.Errorf("%q: %v", c.in, err)
			continue
		}
		if string(got) != c.want {
			t.Errorf("%q: got %q want %q", c.in, got, c.want)
		}
	}
	if _, err := Migrate([]byte("var a:int\n"), "t.fc"); err == nil || !strings.Contains(err.Error(), "parse error") {
		t.Errorf("fc 2 として解析できないソース: %v", err)
	}
}

// TestMigrateRule: 規則は Edit を足し、重なる Edit はエラー。
func TestMigrateRule(t *testing.T) {
	saved := Rules
	defer func() { Rules = saved }()
	Rules = []Rule{{Name: "rename", Apply: func(c *Ctx) {
		for _, tk := range c.Tokens {
			if tk.Kind == syntax.Identifier && tk.Text == "a" {
				c.ReplaceToken(tk, "b")
			}
		}
	}}}
	got, err := Migrate([]byte("var a:bool;\nfunction f():void { a = true; }\n"), "t.fc")
	if err != nil || string(got) != "#fc 3\nvar b:bool;\nfunction f():void { b = true; }\n" {
		t.Errorf("got %q, %v", got, err)
	}
	Rules = append(Rules, Rule{Name: "overlap", Apply: func(c *Ctx) { c.Replace(4, 6, "x") }})
	if _, err := Migrate([]byte("var a:bool;\n"), "t.fc"); err == nil || !strings.Contains(err.Error(), "overlapping") {
		t.Errorf("重なる Edit: %v", err)
	}
}

// TestMigrateIntTypes: 型の位置の整数型名だけを書き換える (変数名・コメント・文字列・モジュール名つきの型はそのまま)。
func TestMigrateIntTypes(t *testing.T) {
	in := `#fc 2
// int のコメント
var a:int;
var b:[4]sint16 = [1, 2, 3, 4];
struct P { x:uint8; y:int16; }
function f(p:*sint, q:fn(int8):uint):int16 { var s = "int"; return (p[0] as int16) + (p[1] as uint16); }
const N = sizeof(sint8) + sizeof(P);
var c = bitcast<*int>(0x2000);
var d:mod.int;
`
	want := `#fc 3
// int のコメント
var a:u8;
var b:[4]i16 = [1, 2, 3, 4];
struct P { x:u8; y:u16; }
function f(p:*i8, q:fn(u8):u8):u16 { var s = "int"; return (p[0] as u16) + (p[1] as u16); }
const N = @sizeof(i8) + @sizeof(P);
var c = @bitcast(*u8, 0x2000);
var d:mod.int;
`
	got, err := Migrate([]byte(in), "t.fc")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// TestMigrateAtBuiltins: 組み込みを @ の形にする。同じファイルで宣言した同名の関数の呼び出しはそのまま。
func TestMigrateAtBuiltins(t *testing.T) {
	in := `#fc 2
include("x.asm");
include("font.chr") options(size: 4096, fill: 0);
const T:[]int = incbin("t.bin");
const _T = textmap("font.txt");
function f():void {
	asm("sei", "cli");
	var n = sizeof(int) + min(1, 2) + clamp(3, 0, 1);
	var p = bitcast<*fn():void>(0x2000);
	unittest_run_tests();
}
`
	want := `#fc 3
@include("x.asm");
@include("font.chr", size: 4096, fill: 0);
const T:[]u8 = @incbin("t.bin");
const _T = @textmap("font.txt");
function f():void {
	@asm("sei", "cli");
	var n = @sizeof(u8) + @min(1, 2) + @clamp(3, 0, 1);
	var p = @bitcast(*fn():void, 0x2000);
	@run_tests();
}
`
	got, err := Migrate([]byte(in), "t.fc")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
	// 同じファイルで宣言した max はそのまま
	got, err = Migrate([]byte("function max(a:int, b:int):int { return a; }\nfunction g():void { max(1, 2); min(1, 2); }\n"), "t.fc")
	if err != nil || !strings.Contains(string(got), "{ max(1, 2); @min(1, 2); }") {
		t.Errorf("同名の宣言: %q, %v", got, err)
	}
}

// TestMigrateAttributes: options(...) → @(...)。真偽値の属性の `: true` は省く。block { } options(); は @(...) { }。
func TestMigrateAttributes(t *testing.T) {
	in := `#fc 2
options(bank: 3, farcall: true);
var v:int options(address: 0x2000);
function f():int options(fastcall: true, inline: true, segment: "game") { return 1; }
function g():void options(inline: false) { }
block {
	var b:int;
} options(bss: "BSS_EX");
`
	want := `#fc 3
@(bank: 3, farcall);
var v:u8 @(address: 0x2000);
function f():u8 @(fastcall, inline, segment: "game") { return 1; }
function g():void @(inline: false) { }
@(bss: "BSS_EX") {
	var b:u8;
}
`
	got, err := Migrate([]byte(in), "t.fc")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}
