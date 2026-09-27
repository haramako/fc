package driver

// コンパイラ全体 (構文解析〜意味解析〜コード生成、ファイルは書かない) の fuzz テスト。実行は:
//
//	go test ./internal/driver -fuzz FuzzCheck -fuzztime 60s
//
// どんな入力でもエラーで返り、panic しないことを確かめる。
// 見つかった入力は testdata/fuzz/ に保存され、以後は通常の go test でも回帰テストとして走る。

import (
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

func FuzzCheck(f *testing.F) {
	for _, src := range fc3Seeds {
		f.Add([]byte(src))
	}
	for _, dir := range []string{filepath.Join(absRepoRoot, "test"), filepath.Join(absRepoRoot, "examples")} {
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
	f.Fuzz(func(t *testing.T, src []byte) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "t.fc"), src, 0o644); err != nil {
			t.Skip()
		}
		c := NewCompiler(absRepoRoot)
		_, _ = c.Check("t.fc", &CheckOptions{Dir: dir})
	})
}
