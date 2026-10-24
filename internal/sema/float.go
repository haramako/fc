package sema

// 浮動小数 (f16 / f32 = IEEE 754 binary16 / binary32) の規則・定数の畳み込み・実行時の下ろし方 (Agent/wiki/design/float-fixed.md)。
//
// 値はビットの並び (定数は ir.Value.Int に符号なしのビット)。演算・比較・変換は fclib/float.fc の関数を呼ぶ (引数と戻り値は同じ大きさの
// 符号なしの整数として渡す)。f16 は f32 に広げて計算して丸め直す。定数の畳み込みは Go の float32 (f16 は丸めを自前で)。NaN を作る
// 演算の結果は 0x7fc00000 (f16 は 0x7e00) に揃える (fclib/float.fc と同じ)。

import (
	"fmt"
	"math"
	"math/big"

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/syntax"
	"github.com/haramako/fc/internal/types"
)

const (
	nan32 = 0x7fc00000
	nan16 = 0x7e00
)

// arithFloat は浮動小数の二項の算術 (+ - * /。arithKind の続き)。
const arithFloat arithKind = 200

// bitsType は浮動小数 t と同じ大きさの符号なしの整数の型 (ビットの並び)。
func (h *Hlc) bitsType(t *types.Type) *types.Type { return h.prog.Types.IntType(t.Size, false) }

// asBits は浮動小数の値 v をビットの並び (符号なしの整数) として読む。
func (h *Hlc) asBits(v ir.Operand) ir.Operand {
	bt := h.bitsType(ir.ValType(v))
	if k, ok := ir.ValIntLiteral(v); ok {
		return ir.NewIntLiteral("", bt, k)
	}
	return ir.NewCastedValue(v, bt, 0)
}

// floatCall は fclib/float.fc の関数 name を呼んだ値 (args は評価済みの値)。
func (h *Hlc) floatCall(name string, args ...ir.Operand) ir.Operand {
	fn := h.moduleFunc(h.builtinModule("float"), name)
	cs := make([]*cexpr, len(args))
	for i, a := range args {
		cs[i] = &cexpr{kind: cOperand, opnd: a}
	}
	return h.rval(ccall(cv(fn), cs...))
}

// to32 は浮動小数の値 v (f16 / f32) の f32 のビット (u32)。
func (h *Hlc) to32(v ir.Operand) ir.Operand {
	if ir.ValType(v).Size == 2 {
		return h.floatCall("from16", h.asBits(v))
	}
	return h.asBits(v)
}

// from32 は f32 のビット b を浮動小数の型 t (f16 / f32) の値にする。
func (h *Hlc) from32(b ir.Operand, t *types.Type) ir.Operand {
	if t.Size == 2 {
		b = h.floatCall("to16", b)
	}
	return ir.NewCastedValue(b, t, 0)
}

// ---------------------------------------------------------------
// Go の側の値 (定数の畳み込み)
// ---------------------------------------------------------------

// f32Of は浮動小数の定数 (型 t、ビット bits) の値 (f16 は f32 に正確に広げる)。
func f32Of(bits int, t *types.Type) float32 {
	if t.Size == 2 {
		return f16ToF32(uint16(bits))
	}
	return math.Float32frombits(uint32(bits))
}

// floatBits は f32 の値 f を浮動小数の型 t のビットにする (f16 は最近接偶数に丸める。NaN は揃える)。
func floatBits(f float32, t *types.Type) int {
	if f != f {
		if t.Size == 2 {
			return nan16
		}
		return nan32
	}
	if t.Size == 2 {
		return int(f32ToF16(f))
	}
	return int(math.Float32bits(f))
}

// f16ToF32 は f16 のビットの値 (正確)。
func f16ToF32(h uint16) float32 {
	neg := h&0x8000 != 0
	e := int(h>>10) & 31
	m := float64(h & 0x3ff)
	var v float64
	switch e {
	case 31:
		if m != 0 {
			return float32(math.NaN())
		}
		v = math.Inf(1)
	case 0:
		v = math.Ldexp(m, -24)
	default:
		v = math.Ldexp(1024+m, e-25)
	}
	if neg {
		v = math.Copysign(v, -1)
	}
	return float32(v)
}

