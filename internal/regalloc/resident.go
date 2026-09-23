package regalloc

// ループ内のレジスタ常駐 (doc/v2_regalloc.md)。最内ループごとに 1 バイトのローカル変数を A に 1 つ、Y に 1 つまで選び、
// ループの中ではレジスタに置いたままにする。ループ内の命令は Classify で、A / Y それぞれについて
// 「レジスタのまま実行できる (friendly)」「触らず壊さない (free)」「壊すので前後で退避する (clobber)」に分かれ、
// コスト (得 − 退避の損) が最大の組を採用する。

import (
	"fmt"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/types"
)

// ResClass は常駐中の命令の扱い。
type ResClass uint8

const (
	ResClobber  ResClass = iota // レジスタを壊す: 変数が live-in なら退避、live-out なら復帰で挟み、命令は変数をメモリ (Home) として出す
	ResFriendly                 // 変数をレジスタのまま扱える
	ResFree                     // 変数を触らずレジスタも壊さない
)

// Decision は命令 1 つの扱い (A / Y / X)。
type Decision struct {
	A, Y, X ResClass
	UseY    bool // A が常駐変数で塞がっているので、この命令は Y で代用する (ldy / sty / cpy)
}

// isIncDec は codegen の incDec と同じ条件 (x = x ± 1、1〜2 バイト、メモリ上)。
func isIncDec(op *ir.Op) bool {
	return isStep(op, 1)
}

// isStep は `x = x ± k` (k は 1〜maxK の定数、1〜2 バイト) の形か。Y / X に常駐する 1 バイトの添字なら iny × k で回せる
// (StepMax まで。castle の 4 バイト飛びのスプライト消去ループ)。
func isStep(op *ir.Op, maxK int) bool {
	if op.Code != ir.OpAdd && op.Code != ir.OpSub || len(op.Src) != 2 {
		return false
	}
	if maxK > 1 && ir.Disabled("step") {
		maxK = 1
	}
	k, lit := ir.ValIntLiteral(op.Src[1])
	if !lit || k < 1 || k > maxK || ir.ValType(op.Dst).Size > 2 {
		return false
	}
	d, s := ir.UnderlyingValue(op.Dst), ir.UnderlyingValue(op.Src[0])
	// ゼロ拡張したバイトを含む cast (`x = ((x as uint8) as int16) + 1`) は x のその場の inc ではない (上位を 0 にする)
	return d != nil && d == s && ir.ValOffset(op.Dst) == ir.ValOffset(op.Src[0]) && ir.ValType(op.Dst).Size == ir.ValType(op.Src[0]).Size &&
		ir.PlainOperand(op.Dst) && ir.PlainOperand(op.Src[0])
}

// stepGain は isStep な命令を iny × k にしたときの得: k = 1 は inc x (5) → iny (2) で 3、k ≥ 2 は lda; clc; adc #k; sta (10) → 2k。
func stepGain(op *ir.Op) int {
	k, _ := ir.ValIntLiteral(op.Src[1])
	if k == 1 {
		return 3
	}
	return 10 - 2*k
}

// StepMax は Y / X に常駐する添字の `i += k` を iny × k にする k の上限 (k 回で 2k サイクル。5 以上は tya; clc; adc; tay の 8 と変わらない)。
const StepMax = 4

// isMemShift は codegen の shiftInMemory が A を使わずに出せる形か (定数シフト、2 バイト以下、`x = x << n` のように
// 結果と入力が同じ場所)。別の場所への写し (lda / sta)、2 バイトの 8 以上のシフト (バイトの移動)、符号付きの右シフト
// (`lda hi; cmp #128`) は A を使うので含めない (x.hi を A に常駐させたまま `x >> 8` を出して A を壊していた。
// SSA のコピー伝播で形が変わって発覚。TestResidentMemShift)。
func isMemShift(op *ir.Op) bool {
	if op.Code != ir.OpShiftLeft && op.Code != ir.OpShiftRight {
		return false
	}
	n, lit := ir.ValIntLiteral(op.Src[1])
	if !lit {
		return false
	}
	size := ir.ValType(op.Dst).Size
	if size > 2 || ir.ValKind(op.Dst) == ir.KindLiteral {
		return false
	}
	d, s := ir.UnderlyingValue(op.Dst), ir.UnderlyingValue(op.Src[0])
	if d == nil || d != s || ir.ValOffset(op.Dst) != ir.ValOffset(op.Src[0]) || !ir.PlainOperand(op.Dst) || !ir.PlainOperand(op.Src[0]) {
		return false
	}
	if op.Code == ir.OpShiftRight && ir.ValType(op.Src[0]).Signed {
		return false
	}
	if size == 2 && n >= 8 {
		return false
	}
	return true
}

// inReg は割付後にレジスタ / フラグに置かれている一時変数か (メモリではない)。
func inReg(o ir.Operand) bool {
	if ir.ValKind(o) != ir.KindLocal {
		return false
	}
	switch ir.ValLocation(o) {
	case ir.LocA, ir.LocY, ir.LocX, ir.LocCond:
		return true
	}
	return false
}

// isMemByte は 1 バイトのメモリ上の値 (ローカル / グローバル) か。
func isMemByte(o ir.Operand) bool {
	if o == nil || ir.ValType(o).Size != 1 {
		return false
	}
	switch o.(type) {
	case *ir.Value, *ir.CastedValue:
	default:
		return false
	}
	k := ir.ValKind(o)
	return (k == ir.KindLocal || k == ir.KindGlobal) && !inReg(o)
}

// isMemOrLit は Y 経由で写せる値 (メモリ上、または即値。2 バイトも可) か。
func isMemOrLit(o ir.Operand) bool {
	if o == nil {
		return false
	}
	if _, ok := o.(*ir.PointeredArray); ok {
		return false
	}
	if inReg(o) {
		return false
	}
	return isMemByte(o) || ir.ValKind(o) == ir.KindLiteral || (ir.ValType(o).Size == 2 && (ir.ValKind(o) == ir.KindLocal || ir.ValKind(o) == ir.KindGlobal))
}

// cmpOperand は cpx / cpy の第 2 オペランドにできる値か: 即値かメモリだが、stack 関数のフレーム (`S+k,x`) は不可
// (cpx / cpy に zp,X のモードは無い。`cpy 0+<S+6,x` を出していた。fuzz で発覚)。
func cmpOperand(o ir.Operand) bool {
	return isMemOrLit(o) && ir.ValLocation(o) != ir.LocFrame
}

