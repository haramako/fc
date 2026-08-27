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

// t.Chdir を使うサブテストがあるため、パスは絶対化しておく
var absGoldenRoot, absRepoRoot string

func init() {
	absGoldenRoot, _ = filepath.Abs(goldenRoot)
	absRepoRoot, _ = filepath.Abs(repoRoot)
}

// テキストgoldenの比較 (git の autocrlf を考慮して CRLF は LF に正規化する)
func normalizeText(s string) string {
	return strings.ReplaceAll(s, "\r\n", "\n")
}

func readGolden(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(absGoldenRoot, rel))
	if err != nil {
		t.Fatalf("golden読み込み失敗: %v", err)
	}
	return normalizeText(string(b))
}

// compileForGolden は test/ ディレクトリで指定テストをHLCコンパイルする。
func compileForGolden(t *testing.T, srcName, target string) *Hlc {
	t.Helper()
	t.Chdir(filepath.Join(absRepoRoot, "test"))
	hlc := NewHlc([]string{".", "../fclib", "../fclib/" + target})
	if err := hlc.Compile(srcName + ".fc"); err != nil {
		t.Fatalf("コンパイル失敗: %v", err)
	}
	return hlc
}

// goldenKey は golden のキー名 (test_basic / test_basic_nes) からソース名とターゲットを得る。
func goldenKeyInfo(name string) (srcName, target string) {
	if strings.HasSuffix(name, "_nes") {
		return strings.TrimSuffix(name, "_nes"), "nes"
	}
	return name, "emu"
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
	matches, err := filepath.Glob(filepath.Join(absGoldenRoot, "ir", "*.ir"))
	if err != nil || len(matches) == 0 {
		t.Fatalf("golden ir が見つからない: %v", err)
	}
	for _, m := range matches {
		name := strings.TrimSuffix(filepath.Base(m), ".ir")
		t.Run(name, func(t *testing.T) {
			srcName, target := goldenKeyInfo(name)
			hlc := compileForGolden(t, srcName, target)
			compareText(t, name+".ir", DumpIR(hlc), readGolden(t, "ir/"+name+".ir"))
		})
	}
}

// allocLambdas は LLC と同じ順序 (モジュール順 × defs内のcode順、extern除外) で
// 割付+delete_unuse を実行し、ダンプを返す。
func allocLambdas(hlc *Hlc) string {
	var b strings.Builder
	for _, me := range hlc.Modules.Entries() {
		mod := me.Val.(*Module)
		for _, d := range mod.Defs {
			if d.Kind != "code" {
				continue
			}
			lmd := d.Val.(*Lambda)
			if truthy(lmd.Opt.GetOr(Sym("extern"))) {
				continue
			}
			AllocateRegister(lmd)
			DeleteUnuse(lmd)
			b.WriteString(DumpAllocLambda(mod.Id, d.Sym, lmd))
		}
	}
	return b.String()
}

// Phase 4: レジスタ割付+delete_unuse 後のIR一致
func TestGoldenAllocIR(t *testing.T) {
	matches, err := filepath.Glob(filepath.Join(absGoldenRoot, "allocir", "*.air"))
	if err != nil || len(matches) == 0 {
		t.Fatalf("golden allocir が見つからない: %v", err)
	}
	for _, m := range matches {
		name := strings.TrimSuffix(filepath.Base(m), ".air")
		t.Run(name, func(t *testing.T) {
			srcName, target := goldenKeyInfo(name)
			hlc := compileForGolden(t, srcName, target)
			compareText(t, name+".air", allocLambdas(hlc), readGolden(t, "allocir/"+name+".air"))
		})
	}
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
