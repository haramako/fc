package codegen

import (
	"fmt"
	"strings"

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/types"
)

// ROM 上のデータ (配列定数・struct 定数・文字列) の出力。

// emitBlock は v を .db/.dw に変換する。
func (l *Llc) emitBlock(sym string, typ *types.Type, val []ir.Operand) []any {
	r := []any{}
	r = append(r, mangle(sym)+":")
	if typ.Kind == types.Struct || (typ.Kind == types.Array && (typ.Base.Kind == types.Struct || typ.Base.Kind == types.Array)) {
		// struct / 入れ子の配列: 要素ごとに型に従って .byte / .word を出す
		return append(r, l.emitData(typ, val)...)
	}
	var op string
	var limit int
	switch typ.Base.Size {
	case 1:
		op = ".byte"
		limit = 256
	case 2:
		op = ".word"
		limit = 65536
	default:
		panic("invalid block base size")
	}
	for s := 0; s < len(val); s += 16 {
		e := min(s+16, len(val))
		parts := make([]string, 0, e-s)
		for _, elem := range val[s:e] {
			lv := ir.ValLiteral(elem)
			switch {
			case lv != nil && lv.Kind == ir.KindLiteral && lv.IsInt:
				parts = append(parts, fmt.Sprintf("%d", ir.FloorMod(lv.Int, limit)))
			case lv != nil && lv.Kind == ir.KindLiteral:
				parts = append(parts, lv.Symbol)
			default:
				parts = append(parts, l.toAsm(elem))
			}
		}
		r = append(r, fmt.Sprintf("\t%s %s", op, strings.Join(parts, ",")))
	}
	return r
}

// emitData は struct / 配列の定数データを型に従って平坦に出力する (1 要素 1 行)。
func (l *Llc) emitData(typ *types.Type, val []ir.Operand) []any {
	r := []any{}
	scalar := func(t *types.Type, v ir.Operand) {
		op, limit := ".byte", 256
		if t.Size == 2 {
			op, limit = ".word", 65536
		} else if t.Size != 1 {
			panic(fmt.Sprintf("invalid data element size %d", t.Size))
		}
		lv := ir.ValLiteral(v)
		switch {
		case lv != nil && lv.Kind == ir.KindLiteral && lv.IsInt:
			r = append(r, fmt.Sprintf("\t%s %d", op, ir.FloorMod(lv.Int, limit)))
		case lv != nil && lv.Kind == ir.KindLiteral:
			r = append(r, fmt.Sprintf("\t%s %s", op, lv.Symbol))
		default:
			r = append(r, fmt.Sprintf("\t%s %s", op, l.toAsm(v)))
		}
	}
	var elem func(t *types.Type, v ir.Operand)
	elem = func(t *types.Type, v ir.Operand) {
		switch t.Kind {
		case types.Struct:
			lv := ir.ValLiteral(v)
			if lv == nil || lv.Kind != ir.KindArrayLiteral || len(lv.Elems) != len(t.Fields) {
				panic(&diag.Error{Msg: fmt.Sprintf("invalid constant for struct %s", t.Name)})
			}
			for i, f := range t.Fields {
				elem(f.Type, lv.Elems[i])
			}
		case types.Array:
			lv := ir.ValLiteral(v)
			if lv == nil || lv.Kind != ir.KindArrayLiteral {
				panic(&diag.Error{Msg: fmt.Sprintf("invalid constant for %s", t)})
			}
			if t.Length >= 0 && len(lv.Elems) != t.Length {
				panic(&diag.Error{Msg: fmt.Sprintf("array %s has %d elements but %d given", t, t.Length, len(lv.Elems))})
			}
			for _, e := range lv.Elems {
				elem(t.Base, e)
			}
		default:
			scalar(t, v)
		}
	}
	if typ.Kind == types.Struct {
		elem(typ, ir.NewArrayLiteral("", typ, val))
	} else {
		for _, v := range val {
			elem(typ.Base, v)
		}
	}
	return r
}
