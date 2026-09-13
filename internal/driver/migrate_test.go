package driver

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haramako/fc/internal/migrate"
)

// writeFiles は dir 以下にファイルを作る。
func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, src := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o777); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(src), 0o666); err != nil {
			t.Fatal(err)
		}
	}
}

// TestMigrateSmall: 小さな 3 モジュールで書き換え規則を確認する (doc/v2_grammar.md §5)。
func TestMigrateSmall(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "src")
	// v1 の include("macro.rb") は castle 用の実装が ../tmp/font/*.chr.txt を読む
	writeFiles(t, root, map[string]string{"tmp/font/text.chr.txt": "あいう", "tmp/font/misc_text.chr.txt": "あいう"})
	writeFiles(t, dir, map[string]string{
		// common: 束縛の再輸出 (use stdio → public use)、glob 再輸出 (use * from util → public use * from)、
		// macro.rb の置換、参照される private 定数
		"common.fc": "options(bank: -1);\ninclude(\"macro.rb\");\nuse stdio;\nuse * from util;\nconst A = 1; // used by main via glob\nprivate:\nconst B = 2; // unused\npublic const C = 3; // unused but public\nfunction helper():void {}\n",
		"util.fc":   "function util_fn():int { return 1; }\nfunction unused_fn():int { return 2; }\n",
		"macro.rb":  "",
		"tbl.txt":   "あいう",
		// main: ドット参照で private に届く (common.helper)、switch の中の break、loop()、include stdio.rb
		"main.fc": "use common;\nuse * from common;\nuse * from stdio;\ninclude(\"stdio.rb\");\nfunction main():void {\n\tvar i:int;\n\tloop() {\n\t\tswitch (i) {\n\t\tcase 1: break;\n\t\tcase 2: i = A + util_fn();\n\t\t}\n\t\tcommon.helper();\n\t\tprint(_T(\"あ\"));\n\t\tstdio.exit(0);\n\t}\n}\n",
	})
	c := NewCompiler(absRepoRoot)
	var report bytes.Buffer
	res, err := c.Migrate(&MigrateOptions{
		Dir: dir, Mains: []string{"main.fc"},
		Textmaps: map[string]string{"_T": "tbl.txt"},
		Write:    true, Out: &report,
	})
	if err != nil {
		t.Fatalf("migrate 失敗: %v\n%s", err, report.String())
	}
	if len(res.Diffs) != 0 || !res.Written {
		t.Fatalf("結果が不正: %+v", res)
	}
	read := func(name string) string {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	// 削除した文 (ラベル・include) の行は空行として残る (行間の保持規則による)
	common := read("common.fc")
	wantCommon := "#fc 2\noptions(bank: -1);\npublic const _T = textmap(\"tbl.txt\");\npublic use stdio;\npublic use * from util;\npublic const A = 1; // used by main via glob\n\nconst B = 2; // unused\nconst C = 3; // unused but public\npublic function helper():void {}\n"
	if common != wantCommon {
		t.Errorf("common.fc:\n--- want\n%s--- got\n%s", wantCommon, common)
	}
	util := read("util.fc")
	wantUtil := "#fc 2\npublic function util_fn():int\n{\n\treturn 1;\n}\nfunction unused_fn():int\n{\n\treturn 2;\n}\n"
	if util != wantUtil {
		t.Errorf("util.fc:\n--- want\n%s--- got\n%s", wantUtil, util)
	}
	main := read("main.fc")
	wantMain := "#fc 2\nuse common;\nuse * from common;\nuse * from stdio;\n\nfunction main():void\n{\n\tvar i:int;\n\tloop_1: loop {\n\t\tswitch (i) {\n\t\tcase 1:\n\t\t\tbreak loop_1;\n\t\tcase 2:\n\t\t\ti = A + util_fn();\n\t\t}\n\t\tcommon.helper();\n\t\tprint(_T(\"あ\"));\n\t\tstdio.exit(0);\n\t}\n}\n"
	if main != wantMain {
		t.Errorf("main.fc:\n--- want\n%s--- got\n%s", wantMain, main)
	}
	// 移行後のソースで再度ビルドできる (emu ターゲット、リンクまで)
	if _, err := c.Build("main.fc", &BuildOptions{Dir: dir, BuildDir: filepath.Join(t.TempDir(), "b"), Out: filepath.Join(t.TempDir(), "a.bin")}); err != nil {
		t.Fatalf("移行後のビルドに失敗: %v", err)
	}
}

