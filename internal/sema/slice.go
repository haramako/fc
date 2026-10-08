package sema

// fc 3 の slice `[]T` / `[]const T` (Agent/discussions/2026-09-20-v3-plan.md)。表現は struct { ptr:*T; len:u8 } の 3 バイトの値 (types.Slice)。
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

// sliceParts は c (配列・slice の式) を評価して、先頭と長さを取り出す。what はエラーの表示用。要素の型・長さ・幅は型を決める段の
// 計画 (planSliceOf) で決め、ここは値を作るだけ。
func (h *Hlc) sliceParts(c *cexpr, what string) sliceParts {
	v, lv := h.lvalValue(c)
	str := h.isStringLit(c)
	plan := h.planSliceOf(h.lvInfo(c, v, lv), str, what, func() string { return describe(h.rvalOf(v, lv)) })
	return h.partsOf(v, lv, plan)
}

// lvInfo は lval が評価した c (値 v、lv なら左辺値) の値の型: 型を決める段が決めた型 (planInfo)。決められなければ評価した
// 値から (左辺値はポインタの先。soa の要素はハンドル)。
func (h *Hlc) lvInfo(c *cexpr, v ir.Operand, lv bool) exprInfo {
	if !lv {
		return h.planInfo(c, v)
	}
	if info, ok := h.exprType(c); ok {
		return info
	}
	if t := ir.ValType(v); t.Kind != types.SoaRef {
		return exprInfo{t: t.Base}
	}
	return exprInfo{t: ir.ValType(v)}
}

// partsOf は評価済みの (v, lv) (計画 plan の配列・slice) の先頭と長さ。
func (h *Hlc) partsOf(v ir.Operand, lv bool, plan slicePlan) sliceParts {
	p := sliceParts{elem: plan.elem, n: plan.n, wide: plan.wide}
	pt := h.prog.Types.PointerTo(plan.elem)
	if lv {
		if b := ir.ValType(v).Base; b != nil && b.Kind == types.Array {
			// ポインタ経由の配列 (`p.arr`): v は配列へのポインタ
			p.ptr = ir.NewCastedValue(v, pt, 0)
			p.base, p.len, p.ro = p.ptr, h.IntValue(plan.n), h.readOnly(v)
			return p
		}
		v = h.rvalOf(v, lv)
	}
	p.ro = h.readOnly(v)
	if plan.n < 0 { // slice
		p.ptr = ir.NewCastedValue(v, pt, 0)
		p.base, p.len = p.ptr, ir.NewCastedValue(v, h.sliceType(plan).SliceLen(), 2)
		return p
	}
	p.ptr, p.base, p.len = ir.NewPointeredArray(v, pt), v, h.IntValue(plan.n)
	return p
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
	return e.kind == cValue && h.strLen(e.val)
}

// retypePtr はポインタの値 p を型 pt のポインタとして見る (配列から作ったポインタは、そのまま型だけを替える)。
func retypePtr(p ir.Operand, pt *types.Type) ir.Operand {
	if pa, ok := p.(*ir.PointeredArray); ok {
		return ir.NewPointeredArray(pa.From, pt)
	}
	return ir.NewCastedValue(p, pt, 0)
}

// toSlice は c (配列 / slice) を slice の型 st にする。配列・slice でなければそのまま (型の検査は代入側がする)。変換の仕方は
// 型を決める段の計画 (planToSlice)。
func (h *Hlc) toSlice(c *cexpr, st *types.Type) ir.Operand {
	if c.kind == cNull {
		panic(&diag.Error{Msg: fmt.Sprintf("null cannot be used as %s (use an empty slice: a[0..0])", st)})
	}
	c = h.withExpected(c, h.prog.Types.ArrayOf(st.SliceOf, -1))
	v, lv := h.lvalValue(c)
	str := h.isStringLit(c)
	info := h.lvInfo(c, v, lv)
	desc := func() string { return describe(h.rvalOf(v, lv)) }
	t, kind := h.planToSlice(info, st, str, desc)
	if kind == toSliceAsIs {
		return h.rvalOf(v, lv)
	}
	p := h.partsOf(v, lv, h.planSliceOf(info, str, "slice", desc))
	return h.newSlice(p.elem, p.ptr, p.len, p.ro, t.IsWideSlice()) // 普通 → 広いは長さを広げて作り直す
}

