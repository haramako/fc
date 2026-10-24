package sema

// 型のない実数の定数 (`1.5`) と固定小数 (`u8f8` など。Kind は Int で Frac が小数のビット数) の規則・定数の畳み込み・実行時の
// 下ろし方 (Agent/wiki/design/float-fixed.md)。
//
// 固定小数の中身は値 × 2^Frac の整数 (raw)。足し算・引き算・比較・シフト・余り・ビット演算は中身の整数の命令そのまま
// (同じ型どうし)。掛け算は「広げて掛けて Frac だけ算術右シフト」、割り算は「被除数を Frac だけ左にずらしてから割る」で、
// どちらも負の無限大へ切り捨て (算術シフト・床除算のまま)。1・2 バイトは 2 倍の幅の整数の命令に、3・4 バイトは 64 ビットの積・
// 被除数が要るので fclib/fixed.fc の関数に下ろす。整数の値 (変数も) との `*` `/` は中身の整数に掛ける・割るだけ。
// 定数の畳み込みは同じ計算を Go の整数でする (実行時と同じ結果)。

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/syntax"
	"github.com/haramako/fc/internal/types"
)

// ---------------------------------------------------------------
// 型の分類
// ---------------------------------------------------------------

// isRealish は整数の規則でなく実数の規則 (この file) で扱う型か (固定小数・浮動小数・型のない実数の定数)。
func isRealish(t *types.Type) bool { return t != nil && (t.IsFixed() || t.Kind == types.Float) }

// fixedRaw は固定小数 t の中身の整数の型 (同じ大きさと符号)。
func (h *Hlc) fixedRaw(t *types.Type) *types.Type { return h.prog.Types.IntType(t.Size, t.Signed) }

// asRaw は固定小数の値 v を中身の整数として読む (ビットはそのまま)。
func (h *Hlc) asRaw(v ir.Operand) ir.Operand {
	t := ir.ValType(v)
	raw := h.fixedRaw(t)
	if k, ok := ir.ValIntLiteral(v); ok {
		return ir.NewIntLiteral("", raw, k)
	}
	return ir.NewCastedValue(v, raw, 0)
}

// ---------------------------------------------------------------
// 型のない実数の定数
// ---------------------------------------------------------------

// realLit は実数のリテラルの綴り (`1.5` / `2e-3` / `1_000.5`) の正確な値。
func realLit(text string) *big.Rat {
	r, ok := new(big.Rat).SetString(strings.ReplaceAll(text, "_", ""))
	if !ok {
		panic(&diag.Error{Msg: fmt.Sprintf("invalid real literal %s", text)})
	}
	return r
}

// bigIntLit は整数のリテラルの綴り text が int64 に入らなければ、その 10 進の綴りと true (字句解析の値は折り返している)。
func bigIntLit(text string) (string, bool) {
	t := strings.ReplaceAll(text, "_", "")
	if len(t) > 0 && (t[0] < '0' || t[0] > '9') {
		return "", false // 文字のリテラル
	}
	base := 10
	if len(t) > 2 && t[0] == '0' && (t[1] == 'x' || t[1] == 'X' || t[1] == 'b' || t[1] == 'B') {
		base, t = map[byte]int{'x': 16, 'X': 16, 'b': 2, 'B': 2}[t[1]], t[2:]
	}
	n, ok := new(big.Int).SetString(t, base)
	if !ok || n.IsInt64() {
		return "", false
	}
	return n.String(), true
}

// RealValue は型のない実数の定数。
func (h *Hlc) RealValue(r *big.Rat) *ir.Value {
	return ir.NewRealLiteral("", h.prog.Types.UntypedReal(), r)
}

// realOf は v が型のない実数の定数ならその値。
func realOf(v ir.Operand) *big.Rat {
	if lv, ok := v.(*ir.Value); ok && lv.Kind == ir.KindLiteral && lv.Real != nil {
		return lv.Real
	}
	return nil
}

// infoRat は型のない定数 (整数か実数) の値。
func infoRat(x exprInfo) *big.Rat {
	if x.r != nil {
		return x.r
	}
	return new(big.Rat).SetInt64(int64(x.n))
}

// ratText は診断に出す型のない実数の定数の書き方。
func ratText(r *big.Rat) string {
	if r.IsInt() {
		return r.Num().String()
	}
	f, _ := r.Float64()
	return fmt.Sprint(f)
}