// f32ToF16 は f32 → f16 (最近接偶数。fclib/float.fc の to16 と同じ)。
func f32ToF16(f float32) uint16 {
	if f != f {
		return nan16
	}
	sign := uint16(0)
	if math.Signbit(float64(f)) {
		sign = 0x8000
	}
	if math.IsInf(float64(f), 0) {
		return sign | 0x7c00
	}
	r := new(big.Rat).SetFloat64(math.Abs(float64(f)))
	return sign | uint16(ratToFloatBitsAbs(r, 5, 10))
}

// ratToFloatBitsAbs は 0 以上の正確な値 r を、指数 eb ビット・仮数 mb ビットの IEEE の形式の最近接 (ちょうど中間なら偶数) の
// ビット (符号を除く) にする。大きすぎれば無限大。
func ratToFloatBitsAbs(r *big.Rat, eb, mb int) uint64 {
	if r.Sign() == 0 {
		return 0
	}
	bias := 1<<(eb-1) - 1
	emax := 1<<eb - 1
	n, d := r.Num(), r.Denom()
	// 2^e <= r < 2^(e+1) の e
	e := n.BitLen() - d.BitLen()
	if new(big.Int).Lsh(d, uint(max(e, 0))).Cmp(new(big.Int).Lsh(n, uint(max(-e, 0)))) > 0 {
		e--
	}
	// 仮数の最下位のビットの重み 2^q (正規化数なら e - mb、非正規化数は 1 - bias - mb で固定)
	q := max(e, 1-bias) - mb
	num, den := new(big.Int).Set(n), new(big.Int).Set(d)
	if q >= 0 {
		den.Lsh(den, uint(q))
	} else {
		num.Lsh(num, uint(-q))
	}
	m := roundHalfEven(new(big.Rat).SetFrac(num, den))
	if m.BitLen() > mb+1 { // 丸めで桁が上がった
		m.Rsh(m, 1)
		q++
	}
	biased := q + mb + bias
	if m.BitLen() <= mb { // 非正規化数 (指数のビットは 0)
		biased = 0
	}
	if biased >= emax {
		return uint64(emax) << uint(mb)
	}
	frac := new(big.Int).And(m, new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), uint(mb)), big.NewInt(1)))
	return uint64(biased)<<uint(mb) | frac.Uint64()
}

// ratToFloat は正確な値 r を浮動小数の型 t の最近接のビットにする。
func ratToFloat(r *big.Rat, t *types.Type) int {
	eb, mb := 8, 23
	if t.Size == 2 {
		eb, mb = 5, 10
	}
	sign := uint64(0)
	if r.Sign() < 0 {
		sign = 1 << uint(eb+mb)
	}
	return int(sign | ratToFloatBitsAbs(new(big.Rat).Abs(r), eb, mb))
}

// ---------------------------------------------------------------
// 項を合わせる・計画 (real.go から)
// ---------------------------------------------------------------

// adaptFloat は型のない定数 x を浮動小数の相手 other に合わせる (最近接に丸める。大きすぎる値は無限大)。
func (h *Hlc) adaptFloat(x, other exprInfo) (adaptedLit, bool) {
	bits := ratToFloat(infoRat(x), other.t)
	if inf := map[int]int{2: 0x7c00, 4: 0x7f800000}[other.t.Size]; bits&inf == inf {
		// 型のない定数は黙って値を変えない (丸めは許すが、範囲の外は無限大にしない。`x as f16` なら無限大)
		panic(&diag.Error{Msg: fmt.Sprintf("%s does not fit in %s (write `%s as %s` for infinity)", ratText(infoRat(x)), other.t, ratText(infoRat(x)), other.t)})
	}
	return adaptedLit{t: other.t, changed: true, n: bits}, true
}

// planFloat は浮動小数の二項の算術の計画 (同じ型どうしか型のない定数。+ - * / だけ)。
func (h *Hlc) planFloat(op cop, a, b exprInfo) (arithPlan, bool) {
	ft := a.t
	if !ft.IsFloat() {
		ft = b.t
	}
	for _, x := range []exprInfo{a, b} {
		if !x.untyped && x.t != ft {
			panic(&diag.Error{Msg: fmt.Sprintf("cannot apply %s to %s and %s (a floating-point value takes the same type; convert with `as`)", opSymbol(op), a.t, b.t)})
		}
	}
	switch op {
	case opAdd, opSub, opMul, opDiv:
	default:
		panic(&diag.Error{Msg: fmt.Sprintf("%s is not defined on %s", opSymbol(op), ft)})
	}
	p := arithPlan{kind: arithFloat, t: ft}
	p.args[0], p.args[1] = h.adaptLit(a, b, false), h.adaptLit(b, a, false)
	return p, true
}

