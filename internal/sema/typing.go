package sema

// 式の型を IR を出す前に決める段 (Agent/wiki/plans/roadmap.md の構造の整理「sema の式に型付きの中間表現を」の最初の一歩)。
//
// 今の sema は lval が型の検査・暗黙の変換・診断・IR の出力を同時にするので、式の型は IR を出すまで分からない。そのせいで、
// 型を知るために一度評価した値を捨てて評価し直す (enum の `f() == .A` の二重評価) や、定数の経路と実行時の経路で検査が
// 食い違う (`+%` の型のない定数の検査: wrap.go) が起きた。ここでは IR を出さずに式の型を決める exprType を作り、対象を
// 広げながら lval の経路を置き換えていく。
//
// 対象: 定数になる式 (constEval をそのまま使う) と、実行時の二項演算 (算術・ビット演算・シフト・比較・論理演算・`+%`)、
// 単項の `- ~ !`、添字・参照はがし・アドレス・struct と soa のフィールド、@len・@min / @max / @clamp、変数・キャスト・
// 呼び出しの葉。それ以外 (slice の範囲など) は分からない (ok = false。使う側は評価してから判定する)。
//
// 使っている所: A1 の幅を上から決める (widen.go の wideWidth / compareWidth)、代入のような変換 (代入・初期化・引数・return・
// struct のフィールド) の型の照合と E・D (rvalAssign / assignPre / preConvert)、enum の短い名前の比較 (resolveEnumShortPair)、
// `+%` (wrapExpr)。
//
// 正しさの物差し: FC_VERIFY_IR (テストと fuzz では常に有効) のとき、lval が式を評価するたびに exprType と実際に出した値の型を
// 比べ (checkExprType。A1 で広い幅で計算した値は広げる前の型と比べる)、代入の照合は評価した値の型でも同じ結論かを確かめる
// (assignPost)。違えばコンパイラの内部エラー。

import (
	"fmt"

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/types"
)

// exprInfo は IR を出す前に分かった式の型。untyped は型のない整数定数 (n はその値)。
type exprInfo struct {
	t       *types.Type
	untyped bool
	n       int
}

// exprType は式 c の型を、IR を出さずに決める (分からなければ ok = false)。診断の panic は外に出さない。
func (h *Hlc) exprType(c *cexpr) (info exprInfo, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			if _, isDiag := r.(*diag.Error); !isDiag {
				panic(r)
			}
			info, ok = exprInfo{}, false
		}
	}()
	return h.exprType0(c)
}

func (h *Hlc) exprType0(c *cexpr) (exprInfo, bool) {
	e := h.constEval(c)
	switch e.kind {
	case cValue:
		v := e.val
		if v.Type == nil || v.Type.Kind == types.Bad || v.Type.Kind == types.TypeName || v.Type.Kind == types.Macro {
			return exprInfo{}, false
		}
		if v.Kind == ir.KindLiteral && v.IsInt && v.Untyped {
			return exprInfo{t: v.Type, untyped: true, n: v.Int}, true
		}
		return exprInfo{t: v.Type}, true // ストレージの別名も宣言の型 (lval は元の場所をその型で読む)、soa は入れ物の型
	case cCast:
		ty := e.ty
		if ty == nil {
			ty = h.typeEval(e.typ)
		}
		return exprInfo{t: ty}, ty != nil
	case cOp:
		return h.opType(e)
	case cOperand:
		// 先に評価した値 (evalOnce・wrapExpr が型を決められない項を評価したもの)
		if v, ok := e.opnd.(*ir.Value); ok && v.Kind == ir.KindLiteral && v.IsInt && v.Untyped {
			return exprInfo{t: v.Type, untyped: true, n: v.Int}, true
		}
		return exprInfo{t: ir.ValType(e.opnd)}, e.opnd != nil
	}
	return exprInfo{}, false
}

