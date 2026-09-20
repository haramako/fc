package driver

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBssPlacementLinkedAndExecuted(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"t.fc": `
use * from stdio;
use dep;
var early:int;
block {
 public var grouped:int;
 block {var inner:int;} options(bss:"BSS_EX");
 var override:int options(segment:"BSS_EX");
} options(bss:"BSS");
struct Item {x:int;}
soa Items:[2]Item;
options(bss:"BSS_EX");
function main():void options(segment:"CODE") {
 early=3;grouped=4;inner=5;override=6;Items[1].x=7;dep.value=8;
 printf(early+grouped+inner+override+Items[1].x+dep.value);exit(0);
}
`,
		"dep.fc": `public var value:int;`,
	}
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0600); err != nil {
			t.Fatal(err)
		}
	}
	var output strings.Builder
	result, err := NewCompiler(absRepoRoot).BuildContext(context.Background(), "t.fc", &BuildOptions{
		Target: "emu", Dir: dir, BuildDir: filepath.Join(dir, "build"), Out: filepath.Join(dir, "test.bin"), Run: true, Stdout: &output,
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(output.String()) != "33" {
		t.Fatalf("output=%q", output.String())
	}
	dbg, err := ParseDbgFile(result.DbgFile)
	if err != nil {
		t.Fatal(err)
	}
	expected := map[string]string{"_t_early": "BSS_EX", "_t_grouped": "BSS", "_t_inner": "BSS_EX", "_t_override": "BSS_EX", "_t_Items_x": "BSS_EX", "_dep_value": "BSS", "_main": "CODE"}
	for _, s := range dbg.Symbols {
		if want, ok := expected[s.Name]; ok && s.Lab {
			seg := dbg.Segments[s.Seg]
			if seg == nil || seg.Name != want {
				t.Errorf("%s segment=%v want %s", s.Name, seg, want)
			}
			delete(expected, s.Name)
		}
	}
	if len(expected) > 0 {
		t.Fatalf("missing linked symbols: %v", expected)
	}
}
