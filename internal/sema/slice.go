package sema

// fc 3 の slice `[]T` / `[]const T` (doc/v3_plan.md)。表現は struct { ptr:*T; len:u8 } の 3 バイトの値 (types.Slice)。
// 値のコピー・引数・戻り値は struct の仕組みのまま、ここでは作り方と使い方を書き換える:
//
//   - 配列 (長さの分かるもの) は slice に暗黙に変換できる (withExpected が opToSlice を挟む)。長さは配列の長さ、
//     文字列リテラルは終端の 0 を含めない (メモリには 0 が残る)
//   - s[i] は s.ptr[i] (範囲の検査はしない)。a[lo..hi] / a[..] は配列・slice の一部 (lo / hi は省ける。定数なら範囲を検査)
//   - @len(x) は配列の長さ (定数)・slice の長さ・enum のメンバーの数。@slice(p, n) / @ptr(s) はポインタとの変換、
//     @copy(dst, src) は短いほうの長さだけ写して、その要素数を返す (mem.copy を使う)
//
// 長さは u8 (要素は 255 個まで): ゲームの配列は大半が小さく、添字とループを 8 ビットで済ませるため。大きいもの
// (メモリのコピーなど) は長さ u16 の広い slice `[:u16]T` (4 バイト)。普通 → 広いは暗黙 (長さを広げて作り直す)、
// 広い → 普通は長さが収まらないことがあるので @slice(@ptr(s), n) と書く。256 要素以上の配列の範囲は広い slice。

import (
	"fmt"

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/syntax"
	"github.com/haramako/fc/internal/types"
)

// maxSliceLen は slice の長さの上限 (len は u8、広い slice は u16)。
func maxSliceLen(wide bool) int {
	if wide {
		return 0xffff
	}
	return 0xff
}

// sliceParts は配列・slice の値の、要素の型・先頭 (ptr: ポインタの値、base: 添字の元)・長さ。
type sliceParts struct {
	elem *types.Type
	ptr  ir.Operand // 先頭へのポインタ (*elem)
	base ir.Operand // OpIndex の元 (配列ならその値、それ以外は ptr)
	len  ir.Operand // 長さ (u8、広い slice は u16)
	n    int        // 長さが定数ならその値 (でなければ -1)
	ro   bool       // 読み取り専用のデータ
	wide bool       // 長さが u16 (広い slice、または 256 要素以上の配列)
}

// sliceParts は c (配列・slice の式) を評価して、先頭と長さを取り出す。what はエラーの表示用。
func (h *Hlc) sliceParts(c *cexpr, what string) sliceParts {
	v, lv := h.lvalValue(c)
	p, rv, ok := h.partsOf(v, lv, h.isStringLit(c), what)
	if !ok {
		panic(&diag.Error{Msg: fmt.Sprintf("%s: %s is not an array or a slice (type %s)", what, describe(rv), ir.ValType(rv))})
	}
	return p
}

// partsOf は評価済みの (v, lv) の sliceParts。配列・slice でなければ ok = false で、rv はその右辺値。
// slice なら rv は slice の値そのもの。
// str は式が文字列リテラルか (長さは終端の 0 を含めない)。
func (h *Hlc) partsOf(v ir.Operand, lv, str bool, what string) (p sliceParts, rv ir.Operand, ok bool) {
	if lv {
		if b := ir.ValType(v).Base; b != nil && b.Kind == types.Array && !b.IsSoa {
			// ポインタ経由の配列 (`p.arr`): v は配列へのポインタ
			ptr := ir.NewCastedValue(v, h.prog.Types.PointerTo(b.Base), 0)
			return h.arrayParts(b, ptr, ptr, h.readOnly(v), false, what), ptr, true
		}
		v = h.rvalOf(v, lv)
	}
	t := ir.ValType(v)
	switch {
	case t.IsSlice():
		ptr := ir.NewCastedValue(v, h.prog.Types.PointerTo(t.SliceOf), 0)
		return sliceParts{elem: t.SliceOf, ptr: ptr, base: ptr, len: ir.NewCastedValue(v, t.SliceLen(), 2), n: -1, ro: h.readOnly(v), wide: t.IsWideSlice()}, v, true
	case t.Kind == types.Array && !t.IsSoa:
		return h.arrayParts(t, ir.NewPointeredArray(v, h.prog.Types.PointerTo(t.Base)), v, h.readOnly(v), str, what), v, true
	}
	return sliceParts{}, v, false
}

