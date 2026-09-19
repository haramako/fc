package driver

// fcc build -g (Mesen 用のデバッグ情報) と --size-report / fcc size。

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDebugInfoAndSizeReport(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	src := "#fc 2\nuse * from stdio;\nvar tab:[4]int;\nfunction f(x:int):int\n{\n\treturn tab[x] + 1;\n}\nfunction main():void\n{\n\ttab[1] = 5;\n\tprintf(f(1), \"\n\");\n\texit(0);\n}\n"
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
	d, err := ParseDbgFile(res.DbgFile)
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
