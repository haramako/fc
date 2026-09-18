package main

// CLI のスモークテスト: 各コマンドを一度ずつ通して、終了コードと出力の要点だけ見る
// (機能の中身は internal/driver 以下のテストが担う)。run() は os.Args / 標準出力を直接使うので逐次実行。

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// runCLI は fcc を引数付きで in-process 実行し、終了コードと標準出力・標準エラーを返す。
func runCLI(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	capture := func(f **os.File) (func() string, error) {
		r, w, err := os.Pipe()
		if err != nil {
			return nil, err
		}
		orig := *f
		*f = w
		done := make(chan string)
		go func() {
			b, _ := io.ReadAll(r)
			done <- string(b)
		}()
		return func() string {
			w.Close()
			*f = orig
			return <-done
		}, nil
	}
	getOut, err := capture(&os.Stdout)
	if err != nil {
		t.Fatal(err)
	}
	getErr, err := capture(&os.Stderr)
	if err != nil {
		t.Fatal(err)
	}
	origArgs := os.Args
	os.Args = append([]string{"fcc"}, args...)
	func() {
		defer func() {
			os.Args = origArgs
			if r := recover(); r != nil {
				code = -1
				t.Errorf("panic: %v", r)
			}
		}()
		code = run()
	}()
	return code, getOut(), getErr()
}

