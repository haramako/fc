package driver

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haramako/fc/internal/nes"
)

func TestFarFunctionPointers(t *testing.T) {
	for _, level := range []int{-1, 2} {
		t.Run(fmtLevel(level), func(t *testing.T) {
			got := runEmuLevel(t, `options(farcall: true);
var callback:farfn(uint8):uint8;
function add(x:uint8):uint8 { return x+7; }
function sub(x:uint8):uint8 options(near: true) { return x-1; }
function change():uint8 { callback = sub; return 3; }
function pass(f:farfn(uint8):uint8):farfn(uint8):uint8 { return f; }
const TABLE:[]farfn(uint8):uint8 = [add,sub,null];
struct Box { f:farfn(uint8):uint8; tag:uint8; }
const BOXES:[]Box = [{add,1},{sub,2}];
function main():void {
 callback = add;
 printf(callback(change()), ",", callback(3), ",");
 var f = pass(add);
 printf(f(5), ",", TABLE[0](6), ",", BOXES[1].f(7), ",");
 f = null;
 printf(f == null, ",", TABLE[2] == null, ",", sizeof(farfn(uint8):uint8), "\n");
 exit(0);
}`, level)
			if strings.TrimSpace(got) != "10,2,12,13,6,1,1,3" {
				t.Fatalf("got %q", got)
			}
		})
	}
}
func fmtLevel(level int) string {
	if level < 0 {
		return "O0"
	}
	return "O2"
}

func TestFarFunctionPointerStorage(t *testing.T) {
	for _, level := range []int{-1, 2} {
		t.Run(fmtLevel(level), func(t *testing.T) {
			source := `options(farcall: true);
function f(x:uint8):uint8 { return x+2; }
const FS:[]farfn(uint8):uint8 = [%s];
var idx:uint8;
var mutable:[90]farfn(uint8):uint8;
struct Box { fnptr:farfn(uint8):uint8; }
soa const CS:[2]Box = [{f},{null}];
soa VS:[2]Box;
var box:Box;
function invoke(p:*farfn(uint8):uint8, x:uint8):uint8 { return (*p)(x); }
function factorial(n:uint8):uint16 {
 if (n == 0) { return 1; }
 var fnptr:farfn(uint8):uint16 = factorial;
 return n * fnptr(n-1);
}
function main():void {
 idx = 85;
 mutable[idx] = FS[idx];
 mutable[89] = f;
 printf(mutable[idx](6), ",", invoke(&mutable[89],7), ",");
 var p:*farfn(uint8):uint8 = FS;
 idx = 89;
 printf(p[idx](8), ",");
 VS[1].fnptr = CS[0].fnptr;
 box.fnptr = VS[1].fnptr;
 printf(box.fnptr(9), ",", CS[1].fnptr == null, ",", factorial(5), "\n");
 exit(0);
}`
			elems := make([]string, 90)
			for i := range elems {
				elems[i] = "f"
			}
			got := runEmuLevel(t, fmt.Sprintf(source, strings.Join(elems, ",")), level)
			if strings.TrimSpace(got) != "8,9,10,11,1,120" {
				t.Fatalf("got %q", got)
			}
		})
	}
}

func TestFarFunctionPointerErrors(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"disabled", `function f():void{} function main():void { var p:farfn():void=f; p(); }`, "farfn calls require options(farcall: true)"},
		{"near variable", `function f():void{} function main():void { var n:fn():void=f; var p:farfn():void=n; }`, "not compatible types"},
		{"near cast", `function main():void { var n:fn():void; var p=bitcast<farfn():void>(n); }`, "sizes differ"},
		{"far to near", `function main():void { var p:farfn():void; var n:fn():void=p; }`, "not compatible types"},
		{"void pointer", `function main():void { var p:farfn():void; var n:*void=p; }`, "not compatible types"},
		{"integer", `function main():void { var p=bitcast<farfn():void>(123); }`, "cannot construct farfn from an integer"},
		{"typed null cast", `function main():void { var n=bitcast<uint16>(null as farfn():void); }`, "sizes differ"},
		{"constant arithmetic", `const Z:farfn():void=null; const N=Z+Z;`, "arithmetic is not supported on farfn"},
		{"constant order", `const Z:farfn():void=null; const N=Z<Z;`, "ordered comparison is not supported on farfn"},
		{"signature", `function f(x:uint8):void{} function main():void { var p:farfn():void=f; }`, "not compatible types"},
		{"fastcall", `function f():void options(fastcall:true){} function main():void { var p:farfn():void=f; }`, "not compatible types"},
		{"cc65", `function f():void options(abi:"cc65"); function main():void { var p:farfn():void=f; }`, "cannot take the address of a cc65 abi function"},
		{"arithmetic", `function main():void { var p:farfn():void; var q=p+p; }`, "arithmetic is not supported on farfn"},
		{"order", `function main():void { var p:farfn():void; var q=p<p; }`, "ordered comparison is not supported on farfn"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := compileErr(t, tc.src); !strings.Contains(got, tc.want) {
				t.Fatalf("got %q; want %q", got, tc.want)
			}
		})
	}
}

