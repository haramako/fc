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

// TestRun は emu ターゲットで実行し、出力と終了コードが取れることを確認する。
func TestRun(t *testing.T) {
	root := repoRoot(t)
	tmp := t.TempDir()
	var out strings.Builder
	c := NewWithHome(root)
	res, err := c.Build(context.Background(), "test_basic.fc", Options{
		Run: true, Stdout: &out, Out: filepath.Join(tmp, "a.bin"),
		Dir: filepath.Join(root, "test"), BuildDir: filepath.Join(tmp, "build"),
	})
	if err != nil {
		t.Fatalf("ビルド失敗: %v", err)
	}
	if res.ExitCode != 0 || out.Len() == 0 {
		t.Errorf("exit=%d out=%q", res.ExitCode, out.String())
	}
}
