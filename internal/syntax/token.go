// Package syntax は fc 言語の字句解析・構文解析・構文木を提供する。
//
// このパッケージは他の internal パッケージ (sema, ir, types) に依存しない
// (doc/v2_plan.md C3)。フォーマッタ等の純粋な構文ツールがここだけで動くこと。
package syntax

import "fmt"

// Pos はソース上の位置。Line / Col は 1 始まり、Col はバイト単位。
type Pos struct {
	Offset int
	Line   int
	Col    int
}

// IsValid は位置情報が設定されているかを返す。
func (p Pos) IsValid() bool { return p.Line > 0 }

// String は "line:col" 形式。
func (p Pos) String() string { return fmt.Sprintf("%d:%d", p.Line, p.Col) }

// Kind はトークンの種別。
type Kind int

// トークン種別。記号・キーワードの集合は旧レキサ (internal/fc/lexer.go) と同一。
const (
	EOF Kind = iota
	Ident
	Number
	String

	// キーワード
	KwInclude
	KwFunction
	KwConst
	KwVar
	KwOptions
	KwIf
	KwElse
	KwElsif
	KwLoop
	KwWhile
	KwFor
	KwReturn
	KwBreak
	KwContinue
	KwIncbin
	KwSwitch
	KwCase
	KwDefault
	KwUse
	KwAs
	KwFrom
	KwPublic
	KwPrivate

	// 記号 (2 文字)
	Leq    // <=
	Geq    // >=
	EqEq   // ==
	AddEq  // +=
	SubEq  // -=
	Neq    // !=
	Arrow  // ->
	Shl    // <<
	Shr    // >>
	AndAnd // &&
	OrOr   // ||

	// 記号 (1 文字)
	LParen    // (
	RParen    // )
	LBrace    // {
	RBrace    // }
	Semicolon // ;
	Colon     // :
	Lt        // <
	Gt        // >
	LBrack    // [
	RBrack    // ]
	Plus      // +
	Minus     // -
	Star      // *
	Slash     // /
	Percent   // %
	Amp       // &
	Pipe      // |
	Caret     // ^
	Assign    // =
	Comma     // ,
	Dot       // .
	Not       // !
)

var kindNames = [...]string{
	EOF: "EOF", Ident: "Ident", Number: "Number", String: "String",
	KwInclude: "include", KwFunction: "function", KwConst: "const", KwVar: "var",
	KwOptions: "options", KwIf: "if", KwElse: "else", KwElsif: "elsif",
	KwLoop: "loop", KwWhile: "while", KwFor: "for", KwReturn: "return",
	KwBreak: "break", KwContinue: "continue", KwIncbin: "incbin",
	KwSwitch: "switch", KwCase: "case", KwDefault: "default",
	KwUse: "use", KwAs: "as", KwFrom: "from", KwPublic: "public", KwPrivate: "private",
	Leq: "<=", Geq: ">=", EqEq: "==", AddEq: "+=", SubEq: "-=", Neq: "!=", Arrow: "->",
	Shl: "<<", Shr: ">>", AndAnd: "&&", OrOr: "||",
	LParen: "(", RParen: ")", LBrace: "{", RBrace: "}", Semicolon: ";", Colon: ":",
	Lt: "<", Gt: ">", LBrack: "[", RBrack: "]", Plus: "+", Minus: "-", Star: "*",
	Slash: "/", Percent: "%", Amp: "&", Pipe: "|", Caret: "^", Assign: "=",
	Comma: ",", Dot: ".", Not: "!",
}

// String はトークン種別の表示名 (キーワードと記号はその綴り)。
func (k Kind) String() string {
	if int(k) < len(kindNames) && kindNames[k] != "" {
		return kindNames[k]
	}
	return fmt.Sprintf("Kind(%d)", int(k))
}

// IsKeyword はキーワードかを返す。
func (k Kind) IsKeyword() bool { return k >= KwInclude && k <= KwPrivate }

// Token は 1 トークン。Pos は先頭、End は末尾の次の位置。
type Token struct {
	Kind Kind
	Pos  Pos
	End  Pos
	Text string // ソース上の綴り (エスケープ解釈前)
	Int  int    // Kind == Number の値
	Str  string // Kind == String のエスケープ解釈後の値
}

func (t Token) String() string {
	switch t.Kind {
	case Number:
		return fmt.Sprintf("Number(%d)", t.Int)
	case String:
		return fmt.Sprintf("String(%q)", t.Str)
	case Ident:
		return fmt.Sprintf("Ident(%s)", t.Text)
	default:
		return t.Kind.String()
	}
}

// Comment はソース中のコメント。Text は区切り (`//` や `/* */`) を含む原文。
type Comment struct {
	Pos  Pos
	End  Pos
	Text string
}

// IsLine は `//` 形式のコメントかを返す。
func (c Comment) IsLine() bool { return len(c.Text) >= 2 && c.Text[1] == '/' }

// Error はこのパッケージが返す位置付きエラー。
type Error struct {
	Filename string
	Pos      Pos
	Msg      string
}

func (e *Error) Error() string {
	return e.Msg
}
