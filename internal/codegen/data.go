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
	if typ.Kind == types.Struct || (typ.Kind == types.Array && (typ.Base.Kind == types.Struct || typ.Base.Kind == types.Array || typ.Base.IsFarFunc())) {
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
			if _, ok := elem.(*ir.CastedValue); ok && typ.Base.Size == 1 && ir.ValLiteral(elem).Type.IsFarFunc() {
				parts = append(parts, strings.TrimPrefix(l.byte(elem, 0), "#"))
				continue
			}
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
		if t.IsFarFunc() {
			var bytes []string
			for i := 0; i < 3; i++ {
				bytes = append(bytes, strings.TrimPrefix(l.byte(v, i), "#"))
			}
			r = append(r, "\t.byte "+strings.Join(bytes, ","))
			return
		}
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

// referencedDefs は関数の中の定義 (defs、出力する行は blocks) のうち、コード (code) か、残したほかの定義から参照されるものの
// 添字を元の順で返す。参照はコメント (`;` から後) を除いた行の中の名前で見る。
func referencedDefs(defs []*ir.Def, blocks [][]any, code []string) []int {
	strip := func(lines []string) string {
		var b strings.Builder
		for _, s := range lines {
			if i := strings.IndexByte(s, ';'); i >= 0 {
				s = s[:i]
			}
			b.WriteString(s)
			b.WriteByte('\n')
		}
		return b.String()
	}
	refers := func(text, sym string) bool {
		isID := func(c byte) bool {
			return c == '_' || c == '$' || c == '@' || c == '.' || '0' <= c && c <= '9' || 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z'
		}
		for i := 0; ; {
			k := strings.Index(text[i:], sym)
			if k < 0 {
				return false
			}
			s, e := i+k, i+k+len(sym)
			if (s == 0 || !isID(text[s-1])) && (e == len(text) || !isID(text[e])) {
				return true
			}
			i = s + 1
		}
	}
	keep := make([]bool, len(defs))
	pending := []string{strip(code)}
	for len(pending) > 0 {
		text := pending[0]
		pending = pending[1:]
		for i, d := range defs {
			if !keep[i] && refers(text, d.Sym) {
				keep[i] = true
				// 自分のラベル行 (`sym:`) を除いた本体から、ほかの定義への参照を探す
				body := (&asmLines{lines: blocks[i]}).flatten()
				if len(body) > 0 {
					body = body[1:]
				}
				pending = append(pending, strip(body))
			}
		}
	}
	var r []int
	for i := range defs {
		if keep[i] {
			r = append(r, i)
		}
	}
	return r
}
