package regalloc

// ループ内の A 常駐 (doc/v2_regalloc.md)。最内ループごとに 1 バイトのローカル変数を 1 つ選び、ループの中では
// A に置いたままにする。ループ内の命令は ResidentClass で「A のまま実行できる (friendly)」「A を使わない (afree)」
// 「A を壊すので前後で退避する (clobber)」に分かれ、コスト (得 − 退避の損) が正の候補を採用する。

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/types"
)

// ResClass は A 常駐中の命令の扱い。
type ResClass uint8

const (
	ResClobber  ResClass = iota // A を壊す: v が live-in なら sta home、live-out なら lda home で挟み、命令は v をメモリ (home) として出す
	ResFriendly                 // v を A のまま扱える (loadA / storeA が何も出さない)
	ResFree                     // v を触らず A も壊さない (inc / dec、メモリ上のシフト、Y で代用する load / if / 比較、分岐)
)

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
	if k == ir.KindLocal && (ir.ValLocation(o) == ir.LocA || ir.ValLocation(o) == ir.LocCond) {
		return false // 割付後: A / フラグにある一時変数はメモリではない (codegen が分類し直すときに効く)
	}
	return k == ir.KindLocal || k == ir.KindGlobal
}

// isMemOrLit は Y 経由で写せる値 (メモリ上、または即値) か。
func isMemOrLit(o ir.Operand) bool {
	if o == nil {
		return false
	}
	if _, ok := o.(*ir.PointeredArray); ok {
		return false
	}
	if ir.ValKind(o) == ir.KindLocal && (ir.ValLocation(o) == ir.LocA || ir.ValLocation(o) == ir.LocCond) {
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
	return o != nil && ir.UnderlyingValue(o) == v && ir.ValOffset(o) == 0 && ir.ValType(o).Size == 1
}

// ResidentClass は v (1 バイト) が A に常駐しているときの lmd.Ops[i] の扱いと、friendly なら節約できるサイクル数の目安。
// liveOut は命令の後でも v が生きているか (結果が別の変数のとき、A を v のままにできるかに効く)。
func ResidentClass(lmd *ir.Lambda, i int, v *ir.Value, liveOut bool) (ResClass, int) {
	op := lmd.Ops[i]
	if !involves(op, v) {
		switch op.Code {
		case ir.OpLabel, ir.OpJump, ir.OpIfCarry, ir.OpIfNotCarry, ir.OpPushResult, ir.OpPushFastcallResult:
			return ResFree, 0
		case ir.OpAdd, ir.OpSub:
			if isIncDec(op) {
				return ResFree, 0
			}
		case ir.OpShiftLeft, ir.OpShiftRight:
			if isMemShift(op) {
				return ResFree, 0
			}
		case ir.OpLoad:
			// ldy / sty で写す (1〜2 バイト)
			if isMemOrLit(op.Src[0]) && isMemOrLit(op.Dst) && ir.ValType(op.Dst).Size <= 2 && ir.ValType(op.Src[0]).Kind != types.Array {
				return ResFree, 0
			}
		case ir.OpIf, ir.OpIfTrue:
			// 1 バイトのメモリ上の値なら ldy; bne。コンディションの一時変数なら分岐だけ
			if isMemByte(op.Src[0]) || ir.ValLocalType(op.Src[0]) == ir.LTTemp {
				return ResFree, 0
			}
		case ir.OpEq, ir.OpLt:
			// 1 バイト同士の比較でコンディションに落ちるなら ldy; cpy (符号付き lt は sbc が要るので不可)
			if condPredicted(lmd, i) && isMemOrLit(op.Src[0]) && isMemOrLit(op.Src[1]) &&
				ir.ValType(op.Src[0]).Size == 1 && ir.ValType(op.Src[1]).Size == 1 &&
				(op.Code == ir.OpEq || (!ir.ValType(op.Src[0]).Signed && !ir.ValType(op.Src[1]).Signed)) {
				return ResFree, 0
			}
		}
		return ResClobber, 0
	}
	// v を扱う命令
	switch op.Code {
	case ir.OpLoad:
		if isV(op.Dst, v) && isMemOrLit(op.Src[0]) && ir.ValType(op.Src[0]).Size == 1 {
			return ResFriendly, 3 // lda x (sta v が消える)
		}
		if isV(op.Src[0], v) && isMemByte(op.Dst) {
			return ResFriendly, 3 // sta x (lda v が消える)
		}
	case ir.OpAdd, ir.OpSub, ir.OpAnd, ir.OpOr, ir.OpXor:
		if isIncDec(op) && isV(op.Dst, v) {
			return ResFriendly, 1 // inc x (5) → clc; adc #1 (4)
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
			return ResFriendly, 6 // lda v … sta v が消える
		}
		if !liveOut && isMemByte(op.Dst) {
			return ResFriendly, 3
		}
	case ir.OpShiftLeft, ir.OpShiftRight:
		if _, lit := ir.ValIntLiteral(op.Src[1]); lit && isV(op.Src[0], v) && (isV(op.Dst, v) || (!liveOut && isMemByte(op.Dst))) {
			return ResFriendly, 3 // asl a (2) vs asl mem (5)
		}
	case ir.OpUminus, ir.OpBitNot:
		if isV(op.Src[0], v) && isV(op.Dst, v) {
			return ResFriendly, 3
		}
	case ir.OpIf, ir.OpIfTrue:
		if isV(op.Src[0], v) {
			return ResFriendly, 1 // lda v (3) → cmp #0 (2)
		}
	case ir.OpEq, ir.OpLt:
		if isV(op.Src[0], v) && condPredicted(lmd, i) && isMemOrLit(op.Src[1]) && ir.ValType(op.Src[1]).Size == 1 {
			return ResFriendly, 3
		}
	case ir.OpIndexPget:
		if isV(op.Dst, v) && ir.ValType(op.In(1)).Size == 1 {
			return ResFriendly, 3
		}
		if isV(op.In(1), v) && !isV(op.Dst, v) && !liveOut {
			return ResFriendly, 1 // tay
		}
	case ir.OpIndexPset:
		if isV(op.In(2), v) && ir.ValType(op.In(1)).Size == 1 && !isV(op.In(1), v) {
			return ResFriendly, 3
		}
	case ir.OpPset:
		if isV(op.In(1), v) {
			return ResFriendly, 3
		}
	case ir.OpFieldPset:
		if isV(op.In(2), v) {
			return ResFriendly, 3
		}
	case ir.OpPushArg, ir.OpPushFastcallArg, ir.OpReturn:
		if isV(op.In(0), v) {
			return ResFriendly, 3
		}
	}
	return ResClobber, 0
}

// AllocateResident は最内ループごとに A に常駐させる変数を選んで IR を書き換える (opt の後、AllocateRegister の前)。
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
			if v, gain := bestCandidate(lmd, cfg, lp, lv); v != nil {
				if os.Getenv("FC_TRACE_RESIDENT") != "" {
					fmt.Fprintf(os.Stderr, "resident: %s loop %s: %s (gain %d/iter)\n", lmd.Id, lp.Header.Label, v.Name, gain)
				}
				makeResident(lmd, cfg, lp, lv, v)
				done = true
				break // IR が変わったので作り直す
			}
		}
		if !done {
			return
		}
	}
}

