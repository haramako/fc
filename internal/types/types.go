// Package types は fc 言語の型と、その同一性 (インターン) を管理する。
//
// 型は Universe 経由で作る。同じ構造の型は同じ *Type になるので、ポインタ比較で同一性を判定できる。
// Universe はコンパイラのインスタンスごとに 1 つ持つ (パッケージレベルの可変状態を持たない: doc/archive/v2_plan.md R1-g)。
package types

import (
	"fmt"
	"strings"
)

// Kind は型の種類。
type Kind uint8

const (
	Void Kind = iota + 1
	Bool
	Int
	Module
	Macro
	Pointer
	Array
	Func
	Struct   // 構造体 (Fields)
	TypeName // 型名を束縛した値の型 (Value.TypeRef が実際の型)
	SoaRef   // SoA コンテナの要素ハンドル (実体は uint8 のインデックス。Base = 要素の struct 型、Soa = コンテナ)
	Bad      // エラーになった宣言の型 (これに触れるエラーは報告しない: 巻き添えの抑制)
)

var kindNames = [...]string{
	Void: "void", Bool: "bool", Int: "int", Module: "module", Macro: "macro",
	Pointer: "pointer", Array: "array", Func: "lambda", Struct: "struct", TypeName: "typename", SoaRef: "soaref", Bad: "bad",
}

func (k Kind) String() string {
	if int(k) < len(kindNames) && k > 0 {
		return kindNames[k]
	}
	return fmt.Sprintf("Kind(%d)", int(k))
}

// Type is interned by Universe. Struct/SoA layout and dependent array sizes
// are completed during declaration resolution; consumers must not modify them.
type Type struct {
	Kind     Kind
	Size     int   // バイト数。長さ省略配列は -1
	Signed   bool  // Int のみ
	Base     *Type // Pointer / Array の要素型、Func の戻り値型
	Length   int   // Array の要素数。省略時 -1
	Params   []*Type
	Fields   []Field // Struct のフィールド (宣言順)
	Name     string  // Struct / SoaRef のモジュール修飾名 (mod.Name)
	Soa      *Type   // SoaRef のコンテナ (`soa` 配列型)、Kind == Array で IsSoa
	IsSoa    bool    // Array が SoA コンテナ (soa 宣言) か
	IsConst  bool    // SoA コンテナが `soa const` (読み出しのみ) か
	Path     string  // SoaRef: 入れ子 struct フィールドのハンドルなら、そのフィールドまでの名前 ("pos_")。最上位は ""
	far      bool
	fastcall bool
	str      string
}

// Field は struct のフィールド。
type Field struct {
	Name   string
	Type   *Type
	Offset int
}

// Field は名前でフィールドを探す。
func (t *Type) Field(name string) (Field, bool) {
	for _, f := range t.Fields {
		if f.Name == name {
			return f, true
		}
	}
	return Field{}, false
}

// String は型の表示名 (`uint8`, `sint16`, `*uint8`, `[4]uint8`, `fastcall fn(uint8):void` など。v2 の前置形)。
func (t *Type) String() string { return t.str }

// Fastcall は Func が fastcall 呼び出し規約かを返す。
func (t *Type) Fastcall() bool { return t.fastcall }

// IsFarFunc reports the three-byte address/bank function pointer representation.
func (t *Type) IsFarFunc() bool { return t != nil && t.Kind == Func && t.far }

// SameFuncSignature ignores only the near/far pointer representation.
func SameFuncSignature(a, b *Type) bool {
	if a == nil || b == nil || a.Kind != Func || b.Kind != Func || a.Base != b.Base || a.fastcall != b.fastcall || len(a.Params) != len(b.Params) {
		return false
	}
	for i, p := range a.Params {
		if p != b.Params[i] {
			return false
		}
	}
	return true
}

// Universe は型のインターン表。
type Universe struct {
	cache map[string]*Type
}

// NewUniverse は空のインターン表を作る。
func NewUniverse() *Universe {
	return &Universe{cache: map[string]*Type{}}
}

// intern は t.str をキーにインターンする。
func (u *Universe) intern(t *Type) *Type {
	if c, ok := u.cache[t.str]; ok {
		return c
	}
	u.cache[t.str] = t
	return t
}

// Void / Bool / Module / Macro は単位型。
func (u *Universe) Void() *Type { return u.intern(&Type{Kind: Void, Size: 0, Length: -1, str: "void"}) }
func (u *Universe) Bool() *Type {
	return u.intern(&Type{Kind: Bool, Size: 1, Length: -1, str: "ubool8"})
}
func (u *Universe) Module() *Type {
	return u.intern(&Type{Kind: Module, Size: 0, Length: -1, str: "module"})
}
func (u *Universe) Macro() *Type {
	return u.intern(&Type{Kind: Macro, Size: 0, Length: -1, str: "macro"})
}

