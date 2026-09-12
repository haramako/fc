package regalloc

// レジスタ割付。lib/fc/allocator.rb 由来。
// use_define / register_vars は Ruby の Hash と同じく挿入順を保つ必要がある。
// ir.CastedValue は Delegator のため下位の ir.Value と同一キーに合流する (ir.UnderlyingValue)。

import (
	"fmt"
	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/ir"
)

// ---------------------------------------------------------------
// live range の計算
// ---------------------------------------------------------------

type useDefineEntry struct {
	v       *ir.Value
	defines []int
	uses    []int
}

// CalcLiveRange は Fc.calc_live_range 相当。lmd の各ローカル変数に live_range を設定する。
func CalcLiveRange(lmd *ir.Lambda) {
	// ラベルの収集
	labels := map[string]int{}
	for i, op := range lmd.Ops {
		if op != nil && op.Code == ir.OpLabel {
			labels[op.Label] = i
		}
	}

	// 変数の定義・使用、制御フローグラフの集計
	var udOrder []*useDefineEntry
	udIndex := map[*ir.Value]*useDefineEntry{}
	record := func(vAny ir.Operand, isDefine bool, i int) {
		if vAny == nil {
			return
		}
		if ir.ValKind(vAny) != ir.KindLocal {
			return
		}
		v := ir.UnderlyingValue(vAny)
		if v == nil {
			// ir.PointeredArray がローカル変数を包む場合、Ruby版は live_range 設定で
			// NoMethodError になる (実際には発生しない経路)
			panic(fmt.Sprintf("cannot record %T in use_define", vAny))
		}
		e := udIndex[v]
		if e == nil {
			e = &useDefineEntry{v: v}
			udIndex[v] = e
			udOrder = append(udOrder, e)
		}
		if isDefine {
			e.defines = append(e.defines, i)
		} else {
			e.uses = append(e.uses, i)
		}
	}

	flow := make([][]int, 0, len(lmd.Ops))
	for i, op := range lmd.Ops {
		var uses, defines []ir.Operand
		var node []int
		switch op.Code {
		case ir.OpLabel, ir.OpAsm, ir.OpPushResult, ir.OpPushFastcallResult:
			// DO NOTHING
		case ir.OpIf:
			uses = append(uses, op.Src[0])
			node = append(node, labels[op.Label])
		case ir.OpJump:
			node = append(node, labels[op.Label])
		case ir.OpReturn:
			uses = append(uses, op.In(0))
		case ir.OpPushArg, ir.OpPushFastcallArg:
			uses = append(uses, op.Src[0])
		case ir.OpLoad, ir.OpUminus, ir.OpNot, ir.OpSignExtension, ir.OpRef, ir.OpCall, ir.OpFastcall:
			defines = append(defines, op.Dst)
			uses = append(uses, op.Src[0])
		case ir.OpAdd, ir.OpSub, ir.OpAnd, ir.OpOr, ir.OpXor,
			ir.OpMul, ir.OpDiv, ir.OpMod, ir.OpEq, ir.OpLt,
			ir.OpShiftLeft, ir.OpShiftRight, ir.OpIndex, ir.OpPget:
			defines = append(defines, op.Dst)
			uses = append(uses, op.Src...)
		case ir.OpPset:
			uses = append(uses, op.Src[0], op.Src[1])
		default:
			panic(fmt.Sprintf("invalid op %v", ir.DumpOp(op, nil)))
		}

		flow = append(flow, node)

		for _, v := range defines {
			record(v, true, i)
		}
		for _, v := range uses {
			record(v, false, i)
		}
	}

	// 引数は、最初に定義されているものとする
	for _, e := range udOrder {
		if e.v.LocalType == ir.LTArg {
			e.defines = append(e.defines, 0)
		}
	}

	// live range を求める
	lrc := NewLiveRangeCalculator(flow)
	for _, e := range udOrder {
		e.v.LiveRange = lrc.CalcLiveRange(e.defines, e.uses)
	}
}

// ---------------------------------------------------------------
// レジスタ割付
// ---------------------------------------------------------------

