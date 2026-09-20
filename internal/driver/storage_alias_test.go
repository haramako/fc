package driver

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStorageAlias(t *testing.T) {
	for _, level := range []int{-1, 2} {
		t.Run(fmt.Sprint(level), func(t *testing.T) {
			got := runEmuLevel(t, `options(farcall:true);
alias work:Work=buffer;
alias bytes:[6]uint8=work;
alias first:uint8=bytes;
alias signed:sint8=first;
var buffer:[32]uint8;
struct Work { x:uint8; y:uint8; params:[4]uint8; }
struct Four { a:uint8; b:uint8; c:uint8; d:uint8; }
struct Shift { pad:uint8; value:Four; }
alias four:Four=buffer;
alias shifted:Shift=buffer;
alias fp:farfn(uint8):uint8=buffer;
var word:uint16;
alias word_bytes:[2]uint8=word;
function add(x:uint8):uint8 { return x+7; }
function edit(p:*Work):void options(noinline:true) { p.y+=3; }
function snapshot():Work { return work; }
function alias():uint8 { return 17; }
function main():void {
 buffer[6]=99;
 work={x:1,y:2};
 printf(work.x,",",work.y,",",work.params[3],",",buffer[6],"\n");
 alias local:Work=work;
 for(var i:uint8=0;i<4;i++) { local.params[i]=i+10; }
 edit(&local);
 var copy=snapshot();
 local.x=9;
 printf(copy.x,",",copy.y,",",copy.params[3],",",work.x,",",bytes[2],",",sizeof(local),"\n");
 for(var i:uint8=0;i<5;i++) { first=first+1; bytes[0]=bytes[0]+2; }
 printf(work.x,",",alias(),"\n");
 signed=-1; printf(first,",",signed<0,"\n");
 word_bytes[0]=52; word_bytes[1]=18;
 printf(word,",",word_bytes[1],"\n");
 four={1,2,3,4}; shifted.value=four;
 printf(buffer[0],buffer[1],buffer[2],buffer[3],buffer[4],"\n");
 four=shifted.value;
 printf(buffer[0],buffer[1],buffer[2],buffer[3],"\n");
 fp=add; printf(fp(3),",",fp!=null,"\n");
 exit(0);
}`, level)
			want := "1,2,0,99\n1,5,13,9,10,6\n24,17\n255,1\n4660,18\n11234\n1234\n10,1\n"
			if got != want {
				t.Fatalf("got %q; want %q", got, want)
			}
		})
	}
}

func TestStorageAliasModules(t *testing.T) {
	for _, level := range []int{-1, 2} {
		t.Run(fmt.Sprint(level), func(t *testing.T) {
			dir := t.TempDir()
			writeFiles(t, dir, map[string]string{
				"main.fc": `use * from stdio;
use a;
use b;
use exported from facade;
function main():void { a.init(); b.change(); printf(a.read(),",",exported,"\n"); exit(0); }
`,
				"shared.fc": `use a; public var mem:[32]uint8;`,
				"a.fc": `use shared;
struct Work { x:uint8; y:uint8; }
alias work:Work=shared.mem;
public function init():void options(noinline:true) {work.x=1;work.y=2;}
public function read():uint8 options(noinline:true) {return work.x+work.y;}
var hidden:uint8;
public alias exported:uint8=hidden;
`,
				"b.fc": `use shared; use exported from a;
alias bytes:[32]uint8=shared.mem;
public function change():void options(noinline:true) {bytes[1]=7;exported=9;}
`,
				"facade.fc": `public use exported from a;`,
			})
			var out strings.Builder
			_, err := NewCompiler(absRepoRoot).BuildContext(t.Context(), "main.fc", &BuildOptions{Dir: dir, BuildDir: filepath.Join(dir, "b"), Out: filepath.Join(dir, "a.bin"), Run: true, Stdout: &out, OptimizeLevel: level, MaxCycles: 1000000})
			if err != nil {
				t.Fatal(err)
			}
			if out.String() != "8,9\n" {
				t.Fatal(out.String())
			}
			asm, err := os.ReadFile(filepath.Join(dir, "b", "_a.s"))
			if err != nil {
				t.Fatal(err)
			}
			code := string(asm)
			if !strings.Contains(code, "_shared_mem") || strings.Contains(code, "sta (") || strings.Contains(code, "lda (") {
				t.Fatalf("alias must access global storage directly:\n%s", code)
			}
			if strings.Contains(code, "_a_work:") || strings.Contains(code, "_a_exported:") {
				t.Fatal("alias allocated storage")
			}
		})
	}
}

func TestStorageAliasErrors(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`var data:[4]uint8; alias a:[5]uint8=data;`, "needs 5 bytes"},
		{`var data:[8]uint8; alias a:[4]uint8=data; alias b:[5]uint8=a;`, "target a has 4 bytes"},
		{`const data=[1,2]; alias a:uint8=data;`, "mutable global storage"},
		{`var data:uint8; alias a:uint8=data; const C=a;`, "cannot read a storage alias"},
		{`var data:uint8; alias a:uint8=data; const C=[a];`, "cannot read a storage alias"},
		{`var data:uint8; alias a:uint8=data; function f(x:uint8=a):void{}`, "constant expression"},
		{`var data:uint8; alias a:[0]uint8=data;`, "storage type"},
		{`function main():void {var data:uint8; alias a:uint8=data;}`, "mutable global storage"},
		{`function f(data:uint8):void {alias a:uint8=data;}`, "mutable global storage"},
		{`var data:[4]uint8; alias a:uint8=data[0];`, "target must name"},
		{`var data:*uint8; alias a:uint8=*data;`, "target must name"},
		{`var data:uint8; alias a:void=data;`, "cannot be void"},
		{`var data:[4]uint8; alias a:[]uint8=data;`, "storage type"},
		{`alias a:uint8=b; alias b:uint8=a;`, "cyclic"},
		{`var data:uint8; function main():void {public alias a:uint8=data;}`, "public alias"},
		{`struct S{x:uint8;} soa data:[2]S; alias a:uint8=data;`, "mutable global storage"},
		{`var data:uint8; block {alias a:uint8=data;} options(bss:"BSS");`, "placement blocks"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			if got := compileErr(t, tc.src); !strings.Contains(got, tc.want) {
				t.Fatalf("got %s; want %s", got, tc.want)
			}
		})
	}
}

func TestStorageAliasVolatile(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"main.fc": `use * from stdio;
var data:uint8 options(address:0x700);
alias value:uint8=data;
function writes():void options(noinline:true) { value=1; value=2; }
function main():void { writes(); printf(value); exit(0); }
`})
	var out strings.Builder
	_, err := NewCompiler(absRepoRoot).BuildContext(t.Context(), "main.fc", &BuildOptions{Dir: dir, BuildDir: filepath.Join(dir, "b"), Out: filepath.Join(dir, "a.bin"), Run: true, Stdout: &out, MaxCycles: 1000000})
	if err != nil {
		t.Fatal(err)
	}
	if out.String() != "2" {
		t.Fatal(out.String())
	}
	asm, err := os.ReadFile(filepath.Join(dir, "b", "_main.s"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(asm), "sta 0+_main_data") != 2 {
		t.Fatalf("volatile writes lost:\n%s", asm)
	}
}
