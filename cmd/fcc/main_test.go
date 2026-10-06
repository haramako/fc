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
	if code, out, _ := runCLI(t); code != 0 || !strings.Contains(out, "Usage: fcc <command>") {
		t.Errorf("no args: code=%d out=%q", code, out)
	}
	if code, out, _ := runCLI(t, "--help"); code != 0 || !strings.Contains(out, "lib fetch") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	if code, _, errOut := runCLI(t, "bogus"); code != 2 || !strings.Contains(errOut, "unexpected argument bogus") {
		t.Errorf("unknown command: code=%d err=%q", code, errOut)
	}
	if code, out, _ := runCLI(t, "version"); code != 0 || !strings.Contains(out, "fcc") {
		t.Errorf("version: code=%d out=%q", code, out)
	}
	if code, out, _ := runCLI(t, "-v"); code != 0 || !strings.Contains(out, "ca65:") {
		t.Errorf("-v: code=%d out=%q", code, out)
	}
	// ファイルの要るコマンドにファイルが無ければ、使い方の案内と終了コード 2
	for _, sub := range []string{"build", "fmt", "check", "size", "watch", "test", "migrate"} {
		if code, _, errOut := runCLI(t, sub); code != 2 || !strings.Contains(errOut, "fcc: error: expected") || !strings.Contains(errOut, "fcc "+sub+" --help") {
			t.Errorf("%s without file: code=%d err=%q", sub, code, errOut)
		}
		if code, out, _ := runCLI(t, sub, "-h"); code != 0 || !strings.Contains(out, "Usage: fcc "+sub) {
			t.Errorf("%s -h: code=%d out=%q", sub, code, out)
		}
	}
}

// TestCLIFlagSyntax: GNU の getopt の形 (オプションはソースの後ろにも書ける、`-O1`、短いオプションをまとめる、長い名前は `--`)。
func TestCLIFlagSyntax(t *testing.T) {
	setup(t)
	write(t, "t.fc", "#fc 2\nuse * from stdio;\nfunction main():void { printf(\"hi\\n\"); exit(3); }\n")
	for _, args := range [][]string{
		{"run", "t.fc", "-O", "1"},
		{"run", "-O1", "t.fc"},
		{"run", "--opt=0", "t.fc", "-t", "emu"},
		{"run", "-temu", "t.fc", "--offline"},
	} {
		if code, out, errOut := runCLI(t, args...); code != 3 || out != "hi\n" {
			t.Errorf("%v: code=%d out=%q err=%q", args, code, out, errOut)
		}
	}
	// `-offline` は `-o ffline` と読めてしまうので、エラーにする
	if code, _, errOut := runCLI(t, "build", "-offline", "t.fc"); code != 2 || !strings.Contains(errOut, "--offline") {
		t.Errorf("-offline: code=%d err=%q", code, errOut)
	}
	if code, _, errOut := runCLI(t, "build", "--bogus", "t.fc"); code != 2 || !strings.Contains(errOut, "unknown flag --bogus") {
		t.Errorf("--bogus: code=%d err=%q", code, errOut)
	}
	if code, _, errOut := runCLI(t, "test", "-t", "foo", "t.fc"); code != 2 || !strings.Contains(errOut, "--target") {
		t.Errorf("test -t foo: code=%d err=%q", code, errOut)
	}
	// 親のコマンドのオプション (lib の -C) はサブコマンドの後ろにも書ける
	if err := os.Mkdir("p", 0o777); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join("p", "fc.toml"), "")
	for _, args := range [][]string{{"lib", "-C", "p", "list"}, {"lib", "list", "-C", "p"}, {"lib", "list", "--directory=p"}} {
		if code, out, _ := runCLI(t, args...); code != 0 || !strings.Contains(out, "no libraries") {
			t.Errorf("%v: code=%d out=%q", args, code, out)
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
	code, out, _ = runCLI(t, "run", "-O", "1", "t.fc")
	if code != 3 || !strings.Contains(out, "hi") {
		t.Errorf("run -O 1: code=%d out=%q", code, out)
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
	// check --json: 1 行 1 診断の JSON (エディタ連携)
	code, out, _ = runCLI(t, "check", "--json", "e.fc")
	if code != 1 || !strings.Contains(out, `"severity":"error"`) || !strings.Contains(out, `"line":2`) || !strings.Contains(out, `"message":"x not found"`) {
		t.Errorf("check --json: code=%d out=%q", code, out)
	}
	code, out, _ = runCLI(t, "check", "--json", "t.fc")
	if code != 0 || !strings.Contains(out, `"severity":"warning"`) {
		t.Errorf("check --json warning: code=%d out=%q", code, out)
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

// Home resolution failures should reach stderr with actionable environment advice.
func TestCLIHomeTempFailure(t *testing.T) {
	dir := setup(t)
	t.Setenv("FC_HOME", "")
	blocked := filepath.Join(dir, "not-a-directory")
	if err := os.WriteFile(blocked, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(name, blocked)
	}
	t.Setenv("FC_CACHE_DIR", blocked) // 同梱の fclib のキャッシュも作れないとき
	code, out, stderr := runCLI(t, "build", "unused.fc")
	if code != 1 || out != "" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out, stderr)
	}
	for _, want := range []string{"failed to create a temporary directory for bundled FC libraries", blocked, "cause:", "FC_HOME", "pass these variables"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr missing %q: %s", want, stderr)
		}
	}
}

// TestCLIUseFromSourceDir: `fcc build m1/main.fc` の use / include はソースのディレクトリから探す (作業ディレクトリでなく)。
// 出力 (-o) は作業ディレクトリ基準、診断の位置は作業ディレクトリ基準で表示する。
func TestCLIUseFromSourceDir(t *testing.T) {
	dir := setup(t)
	if err := os.Mkdir("m1", 0o777); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join("m1", "a.fc"), "#fc 3\npublic function two():u8 { return 2; }\n")
	write(t, filepath.Join("m1", "main.fc"), "#fc 3\nuse * from stdio;\nuse a;\nfunction main():void { printf(a.two(), \"\n\"); exit(0); }\n")
	if code, out, _ := runCLI(t, "run", "m1/main.fc"); code != 0 || out != "2\n" {
		t.Errorf("run m1/main.fc: code=%d out=%q", code, out)
	}
	if code, out, _ := runCLI(t, "build", "-o", "out.bin", "m1/main.fc"); code != 0 {
		t.Errorf("build: code=%d out=%q", code, out)
	}
	if _, err := os.Stat(filepath.Join(dir, "out.bin")); err != nil {
		t.Errorf("out.bin が作業ディレクトリに無い: %v", err)
	}
	write(t, filepath.Join("m1", "bad.fc"), "#fc 3\nuse a;\nfunction main():void { x = 1; }\n")
	want := filepath.Join("m1", "bad.fc") + ":3:"
	if code, out, _ := runCLI(t, "check", "m1/bad.fc"); code != 1 || !strings.Contains(out, want) {
		t.Errorf("check m1/bad.fc: code=%d out=%q (want %q)", code, out, want)
	}
	if code, out, _ := runCLI(t, "build", "m1/bad.fc"); code != 1 || !strings.Contains(out, want) {
		t.Errorf("build m1/bad.fc: code=%d out=%q (want %q)", code, out, want)
	}
	t.Chdir("m1")
	if code, out, _ := runCLI(t, "run", "bad.fc"); code != 1 || !strings.Contains(out, "bad.fc:3:") || strings.Contains(out, "m1") {
		t.Errorf("run bad.fc: code=%d out=%q", code, out)
	}
}