// ---------------------------------------------------------------
// 実行時の下ろし方
// ---------------------------------------------------------------

// emitFloatArith は浮動小数の二項の算術の値。
func (h *Hlc) emitFloatArith(op cop, p arithPlan, left, right ir.Operand) ir.Operand {
	left, right = p.args[0].apply(left), p.args[1].apply(right)
	name := map[cop]string{opAdd: "add32", opSub: "sub32", opMul: "mul32", opDiv: "div32"}[op]
	return h.from32(h.floatCall(name, h.to32(left), h.to32(right)), p.t)
}

// floatUnary は浮動小数の単項の `-` (符号のビットを反転する)。
func (h *Hlc) floatUnary(op cop, v ir.Operand) ir.Operand {
	t := ir.ValType(v)
	if op != opUminus {
		panic(&diag.Error{Msg: fmt.Sprintf("%s is not defined on %s", opSymbol(op), t)})
	}
	bt := h.bitsType(t)
	tmp := h.newTmp(bt)
	h.emit(&ir.Op{Code: ir.OpXor, Dst: tmp, Src: []ir.Operand{h.asBits(v), ir.NewIntLiteral("", bt, 1<<(8*t.Size-1))}})
	return ir.NewCastedValue(tmp, t, 0)
}

// emitFloatCompare は浮動小数の比較 (op は eq / lt / le。IEEE: NaN とはどれも偽、+0 と -0 は等しい) の bool。
func (h *Hlc) emitFloatCompare(op cop, left, right ir.Operand) ir.Operand {
	name := map[cop]string{opEq: "eq32", opLt: "lt32", opLe: "le32"}[op]
	return h.floatCall(name, h.to32(left), h.to32(right))
}

// isFloatCompare は比較の項 a, b のどちらかが浮動小数か (型を決める段で分かるとき)。`<=` は `!(b < a)` に直せない (NaN)。
func (h *Hlc) isFloatCompare(a, b *cexpr) bool {
	for _, c := range []*cexpr{a, b} {
		if info, ok := h.exprType(c); ok && info.t.IsFloat() {
			return true
		}
	}
	return false
}

// floatLe は浮動小数の `a <= b` (ge なら `b <= a`) の bool。
func (h *Hlc) floatLe(op cop, a, b *cexpr) ir.Operand {
	if op == opGe {
		a, b = b, a
	}
	left := h.rval(a)
	right := h.rval(h.withExpected(b, ir.ValType(left)))
	li, ri := valueInfo(left), valueInfo(right)
	p := h.planCompare(opLt, li, ri, li, ri)
	left, right = p.args[0].apply(left), p.args[1].apply(right)
	if lt, rt := ir.ValType(left), ir.ValType(right); lt != rt {
		h.compatible(lt, rt)
	}
	return h.emitFloatCompare(opLe, left, right)
}

