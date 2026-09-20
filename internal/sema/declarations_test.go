package sema

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haramako/fc/internal/ir"
)

func compileModules(t *testing.T, files map[string]string) (*Program, error) {
	t.Helper()
	dir := t.TempDir()
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return Compile(dir, []string{"."}, "t.fc")
}
func TestDeclarationOrderConstantsAndSignatures(t *testing.T) {
	p, err := compileModules(t, map[string]string{"t.fc": `
const TABLE = [later, earlier];
const TOTAL = COUNT * sizeof(Item) + EXTRA;
var grid:[ROWS][COUNT]Item;
function main():void { TABLE[0](); }
function later():void { earlier(); }
function earlier():void {}
function accept(p:*[COUNT]Item):void {}
struct Item { value:Word; }
struct Word { lo:uint8; hi:uint8; }
const EXTRA=3, COUNT=BASE+1, ROWS=2, BASE=3;
`})
	if err != nil {
		t.Fatal(err)
	}
	m, _ := p.Modules.Get("t")
	if v := m.Scope.FindMust("TOTAL", true); v.Int != 11 {
		t.Fatalf("TOTAL=%v", v)
	}
	if v := m.Scope.FindMust("grid", true); v.Type.Size != 16 {
		t.Fatalf("grid size=%d", v.Type.Size)
	}
	if v := m.Scope.FindMust("accept", true); v.Type.Params[0].Base.Size != 8 {
		t.Fatalf("signature=%s", v.Type)
	}
	var table *ir.Def
	for _, d := range m.Defs {
		if d.Sym == "_t_TABLE" {
			table = d
		}
	}
	if table == nil || len(table.Elems) != 2 || table.Elems[0].(*ir.Value).Symbol != "_t_later" {
		t.Fatalf("bad function table: %+v", table)
	}
}
func TestDeclarationOrderRecursiveTypes(t *testing.T) {
	p, err := compileModules(t, map[string]string{"t.fc": `
var a:A;
struct A { next:*B; }
struct B { owner:A; count:uint8; }
const SIZE=sizeof(B);
function main():void {}
`})
	if err != nil {
		t.Fatal(err)
	}
	m, _ := p.Modules.Get("t")
	if m.Scope.FindMust("SIZE", true).Int != 3 {
		t.Fatal("recursive layout was not completed")
	}
}
func TestDeclarationOrderImports(t *testing.T) {
	for _, main := range []string{
		`public const BASE=3; const N=dep.COUNT; use dep;`,
		`public const BASE=3; const N=COUNT; use COUNT from dep;`,
		`public const BASE=3; const N=COUNT; use * from dep;`,
	} {
		t.Run(main, func(t *testing.T) {
			p, err := compileModules(t, map[string]string{
				"t.fc":   main + ` function main():void {}`,
				"dep.fc": `public const COUNT=BASE+2; use BASE from t;`,
			})
			if err != nil {
				t.Fatal(err)
			}
			m, _ := p.Modules.Get("t")
			if m.Scope.FindMust("N", true).Int != 5 {
				t.Fatal("wrong imported constant")
			}
		})
	}
	// The same name can be resolved again in a module via a different dependency.
	p, err := compileModules(t, map[string]string{
		"t.fc":      `const N=value; use * from facade; function main():void {}`,
		"facade.fc": `public use value from dep;`,
		"dep.fc":    `public const value=base+1; use base from t2;`,
		"t2.fc":     `public const base=4; use dep;`,
	})
	if err != nil {
		t.Fatal(err)
	}
	m, _ := p.Modules.Get("t")
	if m.Scope.FindMust("N", true).Int != 5 {
		t.Fatal("wrong reexport")
	}
}
func TestDeclarationOrderCrossModuleTypes(t *testing.T) {
	p, err := compileModules(t, map[string]string{
		"t.fc":   `public struct A { b:*dep.B; } var a:A; use dep; const SIZE=sizeof(dep.B); function main():void {}`,
		"dep.fc": `public struct B { a:t.A; n:uint8; } use t;`,
	})
	if err != nil {
		t.Fatal(err)
	}
	m, _ := p.Modules.Get("t")
	if m.Scope.FindMust("SIZE", true).Int != 3 {
		t.Fatal("wrong cross-module layout")
	}
}
func TestDeclarationOrderPlacement(t *testing.T) {
	p, err := compileModules(t, map[string]string{"t.fc": `
const SIZE=sizeof(items);
block { var items:[COUNT]Item; soa Points:[COUNT]Item; } options(bss:"GROUP");
var handle:*Points;
struct Item { x:uint8; }
const COUNT=4;
options(bss:"DEFAULT");
function main():void { handle=&Points[0]; handle.x=1; }
`})
	if err != nil {
		t.Fatal(err)
	}
	m, _ := p.Modules.Get("t")
	for _, d := range m.Defs {
		if d.Kind == ir.DefBss {
			want := "GROUP"
			if d.Sym == "_t_handle" {
				want = "DEFAULT"
			}
			if d.Segment != want {
				t.Errorf("%s segment=%s want=%s", d.Sym, d.Segment, want)
			}
		}
	}
}
func TestDeclarationOrderErrors(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`const A=B+1; const B=A+1;`, "cyclic declaration dependency: t.A -> t.B -> t.A"},
		{`const N=sizeof(Item); struct Item { data:[N]uint8; }`, "cyclic declaration dependency"},
		{`struct A { b:B; } struct B { a:A; }`, "cyclic declaration dependency"},
		{`const A=1; const A=2;`, "A already defined"},
		{`struct A {} struct A {x:uint8;}`, "A already defined"},
		{`const N=missing+1;`, "missing not found"},
		{`function main():void { x=1; var x:uint8; }`, "x not found"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			_, err := compileSrc(t, tc.src)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error=%v, want %q", err, tc.want)
			}
		})
	}
	_, err := compileModules(t, map[string]string{
		"t.fc":   `public const A=dep.B+1; use dep;`,
		"dep.fc": `public const B=t.A+1; use t;`,
	})
	if err == nil || !strings.Contains(err.Error(), "cyclic declaration dependency") {
		t.Fatalf("cross-module cycle: %v", err)
	}
}

