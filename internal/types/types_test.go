package types

import "testing"

func TestUniverse(t *testing.T) {
	u := NewUniverse()
	i8 := u.IntType(1, false)
	if i8.String() != "u8" || i8.Kind != Int || i8.Size != 1 || i8.Signed {
		t.Errorf("u8: %+v", i8)
	}
	if n, ok := u.Named("int"); !ok || n != i8 {
		t.Error("int は u8 と同一の型であるべき")
	}
	for _, name := range []string{"uint", "int8", "u8"} {
		if n, _ := u.Named(name); n != i8 {
			t.Errorf("%s は u8 と同一であるべき", name)
		}
	}
	if s16, _ := u.Named("i16"); s16.String() != "i16" || !s16.Signed || s16.Size != 2 {
		t.Errorf("i16: %+v", s16)
	}
	if _, ok := u.Named("hoge"); ok {
		t.Error("未知の型名は ok=false")
	}
	if v, _ := u.Named("void"); v.String() != "void" || v.Size != 0 || v != u.Void() {
		t.Error("void")
	}
	if u.Bool().String() != "ubool8" || u.Bool().Size != 1 {
		t.Errorf("bool: %+v", u.Bool())
	}

	p := u.PointerTo(i8)
	if p.String() != "*u8" || p.Size != 2 || p.Base != i8 || p != u.PointerTo(i8) {
		t.Errorf("pointer: %+v", p)
	}
	a := u.ArrayOf(u.IntType(2, false), 10)
	if a.String() != "[10]u16" || a.Size != 20 || a.Length != 10 {
		t.Errorf("array: %+v", a)
	}
	au := u.ArrayOf(i8, -1)
	if au.String() != "[]u8" || au.Size != -1 || au.Length != -1 {
		t.Errorf("unsized array: %+v", au)
	}
	f := u.Func([]*Type{i8, i8}, p, false)
	if f.String() != "fn(u8,u8):*u8" || f.Size != 2 || f.Base != p || f.Fastcall() {
		t.Errorf("func: %+v", f)
	}
	ff := u.Func(nil, u.Void(), true)
	if ff.String() != "fastcall fn():void" || !ff.Fastcall() || ff == u.Func(nil, u.Void(), false) {
		t.Errorf("fastcall func: %+v", ff)
	}
}

func TestCompatible(t *testing.T) {
	u := NewUniverse()
	u8, s8, u16, s16 := u.IntType(1, false), u.IntType(1, true), u.IntType(2, false), u.IntType(2, true)
	cases := []struct{ a, b, want *Type }{
		{u8, u8, u8}, {u8, s8, s8}, {s8, u8, s8}, {u8, u16, u16}, {s16, u8, s16},
		{u.PointerTo(u8), u.ArrayOf(u8, 4), u.PointerTo(u8)},
		{u.ArrayOf(u8, -1), u.ArrayOf(u8, 4), u.ArrayOf(u8, 4)},
		{u.ArrayOf(u8, 3), u.ArrayOf(u8, 4), u.PointerTo(u8)},
		{u.ArrayOf(u8, 4), u.ArrayOf(u8, 4), u.ArrayOf(u8, 4)},
		{u8, u.PointerTo(u8), nil},
		{u.ArrayOf(u8, 4), u.PointerTo(u8), nil}, // 順序依存 (旧実装どおり)
		{u.Void(), u8, nil},
	}
	for i, c := range cases {
		if got := u.Compatible(c.a, c.b); got != c.want {
			t.Errorf("case %d: Compatible(%s, %s) = %v, want %v", i, c.a, c.b, got, c.want)
		}
	}
}

func TestFarFunc(t *testing.T) {
	u := NewUniverse()
	args := []*Type{u.IntType(1, false)}
	near := u.Func(args, u.Void(), false)
	far := u.FarFunc(args, u.Void())
	if far.Size != 3 || far.String() != "farfn(u8):void" || !far.IsFarFunc() || near.IsFarFunc() || far != u.FarFunc(args, u.Void()) {
		t.Fatalf("far=%+v near=%+v", far, near)
	}
	if !SameFuncSignature(near, far) || SameFuncSignature(far, u.FarFunc(nil, u.Void())) {
		t.Fatal("signature mismatch")
	}
	if u.Compatible(far, near) != nil || u.Compatible(near, far) != nil || u.Compatible(u.PointerTo(u.Void()), far) != nil {
		t.Fatal("farfn must retain its representation")
	}
}