// condPredicted は eq / lt の結果が直後の if でだけ使われる (コンディションフラグに割り付く) か。
func condPredicted(lmd *ir.Lambda, i int) bool {
	op := lmd.Ops[i]
	if i+1 >= len(lmd.Ops) || lmd.Ops[i+1] == nil {
		return false
	}
	next := lmd.Ops[i+1]
	if next.Code != ir.OpIf && next.Code != ir.OpIfTrue {
		return false
	}
	t, ok := op.Dst.(*ir.Value)
	return ok && t.LocalType == ir.LTTemp && next.Src[0] == ir.Operand(t)
}

// involves は op が v を読むか書くか。
func involves(op *ir.Op, v *ir.Value) bool {
	if v == nil {
		return false
	}
	defs, uses := ir.DefUse(op)
	for _, o := range defs {
		if ir.UnderlyingValue(o) == v {
			return true
		}
	}
	for _, o := range uses {
		if ir.UnderlyingValue(o) == v {
			return true
		}
	}
	return false
}

func isV(o ir.Operand, v *ir.Value) bool {
	return o != nil && v != nil && ir.UnderlyingValue(o) == v && ir.ValOffset(o) == 0 && ir.ValType(o).Size == 1
}

// friendlyA は v が A に常駐しているとき、op を A のまま実行できるか (できれば節約できるサイクル数の目安)。
func friendlyA(lmd *ir.Lambda, i int, v *ir.Value, liveOut bool) (bool, int) {
	op := lmd.Ops[i]
	switch op.Code {
	case ir.OpLoad:
		if isV(op.Dst, v) && isMemOrLit(op.Src[0]) && ir.ValType(op.Src[0]).Size == 1 {
			return true, 3 // lda x (sta v が消える)
		}
		if isV(op.Src[0], v) && isMemByte(op.Dst) {
			return true, 3 // sta x (lda v が消える)
		}
	case ir.OpAdd, ir.OpSub, ir.OpAnd, ir.OpOr, ir.OpXor:
		if isIncDec(op) && isV(op.Dst, v) {
			return true, 1 // inc x (5) → clc; adc #1 (4)
		}
		first := isV(op.Src[0], v) || (op.Code != ir.OpSub && isV(op.Src[1], v) && !isV(op.Src[0], v))
		if !first || ir.ValType(op.Dst).Size != 1 {
			break
		}
		other := op.Src[1]
		if !isV(op.Src[0], v) {
			other = op.Src[0]
		}
		if !isMemOrLit(other) || ir.ValType(other).Size != 1 {
			break
		}
		if isV(op.Dst, v) {
			return true, 6 // lda v … sta v が消える
		}
		if !liveOut && isMemByte(op.Dst) {
			return true, 3
		}
	case ir.OpShiftLeft, ir.OpShiftRight:
		if _, lit := ir.ValIntLiteral(op.Src[1]); lit && isV(op.Src[0], v) && (isV(op.Dst, v) || (!liveOut && isMemByte(op.Dst))) {
			return true, 3 // asl a (2) vs asl mem (5)
		}
	case ir.OpUminus, ir.OpBitNot:
		if isV(op.Src[0], v) && isV(op.Dst, v) {
			return true, 3
		}
	case ir.OpRolC, ir.OpRorC:
		if isV(op.Src[0], v) && isV(op.Dst, v) {
			return true, 3 // rol mem (5) → rol a (2)
		}
	case ir.OpIf, ir.OpIfTrue:
		if isV(op.Src[0], v) {
			return true, 1 // lda v (3) → cmp #0 (2)
		}
	case ir.OpEq, ir.OpLt:
		if op.Code == ir.OpEq && isV(op.Src[1], v) && condPredicted(lmd, i) && isMemOrLit(op.Src[0]) && ir.ValType(op.Src[0]).Size == 1 {
			return true, 3 // 可換: cmp k
		}
		if isV(op.Src[0], v) && condPredicted(lmd, i) && isMemOrLit(op.Src[1]) && ir.ValType(op.Src[1]).Size == 1 {
			// 符号付きの比較は sec; sbc; bvc; eor で A を壊す (cmp と違って) ので、v がその後も要るなら A のままではできない
			signed := op.Code == ir.OpLt && (ir.ValType(op.Src[0]).Signed || ir.ValType(op.Src[1]).Signed)
			if !signed || !liveOut {
				return true, 3
			}
		}
	case ir.OpIndexPget:
		if isV(op.Dst, v) && ir.ValType(op.In(1)).Size == 1 && !isV(op.In(1), v) {
			return true, 3
		}
		if isV(op.In(1), v) && !isV(op.Dst, v) && !liveOut {
			return true, 1 // tay
		}
	case ir.OpIndexPset:
		if isV(op.In(2), v) && ir.ValType(op.In(1)).Size == 1 && !isV(op.In(1), v) {
			return true, 3
		}
	case ir.OpPset:
		if isV(op.In(1), v) {
			return true, 3
		}
	case ir.OpFieldPset:
		if isV(op.In(2), v) {
			return true, 3
		}
	case ir.OpPushArg, ir.OpPushFastcallArg:
		if isV(op.In(0), v) {
			return true, 3
		}
	case ir.OpReturn:
		if isV(op.In(0), v) {
			// 戻り値を A から書く。ただしグローバルの常駐は return で書き戻さなければならないので friendly にしない
			// (`g0 = g1; return g0` で g0 が書き戻されなかった。fuzz で発覚)
			home := v
			if v.Home != nil {
				home = v.Home
			}
			if home.Kind == ir.KindGlobal {
				return false, 0
			}
			return true, 3
		}
	}
	return false, 0
}

// byteIndex は index_pget / index_pset の添字がそのまま Y / X に入る形か (要素 1 バイト、または opt.scaleIndex でバイト単位にした添字)。
func byteIndex(op *ir.Op) bool {
	return ir.ValType(op.In(0)).Base.Size == 1 || op.Scaled
}

