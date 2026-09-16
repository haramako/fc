package regalloc

// ループ内のレジスタ常駐 (doc/v2_regalloc.md)。最内ループごとに 1 バイトのローカル変数を A に 1 つ、Y に 1 つまで選び、
// ループの中ではレジスタに置いたままにする。ループ内の命令は Classify で、A / Y それぞれについて
// 「レジスタのまま実行できる (friendly)」「触らず壊さない (free)」「壊すので前後で退避する (clobber)」に分かれ、
// コスト (得 − 退避の損) が最大の組を採用する。

import (
	"fmt"
	"os"
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

// Decision は命令 1 つの扱い (A と Y)。
type Decision struct {
	A, Y ResClass
	UseY bool // A が常駐変数で塞がっているので、この命令は Y で代用する (ldy / sty / cpy)
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
	case ir.LocA, ir.LocY, ir.LocCond:
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
			return true, 3
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

// friendlyY は v が Y に常駐しているとき、op を Y のまま実行できるか (添字と 1 バイトのカウンタの形)。
func friendlyY(lmd *ir.Lambda, i int, v *ir.Value) (bool, int) {
	op := lmd.Ops[i]
	switch op.Code {
	case ir.OpIndexPget:
		// 要素 1 バイトの配列 / ゼロページのポインタの添字 (ldy が消える)
		if isV(op.In(1), v) && !isV(op.Dst, v) && ir.ValType(op.In(0)).Base.Size == 1 {
			return true, 3
		}
	case ir.OpIndexPset:
		if isV(op.In(1), v) && !isV(op.In(2), v) && ir.ValType(op.In(0)).Base.Size == 1 {
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

// needsY は op の codegen が (常駐変数としてでなく) Y を作業用に使うか。
func needsY(op *ir.Op, vY *ir.Value) bool {
	switch op.Code {
	case ir.OpPget, ir.OpPset, ir.OpFieldPget, ir.OpFieldPset, ir.OpIndex,
		ir.OpMul, ir.OpDiv, ir.OpMod, ir.OpCall, ir.OpFastcall, ir.OpAsm:
		return true
	case ir.OpIndexPget, ir.OpIndexPset:
		return !isV(op.In(1), vY) || ir.ValType(op.In(0)).Base.Size != 1
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

// Classify は vA が A に、vY が Y に常駐しているときの lmd.Ops[i] の扱い (どちらも nil 可)。
// aLive / yLive はその命令の入口または出口で変数が生きている (レジスタが塞がっている) か。
// 戻り値の gain は friendly で節約できるサイクル数の目安 (A と Y の合計)。
func Classify(lmd *ir.Lambda, i int, vA, vY *ir.Value, aLive, aOut, yLive bool) (Decision, int) {
	op := lmd.Ops[i]
	d := Decision{A: ResFree, Y: ResFree}
	gain := 0
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
		} else if !freeA(op) && !(yFriendly && aFreeWithY(op)) {
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

// AllocateResident は最内ループごとに A / Y に常駐させる変数を選んで IR を書き換える (opt の後、AllocateRegister の前)。
// 調査用: 環境変数 FC_NO_RESIDENT で無効化、FC_TRACE_RESIDENT で選んだ変数と見積もりを stderr に出す。
func AllocateResident(lmd *ir.Lambda) {
	if os.Getenv("FC_NO_RESIDENT") != "" {
		return
	}
	for iter := 0; iter < 16; iter++ {
		cfg := ir.BuildCFG(lmd)
		loops := ir.Innermost(cfg.Loops())
		lv := ir.BuildLiveness(lmd)
		done := false
		for _, lp := range loops {
			if hasResident(lmd, cfg, lp) {
				continue
			}
			vA, vY, gain := bestPair(lmd, cfg, lp, lv)
			if vA == nil && vY == nil {
				continue
			}
			if os.Getenv("FC_TRACE_RESIDENT") != "" {
				fmt.Fprintf(os.Stderr, "resident: %s loop %s: A=%s Y=%s (gain %d/iter)\n", lmd.Id, lp.Header.Label, name(vA), name(vY), gain)
			}
			makeResident(lmd, cfg, lp, lv, vA, vY)
			done = true
			break // IR が変わったので作り直す
		}
		if !done {
			return
		}
	}
}

func name(v *ir.Value) string {
	if v == nil {
		return "-"
	}
	return v.Name
}

func hasResident(lmd *ir.Lambda, cfg *ir.CFG, lp *ir.Loop) bool {
	for b := range lp.Blocks {
		for _, i := range cfg.Ops(b) {
			if lmd.Ops[i].Resident != nil || lmd.Ops[i].ResidentY != nil {
				return true
			}
		}
	}
	return false
}

// candidates はループ内で使われる 1 バイトのローカル変数 (アドレスを取られたもの・結果・一時変数・配列は除く)。
func candidates(lmd *ir.Lambda, cfg *ir.CFG, lp *ir.Loop) []*ir.Value {
	refered := map[*ir.Value]bool{}
	for _, op := range lmd.Ops {
		if op != nil && op.Code == ir.OpRef {
			refered[ir.UnderlyingValue(op.Src[0])] = true
		}
	}
	var r []*ir.Value
	seen := map[*ir.Value]bool{}
	for b := range lp.Blocks {
		for _, i := range cfg.Ops(b) {
			defs, uses := ir.DefUse(lmd.Ops[i])
			for _, o := range append(append([]ir.Operand{}, defs...), uses...) {
				if _, ok := o.(*ir.PointeredArray); ok {
					continue
				}
				v := ir.UnderlyingValue(o)
				if v == nil || v.Kind != ir.KindLocal || seen[v] || v.Type.Size != 1 || v.Type.Kind != types.Int ||
					v.LocalType == ir.LTResult || v.LocalType == ir.LTTemp || refered[v] || v.Location == ir.LocA || v.Location == ir.LocY {
					continue // 一時変数は定義の直後に使うものが大半で、既存の A 割付 (allocateA) が扱う
				}
				seen[v] = true
				r = append(r, v)
			}
		}
	}
	return r
}

// gainOf は (vA, vY) の組をループに常駐させたときの 1 周あたりの正味の得。
func gainOf(lmd *ir.Lambda, cfg *ir.CFG, lp *ir.Loop, lv *ir.Liveness, vA, vY *ir.Value) int {
	gain := 0
	for b := range lp.Blocks {
		for _, i := range cfg.Ops(b) {
			op := lmd.Ops[i]
			aIn, aOut := vA != nil && lv.LiveIn(i, vA), vA != nil && lv.LiveOut(i, vA)
			yIn, yOut := vY != nil && lv.LiveIn(i, vY), vY != nil && lv.LiveOut(i, vY)
			if !aIn && !aOut && !yIn && !yOut && !involves(op, vA) && !involves(op, vY) {
				continue
			}
			d, g := Classify(lmd, i, vA, vY, aIn || aOut, aOut, yIn || yOut)
			gain += g
			if d.A == ResClobber {
				if aIn {
					gain -= 3
				}
				if aOut {
					gain -= 3
				}
			}
			if d.Y == ResClobber {
				if yIn {
					gain -= 3
				}
				if yOut {
					gain -= 3
				}
			}
		}
	}
	return gain
}

// bestPair は正味の得 (1 周あたり) が最大の (A, Y) の組。入口 / 出口の写しがあるので 3 サイクル以上の得を要求する。
func bestPair(lmd *ir.Lambda, cfg *ir.CFG, lp *ir.Loop, lv *ir.Liveness) (*ir.Value, *ir.Value, int) {
	cands := candidates(lmd, cfg, lp)
	var bestA, bestY *ir.Value
	best := 2
	try := func(vA, vY *ir.Value) {
		if g := gainOf(lmd, cfg, lp, lv, vA, vY); g > best {
			bestA, bestY, best = vA, vY, g
		}
	}
	for _, a := range cands {
		try(a, nil)
		try(nil, a)
		for _, y := range cands {
			if y != a {
				try(a, y)
			}
		}
	}
	return bestA, bestY, best
}

// makeResident はループ内の vA / vY を常駐の一時変数に置き換え、入口 / 出口の辺に写す命令を挿す。
func makeResident(lmd *ir.Lambda, cfg *ir.CFG, lp *ir.Loop, lv *ir.Liveness, vA, vY *ir.Value) {
	ops := lmd.Ops
	var rA, rY *ir.Value
	if vA != nil {
		rA = ir.NewLocal(vA.Name+"@A", vA.Type, ir.LTTemp)
		rA.Location, rA.Home = ir.LocA, vA
		lmd.Vars = append(lmd.Vars, rA)
	}
	if vY != nil {
		rY = ir.NewLocal(vY.Name+"@Y", vY.Type, ir.LTTemp)
		rY.Location, rY.Home = ir.LocY, vY
		lmd.Vars = append(lmd.Vars, rY)
	}
	replace := func(o ir.Operand) ir.Operand {
		if isV(o, vA) {
			return rA
		}
		if isV(o, vY) {
			return rY
		}
		return o
	}
	for b := range lp.Blocks {
		for _, i := range cfg.Ops(b) {
			op := ops[i]
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
	// 辺 (from → to) に命令を挿す。to は from の直後 (fallthrough)、jump の飛び先、または条件分岐の飛び先
	onEdge := func(from, to *ir.Block, mk []*ir.Op) {
		if len(mk) == 0 {
			return
		}
		li := lastOp(from)
		last := (*ir.Op)(nil)
		if li >= 0 {
			last = ops[li]
		}
		switch {
		case last != nil && last.Code == ir.OpJump && last.Label == to.Label:
			before[li] = append(before[li], mk...)
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
				after[li] = append(after[li], mk...)
			} else {
				before[firstOp(to)] = append(before[firstOp(to)], mk...)
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
		return r
	}
	exitOps := func(at int) []*ir.Op {
		var r []*ir.Op
		if vA != nil && lv.LiveIn(at, vA) {
			r = append(r, &ir.Op{Code: ir.OpLoad, Dst: vA, Src: []ir.Operand{rA}, Resident: rA, ResIn: true})
		}
		if vY != nil && lv.LiveIn(at, vY) {
			r = append(r, &ir.Op{Code: ir.OpLoad, Dst: vY, Src: []ir.Operand{rY}, ResidentY: rY, ResYIn: true})
		}
		return r
	}
	for _, p := range cfg.Entries(lp) {
		onEdge(p, lp.Header, entryOps(firstOp(lp.Header)))
	}
	for _, e := range cfg.Exits(lp) {
		from, to := e[0], e[1]
		onEdge(from, to, exitOps(firstOp(to)))
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