// roundHalfEven は r に最も近い整数 (ちょうど中間なら偶数)。
func roundHalfEven(r *big.Rat) *big.Int {
	q, m := new(big.Int).QuoRem(r.Num(), r.Denom(), new(big.Int)) // 0 へ切り捨てた商と余り
	twice := new(big.Int).Mul(new(big.Int).Abs(m), big.NewInt(2))
	switch c := twice.Cmp(r.Denom()); {
	case c > 0, c == 0 && q.Bit(0) == 1:
		if r.Sign() < 0 {
			q.Sub(q, big.NewInt(1))
		} else {
			q.Add(q, big.NewInt(1))
		}
	}
	return q
}

// truncRat は r を 0 の向きへ切り捨てた整数。
func truncRat(r *big.Rat) *big.Int { return new(big.Int).Quo(r.Num(), r.Denom()) }

// ratToFixed は r を固定小数 t の中身 (最近接の値、ちょうど中間なら偶数) にする。入らなければ ok = false。
func ratToFixed(r *big.Rat, t *types.Type) (raw int, ok bool) {
	x := roundHalfEven(new(big.Rat).Mul(r, new(big.Rat).SetInt(new(big.Int).Lsh(big.NewInt(1), uint(t.Frac)))))
	lo, hi := intRange(t)
	if !x.IsInt64() || x.Int64() < int64(lo) || x.Int64() > int64(hi) {
		return 0, false
	}
	return int(x.Int64()), true
}

// fixedRat は固定小数 t の中身 raw の正確な値。
func fixedRat(raw int, t *types.Type) *big.Rat {
	return new(big.Rat).SetFrac(big.NewInt(int64(raw)), new(big.Int).Lsh(big.NewInt(1), uint(t.Frac)))
}

// realNotInt は型のない実数の定数を整数の所に使ったときのエラー。
func realNotInt(r *big.Rat, t *types.Type) *diag.Error {
	if r.IsInt() && !r.Num().IsInt64() {
		return &diag.Error{Msg: fmt.Sprintf("%s does not fit in %s", r.Num(), t)} // int64 に入らない整数のリテラル
	}
	return &diag.Error{Msg: fmt.Sprintf("%s is not an integer, so it cannot be %s (write the type of the other side as a fixed-point or floating-point type, or convert with `as`)", ratText(r), t)}
}

// ---------------------------------------------------------------
// 項を合わせる・計画 (typeplan.go から)
// ---------------------------------------------------------------

// adaptNumeric は型のない定数 x (整数・実数) を相手 other に合わせる (adaptLit の実数の版)。handled が偽なら整数の規則のまま。
func (h *Hlc) adaptNumeric(x, other exprInfo, cmp bool) (a adaptedLit, handled bool) {
	if !x.untyped {
		return adaptedLit{}, false
	}
	switch {
	case other.untyped:
		if x.r != nil || other.r != nil {
			return adaptedLit{t: x.t}, true // 定数どうし (畳み込む)
		}
		return adaptedLit{}, false
	case other.t.IsFixed():
		raw, ok := ratToFixed(infoRat(x), other.t)
		if !ok {
			panic(&diag.Error{Msg: fmt.Sprintf("%s does not fit in %s", ratText(infoRat(x)), other.t)})
		}
		return adaptedLit{t: other.t, changed: true, n: raw}, true
	case other.t.IsFloat():
		return h.adaptFloat(x, other)
	case x.r != nil:
		panic(realNotInt(x.r, other.t))
	}
	return adaptedLit{}, false
}

// 固定小数の計算の仕方 (arithKind の続き)。
const (
	arithFixMul   arithKind = iota + 100 // 固定小数 × 固定小数 (同じ型): 広げて掛けて Frac だけ右へ
	arithFixDiv                          // 固定小数 ÷ 固定小数 (同じ型): Frac だけ左へずらしてから割る
	arithFixScale                        // 固定小数 × / ÷ 整数: 中身の整数に掛ける・割る (args の整数の側は中身の型の値)
)

