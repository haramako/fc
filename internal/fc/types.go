package fc

// lib/fc/base.rb の Type の移植。
// Type.new ではなく TypeOf() (Ruby の Type[]) で生成すること (to_s でインターンされる)。

import (
	"fmt"
	"strings"
)

type Type struct {
	Kind   Sym   // :void, :bool, :int, :module, :macro, :pointer, :array, :lambda
	Size   int   // サイズ(byte)。nil相当は -1 (長さ省略配列)
	Signed bool  // intの場合のみ
	Base   *Type // pointer,array,lambdaの場合のみ
	Length int   // 配列の要素数 (nil相当は -1)
	Args   []*Type
	fastcall bool
	str      string
}

// BASIC_TYPES
var basicTypes = map[Sym]struct {
	size   int
	signed bool
}{
	"int": {1, false}, "uint": {1, false}, "sint": {1, true},
	"int8": {1, false}, "sint8": {1, true}, "uint8": {1, false},
	"int16": {2, false}, "sint16": {2, true}, "uint16": {2, false},
}

var typeCache = map[string]*Type{}

// TypeOf は Ruby の Type[ast_or_type] 相当。
func TypeOf(ast any) *Type {
	if t, ok := ast.(*Type); ok {
		return t
	}
	t := newType(ast)
	if cached, ok := typeCache[t.str]; ok {
		return cached
	}
	typeCache[t.str] = t
	return t
}

func newType(ast any) *Type {
	t := &Type{Length: -1}
	switch a := ast.(type) {
	case Sym:
		switch a {
		case "void":
			t.Kind = "void"
			t.Size = 0
		case "bool":
			t.Kind = "bool"
			t.Size = 1
		case "module":
			t.Kind = "module"
			t.Size = 0
		case "macro":
			t.Kind = "macro"
			t.Size = 0
		default:
			t.Kind = "int"
			if bt, ok := basicTypes[a]; ok {
				t.Size = bt.size
				t.Signed = bt.signed
			} else {
				panic(fmt.Sprintf("invalid basic type %s", a))
			}
		}
	case []any:
		switch a[0] {
		case Sym("pointer"):
			t.Kind = "pointer"
			t.Base = TypeOf(a[1])
			t.Size = 2
		case Sym("array"):
			t.Kind = "array"
			t.Base = TypeOf(a[2])
			if n, ok := a[1].(int); ok {
				t.Length = n
				t.Size = t.Base.Size * n
			} else {
				// 長さ省略 (Rubyでは length=nil, size=nil)
				t.Length = -1
				t.Size = -1
			}
		case Sym("lambda"):
			t.Kind = "lambda"
			t.Base = TypeOf(a[2])
			args := a[1].([]any)
			t.Args = make([]*Type, len(args))
			for i, at := range args {
				t.Args[i] = TypeOf(at)
			}
			if len(a) > 3 {
				t.fastcall = truthy(a[3])
			}
			t.Size = 2
		}
	}

	switch t.Kind {
	case "void":
		t.str = "void"
	case "int", "bool":
		s := "u"
		if t.Signed {
			s = "s"
		}
		t.str = fmt.Sprintf("%s%s%d", s, t.Kind, t.Size*8)
	case "module":
		t.str = "module"
	case "macro":
		t.str = "macro"
	case "pointer":
		t.str = t.Base.str + "*"
	case "array":
		l := ""
		if t.Length >= 0 {
			l = fmt.Sprintf("%d", t.Length)
		}
		t.str = fmt.Sprintf("%s[%s]", t.Base.str, l)
	case "lambda":
		fc := ""
		if t.fastcall {
			fc = "fastcall "
		}
		args := make([]string, len(t.Args))
		for i, a := range t.Args {
			args[i] = a.str
		}
		t.str = fmt.Sprintf("%s%s(%s)", fc, t.Base.str, strings.Join(args, ","))
	default:
		panic(fmt.Sprintf("invalid type declaration %v", ast))
	}
	return t
}

func (t *Type) Fastcall() bool  { return t.fastcall }
func (t *Type) String() string  { return t.str }

// truthy は Ruby の真偽値評価 (nil と false のみ偽)。
func truthy(v any) bool {
	if v == nil {
		return false
	}
	if b, ok := v.(bool); ok {
		return b
	}
	return true
}