// floatCast は浮動小数が関わる実行時の `as` (整数 ↔ 浮動小数 ↔ 固定小数、f16 ↔ f32)。
func (h *Hlc) floatCast(v ir.Operand, to *types.Type) ir.Operand {
	from := ir.ValType(v)
	u := h.prog.Types
	switch {
	case from.IsFloat() && to.IsFloat():
		return h.from32(h.to32(v), to)
	case to.IsFloat():
		// 整数 / 固定小数 → 浮動小数: 中身の整数を f32 にしてから (固定小数は 2^-Frac 倍)
		x := v
		frac := 0
		if from.IsFixed() {
			x, frac = h.asRaw(v), from.Frac
		}
		w := u.IntType(4, from.Signed)
		var f ir.Operand
		if from.Signed {
			f = h.floatCall("from_i32", h.explicitCast(syntax.CastAs, x, w))
		} else {
			f = h.floatCall("from_u32", h.explicitCast(syntax.CastAs, x, w))
		}
		if frac != 0 {
			f = h.floatCall("ldexp32", f, ir.NewIntLiteral("", u.IntType(2, true), -frac))
		}
		return h.from32(f, to)
	}
	// 浮動小数 → 整数 (0 へ切り捨て) / 固定小数 (2^小数のビット数 倍して最近接偶数)。範囲の外は端に飽和、NaN は 0。
	// float.fc の to_sat_i / to_sat_u が 1 回の呼び出しでする (呼ぶ所のコードを小さく)
	raw, frac, rnd := to, 0, 0
	if to.IsFixed() {
		raw, frac, rnd = h.fixedRaw(to), to.Frac, 1
	}
	fn := "to_sat_u"
	if raw.Signed {
		fn = "to_sat_i"
	}
	n := h.floatCall(fn, h.to32(v), ir.NewIntLiteral("", u.IntType(2, true), frac),
		ir.NewIntLiteral("", u.Bool(), rnd), ir.NewIntLiteral("", u.IntType(1, false), raw.Size*8))
	if raw.Size < 4 {
		n = ir.NewCastedValue(n, raw, 0) // 飽和させてあるので下位のバイトが値
	}
	if to.IsFixed() {
		return ir.NewCastedValue(n, to, 0)
	}
	return n
}

// ---------------------------------------------------------------
// 定数
// ---------------------------------------------------------------

// foldFloat は浮動小数の定数の二項の演算 (vals の片方は浮動小数、もう片方は同じ型か型のない定数)。
func (h *Hlc) foldFloat(c *cexpr, vals []*ir.Value, infos []exprInfo) (*cexpr, bool) {
	u := h.prog.Types
	switch c.op {
	case opEq, opNe, opLt, opGt, opLe, opGe:
		p := h.planCompare(opLt, infos[0], infos[1], infos[0], infos[1])
		x, y := p.args[0].apply(vals[0]), p.args[1].apply(vals[1])
		if ir.ValType(x) != ir.ValType(y) {
			h.compatible(ir.ValType(x), ir.ValType(y))
		}
		a, _ := ir.ValIntLiteral(x)
		b, _ := ir.ValIntLiteral(y)
		fa, fb := f32Of(a, ir.ValType(x)), f32Of(b, ir.ValType(y))
		r := map[cop]bool{opEq: fa == fb, opNe: fa != fb, opLt: fa < fb, opGt: fa > fb, opLe: fa <= fb, opGe: fa >= fb}[c.op]
		return cv(ir.NewIntLiteral("", u.Bool(), b2i(r))), true
	case opLand, opLor:
		return nil, false
	}
	p := h.planArith(c.op, infos[0], infos[1])
	x, y := p.args[0].apply(vals[0]), p.args[1].apply(vals[1])
	a, _ := ir.ValIntLiteral(x)
	b, _ := ir.ValIntLiteral(y)
	fa, fb := f32Of(a, p.t), f32Of(b, p.t)
	var r float32
	switch c.op {
	case opAdd:
		r = fa + fb
	case opSub:
		r = fa - fb
	case opMul:
		r = fa * fb
	case opDiv:
		r = fa / fb
	}
	return cv(ir.NewIntLiteral("", p.t, floatBits(r, p.t))), true
}

// foldFloatUnary は浮動小数の定数の単項の演算 (`-` は符号のビットを反転する)。
func (h *Hlc) foldFloatUnary(op cop, v *ir.Value) (*cexpr, bool) {
	if op != opUminus {
		panic(&diag.Error{Msg: fmt.Sprintf("%s is not defined on %s", opSymbol(op), v.Type)})
	}
	return cv(ir.NewIntLiteral("", v.Type, v.Int^(1<<(8*v.Type.Size-1)))), true
}

// ratToFloatLit は正確な値 r を浮動小数 t の定数にする。
func (h *Hlc) ratToFloatLit(r *big.Rat, t *types.Type) *ir.Value {
	return ir.NewIntLiteral("", t, ratToFloat(r, t))
}