// planNumeric は二項の算術の項に実数の規則の型 (固定小数・浮動小数・型のない実数の定数) があるときの計画 (planArith から)。
// handled が偽なら整数の規則のまま。
func (h *Hlc) planNumeric(op cop, a, b exprInfo) (p arithPlan, handled bool) {
	if !isRealish(a.t) && !isRealish(b.t) {
		return arithPlan{}, false
	}
	if a.untyped && b.untyped {
		return arithPlan{kind: arithCompat, t: h.prog.Types.UntypedReal(), args: [2]adaptedLit{{t: a.t}, {t: b.t}}}, true
	}
	if a.t.IsFloat() || b.t.IsFloat() {
		return h.planFloat(op, a, b)
	}
	if !a.t.IsFixed() && !b.t.IsFixed() {
		// 型のない実数の定数と整数
		x := a
		if !a.untyped {
			x = b
		}
		panic(realNotInt(x.r, map[bool]*types.Type{true: b.t, false: a.t}[a.untyped]))
	}
	// 固定小数 f と相手 o (i は f の位置)
	f, o, fi := a, b, 0
	if !a.t.IsFixed() {
		f, o, fi = b, a, 1
	}
	ft := f.t
	plan := func(kind arithKind, adapted adaptedLit) arithPlan {
		p := arithPlan{kind: kind, t: ft}
		p.args[fi] = adaptedLit{t: ft}
		p.args[1-fi] = adapted
		return p
	}
	switch op {
	case opShiftLeft, opShiftRight:
		if fi != 0 {
			panic(&diag.Error{Msg: fmt.Sprintf("the shift count must be an integer (got %s)", ft)})
		}
		return arithPlan{kind: arithShift, t: ft}, true
	case opMul, opDiv:
		if o.untyped && o.r == nil || isPlainInt(o.t) && !o.untyped {
			// 整数との掛け算・割り算: 中身の整数に (型のない整数の定数も。正確で速い)
			if op == opDiv && fi != 0 {
				panic(&diag.Error{Msg: fmt.Sprintf("cannot divide an integer by %s (convert the integer with `as %s`)", ft, ft)})
			}
			return plan(arithFixScale, adaptedLit{t: o.t}), true
		}
	}
	switch {
	case o.untyped && (op == opAnd || op == opOr || op == opXor):
		panic(&diag.Error{Msg: fmt.Sprintf("%s of %s and a constant is ambiguous (the bits of the integer or of the fixed-point value?); write `c as %s`", opSymbol(op), ft, ft)})
	case o.untyped:
		ad, _ := h.adaptNumeric(o, f, false)
		if op == opMul || op == opDiv {
			return plan(map[cop]arithKind{opMul: arithFixMul, opDiv: arithFixDiv}[op], ad), true
		}
		return plan(arithCompat, ad), true
	case o.t != ft:
		panic(&diag.Error{Msg: fmt.Sprintf("cannot apply %s to %s and %s (a fixed-point value takes the same type, or an integer only with * and /; convert with `as`)", opSymbol(op), a.t, b.t)})
	}
	switch op {
	case opMul:
		return plan(arithFixMul, adaptedLit{t: ft}), true
	case opDiv:
		return plan(arithFixDiv, adaptedLit{t: ft}), true
	}
	return plan(arithCompat, adaptedLit{t: ft}), true
}

// ---------------------------------------------------------------
// 実行時の下ろし方 (expr.go から)
// ---------------------------------------------------------------

