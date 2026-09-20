package sema

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haramako/fc/internal/ir"
)

func TestBssPlacementPrecedenceAndModuleIsolation(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"t.fc": `
use dep;
use plain;
var early:int;
struct Item {x:int; y:int;}
soa Items:[2]Item;
block {
 public var grouped:int;
 var pinned:int options(segment:"PINNED");
 var legacy:int options(segment:"");
 var device:int options(address:0x2000);
 public soa GroupItems:[2]Item;
 block { public var nested:int; } options(bss:"INNER");
 var afterNested:int;
} options(bss:"OUTER");
var afterGroup:int;
const TABLE:[2]int=[1,2];
options(bss:"DEFAULT");
function main():void {var local:int;local=1;grouped=local+dep.exported+plain.exported;}
`,
		"dep.fc":   `options(bss:"DEPENDENCY"); block {public var exported:int;} options(bss:"DEP_GROUP"); var other:int;`,
		"plain.fc": `public var exported:int;`,
	}
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0600); err != nil {
			t.Fatal(err)
		}
	}
	p, err := Compile(dir, []string{"."}, "t.fc")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := p.Options.Get("bss"); ok {
		t.Fatal("module bss leaked to Program.Options")
	}
	want := map[string]string{
		"_t_early": "DEFAULT", "_t_Items_x": "DEFAULT", "_t_Items_y": "DEFAULT",
		"_t_grouped": "OUTER", "_t_pinned": "PINNED", "_t_legacy": "BSS",
		"_t_GroupItems_x": "OUTER", "_t_GroupItems_y": "OUTER",
		"_t_nested": "INNER", "_t_afterNested": "OUTER", "_t_afterGroup": "DEFAULT",
		"_dep_exported": "DEP_GROUP", "_dep_other": "DEPENDENCY", "_plain_exported": "",
	}
	for _, m := range p.Modules.List() {
		for _, d := range m.Defs {
			if d.Kind == ir.DefBss {
				expected, ok := want[d.Sym]
				if !ok {
					t.Errorf("unexpected BSS allocation: %s", d.Sym)
					continue
				}
				if d.Segment != expected {
					t.Errorf("%s segment=%q want %q", d.Sym, d.Segment, expected)
				}
				delete(want, d.Sym)
			}
			if d.Sym == "_t_device" && d.Kind != ir.DefEqu {
				t.Error("fixed-address variable allocated storage")
			}
			if d.Sym == "_t_TABLE" && d.Kind != ir.DefBlock {
				t.Error("const changed storage kind")
			}
		}
	}
	if len(want) > 0 {
		t.Errorf("missing allocations: %v", want)
	}
}

func TestBssPlacementErrors(t *testing.T) {
	for _, tt := range []struct{ src, want string }{
		{`options(bss:123);`, "non-empty segment name string"},
		{`options(bss:"");`, "non-empty segment name string"},
		{`options(bss:"BSS\nBAD");`, "control characters"},
		{`block {var a:int;} options(segment:"RAM");`, "only accept options(bss"},

		{`block {var a:int;} options(bss:1);`, "non-empty segment name string"},
		{`block {function f():void {}} options(bss:"RAM");`, "may only contain"},
		{`block {const a=1;} options(bss:"RAM");`, "may only contain"},
		{`block {options(bss:"INNER");} options(bss:"RAM");`, "may only contain"},
		{`function f():void {block {var a:int;} options(bss:"RAM");}`, "must be at module level"},
		{`var a:int options(bss:"RAM");`, "bss is only allowed"},
		{`function f():void options(bss:"RAM") {}`, "bss is only allowed"},
	} {
		t.Run(tt.src, func(t *testing.T) {
			_, err := compileSrc(t, tt.src)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error=%v want %q", err, tt.want)
			}
		})
	}
}
