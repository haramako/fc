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

// fc3Seeds は fc 3 の新しい文法 (for-each、範囲、case の範囲、`..=`、@min など) の種 (2026-09-27。リポジトリの .fc には
// まだ少ない)。
var fc3Seeds = []string{
	"#fc 3\nvar A:[4]u8;\nfunction main():void { for (var x in A) { } for (var i, x in A) { } for (var p in &A) { *p = 1; } }\n",
	"#fc 3\nfunction main():void { for (var i in 0..10) { } for (var j:u16 in 0..=300) { if (j == 5) { break; } } }\n",
	"#fc 3\nfunction f(x:u8):u8 { switch (x) { case 0..4: return 1; case 4..=9, 20: return 2; default: return 3; } }\n",
	"#fc 3\nvar b:[8]u8;\nfunction main():void { var s = b[1..=3]; var n = @min(s[0], 200); L: for (var c in \"ab\") { continue L; } }\n",
	"#fc 3\nenum D { N, E }\nstruct S { a:u8; b:[2]u16; }\nvar ss:[4]S;\nfunction g():*u8 { var x:u8; return &x; }\nfunction main():void { for (var p in &ss) { p.b[1] += 1; } switch (D.N) { case .N..=.E: } }\n",
}

// addFCSeeds はリポジトリ内の .fc ファイルを種として登録する。
func addFCSeeds(f *testing.F) {
	f.Helper()
	for _, src := range fc3Seeds {
		f.Add([]byte(src))
	}
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