// emitNumericArith は固定小数の掛け算・割り算・浮動小数の演算 (計画 p) の値。left / right は評価した項。
func (h *Hlc) emitNumericArith(op cop, p arithPlan, left, right ir.Operand) ir.Operand {
	if p.t.IsFloat() {
		return h.emitFloatArith(op, p, left, right)
	}
	ft := p.t
	raw := h.fixedRaw(ft)
	if p.kind == arithFixScale {
		f, n := left, right
		if !ir.ValType(left).IsFixed() {
			f, n = right, left
		}
		n = h.scaleOperand(n, raw, op)
		if k, lit := ir.ValIntLiteral(n); lit && k == 0 && op == opDiv {
			panic(&diag.Error{Msg: "div by 0"})
		}
		tmp := h.newTmp(ft)
		src := []ir.Operand{f, n}
		if f == right {
			src = []ir.Operand{n, f}
		}
		h.emit(&ir.Op{Code: copToOpCode[op], Dst: tmp, Src: src})
		return tmp
	}
	left, right = p.args[0].apply(left), p.args[1].apply(right)
	if k, lit := ir.ValIntLiteral(right); lit && k == 0 && p.kind == arithFixDiv {
		panic(&diag.Error{Msg: "div by 0"})
	}
	a, b := h.asRaw(left), h.asRaw(right)
	if ft.Size > 2 {
		// 3・4 バイト: fclib/fixed.fc の 64 ビットの積・被除数の関数 (4 バイトに広げて渡し、結果の下位を使う)
		w := h.prog.Types.IntType(4, ft.Signed)
		name := map[arithKind]string{arithFixMul: "mul_", arithFixDiv: "div_"}[p.kind] + map[bool]string{true: "i", false: "u"}[ft.Signed]
		fn := h.moduleFunc(h.builtinModule("fixed"), name)
		r := h.rval(ccall(cv(fn), &cexpr{kind: cOperand, opnd: h.cast(a, w)}, &cexpr{kind: cOperand, opnd: h.cast(b, w)}, cint(ft.Frac)))
		return ir.NewCastedValue(r, ft, 0)
	}
	w := h.prog.Types.IntType(2*ft.Size, ft.Signed)
	a, b = h.cast(a, w), h.cast(b, w)
	frac := ir.NewIntLiteral("", h.prog.Types.IntType(1, false), ft.Frac)
	if p.kind == arithFixMul {
		prod := h.newTmp(w)
		h.emit(&ir.Op{Code: ir.OpMul, Dst: prod, Src: []ir.Operand{a, b}})
		sh := h.newTmp(w)
		h.emit(&ir.Op{Code: ir.OpShiftRight, Dst: sh, Src: []ir.Operand{prod, frac}})
		return ir.NewCastedValue(sh, ft, 0)
	}
	sh := h.newTmp(w)
	h.emit(&ir.Op{Code: ir.OpShiftLeft, Dst: sh, Src: []ir.Operand{a, frac}})
	q := h.newTmp(w)
	h.emit(&ir.Op{Code: ir.OpDiv, Dst: q, Src: []ir.Operand{sh, b}})
	return ir.NewCastedValue(q, ft, 0)
}

// scaleOperand は固定小数に掛ける・割る整数の値 n を中身の型 raw の値にする (広い整数・符号の違う割る数はエラー)。
func (h *Hlc) scaleOperand(n ir.Operand, raw *types.Type, op cop) ir.Operand {
	if lv, ok := n.(*ir.Value); ok && lv.Kind == ir.KindLiteral && lv.IsInt && lv.Untyped {
		if lo, hi := intRange(raw); lv.Int < lo || lv.Int > hi {
			panic(&diag.Error{Msg: fmt.Sprintf("%d does not fit in %s, the integer type of the fixed-point value", lv.Int, raw)})
		}
		return ir.NewIntLiteral("", raw, lv.Int)
	}
	nt := ir.ValType(n)
	if nt.Size > raw.Size {
		panic(&diag.Error{Msg: fmt.Sprintf("cannot multiply or divide a %d-byte fixed-point value by %s (narrowing; write `x as %s`)", raw.Size, nt, raw)})
	}
	if op == opDiv && nt.Signed && !raw.Signed {
		panic(&diag.Error{Msg: fmt.Sprintf("cannot divide an unsigned fixed-point value by %s (signed; write `x as %s`)", nt, raw)})
	}
	return h.cast(n, raw)
}

// ---------------------------------------------------------------
// 定数の畳み込み (consteval.go から)
// ---------------------------------------------------------------

