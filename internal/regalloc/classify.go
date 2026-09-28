package regalloc

// Classify とその部品: 常駐の変数を置いたまま命令を実行できるか (friendly)、触らないか (free)、壊すか (clobber)。
// 特別な出し方は形の表 (forms.go)、汎用の出力 (codegen の loadA / storeA が A にある値の lda / sta を出さない形と、添字を
// Y / X のまま使う形) はここの規則で決める。どちらの得もサイクル数は m6502 の表から数える (以前の手書きの 3 / 6 / 1)。

import (
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/m6502"
)

// regPlace は常駐のレジスタの置き場所。
var regPlace = [ir.NumRegs]Place{ir.RegA: PA, ir.RegY: PY, ir.RegX: PX}

// placeOf は割付の結果から見た置き場所 (割付の前の一時変数はメモリ)。
func placeOf(o ir.Operand) Place {
	switch o.(type) {
	case *ir.Value, *ir.CastedValue:
	default:
		return PMem
	}
	if ir.ValKind(o) != ir.KindLocal {
		return PMem
	}
	switch ir.ValLocation(o) {
	case ir.LocA:
		return PA
	case ir.LocY:
		return PY
	case ir.LocX:
		return PX
	case ir.LocCond:
		return PCond
	}
	return PMem
}

// resPlace は regalloc の見込みの置き場所: in のレジスタに置く常駐 (見積もる候補、または置いたまま実行する常駐) と、
// 退避してメモリ側で扱う常駐 (spilled) を上書きし、それ以外は placeOf。
type resPlace struct {
	in      [ir.NumRegs]*ir.Value
	spilled [ir.NumRegs]*ir.Value
}

func (p resPlace) Where(o ir.Operand) Place {
	if o == nil {
		return PMem
	}
	if _, ok := o.(*ir.PointeredArray); ok {
		return PMem
	}
	if uv := ir.UnderlyingValue(o); uv != nil {
		for reg, v := range p.in {
			if v != nil && uv == v {
				return regPlace[reg]
			}
		}
		for _, v := range p.spilled {
			if v != nil && uv == v {
				return PMem
			}
		}
	}
	return placeOf(o)
}

// formOf は命令 i の形 (比較の結果がフラグに乗るかは命令の並び (condPredicted) で見る。Y での代用 (aHeld) は選ばない)。
func formOf(lmd *ir.Lambda, i int, p Placement) (Form, bool) {
	op := lmd.Ops[i]
	switch op.Code {
	case ir.OpLoad:
		return LoadForm(op, p, false)
	case ir.OpIf, ir.OpIfTrue:
		return IfForm(op, p, false)
	case ir.OpEq, ir.OpLt:
		return CmpForm(op, p, condPredicted(lmd, i), false)
	case ir.OpAdd, ir.OpSub:
		return StepForm(op, p, describe{}, stepMax(lmd))
	case ir.OpShiftLeft, ir.OpShiftRight:
		return ShiftMemForm(op, p)
	case ir.OpRolC, ir.OpRorC:
		return RotForm(op, p, describe{})
	}
	return Form{}, false
}

// zp はゼロページのオペランドの命令 (規則の得の見積もり用)。
func zp(mnem string) Ins { return Ins{Mnem: mnem, Arg: Arg{Mode: m6502.ZP}} }

