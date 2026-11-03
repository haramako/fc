package driver

import (
	"strings"
	"testing"
)

// TestModuleNotFound: use のモジュールが見つからないエラーは、探す所と、別のフォルダのモジュールを使う方法 (fc.toml の [lib.*]) を書く
// (隣のフォルダに置いたモジュールで「file tiles.fc not found」だけだった)。[lib.*] の path を書けば隣のフォルダのモジュールを使える。
func TestModuleNotFound(t *testing.T) {
	t.Parallel()
	main := "#fc 4\nuse tiles;\nfunction main():void\n{\n\t@printf(\"{}\\n\", tiles.N);\n}\n"
	tiles := "#fc 4\npublic const N = 3;\n"
	r := testBuild(t, buildSpec{Files: map[string]string{"src/t.fc": main, "gfx/tiles.fc": tiles}, Main: "src/t.fc", CompileOnly: true})
	if r.Err == nil || !strings.Contains(r.Err.Error(), "module tiles not found") || !strings.Contains(r.Err.Error(), "`[lib.NAME]` with `path = \"folder\"`") {
		t.Errorf("err = %v", r.Err)
	}
	r = testBuild(t, buildSpec{Files: map[string]string{"src/t.fc": main, "gfx/tiles.fc": tiles, "fc.toml": "[lib.gfx]\npath = \"gfx\"\n"}, Main: "src/t.fc", Run: true})
	if r.Err != nil || r.Stdout != "3\n" {
		t.Errorf("[lib.gfx]: err = %v, out = %q", r.Err, r.Stdout)
	}
}
