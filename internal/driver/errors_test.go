package driver

// test/errors.fc の「エラーが起こるソースのテスト」。
//
// errors.fc は `//@<正規表現>` で始まる断片の並びで、各断片がその正規表現に一致する
// エラーを出すこと (コンパイルが通ってしまったら失敗) を確認する。
// エラーの期待位置は断片内のエラー行の末尾に `//!` マーカーを置いて示す
// (列も見るなら `//! col=N`)。マーカーのない断片は「断片内を指していること」だけ検査する。

import (
	"errors"
	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/sema"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

const errorsCommon = `function interrupt():void options(symbol:"_interrupt"){}
function interrupt_irq():void options(symbol:"_interrupt_irq"){}
function main():void{}
`

func TestErrorsFC(t *testing.T) {
	txt, err := sema.ReadSource(filepath.Join(absRepoRoot, "test", "errors.fc"))
	if err != nil {
		t.Fatal(err)
	}
	frags := regexp.MustCompile(`(?m)^//@`).Split(string(txt), -1)[1:]
	if len(frags) == 0 {
		t.Fatal("errors.fc に断片がない")
	}

	for i, frag := range frags {
		parts := strings.SplitN(frag, "\n", 2)
		expectedErr := parts[0]
		src := ""
		if len(parts) > 1 {
			src = parts[1]
		}
		t.Run(regexp.MustCompile(`\W+`).ReplaceAllString(expectedErr, "_"), func(t *testing.T) {
			t.Parallel()
			tmp := t.TempDir()
			path := filepath.Join(tmp, "fail_test.fc")
			if err := os.WriteFile(path, []byte(src+"\n"+errorsCommon), 0o666); err != nil {
				t.Fatal(err)
			}
			compiler := NewCompiler(absRepoRoot)
			// 断片は一時ディレクトリ、use/include の検索は test/ 基準 (旧 test-all と同じ)
			_, berr := compiler.Build(path, &BuildOptions{Dir: testDir(), BuildDir: filepath.Join(tmp, "build"), Out: filepath.Join(tmp, "a.bin")})
			if berr == nil {
				t.Fatalf("断片 %d: エラーになるべきコンパイルが成功した", i)
			}
			re, rerr := regexp.Compile(expectedErr)
			if rerr != nil {
				t.Fatalf("期待正規表現が不正: %v", rerr)
			}
			if !re.MatchString(berr.Error()) {
				t.Errorf("断片 %d: エラーメッセージ不一致\n expected: /%s/\n got: %s", i, expectedErr, berr.Error())
			}
			// CompileError は断片内 (errorsCommon より前) の位置、マーカーがあればその行 (と列) を指していること。
			// 外部コマンド (ca65) のエラーは CommandError で位置を持たない
			var ce *diag.Error
			if errors.As(berr, &ce) {
				fragLines := strings.Count(src, "\n") + 1
				if !ce.Pos.IsValid() || ce.Pos.Line > fragLines || !strings.HasSuffix(ce.Pos.Filename, "fail_test.fc") {
					t.Errorf("断片 %d: 位置が不正 %s (断片は %d 行)", i, ce.Pos, fragLines)
				}
				if line, col, ok := errorMarker(src); ok {
					if ce.Pos.Line != line || (col > 0 && ce.Pos.Col != col) {
						t.Errorf("断片 %d: 位置不一致\n expected: line %d col %d (//! マーカー)\n got: line %d col %d", i, line, col, ce.Pos.Line, ce.Pos.Col)
					}
				}
			} else if _, ok := berr.(*CommandError); !ok {
				t.Errorf("断片 %d: エラー型が不正 %T", i, berr)
			}
		})
	}
}

// TestLlcErrorPosition: コード生成時のエラーは生成元の式の位置を指す (位置を持たない命令では関数の宣言位置)。
func TestLlcErrorPosition(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "t.fc")
	src := "var x:int;\n\nfunction main():void\n{\n  x = x / 0;\n}\n"
	if err := os.WriteFile(path, []byte(src), 0o666); err != nil {
		t.Fatal(err)
	}
	compiler := NewCompiler(absRepoRoot)
	_, err := compiler.Build("t.fc", &BuildOptions{Target: "emu", CompileOnly: true, Dir: dir})
	ce, ok := err.(*diag.Error)
	if !ok {
		t.Fatalf("CompileError であるべき: %v", err)
	}
	// LLC で検出されるエラーは、生成元の式 (`x / 0`) の位置を指す (ir.Op.Pos)
	if !strings.Contains(ce.Msg, "div by 0") || ce.Pos.Line != 5 || ce.Pos.Col != 7 {
		t.Errorf("got %+v", ce)
	}
}

var errorMarkerRe = regexp.MustCompile(`//!(?:\s*col=(\d+))?`)

// errorMarker は断片内の `//!` マーカーの行 (1 始まり) と列 (指定がなければ 0) を返す。
func errorMarker(src string) (line, col int, ok bool) {
	for i, l := range strings.Split(src, "\n") {
		if m := errorMarkerRe.FindStringSubmatch(l); m != nil {
			if m[1] != "" {
				col, _ = strconv.Atoi(m[1])
			}
			return i + 1, col, true
		}
	}
	return 0, 0, false
}