// AllocateRegister は Fc.allocate_register 相当。
// 変数の address, location, unuse が設定される。
func AllocateRegister(lmd *ir.Lambda) {
	CalcLiveRange(lmd)

	// ref(&演算子)を受けた変数を集める
	refered := map[*ir.Value]bool{}
	for _, op := range lmd.Ops {
		if op != nil && op.Code == ir.OpRef {
			refered[ir.UnderlyingValue(op.Src[0])] = true
		}
	}

	// live range を求め、{callをまたぐ|refを受ける|引数}だったら、フレームスタック上に確保する
	frameSize := lmd.Type.Base.Size // 帰り値分を予約しておく
	var registerVars []*allocEntry  // 挿入順を保つ (Ruby の Hash 相当)
	fastcall := lmd.Type.Fastcall()
	for _, v := range lmd.Vars {
		beyondCall := false
		if v.LiveRange != nil {
			for i := v.LiveRange.Min + 1; i <= v.LiveRange.Max-1; i++ {
				if i >= 0 && i < len(lmd.Ops) && lmd.Ops[i] != nil &&
					lmd.Ops[i].Code == ir.OpCall && !ir.ValType(lmd.Ops[i].Src[0]).Fastcall() {
					beyondCall = true
				}
			}
		}

		if v.LocalType == ir.LTResult {
			// 返り値
			if fastcall {
				lmd.Result.Location = ir.LocFastcallReg
			} else {
				lmd.Result.Location = ir.LocFrame
			}
			lmd.Result.Address = 0
		} else if beyondCall || v.LocalType == ir.LTArg || refered[v] {
			// 引数か、関数をまたいでいるなら、フレームに割り当てる
			v.Address = frameSize
			if fastcall {
				v.Location = ir.LocFastcallReg
			} else {
				v.Location = ir.LocFrame
			}
			frameSize += v.Type.Size
		} else if v.LiveRange != nil {
			// それ以外の使われてる変数は、レジスターメモリに割り当てる
			registerVars = append(registerVars, &allocEntry{key: v, liveRange: v.LiveRange})
		} else {
			// 未使用フラグをたてる
			v.Location = ir.LocUnused
			v.Unuse = true
		}
	}

	registerVars = allocateCond(lmd, registerVars)
	registerVars = allocateA(lmd, registerVars)

	// 各レジスタのアドレスを割り当てる
	regSize := 0
	allocator := NewAllocator(registerVars)
	for _, reg := range allocator.Regs {
		// アドレスを算出する
		if regSize > 16 {
			panic(&diag.Error{Msg: fmt.Sprintf("frame size over on %s", lmd)})
		}
		// 割り当てる
		for _, v := range reg.vars {
			if fastcall {
				if frameSize+regSize > 16 {
					panic(&diag.Error{Msg: fmt.Sprintf("frame size over on %s", lmd)})
				}
				v.Location = ir.LocFastcallReg
				v.Address = frameSize + regSize
			} else {
				v.Location = ir.LocReg
				v.Address = regSize
			}
		}
		regSize += 2
	}

	lmd.FrameSize = frameSize
}

// allocateA は Aレジスタを割り当てられるなら割り当てる。
func allocateA(lmd *ir.Lambda, registerVars []*allocEntry) []*allocEntry {
	var aVars []*ir.Value
	for _, e := range registerVars {
		v := e.key
		if v.Type.Size != 1 {
			continue
		}
		if v.LiveRange.Max-v.LiveRange.Min == 1 {
			// Aレジスタを割り当てられる組み合わせでなければスルー
			op := lmd.Ops[v.LiveRange.Min]
			if !isSameValue(op.Dst, v) {
				continue
			}
			if !codeIn(op.Code, ir.OpLoad, ir.OpAdd, ir.OpSub, ir.OpAnd, ir.OpOr, ir.OpXor,
				ir.OpMul, ir.OpDiv, ir.OpMod, ir.OpUminus, ir.OpEq, ir.OpLt, ir.OpPget) {
				continue
			}

			nextOp := lmd.Ops[v.LiveRange.Min+1]
			switch nextOp.Code {
			case ir.OpLoad, ir.OpSignExtension, ir.OpAdd, ir.OpAnd, ir.OpOr, ir.OpXor,
				ir.OpEq, ir.OpLt, ir.OpPget, ir.OpSub, ir.OpPushArg,
				ir.OpIf, ir.OpReturn:
				// 最初の入力オペランドが v であること (旧実装の op[2] / if・return では op[1] に相当)
				if !isSameValue(nextOp.In(0), v) {
					continue
				}
			default:
				continue
			}

			aVars = append(aVars, v)
		}
	}
	for _, v := range aVars {
		v.Location = ir.LocA
		registerVars = deleteEntry(registerVars, v)
	}
	return registerVars
}

