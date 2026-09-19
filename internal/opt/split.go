package opt

import (
	"fmt"

	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/types"
)

// splitWords は 2 バイトのローカル変数 v を上位 / 下位の 1 バイト変数 v.lo / v.hi に分ける (doc/v2_regalloc.md §7)。
// 分けた後は 1 バイトの演算の並びになるので、既存の A / Y / X の常駐が半分ずつを扱え、バイトごとの定数畳み込みで
// `crc ^= x << 8` の下位の xor が消える。
//
// バイトに分解できる命令 (それ以外の使用は v のスロットを残して前後で写す = 実体化):
//
//	load v = 定数 / 2 バイトの変数 / 1 バイトの値 (ゼロ拡張)       load v.lo = ..; load v.hi = ..
//	load w = v (w は分けない 2 バイトの変数)                          load w.0 = v.lo; load w.1 = v.hi
//	xor / and / or                                                     バイトごと
//	shift_left v = v, 1                                                shift_left v.lo = v.lo, 1; rolc v.hi = v.hi   (C を通す)
//	shift_right v = v, 1 (符号なし)                                    shift_right v.hi = v.hi, 1; rorc v.lo = v.lo
//	shift_left t = x, 8 / shift_right t = x, 8 (符号なし)              バイトの移動
//	if / if_true v                                                     or t = v.lo, v.hi; if t
//
// 分けるのは、ループ内の分解できる命令の数が実体化の数を上回る変数だけ (一時変数は全部分解できるときだけ)。
func splitWords(lmd *ir.Lambda, u *types.Universe) {
	u8 := u.IntType(1, false)
	cfg := ir.BuildCFG(lmd)
	inLoop := map[int]bool{}
	for _, lp := range cfg.Loops() {
		for b := range lp.Blocks {
			for _, i := range cfg.Ops(b) {
				inLoop[i] = true
			}
		}
	}
	refered := map[*ir.Value]bool{}
	for _, op := range lmd.Ops {
		if op != nil && op.Code == ir.OpRef {
			refered[ir.UnderlyingValue(op.Src[0])] = true
		}
	}
	// 候補: 2 バイトの整数のローカル (引数・結果・アドレスを取られたものは除く)
	cands := map[*ir.Value]bool{}
	for _, v := range lmd.Vars {
		if v.Kind == ir.KindLocal && v.Type.Size == 2 && v.Type.Kind == types.Int && !refered[v] &&
			(v.LocalType == ir.LTNone || v.LocalType == ir.LTTemp) && v.Home == nil {
			cands[v] = true
		}
	}
	if len(cands) == 0 {
		return
	}
	// 評価: 命令ごとに、分解できるか (candidates に依存する: xor v = v, t の t が分けられるかは t 次第なので、
	// 「分けない」と決めた変数を除きながら固定点まで)
	for {
		changed := false
		score := map[*ir.Value]int{}
		for i, op := range lmd.Ops {
			if op == nil {
				continue
			}
			w := 1
			if inLoop[i] {
				w = 4
			}
			vs := wordVars(op, cands)
			if len(vs) == 0 {
				continue
			}
			ok := decomposable(op, cands)
			for _, v := range vs {
				if ok {
					score[v] += w
				} else {
					score[v] -= w
					if v.LocalType == ir.LTTemp {
						score[v] -= 1000 // 一時変数は全部分解できるときだけ
					}
				}
			}
		}
		for v := range cands {
			if score[v] <= 0 {
				delete(cands, v)
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	if len(cands) == 0 {
		return
	}
	// 変換
	parts := map[*ir.Value][2]*ir.Value{}
	var order []*ir.Value // 宣言順 (map の順序で出力が変わらないように)
	for _, v := range lmd.Vars {
		if cands[v] {
			order = append(order, v)
		}
	}
	for _, v := range order {
		lo := ir.NewLocal(v.Name+".lo", u8, ir.LTNone)
		hi := ir.NewLocal(v.Name+".hi", u8, ir.LTNone)
		if v.LocalType == ir.LTTemp {
			lo.LocalType, hi.LocalType = ir.LTTemp, ir.LTTemp
		}
		parts[v] = [2]*ir.Value{lo, hi}
		lmd.Vars = append(lmd.Vars, lo, hi)
	}
	byteOf := func(o ir.Operand, k int) ir.Operand { // o (2 バイト) の k バイト目
		if n, ok := ir.ValIntLiteral(o); ok {
			return ir.NewIntLiteral("", u8, (n>>(8*k))&255)
		}
		if v := ir.UnderlyingValue(o); v != nil && parts[v] != [2]*ir.Value{} && plainWord(o) {
			return parts[v][k]
		}
		if ir.ValType(o).Size == 1 { // 1 バイトの値のゼロ拡張
			if k == 0 {
				return o
			}
			return ir.NewIntLiteral("", u8, 0)
		}
		if cv, ok := o.(*ir.CastedValue); ok && cv.Offset == 0 && cv.Type.Size == 2 && ir.ValType(cv.From).Size == 1 {
			// 1 バイトの値 (変数、struct のフィールド `<u8+1>s0`、狭めた cast) を 2 バイトに広げた cast (`x as int16`):
			// 下位はその値、上位は 0 (符号付きは wordOperand が弾く。cast の 1 バイト目を読むと隣のバイトを拾ってしまう。
			// fuzz で発覚。フィールドの場合は元の変数でなく From の cast を包む: オフセットを保つ)
			if k == 0 {
				return ir.NewCastedValue(cv.From, u8, 0)
			}
			return ir.NewIntLiteral("", u8, 0)
		}
		if v := ir.UnderlyingValue(o); v != nil && v.Type.Size == 1 {
			if k == 0 {
				return ir.NewCastedValue(v, u8, 0)
			}
			return ir.NewIntLiteral("", u8, 0)
		}
		// o (2 バイト。struct のフィールド `<int16+1>s0` のような cast の連鎖も) の k バイト目。オフセットは o の cast が
		// 持っているので足さない (足すと二重に数える。fuzz で発覚)
		return ir.NewCastedValue(o, u8, k)
	}
	var out []*ir.Op
	for _, op := range lmd.Ops {
		if op == nil {
			continue
		}
		vs := wordVars(op, cands)
		if len(vs) == 0 {
			out = append(out, op)
			continue
		}
		if !decomposable(op, cands) {
			// 実体化: 読む変数は前で v に写し、書く変数は後で v から写す
			defs, uses := ir.DefUse(op)
			for _, o := range uses {
				if v := ir.UnderlyingValue(o); v != nil && cands[v] {
					out = append(out, &ir.Op{Code: ir.OpLoad, Dst: ir.NewCastedValue(v, u8, 0), Src: []ir.Operand{parts[v][0]}, Pos: op.Pos},
						&ir.Op{Code: ir.OpLoad, Dst: ir.NewCastedValue(v, u8, 1), Src: []ir.Operand{parts[v][1]}, Pos: op.Pos})
				}
			}
			out = append(out, op)
			for _, o := range defs {
				if v := ir.UnderlyingValue(o); v != nil && cands[v] {
					out = append(out, &ir.Op{Code: ir.OpLoad, Dst: parts[v][0], Src: []ir.Operand{ir.NewCastedValue(v, u8, 0)}, Pos: op.Pos},
						&ir.Op{Code: ir.OpLoad, Dst: parts[v][1], Src: []ir.Operand{ir.NewCastedValue(v, u8, 1)}, Pos: op.Pos})
				}
			}
			continue
		}
		pos := op.Pos
		mk := func(code ir.OpCode, dst ir.Operand, src ...ir.Operand) *ir.Op {
			return &ir.Op{Code: code, Dst: dst, Src: src, Pos: pos}
		}
		switch op.Code {
		case ir.OpLoad:
			if ir.ValType(op.Dst).Size == 2 && isWord(op.Dst, cands) {
				out = append(out, mk(ir.OpLoad, byteOf(op.Dst, 0), byteOf(op.Src[0], 0)), mk(ir.OpLoad, byteOf(op.Dst, 1), byteOf(op.Src[0], 1)))
			} else {
				// load w = v (w は分けない 2 バイト)
				out = append(out, mk(ir.OpLoad, byteOf(op.Dst, 0), byteOf(op.Src[0], 0)), mk(ir.OpLoad, byteOf(op.Dst, 1), byteOf(op.Src[0], 1)))
			}
		case ir.OpXor, ir.OpAnd, ir.OpOr:
			out = append(out, mk(op.Code, byteOf(op.Dst, 0), byteOf(op.Src[0], 0), byteOf(op.Src[1], 0)),
				mk(op.Code, byteOf(op.Dst, 1), byteOf(op.Src[0], 1), byteOf(op.Src[1], 1)))
		case ir.OpShiftLeft:
			n, _ := ir.ValIntLiteral(op.Src[1])
			if n == 1 {
				out = append(out, mk(ir.OpShiftLeft, byteOf(op.Dst, 0), byteOf(op.Src[0], 0), op.Src[1]), mk(ir.OpRolC, byteOf(op.Dst, 1), byteOf(op.Src[0], 1)))
			} else { // 8: lo = 0, hi = src の下位
				out = append(out, mk(ir.OpLoad, byteOf(op.Dst, 1), byteOf(op.Src[0], 0)), mk(ir.OpLoad, byteOf(op.Dst, 0), ir.NewIntLiteral("", u8, 0)))
			}
		case ir.OpShiftRight:
			n, _ := ir.ValIntLiteral(op.Src[1])
			if n == 1 {
				out = append(out, mk(ir.OpShiftRight, byteOf(op.Dst, 1), byteOf(op.Src[0], 1), op.Src[1]), mk(ir.OpRorC, byteOf(op.Dst, 0), byteOf(op.Src[0], 0)))
			} else { // 8: lo = src の上位, hi = 0
				out = append(out, mk(ir.OpLoad, byteOf(op.Dst, 0), byteOf(op.Src[0], 1)), mk(ir.OpLoad, byteOf(op.Dst, 1), ir.NewIntLiteral("", u8, 0)))
			}
		case ir.OpIf, ir.OpIfTrue:
			t := ir.NewLocal(fmt.Sprintf("$w%d", len(lmd.Vars)), u8, ir.LTTemp)
			lmd.Vars = append(lmd.Vars, t)
			out = append(out, mk(ir.OpOr, t, byteOf(op.Src[0], 0), byteOf(op.Src[0], 1)), &ir.Op{Code: op.Code, Src: []ir.Operand{t}, Label: op.Label, Pos: pos})
		default:
			panic("splitWords: decomposable op not handled")
		}
	}
	lmd.Ops = out
	propagateBytes(lmd)
}

// wordVars は op が読み書きする候補の変数 (2 バイトのまま参照しているもの)。
func wordVars(op *ir.Op, cands map[*ir.Value]bool) []*ir.Value {
	var r []*ir.Value
	defs, uses := ir.DefUse(op)
	for _, o := range append(append([]ir.Operand{}, defs...), uses...) {
		if v := ir.UnderlyingValue(o); v != nil && cands[v] {
			r = append(r, v)
		}
	}
	return r
}

// isWord は o が候補の変数を 2 バイトのまま (オフセット 0、サイズ 2) 参照しているか。
func isWord(o ir.Operand, cands map[*ir.Value]bool) bool {
	v := ir.UnderlyingValue(o)
	return v != nil && cands[v] && plainWord(o)
}

// plainWord は o が 2 バイトの変数そのもの、またはそれを 2 バイトのまま読み替えた cast か (`((p0 as int) as int16)` のように
// 途中で 1 バイトに狭めた連鎖は下位バイトのゼロ拡張なので違う: 分けた変数の上位を読んでいた。fuzz で発覚)。
func plainWord(o ir.Operand) bool {
	for {
		switch x := o.(type) {
		case *ir.Value:
			return x.Type.Size == 2
		case *ir.CastedValue:
			if x.Offset != 0 || x.Type.Size != 2 {
				return false
			}
			o = x.From
		default:
			return false
		}
	}
}

// wordOperand は o が分解に使える 2 バイト (または 1 バイト) の値か: 候補の変数、2 バイトのメモリ上の変数、定数、1 バイトの値。
func wordOperand(o ir.Operand, cands map[*ir.Value]bool) bool {
	if o == nil {
		return false
	}
	if _, ok := ir.ValIntLiteral(o); ok {
		return true
	}
	if _, ok := o.(*ir.PointeredArray); ok {
		return false
	}
	v := ir.UnderlyingValue(o)
	if v == nil {
		return false
	}
	if cands[v] {
		return isWord(o, cands)
	}
	if ir.ValType(o).Kind != types.Int {
		return false
	}
	if cv, ok := o.(*ir.CastedValue); ok && cv.Offset == 0 && cv.Type.Size == 2 && ir.ValType(cv.From).Size == 1 {
		// 1 バイトの値を 2 バイトに広げた cast: 符号なしのゼロ拡張だけ (byteOf と同じ判定)
		ft := ir.ValType(cv.From)
		return (ft.Kind == types.Int || ft.Kind == types.Bool) && !ft.Signed
	}
	if v.Type.Size == 1 {
		// 1 バイトの変数 (2 バイトに広げた cast も): 符号なしのゼロ拡張だけ (符号拡張はバイトに分けられない)
		return ir.ValOffset(o) == 0 && (ir.ValType(o).Size == 1 || !v.Type.Signed)
	}
	if ir.ValType(o).Size == 1 {
		return true
	}
	return ir.ValType(o).Size == 2 && (v.Kind == ir.KindLocal || v.Kind == ir.KindGlobal) && v.Type.Kind != types.Array
}

// decomposable は op をバイトごとの命令に分けられるか (候補の変数を含む op について)。
func decomposable(op *ir.Op, cands map[*ir.Value]bool) bool {
	// 候補の変数の一部 (CastedValue で 1 バイト) を触る命令はそのままでは扱えない
	defs, uses := ir.DefUse(op)
	for _, o := range append(append([]ir.Operand{}, defs...), uses...) {
		if v := ir.UnderlyingValue(o); v != nil && cands[v] && !isWord(o, cands) {
			return false
		}
	}
	switch op.Code {
	case ir.OpLoad:
		if ir.ValType(op.Dst).Size != 2 {
			return false
		}
		if isWord(op.Dst, cands) {
			return wordOperand(op.Src[0], cands) && !ir.ValType(op.Src[0]).Signed || (ir.ValType(op.Src[0]).Size == 2 && wordOperand(op.Src[0], cands))
		}
		// load w = v
		wv := ir.UnderlyingValue(op.Dst)
		return isWord(op.Src[0], cands) && wv != nil && (wv.Kind == ir.KindLocal || wv.Kind == ir.KindGlobal) && wv.Type.Kind == types.Int && ir.ValType(op.Dst).Size == 2
	case ir.OpXor, ir.OpAnd, ir.OpOr:
		return ir.ValType(op.Dst).Size == 2 && isWord(op.Dst, cands) && wordOperand(op.Src[0], cands) && wordOperand(op.Src[1], cands) &&
			ir.ValType(op.Src[0]).Size == 2 && ir.ValType(op.Src[1]).Size == 2
	case ir.OpShiftLeft, ir.OpShiftRight:
		n, lit := ir.ValIntLiteral(op.Src[1])
		if !lit || !isWord(op.Dst, cands) {
			return false
		}
		if op.Code == ir.OpShiftRight && ir.ValType(op.Src[0]).Signed {
			return false
		}
		if n == 1 {
			return isWord(op.Src[0], cands) // ローテートは C を通すので v = v << 1 の形だけ
		}
		if n == 8 {
			return wordOperand(op.Src[0], cands) && (op.Code == ir.OpShiftLeft || ir.ValType(op.Src[0]).Size == 2)
		}
		return false
	case ir.OpIf, ir.OpIfTrue:
		return isWord(op.Src[0], cands)
	}
	return false
}

// propagateBytes は分けた後の 1 バイトの一時変数の定数 / コピーを伝播して、恒等演算 (x ^ 0、x | 0、x & 255) を消す。
// 一時変数は定義が 1 つで、定義より後で使われる (opt の前提) ので、定義の右辺が定数か 1 バイトの一時変数なら
// 使用側に置き換えてよい。
func propagateBytes(lmd *ir.Lambda) {
	defs := map[*ir.Value]int{}
	nDefs := map[*ir.Value]int{}
	for i, op := range lmd.Ops {
		if op == nil {
			continue
		}
		ds, _ := ir.DefUse(op)
		for _, d := range ds {
			if v, ok := d.(*ir.Value); ok {
				nDefs[v]++
				defs[v] = i
			}
		}
	}
	subst := map[*ir.Value]ir.Operand{}
	for v, i := range defs {
		op := lmd.Ops[i]
		if v.LocalType != ir.LTTemp || nDefs[v] != 1 || op.Code != ir.OpLoad || v.Type.Size != 1 || op.Dst != ir.Operand(v) {
			continue
		}
		src := op.Src[0]
		if _, lit := ir.ValIntLiteral(src); lit {
			subst[v] = src
		} else if sv, ok := src.(*ir.Value); ok && sv.LocalType == ir.LTTemp && nDefs[sv] == 1 && sv.Type.Size == 1 {
			subst[v] = sv
		}
	}
	resolve := func(o ir.Operand) ir.Operand {
		for k := 0; k < 8; k++ {
			v, ok := o.(*ir.Value)
			if !ok {
				return o
			}
			s, ok := subst[v]
			if !ok {
				return o
			}
			o = s
		}
		return o
	}
	for _, op := range lmd.Ops {
		if op == nil {
			continue
		}
		for k := range op.Src {
			op.Src[k] = resolve(op.Src[k])
		}
	}
	for i, op := range lmd.Ops {
		if op == nil {
			continue
		}
		switch op.Code {
		case ir.OpXor, ir.OpOr:
			for k := 0; k < 2; k++ {
				if n, ok := ir.ValIntLiteral(op.Src[k]); ok && n == 0 {
					other := op.Src[1-k]
					if other == op.Dst {
						lmd.Ops[i] = nil
					} else {
						lmd.Ops[i] = &ir.Op{Code: ir.OpLoad, Dst: op.Dst, Src: []ir.Operand{other}, Pos: op.Pos}
					}
					break
				}
			}
		case ir.OpAnd:
			for k := 0; k < 2; k++ {
				if n, ok := ir.ValIntLiteral(op.Src[k]); ok && ir.ValType(op.Dst).Size == 1 {
					other := op.Src[1-k]
					if n&255 == 255 {
						if other == op.Dst {
							lmd.Ops[i] = nil
						} else {
							lmd.Ops[i] = &ir.Op{Code: ir.OpLoad, Dst: op.Dst, Src: []ir.Operand{other}, Pos: op.Pos}
						}
					} else if n == 0 {
						lmd.Ops[i] = &ir.Op{Code: ir.OpLoad, Dst: op.Dst, Src: []ir.Operand{op.Src[k]}, Pos: op.Pos}
					}
					break
				}
			}
		case ir.OpLoad:
			if op.Src[0] == op.Dst {
				lmd.Ops[i] = nil
			}
		}
	}
	// 使われなくなった一時変数の load を消す (残すと定義と使用の間に挟まって、隣の一時変数の A 割付を止める)
	uses := map[*ir.Value]int{}
	for _, op := range lmd.Ops {
		if op == nil {
			continue
		}
		_, us := ir.DefUse(op)
		for _, o := range us {
			if v := ir.UnderlyingValue(o); v != nil {
				uses[v]++
			}
		}
	}
	for i, op := range lmd.Ops {
		if op == nil || op.Code != ir.OpLoad {
			continue
		}
		if v, ok := op.Dst.(*ir.Value); ok && v.LocalType == ir.LTTemp && uses[v] == 0 {
			lmd.Ops[i] = nil
		}
	}
}