// TestCLICompletion: `fcc __complete` の候補 (補完のスクリプトが呼ぶ) と、`fcc completion` のスクリプト。
func TestCLICompletion(t *testing.T) {
	setup(t)
	for _, tc := range []struct {
		args []string
		want []string // 出力の行の頭 (この順に含む)
		not  string
	}{
		{[]string{""}, []string{"build\t", "lib\t", "completion\t"}, "__complete"},
		{[]string{"bu"}, []string{"build\tBuild a ROM / binary"}, "compile"},
		{[]string{"build", ""}, []string{":files *.fc"}, ""},
		{[]string{"b", "-t", ""}, []string{"nes\t", "emu\t"}, ""},
		{[]string{"build", "-gO", ""}, []string{"0\t", "1\t", "2\t"}, ""},
		{[]string{"build", "-O1", ""}, []string{":files *.fc"}, ""},
		{[]string{"build", "--o"}, []string{"--out\t", "--opt\t", "--offline\t"}, "-o\t"},
		{[]string{"build", "-o", ""}, []string{":files"}, "*.fc"},
		{[]string{"build", "main.fc", ""}, nil, ":files"},
		{[]string{"lib", ""}, []string{"fetch\t", "update\t", "list\t", "add\t"}, ""},
		{[]string{"lib", "list", "--"}, []string{"--directory\t"}, ""},
		{[]string{"lib", "add", "x", ""}, []string{":dirs"}, ""},
		{[]string{"size", "--cfg", ""}, []string{":files *.cfg"}, ""},
		{[]string{"completion", ""}, []string{"zsh\t", "bash\t"}, ""},
	} {
		code, out, _ := runCLI(t, append([]string{"__complete"}, tc.args...)...)
		rest := out
		for _, w := range tc.want {
			i := strings.Index(rest, w)
			if i < 0 || (i > 0 && rest[i-1] != '\n') {
				t.Errorf("%q: %q が無い: %q", tc.args, w, out)
				break
			}
			rest = rest[i+len(w):]
		}
		if code != 0 || (tc.want == nil && out != "") || (tc.not != "" && strings.Contains(out, tc.not)) {
			t.Errorf("%q: code=%d out=%q", tc.args, code, out)
		}
	}
	for _, sh := range []string{"zsh", "bash"} {
		if code, out, _ := runCLI(t, "completion", sh); code != 0 || !strings.Contains(out, "__complete") {
			t.Errorf("completion %s: code=%d out=%q", sh, code, out)
		}
	}
}
