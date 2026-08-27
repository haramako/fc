package fc

// lib/fc/parser_ext.rb のレキサの厳密移植。
// Rubyの実装の癖 (到達不能な数値の -? 、"//\n" が次行を飲み込む、文字列内改行を
// 行番号カウントしない、String#to_i の部分パース等) も 1:1 で再現する。

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
)

// ReadSource はソースファイルを読み込む。
// Ruby版は File.read (テキストモード) で読むため、Windows では CRLF→LF 変換が行われる。
// 同じ挙動になるよう常に CRLF→LF 変換する (オラクルの golden は Windows で生成されている)。
func ReadSource(path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n")), nil
}

type Lexer struct {
	src      []byte
	pos      int
	filename string
	lineNo   int
}

func NewLexer(src []byte, filename string) *Lexer {
	return &Lexer{src: src, filename: filename, lineNo: 1}
}

func (l *Lexer) LineNo() int      { return l.lineNo }
func (l *Lexer) Filename() string { return l.filename }

// 記号トークン。parser_ext.rb:24 の正規表現の並び順を厳守 (先頭一致で最初に当たったもの)。
var symbolTokens = []struct {
	text string
	tok  int
}{
	{"<=", LEQ}, {">=", GEQ}, {"==", EQEQ}, {"+=", ADDEQ}, {"-=", SUBEQ},
	{"!=", NEQ}, {"->", ARROW}, {"<<", LSHIFT}, {">>", RSHIFT},
	{"&&", ANDAND}, {"||", OROR},
	{"(", '('}, {")", ')'}, {"{", '{'}, {"}", '}'}, {";", ';'}, {":", ':'},
	{"<", '<'}, {">", '>'}, {"[", '['}, {"]", ']'}, {"+", '+'}, {"-", '-'},
	{"*", '*'}, {"/", '/'}, {"%", '%'}, {"&", '&'}, {"|", '|'}, {"^", '^'},
	{"=", '='}, {",", ','}, {".", '.'}, {"!", '!'},
}

// キーワード (parser_ext.rb:38 の23語)
var keywordTokens = map[string]int{
	"include": kINCLUDE, "function": kFUNCTION, "const": kCONST, "var": kVAR,
	"options": kOPTIONS, "if": kIF, "else": kELSE, "elsif": kELSIF,
	"loop": kLOOP, "while": kWHILE, "for": kFOR, "return": kRETURN,
	"break": kBREAK, "continue": kCONTINUE, "incbin": kINCBIN,
	"switch": kSWITCH, "case": kCASE, "default": kDEFAULT,
	"use": kUSE, "as": kAS, "from": kFROM, "public": kPUBLIC, "private": kPRIVATE,
}

var (
	reHex   = regexp.MustCompile(`^-?0[xX](\w+)`)
	reBin   = regexp.MustCompile(`^-?0[bB](\d+)`)
	reDec   = regexp.MustCompile(`^-?\d+`)
	reIdent = regexp.MustCompile(`^\w+`)
	reDq    = regexp.MustCompile(`^"(?:[^\\"]|\\.)*"`)
	reSq    = regexp.MustCompile(`^'(?:[^\\']|\\.)*'`)
)

// Rubyの \s 相当
func isRubySpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == '\f' || c == '\v'
}

func (l *Lexer) countNewlines(from, to int) {
	for i := from; i < to; i++ {
		if l.src[i] == '\n' {
			l.lineNo++
		}
	}
}

// コメントと空白を飛ばす ( / \s+ | \/\/.+?\n | \/\*.+?\*\/ /mx 相当 )
func (l *Lexer) skipWS() {
	for {
		if l.pos >= len(l.src) {
			return
		}
		c := l.src[l.pos]
		if isRubySpace(c) {
			p := l.pos
			for p < len(l.src) && isRubySpace(l.src[p]) {
				p++
			}
			l.countNewlines(l.pos, p)
			l.pos = p
			continue
		}
		if c == '/' && l.pos+1 < len(l.src) && l.src[l.pos+1] == '/' {
			// /\/\/.+?\n/m : 内容1文字以上(改行含む)の後の最初の \n。
			// 見つからなければコメントとして成立しない(記号トークンにフォールバック)。
			if l.pos+3 <= len(l.src) {
				idx := bytes.IndexByte(l.src[l.pos+3:], '\n')
				if idx >= 0 {
					end := l.pos + 3 + idx + 1
					l.countNewlines(l.pos, end)
					l.pos = end
					continue
				}
			}
			return
		}
		if c == '/' && l.pos+1 < len(l.src) && l.src[l.pos+1] == '*' {
			// /\/\*.+?\*\//m : 内容1文字以上の後の最初の */
			if l.pos+3 <= len(l.src) {
				idx := bytes.Index(l.src[l.pos+3:], []byte("*/"))
				if idx >= 0 {
					end := l.pos + 3 + idx + 2
					l.countNewlines(l.pos, end)
					l.pos = end
					continue
				}
			}
			return
		}
		return
	}
}

