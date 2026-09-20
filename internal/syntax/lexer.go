package syntax

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

// Lexer は fc ソースの字句解析器。
//
// v1 / v2 共通。コメントは捨てずに Comments() で位置付きで返す (フォーマッタ用)。
// 不正な数値リテラル (`0b2`, `0xZZ`) と不正なエスケープ (`\xZZ`) はエラー。
type Lexer struct {
	src      []byte
	filename string
	off      int // 現在位置 (バイトオフセット)
	line     int // 現在行 (1 始まり)
	lineOff  int // 現在行の先頭オフセット
	comments []Comment
	pragma   string // 先頭行の `#fc 2` プラグマの原文 (行末の改行を含まない。無ければ "")
	verErr   *Error // プラグマの構文エラー (最初の Next で返す)
}

// Version は文法バージョン。fc 1 (2026-09 まで) は削除した。`#fc 2` の宣言は任意 (無くても fc 2)。
const Version = 2

// NewLexer はレキサを作る。src の CRLF は呼び出し側で正規化済みであることを想定するが、
// '\r' は空白として扱うので残っていても動作する。
func NewLexer(src []byte, filename string) *Lexer {
	l := &Lexer{src: src, filename: filename, line: 1}
	l.scanPragma()
	return l
}

// Pragma は先頭行のプラグマの原文 (無ければ "")。
func (l *Lexer) Pragma() string { return l.pragma }

// scanPragma は先頭行の `#fc 2` を読む。`#fc` で始まらなければ何もしない
// (それ以外の `#` は通常の字句解析でエラーになる)。`#fc 1` は fc 1 のソース (もう扱えない) なのでエラー。
func (l *Lexer) scanPragma() {
	if !bytes.HasPrefix(l.src, []byte("#fc")) {
		return
	}
	n := bytes.IndexByte(l.src, '\n')
	if n < 0 {
		n = len(l.src)
	}
	line := string(l.src[:n])
	l.pragma = line
	fields := strings.Fields(line)
	ver := 0
	if len(fields) == 2 && fields[0] == "#fc" {
		ver, _ = strconv.Atoi(fields[1])
	}
	switch ver {
	case Version:
	case 1:
		l.verErr = &Error{Filename: l.filename, Pos: l.pos(), Msg: "fc 1 sources are no longer supported (migrate with `fcc migrate` of fc 0.1 and write `#fc 2`)"}
	default:
		l.verErr = &Error{Filename: l.filename, Pos: l.pos(), Msg: fmt.Sprintf("invalid version pragma %q (expected \"#fc 2\")", line)}
	}
	l.advance(n)
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
	{"<<=", ShlEq}, {">>=", ShrEq}, {"*=", MulEq}, {"/=", DivEq}, {"%=", ModEq}, {"&=", AndEq}, {"|=", OrEq}, {"^=", XorEq}, // v2 (長いものを先に)
	{"<=", Leq}, {">=", Geq}, {"==", EqEq}, {"+=", AddEq}, {"-=", SubEq},
	{"!=", Neq}, {"->", Arrow}, {"<<", Shl}, {">>", Shr},
	{"&&", AndAnd}, {"||", OrOr}, {"++", Inc}, {"--", Dec},
	{"(", LParen}, {")", RParen}, {"{", LBrace}, {"}", RBrace}, {";", Semicolon}, {":", Colon},
	{"<", Lt}, {">", Gt}, {"[", LBrack}, {"]", RBrack}, {"+", Plus}, {"-", Minus},
	{"*", Star}, {"/", Slash}, {"%", Percent}, {"&", Amp}, {"|", Pipe}, {"^", Caret},
	{"=", Assign}, {",", Comma}, {".", Dot}, {"!", Not}, {"~", Tilde},
}

var keywords = map[string]Kind{
	"include": KwInclude, "function": KwFunction, "const": KwConst, "var": KwVar,
	"options": KwOptions, "if": KwIf, "else": KwElse, "elsif": KwElsif,
	"loop": KwLoop, "while": KwWhile, "for": KwFor, "return": KwReturn,
	"break": KwBreak, "continue": KwContinue, "incbin": KwIncbin,
	"switch": KwSwitch, "case": KwCase, "default": KwDefault,
	"use": KwUse, "as": KwAs, "from": KwFrom, "public": KwPublic, "private": KwPrivate,
	"fn": KwFn, "farfn": KwFarFn, "bitcast": KwBitcast, "struct": KwStruct, "sizeof": KwSizeof, "soa": KwSoa,
	"true": KwTrue, "false": KwFalse, "null": KwNull,
}