// allocateCond はコンディションレジスタを割り当てられるなら割り当てる。
func allocateCond(lmd *ir.Lambda, registerVars []*allocEntry) []*allocEntry {
	var condVars []*ir.Value
	for _, e := range registerVars {
		v := e.key
		if v.Type.Size != 1 {
			continue
		}
		if v.LiveRange.Max-v.LiveRange.Min == 1 {
			op := lmd.Ops[v.LiveRange.Min]
			if !isSameValue(op.Dst, v) {
				continue
			}
			if !codeIn(op.Code, ir.OpEq, ir.OpLt, ir.OpNot) {
				continue
			}

			nextOp := lmd.Ops[v.LiveRange.Min+1]
			switch nextOp.Code {
			case ir.OpIf, ir.OpNot:
				if !isSameValue(nextOp.In(0), v) {
					continue
				}
			default:
				continue
			}

			switch op.Code {
			case ir.OpEq:
				v.Location = ir.LocCond
				v.CondPositive = true
				v.CondReg = ir.CondZero
			case ir.OpLt:
				v.Location = ir.LocCond
				v.CondPositive = true
				if ir.ValType(op.Src[0]).Signed || ir.ValType(op.Src[1]).Signed {
					if ir.ValType(op.Src[0]).Size > 1 || ir.ValType(op.Src[1]).Size > 1 {
						// サイズ2以上の符号付き比較はフラグが特定できない。
						// Ruby版は next の前に location/cond_positive を設定済みのまま残す
						// (後で register_vars の割付により location は上書きされる)
						continue
					}
					v.CondReg = ir.CondNegative
				} else {
					v.CondReg = ir.CondCarry
				}
			case ir.OpNot:
				if ir.ValLocation(op.Src[0]) != ir.LocCond {
					continue
				}
				v.Location = ir.LocCond
				v.CondReg = ir.UnderlyingValue(op.Src[0]).CondReg
				v.CondPositive = !ir.UnderlyingValue(op.Src[0]).CondPositive
			default:
				panic("unreachable")
			}
			condVars = append(condVars, v)
		}
	}
	for _, v := range condVars {
		registerVars = deleteEntry(registerVars, v)
	}
	return registerVars
}

func isSameValue(opElem ir.Operand, v *ir.Value) bool {
	// Ruby の op[1] == v (ir.CastedValue は Delegator の == で from と比較される)
	return opElem != nil && ir.UnderlyingValue(opElem) == v
}

func codeIn(c ir.OpCode, codes ...ir.OpCode) bool {
	for _, x := range codes {
		if c == x {
			return true
		}
	}
	return false
}

func deleteEntry(entries []*allocEntry, v *ir.Value) []*allocEntry {
	r := entries[:0]
	for _, e := range entries {
		if e.key != v {
			r = append(r, e)
		}
	}
	return r
}

// ---------------------------------------------------------------
// レジスタアロケータ
// ---------------------------------------------------------------

type allocEntry struct {
	key       *ir.Value
	liveRange *ir.LiveRange
}

type AllocatorReg struct {
	liveRange *ir.LiveRange
	vars      []*ir.Value
}

type Allocator struct {
	Regs []*AllocatorReg
}

func NewAllocator(vars []*allocEntry) *Allocator {
	a := &Allocator{}
	for _, e := range vars {
		found := false
		for _, reg := range a.Regs {
			if !overlapRange(reg.liveRange, e.liveRange) {
				reg.liveRange = joinRange(reg.liveRange, e.liveRange)
				reg.vars = append(reg.vars, e.key)
				found = true
				break
			}
		}
		if !found {
			a.Regs = append(a.Regs, &AllocatorReg{liveRange: e.liveRange, vars: []*ir.Value{e.key}})
		}
	}
	return a
}