// opType は実行時の演算 e の型。
func (h *Hlc) opType(e *cexpr) (exprInfo, bool) {
	u := h.prog.Types
	switch e.op {
	case opEq, opNe, opLt, opGt, opLe, opGe, opLand, opLor, opNot:
		return exprInfo{t: u.Bool()}, true
	case opUminus, opBitNot:
		// 項が型のない定数でもここに来るのは畳まれない形 (`~@max(-124, 57)`: constEval はマクロを展開しない) で、lval は定数の
		// 型の実行時の命令を出す
		a, ok := h.exprType(e.args[0])
		if !ok || a.t.Kind != types.Int {
			return exprInfo{}, false
		}
		return exprInfo{t: a.t}, true
	case opAddWrap, opSubWrap, opMulWrap:
		a, ok := h.exprType(e.args[0])
		if !ok || a.untyped {
			return exprInfo{}, false
		}
		return exprInfo{t: a.t}, true
	case opCall:
		if fn := h.constEval(e.args[0]); fn.kind == cValue && fn.val.Type.Kind == types.Macro {
			// 組み込みの min / max / clamp (マクロが opMin などの節点にする)
			if op, ok := minMaxOps[fn.val.Name]; ok {
				// lval と同じく opMin などの節点にしてから (引数が全部定数なら constEval が畳む)
				return h.exprType(&cexpr{kind: cOp, op: op, args: e.args[1:]})
			}
			if fn.val.Name == "@ptr" && len(e.args) == 2 {
				// @ptr(s): 配列・slice の先頭へのポインタ
				if a, ok := h.exprType(e.args[1]); ok {
					switch {
					case a.t.IsSlice():
						return exprInfo{t: u.PointerTo(a.t.SliceOf)}, true
					case a.t.Kind == types.Array && !a.t.IsSoa:
						return exprInfo{t: u.PointerTo(a.t.Base)}, true
					}
				}
			}
			return exprInfo{}, false
		}
		f, ok := h.exprType(e.args[0])
		if !ok || f.t.Kind != types.Func || f.t.Base == nil || f.t.Base.Kind == types.Void {
			return exprInfo{}, false
		}
		return exprInfo{t: f.t.Base}, true
	case opMin, opMax, opClamp:
		return h.minMaxType(e.op, e.args)
	case opAdd, opSub, opMul, opDiv, opMod, opAnd, opOr, opXor, opShiftLeft, opShiftRight:
		a, ok := h.exprType(e.args[0])
		if !ok {
			return exprInfo{}, false
		}
		b, ok := h.exprType(e.args[1])
		if !ok {
			return exprInfo{}, false
		}
		return h.binaryType(e.op, a, b)
	case opLen:
		// 実行時の @len は slice の長さの型 (配列の @len は定数に畳まれる)
		if a, ok := h.exprType(e.args[0]); ok && a.t.IsSlice() {
			return exprInfo{t: a.t.SliceLen()}, true
		}
	case opIndex:
		// a[i]: 配列・ポインタは要素、slice は SliceOf、soa は要素のハンドル (soaIndex)
		a, ok := h.exprType(e.args[0])
		switch {
		case ok && a.t.IsSoa:
			return exprInfo{t: u.SoaRef(a.t, h.soaElement(a.t), "")}, true
		case !ok || a.untyped:
		case a.t.IsSlice():
			return exprInfo{t: a.t.SliceOf}, true
		case (a.t.Kind == types.Array || a.t.Kind == types.Pointer) && a.t.Base != nil && a.t.Base.Kind != types.Void:
			return exprInfo{t: a.t.Base}, true
		}
	case opDeref:
		if a, ok := h.exprType(e.args[0]); ok && a.t.Kind == types.Pointer && a.t.Base.Kind != types.Void {
			return exprInfo{t: a.t.Base}, true
		}
	case opRef:
		if a, ok := h.exprType(e.args[0]); ok && !a.untyped && a.t.Kind != types.SoaRef {
			return exprInfo{t: u.PointerTo(a.t)}, true
		}
	case opField:
		// a.f: struct の値か struct へのポインタ (自動で参照はがし)。soa のフィールドは扱わない
		a, ok := h.exprType(e.args[0])
		if !ok || a.untyped {
			break
		}
		t := a.t
		if t.Kind == types.SoaRef {
			// soa の要素のフィールド: 入れ子の struct はハンドル、ほかはフィールドの型 (soaField)
			if f, ok := t.Base.Field(e.name); ok {
				if f.Type.Kind == types.Struct {
					return exprInfo{t: u.SoaRef(t.Soa, f.Type, t.Path+e.name+"_")}, true
				}
				return exprInfo{t: f.Type}, true
			}
			break
		}
		if t.Kind == types.Pointer {
			t = t.Base
		}
		if t.Kind == types.Struct && !t.IsSlice() {
			if f, ok := t.Field(e.name); ok {
				return exprInfo{t: f.Type}, true
			}
		}
	}
	return exprInfo{}, false
}

