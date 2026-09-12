package ir

// IR の値 (変数・定数・リテラル) とそのラッパ。lib/fc/base.rb の Value / CastedValue / PointeredArray 由来。

import (
	"fmt"
	"strconv"

	"github.com/haramako/fc/internal/types"
)

type LiveRange struct {
	Min, Max int
}

// LocalType はローカル変数の役割 (レジスタ割付で使う)。
type LocalType uint8

const (
	LTNone   LocalType = iota // ユーザー宣言の変数、またはグローバル
	LTArg                     // 関数の引数
	LTResult                  // 戻り値
	LTTemp                    // コンパイラが作った一時変数
)

var localTypeNames = [...]string{LTNone: "", LTArg: "arg", LTResult: "result", LTTemp: "temp"}

func (l LocalType) String() string {
	if int(l) < len(localTypeNames) {
		return localTypeNames[l]
	}
	return fmt.Sprintf("LocalType(%d)", int(l))
}

// Value は変数・定数・リテラル・モジュール束縛を表す。レジスタ割付の結果も持つ。
//
// Kind ごとに有効なフィールド:
//   - KindLocal:        Name, LocalType
//   - KindGlobal:       Name と、Symbol (アセンブラシンボル) / Module (モジュール束縛) のいずれか (マクロは型で表す)
//   - KindLiteral:      IsInt なら Int、そうでなければ Symbol (関数シンボル)。Name は定数名 ("" なら無名)
//   - KindArrayLiteral: Elems
type Value struct {
	Kind ValueKind
	Type *types.Type
	Name string // 変数名 / 定数名。無名なら ""

	IsInt  bool
	Int    int
	Symbol string
	Elems  []Operand
	Module *Module // モジュール束縛 (`use mod;`)。マクロは Type.Kind == types.Macro で表し、本体は sema が持つ

	// 元が文字列リテラルだった配列 (IsString のとき Str が元の文字列)
	IsString bool
	Str      string

	Public    bool
	LocalType LocalType

	// 以下はレジスタ割付で設定される
	Location     Location
	Address      int // Location が LocFrame / LocReg / LocFastcallReg のとき有効
	Unuse        bool
	LiveRange    *LiveRange
	CondReg      CondReg // Location == LocCond のときのみ
	CondPositive bool    // Location == LocCond のときのみ
}

// HasAddress は Address が有効な置き場所かを返す。
func (v *Value) HasAddress() bool {
	switch v.Location {
	case LocFrame, LocReg, LocFastcallReg:
		return true
	}
	return false
}

func newValue(kind ValueKind, name string, typ *types.Type) *Value {
	if typ == nil {
		panic("ir: value without type")
	}
	return &Value{Kind: kind, Name: name, Type: typ}
}

// NewLocal はローカル変数。
func NewLocal(name string, typ *types.Type, lt LocalType) *Value {
	v := newValue(KindLocal, name, typ)
	v.LocalType = lt
	return v
}

// NewGlobal はアセンブラシンボルを持つグローバル (変数 / 定数配列 / 関数)。
func NewGlobal(name string, typ *types.Type, symbol string) *Value {
	v := newValue(KindGlobal, name, typ)
	v.Symbol = symbol
	return v
}

// NewModuleValue はモジュール束縛 (`use mod;`)。
func NewModuleValue(name string, typ *types.Type, m *Module) *Value {
	v := newValue(KindGlobal, name, typ)
	v.Module = m
	return v
}

// NewIntLiteral は整数リテラル (型は明示)。
func NewIntLiteral(name string, typ *types.Type, n int) *Value {
	v := newValue(KindLiteral, name, typ)
	v.IsInt = true
	v.Int = n
	return v
}

// NewSymbolLiteral は関数シンボルのリテラル。
func NewSymbolLiteral(name string, typ *types.Type, symbol string) *Value {
	v := newValue(KindLiteral, name, typ)
	v.Symbol = symbol
	return v
}

// NewArrayLiteral は配列リテラル。
func NewArrayLiteral(name string, typ *types.Type, elems []Operand) *Value {
	v := newValue(KindArrayLiteral, name, typ)
	v.Elems = elems
	return v
}

func (v *Value) Assignable() bool {
	return v.Kind == KindLocal || v.Kind == KindGlobal
}

// CastedValue は reinterpret_cast 相当。Ruby では Delegator で @from に委譲される。
type CastedValue struct {
	From   Operand // *Value または *CastedValue
	Type   *types.Type
	Offset int
}

func NewCastedValue(from Operand, typ *types.Type, offset int) *CastedValue {
	return &CastedValue{From: from, Type: typ, Offset: offset}
}

// PointeredArray は配列からポインタへ自動変換された値。
type PointeredArray struct {
	From Operand // *Value (または *CastedValue)
	Type *types.Type
}

func NewPointeredArray(from Operand, ptrType *types.Type) *PointeredArray {
	return &PointeredArray{From: from, Type: ptrType}
}

