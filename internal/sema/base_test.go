package sema

// Value / Scope の基本テスト (test/fc/test_base.rb 由来。型のテストは internal/types へ移動)。

import (
	"testing"

	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/types"
)

func TestIntValue(t *testing.T) {
	// 整数リテラルの型推定 (-128 は sint8 に収まる)
	h := &Hlc{prog: NewProgram()}
	cases := []struct {
		n    int
		want string
	}{
		{0, "u8"}, {255, "u8"}, {256, "u16"}, {70000, "u16"},
		{-1, "i8"}, {-127, "i8"}, {-128, "i8"}, {-129, "i16"}, {-255, "i16"},
	}
	for _, c := range cases {
		v := h.IntValue(c.n)
		if v.Type.String() != c.want || !v.IsInt || v.Int != c.n {
			t.Errorf("new_int(%d): got %s want %s", c.n, v.Type, c.want)
		}
	}
}

func TestScope(t *testing.T) {
	u8 := types.NewUniverse().IntType(1, false)
	g := NewScope(nil)
	s := NewScope(g)
	v1 := ir.NewGlobal("a", u8, "_a")
	v2 := ir.NewGlobal("b", u8, "_b")
	g.Declare(v1)
	s.Declare(v2).SetPublic(true)

	if s.Find("a", true).Val != v1 {
		t.Error("親スコープの検索に失敗")
	}
	if s.Find("b", true).Val != v2 {
		t.Error("自スコープの検索に失敗")
	}
	if s.Find("c", true) != nil {
		t.Error("存在しないIDが見つかった")
	}

	// use 経由は public のみ見える
	other := NewScope(nil)
	pub := ir.NewGlobal("p", u8, "_p")
	priv := ir.NewGlobal("q", u8, "_q")
	other.Declare(pub).SetPublic(true)
	other.Declare(priv)
	s.Use(&ModuleInterface{Id: "other", scope: other}, true)
	if s.Find("p", true).Val != pub {
		t.Error("use経由のpublicが見えない")
	}
	if s.Find("q", true) != nil {
		t.Error("use経由のprivateが見えてしまう")
	}

	// 相互use しても無限再帰しない
	other.Use(&ModuleInterface{Id: "self", scope: s}, true)
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
		s.Declare(ir.NewGlobal("b", u8, "_b2"))
	}()
}
