// Package fc は FC コンパイラの Go 実装。
// Ruby 版 (lib/fc/, タグ ruby-frozen) の厳密クローンとして移植され、feature/v2 で
// Go らしい構造へ段階的に移行中 (doc/v2_plan.md)。
// 構文木は internal/syntax、型は internal/types、IR は ir.go / value.go / module.go。
package fc

import (
	"bytes"
	"os"

	"github.com/haramako/fc/internal/syntax"
)

// CompileError はコンパイルエラー。
//
// エラー処理の規約 (doc/v2_plan.md R2 / C6):
//   - 内部では panic(&CompileError{...}) で投げてよい。回復点は Hlc.Compile (構文解析・意味解析) と
//     Llc.Compile (コード生成) の 2 箇所で、そこで Pos が未設定なら処理中の位置を補完する。
//     パッケージ外へは error 値として返る
//   - Error() はメッセージのみ。位置は Pos で提供し、表示の整形は呼び出し側 (CLI) の責務
//   - 文字列の panic はコンパイラ内部の不変条件違反 (バグ) で、回復しない
type CompileError struct {
	Msg string
	Pos syntax.Position
}

func (e *CompileError) Error() string { return e.Msg }

// ReadSource はソースファイルを読み込む。
// Ruby版は File.read (テキストモード) で読むため、Windows では CRLF→LF 変換が行われる。
// 同じ挙動になるよう常に CRLF→LF 変換する (golden は Windows で生成されている)。
func ReadSource(path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n")), nil
}
