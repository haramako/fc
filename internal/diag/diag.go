// Package diag はコンパイラの診断 (エラー・警告) 型を提供する。
package diag

import (
	"fmt"
	"strings"

	"github.com/haramako/fc/internal/syntax"
)

// Error はコンパイルエラー。
//
// エラー処理の規約 (doc/archive/v2_plan.md R2 / C6):
//   - 内部では panic(&diag.Error{...}) で投げてよい。意味解析は文ごとに回復して Program.Errors に集め
//     (複数エラー報告)、次の文へ進む。回復点の最外は sema.CompileModule / CompileBodies と codegen.Llc.Compile で、
//     そこで Pos が未設定なら処理中の位置を補完する。パッケージ外へは error 値 (1 件なら *Error、複数なら ErrorList) として返る
//   - Error() はメッセージのみ。位置は Pos で提供し、表示の整形は呼び出し側 (CLI) の責務
//   - 文字列の panic はコンパイラ内部の不変条件違反 (バグ) で、回復しない
type Error struct {
	Msg string
	Pos syntax.Position

	// Suppressed は「先のエラーの巻き添え」なので報告しないエラー (失敗した宣言の名前を使った箇所など)。
	// 文の処理はここで打ち切るが、一覧には載せない
	Suppressed bool
	// Fatal は文単位の回復をせず、モジュールの処理を打ち切るエラー (エラーが多すぎる、など)
	Fatal bool
}

func (e *Error) Error() string { return e.Msg }

// Warning は警告 (ビルドは続く)。Error と同じく位置付き。
type Warning struct {
	Msg string
	Pos syntax.Position
}

// Errorf は書式付きで Error を作る。
func Errorf(format string, args ...any) *Error {
	return &Error{Msg: fmt.Sprintf(format, args...)}
}

// ErrorList は複数のコンパイルエラー (出現順)。1 件以上あるときだけ error として返す。
// errors.As(err, &e) で *Error を取り出すと最初のエラーになる (Unwrap)。
type ErrorList []*Error

// Error は最初のエラーのメッセージ (残りの件数を添える)。個々の位置付きの表示は CLI が Errors() で行う。
func (l ErrorList) Error() string {
	switch len(l) {
	case 0:
		return "no errors"
	case 1:
		return l[0].Msg
	}
	var b strings.Builder
	b.WriteString(l[0].Msg)
	fmt.Fprintf(&b, " (and %d more error(s))", len(l)-1)
	return b.String()
}

// Unwrap は最初のエラー (errors.As / errors.Is 用)。
func (l ErrorList) Unwrap() error {
	if len(l) == 0 {
		return nil
	}
	return l[0]
}

// Errors は err に含まれる全エラーを返す (*Error なら 1 件、ErrorList なら全部、それ以外は nil)。
func Errors(err error) []*Error {
	switch e := err.(type) {
	case *Error:
		return []*Error{e}
	case ErrorList:
		return e
	}
	return nil
}
