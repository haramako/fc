package driver

// examples/ の実プロジェクト由来サンプル (miku, castle) の回帰テスト。
// このコンパイラの実利用プロジェクトはこの2つだけであり、これらの ROM が
// スナップショット (testdata/golden/examples/) とバイト一致すれば
// 「実プロジェクトの動作が維持できている」とみなす。
//
// サンプルと実プロジェクトの同期・差分確認は tools/sync_examples.ps1 を使う
// (詳細は examples/README.md)。

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// castleCompileLimit はコンパイル時間の退行を捕まえる閾値 (手元で約 1.8 秒。常駐の候補探索が候補数の 3 乗になって
// 55 秒になったのを見落としたことがある。doc/development_notes.md (11))。マシン差を見て目安の 5 倍。
const castleCompileLimit = 10 * time.Second

func runTool(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(ToolPath(name), args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v failed: %v\n%s", name, args, err, out)
	}
}

// compareROM は ROM を golden (testdata/golden/ 相対) とバイト比較する。-update で更新できる。
func compareROM(t *testing.T, gotPath, goldenRel string) {
	t.Helper()
	got, err := os.ReadFile(gotPath)
	if err != nil {
		t.Fatalf("ROM読み込み失敗: %v", err)
	}
	compareGoldenBytes(t, goldenRel, got)
}

// TestExampleMiku は examples/miku (fc-miku 由来) のフルビルド。
// fc 標準のドライバのみで ROM まで生成する構成。
func TestExampleMiku(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(absRepoRoot, "examples", "miku")
	tmp := t.TempDir()
	rom := filepath.Join(tmp, "miku.nes")
	compiler := NewCompiler(absRepoRoot)
	code, err := compiler.Build("miku.fc", &BuildOptions{Target: "nes", Out: rom, Dir: dir, BuildDir: filepath.Join(tmp, "build")})
	if err != nil {
		t.Fatalf("ビルド失敗: %v", err)
	}
	if code != 0 {
		t.Fatalf("ビルド結果コード: %d", code)
	}
	compareROM(t, rom, filepath.Join("examples", "miku.nes"))
}

// TestExampleCastle は examples/castle (castle 由来) のビルド。main.fc の options(base:) / options(linker_config:) /
// options(link:) で自前の data.asm・ld65.cfg・NSD のライブラリを指定しているので、`fcc build -t nes main.fc` だけで
// ROM ができる (以前は fcc compile → ca65 data.asm → ld65 を Rakefile が並べていた)。
func TestExampleCastle(t *testing.T) {
	t.Parallel()
	// castle は .fc-build/ を src の下に作る実プロジェクト手順なので、ツリーごと一時ディレクトリに複製する
	dir := filepath.Join(t.TempDir(), "castle")
	copyDir(t, filepath.Join(absRepoRoot, "examples", "castle"), dir)
	src := filepath.Join(dir, "src")
	rom := filepath.Join(dir, "castle.nes")

	compiler := NewCompiler(absRepoRoot)
	start := time.Now()
	res, err := compiler.BuildContext(context.Background(), "main.fc", &BuildOptions{Target: "nes", Dir: src, Out: rom})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("ビルド失敗: %v", err)
	}
	t.Logf("castle のビルド (ca65 / ld65 込み): %.2f 秒", elapsed.Seconds())
	if elapsed > castleCompileLimit {
		t.Errorf("castle のビルドに %.1f 秒かかった (上限 %v)。最適化パスの計算量の退行を疑う", elapsed.Seconds(), castleCompileLimit)
	}
	if res.DbgFile == "" {
		t.Errorf("dbgfile が無い")
	}
	compareROM(t, rom, filepath.Join("examples", "castle.nes"))
}