// foldNumeric は定数の項 args の演算 c を、実数の規則の型 (型のない実数の定数・固定小数・浮動小数) があれば畳む。handled が偽なら
// 整数の規則のまま。結果は実行時の下ろし方と同じ値。
func (h *Hlc) foldNumeric(c *cexpr, args []*cexpr) (r *cexpr, handled bool) {
	vals := make([]*ir.Value, len(args))
	any := false
	for i, a := range args {
		if a.kind != cValue || a.val.Kind != ir.KindLiteral || !a.val.IsInt && a.val.Real == nil {
			return nil, false
		}
		vals[i] = a.val
		any = any || a.val.Real != nil || isRealish(a.val.Type)
	}
	if !any {
		return nil, false
	}
	u := h.prog.Types
	infos := make([]exprInfo, len(vals))
	for i, v := range vals {
		infos[i] = valueInfo(v)
	}
	boolLit := func(b bool) (*cexpr, bool) { return cv(ir.NewIntLiteral("", u.Bool(), b2i(b))), true }
	if len(vals) == 1 && !infos[0].untyped {
		return h.foldUnaryTyped(c.op, vals[0]) // 型付きの固定小数・浮動小数の単項
	}
	// 型のない定数どうし (少なくとも 1 つは実数): 正確に
	if len(vals) == 1 || infos[0].untyped && infos[1].untyped {
		x := infoRat(infos[0])
		var y *big.Rat
		if len(vals) > 1 {
			y = infoRat(infos[1])
		}
		switch c.op {
		case opUminus:
			return cv(h.RealValue(new(big.Rat).Neg(x))), true
		case opAdd:
			return cv(h.RealValue(new(big.Rat).Add(x, y))), true
		case opSub:
			return cv(h.RealValue(new(big.Rat).Sub(x, y))), true
		case opMul:
			return cv(h.RealValue(new(big.Rat).Mul(x, y))), true
		case opDiv:
			if y.Sign() == 0 {
				panic(&diag.Error{Msg: "div by 0"})
			}
			return cv(h.RealValue(new(big.Rat).Quo(x, y))), true
		case opEq, opNe, opLt, opGt, opLe, opGe:
			if len(vals) == 1 {
				break
			}
			cmp := x.Cmp(y)
			return boolLit(map[cop]bool{opEq: cmp == 0, opNe: cmp != 0, opLt: cmp < 0, opGt: cmp > 0, opLe: cmp <= 0, opGe: cmp >= 0}[c.op])
		}
		panic(&diag.Error{Msg: fmt.Sprintf("%s is not defined on real constants", opSymbol(c.op))})
	}
	if vals[0].Type.IsFloat() || vals[1].Type.IsFloat() {
		return h.foldFloat(c, vals, infos)
	}
	// 固定小数
	switch c.op {
	case opEq, opNe, opLt, opGt, opLe, opGe:
		cp := h.planCompare(map[cop]cop{opEq: opEq, opNe: opEq, opLt: opLt, opGt: opLt, opLe: opLt, opGe: opLt}[c.op], infos[0], infos[1], infos[0], infos[1])
		x, y := cp.args[0].apply(vals[0]), cp.args[1].apply(vals[1])
		if ir.ValType(x) != ir.ValType(y) {
			h.compatible(ir.ValType(x), ir.ValType(y)) // 型の誤り (実行時と同じエラー)
		}
		a, _ := ir.ValIntLiteral(x)
		b, _ := ir.ValIntLiteral(y)
		return boolLit(map[cop]bool{opEq: a == b, opNe: a != b, opLt: a < b, opGt: a > b, opLe: a <= b, opGe: a >= b}[c.op])
	case opLand, opLor:
		return nil, false
	}
	p := h.planArith(c.op, infos[0], infos[1])
	ft := p.t
	x, y := p.args[0].apply(vals[0]), p.args[1].apply(vals[1])
	a, _ := ir.ValIntLiteral(x)
	b, _ := ir.ValIntLiteral(y)
	if p.kind == arithShift {
		b = vals[1].Int
	}
	res := new(big.Int)
	A, B := big.NewInt(int64(a)), big.NewInt(int64(b))
	switch p.kind {
	case arithFixScale:
		fv, nv := vals[0], vals[1]
		if !fv.Type.IsFixed() {
			fv, nv = vals[1], vals[0]
		}
		f := fv.Int
		n, _ := ir.ValIntLiteral(h.scaleOperand(nv, h.fixedRaw(ft), c.op))
		if c.op == opMul {
			res.Mul(big.NewInt(int64(f)), big.NewInt(int64(n)))
		} else {
			if n == 0 {
				panic(&diag.Error{Msg: "div by 0"})
			}
			res = floorDivBig(big.NewInt(int64(f)), big.NewInt(int64(n)))
		}
	case arithFixMul:
		res.Rsh(res.Mul(A, B), uint(ft.Frac)) // big.Int の Rsh は負の数も負の無限大へ (算術シフト)
	case arithFixDiv:
		if b == 0 {
			panic(&diag.Error{Msg: "div by 0"})
		}
		res = floorDivBig(new(big.Int).Lsh(A, uint(ft.Frac)), B)
	case arithShift:
		if c.op == opShiftLeft {
			res.Lsh(A, uint(b))
		} else {
			res.Rsh(A, uint(b))
		}
	default:
		switch c.op {
		case opAdd:
			res.Add(A, B)
		case opSub:
			res.Sub(A, B)
		case opMod:
			if b == 0 {
				panic(&diag.Error{Msg: "div by 0"})
			}
			res.Sub(A, new(big.Int).Mul(floorDivBig(A, B), B))
		case opAnd:
			res.And(A, B)
		case opOr:
			res.Or(A, B)
		case opXor:
			res.Xor(A, B)
		default:
			panic(&diag.Error{Msg: fmt.Sprintf("%s is not defined on %s", opSymbol(c.op), ft)})
		}
	}
	return cv(ir.NewIntLiteral("", ft, wrapBig(res, ft))), true
}

