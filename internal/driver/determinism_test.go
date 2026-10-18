package driver

// ビルドの決定性 (bug hunt 2026-10-18 の Phase 4)。ROM の golden は「1 つの設定で毎回同じ」を見るだけなので、ca65 の並列数、
// 同じビルドディレクトリでの作り直し (asm のキャッシュ)、ソースを変えて戻したとき、相対と絶対の基準ディレクトリで、
// ROM のバイトが同じになることをここで見る。

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

// buildROM は dir の main を opt で ROM にしてバイトを返す。
func buildROM(t *testing.T, main string, opt *BuildOptions) []byte {
	t.Helper()
	if _, err := NewCompiler(absRepoRoot).BuildContext(context.Background(), main, opt); err != nil {
		t.Fatalf("ビルド失敗 (%+v): %v", *opt, err)
	}
	rom, err := os.ReadFile(opt.Out)
	if err != nil {
		t.Fatal(err)
	}
	return rom
}

// TestBuildDeterministicCastle は castle (いちばん大きい実プロジェクト) を、並列数・ビルドディレクトリ・基準ディレクトリの
// 書き方を変えてビルドし、ROM が全部同じであること。
func TestBuildDeterministicCastle(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}
	t.Parallel()
	tmp := t.TempDir()
	dir := filepath.Join(tmp, "castle")
	copyDir(t, filepath.Join(absRepoRoot, "examples", "castle"), dir)
	src := filepath.Join(dir, "src")
	opt := func(name string, jobs int, buildDir string) *BuildOptions {
		return &BuildOptions{Target: "nes", Dir: src, Out: filepath.Join(tmp, name+".nes"), Jobs: jobs, BuildDir: buildDir}
	}
	base := buildROM(t, "main.fc", opt("j1", 1, filepath.Join(tmp, "b1")))
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	rel, err := filepath.Rel(cwd, src)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name string
		opt  *BuildOptions
	}{
		{"並列 16", opt("j16", 16, filepath.Join(tmp, "b16"))},
		{"同じビルドディレクトリで作り直す (キャッシュ)", opt("again", 1, filepath.Join(tmp, "b1"))},
		{"並列の既定 (CPU 数)、既定のビルドディレクトリ", opt("default", 0, "")},
		{"基準ディレクトリが相対", &BuildOptions{Target: "nes", Dir: rel, Out: filepath.Join(tmp, "rel.nes"), BuildDir: filepath.Join(tmp, "brel")}},
	} {
		if got := buildROM(t, "main.fc", c.opt); !bytes.Equal(got, base) {
			t.Errorf("%s: ROM が並列 1 のビルドと違う", c.name)
		}
	}
}

// TestBuildDeterministicEdit は、ソースを変えてビルドし、元に戻して同じビルドディレクトリでビルドしたとき、最初と同じ ROM に
// なること (古いオブジェクトやキャッシュが残らない)。変えたときに ROM が変わることも見る (比べたことの確かめ)。
func TestBuildDeterministicEdit(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	files := func(k string) map[string]string {
		return map[string]string{
			"main.fc": "#fc 4\nuse console;\nuse sub;\nfunction main():void\n{\n\t@printf(\"{}\\n\", sub.f());\n\tconsole.exit(0);\n}\n",
			"sub.fc":  "#fc 4\npublic function f():u8 @(noinline) { return " + k + "; }\n",
		}
	}
	opt := func(name string) *BuildOptions {
		return &BuildOptions{Dir: dir, Out: filepath.Join(dir, name+".bin"), BuildDir: filepath.Join(dir, "b")}
	}
	writeFiles(t, dir, files("1"))
	first := buildROM(t, "main.fc", opt("first"))
	writeFiles(t, dir, files("2"))
	changed := buildROM(t, "main.fc", opt("changed"))
	if bytes.Equal(changed, first) {
		t.Fatalf("ソースを変えても ROM が変わらない (比べられていない)")
	}
	writeFiles(t, dir, files("1"))
	if back := buildROM(t, "main.fc", opt("back")); !bytes.Equal(back, first) {
		t.Errorf("元に戻したソースの ROM が最初と違う (古いオブジェクトかキャッシュが残っている)")
	}
}