// arrayParts は配列型 t の値の sliceParts (str なら文字列リテラルで、長さは終端の 0 を含めない)。
func (h *Hlc) arrayParts(t *types.Type, ptr, base ir.Operand, ro, str bool, what string) sliceParts {
	n := t.Length
	if n < 0 {
		panic(&diag.Error{Msg: fmt.Sprintf("%s: the length of %s is not known (make the slice with @slice(p, n))", what, t)})
	}
	if str && n > 0 {
		n--
	}
	return sliceParts{elem: t.Base, ptr: ptr, base: base, len: h.IntValue(n), n: n, ro: ro, wide: n > maxSliceLen(false)}
}

// newSlice は要素 elem の slice (wide なら長さ u16) の一時変数に ptr と len を入れる。
func (h *Hlc) newSlice(elem *types.Type, ptr, n ir.Operand, ro, wide bool) *ir.Value {
	st := h.prog.Types.Slice(elem, false, wide)
	tmp := h.newTmp(st)
	h.markReadOnly(tmp, ro)
	h.emit(&ir.Op{Code: ir.OpLoad, Dst: ir.NewCastedValue(tmp, h.prog.Types.PointerTo(elem), 0), Src: []ir.Operand{ptr}})
	if !wide {
		n = h.lowByte(n)
	}
	h.emit(&ir.Op{Code: ir.OpLoad, Dst: ir.NewCastedValue(tmp, st.SliceLen(), 2), Src: []ir.Operand{n}})
	return tmp
}

// lowByte は整数 v の下位 1 バイト (slice の長さ)。
func (h *Hlc) lowByte(v ir.Operand) ir.Operand {
	if ir.ValType(v).Size == 1 {
		return v
	}
	return ir.NewCastedValue(v, h.prog.Types.IntType(1, false), 0)
}

// isStringLit は c が文字列リテラル (評価すると IsString の配列の定数) か。
func (h *Hlc) isStringLit(c *cexpr) bool {
	e := h.constEval(c)
	return e.kind == cValue && e.val.IsString
}

// retypePtr はポインタの値 p を型 pt のポインタとして見る (配列から作ったポインタは、そのまま型だけを替える)。
func retypePtr(p ir.Operand, pt *types.Type) ir.Operand {
	if pa, ok := p.(*ir.PointeredArray); ok {
		return ir.NewPointeredArray(pa.From, pt)
	}
	return ir.NewCastedValue(p, pt, 0)
}

// toSlice は c (配列 / slice) を slice の型 st にする。配列・slice でなければそのまま (型の検査は代入側がする)。
func (h *Hlc) toSlice(c *cexpr, st *types.Type) ir.Operand {
	if c.kind == cNull {
		panic(&diag.Error{Msg: fmt.Sprintf("null cannot be used as %s (use an empty slice: a[0..0])", st)})
	}
	c = h.withExpected(c, h.prog.Types.ArrayOf(st.SliceOf, -1))
	v, lv := h.lvalValue(c)
	p, rv, ok := h.partsOf(v, lv, h.isStringLit(c), "slice")
	if !ok {
		return rv // 型の検査は代入側がする
	}
	if rt := ir.ValType(rv); rt.IsSlice() {
		switch {
		case rt.SliceOf != st.SliceOf || rt.IsWideSlice() == st.IsWideSlice():
			return rv // 同じ幅 (と要素の違い) は代入側が検査する
		case st.IsWideSlice():
			return h.newSlice(p.elem, p.ptr, p.len, p.ro, true) // 普通 → 広い: 長さを広げて作り直す
		}
		panic(&diag.Error{Msg: fmt.Sprintf("cannot use %s as %s implicitly (the length may not fit; use @slice(@ptr(s), n))", rt, st)})
	}
	if p.elem != st.SliceOf {
		panic(&diag.Error{Msg: fmt.Sprintf("cannot use %s as %s (element types differ)", ir.ValType(rv), st)})
	}
	if max := maxSliceLen(st.IsWideSlice()); p.n > max {
		hint := ""
		if !st.IsWideSlice() {
			hint = "; use [:u16]" + st.SliceOf.String() + " for longer ones"
		}
		panic(&diag.Error{Msg: fmt.Sprintf("cannot use %s as %s: the slice has at most %d elements%s", ir.ValType(rv), st, max, hint)})
	}
	return h.newSlice(p.elem, p.ptr, p.len, p.ro, st.IsWideSlice())
}