// binaryType は実行時の二項演算の型 (lval の算術の経路: F1 のシフト、adaptLiteral、Compatible、ポインタの加減算)。
func (h *Hlc) binaryType(op cop, a, b exprInfo) (exprInfo, bool) {
	isInt := func(t *types.Type) bool { return t.Kind == types.Int && t.Enum == nil }
	if (op == opShiftLeft || op == opShiftRight) && h.v4() && isInt(a.t) && (b.t.Kind == types.Int || b.t.Kind == types.Bool) {
		return exprInfo{t: a.t}, true // F1: シフトの結果は左辺の型 (shiftByLeft)
	}
	at, bt := h.literalAdapted(a, b), h.literalAdapted(b, a)
	if op == opSub && at.Kind == types.Pointer && bt.Kind == types.Pointer && at.Base == bt.Base {
		return exprInfo{t: h.prog.Types.IntType(2, false)}, true // p - q は要素数 (pointerDiff)
	}
	if t := h.prog.Types.Compatible(at, bt); t != nil {
		return exprInfo{t: t}, true
	}
	if (op == opAdd || op == opSub) && (at.Kind == types.Pointer || at.Kind == types.SoaRef) && bt.Kind == types.Int {
		return exprInfo{t: at}, true // p + i
	}
	return exprInfo{}, false
}

// literalAdapted は二項演算の片方 x の、相手 other に合わせた後の型 (adaptLiteral と同じ判断 literalRule)。
func (h *Hlc) literalAdapted(x, other exprInfo) *types.Type {
	return h.literalType(x, other, false)
}

// literalType は literalRule の判断から、型のない定数 x の合わせた後の型 (型付きならそのまま)。cmp は比較。
func (h *Hlc) literalType(x, other exprInfo, cmp bool) *types.Type {
	if !x.untyped {
		return x.t
	}
	switch literalRule(x.t, x.n, other.untyped, other.t, cmp) {
	case litFits, litTruncate, litCmpError:
		return other.t
	case litSigned16:
		return h.prog.Types.IntType(2, true)
	}
	return x.t
}

