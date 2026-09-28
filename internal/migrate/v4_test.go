package migrate

import (
	"strings"
	"testing"

	"github.com/haramako/fc/internal/sema"
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