// sliceRange は a[lo..hi] (lo / hi は省けば nil)。
func (h *Hlc) sliceRange(a, lo, hi *cexpr) ir.Operand {
	p := h.sliceParts(a, "a[lo..hi]")
	u8 := h.prog.Types.IntType(1, false)
	intOf := func(c *cexpr, what string) ir.Operand {
		v := h.rval(c)
		if t := ir.ValType(v); t.Kind != types.Int && t.Kind != types.Bool || t.Enum != nil {
			panic(&diag.Error{Msg: fmt.Sprintf("a[lo..hi]: %s must be an integer (got %s)", what, t)})
		}
		return v
	}
	var loV, hiV ir.Operand = ir.NewIntLiteral("", u8, 0), p.len
	if lo != nil {
		loV = intOf(lo, "lo")
	}
	if hi != nil {
		hiV = intOf(hi, "hi")
	}
	// 定数の範囲だけ検査する
	loN, loLit := ir.ValIntLiteral(loV)
	hiN, hiLit := ir.ValIntLiteral(hiV)
	if hi == nil && p.n >= 0 {
		hiN, hiLit = p.n, true
	}
	switch {
	case loLit && loN < 0, hiLit && hiN < 0:
		panic(&diag.Error{Msg: "a[lo..hi]: negative index"})
	case loLit && hiLit && loN > hiN:
		panic(&diag.Error{Msg: fmt.Sprintf("a[lo..hi]: lo (%d) > hi (%d)", loN, hiN)})
	case p.n >= 0 && hiLit && hiN > p.n:
		panic(&diag.Error{Msg: fmt.Sprintf("a[lo..hi]: hi (%d) is out of range (length %d)", hiN, p.n)})
	case p.n >= 0 && loLit && loN > p.n:
		panic(&diag.Error{Msg: fmt.Sprintf("a[lo..hi]: lo (%d) is out of range (length %d)", loN, p.n)})
	case hiLit && hiN > maxSliceLen(p.wide):
		panic(&diag.Error{Msg: fmt.Sprintf("a[lo..hi]: a slice has at most %d elements", maxSliceLen(p.wide))})
	}
	ptr := p.ptr
	if !loLit || loN != 0 {
		t := h.newTmp(h.prog.Types.PointerTo(p.elem))
		h.markReadOnly(t, p.ro)
		h.emit(&ir.Op{Code: ir.OpIndex, Dst: t, Src: []ir.Operand{p.base, loV}})
		ptr = t
	}
	var n ir.Operand
	switch {
	case loLit && hiLit:
		n = ir.NewIntLiteral("", u8, hiN-loN)
	case loLit && loN == 0:
		n = hiV
	case p.wide:
		t := h.newTmp(h.prog.Types.IntType(2, false))
		h.emit(&ir.Op{Code: ir.OpSub, Dst: t, Src: []ir.Operand{hiV, loV}})
		n = t
	default:
		t := h.newTmp(u8)
		h.emit(&ir.Op{Code: ir.OpSub, Dst: t, Src: []ir.Operand{h.lowByte(hiV), h.lowByte(loV)}})
		n = t
	}
	return h.newSlice(p.elem, ptr, n, p.ro, p.wide)
}

