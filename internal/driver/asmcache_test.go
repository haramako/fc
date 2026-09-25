package driver

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestAsmCache: 入力 (.s・.include・.incbin・ca65・引数) が前回と同じなら ca65 を起動しない。incbin したファイルの中身だけを
// 変えると (fc の出力 .s は同じ)、そのモジュールだけアセンブルし直して結果が変わる。FC_NO_ASM_CACHE=1 なら毎回起動する。
func TestAsmCache(t *testing.T) {
	dir := t.TempDir()
	write := func(name, s string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(s), 0o666); err != nil {
			t.Fatal(err)
		}
	}
	write("t.fc", `#fc 2
use * from stdio;
const TAB:[]int = incbin("tab.bin");
function main():void
{
	printf(TAB[0] + TAB[1], "\n");
	exit(0);
}
`)
	write("tab.bin", "\x01\x02")
	build := func() (string, int64) {
		t.Helper()
		var out strings.Builder
		c := NewCompiler(absRepoRoot)
		code, err := c.Build("t.fc", &BuildOptions{Dir: dir, BuildDir: filepath.Join(dir, "b"), Out: filepath.Join(dir, "a.bin"), Run: true, Stdout: &out})
		if err != nil || code != 0 {
			t.Fatalf("ビルド失敗: %v (code %d)", err, code)
		}
		return out.String(), c.asmRuns.Load()
	}
	out, runs := build()
	if out != "3\n" || runs == 0 {
		t.Fatalf("1 回目: out=%q runs=%d", out, runs)
	}
	all := runs
	if out, runs = build(); out != "3\n" || runs != 0 {
		t.Errorf("2 回目 (変更なし): out=%q runs=%d (0 のはず)", out, runs)
	}
	write("tab.bin", "\x05\x06")
	if out, runs = build(); out != "11\n" || runs != 1 {
		t.Errorf("tab.bin を変えた後: out=%q runs=%d (1 のはず)", out, runs)
	}
	t.Setenv("FC_NO_ASM_CACHE", "1")
	if out, runs = build(); out != "11\n" || runs != all {
		t.Errorf("FC_NO_ASM_CACHE: out=%q runs=%d (%d のはず)", out, runs, all)
	}
}

// TestReadDepFile: ca65 の --create-dep の出力の読み方 (空白は `\ `、Windows のドライブ名の ':' は区切りでない)。
func TestReadDepFile(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{".fc-build/_t.o:\t.fc-build/_t.s inc\\ a.inc bin.dat\n\n.fc-build/_t.s inc\\ a.inc bin.dat:\n", []string{".fc-build/_t.s", "inc a.inc", "bin.dat"}},
		{"C:\\w\\b\\_t.o:\tC:\\w\\b\\_t.s C:\\fc\\share\\macro.inc\n", []string{"C:\\w\\b\\_t.s", "C:\\fc\\share\\macro.inc"}},
	}
	for _, c := range cases {
		p := filepath.Join(t.TempDir(), "t.d")
		if err := os.WriteFile(p, []byte(c.in), 0o666); err != nil {
			t.Fatal(err)
		}
		got, err := readDepFile(p)
		if err != nil || !reflect.DeepEqual(got, c.want) {
			t.Errorf("%q: got %q, %v want %q", c.in, got, err, c.want)
		}
	}
}