// friendlyY は v が Y に常駐しているとき、op を Y のまま実行できるか (添字と 1 バイトのカウンタの形)。
func friendlyY(lmd *ir.Lambda, i int, v *ir.Value) (bool, int) {
	op := lmd.Ops[i]
	switch op.Code {
	case ir.OpIndexPget:
		// 要素 1 バイトの配列 / ゼロページのポインタの添字 (ldy が消える)
		if isV(op.In(1), v) && !isV(op.Dst, v) && byteIndex(op) {
			return true, 3
		}
	case ir.OpIndexPset:
		if isV(op.In(1), v) && !isV(op.In(2), v) && byteIndex(op) {
			return true, 3
		}
	case ir.OpAdd, ir.OpSub:
		if isStep(op, StepMax) && isV(op.Dst, v) {
			return true, stepGain(op) // inc x (5) → iny (2)。i += k: lda; clc; adc #k; sta (10) → iny × k (2k)
		}
	case ir.OpIf, ir.OpIfTrue:
		if isV(op.Src[0], v) {
			return true, 2 // lda v; bne → cpy #0; bne (直前が iny / dey ならピープホールが cpy を消す)
		}
	case ir.OpEq, ir.OpLt:
		if isV(op.Src[0], v) && condPredicted(lmd, i) && cmpOperand(op.Src[1]) && ir.ValType(op.Src[1]).Size == 1 &&
			(op.Code == ir.OpEq || (!ir.ValType(op.Src[0]).Signed && !ir.ValType(op.Src[1]).Signed)) {
			return true, 3 // lda v; cmp k → cpy k
		}
		if op.Code == ir.OpEq && isV(op.Src[1], v) && condPredicted(lmd, i) && cmpOperand(op.Src[0]) && ir.ValType(op.Src[0]).Size == 1 {
			return true, 3 // 可換: cpy k
		}
	case ir.OpLoad:
		if isV(op.Dst, v) && isMemOrLit(op.Src[0]) && ir.ValType(op.Src[0]).Size == 1 {
			return true, 3 // ldy x
		}
		if isV(op.Src[0], v) && isMemByte(op.Dst) {
			return true, 3 // sty x
		}
	}
	return false, 0
}

// friendlyX は v が X に常駐しているとき、op を X のまま実行できるか (グローバル配列の添字と 1 バイトのカウンタ)。
func friendlyX(lmd *ir.Lambda, i int, v *ir.Value) (bool, int) {
	op := lmd.Ops[i]
	globalArray := func(o ir.Operand) bool {
		return ir.ValKind(o) == ir.KindGlobal && ir.ValType(o).Kind == types.Array && byteIndex(op)
	}
	switch op.Code {
	case ir.OpIndexPget:
		if isV(op.In(1), v) && !isV(op.Dst, v) && globalArray(op.In(0)) {
			return true, 3 // lda a,x
		}
	case ir.OpIndexPset:
		if isV(op.In(1), v) && !isV(op.In(2), v) && globalArray(op.In(0)) {
			return true, 3 // sta a,x
		}
	case ir.OpAdd, ir.OpSub:
		if isStep(op, StepMax) && isV(op.Dst, v) {
			return true, stepGain(op) // inx / dex × k
		}
	case ir.OpIf, ir.OpIfTrue:
		if isV(op.Src[0], v) {
			return true, 2 // cpx #0
		}
	case ir.OpEq, ir.OpLt:
		if isV(op.Src[0], v) && condPredicted(lmd, i) && cmpOperand(op.Src[1]) && ir.ValType(op.Src[1]).Size == 1 &&
			(op.Code == ir.OpEq || (!ir.ValType(op.Src[0]).Signed && !ir.ValType(op.Src[1]).Signed)) {
			return true, 3 // cpx k
		}
		if op.Code == ir.OpEq && isV(op.Src[1], v) && condPredicted(lmd, i) && cmpOperand(op.Src[0]) && ir.ValType(op.Src[0]).Size == 1 {
			return true, 3 // 可換: cpx k
		}
	case ir.OpLoad:
		if isV(op.Dst, v) && isMemOrLit(op.Src[0]) && ir.ValType(op.Src[0]).Size == 1 {
			return true, 3 // ldx x
		}
		if isV(op.Src[0], v) && isMemByte(op.Dst) {
			return true, 3 // stx x
		}
	}
	return false, 0
}

// needsX は op の codegen が X を使うか (stack 系の呼び出しは X = FC_SP にする。ランタイムの乗除算も X を壊しうる)。
func needsX(op *ir.Op) bool {
	switch op.Code {
	case ir.OpPushResult, ir.OpPushArg, ir.OpCall, ir.OpPushFastcallResult, ir.OpPushFastcallArg, ir.OpFastcall,
		ir.OpReturn, ir.OpMul, ir.OpDiv, ir.OpMod, ir.OpAsm, ir.OpSwitch:
		return true
	}
	return false
}

// needsY は op の codegen が (常駐変数としてでなく) Y を作業用に使うか。
func needsY(op *ir.Op, vY *ir.Value) bool {
	switch op.Code {
	case ir.OpPget, ir.OpPset, ir.OpFieldPget, ir.OpFieldPset, ir.OpIndex,
		ir.OpMul, ir.OpDiv, ir.OpMod, ir.OpCall, ir.OpFastcall, ir.OpAsm, ir.OpReturn, // return: グローバルの書き戻しのため
		ir.OpSwitch: // stack 関数ではジャンプテーブルの添字に Y を使う (static では X)
		return true
	case ir.OpIndexPget, ir.OpIndexPset:
		return !isV(op.In(1), vY) || !byteIndex(op)
	case ir.OpPushArg, ir.OpPushFastcallArg:
		return op.ArgY || op.HoldY // 呼び先の Y 渡しの引数を Y に読む / 保持中 (codegen.markArgY)
	case ir.OpLoad, ir.OpSignExtension, ir.OpAdd, ir.OpSub, ir.OpAnd, ir.OpOr, ir.OpXor, ir.OpRolC, ir.OpRorC,
		ir.OpUminus, ir.OpEq, ir.OpLt, ir.OpNot, ir.OpBitNot:
		return op.HoldY
	case ir.OpShiftLeft, ir.OpShiftRight:
		_, lit := ir.ValIntLiteral(op.Src[1])
		return !lit || op.HoldY
	}
	return false
}