// foldUnaryTyped は固定小数の定数の単項の演算。
func (h *Hlc) foldUnaryTyped(op cop, v *ir.Value) (*cexpr, bool) {
	t := v.Type
	if t.IsFloat() {
		return h.foldFloatUnary(op, v)
	}
	switch op {
	case opUminus:
		return cv(ir.NewIntLiteral("", t, wrapInt(-v.Int, t))), true
	case opBitNot:
		return cv(ir.NewIntLiteral("", t, wrapInt(^v.Int, t))), true
	case opNot:
		return cv(ir.NewIntLiteral("", h.prog.Types.Bool(), b2i(v.Int == 0))), true
	}
	return nil, false
}

// floorDivBig は床除算 (商を負の無限大へ。b は 0 でない)。
func floorDivBig(a, b *big.Int) *big.Int {
	q, m := new(big.Int).QuoRem(a, b, new(big.Int))
	if m.Sign() != 0 && (m.Sign() < 0) != (b.Sign() < 0) {
		q.Sub(q, big.NewInt(1))
	}
	return q
}

// wrapBig は x を型 t (整数・固定小数の中身) の幅に折り返す。
func wrapBig(x *big.Int, t *types.Type) int {
	m := new(big.Int).Lsh(big.NewInt(1), uint(8*t.Size))
	r := new(big.Int).Mod(x, m) // 0〜m-1
	return wrapInt(int(r.Int64()), t)
}

// ---------------------------------------------------------------
// 変換
// ---------------------------------------------------------------

// convertNumeric は代入のような変換 (暗黙) で v を typ にする。実数の規則の型でなければ handled が偽。
func (h *Hlc) convertNumeric(v ir.Operand, typ *types.Type) (r ir.Operand, handled bool) {
	vt := ir.ValType(v)
	if !isRealish(typ) && !isRealish(vt) {
		return nil, false
	}
	if typ == vt {
		return v, true
	}
	if lv, ok := v.(*ir.Value); ok && lv.Kind == ir.KindLiteral && lv.Untyped && (lv.IsInt || lv.Real != nil) {
		// 型のない定数は相手に合わせる (入らなければエラー。黙って切り詰めない)
		ad, handled := h.adaptNumeric(valueInfo(lv), exprInfo{t: typ}, false)
		if handled && ad.changed {
			return ir.NewIntLiteral("", typ, ad.n), true
		}
		if lv.Real != nil {
			panic(realNotInt(lv.Real, typ))
		}
	}
	panic(&diag.Error{Msg: fmt.Sprintf("cannot convert %s to %s implicitly (write `x as %s`)", vt, typ, typ)})
}

// numericCastable は `as` で変換できる実数の規則の型の組か (整数 ↔ 固定小数 ↔ 浮動小数、型のない実数の定数から)。
func numericCastable(from, to *types.Type) bool {
	num := func(t *types.Type) bool { return t.Kind == types.Int && t.Enum == nil || t.Kind == types.Float }
	return (isRealish(from) || isRealish(to)) && num(from) && num(to) && !to.IsUntypedReal()
}

