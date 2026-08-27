package fc

// lib/fc/allocator.rb の厳密移植。
// use_define / register_vars は Ruby の Hash と同じく挿入順を保つ必要がある。
// CastedValue は Delegator のため下位の Value と同一キーに合流する (UnderlyingValue)。

import "fmt"

// ---------------------------------------------------------------
// live range の計算
// ---------------------------------------------------------------

type useDefineEntry struct {
	v       *Value
	defines []int
	uses    []int
}

// CalcLiveRange は Fc.calc_live_range 相当。lmd の各ローカル変数に live_range を設定する。
func CalcLiveRange(lmd *Lambda) {
	// ラベルの収集
	labels := map[string]int{}
	for i, op := range lmd.Ops {
		if op != nil && eqAny(op[0], Sym("label")) {
			labels[op[1].(string)] = i
		}
	}

	// 変数の定義・使用、制御フローグラフの集計
	var udOrder []*useDefineEntry
	udIndex := map[*Value]*useDefineEntry{}
	record := func(vAny any, isDefine bool, i int) {
		if vAny == nil {
			return
		}
		if ValKind(vAny) != "local" {
			return
		}
		v := UnderlyingValue(vAny)
		if v == nil {
			// PointeredArray がローカル変数を包む場合、Ruby版は live_range 設定で
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
		var uses, defines []any
		var node []int
		switch op[0] {
		case Sym("label"), Sym("asm"), Sym("push_result"), Sym("push_fastcall_result"):
			// DO NOTHING
		case Sym("if"):
			uses = append(uses, op[1])
			node = append(node, labels[op[2].(string)])
		case Sym("jump"):
			node = append(node, labels[op[1].(string)])
		case Sym("return"):
			uses = append(uses, at(op, 1))
		case Sym("push_arg"), Sym("push_fastcall_arg"):
			uses = append(uses, op[2])
		case Sym("load"), Sym("uminus"), Sym("not"), Sym("sign_extension"), Sym("ref"), Sym("call"), Sym("fastcall"):
			defines = append(defines, op[1])
			uses = append(uses, op[2])
		case Sym("add"), Sym("sub"), Sym("and"), Sym("or"), Sym("xor"),
			Sym("mul"), Sym("div"), Sym("mod"), Sym("eq"), Sym("lt"),
			Sym("shift_left"), Sym("shift_right"), Sym("index"), Sym("pget"):
			// pget は op[3] が存在しない (Ruby では nil になり後段で除外される)
			defines = append(defines, op[1])
			uses = append(uses, at(op, 2), at(op, 3))
		case Sym("pset"):
			uses = append(uses, op[1], op[2])
		default:
			panic(fmt.Sprintf("invalid op %v", dumpOp(op, nil)))
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
		if eqAny(e.v.Opt.GetOr(Sym("local_type")), Sym("arg")) {
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
func AllocateRegister(lmd *Lambda) {
	CalcLiveRange(lmd)

	// ref(&演算子)を受けた変数を集める
	refered := map[*Value]bool{}
	for _, op := range lmd.Ops {
		if op != nil && eqAny(op[0], Sym("ref")) {
			refered[UnderlyingValue(op[2])] = true
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
					eqAny(lmd.Ops[i][0], Sym("call")) && !ValType(lmd.Ops[i][2]).Fastcall() {
					beyondCall = true
				}
			}
		}

		if eqAny(v.Opt.GetOr(Sym("local_type")), Sym("result")) {
			// 返り値
			if fastcall {
				lmd.Result.Location = "fastcall_reg"
			} else {
				lmd.Result.Location = "frame"
			}
			lmd.Result.Address = 0
		} else if beyondCall || eqAny(v.Opt.GetOr(Sym("local_type")), Sym("arg")) || refered[v] {
			// 引数か、関数をまたいでいるなら、フレームに割り当てる
			v.Address = frameSize
			if fastcall {
				v.Location = "fastcall_reg"
			} else {
				v.Location = "frame"
			}
			frameSize += v.Type.Size
		} else if v.LiveRange != nil {
			// それ以外の使われてる変数は、レジスターメモリに割り当てる
			registerVars = append(registerVars, &allocEntry{key: v, liveRange: v.LiveRange})
		} else {
			// 未使用フラグをたてる
			v.Location = "none"
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
			panic(&CompileError{Msg: fmt.Sprintf("frame size over on %s", lmd)})
		}
		// 割り当てる
		for _, kAny := range reg.vars {
			v := kAny.(*Value)
			if fastcall {
				if frameSize+regSize > 16 {
					panic(&CompileError{Msg: fmt.Sprintf("frame size over on %s", lmd)})
				}
				v.Location = "fastcall_reg"
				v.Address = frameSize + regSize
			} else {
				v.Location = "reg"
				v.Address = regSize
			}
		}
		regSize += 2
	}

	lmd.FrameSize = frameSize
}

// allocateA は Aレジスタを割り当てられるなら割り当てる。
func allocateA(lmd *Lambda, registerVars []*allocEntry) []*allocEntry {
	var aVars []*Value
	for _, e := range registerVars {
		v := e.key
		if v.Type.Size != 1 {
			continue
		}
		if v.LiveRange.Max-v.LiveRange.Min == 1 {
			// Aレジスタを割り当てられる組み合わせでなければスルー
			op := lmd.Ops[v.LiveRange.Min]
			if !isSameValue(at(op, 1), v) {
				continue
			}
			if !symIn(op[0], "load", "add", "sub", "and", "or", "xor",
				"mul", "div", "mod", "uminus", "eq", "lt", "pget") {
				continue
			}

			nextOp := lmd.Ops[v.LiveRange.Min+1]
			switch nextOp[0] {
			case Sym("load"), Sym("sign_extension"), Sym("add"), Sym("and"), Sym("or"), Sym("xor"),
				Sym("eq"), Sym("lt"), Sym("pget"), Sym("sub"), Sym("push_arg"):
				if !isSameValue(at(nextOp, 2), v) {
					continue
				}
			case Sym("if"), Sym("return"):
				if !isSameValue(at(nextOp, 1), v) {
					continue
				}
			default:
				continue
			}

			aVars = append(aVars, v)
		}
	}
	for _, v := range aVars {
		v.Location = "a"
		registerVars = deleteEntry(registerVars, v)
	}
	return registerVars
}

// allocateCond はコンディションレジスタを割り当てられるなら割り当てる。
func allocateCond(lmd *Lambda, registerVars []*allocEntry) []*allocEntry {
	var condVars []*Value
	for _, e := range registerVars {
		v := e.key
		if v.Type.Size != 1 {
			continue
		}
		if v.LiveRange.Max-v.LiveRange.Min == 1 {
			op := lmd.Ops[v.LiveRange.Min]
			if !isSameValue(at(op, 1), v) {
				continue
			}
			if !symIn(op[0], "eq", "lt", "not") {
				continue
			}

			nextOp := lmd.Ops[v.LiveRange.Min+1]
			switch nextOp[0] {
			case Sym("if"):
				if !isSameValue(at(nextOp, 1), v) {
					continue
				}
			case Sym("not"):
				if !isSameValue(at(nextOp, 2), v) {
					continue
				}
			default:
				continue
			}

			switch op[0] {
			case Sym("eq"):
				v.Location = "cond"
				v.CondPositive = true
				v.CondReg = "zero"
			case Sym("lt"):
				v.Location = "cond"
				v.CondPositive = true
				if ValType(op[2]).Signed || ValType(op[3]).Signed {
					if ValType(op[2]).Size > 1 || ValType(op[3]).Size > 1 {
						// サイズ2以上の符号付き比較はフラグが特定できない。
						// Ruby版は next の前に location/cond_positive を設定済みのまま残す
						// (後で register_vars の割付により location は上書きされる)
						continue
					}
					v.CondReg = "negative"
				} else {
					v.CondReg = "carry"
				}
			case Sym("not"):
				if ValLocation(op[2]) != "cond" {
					continue
				}
				v.Location = "cond"
				v.CondReg = UnderlyingValue(op[2]).CondReg
				v.CondPositive = !UnderlyingValue(op[2]).CondPositive
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

func isSameValue(opElem any, v *Value) bool {
	// Ruby の op[1] == v (CastedValue は Delegator の == で from と比較される)
	return UnderlyingValue(opElem) == v
}

func symIn(s any, names ...string) bool {
	for _, n := range names {
		if eqAny(s, Sym(n)) {
			return true
		}
	}
	return false
}

func deleteEntry(entries []*allocEntry, v *Value) []*allocEntry {
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
	key       *Value
	liveRange *LiveRange
}

type AllocatorReg struct {
	liveRange *LiveRange
	vars      []any
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
			a.Regs = append(a.Regs, &AllocatorReg{liveRange: e.liveRange, vars: []any{e.key}})
		}
	}
	return a
}

// NewAllocatorGeneric はキーが任意の値のバージョン (単体テスト用)。
func NewAllocatorGeneric(keys []any, ranges []*LiveRange) *Allocator {
	a := &Allocator{}
	for i, k := range keys {
		lr := ranges[i]
		found := false
		for _, reg := range a.Regs {
			if !overlapRange(reg.liveRange, lr) {
				reg.liveRange = joinRange(reg.liveRange, lr)
				reg.vars = append(reg.vars, k)
				found = true
				break
			}
		}
		if !found {
			a.Regs = append(a.Regs, &AllocatorReg{liveRange: lr, vars: []any{k}})
		}
	}
	return a
}

func overlapRange(r1, r2 *LiveRange) bool {
	return r1.Max >= r2.Min && r1.Min <= r2.Max
}

func joinRange(r1, r2 *LiveRange) *LiveRange {
	return &LiveRange{Min: min(r1.Min, r2.Min), Max: max(r1.Max, r2.Max)}
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
func (l *LiveRangeCalculator) CalcLiveRange(defines, uses []int) *LiveRange {
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
	return &LiveRange{Min: minI, Max: maxI}
}

// ---------------------------------------------------------------
// 使っていない変数の削除 (Ruby版では Llc#delete_unuse)
// ---------------------------------------------------------------

func DeleteUnuse(lmd *Lambda) {
	for i, op := range lmd.Ops {
		if op == nil {
			continue
		}
		switch op[0] {
		case Sym("pget"), Sym("load"):
			if op[1] != nil && UnderlyingValue(op[1]) != nil && UnderlyingValue(op[1]).Unuse {
				lmd.Ops[i] = nil
			}
		case Sym("call"), Sym("fastcall"):
			if op[1] != nil && UnderlyingValue(op[1]) != nil && UnderlyingValue(op[1]).Unuse {
				lmd.Ops[i][1] = nil
			}
		}
	}
}
