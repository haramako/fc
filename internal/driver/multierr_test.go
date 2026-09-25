package driver

// 複数エラー報告: 文ごとに回復して全部集める。失敗した宣言の参照 (巻き添え) は報告しない。

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haramako/fc/internal/diag"
)

func buildErrors(t *testing.T, files map[string]string, main string) []*diag.Error {
	t.Helper()
	dir := t.TempDir()
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o666); err != nil {
			t.Fatal(err)
		}
	}
	_, err := NewCompiler(absRepoRoot).Build(main, &BuildOptions{Dir: dir, BuildDir: filepath.Join(dir, "b"), Out: filepath.Join(dir, "a.bin"), CompileOnly: true})
	if err == nil {
		t.Fatal("エラーになるべき")
	}
	return diag.Errors(err)
}

func TestMultipleErrors(t *testing.T) {
	t.Parallel()
	es := buildErrors(t, map[string]string{"t.fc": `#fc 2
var a:int;
var q:Nope;
function f():int { return 1; }
function main():void
{
	a = hoge;
	a = "str";
	q = 1;
	q.x = 2;
	f(1, 2);
	var p:*int = a;
	undefined_fn();
	a = 1;
}
`}, "t.fc")
	var got []string
	for _, e := range es {
		got = append(got, strings.TrimPrefix(e.Pos.String(), e.Pos.Filename+":")+" "+e.Msg)
	}
	want := []string{
		"3:1 unknown type Nope",
		"7:6 hoge not found",
		"8:2 assignment to `a`: cannot assign [4]u8 to u8 (not compatible types)",
		"11:2 `f` expects 0 argument(s) but 2 given",
		"12:2 `p`: cannot assign u8 to *u8 (not compatible types)",
		"13:2 undefined_fn not found",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("errors:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	// q (型が不明な宣言) を使った 9, 10 行目は巻き添えなので出ない。ErrorList は errors.As で最初の 1 件が取れる
	var first *diag.Error
	if err := (diag.ErrorList(es)); !errors.As(err, &first) || first != es[0] {
		t.Errorf("errors.As: %v", first)
	}
	if !strings.Contains(diag.ErrorList(es).Error(), "and 5 more error(s)") {
		t.Errorf("Error(): %q", diag.ErrorList(es).Error())
	}
}

func TestMultipleErrorsAcrossModules(t *testing.T) {
	t.Parallel()
	// use 先のエラーと、無いモジュールの use (その名前の参照は巻き添えで出ない)、パースエラーのモジュール
	es := buildErrors(t, map[string]string{
		"main.fc":   "#fc 2\nuse lib;\nuse nothere;\nuse broken;\nfunction main():void { lib.f(); nothere.x = 1; broken.y = 2; zzz = 3; }\n",
		"lib.fc":    "#fc 2\npublic function f():void { var v:int = bad_name; }\n",
		"broken.fc": "#fc 2\nvar x:int\n",
	}, "main.fc")
	var got []string
	for _, e := range es {
		got = append(got, filepath.Base(e.Pos.Filename)+":"+e.Pos.String()[len(e.Pos.Filename)+1:]+" "+e.Msg)
	}
	joined := strings.Join(got, "\n")
	for _, w := range []string{"main.fc:3:1 file nothere.fc not found", "broken.fc:3:1 parse error", "lib.fc:2:", "bad_name not found", "main.fc:5:", "zzz not found"} {
		if !strings.Contains(joined, w) {
			t.Errorf("missing %q in:\n%s", w, joined)
		}
	}
	for _, e := range es {
		if strings.Contains(e.Msg, "nothere") && !strings.Contains(e.Msg, "not found") || strings.Contains(e.Msg, "broken.y") {
			t.Errorf("巻き添えのエラーが出ている: %s", e.Msg)
		}
	}
}

func TestTooManyErrors(t *testing.T) {
	t.Parallel()
	var b strings.Builder
	b.WriteString("#fc 2\nfunction main():void\n{\n")
	for i := 0; i < 50; i++ {
		b.WriteString("\tnope = 1;\n")
	}
	b.WriteString("}\n")
	es := buildErrors(t, map[string]string{"t.fc": b.String()}, "t.fc")
	if len(es) < 30 || len(es) > 31 || !strings.Contains(es[len(es)-1].Msg, "too many errors") {
		t.Errorf("got %d errors, last: %v", len(es), es[len(es)-1])
	}
}