// registerSliceBuiltins は @len / @slice / @ptr / @copy を登録する (fc 3 だけ: `@` の名前は fc 3 の字句)。
func registerSliceBuiltins(h *Hlc) {
	// @len(x): 配列は長さ (定数)、enum の型名はメンバーの数、slice は長さ (u8)
	h.defconstmacro("@len", func(h *Hlc, args []*cexpr) *cexpr {
		if len(args) != 1 {
			panic(&diag.Error{Msg: "@len takes 1 argument (an array, a slice or an enum type)"})
		}
		a := args[0]
		if a.kind == cValue {
			if t := a.val.TypeRef; a.val.Type.Kind == types.TypeName && t != nil {
				if t.Enum == nil {
					panic(&diag.Error{Msg: fmt.Sprintf("@len(%s): a type has no length (only enum types)", t)})
				}
				return cv(h.IntValue(len(t.Enum.Members)))
			}
			if t := a.val.Type; t.Kind == types.Array && !t.IsSoa && t.Length >= 0 {
				n := t.Length
				if a.val.IsString && n > 0 {
					n--
				}
				return cv(h.IntValue(n))
			}
		}
		return &cexpr{kind: cOp, op: opLen, args: []*cexpr{a}}
	})

	// @slice(p, n): ポインタ (または配列) p の先頭から n 要素の slice (n が u16 なら広い slice)
	h.defmacro("@slice", func(h *Hlc, args []*cexpr, block *syntax.Block) macroResult {
		if len(args) != 2 {
			panic(&diag.Error{Msg: "@slice takes 2 arguments (@slice(p, n))"})
		}
		p := h.rval(args[0])
		t := ir.ValType(p)
		if t.Kind == types.Array && !t.IsSoa {
			p, t = ir.NewPointeredArray(p, h.prog.Types.PointerTo(t.Base)), h.prog.Types.PointerTo(t.Base)
		}
		if t.Kind != types.Pointer || t.Base.Kind == types.Void {
			panic(&diag.Error{Msg: fmt.Sprintf("@slice: the first argument must be a typed pointer (got %s)", t)})
		}
		n := h.rval(args[1])
		if nt := ir.ValType(n); nt.Kind != types.Int || nt.Enum != nil {
			panic(&diag.Error{Msg: fmt.Sprintf("@slice: the length must be an integer (got %s)", nt)})
		}
		// 長さが u16 (の値) なら広い slice
		wide := ir.ValType(n).Size == 2
		if k, lit := ir.ValIntLiteral(n); lit && (k < 0 || k > maxSliceLen(wide)) {
			panic(&diag.Error{Msg: fmt.Sprintf("@slice: length %d is out of range (0..%d)", k, maxSliceLen(wide))})
		}
		return macroResult{expr: cv(h.newSlice(t.Base, p, n, h.readOnly(p), wide))}
	})

	// @ptr(s): slice (または配列) の先頭へのポインタ
	h.defmacro("@ptr", func(h *Hlc, args []*cexpr, block *syntax.Block) macroResult {
		if len(args) != 1 {
			panic(&diag.Error{Msg: "@ptr takes 1 argument (a slice)"})
		}
		p := h.sliceParts(args[0], "@ptr")
		tmp := h.newTmp(h.prog.Types.PointerTo(p.elem))
		h.markReadOnly(tmp, p.ro)
		h.emit(&ir.Op{Code: ir.OpLoad, Dst: tmp, Src: []ir.Operand{p.ptr}})
		return macroResult{expr: cv(tmp)}
	})

	// @copy(dst, src): 短いほうの長さだけ src から dst へ写し、写した要素数 (u8。広い slice があれば u16) を返す
	h.defmacro("@copy", func(h *Hlc, args []*cexpr, block *syntax.Block) macroResult {
		if len(args) != 2 {
			panic(&diag.Error{Msg: "@copy takes 2 arguments (@copy(dst, src))"})
		}
		mem, ok := h.prog.Modules.Get("mem")
		if !ok {
			panic(&diag.Error{Msg: "@copy requires the mem module (add `use mem;`)"})
		}
		d := h.sliceParts(args[0], "@copy")
		s := h.sliceParts(args[1], "@copy")
		if d.ro {
			panic(&diag.Error{Msg: "@copy: the destination is read-only"})
		}
		if d.elem != s.elem {
			panic(&diag.Error{Msg: fmt.Sprintf("@copy: element types differ (%s and %s)", d.elem, s.elem)})
		}
		u8, u16 := h.prog.Types.IntType(1, false), h.prog.Types.IntType(2, false)
		var n ir.Operand
		if d.n >= 0 && s.n >= 0 {
			n = h.IntValue(min(d.n, s.n))
		} else {
			n = h.rval(&cexpr{kind: cOp, op: opMin, args: []*cexpr{cv(h.operandValue(d.len)), cv(h.operandValue(s.len))}})
		}
		size := d.elem.Size
		var bytes ir.Operand
		if k, lit := ir.ValIntLiteral(n); lit {
			bytes = ir.NewIntLiteral("", u16, k*size)
		} else {
			w := h.newTmp(u16)
			h.emit(&ir.Op{Code: ir.OpLoad, Dst: w, Src: []ir.Operand{n}})
			bytes = w
			if size != 1 {
				b := h.newTmp(u16)
				h.emit(&ir.Op{Code: ir.OpMul, Dst: b, Src: []ir.Operand{w, ir.NewIntLiteral("", u16, size)}})
				bytes = b
			}
		}
		bp := h.prog.Types.PointerTo(u8)
		dp := h.operandValue(retypePtr(d.ptr, bp))
		sp := h.operandValue(retypePtr(s.ptr, bp))
		h.lval(ccall(cv(mem.Interface().LookupInternal("copy")), cv(dp), cv(sp), cv(h.operandValue(bytes))))
		return macroResult{expr: cv(h.operandValue(n))}
	})
}

