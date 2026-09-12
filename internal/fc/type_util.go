package fc

// lib/fc/type_util.rb の移植。

import "fmt"

// GuessType は TypeUtil.guess_type 相当。
func GuessType(typ *Type, val Operand) *Type {
	if typ != nil {
		return CompatibleType(typ, ValType(val))
	}
	return ValType(val)
}

// CompatibleTypeOk は TypeUtil.compatible_type? 相当 (互換性がなければ nil)。
func CompatibleTypeOk(a, b *Type) *Type {
	if a == b {
		return a
	}
	if a.Kind == "int" && b.Kind == "int" {
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
	} else if a.Kind == "pointer" && b.Kind == "array" && a.Base == b.Base {
		return a
	} else if a.Kind == "array" && b.Kind == "array" && a.Base == b.Base && a.Length < 0 {
		// 配列の長さを省略した場合
		return b
	} else if a.Kind == "array" && b.Kind == "array" && a.Base == b.Base && a.Length != b.Length {
		return TypeOf([]any{Sym("pointer"), a.Base})
	}
	return nil
}

// CompatibleType は TypeUtil.compatible_type 相当 (互換性がなければ CompileError)。
func CompatibleType(a, b *Type) *Type {
	r := CompatibleTypeOk(a, b)
	if r == nil {
		panic(&CompileError{Msg: fmt.Sprintf("not compatible type '%s' and '%s'", a, b)})
	}
	return r
}