// ---------------------------------------------------------------
// 表示 (エラーメッセージで使用)
// ---------------------------------------------------------------

// valueString は Value#to_s 相当の旧表示を返す。
func (v *Value) String() string {
	if v.Name != "" {
		return "{" + v.Name + "}"
	}
	return v.Inspect()
}

// Inspect は Value#inspect 相当。
func (v *Value) Inspect() string {
	switch {
	case v.Name != "":
		return fmt.Sprintf("{%s:%s}", v.Name, v.Type)
	case v.IsString:
		return `{"` + v.Str + `"}`
	case v.Kind == KindLiteral && v.IsInt:
		return "{" + strconv.Itoa(v.Int) + "}"
	case v.Symbol != "":
		return "{" + v.Symbol + "}"
	case v.Module != nil:
		return "{" + v.Module.Id + "}"
	}
	return "{}"
}

// String は CastedValue#to_s 相当。
func (c *CastedValue) String() string {
	if c.Offset == 0 {
		return fmt.Sprintf("<%s>%s", c.Type, OperandString(c.From))
	}
	return fmt.Sprintf("<%s+%d>%s", c.Type, c.Offset, OperandString(c.From))
}

// String は PointeredArray#to_s 相当。
func (p *PointeredArray) String() string {
	return OperandString(p.From) + "#p"
}

// OperandString はオペランドの表示 (エラーメッセージ用)。
func OperandString(v Operand) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%s", v)
}

// ---------------------------------------------------------------
// オペランドの属性アクセス
// Ruby では CastedValue(Delegator) がメソッドを @from に委譲し、PointeredArray は kind のみ委譲する。
// ---------------------------------------------------------------

// UnderlyingValue は CastedValue の委譲チェーンをたどって *Value を返す (PointeredArray は nil)。
// (Delegator は hash/eql? も委譲するため、Hashキーとしては From と同一視される)
func UnderlyingValue(v Operand) *Value {
	for {
		switch x := v.(type) {
		case *Value:
			return x
		case *CastedValue:
			v = x.From
		default:
			return nil
		}
	}
}

// ValKind は v.kind (PointeredArray も from に委譲する)。
func ValKind(v Operand) ValueKind {
	switch x := v.(type) {
	case *Value:
		return x.Kind
	case *CastedValue:
		return ValKind(x.From)
	case *PointeredArray:
		return ValKind(x.From)
	}
	panic(fmt.Sprintf("ValKind: invalid value %T", v))
}

// ValType は v.type (CastedValue/PointeredArray は自身の型を持つ)。
func ValType(v Operand) *types.Type {
	switch x := v.(type) {
	case *Value:
		return x.Type
	case *CastedValue:
		return x.Type
	case *PointeredArray:
		return x.Type
	}
	panic(fmt.Sprintf("ValType: invalid value %T", v))
}

// ValLiteral はリテラル値の本体 (CastedValue は from に委譲、PointeredArray は nil)。
// 整数リテラルか関数シンボルかは IsInt で判定する。
func ValLiteral(v Operand) *Value {
	switch x := v.(type) {
	case *Value:
		return x
	case *CastedValue:
		return ValLiteral(x.From)
	case *PointeredArray:
		return nil
	}
	panic(fmt.Sprintf("ValLiteral: invalid value %T", v))
}

// ValIntLiteral は v が整数リテラルならその値を返す。
func ValIntLiteral(v Operand) (int, bool) {
	if lv := ValLiteral(v); lv != nil && lv.Kind == KindLiteral && lv.IsInt {
		return lv.Int, true
	}
	return 0, false
}

// ValAssignable は v.assignable?
func ValAssignable(v Operand) bool {
	switch x := v.(type) {
	case *Value:
		return x.Assignable()
	case *CastedValue:
		return ValAssignable(x.From)
	case *PointeredArray:
		return false
	}
	panic(fmt.Sprintf("ValAssignable: invalid value %T", v))
}

// ValLocation は v.location (CastedValue は from に委譲)。
func ValLocation(v Operand) Location {
	switch x := v.(type) {
	case *Value:
		return x.Location
	case *CastedValue:
		return ValLocation(x.From)
	}
	panic(fmt.Sprintf("ValLocation: invalid value %T", v))
}

// ValAddress は v.address (CastedValue は from に委譲)。
func ValAddress(v Operand) int {
	switch x := v.(type) {
	case *Value:
		return x.Address
	case *CastedValue:
		return ValAddress(x.From)
	}
	panic(fmt.Sprintf("ValAddress: invalid value %T", v))
}

// ValLocalType は v.opt[:local_type] (CastedValue は from に委譲)。
func ValLocalType(v Operand) LocalType {
	switch x := v.(type) {
	case *Value:
		return x.LocalType
	case *CastedValue:
		return ValLocalType(x.From)
	}
	panic(fmt.Sprintf("ValLocalType: invalid value %T", v))
}