// freeA は op が A を使わずに実行できるか (Y での代用を除く)。
func freeA(op *ir.Op) bool {
	switch op.Code {
	case ir.OpLabel, ir.OpJump, ir.OpIfCarry, ir.OpIfNotCarry, ir.OpPushResult, ir.OpPushFastcallResult:
		return true
	case ir.OpAdd, ir.OpSub:
		// 2 バイトの dec は `lda lo; bne; dec hi` で下位を見るので A を壊す (inc は inc lo; bne; inc hi で壊さない。
		// A に常駐した g1 が `g0 -= 1` (16 ビット) で消えていた。fuzz で発覚)
		incDecFree := isIncDec(op) && (op.Code == ir.OpAdd || ir.ValType(op.Dst).Size == 1)
		return incDecFree || (isStep(op, StepMax) && (ir.ValLocation(op.Dst) == ir.LocY || ir.ValLocation(op.Dst) == ir.LocX))
	case ir.OpShiftLeft, ir.OpShiftRight:
		return isMemShift(op)
	case ir.OpRolC, ir.OpRorC:
		return ir.UnderlyingValue(op.Dst) == ir.UnderlyingValue(op.Src[0]) && isMemByte(op.Dst) // rol mem
	case ir.OpIf, ir.OpIfTrue:
		// コンディションの一時変数なら分岐だけ。cast を挟んだ一時変数 (`if ((!x) as int16)`) はメモリから A に読むので不可
		// (A に常駐した値を壊して飛んでいた。fuzz で発覚)
		if _, plain := op.Src[0].(*ir.Value); !plain {
			return false
		}
		// 2 バイトの一時変数 (`if ((g as sint16) << 5)`) はコンディションにはならず lda lo; ora hi で A を壊す
		// (fuzz で発覚: A に常駐したグローバルがループの条件で消えた)
		return ir.ValType(op.Src[0]).Size == 1 && ir.ValLocalType(op.Src[0]) == ir.LTTemp && !isMemByte(op.Src[0])
	}
	return false
}

// yVariant は A が塞がっているとき、op を Y で代用できるか (1〜2 バイトの load、1 バイト変数の if、1 バイト符号なしの比較)。
func yVariant(lmd *ir.Lambda, i int) bool {
	op := lmd.Ops[i]
	switch op.Code {
	case ir.OpLoad:
		return isMemOrLit(op.Src[0]) && isMemOrLit(op.Dst) && ir.ValType(op.Dst).Size <= 2 && ir.ValType(op.Src[0]).Kind != types.Array
	case ir.OpIf, ir.OpIfTrue:
		return isMemByte(op.Src[0])
	case ir.OpEq, ir.OpLt:
		return condPredicted(lmd, i) && isMemOrLit(op.Src[0]) && cmpOperand(op.Src[1]) &&
			ir.ValType(op.Src[0]).Size == 1 && ir.ValType(op.Src[1]).Size == 1 &&
			(op.Code == ir.OpEq || (!ir.ValType(op.Src[0]).Signed && !ir.ValType(op.Src[1]).Signed))
	}
	return false
}

// Classify は vA が A に、vY が Y に、vX が X に常駐しているときの lmd.Ops[i] の扱い (どれも nil 可)。
// aLive / yLive はその命令の入口または出口で変数が生きている (レジスタが塞がっている) か。
// 戻り値の gain は friendly で節約できるサイクル数の目安 (合計)。
func Classify(lmd *ir.Lambda, i int, vA, vY, vX *ir.Value, aLive, aOut, yLive bool) (Decision, int) {
	op := lmd.Ops[i]
	d := Decision{A: ResFree, Y: ResFree, X: ResFree}
	gain := 0
	// グローバル変数の常駐: 呼び出し・asm・ポインタ経由の書き込みはその変数を触りうるので退避 / 復帰する
	touches := func(v *ir.Value) bool { return v != nil && v.Kind == ir.KindGlobal && ir.MayTouchGlobals(op) }
	// codegen から呼ばれるときの v はレジスタの一時変数 (Home が元の変数)。cast を挟んだ使用 (`~(l1 as int16)`) は
	// makeResident が置き換えない (元の変数のメモリを読む) ので、Home を触る命令も「この命令に関わる」= 退避が要る
	involved := func(v *ir.Value) bool { return involves(op, v) || v != nil && v.Home != nil && involves(op, v.Home) }
	// X (inx / cpx / ldx / stx は A も Y も使わない。lda a,x は A を使う)
	xFriendly := false
	if vX != nil {
		if involved(vX) {
			if ok, save := friendlyX(lmd, i, vX); ok {
				d.X = ResFriendly
				xFriendly = true
				gain += save
			} else {
				d.X = ResClobber
			}
		} else if needsX(op) || touches(vX) {
			d.X = ResClobber
		}
	}
	// Y (先に決める: Y のまま実行できる命令 (iny / cpy / ldy / sty / lda a,y) は A を使わない)
	yFriendly := false
	if vY != nil && involved(vY) {
		if ok, save := friendlyY(lmd, i, vY); ok {
			d.Y = ResFriendly
			yFriendly = true
			gain += save
		} else {
			d.Y = ResClobber
		}
	} else if touches(vY) {
		d.Y = ResClobber
	}
	// A
	if vA != nil {
		if involved(vA) {
			if ok, save := friendlyA(lmd, i, vA, aOut); ok {
				d.A = ResFriendly
				gain += save
			} else {
				d.A = ResClobber
			}
		} else if touches(vA) {
			d.A = ResClobber
		} else if !freeA(op) && !(yFriendly && aFreeWithY(op)) && !(xFriendly && aFreeWithY(op)) {
			if aLive && (vY == nil || !yLive) && yVariant(lmd, i) {
				d.UseY = true // Y が空いているので Y で代用
			} else {
				d.A = ResClobber
			}
		}
	}
	if vY != nil && !involves(op, vY) && (needsY(op, vY) || d.UseY) {
		d.Y = ResClobber
	}
	return d, gain
}

// aFreeWithY は Y に常駐する変数を扱う friendly な命令のうち、A を使わないもの (添字の lda a,y / sta a,y は A を使う)。
func aFreeWithY(op *ir.Op) bool {
	switch op.Code {
	case ir.OpAdd, ir.OpSub, ir.OpIf, ir.OpIfTrue, ir.OpEq, ir.OpLt, ir.OpLoad:
		return true
	}
	return false
}