// sliceKey は constSlice のメモの鍵 (元の配列の式と slice の型)。
type sliceKey struct {
	c *cexpr
	t *types.Type
}

// constSlice は、配列を slice にする式 (withExpected が挟んだ opToSlice) を、配列が定数のアドレスを持つとき
// (配列・文字列のリテラル、配列の定数・グローバル変数の名前) に、{ 先頭, 長さ } の定数 (ir.KindArrayLiteral、型は
// slice) にする。const の表の中の slice (struct のフィールド・配列の要素・const の宣言) に使う。リテラルは無名の
// 配列定数に切り出す。定数にできなければ c をそのまま返す (実行時に toSlice が作る)。
func (h *Hlc) constSlice(c *cexpr) *cexpr {
	if c.kind != cOp || c.op != opToSlice || c.args[0].kind == cNull {
		return c
	}
	st := c.ty
	key := sliceKey{c.args[0], st}
	if r, ok := h.sliceMemo[key]; ok {
		return r
	}
	x := h.constEval(h.withExpected(c.args[0], h.prog.Types.ArrayOf(st.SliceOf, -1)))
	if x.kind != cValue {
		return c
	}
	v := x.val
	var sym string
	var n int
	switch {
	case v.Kind == ir.KindArrayLiteral && v.Type.Kind == types.Array:
		v = h.fitArrayLiteral(v, h.prog.Types.ArrayOf(st.SliceOf, -1))
		n = len(v.Elems)
		if v.IsString && n > 0 {
			n-- // 文字列リテラルは終端の 0 を含めない (データには残す)
		}
	case v.Kind == ir.KindGlobal && v.Type.Kind == types.Array && !v.Type.IsSoa && v.Symbol != "" && v.Type.Length >= 0:
		n, sym = v.Type.Length, v.Symbol
	default:
		return c
	}
	if v.Type.Base != st.SliceOf && !(v.IsString && st.SliceOf.Kind == types.Int && st.SliceOf.Size == 1) {
		panic(&diag.Error{Msg: fmt.Sprintf("cannot use %s as %s (element types differ)", v.Type, st)})
	}
	if max := maxSliceLen(st.IsWideSlice()); n > max {
		panic(&diag.Error{Msg: fmt.Sprintf("cannot use %s as %s: the slice has at most %d elements", v.Type, st, max)})
	}
	if sym == "" {
		sym = h.addDef(h.tmpName("_"), &ir.Def{Kind: ir.DefBlock, Type: v.Type, Elems: v.Elems})
	}
	r := cv(ir.NewArrayLiteral("", st, []ir.Operand{
		ir.NewSymbolLiteral("", st.Fields[0].Type, sym),
		ir.NewIntLiteral("", st.SliceLen(), n),
	}))
	if h.sliceMemo == nil {
		h.sliceMemo = map[sliceKey]*cexpr{}
	}
	h.sliceMemo[key] = r
	return r
}

// constAddress は、ポインタ pt のフィールドに入る定数 c が配列 (配列の定数・グローバル変数の名前、配列・文字列の
// リテラル) なら、そのアドレスの定数にする (リテラルは無名の配列定数に切り出す。const PS:[N]*T の要素と同じ)。
// それ以外はそのまま。
func (h *Hlc) constAddress(c *cexpr, pt *types.Type) *cexpr {
	if c.kind != cValue {
		return c
	}
	v := c.val
	switch {
	case v.Kind == ir.KindArrayLiteral && v.Type.Kind == types.Array:
		if !v.IsString {
			h.compatibleAssign("field", pt, v.Type)
		}
		sym := h.addDef(h.tmpName("_"), &ir.Def{Kind: ir.DefBlock, Type: v.Type, Elems: v.Elems})
		return cv(ir.NewSymbolLiteral("", pt, sym))
	case v.Kind == ir.KindGlobal && v.Type.Kind == types.Array && !v.Type.IsSoa && v.Symbol != "":
		h.compatibleAssign("field", pt, v.Type)
		return cv(ir.NewSymbolLiteral("", pt, v.Symbol))
	}
	return c
}