// rubyToI は Ruby の String#to_i(base) 相当 (符号なし部分文字列用)。
// 先頭から有効な数字を読み、無効文字で停止する。'_' は前後が数字の場合のみ区切りとして許す。
func rubyToI(s []byte, base int) int {
	n := 0
	i := 0
	for i < len(s) {
		c := s[i]
		if c == '_' {
			// 直前が数字で、次が数字の場合のみスキップ
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

// unescapeDq は gsub(/\\n|\\x../) 相当のエスケープ処理 ("" と """ 用)。
// \n → 改行、\xNN → バイト(N は改行以外の任意2文字、Ruby to_i(16) 準拠)。それ以外はそのまま。
func unescapeDq(s []byte) string {
	var out []byte
	i := 0
	for i < len(s) {
		if s[i] == '\\' && i+1 < len(s) {
			if s[i+1] == 'n' {
				out = append(out, '\n')
				i += 2
				continue
			}
			// /\\x../ : . は改行以外
			if s[i+1] == 'x' && i+3 < len(s) && s[i+2] != '\n' && s[i+3] != '\n' {
				v := rubyToI(s[i+2:i+4], 16)
				out = append(out, byte(v))
				i += 4
				continue
			}
		}
		out = append(out, s[i])
		i++
	}
	return string(out)
}

// NextToken は次のトークンを返す。EOF は tok=0。
// エラーは Ruby の raise "invalid token at N" 相当。
func (l *Lexer) NextToken() (int, any, error) {
	l.skipWS()
	if l.pos >= len(l.src) {
		return 0, nil, nil
	}
	rest := l.src[l.pos:]

	// 記号
	for _, st := range symbolTokens {
		if bytes.HasPrefix(rest, []byte(st.text)) {
			l.pos += len(st.text)
			return st.tok, st.text, nil
		}
	}

	// 16進数 ( -? は記号が先に消費されるため実際には到達しない )
	if m := reHex.FindSubmatch(rest); m != nil {
		l.pos += len(m[0])
		n := rubyToI(m[1], 16)
		if m[0][0] == '-' {
			n = -n
		}
		return NUMBER, n, nil
	}
	// 2進数 ( キャプチャは \d+ なので 2以上の数字で打ち切られる )
	if m := reBin.FindSubmatch(rest); m != nil {
		l.pos += len(m[0])
		n := rubyToI(m[1], 2)
		if m[0][0] == '-' {
			n = -n
		}
		return NUMBER, n, nil
	}
	// 10進数
	if m := reDec.Find(rest); m != nil {
		l.pos += len(m)
		s := m
		neg := false
		if s[0] == '-' {
			neg = true
			s = s[1:]
		}
		n := rubyToI(s, 10)
		if neg {
			n = -n
		}
		return NUMBER, n, nil
	}

	// 識別子/キーワード
	if m := reIdent.Find(rest); m != nil {
		l.pos += len(m)
		word := string(m)
		if tok, ok := keywordTokens[word]; ok {
			return tok, word, nil
		}
		return IDENT, Sym(word), nil
	}

	// """文字列 ( /"""(.*?)"""/m : 内容0文字以上、最初の """ で終端 )
	if bytes.HasPrefix(rest, []byte(`"""`)) {
		if idx := bytes.Index(rest[3:], []byte(`"""`)); idx >= 0 {
			content := rest[3 : 3+idx]
			l.pos += 3 + idx + 3
			// 文字列内の改行は行番号にカウントしない (Ruby版と同じ)
			return STRING, unescapeDq(content), nil
		}
		// 閉じがなければ "" 文字列としての解釈にフォールバック
	}

	// ""文字列
	if m := reDq.Find(rest); m != nil {
		l.pos += len(m)
		return STRING, unescapeDq(m[1 : len(m)-1]), nil
	}

	// ''文字列 (エスケープ処理なし)
	if m := reSq.Find(rest); m != nil {
		l.pos += len(m)
		return STRING, string(m[1 : len(m)-1]), nil
	}

	return 0, nil, fmt.Errorf("invalid token at %d", l.lineNo)
}