// allocRanges は live range の重ならないものを同じレジスタにまとめる (NewAllocator の中核。単体テスト用に分離)。
// 返り値は各レジスタに入るキーの index のリスト。
func allocRanges(ranges []*ir.LiveRange) [][]int {
	var regs [][]int
	var regRanges []*ir.LiveRange
	for i, lr := range ranges {
		found := false
		for j := range regs {
			if !overlapRange(regRanges[j], lr) {
				regRanges[j] = joinRange(regRanges[j], lr)
				regs[j] = append(regs[j], i)
				found = true
				break
			}
		}
		if !found {
			regs = append(regs, []int{i})
			regRanges = append(regRanges, lr)
		}
	}
	return regs
}

func overlapRange(r1, r2 *ir.LiveRange) bool {
	return r1.Max >= r2.Min && r1.Min <= r2.Max
}

func joinRange(r1, r2 *ir.LiveRange) *ir.LiveRange {
	return &ir.LiveRange{Min: min(r1.Min, r2.Min), Max: max(r1.Max, r2.Max)}
}

// ---------------------------------------------------------------
// Live Range の計算を行うクラス
// ---------------------------------------------------------------

type LiveRangeCalculator struct {
	flow [][2][]int // [後続節, 先行節]
}

// NewLiveRangeCalculator は flow (各opの後続節リスト) から構築する。
// 直後の節は自動的に後続節に追加される (最後の節を除く)。
func NewLiveRangeCalculator(flow [][]int) *LiveRangeCalculator {
	l := &LiveRangeCalculator{flow: make([][2][]int, len(flow))}
	for i, node := range flow {
		succ := append([]int{}, node...)
		if i < len(flow)-1 {
			succ = append(succ, i+1)
		}
		l.flow[i][0] = succ
	}
	for i := range l.flow {
		for _, succ := range l.flow[i][0] {
			l.flow[succ][1] = append(l.flow[succ][1], i)
		}
	}
	return l
}

// CalcLiveRange は live range を計算する。全く使われていない場合 nil を返す。
func (l *LiveRangeCalculator) CalcLiveRange(defines, uses []int) *ir.LiveRange {
	type liveInfo struct {
		define, use, liveIn, liveOut bool
	}
	lives := make([]liveInfo, len(l.flow))
	for _, i := range defines {
		lives[i].define = true
	}
	for _, i := range uses {
		lives[i].use = true
		lives[i].liveIn = true
	}
	for {
		finished := true
		for i := len(lives) - 1; i >= 0; i-- {
			// 入り口生存なら、先行節で出口生存
			if lives[i].liveIn {
				for _, pred := range l.flow[i][1] {
					if !lives[pred].liveOut {
						lives[pred].liveOut = true
						finished = false
					}
				}
			}
			// 出口生存で定義節でないなら、入り口生存
			if lives[i].liveOut && !lives[i].define {
				if !lives[i].liveIn {
					lives[i].liveIn = true
					finished = false
				}
			}
		}
		if finished {
			break
		}
	}
	// live range の算出
	maxI := 0
	minI := 1000000
	for i := range lives {
		if lives[i].liveIn || lives[i].liveOut {
			if i > maxI {
				maxI = i
			}
			if i < minI {
				minI = i
			}
		}
	}
	if minI == 1000000 {
		return nil
	}
	return &ir.LiveRange{Min: minI, Max: maxI}
}

// ---------------------------------------------------------------
// 使っていない変数の削除 (Ruby版では Llc#delete_unuse)
// ---------------------------------------------------------------

func DeleteUnuse(lmd *ir.Lambda) {
	for i, op := range lmd.Ops {
		if op == nil {
			continue
		}
		switch op.Code {
		case ir.OpPget, ir.OpLoad:
			if op.Dst != nil && ir.UnderlyingValue(op.Dst) != nil && ir.UnderlyingValue(op.Dst).Unuse {
				lmd.Ops[i] = nil
			}
		case ir.OpCall, ir.OpFastcall:
			if op.Dst != nil && ir.UnderlyingValue(op.Dst) != nil && ir.UnderlyingValue(op.Dst).Unuse {
				op.Dst = nil
			}
		}
	}
}
