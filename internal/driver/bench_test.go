package driver

// 性能退行の検知用ベンチマーク (doc/v2_plan.md R0-5)。
// 基準値は v2_plan.md の作業ログに記録し、各フェーズ末に再計測する。
//
//	go test ./internal/fc -run xxx -bench BenchmarkCastle -benchmem

import (
	"github.com/haramako/fc/internal/codegen"
	"github.com/haramako/fc/internal/sema"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// copyDir は src ディレクトリを dst に再帰コピーする (examples/*/.fc-build を共有しないため)。
func copyDir(tb testing.TB, src, dst string) {
	tb.Helper()
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o777)
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.Create(target)
		if err != nil {
			return err
		}
		defer out.Close()
		_, err = io.Copy(out, in)
		return err
	})
	if err != nil {
		tb.Fatalf("コピー失敗: %v", err)
	}
}

// castleWorkDir は examples/castle を一時ディレクトリに複製し、その src/ に chdir する。
func castleWorkDir(b *testing.B) string {
	b.Helper()
	tmp := b.TempDir()
	copyDir(b, filepath.Join(absRepoRoot, "examples", "castle"), tmp)
	src := filepath.Join(tmp, "src")
	b.Chdir(src)
	return src
}

// BenchmarkCastleFrontend は castle の parse → HLC → LLC (純 Go 部分、ファイル出力なし)。
func BenchmarkCastleFrontend(b *testing.B) {
	castleWorkDir(b)
	libPath := []string{".",
		filepath.ToSlash(filepath.Join(absRepoRoot, "fclib")),
		filepath.ToSlash(filepath.Join(absRepoRoot, "fclib", "nes"))}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hlc := sema.NewHlc(libPath)
		if err := hlc.Compile("main.fc"); err != nil {
			b.Fatalf("コンパイル失敗: %v", err)
		}
		llc := codegen.NewLlc(2, hlc.Types())
		for _, mod := range hlc.Modules.List() {
			if _, _, err := llc.Compile(mod); err != nil {
				b.Fatal(err)
			}
		}
	}
}

// BenchmarkCastleCompile は `fcc compile -t nes main.fc` 相当 (ca65 によるアセンブルまで含む)。
func BenchmarkCastleCompile(b *testing.B) {
	src := castleWorkDir(b)
	compiler := NewCompiler(absRepoRoot)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		if err := os.RemoveAll(filepath.Join(src, BuildPath)); err != nil {
			b.Fatal(err)
		}
		b.StartTimer()
		code, err := compiler.Build("main.fc", &BuildOptions{Target: "nes", CompileOnly: true})
		if err != nil || code != 0 {
			b.Fatalf("コンパイル失敗: code=%d err=%v", code, err)
		}
	}
}
