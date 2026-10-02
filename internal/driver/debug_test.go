package driver

// fcc build -g (Mesen 用のデバッグ情報) と --size-report / fcc size。

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/haramako/fc/internal/cc65"
	"github.com/haramako/fc/internal/sizehtml"
)

func TestDebugInfoAndSizeReport(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	src := "#fc 2\nuse * from stdio;\nvar tab:[4]int;\nfunction f(x:int):int options(noinline: true)\n{\n\treturn tab[x] + 1;\n}\nfunction main():void\n{\n\ttab[1] = 5;\n\tprintf(f(1), \"\n\");\n\texit(0);\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "t.fc"), []byte(src), 0o666); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out", "t.nes")
	if err := os.MkdirAll(filepath.Dir(out), 0o777); err != nil {
		t.Fatal(err)
	}
	res, err := NewCompiler(absRepoRoot).BuildContext(context.Background(), "t.fc", &BuildOptions{
		Target: "nes", Dir: dir, BuildDir: filepath.Join(dir, "b"), Out: out, Debug: true, SizeReport: true,
	})
	if err != nil {
		t.Fatalf("ビルド失敗: %v", err)
	}
	// .s に fc のソース位置 (.dbg file / .dbg line) が入る。パスは ROM の隣から辿れる相対パス
	asm, _ := os.ReadFile(filepath.Join(dir, "b", "_t.s"))
	if !strings.Contains(string(asm), ".dbg file, \"../t.fc\", 0, 0") || !strings.Contains(string(asm), ".dbg line, \"../t.fc\", 6") {
		t.Errorf(".dbg が無い:\n%s", asm)
	}
	// ld65 の dbgfile に fc の行 (type=1) が載り、.mlb にラベルが並ぶ
	if res.DbgFile != filepath.Join(dir, "out", "t.dbg") {
		t.Errorf("DbgFile: %s", res.DbgFile)
	}
	dbg, _ := os.ReadFile(res.DbgFile)
	if !strings.Contains(string(dbg), "name=\"../t.fc\"") || !strings.Contains(string(dbg), "type=1") {
		t.Errorf("dbgfile に fc の行が無い")
	}
	mlb, err := os.ReadFile(filepath.Join(dir, "out", "t.mlb"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"NesPrgRom:", ":_t_f\n", ":_main\n", "NesInternalRam:", ":_t_tab\n"} {
		if !strings.Contains(string(mlb), want) {
			t.Errorf(".mlb に %q が無い:\n%s", want, mlb)
		}
	}
	// サイズの要約: セグメントの合計と関数の一覧 (f は main より小さい)
	joined := strings.Join(res.SizeReport, "\n")
	if !strings.Contains(joined, "bytes in ROM") || !strings.Contains(joined, "_t_f ") || !strings.Contains(joined, "_main ") {
		t.Errorf("size report:\n%s", joined)
	}
	d, err := cc65.ParseDbgFile(res.DbgFile)
	if err != nil {
		t.Fatal(err)
	}
	sizes := map[string]int{}
	for _, e := range d.Sizes() {
		sizes[e.Name] = e.Size
	}
	if sizes["_t_f"] <= 0 || sizes["_main"] <= sizes["_t_f"] {
		t.Errorf("sizes: f=%d main=%d", sizes["_t_f"], sizes["_main"])
	}

	// -g 無しでは .dbg line を出さない (golden の asm は変わらない)
	res2, err := NewCompiler(absRepoRoot).BuildContext(context.Background(), "t.fc", &BuildOptions{
		Target: "nes", Dir: dir, BuildDir: filepath.Join(dir, "b2"), Out: filepath.Join(dir, "out", "t2.nes"),
	})
	if err != nil {
		t.Fatal(err)
	}
	asm2, _ := os.ReadFile(filepath.Join(dir, "b2", "_t.s"))
	if strings.Contains(string(asm2), ".dbg") {
		t.Errorf("-g 無しで .dbg が出ている")
	}
	if _, err := os.Stat(filepath.Join(dir, "out", "t2.mlb")); err == nil {
		t.Errorf("-g 無しで .mlb が出ている")
	}
	if res2.DbgFile == "" {
		t.Errorf("dbgfile はいつも書く (size report / 外部ツール用)")
	}
}

