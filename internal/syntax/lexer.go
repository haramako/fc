package syntax

import (
	"bytes"
	"fmt"
)

// Lexer は fc ソースの字句解析器。
//
// 字句規則は旧レキサ (internal/fc/lexer.go、Ruby 版 parser_ext.rb の厳密移植) と
// 同一のトークン列を生成する。Ruby 由来の癖 (数値リテラルの部分パース、`0b2` の容認、
// `\xZZ` → 0 等) も保存する (doc/v2_plan.md §2.5)。旧レキサと意図的に異なる点:
//
//   - コメントを捨てずに Comments() で位置付きで返す (フォーマッタ用)
//   - `//` コメントは行末まで。空の `//` が次行を飲み込まない
//   - 空のブロックコメント `/**/` を受理する
//   - 位置 (行・列) は文字列リテラル内の改行も正確に数える
type Lexer struct {
	src      []byte
	filename string
	off      int // 現在位置 (バイトオフセット)
	line     int // 現在行 (1 始まり)
	lineOff  int // 現在行の先頭オフセット
	comments []Comment
}

// NewLexer はレキサを作る。src の CRLF は呼び出し側で正規化済みであることを想定するが、
// '\r' は空白として扱うので残っていても動作する。
func NewLexer(src []byte, filename string) *Lexer {
	return &Lexer{src: src, filename: filename, line: 1}
}

// Filename はソースファイル名。
func (l *Lexer) Filename() string { return l.filename }

// Comments はこれまでに読み飛ばしたコメントを出現順に返す。
func (l *Lexer) Comments() []Comment { return l.comments }

// Tokenize は src 全体を字句解析してトークン列 (EOF を含まない) とコメント列を返す。
func Tokenize(src []byte, filename string) ([]Token, []Comment, error) {
	l := NewLexer(src, filename)
	var toks []Token
	for {
		t, err := l.Next()
		if err != nil {
			return nil, nil, err
		}
		if t.Kind == EOF {
			return toks, l.comments, nil
		}
		toks = append(toks, t)
	}
}

// 記号トークン。旧レキサの並び順を厳守 (先頭一致で最初に当たったもの)。
var symbolTokens = []struct {
	text string
	kind Kind
}{
	{"<=", Leq}, {">=", Geq}, {"==", EqEq}, {"+=", AddEq}, {"-=", SubEq},
	{"!=", Neq}, {"->", Arrow}, {"<<", Shl}, {">>", Shr},
	{"&&", AndAnd}, {"||", OrOr},
	{"(", LParen}, {")", RParen}, {"{", LBrace}, {"}", RBrace}, {";", Semicolon}, {":", Colon},
	{"<", Lt}, {">", Gt}, {"[", LBrack}, {"]", RBrack}, {"+", Plus}, {"-", Minus},
	{"*", Star}, {"/", Slash}, {"%", Percent}, {"&", Amp}, {"|", Pipe}, {"^", Caret},
	{"=", Assign}, {",", Comma}, {".", Dot}, {"!", Not},
}

var keywords = map[string]Kind{
	"include": KwInclude, "function": KwFunction, "const": KwConst, "var": KwVar,
	"options": KwOptions, "if": KwIf, "else": KwElse, "elsif": KwElsif,
	"loop": KwLoop, "while": KwWhile, "for": KwFor, "return": KwReturn,
	"break": KwBreak, "continue": KwContinue, "incbin": KwIncbin,
	"switch": KwSwitch, "case": KwCase, "default": KwDefault,
	"use": KwUse, "as": KwAs, "from": KwFrom, "public": KwPublic, "private": KwPrivate,
}

// Ruby の \s 相当
func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == '\f' || c == '\v'
}