// constNumericCast は定数 v の `as` (実数の規則の型が関わるとき)。handled が偽なら整数の規則のまま。
func (h *Hlc) constNumericCast(v *ir.Value, to *types.Type) (*ir.Value, bool) {
	from := v.Type
	if !isRealish(from) && !isRealish(to) {
		return nil, false
	}
	if !numericCastable(from, to) {
		panic(&diag.Error{Msg: fmt.Sprintf("cannot convert %s to %s with `as`", from, to)})
	}
	var r *big.Rat // 正確な値 (浮動小数は段 3)
	switch {
	case v.Real != nil:
		r = v.Real
	case from.IsFloat():
		return h.constFloatCast(v, to)
	case from.IsFixed():
		r = fixedRat(v.Int, from)
	default:
		r = new(big.Rat).SetInt64(int64(v.Int))
	}
	switch {
	case to.IsFloat():
		return h.ratToFloatLit(r, to), true
	case to.IsFixed():
		if from.IsFixed() {
			// 固定小数どうし: 小数のビットを減らす向きは負の無限大へ切り捨て (算術シフト)
			x := new(big.Int).Lsh(big.NewInt(int64(v.Int)), uint(max(0, to.Frac-from.Frac)))
			x.Rsh(x, uint(max(0, from.Frac-to.Frac)))
			return ir.NewIntLiteral("", to, wrapBig(x, to)), true
		}
		x := roundHalfEven(new(big.Rat).Mul(r, new(big.Rat).SetInt(new(big.Int).Lsh(big.NewInt(1), uint(to.Frac)))))
		return ir.NewIntLiteral("", to, wrapBig(x, to)), true
	}
	// → 整数: 固定小数は負の無限大へ切り捨て、型のない実数は 0 へ切り捨て
	var x *big.Int
	if from.IsFixed() {
		x = new(big.Int).Rsh(big.NewInt(int64(v.Int)), uint(from.Frac))
	} else {
		x = truncRat(r)
	}
	return ir.NewIntLiteral("", to, wrapBig(x, to)), true
}

// numericCast は実行時の値 v の `as` (実数の規則の型が関わるとき)。handled が偽なら整数の規則のまま。
func (h *Hlc) numericCast(v ir.Operand, to *types.Type) (ir.Operand, bool) {
	from := ir.ValType(v)
	if !isRealish(from) && !isRealish(to) {
		return nil, false
	}
	if lv, ok := v.(*ir.Value); ok && lv.Kind == ir.KindLiteral && (lv.IsInt || lv.Real != nil) {
		if r, ok := h.constNumericCast(lv, to); ok {
			return r, true
		}
	}
	if !numericCastable(from, to) {
		panic(&diag.Error{Msg: fmt.Sprintf("cannot convert %s to %s with `as`", from, to)})
	}
	if from.IsFloat() || to.IsFloat() {
		return h.floatCast(v, to), true
	}
	u := h.prog.Types
	if from == to {
		return v, true
	}
	// 整数 / 固定小数どうし: 中身の整数を、両方が入る幅 w で小数のビットの差だけずらしてから to の大きさにする
	fromFrac, toFrac := from.Frac, to.Frac
	src := v
	if from.IsFixed() {
		src = h.asRaw(v)
	}
	w := u.IntType(max(from.Size, to.Size), from.Signed)
	x := h.cast(src, w)
	if d := toFrac - fromFrac; d > 0 {
		t := h.newTmp(w)
		h.emit(&ir.Op{Code: ir.OpShiftLeft, Dst: t, Src: []ir.Operand{x, ir.NewIntLiteral("", u.IntType(1, false), d)}})
		x = t
	} else if d < 0 {
		t := h.newTmp(w)
		h.emit(&ir.Op{Code: ir.OpShiftRight, Dst: t, Src: []ir.Operand{x, ir.NewIntLiteral("", u.IntType(1, false), -d)}})
		x = t
	}
	if !to.IsFixed() {
		return h.explicitCast(syntax.CastAs, x, to), true // w (to 以上の大きさ) から狭める
	}
	return ir.NewCastedValue(h.explicitCast(syntax.CastAs, x, h.fixedRaw(to)), to, 0), true
}

// ---------------------------------------------------------------
// 出力 (format.go から)
// ---------------------------------------------------------------

const (
	fixMaxFrac = 28 // 出力できる固定小数の小数のビット数の上限 (fmt.fixdec が小数部 × 10 を 32 ビットで計算する)
	fmtMaxPrec = 20 // `{:.N}` の N の上限 (printf の書き先 32 バイトに整数の 10 桁・小数点・符号と入る)
)

// formatFixed は固定小数の定数 (中身 raw) を書式 sp の文字 (ASCII) にする (ir.FormatFixed。fclib/fmt.fc の fixdec と同じ結果)。
func formatFixed(raw int, t *types.Type, sp ir.LogSpec) string {
	return ir.FormatFixed(raw, t.Frac, sp)
}

// untypedRealType は型を書かない変数の値になった型のない実数の定数 r の型 (`var x = 1.5` は f32)。
func (h *Hlc) untypedRealType(r *big.Rat) *types.Type { return h.prog.Types.FloatType(4) }