func hasResident(lmd *ir.Lambda, cfg *ir.CFG, lp *ir.Loop) bool {
	for b := range lp.Blocks {
		for _, i := range cfg.Ops(b) {
			if lmd.Ops[i].Resident != nil {
				return true
			}
		}
	}
	return false
}

// candidates はループ内で使われる 1 バイトのローカル変数 (アドレスを取られたもの・結果・配列は除く)。
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
					v.LocalType == ir.LTResult || v.LocalType == ir.LTTemp || refered[v] || v.Location == ir.LocA {
					continue // 一時変数は定義の直後に使うものが大半で、既存の A 割付 (allocateA) が扱う
				}
				seen[v] = true
				r = append(r, v)
			}
		}
	}
	return r
}

// bestCandidate は正味の得 (1 周あたりのサイクル数) が最大の候補。入口 / 出口の写し (ループ 1 回あたり数サイクル) が
// あるので、周回数が少ないループで損しないように 1 周あたり 3 サイクル以上の得を要求する。
func bestCandidate(lmd *ir.Lambda, cfg *ir.CFG, lp *ir.Loop, lv *ir.Liveness) (*ir.Value, int) {
	var best *ir.Value
	bestGain := 2
	for _, v := range candidates(lmd, cfg, lp) {
		gain := 0
		for b := range lp.Blocks {
			for _, i := range cfg.Ops(b) {
				in, out := lv.LiveIn(i, v), lv.LiveOut(i, v)
				if !in && !out && !involves(lmd.Ops[i], v) {
					continue
				}
				class, save := ResidentClass(lmd, i, v, out)
				switch class {
				case ResFriendly:
					gain += save
				case ResClobber:
					if in {
						gain -= 3
					}
					if out {
						gain -= 3
					}
				}
			}
		}
		if gain > bestGain {
			best, bestGain = v, gain
		}
	}
	return best, bestGain
}

