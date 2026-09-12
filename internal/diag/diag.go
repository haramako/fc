// Package diag はコンパイラの診断 (エラー) 型を提供する。
package diag

import (
	"fmt"

	"github.com/haramako/fc/internal/syntax"
)

// Error はコンパイルエラー。
//
// エラー処理の規約 (doc/v2_plan.md R2 / C6):
//   - 内部では panic(&diag.Error{...}) で投げてよい。回復点は sema.Hlc.Compile (構文解析・意味解析) と
//     codegen.Llc.Compile (コード生成) の 2 箇所で、そこで Pos が未設定なら処理中の位置を補完する。
//     パッケージ外へは error 値として返る
//   - Error() はメッセージのみ。位置は Pos で提供し、表示の整形は呼び出し側 (CLI) の責務
//   - 文字列の panic はコンパイラ内部の不変条件違反 (バグ) で、回復しない
type Error struct {
	Msg string
	Pos syntax.Position
}

func (e *Error) Error() string { return e.Msg }

// Errorf は書式付きで Error を作る。
func Errorf(format string, args ...any) *Error {
	return &Error{Msg: fmt.Sprintf(format, args...)}
}
