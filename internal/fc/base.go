// Package fc は FC コンパイラの Go 実装。
// Ruby 版 (lib/fc/, タグ ruby-frozen) の厳密クローンとして移植され、feature/v2 で
// Go らしい構造へ段階的に移行中 (doc/v2_plan.md)。
// 構文木は internal/syntax、型は internal/types、IR は ir.go / value.go / module.go。
package fc

import (
	"bytes"
	"os"
)

// CompileError はコンパイルエラー。Filename / LineNo は Hlc.Compile の回復点で補完される。
type CompileError struct {
	Msg      string
	Filename string
	LineNo   int
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