// constFloatCast は浮動小数の定数 v の `as` (実行時の floatCast と同じ結果)。
func (h *Hlc) constFloatCast(v *ir.Value, to *types.Type) (*ir.Value, bool) {
	f := f32Of(v.Int, v.Type)
	switch {
	case to.IsFloat():
		return ir.NewIntLiteral("", to, floatBits(f, to)), true
	case f != f:
		return ir.NewIntLiteral("", to, 0), true
	case to.IsFixed():
		// 実行時は f32 の ldexp (2^Frac 倍。桁あふれは無限大) の後に最近接偶数の整数
		s := float32(math.Ldexp(float64(f), to.Frac))
		return ir.NewIntLiteral("", to, satFloat(math.RoundToEven(float64(s)), h.fixedRaw(to))), true
	}
	return ir.NewIntLiteral("", to, satFloat(math.Trunc(float64(f)), to)), true
}

// satFloat は整数の値 x (float64、無限大も) を整数の型 t の範囲に飽和させる。
func satFloat(x float64, t *types.Type) int {
	lo, hi := intRange(t)
	switch {
	case x <= float64(lo):
		return lo
	case x >= float64(hi):
		return hi
	}
	return int(x)
}

// floatNotCond は浮動小数を条件に使ったときのエラー。
func floatNotCond(t *types.Type) *diag.Error {
	return &diag.Error{Msg: fmt.Sprintf("a value of type %s cannot be used as a condition (compare it, e.g. `x != 0.0`)", t)}
}

// ---------------------------------------------------------------
// 数学の組み込み (@sqrt / @floor / @ceil / @trunc / @round)
// ---------------------------------------------------------------

// floatMath は数学の組み込みの名前 → fclib/float.fc の関数と Go の計算 (定数の畳み込み)。
var floatMath = map[string]struct {
	fn string
	g  func(float64) float64
}{
	"@sqrt":  {"sqrt32", math.Sqrt},
	"@floor": {"floor32", math.Floor},
	"@ceil":  {"ceil32", math.Ceil},
	"@trunc": {"trunc32", math.Trunc},
	"@round": {"round32", math.Round},
}

// registerFloatBuiltins は浮動小数の数学の組み込みを登録する。引数は f16 / f32 (型のない実数の定数は f32)、結果は同じ型。sqrt は
// 正しく丸め、floor / ceil / trunc は向き、round は 0 から遠い向きの四捨五入 (どれも IEEE どおりで、NaN は NaN、符号付きの 0 を保つ)。
func registerFloatBuiltins(h *Hlc) {
	for name, m := range floatMath {
		name, m := name, m
		typing := macroTyping{typ: func(h *Hlc, args []*cexpr) (exprInfo, bool) {
			if len(args) != 1 {
				return exprInfo{}, false
			}
			a, ok := h.exprType(args[0])
			switch {
			case !ok:
				return exprInfo{}, false
			case a.t.IsFloat():
				return exprInfo{t: a.t}, true
			case a.untyped && a.r != nil:
				return exprInfo{t: h.prog.Types.FloatType(4)}, true
			}
			return exprInfo{}, false
		}}
		h.defmacroTyped(name, typing, func(h *Hlc, args []*cexpr, block *syntax.Block) macroResult {
			if len(args) != 1 {
				panic(&diag.Error{Msg: fmt.Sprintf("%s takes 1 argument (f16 or f32)", name)})
			}
			if !h.v4() {
				panic(&diag.Error{Msg: fmt.Sprintf("%s is fc 4 only", name)})
			}
			v := h.rval(args[0])
			if r := realOf(v); r != nil {
				v = h.ratToFloatLit(r, h.prog.Types.FloatType(4))
			}
			t := ir.ValType(v)
			if !t.IsFloat() {
				panic(&diag.Error{Msg: fmt.Sprintf("%s takes f16 or f32 (got %s; convert with `as f32`)", name, t)})
			}
			if k, ok := ir.ValIntLiteral(v); ok {
				f := f32Of(k, t)
				r := float32(m.g(float64(f))) // float64 で正確 (sqrt は 2 倍の精度から丸めても正しい)
				return macroResult{expr: cv(ir.NewIntLiteral("", t, floatBits(r, t)))}
			}
			return macroResult{expr: &cexpr{kind: cOperand, opnd: h.from32(h.floatCall(m.fn, h.to32(v)), t)}}
		})
	}
}
