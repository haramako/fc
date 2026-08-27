package fc

import (
	"os"
	"path/filepath"
	"regexp"
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

var reIRComment = regexp.MustCompile(`^\s*; \d{4}:`)

// normalizeAsm は IRコメント行を除去する (dumper.rb の normalize_asm 相当 + 末尾改行)。
func normalizeAsm(lines []string) string {
	var out []string
	for _, line := range lines {
		if reIRComment.MatchString(line) {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n") + "\n"
}

// Phase 5: 正規化済みアセンブラ(.s/.inc)一致
func TestGoldenAsm(t *testing.T) {
	dirs, err := os.ReadDir(filepath.Join(absGoldenRoot, "asm"))
	if err != nil || len(dirs) == 0 {
		t.Fatalf("golden asm が見つからない: %v", err)
	}
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		name := d.Name()
		t.Run(name, func(t *testing.T) {
			srcName, target := goldenKeyInfo(name)
			hlc := compileForGolden(t, srcName, target)
			llc := NewLlc(2)
			for _, me := range hlc.Modules.Entries() {
				mod := me.Val.(*Module)
				asm, inc := llc.Compile(mod)
				compareText(t, name+"/"+ToS(mod.Id)+".s", normalizeAsm(asm),
					readGolden(t, "asm/"+name+"/"+ToS(mod.Id)+".s"))
				compareText(t, name+"/"+ToS(mod.Id)+".inc", normalizeAsm(inc),
					readGolden(t, "asm/"+name+"/"+ToS(mod.Id)+".inc"))
			}
		})
	}
}

// Phase 6: リンク済みバイナリ(a.bin/a.nes)のバイト一致
func TestGoldenBinary(t *testing.T) {
	matches, err := filepath.Glob(filepath.Join(absGoldenRoot, "bin", "*"))
	if err != nil || len(matches) == 0 {
		t.Fatalf("golden bin が見つからない: %v", err)
	}
	for _, m := range matches {
		base := filepath.Base(m)
		name := strings.TrimSuffix(base, filepath.Ext(base))
		t.Run(name, func(t *testing.T) {
			srcName, target := goldenKeyInfo(name)
			t.Chdir(filepath.Join(absRepoRoot, "test"))
			compiler := NewCompiler(absRepoRoot)
			code, berr := compiler.Build(srcName+".fc", &BuildOptions{Target: target})
			if berr != nil {
				t.Fatalf("ビルド失敗: %v", berr)
			}
			if code != 0 {
				t.Fatalf("ビルド結果コード: %d", code)
			}
			outName := "a.bin"
			if target == "nes" {
				outName = "a.nes"
			}
			got, err := os.ReadFile(outName)
			if err != nil {
				t.Fatalf("出力読み込み失敗: %v", err)
			}
			want, err := os.ReadFile(m)
			if err != nil {
				t.Fatalf("golden読み込み失敗: %v", err)
			}
			if len(got) != len(want) {
				t.Fatalf("サイズ不一致: got %d want %d", len(got), len(want))
			}
			for i := range got {
				if got[i] != want[i] {
					t.Fatalf("バイト不一致: offset 0x%04x got %02x want %02x", i, got[i], want[i])
				}
			}
		})
	}
}

// Phase 7: エミュレータ実行の stdout / 終了コード一致
func TestGoldenStdout(t *testing.T) {
	matches, err := filepath.Glob(filepath.Join(absGoldenRoot, "stdout", "*.txt"))
	if err != nil || len(matches) == 0 {
		t.Fatalf("golden stdout が見つからない: %v", err)
	}
	for _, m := range matches {
		name := strings.TrimSuffix(filepath.Base(m), ".txt")
		t.Run(name, func(t *testing.T) {
			srcName, target := goldenKeyInfo(name)
			t.Chdir(filepath.Join(absRepoRoot, "test"))
			compiler := NewCompiler(absRepoRoot)
			var out strings.Builder
			code, berr := compiler.Build(srcName+".fc", &BuildOptions{Target: target, Run: true, Stdout: &out})
			if berr != nil {
				t.Fatalf("ビルド失敗: %v", berr)
			}
			compareText(t, name+".txt", out.String(), readGolden(t, "stdout/"+name+".txt"))
			wantExit := strings.TrimSpace(readGolden(t, "stdout/"+name+".exit"))
			if ToS(code) != wantExit {
				t.Errorf("終了コード不一致: got %d want %s", code, wantExit)
			}
		})
	}
}
