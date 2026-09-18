package regalloc

// ループ内のレジスタ常駐 (doc/v2_regalloc.md)。最内ループごとに 1 バイトのローカル変数を A に 1 つ、Y に 1 つまで選び、
// ループの中ではレジスタに置いたままにする。ループ内の命令は Classify で、A / Y それぞれについて
// 「レジスタのまま実行できる (friendly)」「触らず壊さない (free)」「壊すので前後で退避する (clobber)」に分かれ、
// コスト (得 − 退避の損) が最大の組を採用する。

import (
	"fmt"
	"os"
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
	if op.Code != ir.OpAdd && op.Code != ir.OpSub || len(op.Src) != 2 {
		return false
	}
	k, lit := ir.ValIntLiteral(op.Src[1])
	if !lit || k != 1 || ir.ValType(op.Dst).Size > 2 {
		return false
	}
	d, s := ir.UnderlyingValue(op.Dst), ir.UnderlyingValue(op.Src[0])
	return d != nil && d == s && ir.ValOffset(op.Dst) == ir.ValOffset(op.Src[0]) && ir.ValType(op.Dst).Size == ir.ValType(op.Src[0]).Size
}

// isMemShift は codegen の shiftInMemory と同じ条件 (定数シフト、2 バイト以下、1 バイトなら x = x << n の形)。
func isMemShift(op *ir.Op) bool {
	if op.Code != ir.OpShiftLeft && op.Code != ir.OpShiftRight {
		return false
	}
	if _, lit := ir.ValIntLiteral(op.Src[1]); !lit {
		return false
	}
	size := ir.ValType(op.Dst).Size
	if size > 2 || ir.ValKind(op.Dst) == ir.KindLiteral {
		return false
	}
	if size == 1 {
		d, s := ir.UnderlyingValue(op.Dst), ir.UnderlyingValue(op.Src[0])
		return d != nil && d == s && ir.ValOffset(op.Dst) == ir.ValOffset(op.Src[0])
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
	case ir.OpIf, ir.OpIfTrue:
		if isV(op.Src[0], v) {
			return true, 1 // lda v (3) → cmp #0 (2)
		}
	case ir.OpEq, ir.OpLt:
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
	case ir.OpPushArg, ir.OpPushFastcallArg, ir.OpReturn:
		if isV(op.In(0), v) {
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
		if isIncDec(op) && isV(op.Dst, v) {
			return true, 3 // inc x (5) → iny (2)
		}
	case ir.OpIf, ir.OpIfTrue:
		if isV(op.Src[0], v) {
			return true, 2 // lda v; bne → cpy #0; bne (直前が iny / dey ならピープホールが cpy を消す)
		}
	case ir.OpEq, ir.OpLt:
		if isV(op.Src[0], v) && condPredicted(lmd, i) && isMemOrLit(op.Src[1]) && ir.ValType(op.Src[1]).Size == 1 &&
			(op.Code == ir.OpEq || (!ir.ValType(op.Src[0]).Signed && !ir.ValType(op.Src[1]).Signed)) {
			return true, 3 // lda v; cmp k → cpy k
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
		if isIncDec(op) && isV(op.Dst, v) {
			return true, 3 // inx / dex
		}
	case ir.OpIf, ir.OpIfTrue:
		if isV(op.Src[0], v) {
			return true, 2 // cpx #0
		}
	case ir.OpEq, ir.OpLt:
		if isV(op.Src[0], v) && condPredicted(lmd, i) && isMemOrLit(op.Src[1]) && ir.ValType(op.Src[1]).Size == 1 &&
			(op.Code == ir.OpEq || (!ir.ValType(op.Src[0]).Signed && !ir.ValType(op.Src[1]).Signed)) {
			return true, 3 // cpx k
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
		ir.OpReturn, ir.OpMul, ir.OpDiv, ir.OpMod, ir.OpAsm:
		return true
	}
	return false
}

// needsY は op の codegen が (常駐変数としてでなく) Y を作業用に使うか。
func needsY(op *ir.Op, vY *ir.Value) bool {
	switch op.Code {
	case ir.OpPget, ir.OpPset, ir.OpFieldPget, ir.OpFieldPset, ir.OpIndex,
		ir.OpMul, ir.OpDiv, ir.OpMod, ir.OpCall, ir.OpFastcall, ir.OpAsm, ir.OpReturn: // return: グローバルの書き戻しのため
		return true
	case ir.OpIndexPget, ir.OpIndexPset:
		return !isV(op.In(1), vY) || !byteIndex(op)
	case ir.OpShiftLeft, ir.OpShiftRight:
		_, lit := ir.ValIntLiteral(op.Src[1])
		return !lit
	}
	return false
}

// freeA は op が A を使わずに実行できるか (Y での代用を除く)。
func freeA(op *ir.Op) bool {
	switch op.Code {
	case ir.OpLabel, ir.OpJump, ir.OpIfCarry, ir.OpIfNotCarry, ir.OpPushResult, ir.OpPushFastcallResult:
		return true
	case ir.OpAdd, ir.OpSub:
		return isIncDec(op)
	case ir.OpShiftLeft, ir.OpShiftRight:
		return isMemShift(op)
	case ir.OpIf, ir.OpIfTrue:
		// コンディションの一時変数なら分岐だけ
		return ir.ValLocalType(op.Src[0]) == ir.LTTemp && !isMemByte(op.Src[0])
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
		return condPredicted(lmd, i) && isMemOrLit(op.Src[0]) && isMemOrLit(op.Src[1]) &&
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
	// X (inx / cpx / ldx / stx は A も Y も使わない。lda a,x は A を使う)
	xFriendly := false
	if vX != nil {
		if involves(op, vX) {
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
	if vY != nil && involves(op, vY) {
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
		if involves(op, vA) {
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
	if os.Getenv("FC_NO_RESIDENT") != "" {
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
		if !done && !funcTried {
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
func (r region) entries() [][2]*ir.Block {
	var e [][2]*ir.Block
	for _, b := range r.blocks() {
		for _, p := range b.Preds {
			if !r[p] {
				e = append(e, [2]*ir.Block{p, b})
			}
		}
	}
	return e
}

func (r region) exits() [][2]*ir.Block {
	var e [][2]*ir.Block
	for _, b := range r.blocks() {
		for _, s := range b.Succs {
			if !r[s] {
				e = append(e, [2]*ir.Block{b, s})
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
			if d.X == ResClobber {
				if xIn && !cleanX {
					gain -= 3
				}
				if xOut {
					gain -= 3
				}
			}
			if d.A == ResClobber {
				if aIn && !cleanA {
					gain -= 3
				}
				if aOut {
					gain -= 3
				}
			}
			if d.Y == ResClobber {
				if yIn && !cleanY {
					gain -= 3
				}
				if yOut {
					gain -= 3
				}
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
	replace := func(o ir.Operand) ir.Operand {
		if isV(o, vA) {
			return rA
		}
		if isV(o, vY) {
			return rY
		}
		if isV(o, vX) {
			return rX
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
	onEdge := func(from, to *ir.Block, mk []*ir.Op, spill bool) {
		if len(mk) == 0 {
			return
		}
		li := lastOp(from)
		last := (*ir.Op)(nil)
		if li >= 0 {
			last = ops[li]
		}
		backOver := func(pos int) int { // pos の手前にある写しの前へ
			for pos > 0 && isResCopy(ops[pos-1]) {
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
		case last != nil && isCondBranch(last) && last.Label == to.Label && to.Index != from.Index+1:
			// 条件分岐の飛び先: 辺を分割して末尾に新しいブロック
			l := newLabel()
			last.Label = l
			tail = append(tail, &ir.Op{Code: ir.OpLabel, Label: l})
			tail = append(tail, mk...)
			tail = append(tail, &ir.Op{Code: ir.OpJump, Label: to.Label})
		default:
			// fallthrough (from の直後が to)。from の末尾に足す (条件分岐の落ちる側なら分岐の後 = to の手前)
			if li >= 0 {
				pos := li + 1
				if !spill {
					for pos < len(ops) && isResCopy(ops[pos]) {
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
