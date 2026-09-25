package driver

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultArguments(t *testing.T) {
	for _, level := range []int{-1, 2} {
		t.Run(fmt.Sprintf("level_%d", level), func(t *testing.T) {
			got := runEmuLevel(t, `options(farcall: true);
function sum(x:u8, y:u8 = BASE+1, z:u8 = sizeof(u16)):u8 { return x+y+z; }
function all(x:u8 = 4, y:u8 = 5):u8 { return x+y; }
function recurse(n:u8, amount:u8 = 2):u8 {
    if (n == 0) { return 0; }
    return amount + recurse(n-1);
}
function add(x:u8):u8 { return x+3; }
function callback(f:farfn(u8):u8 = add, x:u8 = 7):u8 { return f(x); }
function nullable(p:*u8 = null, f:fn():void = null):u8 { return (p == null) + (f == null); }
struct Pair { x:u8; y:u16; }
const PAIR:Pair = {4,1000};
function pair(p:Pair = PAIR):u16 { p.x+=1; return p.x+p.y; }
function text(p:*u8 = "abc"):u8 { return p[1]; }
function negate(x:i16 = -2):i16 { return x; }
function small(x:u8 = 260):u8 { return x; }
const MESSAGE="test";
function same_text(p:*u8=MESSAGE):u8 { return p == MESSAGE; }
var counter:u8;
function bump():u8 { counter+=1; return counter; }
const BASE=5;
const alias=sum;
const far_alias:farfn(u8,u8,u8):u8=sum;
function main():void {
    printf(sum(1), ",", sum(1,2), ",", sum(1,2,3), ",", alias(2), ",", all(), ",");
    printf(recurse(3), ",", callback(), ",", nullable(), ",", pair(), ",", pair(), ",");
    printf(text(), ",", negate() == -2, ",", small(), ",", sum(bump(),bump()), ",", counter, ",", same_text(), "\n");
    var fp:fn(u8,u8,u8):u8=sum;
    var far:farfn(u8,u8,u8):u8=sum;
    const LOCAL=13;
    function local(x:u8=LOCAL):u8 { return x; }
    printf(fp(2,3,4), ",", far(3,4,5), ",", far_alias(4), ",", local(), "\n");
    exit(0);
}`, level)
			const want = "9,5,6,10,9,6,10,2,1005,1005,98,1,4,5,2,1\n9,12,12,13\n"
			if got != want {
				t.Fatalf("got %q; want %q", got, want)
			}
		})
	}
}

func TestDefaultArgumentsDeclarationScope(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"main.fc": `#fc 2
options(farcall:true);
use * from stdio;
use api;
include("external.asm");
function external(x:u8=6):u8 options(abi:"cc65");
const DEFAULT=99;
function main():void {
    const DEFAULT=88;
    printf(api.value(), ",", api.callback(), ",", api.self(), ",", api.other(), ",", api.mutual(), ",", external(), "\n");
    exit(0);
}
`,
		"api.fc": `#fc 2
options(bank:1);
public function value(x:u8=DEFAULT):u8 options(noinline:true) { return x; }
public function callback(f:farfn():u8=hidden):u8 { return f(); }
public function self(f:*void=self):u8 { return f != null; }
public function other(f:fn():u8=hidden):u8 options(noinline:true) { return f(); }
public function mutual(f:*void=partner):u8 { return f != null; }
function partner(f:*void=mutual):u8 { return f != null; }
function hidden():u8 { return 17; }
const DEFAULT=LATER+1;
const LATER=6;
`,
		"external.asm": "_main_external:\n clc\n adc #1\n rts\n",
	})
	var out strings.Builder
	res, err := NewCompiler(absRepoRoot).BuildContext(t.Context(), "main.fc", &BuildOptions{
		Dir: dir, BuildDir: filepath.Join(dir, "b"), Out: filepath.Join(dir, "a.bin"),
		Run: true, Stdout: &out, MaxCycles: 1_000_000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.ExitCode != 0 || out.String() != "7,17,1,17,1,7\n" {
		t.Fatalf("exit=%d out=%q", res.ExitCode, out.String())
	}
}

func TestDefaultArgumentErrors(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, src, want string }{
		{"required after optional", `function f(a:u8=1,b:u8):void {}`, "required parameter b cannot follow a default argument"},
		{"runtime global", `var g:u8; function f(a:u8=g):void {}`, "must be a constant expression"},
		{"runtime call", `function g():u8{return 1;} function f(a:u8=g()):void {}`, "must be a constant expression"},
		{"parameter reference", `const a=9; function f(a:u8,b:u8=a):void {}`, "must be a constant expression"},
		{"self parameter", `const a=9; function f(a:u8=a):void {}`, "must be a constant expression"},
		{"unknown", `function f(a:u8=MISSING):void {}`, "MISSING not found"},
		{"wrong type unused", `function f(a:u8=null):void {}`, "null"},
		{"wrong type explicitly supplied", `function f(a:*u8=1):void {} function main():void {f(null);}`, "default argument a of f"},
		{"too few", `function f(a:u8,b:u8=2):void {} function main():void {f();}`, "expects 1..2 argument(s) but 0 given"},
		{"too many", `function f(a:u8=1):void {} function main():void {f(1,2);}`, "expects 1 argument(s) but 2 given"},
		{"near pointer", `function f(a:u8=1):void {} function main():void {var p=f; p();}`, "expects 1 argument(s) but 0 given"},
		{"far pointer", `options(farcall:true); function f(a:u8=1):void {} function main():void {var p:farfn(u8):void=f; p();}`, "expects 1 argument(s) but 0 given"},
		{"table", `function f(a:u8=1):void {} const TABLE=[f]; function main():void {TABLE[0]();}`, "expects 1 argument(s) but 0 given"},
		{"returned pointer", `function f(a:u8=1):void {} function get():fn(u8):void{return f;} function main():void {get()();}`, "expects 1 argument(s) but 0 given"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := compileErr(t, tc.src); !strings.Contains(got, tc.want) {
				t.Fatalf("got %q; want %q", got, tc.want)
			}
		})
	}
}
