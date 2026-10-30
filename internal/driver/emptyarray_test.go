package driver

import (
	"strings"
	"testing"
)

// TestEmptyArrayLiteral: 宣言・変数・引数の型があれば空の `[]` は長さ 0 の配列 (要素の型は宣言の型から。GitHub の Issue #22)。
// 長さ 0 の配列を for-each で回しても「いつも同じ結果」の比較の警告を出さない。型の分からない `[]` はエラーのまま。
func TestEmptyArrayLiteral(t *testing.T) {
	t.Parallel()
	src := `#fc 4
struct Belt {
	col:u16;
	v:i8;
}
const NO_BELTS:[?]Belt = [];
const NONE:[0]u8 = [];
const SOME:[?]Belt = [{1, 2}];
const TABLE:[2][]const Belt = [NO_BELTS, SOME];
function count(s:[]const u8):u8 { return @len(s); }
function main():void
{
	var e:[]const u8 = [];
	var k:u8 = 0;
	for (var b in NO_BELTS) {
		k += 1;
	}
	@printf("{} {} {} {} {} {}\n", @len(NO_BELTS), @len(NONE), @len(TABLE[0]), @len(TABLE[1]), count(e) + count([]), k);
}
`
	for _, level := range []int{0, -1} {
		r := testBuild(t, buildSpec{Files: map[string]string{"t.fc": src}, Level: level, Run: true})
		if r.Err != nil {
			t.Fatal(r.Err)
		}
		if r.Stdout != "0 0 0 1 0 0\n" {
			t.Errorf("-O %d: got %q", level, r.Stdout)
		}
		if len(r.Res.Warnings) != 0 {
			t.Errorf("-O %d: 警告が出た: %v", level, r.Res.Warnings)
		}
	}
	r := testBuild(t, buildSpec{Files: map[string]string{"t.fc": "#fc 4\nfunction main():void\n{\n\tvar a = [];\n}\n"}, CompileOnly: true})
	if r.Err == nil || !strings.Contains(r.Err.Error(), "cannot infer the element type of an empty array literal") {
		t.Errorf("型の無い []: %v", r.Err)
	}
}
