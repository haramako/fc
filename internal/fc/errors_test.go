package fc

// test/test-all の「エラーが起こるソースのテスト」の移植。
// errors.fc を //@ 区切りでパースし、各断片が期待の正規表現に一致する
// CompileError を出すことを確認する。
//
// 注: 現行の Ruby 版 test-all は「例外が出なかった」ケースを失敗にしていないが、
// 全断片がエラーになることを確認済みのため、Go 版では厳格化して
// 「コンパイルが通ってしまったら失敗」とする。

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const errorsCommon = `function interrupt():void options(symbol:"_interrupt"){}
function interrupt_irq():void options(symbol:"_interrupt_irq"){}
function main():void{}
`

func TestErrorsFC(t *testing.T) {
	txt, err := ReadSource(filepath.Join(absRepoRoot, "test", "errors.fc"))
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
			t.Chdir(filepath.Join(absRepoRoot, "test"))
			tmp := t.TempDir()
			path := filepath.Join(tmp, "fail_test.fc")
			if err := os.WriteFile(path, []byte(src+"\n"+errorsCommon), 0o666); err != nil {
				t.Fatal(err)
			}
			compiler := NewCompiler(absRepoRoot)
			_, berr := compiler.Build(path, &BuildOptions{})
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
			// CompileError は断片内 (errorsCommon より前) の位置を指していること。
			// 外部コマンド (ca65) のエラーは CommandError で位置を持たない
			if ce, ok := berr.(*CompileError); ok {
				fragLines := strings.Count(src, "\n") + 1
				if !ce.Pos.IsValid() || ce.Pos.Line > fragLines || !strings.HasSuffix(ce.Pos.Filename, "fail_test.fc") {
					t.Errorf("断片 %d: 位置が不正 %s (断片は %d 行)", i, ce.Pos, fragLines)
				}
			} else if _, ok := berr.(*CommandError); !ok {
				t.Errorf("断片 %d: エラー型が不正 %T", i, berr)
			}
		})
	}
}
