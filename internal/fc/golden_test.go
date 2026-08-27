package fc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// golden データは testdata/golden/ (リポジトリルート) にあり、
// tools/gen_golden.rb で Ruby版(オラクル)から生成される。
// 形式は doc/go_port_dump_format.md を参照。

const goldenRoot = "../../testdata/golden"
const repoRoot = "../.."

// テキストgoldenの比較 (git の autocrlf を考慮して CRLF は LF に正規化する)
func normalizeText(s string) string {
	return strings.ReplaceAll(s, "\r\n", "\n")
}

func readGolden(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(goldenRoot, rel))
	if err != nil {
		t.Fatalf("golden読み込み失敗: %v", err)
	}
	return normalizeText(string(b))
}

// 差分の最初の行を報告する
func compareText(t *testing.T, name, got, want string) {
	t.Helper()
	if got == want {
		return
	}
	gotLines := strings.Split(got, "\n")
	wantLines := strings.Split(want, "\n")
	for i := 0; i < len(gotLines) || i < len(wantLines); i++ {
		var g, w string
		if i < len(gotLines) {
			g = gotLines[i]
		}
		if i < len(wantLines) {
			w = wantLines[i]
		}
		if g != w {
			t.Errorf("%s: %d行目が不一致\n got:  %q\n want: %q", name, i+1, g, w)
			return
		}
	}
	t.Errorf("%s: 不一致 (行数 got=%d want=%d)", name, len(gotLines), len(wantLines))
}

// Phase 1: test/test_*.fc + fclib/**/*.fc (x6502除く) のAST一致
func TestGoldenAST(t *testing.T) {
	astRoot := filepath.Join(goldenRoot, "ast")
	err := filepath.WalkDir(astRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".ast") {
			return nil
		}
		rel, _ := filepath.Rel(astRoot, path)
		rel = filepath.ToSlash(rel)
		name := strings.TrimSuffix(rel, ".ast") // 例: test/test_basic, fclib/nes/stdio
		t.Run(name, func(t *testing.T) {
			srcPath := filepath.Join(repoRoot, name+".fc")
			src, err := ReadSource(srcPath)
			if err != nil {
				t.Fatalf("ソース読み込み失敗: %v", err)
			}
			ast, posInfo, err := ParseSrc(src, srcPath)
			if err != nil {
				t.Fatalf("パース失敗: %v", err)
			}
			compareText(t, name+".ast", DumpAST(ast), readGolden(t, "ast/"+rel))
			compareText(t, name+".pos", DumpPos(posInfo), readGolden(t, "ast/"+name+".pos"))
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Phase 3: 全テストの HLC 出力(IR)一致
func TestGoldenIR(t *testing.T) {
	t.Skip("Phase 3 で実装")
}

// Phase 4: レジスタ割付+delete_unuse 後のIR一致
func TestGoldenAllocIR(t *testing.T) {
	t.Skip("Phase 4 で実装")
}

// Phase 5: 正規化済みアセンブラ(.s/.inc)一致
func TestGoldenAsm(t *testing.T) {
	t.Skip("Phase 5 で実装")
}

// Phase 6: リンク済みバイナリ(a.bin/a.nes)のバイト一致
func TestGoldenBinary(t *testing.T) {
	t.Skip("Phase 6 で実装")
}

// Phase 7: エミュレータ実行の stdout / 終了コード一致
func TestGoldenStdout(t *testing.T) {
	t.Skip("Phase 7 で実装")
}