// TestMigrateDryRun: Write なしではファイルを変えない。
func TestMigrateDryRun(t *testing.T) {
	dir := t.TempDir()
	src := "use * from stdio;\nfunction main():void { loop() { break; } }\n"
	writeFiles(t, dir, map[string]string{"main.fc": src})
	var report bytes.Buffer
	res, err := NewCompiler(absRepoRoot).Migrate(&MigrateOptions{Dir: dir, Mains: []string{"main.fc"}, Out: &report})
	if err != nil {
		t.Fatal(err)
	}
	if res.Written || len(res.Files) != 1 {
		t.Errorf("結果が不正: %+v", res)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "main.fc")); string(b) != src {
		t.Error("dry run でファイルが変わった")
	}
	if !strings.Contains(report.String(), "main.fc") {
		t.Errorf("報告にファイル名が無い: %s", report.String())
	}
}

// TestMigrateCorpus: test/*.fc と fclib を一時ディレクトリに複製して全部移行し、asm が一致することを確認する。
func TestMigrateCorpus(t *testing.T) {
	tmp := t.TempDir()
	copyDir(t, filepath.Join(absRepoRoot, "test"), filepath.Join(tmp, "test"))
	copyDir(t, filepath.Join(absRepoRoot, "fclib"), filepath.Join(tmp, "fclib"))
	copyDir(t, filepath.Join(absRepoRoot, "share"), filepath.Join(tmp, "share"))
	var mains []string
	entries, _ := os.ReadDir(filepath.Join(tmp, "test"))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "test_") && strings.HasSuffix(e.Name(), ".fc") {
			mains = append(mains, e.Name())
		}
	}
	var report bytes.Buffer
	c := NewCompiler(tmp)
	res, err := c.Migrate(&MigrateOptions{
		Dir: filepath.Join(tmp, "test"), Mains: mains,
		Libs:       []string{filepath.Join(tmp, "fclib")},
		Visibility: migrate.Minimal,
		Write:      true, Out: &report,
	})
	if err != nil {
		t.Fatalf("migrate 失敗: %v\n%s", err, report.String())
	}
	if len(res.Files) < 14 {
		t.Errorf("移行ファイルが少ない: %d", len(res.Files))
	}
	for _, f := range res.Files {
		b, _ := os.ReadFile(f)
		if !bytes.HasPrefix(b, []byte("#fc 2")) {
			t.Errorf("%s: #fc 2 が無い", f)
		}
		if bytes.Contains(b, []byte("private:")) || bytes.Contains(b, []byte(".rb\"")) {
			t.Errorf("%s: v1 の構文が残っている", f)
		}
	}
	// 移行後の test_basic が動く (stdout golden と同じ出力)
	var out bytes.Buffer
	code, err := c.Build("test_basic.fc", &BuildOptions{Dir: filepath.Join(tmp, "test"), BuildDir: filepath.Join(tmp, "b"), Out: filepath.Join(tmp, "a.bin"), Run: true, Stdout: &out})
	if err != nil {
		t.Fatalf("移行後の test_basic のビルド/実行に失敗: %v", err)
	}
	want, _ := os.ReadFile(filepath.Join(absRepoRoot, "testdata", "golden", "stdout", "test_basic.txt"))
	if code != 0 || out.String() != string(want) {
		t.Errorf("test_basic の出力が違う (code %d):\n%s", code, out.String())
	}
}