// setup は一時ディレクトリに作業を移し、FC_HOME をリポジトリに向ける。
func setup(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	root, err := filepath.Abs(filepath.Join(filepath.Dir(file), "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("FC_HOME", root)
	dir := t.TempDir()
	t.Chdir(dir)
	return dir
}

func write(t *testing.T, name, src string) {
	t.Helper()
	if err := os.WriteFile(name, []byte(src), 0o666); err != nil {
		t.Fatal(err)
	}
}

func TestCLIUsageAndVersion(t *testing.T) {
	setup(t)
	if code, out, _ := runCLI(t); code != 0 || !strings.Contains(out, "Usage: fcc") {
		t.Errorf("no args: code=%d out=%q", code, out)
	}
	if code, out, _ := runCLI(t, "bogus"); code != 0 || !strings.Contains(out, "Usage: fcc") {
		t.Errorf("unknown command: code=%d out=%q", code, out)
	}
	if code, out, _ := runCLI(t, "build"); code != 0 || !strings.Contains(out, "Usage: fcc") {
		t.Errorf("build without file: code=%d out=%q", code, out)
	}
	if code, out, _ := runCLI(t, "version"); code != 0 || !strings.Contains(out, "fcc") {
		t.Errorf("version: code=%d out=%q", code, out)
	}
	for _, sub := range []string{"fmt", "check"} {
		if code, out, _ := runCLI(t, sub); code != 0 || !strings.Contains(out, "Usage: fcc "+sub) {
			t.Errorf("%s without file: code=%d out=%q", sub, code, out)
		}
	}
}

func TestCLIBuildRunCheck(t *testing.T) {
	dir := setup(t)
	write(t, "t.fc", "#fc 2\nuse * from stdio;\nvar a:int;\nfunction main():void { a = 1; if (a & 1 == 1) { printf(\"hi\\n\"); } exit(3); }\n")

	code, out, errOut := runCLI(t, "build", "-o", "t.bin", "t.fc")
	if code != 0 || !strings.Contains(errOut, "warning:") {
		t.Errorf("build: code=%d out=%q err=%q", code, out, errOut)
	}
	if _, err := os.Stat(filepath.Join(dir, "t.bin")); err != nil {
		t.Errorf("t.bin が無い: %v", err)
	}

	// run は emu で実行し、プログラムの出力と終了コードをそのまま返す
	code, out, _ = runCLI(t, "run", "t.fc")
	if code != 3 || !strings.Contains(out, "hi") {
		t.Errorf("run: code=%d out=%q", code, out)
	}
	code, out, _ = runCLI(t, "build", "-e", "-O", "1", "t.fc")
	if code != 3 || !strings.Contains(out, "hi") {
		t.Errorf("build -e: code=%d out=%q", code, out)
	}

	// compile は .o だけ作る
	if code, _, _ := runCLI(t, "compile", "t.fc"); code != 0 {
		t.Errorf("compile: code=%d", code)
	}
	if _, err := os.Stat(filepath.Join(dir, ".fc-build", "_t.o")); err != nil {
		t.Errorf("_t.o が無い: %v", err)
	}

	// check: 警告は標準エラー、エラーは file:line:col: error: で終了コード 1
	code, _, errOut = runCLI(t, "check", "t.fc")
	if code != 0 || !strings.Contains(errOut, "t.fc:4:") || !strings.Contains(errOut, "warning:") {
		t.Errorf("check: code=%d err=%q", code, errOut)
	}
	write(t, "e.fc", "#fc 2\nfunction main():void { x = 1; }\n")
	code, out, _ = runCLI(t, "check", "e.fc")
	if code != 1 || !strings.Contains(out, "e.fc:2:") || !strings.Contains(out, "error:") {
		t.Errorf("check error: code=%d out=%q", code, out)
	}
	code, out, _ = runCLI(t, "build", "e.fc")
	if code != 1 || !strings.Contains(out, "error: x not found") {
		t.Errorf("build error: code=%d out=%q", code, out)
	}
	if code, out, _ := runCLI(t, "build", "missing.fc"); code != 1 || out == "" {
		t.Errorf("missing file: code=%d out=%q", code, out)
	}
}

func TestCLIFmt(t *testing.T) {
	setup(t)
	write(t, "u.fc", "#fc 2\nvar   a:int;\nfunction f():void { a=1; }\n")
	write(t, "ok.fc", "#fc 2\nvar a:int;\n")
	want := "#fc 2\nvar a:int;\nfunction f():void\n{\n\ta = 1;\n}\n"

	code, out, _ := runCLI(t, "fmt", "u.fc")
	if code != 0 || out != want {
		t.Errorf("fmt: code=%d out=%q", code, out)
	}
	code, out, _ = runCLI(t, "fmt", "-l", "u.fc", "ok.fc")
	if code != 0 || out != "u.fc\n" {
		t.Errorf("fmt -l: code=%d out=%q", code, out)
	}
	code, out, _ = runCLI(t, "fmt", "-d", "u.fc")
	if code != 0 || !strings.Contains(out, "--- u.fc (original)") || !strings.Contains(out, "+var a:int;") {
		t.Errorf("fmt -d: code=%d out=%q", code, out)
	}
	if code, _, _ := runCLI(t, "fmt", "-w", "u.fc"); code != 0 {
		t.Errorf("fmt -w: code=%d", code)
	}
	if b, _ := os.ReadFile("u.fc"); string(b) != want {
		t.Errorf("fmt -w の結果: %q", b)
	}
	if code, out, _ := runCLI(t, "fmt", "-l", "u.fc"); code != 0 || out != "" {
		t.Errorf("fmt -l after -w: code=%d out=%q", code, out)
	}

	// CRLF のファイルは CRLF のまま
	write(t, "crlf.fc", "#fc 2\r\nvar   a:int;\r\n")
	if code, out, _ := runCLI(t, "fmt", "crlf.fc"); code != 0 || out != "#fc 2\r\nvar a:int;\r\n" {
		t.Errorf("fmt crlf: code=%d out=%q", code, out)
	}

	write(t, "bad.fc", "#fc 2\nvar a:int\n")
	code, _, errOut := runCLI(t, "fmt", "bad.fc")
	if code != 1 || !strings.Contains(errOut, "bad.fc:") || !strings.Contains(errOut, "error:") {
		t.Errorf("fmt parse error: code=%d err=%q", code, errOut)
	}
	if code, _, errOut := runCLI(t, "fmt", "none.fc"); code != 1 || errOut == "" {
		t.Errorf("fmt missing: code=%d err=%q", code, errOut)
	}
}
