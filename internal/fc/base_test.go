package fc

// test/fc/test_base.rb の移植 + Value/Scope の基本テスト。

import "testing"

func TestType(t *testing.T) {
	t.Run("as void", func(t *testing.T) {
		v := TypeOf(Sym("void"))
		if v.Kind != "void" || v.Base != nil || v.Size != 0 {
			t.Errorf("void: %+v", v)
		}
	})
	t.Run("as bool", func(t *testing.T) {
		v := TypeOf(Sym("bool"))
		if v.Kind != "bool" || v.Base != nil || v.Size != 1 {
			t.Errorf("bool: %+v", v)
		}
	})
	t.Run("as int", func(t *testing.T) {
		v := TypeOf(Sym("int"))
		if v.Kind != "int" || v.Signed != false || v.Base != nil || v.Size != 1 {
			t.Errorf("int: %+v", v)
		}
	})
	t.Run("as array", func(t *testing.T) {
		v := TypeOf([]any{Sym("array"), 10, Sym("int16")})
		if v.Kind != "array" || v.Base != TypeOf(Sym("int16")) || v.Length != 10 || v.Size != 20 {
			t.Errorf("array: %+v", v)
		}
	})
	t.Run("as pointer", func(t *testing.T) {
		v := TypeOf([]any{Sym("pointer"), Sym("int")})
		if v.Kind != "pointer" || v.Size != 2 || v.Base != TypeOf(Sym("int")) {
			t.Errorf("pointer: %+v", v)
		}
	})
	t.Run("as lambda", func(t *testing.T) {
		v := TypeOf([]any{Sym("lambda"), []any{TypeOf(Sym("int")), TypeOf(Sym("int"))}, []any{Sym("pointer"), Sym("int")}})
		if v.Kind != "lambda" || v.Size != 2 || v.Base != TypeOf([]any{Sym("pointer"), Sym("int")}) {
			t.Errorf("lambda: %+v", v)
		}
		if len(v.Args) != 2 || v.Args[0] != TypeOf(Sym("int")) || v.Args[1] != TypeOf(Sym("int")) {
			t.Errorf("lambda args: %+v", v.Args)
		}
	})
	t.Run("as complex type", func(t *testing.T) {
		v := TypeOf([]any{Sym("array"), 10, []any{Sym("pointer"), []any{Sym("array"), 2, Sym("int")}}})
		if v.String() != "uint8[2]*[10]" {
			t.Errorf("complex: %s", v)
		}
	})
	// インターンの確認
	t.Run("intern", func(t *testing.T) {
		a := TypeOf([]any{Sym("pointer"), Sym("uint8")})
		b := TypeOf([]any{Sym("pointer"), Sym("int")})
		if a != b {
			t.Errorf("intern: %p != %p", a, b)
		}
	})
}

func TestValue(t *testing.T) {
	// Value.new_int の型推定 (Ruby: n<-127 は sint16 という境界も含めて)
	cases := []struct {
		n    int
		want string
	}{
		{0, "uint8"}, {255, "uint8"}, {256, "uint16"}, {70000, "uint16"},
		{-1, "sint8"}, {-127, "sint8"}, {-128, "sint16"}, {-255, "sint16"},
	}
	for _, c := range cases {
		v := NewIntValue(c.n)
		if v.Type.String() != c.want {
			t.Errorf("new_int(%d): got %s want %s", c.n, v.Type, c.want)
		}
	}
}

func TestScope(t *testing.T) {
	g := NewScope(nil)
	s := NewScope(g)
	v1 := NewValue(KindGlobal, Sym("a"), TypeOf(Sym("int")), nil, nil)
	v2 := NewValue(KindGlobal, Sym("b"), TypeOf(Sym("int")), nil, nil)
	v2.Public = true
	g.Declare(v1)
	s.Declare(v2)

	if s.Find(Sym("a"), true) != v1 {
		t.Error("親スコープの検索に失敗")
	}
	if s.Find(Sym("b"), true) != v2 {
		t.Error("自スコープの検索に失敗")
	}
	if s.Find(Sym("c"), true) != nil {
		t.Error("存在しないIDが見つかった")
	}

	// use 経由は public のみ見える
	other := NewScope(nil)
	pub := NewValue(KindGlobal, Sym("p"), TypeOf(Sym("int")), nil, nil)
	pub.Public = true
	priv := NewValue(KindGlobal, Sym("q"), TypeOf(Sym("int")), nil, nil)
	other.Declare(pub)
	other.Declare(priv)
	s.Use(other)
	if s.Find(Sym("p"), true) != pub {
		t.Error("use経由のpublicが見えない")
	}
	if s.Find(Sym("q"), true) != nil {
		t.Error("use経由のprivateが見えてしまう")
	}

	// 相互use しても無限再帰しない
	other.Use(s)
	if s.Find(Sym("nothing"), true) != nil {
		t.Error("相互useで誤検出")
	}
	_ = s.IdList()
}
