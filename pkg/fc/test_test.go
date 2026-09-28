package fc

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestFclibModuleTests: fclib のモジュールの @(test) の関数を fcc test (Compiler.Test) で走らせる (-O 0 / -O 2)。
func TestFclibModuleTests(t *testing.T) {
	t.Parallel()
	c, err := New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	files, _ := filepath.Glob(filepath.Join("..", "..", "fclib", "*.fc"))
	emu, _ := filepath.Glob(filepath.Join("..", "..", "fclib", "emu", "*.fc"))
	n := 0
	for _, f := range append(files, emu...) {
		src, err := os.ReadFile(f)
		if err != nil || !bytes.Contains(src, []byte("@(test)")) {
			continue
		}
		n++
		for _, level := range []int{-1, 2} {
			var out bytes.Buffer
			res, err := c.Test(context.Background(), []string{f}, Options{Stdout: &out, OptimizeLevel: level})
			if err != nil {
				t.Fatalf("%s: %v", f, err)
			}
			if res.ExitCode != 0 || !strings.HasSuffix(out.String(), " tests ok\n") {
				t.Errorf("%s (-O %d): exit %d\n%s", f, level, res.ExitCode, out.String())
			}
		}
	}
	if n == 0 {
		t.Fatal("@(test) のある fclib のモジュールが無い")
	}
}

// TestTestCommand: @(test) の関数を「モジュール.名前: 」と出して呼び、@assert / @assert_eq が落ちたら場所と式 (と値) を出して
// 終了コード 1 で止まる。全部通れば「N tests ok」。
func TestTestCommand(t *testing.T) {
	t.Parallel()
	c, err := New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	dir := t.TempDir()
	write := func(src string) string {
		p := filepath.Join(dir, "m.fc")
		if err := os.WriteFile(p, []byte(src), 0o666); err != nil {
			t.Fatal(err)
		}
		return p
	}
	run := func(p string) (string, int) {
		var out bytes.Buffer
		res, err := c.Test(context.Background(), []string{p}, Options{Stdout: &out})
		if err != nil {
			t.Fatal(err)
		}
		return out.String(), res.ExitCode
	}
	head := "#fc 4\nfunction add(a:u8, b:u8):u8 { return a + b; }\nfunction test_ok():void @(test) { @assert(add(1, 2) == 3); @assert_eq(add(2, 2), 4); }\n"
	if out, code := run(write(head)); code != 0 || out != "m.test_ok: ok\n1 tests ok\n" {
		t.Errorf("通るとき: %q (exit %d)", out, code)
	}
	out, code := run(write(head + "function test_eq():void @(test)\n{\n\t@assert_eq(add(2, 2), 5);\n}\n"))
	if code != 1 || !strings.HasSuffix(out, "m.test_eq: m.fc:6: assert_eq failed: add(2, 2) is 4 (want 5)\n") {
		t.Errorf("@assert_eq: %q (exit %d)", out, code)
	}
	out, code = run(write(head + "function test_a():void @(test)\n{\n\t@assert(add(1, 1) > 2, \"too small\");\n}\n"))
	if code != 1 || !strings.HasSuffix(out, "m.test_a: panic: m.fc:6: assert failed: add(1, 1) > 2: too small\n") {
		t.Errorf("@assert: %q (exit %d)", out, code)
	}
}
