package syntax

// パーサとフォーマッタの fuzz テスト。実行は:
//
//	go test ./internal/syntax -fuzz FuzzParse -fuzztime 60s
//	go test ./internal/syntax -fuzz FuzzFormat -fuzztime 60s
//
// 見つかった入力は testdata/fuzz/ に保存され、以後は通常の go test でも回帰テストとして走る。

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// addFCSeeds はリポジトリ内の .fc ファイルを種として登録する。
func addFCSeeds(f *testing.F) {
	f.Helper()
	for _, dir := range []string{"../../test", "../../fclib", "../../examples"} {
		_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".fc") {
				return nil
			}
			if b, err := os.ReadFile(path); err == nil {
				f.Add(b)
			}
			return nil
		})
	}
}

// FuzzParse はどんな入力でも Parse が panic しないことを確かめる。
func FuzzParse(f *testing.F) {
	addFCSeeds(f)
	f.Fuzz(func(t *testing.T, src []byte) {
		_, _ = Parse(src, "fuzz.fc")
	})
}

// FuzzFormat は Format が panic せず、成功したら「出力が再パースできる」
// 「もう一度整形しても変わらない (冪等)」ことを確かめる。
func FuzzFormat(f *testing.F) {
	addFCSeeds(f)
	f.Fuzz(func(t *testing.T, src []byte) {
		out, err := Format(src, "fuzz.fc")
		if err != nil {
			return
		}
		if _, err := Parse(out, "fuzz.fc"); err != nil {
			t.Fatalf("整形結果がパースできない: %v\n--- 入力:\n%s\n--- 出力:\n%s", err, src, out)
		}
		out2, err := Format(out, "fuzz.fc")
		if err != nil {
			t.Fatalf("2回目の整形が失敗: %v\n--- 出力:\n%s", err, out)
		}
		if !bytes.Equal(out, out2) {
			t.Fatalf("整形が冪等でない\n--- 1回目:\n%s\n--- 2回目:\n%s", out, out2)
		}
	})
}