// checkExprType は lval が式 c を評価した値 v の型が exprType と同じかを確かめる (FC_VERIFY_IR のときだけ)。lv は v が左辺値
// (その場所を指すポインタ) か (値の型はポインタの先)。
func (h *Hlc) checkExprType(c *cexpr, v ir.Operand, lv bool) {
	if v == nil || !h.prog.Config.VerifyIR() || c.kind != cOp {
		return
	}
	switch c.op {
	case opAdd, opSub, opMul, opDiv, opMod, opAnd, opOr, opXor, opShiftLeft, opShiftRight,
		opAddWrap, opSubWrap, opMulWrap, opUminus, opBitNot, opIndex, opDeref, opRef, opField,
		opMin, opMax, opClamp, opLen:
	default:
		return
	}
	info, ok := h.exprType(c)
	if !ok {
		return
	}
	got := ir.ValType(v)
	if lv {
		if got.Kind != types.Pointer {
			return // soa のハンドルなど
		}
		got = got.Base
	}
	if info.untyped {
		if lv, isLit := v.(*ir.Value); isLit && lv.Kind == ir.KindLiteral && lv.Untyped {
			return
		}
	}
	if got.Kind == types.Int && info.t.Kind == types.Int && got.Signed == info.t.Signed && got.Size > info.t.Size {
		return // A1 で広い幅で計算した (上から決めた幅。widen.go)
	}
	if got != info.t {
		panic(&diag.Error{Msg: fmt.Sprintf("internal: exprType of %s is %s but lval made %s (sema/typing.go)", h.exprText(c), info.t, got)})
	}
}

var minMaxOps = map[string]cop{"min": opMin, "max": opMax, "clamp": opClamp}

// minMaxType は min / max / clamp の型 (lval と同じ順: 最初の引数を各引数と比較の規則で合わせ、全部の互換型)。
func (h *Hlc) minMaxType(op cop, args []*cexpr) (exprInfo, bool) {
	if len(args) < 2 {
		return exprInfo{}, false
	}
	infos := make([]exprInfo, len(args))
	for i, a := range args {
		info, ok := h.exprType(a)
		if !ok {
			return exprInfo{}, false
		}
		infos[i] = info
	}
	v0 := infos[0]
	adapted := make([]*types.Type, len(infos))
	for i := 1; i < len(infos); i++ {
		a0, ai := h.literalAdaptedCmp(v0, infos[i]), h.literalAdaptedCmp(infos[i], v0)
		if v0.untyped && a0 != v0.t {
			v0 = exprInfo{t: a0} // 相手の型に合わせた定数 (以後は型付き)
		}
		adapted[i] = ai
	}
	t := v0.t
	for _, it := range adapted[1:] {
		if t = h.prog.Types.Compatible(t, it); t == nil {
			return exprInfo{}, false
		}
	}
	if t.Kind != types.Int && t.Kind != types.Bool {
		return exprInfo{}, false
	}
	return exprInfo{t: t}, true
}

// literalAdaptedCmp は比較の規則の literalAdapted (adaptLiteral の cmp)。
func (h *Hlc) literalAdaptedCmp(x, other exprInfo) *types.Type {
	return h.literalType(x, other, true)
}

// preConvert は式 c を代入のような変換で to にするときの E・D の判定を、評価する前に型を決める段の情報でする (診断は IR を
// 出す前に出る)。判定できたら true (評価のあとの convertValue は判定しない)。式の型は A1 の広げる前の型: 代入先が広ければ代入先の
// 幅で計算するので縮小にならず、狭ければ式の型から縮小になる。定数は値で (折り返した型付きの定数は代入先の幅で畳み込み直した値)。
func (h *Hlc) preConvert(c *cexpr, to *types.Type) bool {
	if c == nil || to == nil {
		return false
	}
	info, ok := h.exprType(c)
	if !ok {
		return false
	}
	from, k, isConst := info.t, info.n, info.untyped
	if e := h.constEval(c); !isConst && e.isLiteralInt() {
		k, isConst = e.val.Int, true
		if tc := h.taint[e.val]; tc != nil && h.v4() && to.Kind == types.Int && to.Size > e.val.Type.Size {
			if r := h.foldAtWidth(tc, to.Size); r != nil {
				k = wrapInt(r.val.Int, to) // widenArith と同じ値
			}
		}
	} else if isConst && !(e.kind == cValue && e.val.Kind == ir.KindLiteral) {
		return false // 畳まれない型のない定数の式 (マクロ): 評価してから判定する
	}
	h.convReport(convRule(from, k, isConst, to), from, k, to, c)
	return true
}