func TestDeclarationOrderRecursiveArraysAndSoa(t *testing.T) {
	for _, src := range []string{
		`struct Node { next:*[2]Node; n:uint8; } const SIZE=sizeof([2]Node);`,
		`const SIZE=sizeof([2]Node); struct Node { next:*[2]Node; n:uint8; }`,
		`struct Node { callback:fn([2]Node):void; n:uint8; } const SIZE=sizeof([2]Node);`,
	} {
		p, err := compileModules(t, map[string]string{"t.fc": src})
		if err != nil {
			t.Fatal(err)
		}
		m, _ := p.Modules.Get("t")
		if m.Scope.FindMust("SIZE", true).Int != 6 {
			t.Fatal("stale array size")
		}
	}
	for _, src := range []string{
		`struct Node {next:*Nodes; n:uint8;} soa Nodes:[COUNT]Node; const COUNT=4;`,
		`soa Nodes:[COUNT]Node; struct Node {next:*Nodes; n:uint8;} const COUNT=4;`,
	} {
		p, err := compileModules(t, map[string]string{"t.fc": src + ` const SIZE=sizeof(Node); function main():void { var n:*Nodes=&Nodes[0]; n.next=&Nodes[1]; n.next.n=7; }`})
		if err != nil {
			t.Fatal(err)
		}
		m, _ := p.Modules.Get("t")
		if m.Scope.FindMust("SIZE", true).Int != 2 {
			t.Fatal("wrong SoA handle size")
		}
	}
}

func TestDeclarationOrderKeepsEnumerationOrder(t *testing.T) {
	p, err := compileModules(t, map[string]string{"t.fc": `
public const TABLE=[last,first];
public function first():void {}
public struct Item {x:uint8;}
public function last():void {}
`})
	if err != nil {
		t.Fatal(err)
	}
	m, _ := p.Modules.Get("t")
	if got := strings.Join(m.Interface().Exports(), ","); got != "TABLE,first,Item,last" {
		t.Fatalf("exports: %s", got)
	}
}
func TestDeclarationOrderInvalidSoaDoesNotPanic(t *testing.T) {
	_, err := compileSrc(t, `soa Nodes:[0]Node; struct Node {next:*Nodes; n:uint8;} function main():void {var n:*Nodes=&Nodes[0]; n.next.n=7;}`)
	if err == nil || !strings.Contains(err.Error(), "length must be 1..256") {
		t.Fatalf("error: %v", err)
	}
}
