package fc

import "testing"

func TestTextConverter(t *testing.T) {
	// using: インデックスは登録順 (あ=0, い=1, ↓=2, ゛=3, か=4, Ａ=5, ？=6, 　=7)
	tc := NewTextConverter("あい↓゛かＡ？　")

	t.Run("基本変換", func(t *testing.T) {
		got := tc.Conv("あい")
		want := []int{0, 1}
		if !eqInts(got, want) {
			t.Errorf("got %v want %v", got, want)
		}
	})

	t.Run("濁点分解と全角化", func(t *testing.T) {
		// が → ゛か, A → Ａ, ? → ？, 空白 → 　, \n → ↓
		got := tc.Conv("がA? \n")
		want := []int{3, 4, 5, 6, 7, 2}
		if !eqInts(got, want) {
			t.Errorf("got %v want %v", got, want)
		}
	})

	t.Run("tr表のずれの再現", func(t *testing.T) {
		// Ruby の tr はソース15文字:宛先14文字のため、\→＊, *→＝, =→＠, @→＠ になる
		tc2 := NewTextConverter("＊＝＠．")
		got := tc2.Conv(`\*=@.`)
		want := []int{0, 1, 2, 2, 3}
		if !eqInts(got, want) {
			t.Errorf("got %v want %v", got, want)
		}
	})

	t.Run("表にない文字は新規登録", func(t *testing.T) {
		tc3 := NewTextConverter("あ")
		got := tc3.Conv("あんん")
		want := []int{0, 1, 1} // ん が新規登録されて index 1
		if !eqInts(got, want) {
			t.Errorf("got %v want %v", got, want)
		}
	})
}

func eqInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
