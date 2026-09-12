package driver

import (
	"flag"
	"fmt"
	"github.com/haramako/fc/internal/codegen"
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/regalloc"
	"github.com/haramako/fc/internal/sema"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// golden データは testdata/golden/ (リポジトリルート) にある。
// Go 自身の出力をスナップショットとして保持し、以下で再生成する:
//
//	go test ./internal/fc -run 'TestGolden|TestExample' -update
//
// (移植期は Ruby 版オラクルの tools/gen_golden.rb で生成していた。形式は doc/go_port_dump_format.md)

var update = flag.Bool("update", false, "golden を現在の出力で書き換える")

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

func writeGolden(t *testing.T, rel string, data []byte) {
	t.Helper()
	path := filepath.Join(absGoldenRoot, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o666); err != nil {
		t.Fatalf("golden書き込み失敗: %v", err)
	}
	t.Logf("golden 更新: %s", rel)
}

// compareGolden はテキスト golden (rel は testdata/golden からの相対パス) と比較する。
// -update 時は比較せず got で上書きする (改行は LF)。
func compareGolden(t *testing.T, rel, got string) {
	t.Helper()
	if *update {
		writeGolden(t, rel, []byte(normalizeText(got)))
		return
	}
	compareText(t, rel, got, readGolden(t, rel))
}

// compareGoldenBytes はバイナリ golden と比較する。-update 時は got で上書きする。
func compareGoldenBytes(t *testing.T, rel string, got []byte) {
	t.Helper()
	if *update {
		writeGolden(t, rel, got)
		return
	}
	want, err := os.ReadFile(filepath.Join(absGoldenRoot, rel))
	if err != nil {
		t.Fatalf("golden読み込み失敗: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("%s: サイズ不一致: got %d want %d", rel, len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("%s: バイト不一致: offset 0x%04x got %02x want %02x", rel, i, got[i], want[i])
		}
	}
}

// compileForGolden は test/ ディレクトリで指定テストをHLCコンパイルする。
func compileForGolden(t *testing.T, srcName, target string) *sema.Hlc {
	t.Helper()
	t.Chdir(filepath.Join(absRepoRoot, "test"))
	hlc := sema.NewHlc([]string{".", "../fclib", "../fclib/" + target})
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

// 全テストの HLC 出力(IR)一致
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
			compareGolden(t, "ir/"+name+".ir", ir.DumpProgram(hlc.Options, hlc.Modules.List()))
		})
	}
}

// allocLambdas は LLC と同じ順序 (モジュール順 × defs内のcode順、extern除外) で
// 割付+delete_unuse を実行し、ダンプを返す。
func allocLambdas(hlc *sema.Hlc) string {
	var b strings.Builder
	for _, mod := range hlc.Modules.List() {
		for _, d := range mod.Defs {
			if d.Kind != ir.DefCode || d.Lambda.Extern {
				continue
			}
			regalloc.AllocateRegister(d.Lambda)
			regalloc.DeleteUnuse(d.Lambda)
			b.WriteString(ir.DumpAllocLambda(mod.Id, d.Sym, d.Lambda))
		}
	}
	return b.String()
}

// レジスタ割付+delete_unuse 後のIR一致
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
			compareGolden(t, "allocir/"+name+".air", allocLambdas(hlc))
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

// 正規化済みアセンブラ(.s/.inc)一致
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
			llc := codegen.NewLlc(2, hlc.Types())
			for _, mod := range hlc.Modules.List() {
				asm, inc, err := llc.Compile(mod)
				if err != nil {
					t.Fatalf("コード生成失敗: %v", err)
				}
				compareGolden(t, "asm/"+name+"/"+mod.Id+".s", normalizeAsm(asm))
				compareGolden(t, "asm/"+name+"/"+mod.Id+".inc", normalizeAsm(inc))
			}
		})
	}
}

// リンク済みバイナリ(a.bin/a.nes)のバイト一致
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
			compareGoldenBytes(t, "bin/"+base, got)
		})
	}
}

// エミュレータ実行の stdout / 終了コード一致
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
			compareGolden(t, "stdout/"+name+".txt", out.String())
			compareGolden(t, "stdout/"+name+".exit", fmt.Sprintf("%d\n", code))
		})
	}
}