// Bad はエラーになった宣言に付ける型。どの型とも互換で、これを使う式のエラーは抑制される。
func (u *Universe) Bad() *Type {
	return u.intern(&Type{Kind: Bad, Size: 1, Length: -1, str: "<error>"})
}

// TypeName は型名を束縛した値 (struct 宣言) の型。
func (u *Universe) TypeName() *Type {
	return u.intern(&Type{Kind: TypeName, Size: 0, Length: -1, str: "typename"})
}

// NewStruct は名前付き struct 型を作る (フィールドは SetFields で後から入れる。自己参照のため)。
// qualName はモジュール修飾名 (mod.Name)。同名は同じ型。
func (u *Universe) NewStruct(qualName string) *Type {
	return u.intern(&Type{Kind: Struct, Name: qualName, Size: -1, Length: -1, str: qualName})
}

// SetFields は struct のフィールドを確定し、オフセットとサイズを計算する (詰めて配置、アラインメントなし)。
func (u *Universe) SetFields(t *Type, fields []Field) {
	off := 0
	for i := range fields {
		fields[i].Offset = off
		off += fields[i].Type.Size
	}
	t.Fields = fields
	t.Size = off
	u.refreshArraySizes()
}

// SoaArray は SoA コンテナの型 (`soa Name:[N]Elem`)。配列型だが IsSoa で区別し、要素はメモリ上で分散する
// (フィールドごとの配列)。値としては添字で要素ハンドル (SoaRef) を得る以外の使い方はない。
// elem == nil reserves the identity; a later call with elem/length completes its layout.
func (u *Universe) SoaArray(qualName string, elem *Type, length int, isConst bool) *Type {
	str := "soa " + qualName
	if isConst {
		str = "soa const " + qualName
	}
	t := u.intern(&Type{Kind: Array, Base: elem, Length: -1, Size: -1, IsSoa: true, IsConst: isConst, Name: qualName, str: str})
	if elem != nil && length >= 0 {
		t.Base, t.Length, t.Size = elem, length, elem.Size*length
	}
	return t
}

// SoaRef は SoA コンテナの要素ハンドル (`*Name`。1 バイトのインデックス)。
// base はハンドルが指す struct (最上位ならコンテナの要素型)、path は入れ子フィールドの名前の連結 ("" / "pos_")。
func (u *Universe) SoaRef(soa, base *Type, path string) *Type {
	str := "*" + soa.Name
	if path != "" {
		str += "." + strings.TrimSuffix(path, "_")
	}
	return u.intern(&Type{Kind: SoaRef, Size: 1, Base: base, Soa: soa, Name: soa.Name, Path: path, Length: -1, str: str})
}

// IntType は size バイトの整数型。
func (u *Universe) IntType(size int, signed bool) *Type {
	s := "u"
	if signed {
		s = "i"
	}
	return u.intern(&Type{Kind: Int, Size: size, Signed: signed, Length: -1, str: fmt.Sprintf("%s%d", s, size*8)})
}

type intSpec struct {
	size   int
	signed bool
}

// IntTypeNames は整数型の名前 (fc 3 の正式名。fc 2 でも使える) → (サイズ, 符号)。doc/v3_plan.md §7。
var IntTypeNames = map[string]intSpec{
	"u8": {1, false}, "i8": {1, true}, "u16": {2, false}, "i16": {2, true},
}

// V2IntTypeNames は fc 2 だけの整数型の名前 → fc 3 の名前 (`fcc migrate` の書き換えと fc 3 での案内に使う)。
// `int` / `uint` / `int8` / `uint8` は u8、`sint` / `sint8` は i8、`int16` / `uint16` は u16、`sint16` は i16。
var V2IntTypeNames = map[string]string{
	"int": "u8", "uint": "u8", "int8": "u8", "uint8": "u8",
	"sint": "i8", "sint8": "i8",
	"int16": "u16", "uint16": "u16",
	"sint16": "i16",
}

// Named は型名から型を返す (fc 2 の規則: 古い名前も fc 3 の名前も引ける)。未知の名前なら ok=false。
func (u *Universe) Named(name string) (t *Type, ok bool) {
	return u.NamedIn(name, 2)
}