// v2Keywords は v2 で足した予約語のうち、v1 では識別子として使えていたもの (v1 のソースを壊さない)。
var v2Keywords = map[Kind]bool{KwTrue: true, KwFalse: true, KwNull: true}

// 空白文字 (スペース・タブ・改行など)
func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == '\f' || c == '\v'
}

// 識別子を構成する文字 (ASCII の英数字と _)
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
	if l.verErr != nil {
		return Token{}, l.verErr
	}
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
		n, v, msg := scanNumber(rest)
		if msg != "" {
			return Token{}, &Error{Filename: l.filename, Pos: start, Msg: msg}
		}
		t := tok(Number, n)
		t.Int = v
		return t, nil
	}

	// 識別子 / キーワード
	if isWord(rest[0]) {
		n := 0
		for n < len(rest) && isWord(rest[n]) {
			n++
		}
		if kind, ok := keywords[string(rest[:n])]; ok {
			if kind == KwPrivate {
				// `private` は予約語ではない (宣言はデフォルトで private。fc 1 の `private:` ラベルの名残)
				return tok(Identifier, n), nil
			}
			return tok(kind, n), nil
		}
		return tok(Identifier, n), nil
	}

	// 文字列
	if n, s, msg := scanString(rest); n > 0 {
		if msg != "" {
			return Token{}, &Error{Filename: l.filename, Pos: start, Msg: msg}
		}
		t := tok(String, n)
		t.Str = s
		return t, nil
	}

	return Token{}, &Error{Filename: l.filename, Pos: start, Msg: fmt.Sprintf("invalid token at %d", start.Line)}
}

// scanNumber は数値リテラルを読む。返り値は消費バイト数と値 (msg が非空ならエラー)。
//
//	0x[0-9a-fA-F_]+   16 進    0b[01_]+   2 進    [0-9_]+   10 進
//
// `_` は桁の間の区切りとして許す (`1_000`, `0xab_cd`)。基数に合わない桁 (`0b2`, `0xZZ`) はエラー。
// `12ab` は 12 の後に識別子 ab (C と同じく分かれる)。
func scanNumber(s []byte) (n int, val int, msg string) {
	base := 10
	if len(s) >= 2 && s[0] == '0' && (s[1] == 'x' || s[1] == 'X') {
		base, n = 16, 2
	} else if len(s) >= 2 && s[0] == '0' && (s[1] == 'b' || s[1] == 'B') {
		base, n = 2, 2
	}
	digits := 0
	for n < len(s) {
		c := s[n]
		if c == '_' {
			// 桁の間だけ区切りとして許す
			if digits == 0 || n+1 >= len(s) || digitVal(s[n+1], base) < 0 {
				return n, 0, "invalid digit '_' in numeric literal (allowed only between digits)"
			}
			n++
			continue
		}
		d := digitVal(c, base)
		if d < 0 {
			if base != 10 && isWord(c) {
				return n, 0, fmt.Sprintf("invalid digit %q in base %d literal", c, base)
			}
			break
		}
		val = val*base + d
		digits++
		n++
	}
	if base != 10 && digits == 0 {
		return n, 0, fmt.Sprintf("base %d literal needs digits", base)
	}
	return n, val, ""
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
func scanString(s []byte) (n int, val string, msg string) {
	if bytes.HasPrefix(s, []byte(`"""`)) {
		if idx := bytes.Index(s[3:], []byte(`"""`)); idx >= 0 {
			val, msg = unescape(s[3 : 3+idx])
			return 3 + idx + 3, val, msg
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
					val, msg = unescape(s[1:i])
					return i + 1, val, msg
				}
				return i + 1, string(s[1:i]), ""
			}
			if c == '\\' {
				if i+1 >= len(s) || s[i+1] == '\n' {
					return 0, "", ""
				}
				i += 2
				continue
			}
			i++
		}
	}
	return 0, "", ""
}

// unescape はエスケープを解釈する。\n → 改行、\xNN → バイト (NN は 16 進 2 桁。それ以外はエラー)。
// 他の `\` はそのまま残す。
func unescape(s []byte) (string, string) {
	var out []byte
	i := 0
	for i < len(s) {
		if s[i] == '\\' && i+1 < len(s) {
			if s[i+1] == 'n' {
				out = append(out, '\n')
				i += 2
				continue
			}
			if s[i+1] == 'x' {
				if i+3 >= len(s) || digitVal(s[i+2], 16) < 0 || digitVal(s[i+3], 16) < 0 {
					return "", "invalid \\x escape (needs 2 hex digits)"
				}
				out = append(out, byte(digitVal(s[i+2], 16)*16+digitVal(s[i+3], 16)))
				i += 4
				continue
			}
		}
		out = append(out, s[i])
		i++
	}
	return string(out), ""
}
