package cc65

import "testing"

// TestDbgFields: dbgfile のレコードのフィールドの読み方 (引用符の中の , は区切りでない。値の引用符は外す)。
func TestDbgFields(t *testing.T) {
	got := dbgFields(`id=3,name="a,b",size=0x10,type=lab,empty="",last`)
	want := map[string]string{"id": "3", "name": "a,b", "size": "0x10", "type": "lab", "empty": ""}
	if len(got) != len(want) {
		t.Errorf("got %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: got %q, want %q", k, got[k], v)
		}
	}
}