// AllocateResident はループごと (内側から) と、最後にループの外 (関数の直線部分) に A / Y / X に常駐させる変数を選んで
// IR を書き換える (opt の後、AllocateRegister の前)。
// 調査用: 環境変数 FC_NO_RESIDENT で無効化、FC_TRACE_RESIDENT で選んだ変数と見積もりを stderr に出す。
func AllocateResident(lmd *ir.Lambda) {
	if ir.Disabled("resident") {
		return
	}
	funcTried := false
	for iter := 0; iter < 32; iter++ {
		cfg := ir.BuildCFG(lmd)
		loops := cfg.Loops()
		// 内側から (小さいループから) 順に。外側のループの領域は、その中のループを除いたブロック
		// (内側のループは、そこに入る辺で退避し出る辺で復帰する「通過する区間」として扱う)
		sort.Slice(loops, func(i, j int) bool { return len(loops[i].Blocks) < len(loops[j].Blocks) })
		lv := ir.BuildLivenessWithGlobals(lmd)
		done := false
		for _, lp := range loops {
			r := regionOf(lp, loops)
			if len(r) == 0 || hasResident(lmd, cfg, r) {
				continue
			}
			inner := region{} // 領域から除いた内側のループ (境界を毎周通る)
			for b := range lp.Blocks {
				if !r[b] {
					inner[b] = true
				}
			}
			vA, vY, vX, gain := bestPair(lmd, cfg, r, inner, lv, lmd.ABI != ir.ABIStack)
			if vA == nil && vY == nil && vX == nil {
				continue
			}
			if os.Getenv("FC_TRACE_RESIDENT") != "" {
				fmt.Fprintf(os.Stderr, "resident: %s loop %s: A=%s Y=%s X=%s (gain %d/iter)\n", lmd.Id, lp.Header.Label, name(vA), name(vY), name(vX), gain)
			}
			makeResident(lmd, cfg, r, lv, vA, vY, vX)
			done = true
			break // IR が変わったので作り直す
		}
		if !done && !funcTried && !ir.Disabled("func-resident") {
			// ループの外 (関数の直線部分) を 1 つの領域に。入口は関数の先頭 (1 回)、出口は return (退避で書き戻す)、
			// ループは通過する区間 (境界の辺で退避 / 復帰。ループ側の写しとの順序は onEdge が保つ)
			funcTried = true
			r, inner := region{}, region{}
			for _, b := range cfg.Blocks {
				r[b] = true
			}
			for _, lp := range loops {
				for b := range lp.Blocks {
					delete(r, b)
					inner[b] = true
				}
			}
			vA, vY, vX, gain := bestPair(lmd, cfg, r, inner, lv, lmd.ABI != ir.ABIStack)
			if vA != nil || vY != nil || vX != nil {
				if os.Getenv("FC_TRACE_RESIDENT") != "" {
					fmt.Fprintf(os.Stderr, "resident: %s func: A=%s Y=%s X=%s (gain %d)\n", lmd.Id, name(vA), name(vY), name(vX), gain)
				}
				makeResident(lmd, cfg, r, lv, vA, vY, vX)
				done = true
			}
		}
		if !done {
			return
		}
	}
}

// isResCopy は別の常駐の入口 / 出口の写し (load temp = home / load home = temp)。領域が重なるとき (外側のループ、関数全体) は
// 印を付けず、この領域の写しはその前 (退避) / 後 (復帰) に置く。
func isResCopy(op *ir.Op) bool {
	if op == nil || op.Code != ir.OpLoad {
		return false
	}
	d, s := ir.UnderlyingValue(op.Dst), ir.UnderlyingValue(op.Src[0])
	if d == nil || s == nil {
		return false
	}
	return (isResident(d) && d.Home == s) || (isResident(s) && s.Home == d)
}

// isResSpill は常駐の写しのうち、レジスタからメモリ (Home) へ書く退避か (逆はレジスタへの読み込み = 入口の写し / 復帰)。
func isResSpill(op *ir.Op) bool {
	if !isResCopy(op) {
		return false
	}
	s := ir.UnderlyingValue(op.Src[0])
	return isResident(s) && s.Home == ir.UnderlyingValue(op.Dst)
}

// region は常駐の対象になるブロックの集合 (ループから、その中のループを除いたもの)。
type region map[*ir.Block]bool

// regionOf は lp のブロックから、lp に含まれる他のループのブロックを除いたもの。
func regionOf(lp *ir.Loop, loops []*ir.Loop) region {
	r := region{}
	for b := range lp.Blocks {
		r[b] = true
	}
	for _, other := range loops {
		if other == lp || len(other.Blocks) >= len(lp.Blocks) {
			continue
		}
		nested := true
		for b := range other.Blocks {
			if !lp.Blocks[b] {
				nested = false
				break
			}
		}
		if nested {
			for b := range other.Blocks {
				delete(r, b)
			}
		}
	}
	return r
}

// blocks は領域のブロックを Index 順に (map の順序で出力が変わらないように)。
func (r region) blocks() []*ir.Block {
	var bs []*ir.Block
	for b := range r {
		bs = append(bs, b)
	}
	sort.Slice(bs, func(i, j int) bool { return bs[i].Index < bs[j].Index })
	return bs
}

// entries / exits は領域に入る辺 (from は外、to は中) と出る辺 (from は中、to は外)。
// entries / exits は領域に入る辺 / 出る辺 (同じ 2 ブロック間に飛ぶ辺と落ちる辺の両方があっても 1 つ。onEdge が両方に置く)。
func (r region) entries() [][2]*ir.Block {
	var e [][2]*ir.Block
	seen := map[[2]*ir.Block]bool{}
	for _, b := range r.blocks() {
		for _, p := range b.Preds {
			if k := [2]*ir.Block{p, b}; !r[p] && !seen[k] {
				seen[k] = true
				e = append(e, k)
			}
		}
	}
	return e
}

func (r region) exits() [][2]*ir.Block {
	var e [][2]*ir.Block
	seen := map[[2]*ir.Block]bool{}
	for _, b := range r.blocks() {
		for _, s := range b.Succs {
			if k := [2]*ir.Block{b, s}; !r[s] && !seen[k] {
				seen[k] = true
				e = append(e, k)
			}
		}
	}
	return e
}

func name(v *ir.Value) string {
	if v == nil {
		return "-"
	}
	return v.Name
}

func hasResident(lmd *ir.Lambda, cfg *ir.CFG, r region) bool {
	for b := range r {
		for _, i := range cfg.Ops(b) {
			if lmd.Ops[i].Resident != nil || lmd.Ops[i].ResidentY != nil || lmd.Ops[i].ResidentX != nil {
				return true
			}
		}
	}
	return false
}

