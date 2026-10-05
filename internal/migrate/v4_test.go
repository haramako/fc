package migrate

import (
	"strings"
	"testing"

	"github.com/haramako/fc/internal/sema"
	"github.com/haramako/fc/internal/syntax"
)

// TestToV4: fc 3 のソースに sema の書き換え (挿入) を当てて `#fc 4` にする。fc 4 はそのまま、fc 2 はエラー (先に fc 3 へ)。
func TestToV4(t *testing.T) {
	src := "#fc 3\nvar d:u16 = a + b;\n"
	s := strings.Index(src, "a + b")
	e := s + len("a + b")
	got, err := ToV4([]byte(src), "t.fc", []sema.Rewrite{
		{Start: s, End: s, Text: "("},
		{Start: e, End: e, Text: ") as u8"},
	})
	if want := "#fc 4\nvar d:u16 = (a + b) as u8;\n"; err != nil || string(got) != want {
		t.Errorf("got %q, %v\nwant %q", got, err, want)
	}

	v4 := "#fc 4\nvar x:u8;\n"
	if got, err := ToV4([]byte(v4), "t.fc", nil); err != nil || string(got) != v4 {
		t.Errorf("fc 4: got %q, %v", got, err)
	}
	if _, err := ToV4([]byte("var x:int;\n"), "t.fc", nil); err == nil || !strings.Contains(err.Error(), "fc 2") {
		t.Errorf("fc 2: err = %v", err)
	}
}

// TestToV4Elsif: fc 4 では elsif をなくした (else if と書く)。migrate は elsif を else if に書き換え、fc 4 の elsif はエラー。
func TestToV4Elsif(t *testing.T) {
	src := "#fc 3\nfunction f(a:u8):u8\n{\n\tif (a == 0) {\n\t\treturn 1;\n\t} elsif (a == 1) {\n\t\treturn 2;\n\t} else {\n\t\treturn 3;\n\t}\n}\n"
	got, err := ToV4([]byte(src), "t.fc", nil)
	if want := strings.Replace(strings.Replace(src, "elsif", "else if", 1), "#fc 3", "#fc 4", 1); err != nil || string(got) != want {
		t.Errorf("got %q, %v\nwant %q", got, err, want)
	}
	if _, err := syntax.Parse([]byte(strings.Replace(src, "#fc 3", "#fc 4", 1)), "t.fc"); err == nil || !strings.Contains(err.Error(), "`elsif` is written `else if` in fc 4") {
		t.Errorf("fc 4 elsif: err = %v", err)
	}
}

// TestToV4DoKeyword: fc 4 で予約語にした do と同じ綴りの名前は do_ にする (sema の書き換えと同じ位置でも `_` が先)。
func TestToV4DoKeyword(t *testing.T) {
	src := "#fc 3\nvar do:u8;\nvar d:u16 = do + 1;\n"
	s := strings.Index(src, "do + 1")
	e := s + len("do + 1")
	got, err := ToV4([]byte(src), "t.fc", []sema.Rewrite{
		{Start: s, End: s, Text: "("},
		{Start: e, End: e, Text: ") as u8"},
	})
	if want := "#fc 4\nvar do_:u8;\nvar d:u16 = (do_ + 1) as u8;\n"; err != nil || string(got) != want {
		t.Errorf("got %q, %v\nwant %q", got, err, want)
	}
	if _, err := syntax.Parse([]byte("#fc 4\nvar do:u8;\n"), "t.fc"); err == nil {
		t.Errorf("fc 4: do should be reserved")
	}
}