// rvalAssign は c を to への代入のように評価する (引数・return・struct と soa のフィールド。what は診断の主語)。型の照合
// (assignPre) と E・D (preConvert) を評価の前に型を決める段の情報で判定し、to の幅で評価して (A1)、const を外す警告をし
// (warnDropConst)、to に変換した値を返す。check は評価した値を見るほかの検査 (return の局所変数のアドレス)。代入のような変換の
// 検査をここに集める (以前は呼び出し元ごとに compatibleAssign・warnDropConst を並べていて、struct のフィールドは文言の違う
// compatible だけで const を外す警告が無かった)。代入と変数の初期化は左辺・宣言の事情があるので assignPre / assignPost を直接使う。
func (h *Hlc) rvalAssign(c *cexpr, to *types.Type, what string, check func(v ir.Operand)) ir.Operand {
	pre := h.assignPre(what, c, to)
	var v ir.Operand
	checked := false
	if to != nil && to.Kind == types.Array && !to.IsSoa {
		v = h.arrayValue(c) // `x = a[1]` (2 次元配列の行): 行の写し
	}
	if v == nil {
		v, checked = h.rvalPreConv(c, to, true)
	}
	h.assignPost(what, to, v, pre)
	if check != nil {
		check(v)
	}
	h.warnDropConst(what, to, v)
	return h.convertValue(v, to, c, checked)
}

// assignPre は c を to に代入できるか (compatibleAssign) を、評価の前に型を決める段の型で判定する。判定できたら true
// (assignPost は判定し直さない)。診断は IR を出す前に出る。
func (h *Hlc) assignPre(what string, c *cexpr, to *types.Type) bool {
	if c == nil || to == nil {
		return false
	}
	info, ok := h.exprType(c)
	if !ok || info.t.Kind == types.SoaRef {
		return false // soa の要素は型を決める段ではハンドル (値として読むと struct): 評価してから
	}
	h.compatibleAssign(what, to, info.t)
	return true
}

// assignPost は評価した値 v で代入の型の照合をする (評価の前に判定できなかったとき)。判定済み (pre) なら、FC_VERIFY_IR の
// ときに評価した値の型でも同じ結論か (代入できるか) を確かめる。
func (h *Hlc) assignPost(what string, to *types.Type, v ir.Operand, pre bool) {
	if !pre {
		h.compatibleAssign(what, to, ir.ValType(v))
		return
	}
	if h.prog.Config.VerifyIR() && h.prog.Types.Compatible(to, ir.ValType(v)) == nil {
		panic(&diag.Error{Msg: fmt.Sprintf("internal: %s: the typing stage accepted the value but %s is not assignable to %s (sema/typing.go)", what, ir.ValType(v), to)})
	}
}

// rvalPreConv は c を to の幅で評価し (rvalWide)、pre なら E・D をその前に判定する (preConvert。判定したか)。判定で足した
// migrate の書き換え (`c as T`) は、評価の中と A1 (widenArith) で足した書き換えの後に並べる: 同じ位置に入れる `as` は足した順に
// 当たるので、外側の変換が先だと `x as u16 as i8` (fc 3 の 8 ビットの計算を保つ `as i8` が外になる) になっていた
// (fuzz の TestRandomMigrate)。
func (h *Hlc) rvalPreConv(c *cexpr, to *types.Type, pre bool) (ir.Operand, bool) {
	n := len(h.prog.Rewrites)
	checked := pre && h.preConvert(c, to)
	held := append([]Rewrite(nil), h.prog.Rewrites[n:]...)
	h.prog.Rewrites = h.prog.Rewrites[:n]
	v := h.rvalWide(c, to)
	if h.rewriting() && to != nil {
		h.widenArith(v, to) // 8 ビットの計算を保つ `as i8` (convertValue でも呼ぶが、同じ書き換えは 1 つにまとまる) を外側の変換より先に
	}
	h.prog.Rewrites = append(h.prog.Rewrites, held...)
	return v, checked
}
