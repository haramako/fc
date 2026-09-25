package driver

// fc 3 (`#fc 3`) の言語の規則のテスト (doc/v3_plan.md)。fc 2 と fc 3 のモジュールは 1 つのプログラムに混ぜられる。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// buildFiles は files (ファイル名 → ソース) を t.fc から emu でビルドして走らせ、出力とエラーを返す。
func buildFiles(t *testing.T, files map[string]string) (string, error) {
	t.Helper()
	dir := t.TempDir()
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o666); err != nil {
			t.Fatal(err)
		}
	}
	var out strings.Builder
	_, err := NewCompiler(absRepoRoot).Build("t.fc", &BuildOptions{Dir: dir, BuildDir: filepath.Join(dir, "b"), Out: filepath.Join(dir, "a.bin"), Run: true, Stdout: &out, MaxCycles: 10_000_000})
	return out.String(), err
}

// TestV3IntTypes: fc 3 の整数型名は u8 / i8 / u16 / i16 だけ。fc 2 の名前 (int など) は案内つきのエラー、u8 などは宣言できない。
// fc 2 のモジュール (古い名前) と fc 3 のモジュールを混ぜられ、fc 2 でも短い名前を使える。
func TestV3IntTypes(t *testing.T) {
	t.Parallel()
	out, err := buildFiles(t, map[string]string{
		"t.fc": `#fc 3
use * from stdio;
use lib;
function main():void
{
	var a:u8 = 200;
	var b:i8 = -3;
	var c:u16 = 1000;
	var d:i16 = -1000;
	printf(lib.add(a, 55), " ", (b as i16) + d, " ", c + lib.SIZE, "\n");
	exit(0);
}
`,
		"lib.fc": `#fc 2
public const SIZE:int16 = 24;
public function add(x:int, y:u8):int16 { return (x as int16) + y; }
`,
	})
	if err != nil || out != "255 64533 1024\n" {
		t.Errorf("got %q, %v", out, err)
	}
	for _, c := range []struct{ src, msg string }{
		{"#fc 3\nvar a:int;\nfunction main():void { }\n", "int is not a type in fc 3 (write u8; `fcc migrate` rewrites fc 2 sources)"},
		{"#fc 3\nvar a:sint16;\nfunction main():void { }\n", "sint16 is not a type in fc 3 (write i16"},
		{"#fc 3\nfunction main():void { var n = sizeof(uint8); }\n", "uint8 is not a type in fc 3 (write u8"},
		{"#fc 3\nvar u8:u8;\nfunction main():void { }\n", "u8 cannot be declared (it is a type name in fc 3)"},
		{"#fc 3\nfunction main():void { var i16 = 1; }\n", "i16 cannot be declared"},
		{"#fc 3\nfunction u16():void { }\nfunction main():void { }\n", "u16 cannot be declared"},
	} {
		if _, err := buildFiles(t, map[string]string{"t.fc": c.src}); err == nil || !strings.Contains(err.Error(), c.msg) {
			t.Errorf("%q: got %v, want /%s/", c.src, err, c.msg)
		}
	}
}
