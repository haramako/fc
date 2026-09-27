package sema

import (
	"strings"
	"testing"
)

// TestParsePO: gettext の .po の読み込み (複数行の文字列・エスケープ・コメント・fuzzy・廃止エントリ・ヘッダー)。
func TestParsePO(t *testing.T) {
	src := "\xef\xbb\xbf" + `# translator comment
msgid ""
msgstr ""
"Language: en\n"
"Content-Type: text/plain; charset=UTF-8\n"

msgid "あい"
msgstr "hi"

msgctxt "greeting"
msgid "あい"
msgstr "hello"

msgctxt ""
msgid "あい"
msgstr "empty ctxt"

#. extracted comment
#: src/a.fc:10
msgid ""
"一行目\n"
"二行目"
msgstr ""
"line 1\n"
"line 2"

#, fuzzy
#| msgid "古い"
msgid "新しい"
msgstr "new"

msgid "未訳"
msgstr ""

msgid "esc"
msgstr "a\\b\"c\td\re"

#~ msgid "廃止"
#~ msgstr "obsolete"
msgid "last"
msgstr "LAST"`
	cat, err := parsePO("t.po", []byte(strings.ReplaceAll(src, "\n", "\r\n")))
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		key    poKey
		want   string
		reason string
	}{
		{poKey{id: "あい"}, "hi", ""},
		{poKey{hasCtxt: true, ctxt: "greeting", id: "あい"}, "hello", ""},
		{poKey{hasCtxt: true, id: "あい"}, "empty ctxt", ""}, // msgctxt "" は msgctxt なしと別
		{poKey{id: "一行目\n二行目"}, "line 1\nline 2", ""},
		{poKey{id: "新しい"}, "", "fuzzy"},
		{poKey{id: "未訳"}, "", "empty msgstr"},
		{poKey{id: "esc"}, "a\\b\"c\td\re", ""},
		{poKey{id: "廃止"}, "", "no entry"},
		{poKey{id: "last"}, "LAST", ""},
		{poKey{id: ""}, "", "no entry"}, // ヘッダーは訳に使わない
		{poKey{hasCtxt: true, ctxt: "nope", id: "あい"}, "", "no entry"},
	}
	for _, c := range cases {
		s, ok, reason := cat.lookup(c.key)
		if s != c.want || ok != (c.reason == "") || reason != c.reason {
			t.Errorf("%+v: got %q %v %q, want %q %q", c.key, s, ok, reason, c.want, c.reason)
		}
	}
	if len(cat.entries) != 8 {
		t.Errorf("エントリの数: %d", len(cat.entries))
	}
}

func TestParsePOErrors(t *testing.T) {
	cases := []struct{ src, want string }{
		{"msgid \"a\"\nmsgid_plural \"as\"\nmsgstr[0] \"x\"\n", "t.po:2: msgid_plural is not supported"},
		{"msgid \"a\"\nmsgstr[0] \"x\"\n", "t.po:2: msgstr[n] (plural forms) is not supported"},
		{"msgid \"a\"\n", "t.po:1: msgid without msgstr"},
		{"msgctxt \"c\"\nmsgstr \"x\"\n", "t.po:2: msgstr without msgid"},
		{"msgid \"a\"\nmsgstr \"x\"\nmsgid \"a\"\nmsgstr \"y\"\n", "t.po:3: duplicate entry"},
		{"msgid \"a\\q\"\nmsgstr \"x\"\n", "t.po:1: msgid: unknown escape \\q"},
		{"msgid \"a\nmsgstr \"x\"\n", "t.po:1: msgid: expected a quoted string"},
		{"\"x\"\n", "t.po:1: a string line must follow"},
		{"msgid \"a\"\nmsgstr \"x\"\n\n\"y\"\n", "t.po:4: a string line must follow"},
		{"foo \"a\"\n", "t.po:1: unknown line"},
	}
	for _, c := range cases {
		_, err := parsePO("t.po", []byte(c.src))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%q: got %v, want /%s/", c.src, err, c.want)
		}
	}
}