// Ruby の \w 相当 (ASCII)
func isWord(c byte) bool {
	return c == '_' || (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// pos は現在位置。
func (l *Lexer) pos() Pos {
	return Pos{Offset: l.off, Line: l.line, Col: l.off - l.lineOff + 1}
}

// advance は n バイト進める (改行を数える)。
func (l *Lexer) advance(n int) {
	end := l.off + n
	for i := l.off; i < end; i++ {
		if l.src[i] == '\n' {
			l.line++
			l.lineOff = i + 1
		}
	}
	l.off = end
}

func (l *Lexer) peek(i int) byte {
	if l.off+i < len(l.src) {
		return l.src[l.off+i]
	}
	return 0
}

// skipSpaceAndComments は空白とコメントを読み飛ばし、コメントは記録する。
func (l *Lexer) skipSpaceAndComments() {
	for l.off < len(l.src) {
		c := l.src[l.off]
		switch {
		case isSpace(c):
			n := 0
			for l.off+n < len(l.src) && isSpace(l.src[l.off+n]) {
				n++
			}
			l.advance(n)
		case c == '/' && l.peek(1) == '/':
			// 行末まで (改行は含まない)
			n := 2
			for l.off+n < len(l.src) && l.src[l.off+n] != '\n' {
				n++
			}
			l.addComment(n)
		case c == '/' && l.peek(1) == '*':
			// 最初の */ まで。閉じていなければコメントとして成立しない (記号にフォールバック)
			idx := bytes.Index(l.src[l.off+2:], []byte("*/"))
			if idx < 0 {
				return
			}
			l.addComment(2 + idx + 2)
		default:
			return
		}
	}
}

func (l *Lexer) addComment(n int) {
	start := l.pos()
	text := string(l.src[l.off : l.off+n])
	l.advance(n)
	l.comments = append(l.comments, Comment{Pos: start, End: l.pos(), Text: text})
}

// Next は次のトークンを返す。入力の終わりでは Kind == EOF。
func (l *Lexer) Next() (Token, error) {
	l.skipSpaceAndComments()
	start := l.pos()
	if l.off >= len(l.src) {
		return Token{Kind: EOF, Pos: start, End: start}, nil
	}
	rest := l.src[l.off:]

	tok := func(kind Kind, n int) Token {
		text := string(rest[:n])
		l.advance(n)
		return Token{Kind: kind, Pos: start, End: l.pos(), Text: text}
	}

	// 記号
	for _, st := range symbolTokens {
		if bytes.HasPrefix(rest, []byte(st.text)) {
			return tok(st.kind, len(st.text)), nil
		}
	}

	// 数値 ( 旧レキサの -? は記号が先に消費されるため到達しない )
	if isDigit(rest[0]) {
		if n, v, ok := scanNumber(rest); ok {
			t := tok(Number, n)
			t.Int = v
			return t, nil
		}
	}

	// 識別子 / キーワード
	if isWord(rest[0]) {
		n := 0
		for n < len(rest) && isWord(rest[n]) {
			n++
		}
		word := string(rest[:n])
		if kind, ok := keywords[word]; ok {
			return tok(kind, n), nil
		}
		return tok(Ident, n), nil
	}

	// 文字列
	if n, s, ok := scanString(rest); ok {
		t := tok(String, n)
		t.Str = s
		return t, nil
	}

	return Token{}, &Error{Filename: l.filename, Pos: start, Msg: fmt.Sprintf("invalid token at %d", start.Line)}
}

// scanNumber は数値リテラルを読む。返り値は消費バイト数と値。
// 旧レキサの正規表現 ( ^0[xX](\w+) | ^0[bB](\d+) | ^\d+ ) と Ruby の String#to_i(base) の
// 部分パース挙動を保存する: 無効な桁は値としては無視されるが文字としては消費される
// (例: `0xZZ` → 0、`0b2` → 0、`12ab` → 12 の後に識別子 `ab`)。
func scanNumber(s []byte) (n int, val int, ok bool) {
	if len(s) >= 3 && s[0] == '0' && (s[1] == 'x' || s[1] == 'X') && isWord(s[2]) {
		n = 2
		for n < len(s) && isWord(s[n]) {
			n++
		}
		return n, rubyToI(s[2:n], 16), true
	}
	if len(s) >= 3 && s[0] == '0' && (s[1] == 'b' || s[1] == 'B') && isDigit(s[2]) {
		n = 2
		for n < len(s) && isDigit(s[n]) {
			n++
		}
		return n, rubyToI(s[2:n], 2), true
	}
	for n < len(s) && isDigit(s[n]) {
		n++
	}
	if n == 0 {
		return 0, 0, false
	}
	return n, rubyToI(s[:n], 10), true
}

// rubyToI は Ruby の String#to_i(base) 相当 (符号なし部分文字列用)。
// 先頭から有効な数字を読み、無効文字で停止する。'_' は前後が数字の場合のみ区切りとして許す。
func rubyToI(s []byte, base int) int {
	n := 0
	i := 0
	for i < len(s) {
		c := s[i]
		if c == '_' {
			if i == 0 || digitVal(s[i-1], base) < 0 {
				break
			}
			if i+1 >= len(s) || digitVal(s[i+1], base) < 0 {
				break
			}
			i++
			continue
		}
		d := digitVal(c, base)
		if d < 0 {
			break
		}
		n = n*base + d
		i++
	}
	return n
}

func digitVal(c byte, base int) int {
	var d int
	switch {
	case c >= '0' && c <= '9':
		d = int(c - '0')
	case c >= 'a' && c <= 'z':
		d = int(c-'a') + 10
	case c >= 'A' && c <= 'Z':
		d = int(c-'A') + 10
	default:
		return -1
	}
	if d >= base {
		return -1
	}
	return d
}

// scanString は文字列リテラルを読む。返り値は消費バイト数と解釈後の値。
// 形式は旧レキサと同一: `"""..."""` (複数行・エスケープ解釈)、`"..."` (エスケープ解釈)、
// `'...'` (エスケープ解釈なし)。
func scanString(s []byte) (n int, val string, ok bool) {
	if bytes.HasPrefix(s, []byte(`"""`)) {
		if idx := bytes.Index(s[3:], []byte(`"""`)); idx >= 0 {
			return 3 + idx + 3, unescape(s[3 : 3+idx]), true
		}
		// 閉じがなければ "" としての解釈にフォールバック
	}
	if s[0] == '"' || s[0] == '\'' {
		// /"(?:[^\\"]|\\.)*"/ : `.` は改行以外、[^\\"] は改行を含む
		q := s[0]
		i := 1
		for i < len(s) {
			c := s[i]
			if c == q {
				if q == '"' {
					return i + 1, unescape(s[1:i]), true
				}
				return i + 1, string(s[1:i]), true
			}
			if c == '\\' {
				if i+1 >= len(s) || s[i+1] == '\n' {
					return 0, "", false
				}
				i += 2
				continue
			}
			i++
		}
	}
	return 0, "", false
}

// unescape は gsub(/\\n|\\x../) 相当のエスケープ処理。
// \n → 改行、\xNN → バイト (N は改行以外の任意 2 文字、Ruby to_i(16) 準拠で不正なら 0)。それ以外はそのまま。
func unescape(s []byte) string {
	var out []byte
	i := 0
	for i < len(s) {
		if s[i] == '\\' && i+1 < len(s) {
			if s[i+1] == 'n' {
				out = append(out, '\n')
				i += 2
				continue
			}
			if s[i+1] == 'x' && i+3 < len(s) && s[i+2] != '\n' && s[i+3] != '\n' {
				out = append(out, byte(rubyToI(s[i+2:i+4], 16)))
				i += 4
				continue
			}
		}
		out = append(out, s[i])
		i++
	}
	return string(out)
}
