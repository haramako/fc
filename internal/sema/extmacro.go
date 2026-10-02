package sema

// 外部コマンドの定数マクロ (fc.toml の [macro_server.<name>]。internal/extmacro、Agent/wiki/plans/external-macros.md)。
// driver が UseExternalMacros で渡したマクロを、組み込みの定数マクロ (@lz4 など) と同じく `@名前` で登録する。引数は定数
// (整数・文字列・整数の配列) で、結果は定数 (整数・型付きの整数の配列・u8 の配列・文字列)。

import (
	"fmt"

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/extmacro"
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/types"
)

// UseExternalMacros は pool のマクロを `@名前` で使えるようにする (組み込みと同じ名前はエラー)。
func (p *Program) UseExternalMacros(pool *extmacro.Pool) error {
	h := &Hlc{prog: p, scope: p.global}
	for _, name := range pool.Macros() {
		at := "@" + name
		if p.global.Find(at, true) != nil {
			return fmt.Errorf("external macro %s has the same name as a built-in", at)
		}
		h.defconstmacro(at, func(h *Hlc, args []*cexpr) *cexpr {
			return h.callExternalMacro(pool, name, args)
		})
	}
	return nil
}

// callExternalMacro はマクロ name を定数の引数で呼び、結果を定数にする。
func (h *Hlc) callExternalMacro(pool *extmacro.Pool, name string, args []*cexpr) *cexpr {
	vals := make([]any, len(args))
	for i, a := range args {
		v, err := h.externalArg(a)
		if err != nil {
			panic(&diag.Error{Msg: fmt.Sprintf("argument %d of @%s: %v", i+1, name, err)})
		}
		vals[i] = v
	}
	r, err := pool.Call(name, vals)
	if err != nil {
		panic(&diag.Error{Msg: err.Error()})
	}
	switch r.Kind {
	case "int":
		if r.Type == "" {
			return cv(h.IntValue(r.Int))
		}
		return cv(ir.NewIntLiteral("", h.externalIntType(r.Type), r.Int))
	case "data", "bytes":
		t, data := h.prog.Types.IntType(1, false), r.Data
		if r.Kind == "data" {
			t = h.externalIntType(r.Type)
		} else {
			data = make([]int, len(r.Bytes))
			for i, b := range r.Bytes {
				data[i] = int(b)
			}
		}
		elems := make([]ir.Operand, len(data))
		for i, n := range data {
			elems[i] = ir.NewIntLiteral("", t, n)
		}
		return cv(ir.NewArrayLiteral(h.tmpName("$"), h.prog.Types.ArrayOf(t, len(elems)), elems))
	}
	return h.constEval(cstr(r.Str)) // 文字列のリテラルと同じ (fc 4 は 0 終端にしない [N]u8)
}

// externalIntType は結果の型の名前 (u8 / i8 / u16 / i16) の型。
func (h *Hlc) externalIntType(name string) *types.Type {
	size, signed := 1, name[0] == 'i'
	if name[1:] == "16" {
		size = 2
	}
	return h.prog.Types.IntType(size, signed)
}

// externalArg は評価済みの定数 c をマクロの引数 (int / string / []byte / []int) にする。
func (h *Hlc) externalArg(c *cexpr) (any, error) {
	if c.kind == cValue {
		v := c.val
		if lit := h.prog.constArrays[v]; lit != nil {
			v = lit // 名前付きの配列定数 (`const D:[3]u8 = [...]`)
		}
		switch {
		case v.IsString:
			return v.Str, nil
		case v.Kind == ir.KindLiteral && v.IsInt:
			return v.Int, nil
		case v.Kind == ir.KindArrayLiteral && v.Type.Kind == types.Array && v.Type.Base.Kind == types.Int:
			ns := make([]int, len(v.Elems))
			for i, e := range v.Elems {
				x := ir.ValLiteral(e)
				if x == nil || !x.IsInt {
					return nil, fmt.Errorf("the array elements must be constant integers")
				}
				ns[i] = x.Int
			}
			if t := v.Type.Base; t.Size == 1 && !t.Signed {
				b := make([]byte, len(ns))
				for i, n := range ns {
					b[i] = byte(n)
				}
				return b, nil // u8 の配列はバイト列 (@incbin の中身など)
			}
			return ns, nil
		}
	}
	return nil, fmt.Errorf("must be a constant integer, string or integer array")
}
