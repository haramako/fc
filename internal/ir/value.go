package ir

// IR の値 (変数・定数・リテラル) とそのラッパ。lib/fc/base.rb の Value / CastedValue / PointeredArray 由来。

import (
	"fmt"
	"strconv"

	"github.com/haramako/fc/internal/types"
)

type LiveRange struct {
	Min, Max int
	// Writes は生存区間の外にある定義 (使われない書き込み) の位置。書き込みはメモリに起きるので、そこで生きている
	// 別の変数と番地を共有できない (ループ変数と同じ番地に置かれた dead store で無限ループになった。fuzz で発覚)
	Writes []int
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

	IsInt   bool
	Int     int
	Symbol  string
	Elems   []Operand
	Module  *ModuleInterface // モジュール束縛 (`use mod;`)。マクロは Type.Kind == types.Macro で表し、本体は sema が持つ
	TypeRef *types.Type      // 型名の束縛 (struct / soa 宣言)。Type.Kind == types.TypeName

	// 元が文字列リテラルだった配列 (IsString のとき Str が元の文字列)
	IsString bool
	Str      string

	Public    bool
	LocalType LocalType
	// Volatile はグローバル変数で、読むたび / 書くたびに意味がある (レジスタに置いたままにできない):
	// options(address:) の I/O レジスタ、asm から参照される変数、options(volatile: true) (doc/language_reference.md §2)
	Volatile bool
	Build    bool // fc 3 の @(build) の const (@if の条件に使える。値はビルドの設定で上書きできる)

	// 以下はレジスタ割付で設定される
	Home         *Value // Location == LocA / LocY / LocX でループ内に常駐する一時変数のメモリ側 (退避先。regalloc.AllocateResident)
	Clean        bool   // Home と常に一致する (領域内で書き換えられない。引数など) ので、レジスタを壊す命令の前の退避 (書き戻し) が要らない
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
	case LocFrame, LocReg, LocFastcallReg, LocStatic:
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
func NewModuleValue(name string, typ *types.Type, m *ModuleInterface) *Value {
	v := newValue(KindGlobal, name, typ)
	v.Module = m
	return v
}

// NewTypeValue は型名の束縛 (`struct Name` / `soa Name`)。typ は Kind == types.TypeName、ref が実際の型。
func NewTypeValue(name string, typ *types.Type, ref *types.Type) *Value {
	v := newValue(KindGlobal, name, typ)
	v.TypeRef = ref
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

// CastedValue は reinterpret_cast 相当: 元の値 From の Offset バイト目から Width バイトを読み、残りの上位バイトを 0 にして
// (ゼロ拡張。符号拡張は sema が sign_extension を挟む) Type として見る。Offset で struct のフィールドも表す。
//
// 常に 1 段の正規形で、From は CastedValue ではない (NewCastedValue が入れ子を畳む)。以前は入れ子のまま持っていて、
// `((l0 as int) as int16)` の「内側で 1 バイトに狭めてから広げる」を各パスが外側の型とオフセットの合計だけで読み、
// 内側の切り詰めを落とすバグが fuzz で何度も出た (castBits / castFits / plainWord / splitWords / 常駐の差し替え)。
// Width が内側の切り詰めを持つので、「元の変数の一部をそのまま読む」かは Width == Type.Size で分かる (Plain)。
type CastedValue struct {
	From   Operand // *Value または *PointeredArray
	Type   *types.Type
	Offset int
	Width  int // From から読むバイト数 (0〜Type.Size)。Type.Size より小さければ上位はゼロ拡張
}

// NewCastedValue は from を Offset バイト目から typ として見る cast を作る。from が cast なら 1 段に畳む
// (外の cast が読めるのは内側の Width のうち Offset より後だけ)。
func NewCastedValue(from Operand, typ *types.Type, offset int) *CastedValue {
	if c, ok := from.(*CastedValue); ok {
		return &CastedValue{From: c.From, Type: typ, Offset: c.Offset + offset, Width: clampWidth(c.Width-offset, typ)}
	}
	return &CastedValue{From: from, Type: typ, Offset: offset, Width: naturalWidth(from, typ, offset)}
}

// RebaseCast は cv の元の値を from に差し替えたもの (インライン展開・常駐・SSA の差し替え)。cv の Width (元の値で
// 切り詰めた幅) は保ち、from が cast ならそれと畳む。
func RebaseCast(cv *CastedValue, from Operand) *CastedValue {
	r := NewCastedValue(from, cv.Type, cv.Offset)
	if cv.Width < r.Width {
		r.Width = cv.Width
	}
	return r
}

// naturalWidth は cast していない from を offset バイト目から typ として読むときの幅: 変数は自分の大きさまで、
// リテラルは値のビット列を無限に持つので typ の幅全部 (codegen はリテラルの型に関係なく値のバイトを取り出す)。
func naturalWidth(from Operand, typ *types.Type, offset int) int {
	if v, ok := from.(*Value); ok && v.Kind == KindLiteral {
		return typ.Size
	}
	return clampWidth(ValType(from).Size-offset, typ)
}

func clampWidth(w int, typ *types.Type) int {
	return max(0, min(w, typ.Size))
}

// Plain は cv が元の値の一部をそのまま読むか (ゼロ拡張したバイトが無い)。
func (c *CastedValue) Plain() bool { return c.Width == c.Type.Size }

// PlainOperand は o が変数・リテラルそのもの、または元の値の一部をそのまま読む cast か。
func PlainOperand(o Operand) bool {
	if cv, ok := o.(*CastedValue); ok {
		return cv.Plain()
	}
	return true
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
	w := ""
	if c.Truncated() {
		w = fmt.Sprintf("/%d", c.Width)
	}
	if c.Offset == 0 {
		return fmt.Sprintf("<%s%s>%s", c.Type, w, OperandString(c.From))
	}
	return fmt.Sprintf("<%s+%d%s>%s", c.Type, c.Offset, w, OperandString(c.From))
}

// Truncated は cv の Width が、元の値をそのまま cast したときの幅より狭いか (入れ子の cast を畳んで内側で切り詰めた)。
// 表示とダンプだけが使う (golden を変えないため、畳まなくても同じ幅なら表示しない)。
func (c *CastedValue) Truncated() bool {
	return c.Width != naturalWidth(c.From, c.Type, c.Offset)
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
// CastedValue は From の値を、PointeredArray は From の kind だけを引き継ぐ。
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

// ValAddress は v.address (CastedValue は from に委譲し、Offset (struct のフィールド) を足す)。
func ValAddress(v Operand) int {
	switch x := v.(type) {
	case *Value:
		return x.Address
	case *CastedValue:
		return ValAddress(x.From) + x.Offset
	}
	panic(fmt.Sprintf("ValAddress: invalid value %T", v))
}

// ValOffset は CastedValue の連鎖の Offset の合計 (struct のフィールドの、変数先頭からのバイト位置)。
func ValOffset(v Operand) int {
	switch x := v.(type) {
	case *CastedValue:
		return ValOffset(x.From) + x.Offset
	}
	return 0
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
