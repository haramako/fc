package fc

// Value / Scope の基本テスト (test/fc/test_base.rb 由来。型のテストは internal/types へ移動)。

import "testing"

func TestIntValue(t *testing.T) {
	// Value.new_int の型推定 (Ruby: n<-127 は sint16 という境界も含めて)
	h := NewHlc(nil)
	cases := []struct {
		n    int
		want string
	}{
		{0, "uint8"}, {255, "uint8"}, {256, "uint16"}, {70000, "uint16"},
		{-1, "sint8"}, {-127, "sint8"}, {-128, "sint16"}, {-255, "sint16"},
	}
	for _, c := range cases {
		v := h.IntValue(c.n)
		if v.Type.String() != c.want || !v.IsInt || v.Int != c.n {
			t.Errorf("new_int(%d): got %s want %s", c.n, v.Type, c.want)
		}
	}
}

func TestScope(t *testing.T) {
	h := NewHlc(nil)
	u8 := h.Types().IntType(1, false)
	g := NewScope(nil)
	s := NewScope(g)
	v1 := NewGlobal("a", u8, "_a")
	v2 := NewGlobal("b", u8, "_b")
	v2.Public = true
	g.Declare(v1)
	s.Declare(v2)

	if s.Find("a", true) != v1 {
		t.Error("親スコープの検索に失敗")
	}
	if s.Find("b", true) != v2 {
		t.Error("自スコープの検索に失敗")
	}
	if s.Find("c", true) != nil {
		t.Error("存在しないIDが見つかった")
	}

	// use 経由は public のみ見える
	other := NewScope(nil)
	pub := NewGlobal("p", u8, "_p")
	pub.Public = true
	priv := NewGlobal("q", u8, "_q")
	other.Declare(pub)
	other.Declare(priv)
	s.Use(other)
	if s.Find("p", true) != pub {
		t.Error("use経由のpublicが見えない")
	}
	if s.Find("q", true) != nil {
		t.Error("use経由のprivateが見えてしまう")
	}

	// 相互use しても無限再帰しない
	other.Use(s)
	if s.Find("nothing", true) != nil {
		t.Error("相互useで誤検出")
	}
	// IdList は自スコープの宣言順 → use 先 → 親
	ids := s.IdList()
	if len(ids) < 3 || ids[0] != "b" || ids[1] != "p" {
		t.Errorf("IdList: %v", ids)
	}

	// 二重宣言はエラー
	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Error("二重宣言が通った")
			}
		}()
		s.Declare(NewGlobal("b", u8, "_b2"))
	}()
}

func TestOptions(t *testing.T) {
	var o Options
	o.Set("a", OptionValue{Kind: OptInt, Int: 1})
	o.Set("b", OptionValue{Kind: OptStr, Str: "x"})
	o.Set("a", OptionValue{Kind: OptInt, Int: 2}) // 後勝ち・位置維持
	if len(o) != 2 || o[0].Key != "a" || o[0].Value.Int != 2 || o[1].Key != "b" {
		t.Errorf("Options: %+v", o)
	}
	if n, ok := o.Int("a"); !ok || n != 2 {
		t.Error("Int")
	}
	if _, ok := o.Int("b"); ok {
		t.Error("文字列を Int で取れてはいけない")
	}
	if !o.Has("b") || o.Has("c") {
		t.Error("Has")
	}
	if (OptionValue{Kind: OptIdent, Str: "my"}).Text() != "my" || (OptionValue{Kind: OptInt, Int: 5}).Text() != "5" {
		t.Error("Text")
	}
}
