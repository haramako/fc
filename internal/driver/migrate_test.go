package driver

// fc 2 → fc 3 の migrate の回帰テスト。castle / miku / bench / golden のテストプログラムと fclib のソースを
// `fcc migrate` (internal/migrate) で fc 3 に書き換えてビルドし、fc 2 のままのビルドと ROM・バイナリがバイト単位で
// 一致することを確かめる。fc 3 で文法を変えるときは、migrate の規則を足してここが通ることを確かめる
// (doc/v3_plan.md の「V2 からの移行」)。

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haramako/fc/internal/migrate"
	"github.com/haramako/fc/internal/syntax"
)

// migrateTree は dir の下の .fc を全部 fc 3 に書き換える (書き換えた数を返す)。fc 2 として解析できないファイル
// (test/errors.fc のようなエラーの検査用) はそのまま残す。
func migrateTree(tb testing.TB, dir string) int {
	tb.Helper()
	n := 0
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".fc") {
			return err
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		src = bytes.ReplaceAll(src, []byte("\r\n"), []byte("\n"))
		if _, perr := syntax.Parse(src, path); perr != nil {
			return nil
		}
		out, err := migrate.Migrate(src, path)
		if err != nil {
			return err
		}
		f, err := syntax.Parse(out, path)
		if err != nil || f.Version != syntax.Version3 {
			tb.Fatalf("%s: migrate の結果が fc 3 でない: %v", path, err)
		}
		n++
		return os.WriteFile(path, out, 0o666)
	})
	if err != nil {
		tb.Fatalf("migrate: %v", err)
	}
	return n
}

// migratedHome は fclib を fc 3 に書き換えた FC_HOME (fclib と share の写し) を t の一時ディレクトリに作る。
func migratedHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	copyDir(t, filepath.Join(absRepoRoot, "fclib"), filepath.Join(dir, "fclib"))
	copyDir(t, filepath.Join(absRepoRoot, "share"), filepath.Join(dir, "share"))
	if migrateTree(t, filepath.Join(dir, "fclib")) == 0 {
		t.Fatal("fclib に .fc が無い")
	}
	return dir
}

// sameBytes は got と want (ファイル) がバイト単位で一致するか。
func sameBytes(t *testing.T, gotPath, wantPath string) {
	t.Helper()
	got, err := os.ReadFile(gotPath)
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		i := 0
		for i < len(got) && i < len(want) && got[i] == want[i] {
			i++
		}
		t.Errorf("%s: fc 2 のビルド (%s) と一致しない (offset 0x%04x、%d / %d バイト)", gotPath, wantPath, i, len(got), len(want))
	}
}

// TestMigrateExamples: castle / miku の fc 2 のソース (testdata/migrate/v2/。examples/ は fc 3 に migrate 済み) を migrate して
// ビルドした ROM が、fc 2 の ROM の golden と一致する。資源 (画像・音) は examples/ の木を使い、.fc だけ fc 2 の版で上書きする。
func TestMigrateExamples(t *testing.T) {
	t.Parallel()
	home := migratedHome(t)
	for _, ex := range []struct{ name, dir, main string }{
		{"miku", "", "miku.fc"},
		{"castle", "src", "main.fc"},
	} {
		t.Run(ex.name, func(t *testing.T) {
			t.Parallel()
			root := filepath.Join(t.TempDir(), ex.name)
			copyDir(t, filepath.Join(absRepoRoot, "examples", ex.name), root)
			copyDir(t, filepath.Join(absRepoRoot, "testdata", "migrate", "v2", ex.name), root) // .fc を fc 2 の版に
			if migrateTree(t, root) == 0 {
				t.Fatal(".fc が無い")
			}
			src := filepath.Join(root, ex.dir)
			rom := filepath.Join(root, ex.name+".nes")
			code, err := NewCompiler(home).Build(ex.main, &BuildOptions{Target: "nes", Dir: src, Out: rom, BuildDir: filepath.Join(root, "build")})
			if err != nil || code != 0 {
				t.Fatalf("ビルド失敗: %v (code %d)", err, code)
			}
			sameBytes(t, rom, filepath.Join(absGoldenRoot, "examples", ex.name+".nes"))
		})
	}
}

// TestMigrateGoldenPrograms: test/ の golden のプログラム (testdata/golden/bin) を migrate してビルドしたバイナリが、
// fc 2 のバイナリの golden と一致する。
func TestMigrateGoldenPrograms(t *testing.T) {
	t.Parallel()
	home := migratedHome(t)
	dir := filepath.Join(t.TempDir(), "test")
	copyDir(t, testDir(), dir)
	migrateTree(t, dir)
	matches, err := filepath.Glob(filepath.Join(absGoldenRoot, "bin", "*"))
	if err != nil || len(matches) == 0 {
		t.Fatalf("golden bin が見つからない: %v", err)
	}
	for _, m := range matches {
		base := filepath.Base(m)
		name := strings.TrimSuffix(base, filepath.Ext(base))
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			srcName, target := goldenKeyInfo(name)
			tmp := t.TempDir()
			out := filepath.Join(tmp, base)
			code, err := NewCompiler(home).Build(srcName+".fc", &BuildOptions{Target: target, Out: out, Dir: dir, BuildDir: filepath.Join(tmp, "build")})
			if err != nil || code != 0 {
				t.Fatalf("ビルド失敗: %v (code %d)", err, code)
			}
			sameBytes(t, out, m)
		})
	}
}

// TestMigrateBench: bench/ のプログラムを fc 2 のままと migrate した後でビルドし、バイナリが一致する。
func TestMigrateBench(t *testing.T) {
	t.Parallel()
	home := migratedHome(t)
	orig := filepath.Join(absRepoRoot, "bench")
	mig := filepath.Join(t.TempDir(), "bench")
	copyDir(t, orig, mig)
	migrateTree(t, mig)
	files, _ := filepath.Glob(filepath.Join(orig, "*.fc"))
	if len(files) == 0 {
		t.Fatal("bench に .fc が無い")
	}
	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), ".fc")
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			tmp := t.TempDir()
			build := func(fcHome, dir, tag string) string {
				out := filepath.Join(tmp, tag+".bin")
				code, err := NewCompiler(fcHome).Build(name+".fc", &BuildOptions{Target: "emu", Out: out, Dir: dir, BuildDir: filepath.Join(tmp, tag)})
				if err != nil || code != 0 {
					t.Fatalf("%s: ビルド失敗: %v (code %d)", tag, err, code)
				}
				return out
			}
			sameBytes(t, build(home, mig, "v3"), build(absRepoRoot, orig, "v2"))
		})
	}
}