// fixedSliceLen は a[x..x + K] / a[x..=x + K] (x は同じ変数、K は定数) の長さ K / K + 1 (その形でなければ false)。長さが u8 に
// 入れば、長さが u16 の配列・slice から切っても普通の slice になる (`bits[i..i + 8]`、i は u16: games の bitmap)。
func (h *Hlc) fixedSliceLen(lo, hi *cexpr, incl bool) (int, bool) {
	if lo == nil || hi == nil {
		return 0, false
	}
	l, r := h.constEval(lo), h.constEval(hi)
	if r.kind != cOp || r.op != opAdd || len(r.args) != 2 || l.kind != cValue || l.val.Kind == ir.KindLiteral {
		return 0, false
	}
	x, k := r.args[0], r.args[1]
	if k.kind == cValue && k.val == l.val {
		x, k = k, x
	}
	if x.kind != cValue || x.val != l.val || !k.isLiteralInt() {
		return 0, false
	}
	n := k.val.Int
	if incl {
		n++
	}
	return n, n >= 0 && n <= maxSliceLen(false)
}

// sliceRange は a[lo..hi] (lo / hi は省けば nil)。
func (h *Hlc) sliceRange(a, lo, hi *cexpr, incl bool) ir.Operand {
	p := h.sliceParts(a, "a[lo..hi]")
	if k, ok := h.fixedSliceLen(lo, hi, incl); ok && p.wide {
		// 長さの決まった窓: 先頭だけ計算して、長さは定数の普通の slice
		loV := h.rval(lo)
		planSliceBound(h.planInfo(lo, loV), "lo")
		t := h.newTmp(h.prog.Types.PointerTo(p.elem))
		h.markReadOnly(t, p.ro)
		h.emit(&ir.Op{Code: ir.OpIndex, Dst: t, Src: []ir.Operand{p.base, loV}})
		return h.newSlice(p.elem, t, ir.NewIntLiteral("", h.prog.Types.IntType(1, false), k), p.ro, false)
	}
	u8 := h.prog.Types.IntType(1, false)
	intOf := func(c *cexpr, what string) ir.Operand {
		v := h.rval(c)
		planSliceBound(h.planInfo(c, v), what) // 型を決める段の計画 (typeplan.go)
		return v
	}
	var loV, hiV ir.Operand = ir.NewIntLiteral("", u8, 0), p.len
	if lo != nil {
		loV = intOf(lo, "lo")
	}
	if hi != nil {
		hiV = intOf(hi, "hi")
		if incl { // `a[lo..=hi]` は `a[lo..hi + 1]` (広い slice なら u16 で足す)
			if k, ok := ir.ValIntLiteral(hiV); ok {
				hiV = h.IntValue(k + 1)
			} else {
				t := h.prog.Types.IntType(1, false)
				if p.wide || ir.ValType(hiV).Size > 1 {
					t = h.prog.Types.IntType(2, false)
				}
				tmp := h.newTmp(t)
				h.emit(&ir.Op{Code: ir.OpAdd, Dst: tmp, Src: []ir.Operand{h.cast(hiV, t), ir.NewIntLiteral("", t, 1)}})
				hiV = tmp
			}
		}
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
	// @len(x): 配列と soa は要素の数 (定数)、enum の型名はメンバーの数、slice は長さ (u8)
	h.defconstmacro("@len", func(h *Hlc, args []*cexpr) *cexpr {
		if len(args) != 1 {
			panic(&diag.Error{Msg: "@len takes 1 argument (an array, a slice or an enum type)"})
		}
		a := args[0]
		if t := a.typeOf(); t != nil && t.Kind != types.Soa {
			if t.Enum == nil {
				panic(&diag.Error{Msg: fmt.Sprintf("@len(%s): a type has no length (only enum types)", t)})
			}
			return cv(h.IntValue(len(t.Enum.Members)))
		}
		if a.kind == cValue && a.val.Type.Kind == types.Soa {
			h.soaOf(a.val.Type) // 宣言を解決して要素の数を決める (宣言より前の const からも)
			return cv(h.IntValue(a.val.Type.Length))
		}
		if a.kind == cValue {
			if t := a.val.Type; t.Kind == types.Array && t.Length >= 0 {
				n := t.Length
				if h.strLen(a.val) && n > 0 {
					n--
				}
				return cv(h.IntValue(n))
			}
		}
		return &cexpr{kind: cOp, op: opLen, args: []*cexpr{a}}
	})

	// @slice(p, n): ポインタ (または配列) p の先頭から n 要素の slice (n が u16 なら広い slice)
	h.defmacroTyped("@slice", sliceMacro, func(h *Hlc, args []*cexpr, block *syntax.Block) macroResult {
		if len(args) != 2 {
			panic(&diag.Error{Msg: "@slice takes 2 arguments (@slice(p, n))"})
		}
		p := h.rval(args[0])
		t := ir.ValType(p)
		if t.Kind == types.Array {
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
	h.defmacroTyped("@ptr", ptrMacro, func(h *Hlc, args []*cexpr, block *syntax.Block) macroResult {
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
		copyFn := h.moduleFunc(h.builtinModule("mem"), "copy") // use しなくても読み込む (printf の fmt と同じ)
		if ft := ir.ValType(copyFn); ft.Kind == types.Func && len(ft.Params) == 2 {
			// fc 4 の mem.copy(dst:[:u16]u8, src:[:u16]const u8) (Agent/wiki/plans/v4-stdlib.md §3.1)。旧 fclib の mem (プロジェクトの横に
			// コピーしたもの) なら下の copy(to, from, size)
			nb := h.operandValue(bytes)
			h.lval(ccall(cv(copyFn), cv(h.newSlice(u8, dp, nb, false, true)), cv(h.newSlice(u8, sp, nb, true, true))))
			return macroResult{expr: cv(h.operandValue(n))}
		}
		h.lval(ccall(cv(copyFn), cv(dp), cv(sp), cv(h.operandValue(bytes))))
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
		if v.StrTerm && n > 0 {
			n-- // 文字列リテラルは終端の 0 を含めない (データには残す。fc 4 の文字列には 0 が無い)
		}
	case v.Kind == ir.KindGlobal && v.Type.Kind == types.Array && v.Symbol != "" && v.Type.Length >= 0:
		n, sym = v.Type.Length, v.Symbol
		if h.strLen(v) && n > 0 {
			n-- // 名前付きの文字列定数 (fc 4): リテラルと同じく終端の 0 を含めない
		}
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
	if c.kind == cOp && c.op == opRef {
		return h.constRefAddress(c, pt)
	}
	if c.kind != cValue {
		return c
	}
	v := c.val
	switch {
	case v.Kind == ir.KindArrayLiteral && v.Type.Kind == types.Array:
		if !v.IsString {
			h.compatibleAssign("field", pt, v.Type)
		}
		h.strPtr(v)
		sym := h.addDef(h.tmpName("_"), &ir.Def{Kind: ir.DefBlock, Type: v.Type, Elems: v.Elems})
		return cv(ir.NewSymbolLiteral("", pt, sym))
	case v.Kind == ir.KindGlobal && v.Type.Kind == types.Array && v.Symbol != "":
		h.compatibleAssign("field", pt, v.Type)
		h.strPtr(v)
		return cv(ir.NewSymbolLiteral("", pt, v.Symbol))
	}
	return c
}

// constRefAddress は const の表の要素・struct のフィールドに書いたグローバル変数・配列定数の (要素・フィールドの) アドレス
// (`&gp`、`&g[1]`、`&s.f`、`&a[2].f`) を、シンボル + バイト数の定数にする (データに `.word sym+N` と並ぶ)。添字は定数だけ。
// pt は置く所のポインタの型 (nil なら指す先の型のポインタ)。ローカル変数・実行時の添字・ポインタの先などは c のまま。
func (h *Hlc) constRefAddress(c *cexpr, pt *types.Type) *cexpr {
	if c.kind != cOp || c.op != opRef {
		return c
	}
	base, off, t, ok := h.constLvalAddr(c.args[0])
	if !ok {
		return c
	}
	ptr := h.prog.Types.PointerToRO(t, base.ReadOnly)
	if pt != nil {
		h.compatibleAssign("address", pt, ptr)
		ptr = pt
	}
	v := ir.NewSymbolLiteral("", ptr, base.Symbol)
	v.SymOffset = off
	return cv(v)
}

// constLvalAddr は評価済みの左辺値 c がグローバル変数・配列定数 (の定数の添字の要素・フィールド) なら、その変数と
// 先頭からのバイト数と型を返す。
func (h *Hlc) constLvalAddr(c *cexpr) (base *ir.Value, off int, t *types.Type, ok bool) {
	switch {
	case c.kind == cValue:
		v := c.val
		if v.Kind != ir.KindGlobal || v.Symbol == "" || v.Type.Kind == types.Soa || h.prog.storageAliases[v] != nil {
			return nil, 0, nil, false
		}
		return v, 0, v.Type, true
	case c.kind == cOp && c.op == opIndex:
		base, off, t, ok = h.constLvalAddr(c.args[0])
		if !ok || t.Kind != types.Array || !c.args[1].isLiteralInt() {
			return nil, 0, nil, false
		}
		i := c.args[1].val.Int
		if i < 0 || t.Length >= 0 && i >= t.Length {
			panic(&diag.Error{Msg: fmt.Sprintf("index %d is out of the array (length %d)", i, t.Length)})
		}
		return base, off + i*t.Base.Size, t.Base, true
	case c.kind == cOp && c.op == opField:
		base, off, t, ok = h.constLvalAddr(c.args[0])
		if !ok || t.Kind != types.Struct || t.IsSlice() {
			return nil, 0, nil, false
		}
		fd, found := t.Field(c.name)
		if !found {
			return nil, 0, nil, false
		}
		return base, off + fd.Offset, fd.Type, true
	}
	return nil, 0, nil, false
}