// NamedIn は文法バージョン version のモジュールでの型名の解決。fc 3 では fc 2 だけの整数型の名前 (int など) を引かない。
func (u *Universe) NamedIn(name string, version int) (t *Type, ok bool) {
	switch name {
	case "void":
		return u.Void(), true
	case "bool":
		return u.Bool(), true
	case "module":
		return u.Module(), true
	case "macro":
		return u.Macro(), true
	}
	if bt, ok := IntTypeNames[name]; ok {
		return u.IntType(bt.size, bt.signed), true
	}
	if n, ok := V2IntTypeNames[name]; ok && version < 3 {
		bt := IntTypeNames[n]
		return u.IntType(bt.size, bt.signed), true
	}
	return nil, false
}

// PointerTo は base へのポインタ型。
func (u *Universe) PointerTo(base *Type) *Type {
	return u.intern(&Type{Kind: Pointer, Size: 2, Base: base, Length: -1, str: "*" + base.str})
}

// ArrayOf は base の配列型。length < 0 なら長さ省略 (Size も -1)。
func (u *Universe) ArrayOf(base *Type, length int) *Type {
	t := &Type{Kind: Array, Base: base, Length: -1, Size: -1}
	l := ""
	if length >= 0 {
		t.Length = length
		if base.Size >= 0 {
			t.Size = base.Size * length
		}
		l = fmt.Sprintf("%d", length)
	}
	t.str = fmt.Sprintf("[%s]%s", l, base.str)
	return u.intern(t)
}

// Func は関数型。
func (u *Universe) Func(params []*Type, result *Type, fastcall bool) *Type {
	t := &Type{Kind: Func, Size: 2, Base: result, Params: params, fastcall: fastcall, Length: -1}
	fc := ""
	if fastcall {
		fc = "fastcall "
	}
	args := make([]string, len(params))
	for i, a := range params {
		args[i] = a.str
	}
	t.str = fmt.Sprintf("%sfn(%s):%s", fc, strings.Join(args, ","), result.str)
	return u.intern(t)
}

// FarFunc はアドレスとバンクを保持する 3 バイトの関数ポインタ型。
func (u *Universe) FarFunc(params []*Type, result *Type) *Type {
	t := *u.Func(params, result, false)
	t.far, t.Size, t.str = true, 3, "far"+t.str
	return u.intern(&t)
}

// Compatible は a と b の互換型を返す (TypeUtil.compatible_type? 相当)。互換性がなければ nil。
// 代入では a が代入先 (*void の規則だけ向きがある)。
//   - 整数同士: サイズが大きい方。同サイズなら符号付きの方
//   - ポインタと同じ要素型の配列: ポインタ
//   - 要素型が同じ配列同士: a が長さ省略なら b、長さが違えば要素型へのポインタ
func (u *Universe) Compatible(a, b *Type) *Type {
	if a == b {
		return a
	}
	// エラーになった宣言の型は何とでも互換 (巻き添えのエラーを出さない)
	if a.Kind == Bad {
		return b
	}
	if b.Kind == Bad {
		return a
	}
	// bool は uint8 と互換 (比較・論理演算の結果と true / false は bool。整数と混ぜれば uint8)
	if a.Kind == Bool {
		a = u.IntType(1, false)
	}
	if b.Kind == Bool {
		b = u.IntType(1, false)
	}
	if a == b {
		return a
	}
	// *void (a 側 = 代入先) にはデータポインタ / near 関数ポインタ / 配列が入る。逆 (*void → *T) は bitcast が要る
	if a.Kind == Pointer && a.Base.Kind == Void && (b.Kind == Pointer || (b.Kind == Func && !b.IsFarFunc()) || b.Kind == Array) {
		return a
	}
	if a.Kind == Int && b.Kind == Int {
		if a.Size == b.Size {
			if a.Signed {
				return a
			}
			return b
		}
		if a.Size > b.Size {
			return a
		}
		return b
	} else if a.Kind == Pointer && b.Kind == Array && a.Base == b.Base {
		return a
	} else if a.Kind == Array && b.Kind == Array && a.Base == b.Base && a.Length < 0 {
		// 配列の長さを省略した場合
		return b
	} else if a.Kind == Array && b.Kind == Array && a.Base == b.Base && a.Length != b.Length {
		return u.PointerTo(a.Base)
	}
	return nil
}

// Arrays may be interned through pointers before a struct's layout is known.
// Refresh dependent array sizes when a struct becomes complete; pointer/function
// sizes are fixed and therefore break recursive layout dependencies.
func (u *Universe) refreshArraySizes() {
	for {
		changed := false
		for _, t := range u.cache {
			if t.Kind == Array && !t.IsSoa && t.Size < 0 && t.Length >= 0 && t.Base.Size >= 0 {
				size := t.Length * t.Base.Size
				if t.Size != size {
					t.Size = size
					changed = true
				}
			}
		}
		if !changed {
			return
		}
	}
}