// This uses real MMC3 PRG mappings: a.add and b.add have identical CPU addresses.
func TestFarFunctionPointerMMC3(t *testing.T) {
	for _, level := range []int{-1, 2} {
		t.Run(fmtLevel(level), func(t *testing.T) {
			dir := t.TempDir()
			trampoline, err := os.ReadFile(filepath.Join(absRepoRoot, "fclib", "nes", "farcall_mmc3.asm"))
			if err != nil {
				t.Fatal(err)
			}
			writeFiles(t, dir, map[string]string{
				"main.fc": `#fc 2
options(bank:3, bank_count:4, mapper:"MMC3", farcall:true);
use nes;
use mmc3;
use a;
use b;
use slot1;
var result:[16]uint8 options(address:0x700);
var callback:farfn(uint8):uint8;
const TABLE:[]farfn(uint8):uint8 = [a.add,b.add,slot1.add,fixed];
function irq():void options(symbol:"_interrupt_irq") {}
function nmi():void options(symbol:"_interrupt") {}
function fixed(x:uint8):uint8 { return x+20; }
function same(x:farfn(uint8):uint8,y:farfn(uint8):uint8):uint8 options(noinline:true) { return x==y; }
function main():void {
 mmc3.set(0,2);
 mmc3.set(1,0);
 for (var i:uint8=0; i<4; i++) { result[i]=TABLE[i](1); }
 callback=a.add;
 result[4]=callback(TABLE[1](2));
 result[5]=a.nested(3);
 result[6]=mmc3.pbank_bak[0];
 result[7]=mmc3.pbank_bak[1];
 result[8]=same(a.add,b.add);
 result[9]=same(a.add,a.add);
 result[10]=1;
}
`,
				"a.fc": `#fc 2
options(bank:0, org:0x8000);
use b;
use mmc3;
public function add(x:uint8):uint8 options(near:true, noinline:true) { return x+5; }
public function nested(x:uint8):uint8 {
 var f:farfn(uint8):uint8=add;
 var same_bank=f(x);
 f=b.add;
 var other_bank=f(x);
 return same_bank+other_bank+mmc3.pbank_bak[0];
}
`,
				"b.fc": `#fc 2
options(bank:1,org:0x8000);
public function add(x:uint8):uint8 options(near:true,noinline:true) { return x+9; }
`,
				"slot1.fc": `#fc 2
options(bank:2,org:0xa000);
public function add(x:uint8):uint8 { return x+13; }
`,
				"mmc3.fc": `#fc 2
options(bank:3);
public var pbank_bak:[2]uint8;
var select:uint8 options(address:0x8000);
var data:uint8 options(address:0x8001);
public function set(slot:uint8,bank:uint8):void { pbank_bak[slot]=bank; select=6+slot; data=bank; }
include("trampoline.asm");
`,
				"trampoline.asm": string(trampoline) + "\n.global _a_add, _b_add\n.assert _a_add = _b_add, lderror, \"test requires identical CPU addresses\"\n",
			})
			rom := filepath.Join(dir, "test.nes")
			res, err := NewCompiler(absRepoRoot).BuildContext(t.Context(), "main.fc", &BuildOptions{Target: "nes", Dir: dir, BuildDir: filepath.Join(dir, "b"), Out: rom, OptimizeLevel: level})
			if err != nil {
				t.Fatal(err)
			}
			hasIndirect := false
			for _, call := range res.FarCalls {
				if strings.HasPrefix(call.Callee, "indirect farfn(") {
					hasIndirect = true
				}
			}
			if !hasIndirect {
				t.Fatal("farfn calls missing from far-call report")
			}
			machine, err := nes.LoadFile(rom)
			if err != nil {
				t.Fatal(err)
			}
			if err = machine.RunFrames(10); err != nil {
				t.Fatal(err)
			}
			want := []int{6, 10, 14, 21, 16, 20, 2, 0, 0, 1, 1}
			for i, v := range want {
				if got := machine.Get(0x700 + i); got != v {
					t.Errorf("result[%d]=%d; want %d", i, got, v)
				}
			}
			if off := machine.RomOffset(0x8000); off != 16+2*0x2000 {
				t.Errorf("slot 0 mapping: %x", off)
			}
			if off := machine.RomOffset(0xa000); off != 16 {
				t.Errorf("slot 1 mapping: %x", off)
			}
		})
	}
}

func TestFarFunctionPointerBankRange(t *testing.T) {
	// Include a far alias that is never called: its representation must still be valid.
	for _, storage := range []string{
		"const POINTER:farfn():void=f;",
		"const POINTER:[]farfn():void=[f];",
		"struct Box { f:farfn():void; } soa const POINTER:[1]Box=[{f}];",
	} {
		t.Run(storage, func(t *testing.T) {
			dir := t.TempDir()
			source := "#fc 2\nuse * from stdio;\n" + storage + "\nfunction f():void{}\nfunction main():void{exit(0);}\n"
			writeFiles(t, dir, map[string]string{"main.fc": source})
			opt := &BuildOptions{Dir: dir, BuildDir: filepath.Join(dir, "b"), Out: filepath.Join(dir, "a.bin")}
			if _, err := NewCompiler(absRepoRoot).Build("main.fc", opt); err != nil {
				t.Fatal(err)
			}
			cfg, err := os.ReadFile(filepath.Join(dir, "b", "ld65.cfg"))
			if err != nil {
				t.Fatal(err)
			}
			source = strings.Replace(source, "#fc 2", "#fc 2\noptions(linker_config: \"custom.cfg\");", 1)
			writeFiles(t, dir, map[string]string{"main.fc": source})
			for _, bank := range []int{255, 256} {
				writeFiles(t, dir, map[string]string{"custom.cfg": strings.Replace(string(cfg), "bank = 0;", fmt.Sprintf("bank = %d;", bank), 1)})
				_, err = NewCompiler(absRepoRoot).Build("main.fc", opt)
				if bank == 255 && err != nil {
					t.Fatal(err)
				}
				if bank == 256 && (err == nil || !strings.Contains(err.Error(), "farfn bank must be in 0..255")) {
					t.Fatalf("bank=256: %v", err)
				}
			}
		})
	}
}