// makeResident はループ内の v を A 常駐の一時変数 vA に置き換え、入口 / 出口の辺に写す命令を挿す。
func makeResident(lmd *ir.Lambda, cfg *ir.CFG, lp *ir.Loop, lv *ir.Liveness, v *ir.Value) {
	vA := ir.NewLocal(v.Name+"@A", v.Type, ir.LTTemp)
	vA.Location = ir.LocA
	vA.Home = v
	lmd.Vars = append(lmd.Vars, vA)
	ops := lmd.Ops
	replace := func(o ir.Operand) ir.Operand {
		if isV(o, v) {
			return vA
		}
		return o
	}
	inLoop := make([]bool, len(ops))
	for b := range lp.Blocks {
		for _, i := range cfg.Ops(b) {
			inLoop[i] = true
			op := ops[i]
			// 可換な演算で v が第 2 入力なら第 1 に
			switch op.Code {
			case ir.OpAdd, ir.OpAnd, ir.OpOr, ir.OpXor:
				if isV(op.Src[1], v) && !isV(op.Src[0], v) {
					op.Src[0], op.Src[1] = op.Src[1], op.Src[0]
				}
			}
			op.Resident = vA
			op.ResIn, op.ResOut = lv.LiveIn(i, v), lv.LiveOut(i, v)
			for k := range op.Src {
				op.Src[k] = replace(op.Src[k])
			}
			if op.Dst != nil {
				op.Dst = replace(op.Dst)
			}
		}
	}
	// 入口 / 出口の写し。挿入は添字が変わらないように「位置 → 前に挿す命令」を集めてから 1 度に作り直す
	before := map[int][]*ir.Op{} // ops[i] の前に
	after := map[int][]*ir.Op{}  // ops[i] の後に
	var tail []*ir.Op            // 末尾に足すブロック (辺の分割)
	entryOp := func() *ir.Op {
		return &ir.Op{Code: ir.OpLoad, Dst: vA, Src: []ir.Operand{v}, Resident: vA, ResOut: true}
	}
	exitOp := func() *ir.Op {
		return &ir.Op{Code: ir.OpLoad, Dst: v, Src: []ir.Operand{vA}, Resident: vA, ResIn: true}
	}
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
	onEdge := func(from, to *ir.Block, mk func() *ir.Op) {
		li := lastOp(from)
		last := (*ir.Op)(nil)
		if li >= 0 {
			last = ops[li]
		}
		switch {
		case last != nil && last.Code == ir.OpJump && last.Label == to.Label:
			before[li] = append(before[li], mk())
		case last != nil && isCondBranch(last) && last.Label == to.Label && to.Index != from.Index+1:
			// 条件分岐の飛び先: 辺を分割して末尾に新しいブロック
			l := newLabel()
			last.Label = l
			tail = append(tail, &ir.Op{Code: ir.OpLabel, Label: l}, mk(), &ir.Op{Code: ir.OpJump, Label: to.Label})
		default:
			// fallthrough (from の直後が to)。from の末尾に足す (条件分岐の落ちる側なら分岐の後 = to の手前)
			if li >= 0 {
				after[li] = append(after[li], mk())
			} else {
				before[firstOp(to)] = append(before[firstOp(to)], mk())
			}
		}
	}
	for _, p := range cfg.Entries(lp) {
		if lv.LiveIn(firstOp(lp.Header), v) {
			onEdge(p, lp.Header, entryOp)
		}
	}
	for _, e := range cfg.Exits(lp) {
		from, to := e[0], e[1]
		if lv.LiveIn(firstOp(to), v) {
			onEdge(from, to, exitOp)
		}
	}
	var out []*ir.Op
	for i, op := range ops {
		out = append(out, before[i]...)
		if op != nil {
			out = append(out, op)
		}
		out = append(out, after[i]...)
	}
	// 末尾のブロックの前に return が無いと落ちてくるので、末尾の分割ブロックは必ず jump で終わる (上で付けている)
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
