package driver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCheckMatchesBuildOptions: fcc check は fcc build と同じ前段 (compileFront) を通るので、main モジュールの
// options(static_zp / static_ram / fastcall_reg) の範囲の検査と、静的フレームの上限の判定が build と一致する。
// 以前は check が既定値を決め打ちで使っていて、build では出るエラーが check では出なかった。
func TestCheckMatchesBuildOptions(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, msg string }{
		{"range", "#fc 2\noptions(static_zp: 300);\nfunction main():void { }\n", "options(static_zp: 300): must be 0..256"},
		{"fastcall_reg", "#fc 2\noptions(fastcall_reg: 8);\nfunction main():void { }\n", "options(fastcall_reg: 8): must be 16..128"},
		// static_zp を 0、static_ram を 1 にすると f のフレーム (13 バイト) が置けない (古い check はこれを見逃していた)
		{"frame", "#fc 2\noptions(static_zp: 0, static_ram: 1);\nfunction f(n:int):int options(noinline: true) { var arr:[8]int; arr[n & 7] = n; return arr[0]; }\nfunction main():void { f(1); }\n", "static frames do not fit"},
	} {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "t.fc"), []byte(c.src), 0o666); err != nil {
			t.Fatal(err)
		}
		_, berr := buildCode(t, NewCompiler(absRepoRoot), "t.fc", &BuildOptions{Dir: dir, BuildDir: filepath.Join(dir, "b"), Out: filepath.Join(dir, "a.bin"), CompileOnly: true}, false, nil, 0)
		_, cerr := NewCompiler(absRepoRoot).Check("t.fc", &CheckOptions{Dir: dir})
		if berr == nil || cerr == nil {
			t.Errorf("%s: build=%v check=%v (どちらもエラーのはず)", c.name, berr, cerr)
			continue
		}
		if berr.Error() != cerr.Error() {
			t.Errorf("%s: build と check のエラーが違う:\n build: %v\n check: %v", c.name, berr, cerr)
		}
		if c.msg != "" && !strings.Contains(cerr.Error(), c.msg) {
			t.Errorf("%s: got %v, want /%s/", c.name, cerr, c.msg)
		}
	}
}