// candidates はループ内で使われる 1 バイトのローカル変数 (アドレスを取られたもの・結果・一時変数・配列は除く)。
func candidates(lmd *ir.Lambda, cfg *ir.CFG, r region) []*ir.Value {
	refered := map[*ir.Value]bool{}
	for _, op := range lmd.Ops {
		if op != nil && op.Code == ir.OpRef {
			refered[ir.UnderlyingValue(op.Src[0])] = true
		}
	}
	for _, v := range lmd.Vars {
		if v.Home != nil {
			refered[v.Home] = true // 別の領域 (内側のループ) ですでに常駐している変数は、その写しと干渉するので除く
		}
	}
	var res []*ir.Value
	seen := map[*ir.Value]bool{}
	for _, b := range r.blocks() { // 出現順 (同点のときの選択が実行ごとに変わらないように)
		for _, i := range cfg.Ops(b) {
			defs, uses := ir.DefUse(lmd.Ops[i])
			for _, o := range append(append([]ir.Operand{}, defs...), uses...) {
				if _, ok := o.(*ir.PointeredArray); ok {
					continue
				}
				v := ir.UnderlyingValue(o)
				if v == nil || seen[v] || v.Type.Size != 1 || (v.Type.Kind != types.Int && v.Type.Kind != types.Bool) || refered[v] || isResident(v) {
					continue
				}
				switch v.Kind {
				case ir.KindLocal:
					if v.LocalType == ir.LTResult || v.LocalType == ir.LTTemp {
						continue // 一時変数は定義の直後に使うものが大半で、既存の A 割付 (allocateA) が扱う
					}
				case ir.KindGlobal:
					if v.Volatile || v.Symbol == "" {
						continue // I/O レジスタ・asm が触る変数はレジスタに置いたままにできない
					}
				default:
					continue
				}
				seen[v] = true
				res = append(res, v)
			}
		}
	}
	return res
}

// readOnly は領域内に v の定義が無い (Home が常に最新で、退避の書き戻しが要らない) か。
func readOnly(lmd *ir.Lambda, cfg *ir.CFG, r region, v *ir.Value) bool {
	if v == nil {
		return true
	}
	for b := range r {
		for _, i := range cfg.Ops(b) {
			defs, _ := ir.DefUse(lmd.Ops[i])
			for _, d := range defs {
				if ir.UnderlyingValue(d) == v {
					return false
				}
			}
		}
	}
	return true
}

// gainOf は (vA, vY) の組をループに常駐させたときの 1 周あたりの正味の得。
func gainOf(lmd *ir.Lambda, cfg *ir.CFG, r, inner region, lv *ir.Liveness, vA, vY, vX *ir.Value) int {
	gain := 0
	cleanA, cleanY, cleanX := readOnly(lmd, cfg, r, vA), readOnly(lmd, cfg, r, vY), readOnly(lmd, cfg, r, vX)
	clean := map[*ir.Value]bool{vA: cleanA, vY: cleanY, vX: cleanX}
	for b := range r {
		for _, i := range cfg.Ops(b) {
			op := lmd.Ops[i]
			aIn, aOut := vA != nil && lv.LiveIn(i, vA), vA != nil && lv.LiveOut(i, vA)
			yIn, yOut := vY != nil && lv.LiveIn(i, vY), vY != nil && lv.LiveOut(i, vY)
			xIn, xOut := vX != nil && lv.LiveIn(i, vX), vX != nil && lv.LiveOut(i, vX)
			if !aIn && !aOut && !yIn && !yOut && !xIn && !xOut && !involves(op, vA) && !involves(op, vY) && !involves(op, vX) {
				continue
			}
			d, g := Classify(lmd, i, vA, vY, vX, aIn || aOut, aOut, yIn || yOut)
			gain += g
			restored := false
			if d.X == ResClobber {
				if xIn && !cleanX {
					gain -= 3
				}
				if xOut {
					gain -= 3
					restored = true
				}
			}
			if d.A == ResClobber {
				if aIn && !cleanA {
					gain -= 3
				}
				if aOut {
					gain -= 3
					restored = true
				}
			}
			if d.Y == ResClobber {
				if yIn && !cleanY {
					gain -= 3
				}
				if yOut {
					gain -= 3
					restored = true
				}
			}
			if restored && ir.CondRestoreNeedsFlags(op) {
				gain -= 7 // 結果が N / Z のフラグなので復帰を php / plp で挟む (codegen)
			}
		}
	}
	// 領域の中のループ (通過する区間) との境界の写し。ループの入口 / 出口 (領域の外との境界) は 1 回だけなので数えない
	first := func(b *ir.Block) int {
		if o := cfg.Ops(b); len(o) > 0 {
			return o[0]
		}
		return b.Start
	}
	for _, e := range r.entries() {
		if inner[e[0]] {
			for _, v := range []*ir.Value{vA, vY, vX} {
				if v != nil && lv.LiveIn(first(e[1]), v) {
					gain -= 3
				}
			}
		}
	}
	for _, e := range r.exits() {
		if inner[e[1]] {
			for _, v := range []*ir.Value{vA, vY, vX} {
				if v != nil && lv.LiveIn(first(e[1]), v) && !clean[v] {
					gain -= 3
				}
			}
		}
	}
	// 関数全体の領域: 先頭で引数などを写す (1 回)。return での書き戻しは退避 (Classify) として数えられている
	if entry := cfg.Blocks[0]; r[entry] {
		for _, v := range []*ir.Value{vA, vY, vX} {
			if v != nil && lv.LiveIn(first(entry), v) {
				gain -= 3
			}
		}
	}
	return gain
}

// bestPair は正味の得 (1 周あたり) が最大の (A, Y) の組。入口 / 出口の写しがあるので 3 サイクル以上の得を要求する。
func bestPair(lmd *ir.Lambda, cfg *ir.CFG, r, inner region, lv *ir.Liveness, allowX bool) (*ir.Value, *ir.Value, *ir.Value, int) {
	cands := candidates(lmd, cfg, r)
	var bestA, bestY, bestX *ir.Value
	best := 2
	try := func(vA, vY, vX *ir.Value) {
		if g := gainOf(lmd, cfg, r, inner, lv, vA, vY, vX); g > best {
			bestA, bestY, bestX, best = vA, vY, vX, g
		}
	}
	// 組み合わせは候補数の 3 乗になる (関数全体の領域では候補が 20 を超えて castle のコンパイルが 55 秒になった) ので、
	// まずレジスタごとに単独の得を見て上位だけを残す (得が無い変数は組にしても得にならない)
	const keep = 4
	top := func(reg int) []*ir.Value {
		type sc struct {
			v *ir.Value
			g int
		}
		var scs []sc
		for _, v := range cands {
			var g int
			switch reg {
			case 0:
				g = gainOf(lmd, cfg, r, inner, lv, v, nil, nil)
			case 1:
				g = gainOf(lmd, cfg, r, inner, lv, nil, v, nil)
			default:
				g = gainOf(lmd, cfg, r, inner, lv, nil, nil, v)
			}
			if g > 0 {
				scs = append(scs, sc{v, g})
			}
		}
		sort.SliceStable(scs, func(i, j int) bool { return scs[i].g > scs[j].g })
		res := []*ir.Value{nil}
		for i := 0; i < len(scs) && i < keep; i++ {
			res = append(res, scs[i].v)
		}
		return res
	}
	as, ys := top(0), top(1)
	xs := []*ir.Value{nil}
	if allowX {
		xs = top(2)
	}
	// 同点なら Y を優先する (X はポインタの添字に使えない) ので、Y を最も内側で回す
	for _, a := range as {
		for _, x := range xs {
			if x != nil && x == a {
				continue
			}
			for _, y := range ys {
				if y != nil && (y == a || y == x) {
					continue
				}
				if a == nil && y == nil && x == nil {
					continue
				}
				try(a, y, x)
			}
		}
	}
	return bestA, bestY, bestX, best
}

