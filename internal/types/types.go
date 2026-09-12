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
)

var kindNames = [...]string{
	Void: "void", Bool: "bool", Int: "int", Module: "module", Macro: "macro",
	Pointer: "pointer", Array: "array", Func: "lambda",
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
	fastcall bool
	str      string
}

// String は型の表示名 (`uint8`, `sint16`, `uint8*`, `uint8[4]`, `fastcall void(uint8)` など)。
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
	return u.intern(&Type{Kind: Pointer, Size: 2, Base: base, Length: -1, str: base.str + "*"})
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
	t.str = fmt.Sprintf("%s[%s]", base.str, l)
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
	t.str = fmt.Sprintf("%s%s(%s)", fc, result.str, strings.Join(args, ","))
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