// friendlyA は v が A に常駐しているとき、op を A のまま実行できるか (できれば節約できるサイクル数の目安)。
func friendlyA(lmd *ir.Lambda, i int, v *ir.Value, liveOut bool) (bool, int) {
	op := lmd.Ops[i]
	var p resPlace
	p.in[ir.RegA] = v
	if f, ok := formOf(lmd, i, p); ok && f.Reg() == PA {
		return true, f.Gain() // cmp #0 / eq の cmp a / rol a
	}
	// 汎用の出力が A にある値をそのまま使う形 (loadA / storeA は A にある値の lda / sta を出さない)
	switch op.Code {
	case ir.OpLoad:
		if isV(op.Dst, v) && isMemOrLit(op.Src[0]) && ir.ValType(op.Src[0]).Size == 1 {
			return true, Cycles([]Ins{zp("sta")}) // lda x (sta v が消える)
		}
		if isV(op.Src[0], v) && isMemByte(op.Dst) {
			return true, Cycles([]Ins{zp("lda")}) // sta x (lda v が消える)
		}
	case ir.OpAdd, ir.OpSub, ir.OpAnd, ir.OpOr, ir.OpXor:
		if isIncDec(op) && isV(op.Dst, v) {
			return true, Cycles([]Ins{zp("inc")}) - Cycles([]Ins{imp("clc"), ins("adc", imm(1))}) // inc x → clc; adc #1
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
			return true, Cycles([]Ins{zp("lda"), zp("sta")}) // lda v … sta v が消える
		}
		if !liveOut && isMemByte(op.Dst) {
			return true, Cycles([]Ins{zp("lda")})
		}
	case ir.OpShiftLeft, ir.OpShiftRight:
		if _, lit := ir.ValIntLiteral(op.Src[1]); lit && isV(op.Src[0], v) && (isV(op.Dst, v) || (!liveOut && isMemByte(op.Dst))) {
			return true, Cycles([]Ins{zp("asl")}) - Cycles([]Ins{acc("asl")}) // asl mem → asl a
		}
	case ir.OpUminus, ir.OpBitNot:
		if isV(op.Src[0], v) && isV(op.Dst, v) {
			return true, Cycles([]Ins{zp("lda")}) // 少なめに、lda v が消える分だけ数える
		}
	case ir.OpEq, ir.OpLt:
		// (eq で b が A の形は形の表: FormCmpCommA)
		if isV(op.Src[0], v) && condPredicted(lmd, i) && isMemOrLit(op.Src[1]) && ir.ValType(op.Src[1]).Size == 1 {
			// 符号付きの比較は sec; sbc; bvc; eor で A を壊す (cmp と違って) ので、v がその後も要るなら A のままではできない
			signed := op.Code == ir.OpLt && op.IsSigned()
			if !signed || !liveOut {
				return true, Cycles([]Ins{zp("lda")}) // lda v; cmp k → cmp k
			}
		}
	case ir.OpLoadMem:
		// 添字付きの読み出しだけ (添字の無いポインタ経由の読み出しは ldy #k を使い、結果を A に残す形ではない)
		if m := op.Mem(); m.Index != nil {
			if isV(op.Dst, v) && !isV(m.Index, v) {
				return true, Cycles([]Ins{zp("sta")})
			}
			if isV(m.Index, v) && !isV(op.Dst, v) && !liveOut {
				return true, Cycles([]Ins{zp("ldy")}) - Cycles([]Ins{imp("tay")}) // ldy v → tay
			}
		}
	case ir.OpStoreMem:
		// 書く先が値より大きい (u8 の常駐を i16 の要素に書く) と、codegen は上位を lda #0 で書いて A を壊すので friendly にしない
		// (`a3[3] = l0` (a3:[16]i16) の後の l0@A の dec が 0 から始まってループが終わらなかった。fuzz で発覚)
		if m := op.Mem(); isV(op.MemValue(), v) && m.Width <= ir.ValType(v).Size && (m.Index == nil || !isV(m.Index, v)) {
			return true, Cycles([]Ins{zp("lda")})
		}
	case ir.OpPushArg, ir.OpPushFastcallArg:
		if isV(op.In(0), v) {
			return true, Cycles([]Ins{zp("lda")})
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
			return true, Cycles([]Ins{zp("lda")})
		}
	}
	return false, 0
}

// friendlyY は v が Y に常駐しているとき、op を Y のまま実行できるか (添字と 1 バイトのカウンタの形)。
func friendlyY(lmd *ir.Lambda, i int, v *ir.Value) (bool, int) {
	op := lmd.Ops[i]
	var p resPlace
	p.in[ir.RegY] = v
	if f, ok := formOf(lmd, i, p); ok && f.Reg() == PY {
		return true, f.Gain() // ldy / sty / cpy / iny × k
	}
	// 汎用の出力が添字を Y のまま使う形 (要素 1 バイトの配列 / ゼロページのポインタの添字: ldy が消える)
	switch op.Code {
	case ir.OpLoadMem:
		if directIndex(op) && isV(op.In(1), v) && !isV(op.Dst, v) {
			return true, Cycles([]Ins{zp("ldy")})
		}
	case ir.OpStoreMem:
		if directIndex(op) && isV(op.In(1), v) && !isV(op.MemValue(), v) {
			return true, Cycles([]Ins{zp("ldy")})
		}
	}
	return false, 0
}

