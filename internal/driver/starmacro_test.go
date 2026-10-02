package driver

import (
	"strings"
	"testing"
)

// TestStarlarkMacros: fc.toml の [macro_script.*] の Starlark の定数マクロ (internal/starmacro)。型付きの配列 (math を使う)・
// バイト列・文字列・整数の結果、read / glob で読むファイル、load、エラー (スクリプトの誤り・プロジェクトの外・組み込みと同じ名前)。
func TestStarlarkMacros(t *testing.T) {
	t.Parallel()
	files := map[string]string{
		"fc.toml": "[macro_script.tables]\nfile = \"tools/tables.star\"\n",
		"tools/tables.star": `load("util.star", "clamp")

def sin_table(n, amp):
    return fc.array("i8", [clamp(int(math.round(amp * math.sin(2 * math.pi * i / n))), -128, 127) for i in range(n)])

def rev(b):
    return bytes(reversed(list(b.elems())))

def concat(pattern):
    out = []
    for f in glob(pattern):
        out += list(read(f).elems())
    return bytes(out)

def nfiles(pattern):
    return len(glob(pattern))

def big(n):
    return fc.int("u16", n * 1000)

def name():
    return "fc" + str(len(read("res/a.txt")))

def _private():
    return 1
`,
		"tools/util.star": "def clamp(x, lo, hi):\n    return max(lo, min(hi, x))\n",
		"res/a.txt":       "ab",
		"res/b.txt":       "cde",
		"t.fc": `#fc 4
use console;
const S = @sin_table(8, 100);
const D:[3]u8 = [1, 2, 3];
const R = @rev(D);
const C = @concat("res/*.txt");
const N = @nfiles("res/*.txt");
const B = @big(40);
const M = @name();
function main():void
{
	@printf("{} {} {} {} {} {}\n", S[2], S[6], R[0], @len(C), C[4], N);
	@printf("{} {} {}\n", B, @sizeof(B), M);
	console.exit(0);
}
`}
	out, err := buildBothLevels(t, files)
	if want := "100 -100 3 5 101 2\n40000 2 fc2\n"; err != nil || out != want {
		t.Errorf("got %q, %v, want %q", out, err, want)
	}
	for _, c := range []struct{ star, src, msg string }{
		{"def f():\n    return 1 / 0\n", "const X = @f();", "division by zero"},
		{"def f():\n    return read(\"../x.txt\")\n", "const X = @f();", "outside the project"},
		{"def f():\n    return [1, 300]\n", "const X = @f();", "write fc.array"},
		{"def f():\n    return fc.array(\"i8\", [200])\n", "const X = @f();", "does not fit in i8"},
		{"def f():\n    return None\n", "const X = @f();", "the result must be"},
		{"def lz4():\n    return 1\n", "", "same name as a built-in"},
		{"def f(\n", "", "tools/tables.star"},
		{"def _f():\n    return 1\n", "const X = @_f();", "@_f not found"},
	} {
		fs := map[string]string{"fc.toml": files["fc.toml"], "tools/tables.star": c.star, "t.fc": "#fc 4\n" + c.src + "\nfunction main():void {}\n"}
		if _, err := buildFiles(t, fs); err == nil || !strings.Contains(err.Error(), c.msg) {
			t.Errorf("%q: got %v, want /%s/", c.star, err, c.msg)
		}
	}
}
