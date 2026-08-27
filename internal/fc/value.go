package fc

// lib/fc/base.rb の Value / CastedValue / PointeredArray の移植。

import "fmt"

// LiveRange は Ruby の Range (min..max) 相当。nil は *LiveRange(nil) で表す。
type LiveRange struct {
	Min, Max int
}

type Value struct {
	Kind       Sym   // :local, :global, :literal, :array_literal, :module
	Type       *Type
	Id         any // 変数名 (Sym) または nil
	Val        any // literal/array_literal の場合のみ (int, Sym, []any(Value列), string, *Module, マクロ等)
	Opt        *OMap
	BaseString any // 元の値が文字列だった場合、その文字列 (string)。なければ nil
	Public     bool

	// 以下はアセンブラで使用
	Address      any // アドレス (int) または nil
	Location     Sym // :frame, :reg, :mem, :none, :a, :cond, :fastcall_reg または ""
	Unuse        bool
	LiveRange    *LiveRange
	CondReg      Sym  // :carry, :zero, :negative (location==:cond のときのみ)
	CondPositive bool // location==:cond のときのみ
}

func NewValue(kind Sym, id any, typ *Type, val any, opt *OMap) *Value {
	if typ == nil {
		panic(&CompileError{Msg: "invalid type, nil"})
	}
	switch kind {
	case "local", "global", "literal", "array_literal", "module":
	default:
		panic(fmt.Sprintf("invalid kind, %s", kind))
	}
	if opt == nil {
		opt = NewOMap()
	}
	return &Value{Kind: kind, Id: id, Type: typ, Val: val, Opt: opt}
}

// NewIntValue は Value.new_int 相当。
func NewIntValue(n int) *Value {
	var t *Type
	switch {
	case n >= 256:
		t = TypeOf(Sym("int16"))
	case n < -127:
		t = TypeOf(Sym("sint16"))
	case n < 0:
		t = TypeOf(Sym("sint8"))
	default:
		t = TypeOf(Sym("int8"))
	}
	return NewValue("literal", nil, t, n, nil)
}

func (v *Value) Assignable() bool {
	return v.Kind == "local" || v.Kind == "global"
}

// CastedValue は reinterpret_cast 相当。Ruby では Delegator で @from に委譲される。
type CastedValue struct {
	From   any // *Value または *CastedValue
	Type   *Type
	Offset int
}

func NewCastedValue(from any, typ *Type, offset int) *CastedValue {
	return &CastedValue{From: from, Type: typ, Offset: offset}
}

// PointeredArray は配列からポインタへ自動変換された値。
type PointeredArray struct {
	From any // *Value (または CastedValue)
	Type *Type
}

func NewPointeredArray(from any) *PointeredArray {
	return &PointeredArray{From: from, Type: TypeOf([]any{Sym("pointer"), ValType(from).Base})}
}

// ---------------------------------------------------------------
// Ruby の to_s / inspect 相当 (エラーメッセージで使用)
// ---------------------------------------------------------------

// Inspect は Value#inspect 相当。
func (v *Value) Inspect() string {
	if v.Id != nil {
		return fmt.Sprintf("{%s:%s}", ToS(v.Id), v.Type)
	} else if v.BaseString != nil {
		return `{"` + v.BaseString.(string) + `"}`
	}
	return fmt.Sprintf("{%s}", ToS(v.Val))
}

// String は Value#to_s 相当。
func (v *Value) String() string {
	if v.Id != nil {
		return fmt.Sprintf("{%s}", ToS(v.Id))
	}
	return v.Inspect()
}

// String は CastedValue#to_s 相当。
func (c *CastedValue) String() string {
	if c.Offset == 0 {
		return fmt.Sprintf("<%s>%s", c.Type, valToS(c.From))
	}
	return fmt.Sprintf("<%s+%d>%s", c.Type, c.Offset, valToS(c.From))
}

// String は PointeredArray#to_s 相当。
func (p *PointeredArray) String() string {
	return valToS(p.From) + "#p"
}

// valToS は ops の要素になりうる値の to_s。
func valToS(v any) string {
	return ToS(v)
}

// ---------------------------------------------------------------
// 動的ディスパッチヘルパ
// Ruby では CastedValue(Delegator) がメソッドを @from に委譲し、
// PointeredArray は kind のみ委譲する。ops の要素は Value/CastedValue/PointeredArray/
// Sym/string/Type/Lambda/int/nil のいずれか。
// ---------------------------------------------------------------

// UnderlyingValue は CastedValue の委譲チェーンをたどって *Value を返す。
// (Delegator は hash/eql? も委譲するため、Hashキーとしては From と同一視される)
func UnderlyingValue(v any) *Value {
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
func ValKind(v any) Sym {
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
func ValType(v any) *Type {
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

// ValVal は v.val (PointeredArray は常に nil)。
func ValVal(v any) any {
	switch x := v.(type) {
	case *Value:
		return x.Val
	case *CastedValue:
		return ValVal(x.From)
	case *PointeredArray:
		return nil
	}
	panic(fmt.Sprintf("ValVal: invalid value %T", v))
}

// ValAssignable は v.assignable?
func ValAssignable(v any) bool {
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
func ValLocation(v any) Sym {
	switch x := v.(type) {
	case *Value:
		return x.Location
	case *CastedValue:
		return ValLocation(x.From)
	}
	panic(fmt.Sprintf("ValLocation: invalid value %T", v))
}
