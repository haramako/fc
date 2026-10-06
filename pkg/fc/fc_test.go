package fc

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	p, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// TestBuildMiku は公開 API 経由で examples/miku をビルドし、golden の ROM と一致することを確認する。
func TestBuildMiku(t *testing.T) {
	root := repoRoot(t)
	c := NewWithHome(root)
	defer c.Close()
	tmp := t.TempDir()
	res, err := c.Build(context.Background(), "miku.fc", Options{
		Target: TargetNES, Out: filepath.Join(tmp, "miku.nes"),
		Dir: filepath.Join(root, "examples", "miku"), BuildDir: filepath.Join(tmp, "build"),
	})
	if err != nil {
		t.Fatalf("ビルド失敗: %v", err)
	}
	if res.Out == "" || res.MapFile == "" || len(res.Objects) == 0 || res.BuildDir != filepath.Join(tmp, "build") {
		t.Errorf("Result: %+v", res)
	}
	got, err := os.ReadFile(res.Out)
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join(root, "testdata", "golden", "examples", "miku.nes"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Error("ROM が golden と一致しない")
	}
}

// TestBuildError はコンパイルエラーが *Error (位置付き) で返ることを確認する。
func TestBuildError(t *testing.T) {
	root := repoRoot(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "t.fc"), []byte("function main():void\n{\n  hoge = 1;\n}\n"), 0o666); err != nil {
		t.Fatal(err)
	}
	c := NewWithHome(root)
	_, err := c.Build(context.Background(), "t.fc", Options{Dir: dir, CompileOnly: true})
	var ce *Error
	if !errors.As(err, &ce) {
		t.Fatalf("*Error であるべき: %v", err)
	}
	if !strings.Contains(ce.Msg, "hoge not found") || ce.Pos.Line != 3 || ce.Pos.Col != 3 {
		t.Errorf("got %+v", ce)
	}
}

// TestRun は emu ターゲットでビルドして走らせ (Build と Run)、出力と終了コードが取れることを確認する。
func TestRun(t *testing.T) {
	root := repoRoot(t)
	tmp := t.TempDir()
	var out strings.Builder
	c := NewWithHome(root)
	res, err := c.Build(context.Background(), "test_basic.fc", Options{
		Out: filepath.Join(tmp, "a.bin"), Dir: filepath.Join(root, "test"), BuildDir: filepath.Join(tmp, "build"),
	})
	if err != nil {
		t.Fatalf("ビルド失敗: %v", err)
	}
	r, err := c.Run(context.Background(), res, RunOptions{Stdout: &out})
	if err != nil || r.ExitCode != 0 || out.Len() == 0 {
		t.Errorf("exit=%v out=%q err=%v", r, out.String(), err)
	}
	// リンクしていない (CompileOnly) ものは走らせられない
	res, err = c.Build(context.Background(), "test_basic.fc", Options{
		CompileOnly: true, Out: filepath.Join(tmp, "b.bin"), Dir: filepath.Join(root, "test"), BuildDir: filepath.Join(tmp, "build2"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Run(context.Background(), res, RunOptions{Stdout: &out}); err == nil {
		t.Errorf("CompileOnly の結果を走らせられた")
	}
}

// TestRunNES: Run の NES の ROM は内蔵の NES のランナーで console.exit まで走らせ、console の出力と終了コードを返す。
// 終わらなければエラー (画面は描かないのでエミュレータで開く)。
func TestRunNES(t *testing.T) {
	t.Parallel()
	c, err := New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	dir := t.TempDir()
	write := func(src string) {
		if err := os.WriteFile(filepath.Join(dir, "t.fc"), []byte(src), 0o666); err != nil {
			t.Fatal(err)
		}
	}
	write("#fc 4\nuse console;\nfunction main():void\n{\n\tconsole.init();\n\t@printf(\"hello {}\n\", 42);\n\tconsole.exit(3);\n}\n")
	var out bytes.Buffer
	buildRun := func() (*RunResult, error) {
		res, err := c.Build(context.Background(), "t.fc", Options{Target: TargetNES, Dir: dir, Out: filepath.Join(dir, "t.nes")})
		if err != nil {
			t.Fatal(err)
		}
		return c.Run(context.Background(), res, RunOptions{Stdout: &out, MaxFrames: 60})
	}
	r, err := buildRun()
	if err != nil {
		t.Fatal(err)
	}
	if out.String() != "hello 42\n" || r.ExitCode != 3 {
		t.Errorf("出力 %q、終了コード %d", out.String(), r.ExitCode)
	}
	write("#fc 4\nfunction main():void\n{\n\twhile (true) {\n\t}\n}\n")
	if _, err := buildRun(); err == nil || !strings.Contains(err.Error(), "did not exit") {
		t.Errorf("終わらないプログラム: %v", err)
	}
}