// TestSizeReportBanks: --size-report のバンクの表 (リンカ設定の ROM の領域ごとの使用量・空きと置いたモジュール) と、モジュールの
// 間の呼び出しの数 (バンクをまたぐ far call の数も)。fc.toml のバンクの表のプログラム (layoutFiles) で。
func TestSizeReportBanks(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for name, src := range layoutFiles() {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o666); err != nil {
			t.Fatal(err)
		}
	}
	res, err := NewCompiler(absRepoRoot).BuildContext(context.Background(), "main.fc", &BuildOptions{
		Target: "nes", Dir: dir, BuildDir: filepath.Join(dir, "b"), Out: filepath.Join(dir, "t.nes"), SizeReport: true,
	})
	if err != nil {
		t.Fatalf("ビルド失敗: %v", err)
	}
	joined := strings.Join(res.SizeReport, "\n")
	for _, re := range []string{
		`(?m)^banks \(ROM areas`,
		`(?m)^  ROM0 +\$8000 +8192 +\d+ +\d+ +\d+%  ma \d+`, // [bank.a] は 0 番、8K のバンク
		`(?m)^  FIXED +\$C000 .*main \d+`,
		`(?m)^calls between modules`,
		`(?m)^  main \(FIXED\) +-> mb \(ROM1\) +1  far 1$`,
		`(?m)^  ROM2 +\$8000 +8192 +0 +8192 +0%$`, // 空きのバンク
	} {
		if !regexp.MustCompile(re).MatchString(joined) {
			t.Errorf("size report に /%s/ が無い:\n%s", re, joined)
		}
	}
}

// TestSizeHTML: --size-html のページ (internal/sizehtml) に、--size-report と同じデータ (ROM の領域とモジュール、モジュールの間の
// 呼び出し、関数) が JSON で入る。見た目 (JS) は確かめない。
func TestSizeHTML(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for name, src := range layoutFiles() {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o666); err != nil {
			t.Fatal(err)
		}
	}
	page := filepath.Join(dir, "size.html")
	if _, err := NewCompiler(absRepoRoot).BuildContext(context.Background(), "main.fc", &BuildOptions{
		Target: "nes", Dir: dir, BuildDir: filepath.Join(dir, "b"), Out: filepath.Join(dir, "t.nes"), SizeHTML: page,
	}); err != nil {
		t.Fatalf("ビルド失敗: %v", err)
	}
	b, err := os.ReadFile(page)
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`const R = (\{.*\});\n`).FindSubmatch(b)
	if m == nil {
		t.Fatalf("データが無い:\n%s", b)
	}
	var rep sizehtml.Report
	if err := json.Unmarshal(m[1], &rep); err != nil {
		t.Fatal(err)
	}
	areas := map[string]sizehtml.Area{}
	for _, a := range rep.Areas {
		areas[a.Name] = a
	}
	if a := areas["ROM0"]; a.Size != 8192 || len(a.Segments) != 1 || a.Segments[0].Name != "ma" || a.Used != a.Segments[0].Size {
		t.Errorf("ROM0: %+v", a)
	}
	if a, ok := areas["ROM2"]; !ok || a.Segments == nil || a.Used != 0 {
		t.Errorf("空きのバンク ROM2: %+v (segments は null でなく空の配列)", a)
	}
	found := false
	for _, c := range rep.Calls {
		found = found || c.From == "main" && c.To == "mb" && c.Count == 1 && c.Far == 1
	}
	if !found || !rep.HasCalls || len(rep.Functions) == 0 || rep.Title != "main.fc" || !strings.Contains(string(b), "<h1>main.fc</h1>") {
		t.Errorf("calls %+v, functions %d, title %q", rep.Calls, len(rep.Functions), rep.Title)
	}
}
