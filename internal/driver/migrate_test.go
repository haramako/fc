package driver

// migrate の回帰テスト。castle / miku / bench / golden のテストプログラムと fclib のソースを `fcc migrate`
// (Compiler.Migrate: fc 2 → 3 は internal/migrate の構文の書き換え、fc 3 → 4 は sema が集める意味の書き換え) で最新の版に
// 書き換えてビルドし、元のままのビルドや golden と ROM・バイナリがバイト単位で一致することを確かめる。版で文法・意味を
// 変えるときは、migrate の規則 (fc 3 → 4 は sema の Rewrite) を足してここが通ることを確かめる
// (doc/v3_plan.md の「V2 からの移行」、doc/v4_plan.md §0)。

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haramako/fc/internal/sema"
	"github.com/haramako/fc/internal/syntax"
)

// fcFiles は dir の下の .fc のうち解析できるものを返す (test/errors.fc のようなエラーの検査用は除く)。skip が true の
// ディレクトリには入らない。
func fcFiles(tb testing.TB, dir string, skip func(path string) bool) []string {
	tb.Helper()
	var files []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != dir && skip != nil && skip(path) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".fc") {
			return nil
		}
		src, err := sema.ReadSource(path)
		if err != nil {
			return err
		}
		if _, perr := syntax.Parse(src, path); perr != nil {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		tb.Fatal(err)
	}
	return files
}

// migrateFiles は files を c (FC_HOME) で最新の版に書き換えて上書きする (書き換えた数を返す)。
func migrateFiles(tb testing.TB, c *Compiler, files []string, target string) int {
	tb.Helper()
	out, err := c.Migrate(files, &MigrateOptions{Target: target})
	if err != nil {
		tb.Fatalf("migrate: %v", err)
	}
	for _, path := range files {
		f, err := syntax.Parse(out[path], path)
		if err != nil || f.Version != syntax.LatestVersion {
			tb.Fatalf("%s: migrate の結果が fc %d でない: %v", path, syntax.LatestVersion, err)
		}
		if err := os.WriteFile(path, out[path], 0o666); err != nil {
			tb.Fatal(err)
		}
	}
	return len(files)
}

// migrateTree は dir の下の .fc を全部最新の版に書き換える (書き換えた数を返す)。fclib は home の fclib。
func migrateTree(tb testing.TB, home, dir, target string) int {
	tb.Helper()
	return migrateFiles(tb, NewCompiler(home), fcFiles(tb, dir, nil), target)
}

// migratedHome は fclib を最新の版に書き換えた FC_HOME を一時ディレクトリに作る。
func migratedHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	copyDir(t, filepath.Join(absRepoRoot, "fclib"), filepath.Join(dir, "fclib"))
	copyDir(t, filepath.Join(absRepoRoot, "share"), filepath.Join(dir, "share"))
	lib := filepath.Join(dir, "fclib")
	isNes := func(p string) bool { return filepath.Base(p) == "nes" }
	c := NewCompiler(dir)
	n := migrateFiles(t, c, fcFiles(t, lib, isNes), "emu")
	n += migrateFiles(t, c, fcFiles(t, filepath.Join(lib, "nes"), nil), "nes")
	if n == 0 {
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

// TestMigrateGoldenPrograms: test/ の golden のプログラム (testdata/golden/bin) を migrate してビルドしたバイナリが、
// fc 2 のバイナリの golden と一致する。
func TestMigrateGoldenPrograms(t *testing.T) {
	t.Parallel()
	home := migratedHome(t)
	dir := filepath.Join(t.TempDir(), "test")
	copyDir(t, testDir(), dir)
	migrateTree(t, home, dir, "emu")
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
	migrateTree(t, home, mig, "emu")
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
			sameBytes(t, build(home, mig, "migrated"), build(absRepoRoot, orig, "orig"))
		})
	}
}

// TestMigrateExamples: examples の castle と miku (fc 3) を最新の版に migrate してビルドした ROM が、今の ROM の golden と
// バイト単位で一致する (fc 3 → 4 は意味の変わる所に書き換えを足して、ROM を変えずに移す。doc/v4_plan.md §0)。
func TestMigrateExamples(t *testing.T) {
	t.Parallel()
	for _, ex := range []struct{ name, src, main string }{
		{"castle", "src", "main.fc"},
		{"miku", ".", "miku.fc"},
	} {
		t.Run(ex.name, func(t *testing.T) {
			t.Parallel()
			root := filepath.Join(t.TempDir(), ex.name)
			copyDir(t, filepath.Join(absRepoRoot, "examples", ex.name), root)
			dir := filepath.Join(root, ex.src)
			if migrateTree(t, absRepoRoot, dir, "nes") == 0 {
				t.Fatal(".fc が無い")
			}
			rom := filepath.Join(root, ex.name+".nes")
			code, err := NewCompiler(absRepoRoot).Build(ex.main, &BuildOptions{Target: "nes", Out: rom, Dir: dir, BuildDir: filepath.Join(root, "build")})
			if err != nil || code != 0 {
				t.Fatalf("ビルド失敗: %v (code %d)", err, code)
			}
			compareROM(t, rom, filepath.Join("examples", ex.name+".nes"))
		})
	}
}
