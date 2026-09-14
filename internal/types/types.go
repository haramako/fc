// Package types は fc 言語の型と、その同一性 (インターン) を管理する。
//
// 型は Universe 経由で作る。同じ構造の型は同じ *Type になるので、ポインタ比較で同一性を判定できる。
// Universe はコンパイラのインスタンスごとに 1 つ持つ (パッケージレベルの可変状態を持たない: doc/v2_plan.md R1-g)。
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
)

var kindNames = [...]string{
	Void: "void", Bool: "bool", Int: "int", Module: "module", Macro: "macro",
	Pointer: "pointer", Array: "array", Func: "lambda", Struct: "struct", TypeName: "typename", SoaRef: "soaref",
}

func (k Kind) String() string {
	if int(k) < len(kindNames) && k > 0 {
		return kindNames[k]
	}
	return fmt.Sprintf("Kind(%d)", int(k))
}

// Type は fc の型。Universe でインターンされるので、フィールドは変更しないこと。
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

// TypeName は型名を束縛した値 (struct 宣言) の型。
func (u *Universe) TypeName() *Type {
	return u.intern(&Type{Kind: TypeName, Size: 0, Length: -1, str: "typename"})
}

// NewStruct は名前付き struct 型を作る (フィールドは SetFields で後から入れる。自己参照のため)。
// qualName はモジュール修飾名 (mod.Name)。同名は同じ型。
func (u *Universe) NewStruct(qualName string) *Type {
	return u.intern(&Type{Kind: Struct, Name: qualName, Length: -1, str: "struct " + qualName})
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
}

// SoaArray は SoA コンテナの型 (`soa Name:[N]Elem`)。配列型だが IsSoa で区別し、要素はメモリ上で分散する。
func (u *Universe) SoaArray(qualName string, elem *Type, length int) *Type {
	return u.intern(&Type{Kind: Array, Base: elem, Length: length, Size: elem.Size * length, IsSoa: true, Name: qualName,
		str: fmt.Sprintf("soa %s [%d]%s", qualName, length, elem.str)})
}

// SoaRef は SoA コンテナの要素ハンドル (`*Name`。1 バイトのインデックス)。
func (u *Universe) SoaRef(soa *Type) *Type {
	return u.intern(&Type{Kind: SoaRef, Size: 1, Base: soa.Base, Soa: soa, Name: soa.Name, Length: -1, str: "*" + soa.Name})
}

// IntType は size バイトの整数型。
func (u *Universe) IntType(size int, signed bool) *Type {
	s := "u"
	if signed {
		s = "s"
	}
	return u.intern(&Type{Kind: Int, Size: size, Signed: signed, Length: -1, str: fmt.Sprintf("%sint%d", s, size*8)})
}

// 基本型の名前 → (サイズ, 符号)。
var basicTypes = map[string]struct {
	size   int
	signed bool
}{
	"int": {1, false}, "uint": {1, false}, "sint": {1, true},
	"int8": {1, false}, "sint8": {1, true}, "uint8": {1, false},
	"int16": {2, false}, "sint16": {2, true}, "uint16": {2, false},
}

// Named は型名から型を返す。未知の名前なら ok=false。
func (u *Universe) Named(name string) (t *Type, ok bool) {
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
	if bt, ok := basicTypes[name]; ok {
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
		t.Size = base.Size * length
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

// Compatible は a と b の互換型を返す (TypeUtil.compatible_type? 相当)。互換性がなければ nil。
//   - 整数同士: サイズが大きい方。同サイズなら符号付きの方
//   - ポインタと同じ要素型の配列: ポインタ
//   - 要素型が同じ配列同士: a が長さ省略なら b、長さが違えば要素型へのポインタ
func (u *Universe) Compatible(a, b *Type) *Type {
	if a == b {
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
