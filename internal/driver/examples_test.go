package driver

// examples/ の実プロジェクト由来サンプル (miku, castle) の回帰テスト。
// このコンパイラの実利用プロジェクトはこの2つだけであり、これらの ROM が
// スナップショット (testdata/golden/examples/) とバイト一致すれば
// 「実プロジェクトの動作が維持できている」とみなす。
//
// サンプルと実プロジェクトの同期・差分確認は tools/sync_examples.ps1 を使う
// (詳細は examples/README.md)。

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func runTool(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
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

// TestExampleCastle は examples/castle (castle 由来) のビルド。
// 実プロジェクトの Rakefile と同じ手順:
//
//	fcc compile -t nes main.fc → ca65 data.asm → ld65 (プロジェクト独自の ld65.cfg)
func TestExampleCastle(t *testing.T) {
	t.Parallel()
	// castle は .fc-build/ を ld65 の入力に使う実プロジェクト手順をなぞるので、ツリーごと一時ディレクトリに複製する
	dir := filepath.Join(t.TempDir(), "castle")
	copyDir(t, filepath.Join(absRepoRoot, "examples", "castle"), dir)
	src := filepath.Join(dir, "src")

	compiler := NewCompiler(absRepoRoot)
	code, err := compiler.Build("main.fc", &BuildOptions{Target: "nes", CompileOnly: true, Dir: src})
	if err != nil {
		t.Fatalf("コンパイル失敗: %v", err)
	}
	if code != 0 {
		t.Fatalf("コンパイル結果コード: %d", code)
	}

	runTool(t, src, "ca65", "data.asm", "-o", ".fc-build/data.o")

	// リンク (実プロジェクトの Rakefile と同じ引数構成。obj はソート順 = Dir.glob 相当)
	objs, err := filepath.Glob(filepath.Join(src, DefaultBuildDirName, "*.o"))
	if err != nil || len(objs) == 0 {
		t.Fatalf("オブジェクトファイルが見つからない: %v", err)
	}
	tmp := t.TempDir()
	rom := filepath.Join(tmp, "castle.nes")
	mapFile := filepath.Join(tmp, "castle.map")
	args := []string{"-o", rom, "-vm", "-m", mapFile, "-C", "ld65.cfg"}
	for _, o := range objs {
		rel, _ := filepath.Rel(dir, o)
		args = append(args, filepath.ToSlash(rel))
	}
	args = append(args, "res/sound/bgm.o", "res/sound/castle.o", "nsd/lib/NSD.lib")
	runTool(t, dir, "ld65", args...)

	compareROM(t, rom, filepath.Join("examples", "castle.nes"))
}
