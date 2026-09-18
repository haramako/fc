package driver

// soa コンテナ (doc/v2_types_struct.md §4.5) のテスト。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestV2SoaProgram は test/test_soa.fc (unittest 形式) を emu で実行する。
func TestV2SoaProgram(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	var out strings.Builder
	code, err := NewCompiler(absRepoRoot).Build("test_soa.fc", &BuildOptions{
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

func TestSoaModules(t *testing.T) {
	t.Parallel()
	// 他モジュールの soa: mod.Name[i]、*mod.Name、use Name from mod、options(segment)
	dir := t.TempDir()
	files := map[string]string{
		"en.fc": "#fc 2\npublic struct Enemy { x:int; hp:int16; }\npublic soa Enemies:[4]Enemy options(segment: \"BSS\");\n" +
			"public function hit(e:*Enemies):void { e.hp -= 10; }\n",
		"t.fc": `#fc 2
use * from stdio;
use en;
use Enemies from en;
function main():void
{
	var e:*en.Enemies = &en.Enemies[1];
	e.x = 5;
	e.hp = 100;
	en.hit(e);
	var f:*Enemies = &Enemies[1];
	printf("x=", f.x, " hp=", f.hp, " same=", e == f, "\n");
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
	if code != 0 || out.String() != "x=5 hp=90 same=1\n" {
		t.Errorf("code=%d out=%q", code, out.String())
	}
}

func TestSoaErrors(t *testing.T) {
	t.Parallel()
	pre := "struct P { x:int; y:int16; a:int; }\nsoa Ps:[4]P;\nsoa const Cs:[2]P = [{1, 2, 3}, {4, 5, 6}];\n"
	cases := []struct{ src, want string }{
		{"struct Q { a:[2]int; }\nsoa Qs:[4]Q;\n", "array field a is not allowed"},
		{"struct Q { a:int; }\nsoa Qs:[300]Q;\n", "length must be 1..256"},
		{"soa Qs:[4]int;\n", "type must be [N]Struct"},
		{pre + "function main():void { Cs[0].x = 1; }\n", "cannot assign to element of soa const"},
		{pre + "function main():void { Cs[0].y = 1; }\n", "cannot assign to element of soa const"},
		{pre + "function main():void { var q:P; Cs[0] = q; }\n", "cannot assign to element of soa const"},
		{pre + "function main():void { var i:int16 = 1; Ps[i].x = 1; }\n", "index must be 1 byte"},
		{pre + "function main():void { var v = Ps; }\n", "can only be indexed"},
		{pre + "function main():void { var p:*Ps; var q:*P = bitcast<*P>(p); }\n", "sizes differ"},
		{pre + "function main():void { var p:*Ps; p.z = 1; }\n", "has no field z"},
		{pre + "function main():void { var p = &Ps[0].y; }\n", "cannot take the address of soa field y"},
	}
	for _, c := range cases {
		got := compileErr(t, c.src)
		if !strings.Contains(got, c.want) {
			t.Errorf("%q:\n  got  %q\n  want /%s/", c.src, got, c.want)
		}
	}
}