// makeResident はループ内の vA / vY を常駐の一時変数に置き換え、入口 / 出口の辺に写す命令を挿す。
func makeResident(lmd *ir.Lambda, cfg *ir.CFG, r region, lv *ir.Liveness, vA, vY, vX *ir.Value) {
	ops := lmd.Ops
	var rA, rY, rX *ir.Value
	if vX != nil {
		rX = ir.NewLocal(vX.Name+"@X", vX.Type, ir.LTTemp)
		rX.Location, rX.Home, rX.Clean = ir.LocX, vX, readOnly(lmd, cfg, r, vX)
		lmd.Vars = append(lmd.Vars, rX)
	}
	if vA != nil {
		rA = ir.NewLocal(vA.Name+"@A", vA.Type, ir.LTTemp)
		rA.Location, rA.Home, rA.Clean = ir.LocA, vA, readOnly(lmd, cfg, r, vA)
		lmd.Vars = append(lmd.Vars, rA)
	}
	if vY != nil {
		rY = ir.NewLocal(vY.Name+"@Y", vY.Type, ir.LTTemp)
		rY.Location, rY.Home, rY.Clean = ir.LocY, vY, readOnly(lmd, cfg, r, vY)
		lmd.Vars = append(lmd.Vars, rY)
	}
	// 常駐変数に差し替える。cast (`(x as int)` の符号の読み替え) は残す: 落とすと `lt` の符号が変わる
	// (`(f() as int) >= 0` が符号付きの比較になって偽になった。fuzz で発覚)
	rebase := func(o ir.Operand, nv *ir.Value) ir.Operand {
		if cv, ok := o.(*ir.CastedValue); ok {
			return ir.RebaseCast(cv, nv)
		}
		return nv
	}
	replace := func(o ir.Operand) ir.Operand {
		if isV(o, vA) {
			return rebase(o, rA)
		}
		if isV(o, vY) {
			return rebase(o, rY)
		}
		if isV(o, vX) {
			return rebase(o, rX)
		}
		return o
	}
	for b := range r {
		for _, i := range cfg.Ops(b) {
			op := ops[i]
			if isResCopy(op) {
				continue // 内側のループの写し (その常駐の印のまま)
			}
			// 可換な演算で vA が第 2 入力なら第 1 に
			switch op.Code {
			case ir.OpAdd, ir.OpAnd, ir.OpOr, ir.OpXor:
				if vA != nil && isV(op.Src[1], vA) && !isV(op.Src[0], vA) {
					op.Src[0], op.Src[1] = op.Src[1], op.Src[0]
				}
			}
			if vA != nil {
				op.Resident = rA
				op.ResIn, op.ResOut = lv.LiveIn(i, vA), lv.LiveOut(i, vA)
			}
			if vY != nil {
				op.ResidentY = rY
				op.ResYIn, op.ResYOut = lv.LiveIn(i, vY), lv.LiveOut(i, vY)
			}
			if vX != nil {
				op.ResidentX = rX
				op.ResXIn, op.ResXOut = lv.LiveIn(i, vX), lv.LiveOut(i, vX)
			}
			for k := range op.Src {
				op.Src[k] = replace(op.Src[k])
			}
			if op.Dst != nil {
				op.Dst = replace(op.Dst)
			}
		}
	}
	// 入口 / 出口の写し。挿入は添字が変わらないように「位置 → 挿す命令」を集めてから 1 度に作り直す
	before := map[int][]*ir.Op{} // ops[i] の前に
	after := map[int][]*ir.Op{}  // ops[i] の後に
	var tail []*ir.Op            // 末尾に足すブロック (辺の分割)
	firstOp := func(b *ir.Block) int {
		o := cfg.Ops(b)
		if len(o) == 0 {
			return b.Start
		}
		return o[0]
	}
	lastOp := func(b *ir.Block) int {
		o := cfg.Ops(b)
		if len(o) == 0 {
			return -1
		}
		return o[len(o)-1]
	}
	labelNo := 0
	for _, op := range ops {
		if op != nil && op.Code == ir.OpLabel {
			if k := strings.LastIndex(op.Label, "_"); k >= 0 {
				if n, err := strconv.Atoi(op.Label[k+1:]); err == nil && n > labelNo {
					labelNo = n
				}
			}
		}
	}
	newLabel := func() string {
		labelNo++
		return fmt.Sprintf("@res_%d", labelNo)
	}
	// 辺 (from → to) に命令を挿す。to は from の直後 (fallthrough)、jump の飛び先、または条件分岐の飛び先。
	// 同じ辺に内側のループの写しがすでにあれば、退避 (spill) はその前、復帰はその後に置く
	// (内側に入る辺: この領域を退避してから内側を復帰、内側から出る辺: 内側を退避してからこの領域を復帰)
	// copyBlock は b が「ラベル; 常駐の写しだけ; jmp」のブロック (先の領域が辺を分割して作ったもの) なら、
	// 最初の写しと jmp の位置を返す。
	copyBlock := func(b *ir.Block) (first, jump int, ok bool) {
		o := cfg.Ops(b)
		if len(o) < 2 || ops[o[len(o)-1]].Code != ir.OpJump {
			return 0, 0, false
		}
		first, firstLoad := -1, -1
		for _, i := range o[:len(o)-1] {
			switch {
			case ops[i].Code == ir.OpLabel:
			case isResCopy(ops[i]):
				if first < 0 {
					first = i
				}
				if firstLoad < 0 && !isResSpill(ops[i]) {
					firstLoad = i
				}
			default:
				return 0, 0, false
			}
		}
		if first < 0 {
			return 0, 0, false
		}
		// 復帰の置き場所: 内側の退避の後、内側の入口の写し (レジスタへの読み込み) があればその前、無ければ jmp の前
		if firstLoad >= 0 {
			return first, firstLoad, true
		}
		return first, o[len(o)-1], true
	}
	onEdge := func(from, to *ir.Block, mk []*ir.Op, spill bool) {
		if len(mk) == 0 {
			return
		}
		li := lastOp(from)
		last := (*ir.Op)(nil)
		if li >= 0 {
			last = ops[li]
		}
		if first, jump, ok := copyBlock(to); ok {
			// to は内側の領域がこの辺を分割して作った写しだけのブロック (写し; jmp 元の飛び先): 退避はその写しの前、
			// 復帰は後 (jmp の前) に置く (もう一度分割して手前にブロックを作ると、内側の g2@X の退避の前にこの領域の
			// g3@X の復帰が来て X が上書きされた。fuzz で発覚)
			pos := jump
			if spill {
				pos = first
			}
			before[pos] = append(before[pos], mk...)
			return
		}
		// pos の手前にある (内側の領域の入口の) 読み込みの写しの前へ。退避 (レジスタ → Home) は越えない: 内側の出口の辺を
		// 分割した写しだけのブロック (`内側の退避; この領域の復帰; jmp`) がこの領域の外への出口でもあるとき、書き戻しを
		// 内側の退避より前に置くと、X がまだ内側の変数を持っているのにそれをこの領域の変数の Home に書いていた
		// (`l1.lo = X (= l4)`。広げた生成器の fuzz で発覚。TestResidentExitThroughCopyBlock)
		backOver := func(pos int) int {
			for pos > 0 && isResCopy(ops[pos-1]) && !isResSpill(ops[pos-1]) {
				pos--
			}
			return pos
		}
		switch {
		case last != nil && last.Code == ir.OpJump && last.Label == to.Label:
			pos := li
			if spill {
				pos = backOver(pos)
			}
			before[pos] = append(before[pos], mk...)
		case last != nil && last.Code == ir.OpSwitch && to.Label != "" && slices.Contains(last.Labels, to.Label):
			// ジャンプテーブルの飛び先: 表の項目を新しいブロックに向ける
			l := newLabel()
			for k, x := range last.Labels {
				if x == to.Label {
					last.Labels[k] = l
				}
			}
			tail = append(tail, &ir.Op{Code: ir.OpLabel, Label: l})
			tail = append(tail, mk...)
			tail = append(tail, &ir.Op{Code: ir.OpJump, Label: to.Label})
		case last != nil && isCondBranch(last) && last.Label == to.Label:
			// 条件分岐の飛び先: 辺を分割して末尾に新しいブロック
			l := newLabel()
			last.Label = l
			tail = append(tail, &ir.Op{Code: ir.OpLabel, Label: l})
			tail = append(tail, mk...)
			tail = append(tail, &ir.Op{Code: ir.OpJump, Label: to.Label})
			if to.Index != from.Index+1 {
				break
			}
			// 落ちる先も同じブロック (`if c goto next`。simplifyJumps が消すが念のため): 落ちる側にも置く
			fallthrough
		default:
			// fallthrough (from の直後が to)。from の末尾に足す (条件分岐の落ちる側なら分岐の後 = to の手前)
			if li >= 0 {
				pos := li + 1
				if spill {
					// from の末尾にある内側のループの入口の写し (ラベルの前に置かれている) より前に退避する
					// (`tax` (内側の l0@X) の後に `stx` (この領域の l1) を出していた。fuzz で発覚)
					pos = backOver(pos)
				} else {
					// 復帰は、前の (内側の) 領域の退避の後、次の (内側の) 領域の入口の写し (レジスタへの読み込み) の前に
					// (`ldx l3` (次の領域の入口) の後に `ldx p1` (この領域の復帰) を出して、X が p1 のままループに
					// 入っていた。fuzz で発覚)
					for pos < len(ops) && isResSpill(ops[pos]) {
						pos++
					}
				}
				if pos < len(ops) {
					before[pos] = append(before[pos], mk...)
				} else {
					after[li] = append(after[li], mk...)
				}
			} else {
				pos := firstOp(to)
				if spill {
					pos = backOver(pos)
				}
				before[pos] = append(before[pos], mk...)
			}
		}
	}
	entryOps := func(at int) []*ir.Op {
		var r []*ir.Op
		if vA != nil && lv.LiveIn(at, vA) {
			r = append(r, &ir.Op{Code: ir.OpLoad, Dst: rA, Src: []ir.Operand{vA}, Resident: rA, ResOut: true})
		}
		if vY != nil && lv.LiveIn(at, vY) {
			r = append(r, &ir.Op{Code: ir.OpLoad, Dst: rY, Src: []ir.Operand{vY}, ResidentY: rY, ResYOut: true})
		}
		if vX != nil && lv.LiveIn(at, vX) {
			r = append(r, &ir.Op{Code: ir.OpLoad, Dst: rX, Src: []ir.Operand{vX}, ResidentX: rX, ResXOut: true})
		}
		return r
	}
	exitOps := func(at int) []*ir.Op { // 書き戻し (Clean なら Home が最新なので不要)
		var r []*ir.Op
		if vA != nil && lv.LiveIn(at, vA) && !rA.Clean {
			r = append(r, &ir.Op{Code: ir.OpLoad, Dst: vA, Src: []ir.Operand{rA}, Resident: rA, ResIn: true})
		}
		if vY != nil && lv.LiveIn(at, vY) && !rY.Clean {
			r = append(r, &ir.Op{Code: ir.OpLoad, Dst: vY, Src: []ir.Operand{rY}, ResidentY: rY, ResYIn: true})
		}
		if vX != nil && lv.LiveIn(at, vX) && !rX.Clean {
			r = append(r, &ir.Op{Code: ir.OpLoad, Dst: vX, Src: []ir.Operand{rX}, ResidentX: rX, ResXIn: true})
		}
		return r
	}
	for _, e := range r.entries() {
		from, to := e[0], e[1]
		onEdge(from, to, entryOps(firstOp(to)), false)
	}
	for _, e := range r.exits() {
		from, to := e[0], e[1]
		onEdge(from, to, exitOps(firstOp(to)), true)
	}
	// 関数全体の領域: 先頭で写す (return での書き戻しは退避として codegen が出す)
	if entry := cfg.Blocks[0]; r[entry] {
		at := firstOp(entry)
		before[at] = append(entryOps(at), before[at]...)
	}
	var out []*ir.Op
	for i, op := range ops {
		out = append(out, before[i]...)
		if op != nil {
			out = append(out, op)
		}
		out = append(out, after[i]...)
	}
	out = append(out, tail...)
	lmd.Ops = out
}

func isCondBranch(op *ir.Op) bool {
	switch op.Code {
	case ir.OpIf, ir.OpIfTrue, ir.OpIfCarry, ir.OpIfNotCarry:
		return true
	}
	return false
}
