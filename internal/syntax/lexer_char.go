package syntax

import (
	"bytes"
	"fmt"
	"unicode/utf8"
)

// scanChar は fc 4 の文字のリテラル `'A'` を読む。返り値は消費バイト数と文字のコード (UTF-8 の 1 文字の code point。ASCII 以外は
// @textmap の変換器に渡すときだけ意味を持つ: sema)。エスケープは \n / \t / \0 / \\ / \' / \xNN。
func scanChar(s []byte) (n int, val int, msg string) {
	// 閉じの ' を探す (\' と \\ は飛ばす)
	end := -1
	for i := 1; i < len(s); i++ {
		if s[i] == '\n' {
			break
		}
		if s[i] == '\\' {
			i++
			continue
		}
		if s[i] == '\'' {
			end = i
			break
		}
	}
	if end < 0 {
		return 0, 0, `unterminated character literal (in fc 4 '...' is one character; write strings with "...")`
	}
	body := s[1:end]
	n = end + 1
	if len(body) >= 2 && body[0] == '\\' {
		switch {
		case len(body) == 2 && body[1] == 'n':
			return n, '\n', ""
		case len(body) == 2 && body[1] == 't':
			return n, '\t', ""
		case len(body) == 2 && body[1] == '0':
			return n, 0, ""
		case len(body) == 2 && (body[1] == '\\' || body[1] == '\''):
			return n, int(body[1]), ""
		case len(body) == 4 && body[1] == 'x' && digitVal(body[2], 16) >= 0 && digitVal(body[3], 16) >= 0:
			return n, digitVal(body[2], 16)*16 + digitVal(body[3], 16), ""
		}
		return 0, 0, fmt.Sprintf(`invalid escape in character literal '%s' (\n \t \0 \\ \' \xNN)`, body)
	}
	r, size := utf8.DecodeRune(body)
	if len(body) == 0 || size != len(body) || r == utf8.RuneError || bytes.IndexByte(body, '\'') >= 0 {
		return 0, 0, fmt.Sprintf(`'%s' is not one character (in fc 4 '...' is a character literal; write strings with "...")`, body)
	}
	return n, int(r), ""
}