// friendlyX は v が X に常駐しているとき、op を X のまま実行できるか (グローバル配列の添字と 1 バイトのカウンタ)。
func friendlyX(lmd *ir.Lambda, i int, v *ir.Value) (bool, int) {
	op := lmd.Ops[i]
	var p resPlace
	p.in[ir.RegX] = v
	if f, ok := formOf(lmd, i, p); ok && f.Reg() == PX {
		return true, f.Gain() // ldx / stx / cpx / inx × k
	}
	// 汎用の出力がグローバル配列の添字を X のまま使う形 (lda a,x: ldy v が消える)
	globalArray := func() bool { return byteIndex(op) && op.Mem().BaseIsArray() }
	switch op.Code {
	case ir.OpLoadMem:
		if globalArray() && isV(op.In(1), v) && !isV(op.Dst, v) {
			return true, Cycles([]Ins{zp("ldy")})
		}
	case ir.OpStoreMem:
		if globalArray() && isV(op.In(1), v) && !isV(op.MemValue(), v) {
			return true, Cycles([]Ins{zp("ldy")})
		}
	}
	return false, 0
}

// freeA は op が A を使わずに実行できるか (Y での代用を除く)。p は Y / X に置いたまま実行する常駐と、退避する常駐を
// 反映した置き場所 (Classify)。形 (メモリ上の inc / シフト / rol、フラグの分岐、Y / X のままの ldy / cpy / iny …) は
// 命令列が A を書かなければ触らない。
func freeA(lmd *ir.Lambda, i int, p Placement) bool {
	switch lmd.Ops[i].Code {
	case ir.OpLabel, ir.OpJump, ir.OpIfCarry, ir.OpIfNotCarry, ir.OpPushResult, ir.OpPushFastcallResult:
		return true
	}
	f, ok := formOf(lmd, i, p)
	return ok && !f.WritesA()
}

// yVariant は A が塞がっているとき、op を Y で代用できるか (1〜2 バイトの load、1 バイト変数の if、1 バイト符号なしの比較)。
func yVariant(lmd *ir.Lambda, i int) bool {
	op := lmd.Ops[i]
	switch op.Code {
	case ir.OpLoad:
		return copyViaY(op)
	case ir.OpIf, ir.OpIfTrue:
		return ifViaY(op)
	case ir.OpEq, ir.OpLt:
		return condPredicted(lmd, i) && cmpViaY(op)
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
	// A を触らないかを見る置き場所: Y / X のまま実行する常駐はそのレジスタに、退避する常駐はメモリに
	var pa resPlace
	// X (inx / cpx / ldx / stx は A も Y も使わない。lda a,x は A を使う)
	if vX != nil {
		if involved(vX) {
			// stack 系の呼び出しの引数を積んでいる間 (HoldX: X = FC_SP) は、codegen が X の常駐を退避してメモリ側で扱う
			if ok, save := friendlyX(lmd, i, vX); ok && !op.HoldX {
				d.X = ResFriendly
				pa.in[ir.RegX] = vX
				gain += save
			} else {
				d.X = ResClobber
				pa.spilled[ir.RegX] = vX
			}
		} else if needsX(op) || touches(vX) {
			d.X = ResClobber
		}
	}
	// Y (先に決める: Y のまま実行できる命令 (iny / cpy / ldy / sty / lda a,y) は A を使わない)。呼び出しの引数を Y に
	// 保持中 (markArgY) は、codegen が Y の常駐を退避してメモリ側で扱い、Y での代用もしないので同じに見る
	// (`sty l0` と見積もって A は触らないとしたのに、codegen はメモリの l0.lo を A で写していた。fuzz で発覚)
	holdY := op.ArgY || op.HoldY
	if vY != nil && involved(vY) {
		if ok, save := friendlyY(lmd, i, vY); ok && !holdY {
			d.Y = ResFriendly
			pa.in[ir.RegY] = vY
			gain += save
		} else {
			d.Y = ResClobber
			pa.spilled[ir.RegY] = vY
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
		} else if !freeA(lmd, i, pa) {
			if aLive && (vY == nil || !yLive) && !holdY && yVariant(lmd, i) {
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
