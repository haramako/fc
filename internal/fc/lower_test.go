package fc

// 新パーサ + Lower と旧 goyacc パーサの差分テスト (doc/v2_plan.md R1-b)。
// コーパス全体で (1) AST の S 式が一致、(2) pos_info のキー集合と順序が一致することを確認する。
// pos_info の行番号は意図的に異なる (旧: reduce 時のレキサ行 / 新: 文の開始行) ので比較しない。
// 旧パーサは R1-c で削除されるので、このテストもそのとき削除する。

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/haramako/fc/internal/syntax"
)

func posKeys(m *OMap) string {
	var b strings.Builder
	for _, e := range m.Entries() {
		b.WriteString(Canon(e.Key))
		b.WriteByte('\n')
	}
	return b.String()
}

func TestLowerDiff(t *testing.T) {
	for _, path := range corpusFiles(t) {
		rel, _ := filepath.Rel(absRepoRoot, path)
		t.Run(filepath.ToSlash(rel), func(t *testing.T) {
			src, err := ReadSource(path)
			if err != nil {
				t.Fatal(err)
			}
			oldAst, oldPos, oerr := parseSrcOld(src, path)
			f, nerr := syntax.Parse(src, path)
			if (oerr == nil) != (nerr == nil) {
				t.Fatalf("エラー有無が不一致: old=%v new=%v", oerr, nerr)
			}
			if oerr != nil {
				return // 両方パースエラー (errors.fc)
			}
			newAst, newPos := Lower(f)
			compareText(t, "ast", SexpStr(newAst), SexpStr(oldAst))
			compareText(t, "pos_info keys", posKeys(newPos), posKeys(oldPos))
		})
	}
}

// TestLowerPosInfoLine は pos_info の行番号が文の開始行であることを確認する。
func TestLowerPosInfoLine(t *testing.T) {
	src := []byte("var a:int;\n\nfunction f():void\n{\n  a = 1;\n  if (a) {\n    a = 2;\n  }\n}\n")
	f, err := syntax.Parse(src, "t.fc")
	if err != nil {
		t.Fatal(err)
	}
	_, pos := Lower(f)
	var lines []int
	for _, e := range pos.Entries() {
		lines = append(lines, e.Val.([]any)[1].(int))
	}
	// 登録順は reduce 順 (兄弟は出現順、内側の文が外側より先): var (1), a=1 (5), a=2 (7), if (6), function (3)
	want := []int{1, 5, 7, 6, 3}
	if len(lines) != len(want) {
		t.Fatalf("pos_info 数: got %v want %v", lines, want)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Fatalf("pos_info 行: got %v want %v", lines, want)
		}
	}
}

// TestParseErrorMessage は構文エラーのメッセージが旧実装と同じく "parse error" を含むことを確認する。
func TestParseErrorMessage(t *testing.T) {
	_, _, err := ParseSrc([]byte("hoge fuga\n"), "e.fc")
	if err == nil {
		t.Fatal("エラーになるべき")
	}
	ce, ok := err.(*CompileError)
	if !ok {
		t.Fatalf("*CompileError であるべき: %T", err)
	}
	if !strings.Contains(ce.Msg, "parse error") || ce.Filename != "e.fc" || ce.LineNo != 1 {
		t.Errorf("エラー内容: %+v", ce)
	}
	// 字句エラーも CompileError になる
	_, _, err = ParseSrc([]byte("var a = #;\n"), "e.fc")
	if err == nil || !strings.Contains(err.Error(), "invalid token") {
		t.Errorf("字句エラー: %v", err)
	}
}
