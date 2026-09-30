package sema

// gettext の .po の読み込み (textmap の翻訳。docs/reference/language.md の「独自の文字表と翻訳」)。
//
// 扱うもの: msgctxt / msgid / msgstr (複数行に分かれた文字列、エスケープ \\ \" \n \t \r)、コメント (# #. #: #|)、
// フラグ行 `#, fuzzy`、廃止エントリ `#~` (無視する)、ヘッダー (msgctxt の無い msgid ""。訳には使わない)。
// msgid_plural / msgstr[n] は使わないのでエラーにする。

import (
	"fmt"
	"strings"

	"github.com/haramako/fc/internal/diag"
)

// poKey は照合のキー。PO では「msgctxt なし」と「msgctxt が空文字列」は別物なので hasCtxt で区別する。
type poKey struct {
	hasCtxt bool
	ctxt    string
	id      string
}

// poEntry は 1 エントリの訳。
type poEntry struct {
	str   string
	fuzzy bool
}

// poCatalog は読んだ .po の訳の表。
type poCatalog struct {
	path    string
	entries map[poKey]*poEntry
}

// lookup は (ctxt, id) の訳を返す。使えない (エントリが無い・fuzzy・msgstr が空) なら ok が false で、reason がその理由。
func (c *poCatalog) lookup(k poKey) (str string, ok bool, reason string) {
	e := c.entries[k]
	switch {
	case e == nil:
		return "", false, "no entry"
	case e.fuzzy:
		return "", false, "fuzzy"
	case e.str == "":
		return "", false, "empty msgstr"
	}
	return e.str, true, ""
}

// poPending は読んでいる途中のエントリ。
type poPending struct {
	key    poKey
	hasID  bool
	str    string
	hasStr bool
	fuzzy  bool
	line   int
}

// parsePO は .po の中身を読む。path はエラーメッセージ用。
func parsePO(path string, data []byte) (*poCatalog, error) {
	cat := &poCatalog{path: path, entries: map[poKey]*poEntry{}}
	text := strings.TrimPrefix(string(data), "\xef\xbb\xbf") // UTF-8 の BOM
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	fail := func(ln int, format string, args ...any) error {
		return &diag.Error{Msg: fmt.Sprintf("%s:%d: %s", path, ln, fmt.Sprintf(format, args...))}
	}

	var (
		cur          *poPending
		pendingFuzzy bool    // 次のエントリに付くフラグ (`#, fuzzy` はエントリの前に書く)
		field        *string // 続きの "..." 行を足す先 (nil なら続きの行は書けない)
		fieldName    string
	)
	// flush は読み終えたエントリを表に入れる。
	flush := func() error {
		if cur == nil {
			return nil
		}
		e := cur
		cur, field = nil, nil
		if !e.hasID {
			return fail(e.line, "msgctxt without msgid")
		}
		if !e.hasStr {
			return fail(e.line, "msgid without msgstr")
		}
		if !e.key.hasCtxt && e.key.id == "" {
			return nil // ヘッダー
		}
		if _, dup := cat.entries[e.key]; dup {
			return fail(e.line, "duplicate entry (msgctxt %q, msgid %q)", e.key.ctxt, e.key.id)
		}
		cat.entries[e.key] = &poEntry{str: e.str, fuzzy: e.fuzzy}
		return nil
	}
	// start は新しいエントリを始める (読んでいたエントリは終える)。
	start := func(ln int) error {
		if err := flush(); err != nil {
			return err
		}
		cur = &poPending{line: ln, fuzzy: pendingFuzzy}
		pendingFuzzy = false
		return nil
	}

	for i, raw := range lines {
		ln := i + 1
		line := strings.TrimSpace(raw)
		switch {
		case line == "":
			field = nil
			continue
		case strings.HasPrefix(line, "#"):
			// コメント・フラグはエントリの前に書く: msgstr まで読んだエントリはここで終わる
			if cur != nil && cur.hasStr {
				if err := flush(); err != nil {
					return nil, err
				}
			}
			field = nil
			if strings.HasPrefix(line, "#,") {
				for _, f := range strings.Split(line[2:], ",") {
					if strings.TrimSpace(f) == "fuzzy" {
						pendingFuzzy = true
					}
				}
			}
			continue // #  #.  #:  #|  #~ (廃止エントリ) は読み飛ばす
		case strings.HasPrefix(line, "\""):
			if field == nil {
				return nil, fail(ln, "a string line must follow msgctxt, msgid or msgstr")
			}
			s, err := poUnquote(line)
			if err != nil {
				return nil, fail(ln, "%s: %v", fieldName, err)
			}
			*field += s
			continue
		}
		kw, rest, _ := strings.Cut(line, " ")
		rest = strings.TrimSpace(rest)
		switch {
		case kw == "msgctxt":
			if err := start(ln); err != nil {
				return nil, err
			}
			cur.key.hasCtxt = true
			field = &cur.key.ctxt
		case kw == "msgid":
			if cur == nil || cur.hasID {
				if err := start(ln); err != nil {
					return nil, err
				}
			}
			cur.hasID = true
			field = &cur.key.id
		case kw == "msgstr":
			if cur == nil || !cur.hasID || cur.hasStr {
				return nil, fail(ln, "msgstr without msgid")
			}
			cur.hasStr = true
			field = &cur.str
		case kw == "msgid_plural":
			return nil, fail(ln, "msgid_plural is not supported")
		case strings.HasPrefix(kw, "msgstr["):
			return nil, fail(ln, "msgstr[n] (plural forms) is not supported")
		default:
			return nil, fail(ln, "unknown line %q", line)
		}
		fieldName = kw
		s, err := poUnquote(rest)
		if err != nil {
			return nil, fail(ln, "%s: %v", kw, err)
		}
		*field = s
	}
	if err := flush(); err != nil {
		return nil, err
	}
	return cat, nil
}

// poUnquote は "..." の文字列を読む (エスケープ \\ \" \n \t \r)。
func poUnquote(s string) (string, error) {
	if len(s) < 2 || s[0] != '"' || s[len(s)-1] != '"' {
		return "", fmt.Errorf("expected a quoted string, got %q", s)
	}
	s = s[1 : len(s)-1]
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '"':
			return "", fmt.Errorf("unescaped \" in a string")
		case '\\':
			i++
			if i >= len(s) {
				return "", fmt.Errorf("string ends with \\")
			}
			switch s[i] {
			case '\\':
				b.WriteByte('\\')
			case '"':
				b.WriteByte('"')
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'r':
				b.WriteByte('\r')
			default:
				return "", fmt.Errorf("unknown escape \\%c", s[i])
			}
		default:
			b.WriteByte(c)
		}
	}
	return b.String(), nil
}
