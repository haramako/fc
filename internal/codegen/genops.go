package codegen

// 関数 1 つのコード生成: IR の命令の種類ごとのアセンブリの生成 (compileLambda の本体の switch を命令ごとのメソッドに分けたもの)。
//
// funcGen は compileLambda の局所変数だった状態 (出力の行、処理中の命令、常駐の復帰、呼び出しの引数の積み方) を持ち、
// *Llc を埋め込むのでモジュール単位の状態 (Lambdas / Limits / fused …) とオペランドの読み書き (loadA / storeA / byte …) は
// そのまま使える。各メソッドは 1 つの IR の命令を l.r に出す。

import (
	"fmt"
	"os"
	"strings"

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/types"
)

// funcGen は関数 1 つのコード生成の状態。
type funcGen struct {
	*Llc
	lmd *ir.Lambda
	ops []*ir.Op
	r   *asmLines // 出力の行 (命令ごとに追加する)

	// 処理中の命令
	op      *ir.Op
	opNo    int
	restore [ir.NumRegs]bool // 命令の後でメモリ側から戻す常駐 (退避したもの。genSwitch は飛ぶ側の経路でも戻す)

	// 呼び出しの引数の積み方 (push_result から call まで。入れ子は calls の末尾が内側)
	calls               []*pendingCall
	pushArgSize         int // stack 系: S+k,x に積んだ引数のバイト数
	pushFastcallArgSize int // fastcall: FC_FASTCALL_REG に積んだバイト数
}

// genLabel は Label のコード生成。
func (l *funcGen) genLabel() {
	r, op := l.r, l.op
	r.push(op.Label + ":")
	l.dbgLast = "" // 合流点の後は位置を出し直す
}

// genIf は If / IfTrue のコード生成。
func (l *funcGen) genIf() {
	r, op := l.r, l.op
	restore := &l.restore
	// OpIf は値が 0 のとき、OpIfTrue は 0 でないときに Label へ
	onTrue := op.Code == ir.OpIfTrue
	if ir.ValLocation(op.In(0)) == ir.LocCond {
		// コンディションレジスタの場合。CondPositive のとき「真 ⇔ フラグがセット」、ただし C だけは
		// 「真 ⇔ C クリア」(比較 a < b は C クリアで真。regalloc.allocateCond 参照)
		r.push(fmt.Sprintf("%s %s", condJump(ir.UnderlyingValue(op.In(0)), onTrue), op.Label))
	} else if l.inA(op.In(0)) && ir.ValType(op.In(0)).Size == 1 {
		// A に常駐している値: フラグが A を反映しているとは限らない (直前が A の演算ならピープホールが cmp を消す)
		r.push("cmp #0")
		r.push(fmt.Sprintf("%s %s", ifElse(onTrue, "bne", "beq"), op.Label))
	} else if l.inY(op.In(0)) && ir.ValType(op.In(0)).Size == 1 {
		r.push("cpy #0")
		r.push(fmt.Sprintf("%s %s", ifElse(onTrue, "bne", "beq"), op.Label))
	} else if l.inX(op.In(0)) && ir.ValType(op.In(0)).Size == 1 {
		r.push("cpx #0")
		r.push(fmt.Sprintf("%s %s", ifElse(onTrue, "bne", "beq"), op.Label))
	} else if l.aHeld && ir.ValType(op.In(0)).Size == 1 {
		// A は常駐変数で塞がっている: Y で検査する
		r.push(markTest(fmt.Sprintf("ldy %s", l.byte(op.In(0), 0))))
		r.push(fmt.Sprintf("%s %s", ifElse(onTrue, "bne", "beq"), op.Label))
	} else if restore[ir.RegA] {
		// A に常駐している変数を壊して検査し、飛び先でも A が要る: 飛ぶ側の経路でも復帰する
		// (この後の共通の復帰は落ちてくる側にしか効かない)。条件を反転して飛ばない側を @f に逃がし、
		// 飛ぶ側は `lda home; jmp L` を通す
		size := ir.ValType(op.In(0)).Size
		labels := l.newLabels(2)
		fall, taken := labels[0], labels[1]
		for i := 0; i < size; i++ {
			r.push(l.testA(op.In(0), i))
			switch {
			case onTrue && i < size-1:
				r.push(fmt.Sprintf("bne %s", taken)) // どれかのバイトが 0 でなければ飛ぶ
			case onTrue:
				r.push(fmt.Sprintf("beq %s", fall))
			default:
				r.push(fmt.Sprintf("bne %s", fall)) // 全バイトが 0 なら飛ぶ
			}
		}
		r.push(taken + ":")
		r.push("lda " + l.byte(op.Res[ir.RegA].V.Home, 0))
		r.push(fmt.Sprintf("jmp %s", op.Label))
		r.push(fall + ":")
	} else if onTrue {
		// 値のどれかのバイトが 0 でなければ飛ぶ
		for i := 0; i < ir.ValType(op.In(0)).Size; i++ {
			r.push(l.testA(op.In(0), i))
			r.push(fmt.Sprintf("bne %s", op.Label))
		}
	} else {
		// 全バイトが 0 なら飛ぶ
		thenLabel := l.newLabel()
		size := ir.ValType(op.In(0)).Size
		for i := 0; i < size; i++ {
			r.push(l.testA(op.In(0), i))
			if i == size-1 {
				r.push(fmt.Sprintf("beq %s", op.Label))
			} else {
				r.push(fmt.Sprintf("bne %s", thenLabel))
			}
		}
		r.push(thenLabel + ":")
	}
}

// genIfCarry は IfCarry のコード生成。
func (l *funcGen) genIfCarry() {
	r, op := l.r, l.op
	r.push(fmt.Sprintf("bcs %s", op.Label))
}

// genIfNotCarry は IfNotCarry のコード生成。
func (l *funcGen) genIfNotCarry() {
	r, op := l.r, l.op
	r.push(fmt.Sprintf("bcc %s", op.Label))
}

// genJump は Jump のコード生成。
func (l *funcGen) genJump() {
	r, op := l.r, l.op
	r.push(fmt.Sprintf("jmp %s", op.Label))
}

// genSwitch は Switch のコード生成。
func (l *funcGen) genSwitch() {
	r, op, lmd := l.r, l.op, l.lmd
	restore := &l.restore
	// ジャンプテーブル: 飛び先 - 1 を下位 / 上位の表に並べ、pha; pha; rts で飛ぶ。範囲外は次の命令へ落ちる。
	// 飛ぶ側の経路では常駐レジスタをここで復帰する (共通の復帰は落ちる側にしか効かない)
	minV, _ := ir.ValIntLiteral(op.In(1))
	labels := l.newLabels(3)
	lo, hi, fall := labels[0], labels[1], labels[2]
	r.push(l.loadA(op.In(0), 0))
	if minV&255 != 0 {
		r.push("sec", fmt.Sprintf("sbc #%d", minV&255))
	}
	// 表の添字は X。stack 関数 (再帰) では X がフレームポインタなので Y を使う (regalloc は switch を Y の clobber
	// と見る)。devirtualization で再帰する関数に switch が入って発覚
	ix := "x"
	if lmd.ABI == ir.ABIStack {
		ix = "y"
	}
	r.push(fmt.Sprintf("cmp #%d", len(op.Labels)), fmt.Sprintf("bcs %s", fall), "ta"+ix,
		fmt.Sprintf("lda %s,%s", hi, ix), "pha", fmt.Sprintf("lda %s,%s", lo, ix), "pha")
	r.push(l.restoreResident(op, *restore, ir.RegX, ir.RegY, ir.RegA)...)
	r.push("rts")
	los := make([]string, len(op.Labels))
	his := make([]string, len(op.Labels))
	for k, t := range op.Labels {
		los[k] = fmt.Sprintf("<(%s-1)", t)
		his[k] = fmt.Sprintf(">(%s-1)", t)
	}
	r.push(lo+":", ".byte "+strings.Join(los, ","), hi+":", ".byte "+strings.Join(his, ","), fall+":")
}

// genReturn は Return のコード生成。
func (l *funcGen) genReturn() {
	r, op, lmd := l.r, l.op, l.lmd
	if op.In(0) != nil {
		r.push(l.load(lmd.Result, op.In(0)))
	}
	if lmd.Entry && lmd.Type.Base.Size > 0 {
		// 呼び出し側はスタック (FC_SP の指す位置) から戻り値を読む。本体が X を使ったかもしれないので戻す
		r.push("ldx FC_SP")
		for i := 0; i < lmd.Type.Base.Size; i++ {
			r.push(fmt.Sprintf("lda %s", staticAddr(lmd, i)), fmt.Sprintf("sta <S+%d,x", i))
		}
	}
	if lmd.ABI == ir.ABIStack && lmd.FrameSize > 0 {
		r.push("lda FC_SP", "sec", fmt.Sprintf("sbc #%d", lmd.FrameSize), "sta FC_SP") // 空き先頭を戻す
	}
	if lmd.RegResult {
		r.push(fmt.Sprintf("lda %s", staticAddr(lmd, 0))) // 戻り値を A にも (直前が同じ lda / sta ならピープホールが消す)
	}
	r.push("rts")
}

// genPushResult は PushResult / PushFastcallResult のコード生成。
func (l *funcGen) genPushResult() {
	r, op, opNo, ops, lmd := l.r, l.op, l.opNo, l.ops, l.lmd
	restore := &l.restore
	// 呼び出しの開始。呼び先の種類 (static / stack / fastcall) で引数の置き場所が決まる (IR の flavor は見ない)
	pc := l.resolveCall(ops, opNo)
	l.calls = append(l.calls, pc)
	switch pc.kind {
	case ckStack:
		l.pushArgSize += op.Type.Size
		r.push(l.loadSP(lmd)) // 引数 (S+k,x) の前に X をスタックの空き先頭に
		l.holdX++             // call まで X = FC_SP のまま (X の常駐はここで退避済み。復帰は call の後)
		restore[ir.RegX] = false
	case ckFastcallReg:
		l.pushFastcallArgSize += op.Type.Size
	}
}

// genPushArg は PushArg / PushFastcallArg のコード生成。
func (l *funcGen) genPushArg() {
	r, op, opNo, ops, lmd := l.r, l.op, l.opNo, l.ops, l.lmd
	restore := &l.restore
	pc := l.calls[len(l.calls)-1]
	if op.ArgY && pc.kind == ckStatic && !pc.far && pc.callee.RegArgY && pc.argOff == regArgYOffset(pc.callee) {
		// 最後から 2 つ目の引数は Y に置いて呼ぶ (markArgY: 直後が最後の引数の push_arg、その直後が call)
		r.push(l.loadY(op.In(0))...)
		pc.inY = true
		pc.argOff++
		return
	}
	for i := 0; i < op.Type.Size; i++ {
		r.push(l.loadA(op.In(0), i))
		switch pc.kind {
		case ckStatic:
			if pc.callee.RegArg && !pc.far && pc.argOff == regArgOffset(pc.callee) && nextOp(ops, opNo) == pc.callOp {
				pc.inA = true // 最後の引数は A のまま呼ぶ (直後が call のときだけ)
				pc.argOff++
				// A の常駐変数は call まで A に戻さない (復帰の lda で引数が消える)。この引数が常駐変数そのもの
				// (friendly: 退避していない) なら、call で退避しない代わりにここで書き戻す
				if op.Res[ir.RegA].V != nil && op.Res[ir.RegA].In && !op.Res[ir.RegA].V.Clean && !l.resMem[ir.RegA] {
					r.push(l.spillResident(op, ir.RegA))
				}
				restore[ir.RegA] = false
				l.holdA = true
				break
			}
			r.push(fmt.Sprintf("sta %s", staticAddr(pc.callee, pc.argOff)))
			pc.argOff++
		case ckStack:
			r.push(fmt.Sprintf("sta <S+%d,x", l.stackBase(lmd)+l.pushArgSize))
			l.pushArgSize++
		case ckFastcallReg:
			r.push(fmt.Sprintf("sta <FC_FASTCALL_REG+%d", l.pushFastcallArgSize))
			l.pushFastcallArgSize++
		case ckCc65:
			r.push(fmt.Sprintf("sta <FC_FASTCALL_REG+%d", i)) // 呼ぶ直前に A / X へ (引数は 1 つだけ)
		}
	}
}

// genCall は Call / Fastcall のコード生成。
func (l *funcGen) genCall() {
	r, op, opNo, ops, lmd := l.r, l.op, l.opNo, l.ops, l.lmd
	pc := l.calls[len(l.calls)-1]
	l.calls = l.calls[:len(l.calls)-1]
	l.holdA = false // A / Y の引数の保持はここまで (常駐の退避の抑制はこの命令の前で見た)
	if pc.kind == ckStack && l.holdX > 0 {
		l.holdX-- // X = FC_SP の保持はここまで (復帰の可否はこの命令の前で見た)
	}
	fnType := ir.ValType(op.In(0))
	var sym string
	if ir.ValKind(op.In(0)) == ir.KindLiteral {
		sym = mangle(ir.ValLiteral(op.In(0)).Symbol)
	}
	switch pc.kind {
	case ckStatic:
		// 引数は呼び先のフレームに書いてある。static / entry の関数からは jsr、stack の関数からは X を進めて呼ぶ
		target := sym
		if pc.callee.Entry {
			target = directSym(sym) // プロローグ (スタックからのコピー) を飛ばす
		}
		if pc.inY && pc.callee.RegArg && !pc.inA {
			panic("Y argument in register but A argument in frame") // markArgY が保証する (直後が最後の引数、その直後が call)
		}
		switch {
		case (pc.callee.RegArg && !pc.inA) || (pc.callee.RegArgY && !pc.inY && !pc.callee.RegArg):
			target = frameSym(sym) // レジスタ渡しの引数もフレームに書いた: 入口の sty / sta を飛ばす
		case pc.callee.RegArgY && !pc.inY:
			target = aSym(sym) // Y の引数だけフレームに書いた (A の引数は A): sty だけ飛ばす
		}
		if op.Far {
			r.push(l.farCallSetup(target))
			r.push(l.callStatic(lmd, "farcall"))
		} else {
			r.push(l.callStatic(lmd, target))
		}
		if op.Dst != nil {
			if pc.callee.RegResult && !op.Far && lmd.ABI != ir.ABIStack {
				r.push(l.storeA(op.Dst, 0)) // 戻り値は A で返ってくる (stack 関数は X を戻すのに A を使うので不可)
			} else {
				for i := 0; i < ir.ValType(op.Dst).Size; i++ {
					r.push(fmt.Sprintf("lda %s", staticAddr(pc.callee, i)))
					r.push(l.storeA(op.Dst, i))
				}
			}
		}
	case ckStack:
		for _, a := range fnType.Params {
			l.pushArgSize -= a.Size
		}
		l.pushArgSize -= fnType.Base.Size
		base := l.stackBase(lmd) + l.pushArgSize
		if fnType.IsFarFunc() {
			for i := 0; i < 3; i++ {
				r.push(l.loadA(op.In(0), i))
				r.push(fmt.Sprintf("sta FC_FARCALL+%d", i))
			}
			r.push(l.callStackish(lmd, "farcall"))
		} else if op.Far {
			// 別バンクの関数: 呼び先とバンクを FC_FARCALL に置いて farcall (ターゲット側のトランポリン) を呼ぶ
			r.push(l.farCallSetup(ir.ValLiteral(op.In(0)).Symbol))
			r.push(l.callStackish(lmd, "farcall"))
		} else if sym != "" {
			r.push(l.callStackish(lmd, sym))
		} else {
			// 関数ポインタから呼ぶ (表から読んだ 添字付きの load_mem が reg に直接書いたなら写さない)
			if !fnPtrInReg(lmd, ops, opNo) {
				r.push(l.loadA(op.In(0), 0))
				r.push("sta <reg+0")
				r.push(l.loadA(op.In(0), 1))
				r.push("sta <reg+1")
			}
			r.push(l.callStackish(lmd, "jsr_reg"))
		}
		if op.Dst != nil {
			r.push(l.loadSP(lmd)) // 呼び先が X を壊したかもしれない
			for i := 0; i < ir.ValType(op.Dst).Size; i++ {
				r.push(fmt.Sprintf("lda <%d+S+%d,x", i, base))
				r.push(l.storeA(op.Dst, i))
			}
		}
	case ckCc65:
		// cc65 の __fastcall__: 引数を A (下位) / X (上位) に載せて jsr。戻り値は A / X (呼び先は A/X/Y を壊す)
		for _, p := range fnType.Params {
			r.push("lda <FC_FASTCALL_REG+0")
			if p.Size == 2 {
				r.push("ldx <FC_FASTCALL_REG+1")
			}
		}
		r.push(fmt.Sprintf("jsr %s", sym))
		if op.Dst != nil {
			size := ir.ValType(op.Dst).Size
			r.push("sta <FC_FASTCALL_REG+0")
			if size == 2 {
				r.push("stx <FC_FASTCALL_REG+1")
			}
			r.push(l.restoreX(lmd)...)
			for i := 0; i < size; i++ {
				r.push(fmt.Sprintf("lda <FC_FASTCALL_REG+%d", i))
				r.push(l.storeA(op.Dst, i))
			}
		} else {
			r.push(l.restoreX(lmd)...)
		}
	case ckFastcallReg:
		l.pushFastcallArgSize = 0
		if op.Far {
			r.push(l.farCallSetup(ir.ValLiteral(op.In(0)).Symbol))
			r.push("jsr farcall")
		} else if sym != "" {
			r.push(fmt.Sprintf("jsr %s", sym))
		} else {
			if !fnPtrInReg(lmd, ops, opNo) {
				r.push(l.loadA(op.In(0), 0))
				r.push("sta <reg+0")
				r.push(l.loadA(op.In(0), 1))
				r.push("sta <reg+1")
			}
			r.push("jsr jsr_reg")
		}
		if op.Dst != nil {
			for i := 0; i < ir.ValType(op.Dst).Size; i++ {
				r.push(fmt.Sprintf("lda <%d+FC_FASTCALL_REG", i))
				r.push(l.storeA(op.Dst, i))
			}
		}
	}
}

// genLoad は Load のコード生成。
func (l *funcGen) genLoad() {
	r, op := l.r, l.op
	if l.inY(op.Dst) {
		// Y に常駐する変数への代入: ldy x (A の一時変数なら tay)
		if l.inA(op.In(0)) {
			r.push("tay")
		} else {
			r.push(fmt.Sprintf("ldy %s", l.byte(op.In(0), 0)))
		}
		return
	}
	if l.inY(op.In(0)) {
		r.push(fmt.Sprintf("sty %s", l.byte(op.Dst, 0)))
		return
	}
	if l.inX(op.Dst) {
		if l.inA(op.In(0)) {
			r.push("tax")
		} else {
			r.push(fmt.Sprintf("ldx %s", l.byte(op.In(0), 0)))
		}
		return
	}
	if l.inX(op.In(0)) {
		r.push(fmt.Sprintf("stx %s", l.byte(op.Dst, 0)))
		return
	}
	if l.aHeld {
		// A は常駐変数で塞がっている: Y で写す
		for i := 0; i < ir.ValType(op.Dst).Size; i++ {
			if !l.sameByte(op.Dst, op.In(0), i) {
				r.push(fmt.Sprintf("ldy %s", l.byte(op.In(0), i)), fmt.Sprintf("sty %s", l.byte(op.Dst, i)))
			}
		}
		return
	}
	r.push(l.load(op.Dst, op.In(0)))
}

// genSignExtension は SignExtension のコード生成。
func (l *funcGen) genSignExtension() {
	r, op := l.r, l.op
	labels := l.newLabels(2)
	plsLabel, endLabel := labels[0], labels[1]
	// TODO: サイズ1->2以上の場合を実装すること、いまはそれしかないから十分だけど
	r.push(l.loadA(op.In(0), 0))
	if l.inA(op.In(0)) {
		// 入力が A にある (呼び出しの戻り値など) と lda が出ず、N が A を反映しているとは限らない
		// (常駐 Y の復帰の ldy の後だった。fuzz で発覚)。直前が A の演算ならピープホールが消す
		r.push("cmp #0")
	}
	r.push(l.storeA(op.Dst, 0))
	r.push(fmt.Sprintf("bpl %s", plsLabel))
	r.push("lda #255")
	r.push(fmt.Sprintf("jmp %s", endLabel))
	r.push(plsLabel + ":")
	r.push("lda #0")
	r.push(endLabel + ":")
	r.push(l.storeA(op.Dst, 1))
}

// genAddSub は Add / Sub のコード生成。
func (l *funcGen) genAddSub() {
	r, op := l.r, l.op
	if lines, ok := l.incDec(op); ok {
		r.push(lines)
		return
	}
	carry, alu := "clc", "adc"
	if op.Code == ir.OpSub {
		carry, alu = "sec", "sbc"
	}
	for i := 0; i < ir.ValType(op.Dst).Size; i++ {
		if i == 0 {
			r.push(carry)
		}
		r.push(l.loadA(op.In(0), i))
		r.push(fmt.Sprintf("%s %s", alu, l.byte(op.In(1), i)))
		r.push(l.storeA(op.Dst, i))
	}
}

// genBitwise は And / Or / Xor のコード生成。
func (l *funcGen) genBitwise() {
	r, op := l.r, l.op
	for i := 0; i < ir.ValType(op.Dst).Size; i++ {
		r.push(l.loadA(op.In(0), i))
		as := map[ir.OpCode]string{ir.OpAnd: "and", ir.OpOr: "ora", ir.OpXor: "eor"}[op.Code]
		r.push(fmt.Sprintf("%s %s", as, l.byte(op.In(1), i)))
		r.push(l.storeA(op.Dst, i))
	}
}

// genMulDivMod は Mul / Div / Mod のコード生成。
func (l *funcGen) genMulDivMod() {
	r, op := l.r, l.op
	r.push(l.mulDivMod(op))
}

// genRotateCarry は RolC / RorC のコード生成。
func (l *funcGen) genRotateCarry() {
	r, op := l.r, l.op
	// C を通す 1 バイトの回転 (直前の shift_left / shift_right / rolc が残した C を受ける。間に C を変える命令は無い)
	mn := ifElse(op.Code == ir.OpRolC, "rol", "ror")
	if l.inA(op.Dst) && l.inA(op.In(0)) {
		r.push(mn + " a")
	} else if isValueOrCasted(op.Dst) && !l.inA(op.Dst) && !l.inY(op.Dst) && !l.inX(op.Dst) &&
		ir.ValLocation(op.Dst) != ir.LocCond && l.byte(op.Dst, 0) == l.byte(op.In(0), 0) {
		r.push(mn + " " + l.byte(op.Dst, 0))
	} else {
		r.push(l.loadA(op.In(0), 0), mn+" a", l.storeA(op.Dst, 0))
	}
}

// genShift は ShiftLeft / ShiftRight のコード生成。
func (l *funcGen) genShift() {
	r, op, lmd := l.r, l.op, l.lmd
	signed := ir.ValType(op.In(0)).Signed
	rotate := ifElse(op.Code == ir.OpShiftLeft, "rol", "ror")
	if n, ok := ir.ValIntLiteral(op.In(1)); ok {
		// 定数の場合
		if lines, ok := l.shiftByte(op, n, signed); ok && !lmd.Cfg().Disabled("shift8") {
			r.push(lines) // 2 バイトの 8 以上のシフトはバイトの移動 (8.8 固定小数の `x >> 8` など)
		} else if lines, ok := l.shiftInMemory(op, n, signed); ok {
			r.push(lines)
		} else if ir.ValType(op.Dst).Size == 1 {
			// サイズが1
			r.push(l.loadA(op.In(0), 0))
			for k := 0; k < n; k++ {
				switch {
				case signed && op.Code == ir.OpShiftRight:
					r.push("cmp #128", "ror a") // 算術シフト (符号を C に立ててから回す)
				case op.Code == ir.OpShiftLeft:
					r.push("asl a")
				default:
					r.push("lsr a")
				}
			}
			r.push(l.storeA(op.Dst, 0))
		} else {
			// サイズが２以上
			r.push(anyIfy(l.load(op.Dst, op.In(0))))
			for k := 0; k < n; k++ {
				if op.Code == ir.OpShiftLeft {
					// 左シフト
					for i := 0; i < ir.ValType(op.Dst).Size; i++ {
						r.push(l.loadA(op.Dst, i))
						if i == 0 {
							r.push("clc")
						}
						r.push("rol a")
						r.push(l.storeA(op.Dst, i))
					}
				} else {
					// 右シフト
					for i := ir.ValType(op.Dst).Size - 1; i >= 0; i-- {
						r.push(l.loadA(op.Dst, i))
						if i == ir.ValType(op.Dst).Size-1 {
							if signed {
								r.push("cmp #128")
							} else {
								r.push("clc")
							}
						}
						r.push("ror a")
						r.push(l.storeA(op.Dst, i))
					}
				}
			}
		}
	} else {
		// 定数でない場合
		// TODO: もうちょっと整理して効率よくできるはず
		if size := ir.ValType(op.Dst).Size; size != 1 {
			// 2 バイト以上: 回数を Y に置き、値を Dst に写してから 1 ビットずつ回す (定数の回数の形と同じ)。
			// 回数を先に読む (Dst と同じ変数のことがある: `x = w << x`)
			labels := l.newLabels(2)
			loopLabel, endLabel := labels[0], labels[1]
			r.push(l.loadA(op.In(1), 0))
			r.push("tay")
			r.push(anyIfy(l.load(op.Dst, op.In(0))))
			r.push(loopLabel + ":")
			r.push("cpy #0")
			r.push(fmt.Sprintf("beq %s", endLabel))
			if op.Code == ir.OpShiftLeft {
				for i := 0; i < size; i++ {
					r.push(l.loadA(op.Dst, i))
					if i == 0 {
						r.push("clc")
					}
					r.push("rol a")
					r.push(l.storeA(op.Dst, i))
				}
			} else {
				for i := size - 1; i >= 0; i-- {
					r.push(l.loadA(op.Dst, i))
					if i == size-1 {
						if signed {
							r.push("cmp #128") // 算術右シフト
						} else {
							r.push("clc")
						}
					}
					r.push("ror a")
					r.push(l.storeA(op.Dst, i))
				}
			}
			r.push("dey")
			r.push(fmt.Sprintf("jmp %s", loopLabel))
			r.push(endLabel + ":")
			return
		}
		labels := l.newLabels(2)
		loopLabel, endLabel := labels[0], labels[1]
		r.push(l.loadA(op.In(1), 0))
		r.push("tay")
		r.push(l.loadA(op.In(0), 0))
		r.push(loopLabel + ":")
		r.push("cpy #0")
		r.push(fmt.Sprintf("beq %s", endLabel))
		if signed && op.Code == ir.OpShiftRight {
			r.push("cmp #128") // 算術右シフト: 符号を C に立ててから回す
		} else {
			r.push("clc") // 左シフトは符号付きでも C を 0 に (符号を回し込んで `-109 << n` が 39 になっていた。fuzz で発覚)
		}
		r.push(fmt.Sprintf("%s a", rotate))
		r.push("dey")
		r.push(fmt.Sprintf("jmp %s", loopLabel))
		r.push(endLabel + ":")
		r.push(l.storeA(op.Dst, 0))
	}
}

// genUminus は Uminus のコード生成。
func (l *funcGen) genUminus() {
	r, op := l.r, l.op
	if ir.ValType(op.Dst).Kind != types.Int {
		panic(&diag.Error{Msg: fmt.Sprintf("cannot negate non-integer type %s", ir.ValType(op.Dst))})
	}
	if l.inA(op.In(0)) && ir.ValType(op.Dst).Size == 1 {
		// A にある値の 2 の補数
		r.push("eor #255", "clc", "adc #1", l.storeA(op.Dst, 0))
		return
	}
	for i := 0; i < ir.ValType(op.Dst).Size; i++ {
		if i == 0 {
			r.push("sec")
		}
		r.push("lda #0")
		if ir.ValType(op.In(0)).Size > i {
			r.push(fmt.Sprintf("sbc %s", l.byte(op.In(0), i)))
		}
		r.push(l.storeA(op.Dst, i))
	}
}

// genEq は Eq のコード生成。
func (l *funcGen) genEq() {
	r, op := l.r, l.op
	if l.inY(op.In(0)) && ir.ValLocation(op.Dst) == ir.LocCond {
		r.push(fmt.Sprintf("cpy %s", l.byte(op.In(1), 0)))
		return
	}
	if l.inX(op.In(0)) && ir.ValLocation(op.Dst) == ir.LocCond {
		r.push(fmt.Sprintf("cpx %s", l.byte(op.In(1), 0)))
		return
	}
	// 可換なので第 2 入力がレジスタにあっても同じ (`on_idx == i` の i@X → cpx on_idx)
	if l.inY(op.In(1)) && ir.ValLocation(op.Dst) == ir.LocCond && ir.ValType(op.In(0)).Size == 1 {
		r.push(fmt.Sprintf("cpy %s", l.byte(op.In(0), 0)))
		return
	}
	if l.inX(op.In(1)) && ir.ValLocation(op.Dst) == ir.LocCond && ir.ValType(op.In(0)).Size == 1 {
		r.push(fmt.Sprintf("cpx %s", l.byte(op.In(0), 0)))
		return
	}
	if l.inA(op.In(1)) && ir.ValLocation(op.Dst) == ir.LocCond && ir.ValType(op.In(0)).Size == 1 {
		r.push(fmt.Sprintf("cmp %s", l.byte(op.In(0), 0)))
		return
	}
	if l.aHeld && ir.ValLocation(op.Dst) == ir.LocCond {
		r.push(fmt.Sprintf("ldy %s", l.byte(op.In(0), 0)), fmt.Sprintf("cpy %s", l.byte(op.In(1), 0)))
		return
	}
	labels := l.newLabels(2)
	falseLabel, endLabel := labels[0], labels[1]
	size := max(ir.ValType(op.In(0)).Size, ir.ValType(op.In(1)).Size)
	for i := 0; i < size; i++ {
		r.push(l.loadA(op.In(0), i))
		r.push(fmt.Sprintf("cmp %s", l.byte(op.In(1), i)))
		r.push(fmt.Sprintf("bne %s", falseLabel))
	}
	if ir.ValLocation(op.Dst) == ir.LocCond {
		r.lines = r.lines[:len(r.lines)-1] // 最後のbneを消す
		if size != 1 {
			r.push(falseLabel + ":")
		}
	} else {
		// falseのとき
		r.push("lda #1")
		r.push(l.storeA(op.Dst, 0))
		r.push(fmt.Sprintf("jmp %s", endLabel))
		// trueのとき
		r.push(falseLabel + ":")
		r.push("lda #0")
		r.push(l.storeA(op.Dst, 0))
		r.push(endLabel + ":")
	}
}

// genLt は Lt のコード生成。
func (l *funcGen) genLt() {
	r, op, lmd := l.r, l.op, l.lmd
	// a < b。フラグの意味 (LocCond のとき OpIf が見る):
	//   符号なし: 多バイトの減算の借り = C クリア ⇔ a < b
	//   符号付き: 減算結果の符号 (V でオーバーフローを補正) = N セット ⇔ a < b
	// どちらか一方でも符号付きなら符号付き比較 (v1 は左辺しか見ておらず、多バイトや
	// オーバーフローのある符号付き比較も壊れていた。2026-09-14 に書き直し)
	if l.inY(op.In(0)) && ir.ValLocation(op.Dst) == ir.LocCond {
		r.push(fmt.Sprintf("cpy %s", l.byte(op.In(1), 0)))
		return
	}
	if l.inX(op.In(0)) && ir.ValLocation(op.Dst) == ir.LocCond {
		r.push(fmt.Sprintf("cpx %s", l.byte(op.In(1), 0)))
		return
	}
	if l.aHeld && ir.ValLocation(op.Dst) == ir.LocCond {
		r.push(fmt.Sprintf("ldy %s", l.byte(op.In(0), 0)), fmt.Sprintf("cpy %s", l.byte(op.In(1), 0)))
		return
	}
	labels := l.newLabels(3)
	trueLabel, endLabel, skipLabel := labels[0], labels[1], labels[2]
	size := max(ir.ValType(op.In(0)).Size, ir.ValType(op.In(1)).Size)
	signed := ir.ValType(op.In(0)).Signed || ir.ValType(op.In(1)).Signed
	if lmd.Cfg().Trace("signed") != "" && signed {
		// 調査用: 符号付き比較の場所を列挙する
		lit0, ok0 := ir.ValIntLiteral(op.In(0))
		lit1, ok1 := ir.ValIntLiteral(op.In(1))
		fmt.Fprintf(os.Stderr, "SIGNED_LT %s mixed=%v bigliteral=%v %s:%s < %s:%s cond=%v\n", op.Pos,
			ir.ValType(op.In(0)).Signed != ir.ValType(op.In(1)).Signed, ok0 && lit0 >= 128 || ok1 && lit1 >= 128,
			ir.OperandString(op.In(0)), ir.ValType(op.In(0)), ir.OperandString(op.In(1)), ir.ValType(op.In(1)),
			ir.ValLocation(op.Dst) == ir.LocCond)
	}
	if lit, ok := ir.ValIntLiteral(op.In(1)); signed && ok && lit == 0 {
		// a < 0 (符号付き) は a の最上位バイトの符号ビットそのもの。値が A にあるときは cmp #0 で N を立て直す
		// (呼び出しの後の常駐の復帰 `ldx` が N を壊していた。fuzz で発覚。不要ならピープホールが消す)
		r.push(l.testA(op.In(0), size-1))
	} else {
		r.push(l.loadA(op.In(0), 0))
		if size == 1 && signed {
			r.push("sec")
			r.push(fmt.Sprintf("sbc %s", l.byte(op.In(1), 0)))
		} else {
			r.push(fmt.Sprintf("cmp %s", l.byte(op.In(1), 0)))
		}
		for i := 1; i < size; i++ {
			r.push(l.loadA(op.In(0), i))
			r.push(fmt.Sprintf("sbc %s", l.byte(op.In(1), i)))
		}
		if signed {
			r.push(fmt.Sprintf("bvc %s", skipLabel))
			r.push("eor #$80")
			r.push(skipLabel + ":")
		}
	}
	if ir.ValLocation(op.Dst) != ir.LocCond {
		// 値として 0 / 1 を作る
		if signed {
			r.push(fmt.Sprintf("bmi %s", trueLabel))
			r.push("lda #0")
			r.push(fmt.Sprintf("beq %s", endLabel)) // lda #0 で Z が立つ
			r.push(trueLabel + ":")
			r.push("lda #1")
			r.push(endLabel + ":")
		} else {
			// A = C (a >= b) を反転
			r.push("lda #0")
			r.push("rol a")
			r.push("eor #1")
		}
		r.push(l.storeA(op.Dst, 0))
	}
}

// genNot は Not のコード生成。
func (l *funcGen) genNot() {
	r, op := l.r, l.op
	if ir.ValLocation(op.Dst) != ir.LocCond && ir.ValLocation(op.In(0)) == ir.LocCond {
		// 入力がフラグで結果は値 (`!((a < b) as int16)` のように cast を挟むと regalloc が入力だけ cond にする):
		// フラグで分岐して 0 / 1 を作る (lda はフラグを変えるので先に分岐する)
		labels := l.newLabels(2)
		trueLabel, endLabel := labels[0], labels[1]
		r.push(fmt.Sprintf("%s %s", condJump(ir.UnderlyingValue(op.In(0)), true), trueLabel))
		r.push("lda #1", fmt.Sprintf("jmp %s", endLabel), trueLabel+":", "lda #0", endLabel+":")
		r.push(l.storeA(op.Dst, 0))
	} else if ir.ValLocation(op.Dst) != ir.LocCond {
		labels := l.newLabels(2)
		trueLabel, endLabel := labels[0], labels[1]
		// 全バイトが 0 のとき 1 (バイトごとに beq すると「どれかが 0」になってしまう。SSA の定数畳み込みとの
		// 差分テストで発覚)
		r.push(l.loadA(op.In(0), 0))
		for i := 1; i < ir.ValType(op.In(0)).Size; i++ {
			r.push(fmt.Sprintf("ora %s", l.byte(op.In(0), i)))
		}
		r.push(fmt.Sprintf("beq %s", trueLabel))
		// falseのとき
		r.push("lda #0")
		r.push(l.storeA(op.Dst, 0))
		r.push(fmt.Sprintf("jmp %s", endLabel))
		// trueのとき
		r.push(trueLabel + ":")
		r.push("lda #1")
		r.push(l.storeA(op.Dst, 0))
		r.push(endLabel + ":")
	}
}

// genBitNot は BitNot のコード生成。
func (l *funcGen) genBitNot() {
	r, op := l.r, l.op
	for i := 0; i < ir.ValType(op.Dst).Size; i++ {
		r.push(l.loadA(op.In(0), i))
		r.push("eor #255")
		r.push(l.storeA(op.Dst, i))
	}
}

// genAsm は Asm のコード生成。
func (l *funcGen) genAsm() {
	r, op := l.r, l.op
	r.push(op.Text)
}

// genIndex は Index のコード生成。
func (l *funcGen) genIndex() {
	r, op := l.r, l.op
	if es := ir.ValType(op.In(0)).Base.Size; es != 1 && es != 2 {
		r.push(l.indexLarge(op))
	} else if ir.ValType(op.In(1)).Size == 1 {
		// インデックスのサイズが１
		if ir.ValType(op.In(0)).Kind == types.Array && ir.ValLocation(op.In(0)) == ir.LocFrame {
			// フレーム上のローカル配列: 先頭は S + addr + X (ゼロページなので上位は 0。OpRef と同じ)
			r.push(l.loadYIdx(op.In(1), ir.ValType(op.In(0)).Base.Size))
			r.push("sty <reg+0")
			r.push("txa")
			r.push("clc")
			r.push(fmt.Sprintf("adc #.LOBYTE(S+%d)", ir.ValAddress(op.In(0))))
			r.push("clc")
			r.push("adc <reg+0")
			r.push(l.storeA(op.Dst, 0))
			r.push("lda #0")
			r.push(l.storeA(op.Dst, 1))
		} else if es == 2 && (ir.ValType(op.In(0)).Kind == types.Pointer || ir.ValType(op.In(0)).Size > 256) {
			// 要素 2 バイトで i * 2 が 1 バイトに収まらないことがある (長さの分からないポインタ、128 要素を超える配列):
			// 9 ビット目 (asl の C) を上位に足す (捨てていて `a[150]` (a:[200]u16) が a[22] だった。survey 2026-09-27)
			r.push(l.loadA(op.In(1), 0), "asl a", "sta <reg+0", "lda #0", "rol a", "sta <reg+1", "clc")
			if ir.ValType(op.In(0)).Kind == types.Array {
				r.push(fmt.Sprintf("lda #.LOBYTE(%s)", l.addrExpr(op.In(0))), "adc <reg+0", l.storeA(op.Dst, 0),
					fmt.Sprintf("lda #.HIBYTE(%s)", l.addrExpr(op.In(0))), "adc <reg+1", l.storeA(op.Dst, 1))
			} else {
				r.push(l.loadA(op.In(0), 0), "adc <reg+0", l.storeA(op.Dst, 0), l.loadA(op.In(0), 1), "adc <reg+1", l.storeA(op.Dst, 1))
			}
		} else if ir.ValType(op.In(0)).Kind == types.Array {
			r.push(l.loadYIdx(op.In(1), ir.ValType(op.In(0)).Base.Size))
			r.push("sty <reg+0")
			r.push("clc")
			r.push(fmt.Sprintf("lda #.LOBYTE(%s)", l.addrExpr(op.In(0))))
			r.push("adc <reg+0")
			r.push(l.storeA(op.Dst, 0))
			r.push(fmt.Sprintf("lda #.HIBYTE(%s)", l.addrExpr(op.In(0))))
			r.push("adc #0")
			r.push(l.storeA(op.Dst, 1))
		} else if ir.ValType(op.In(0)).Kind == types.Pointer {
			r.push(l.loadYIdx(op.In(1), ir.ValType(op.In(0)).Base.Size))
			r.push("sty <reg+0")
			r.push("clc")
			r.push(l.loadA(op.In(0), 0))
			r.push("adc <reg+0")
			r.push(l.storeA(op.Dst, 0))
			r.push(l.loadA(op.In(0), 1))
			r.push("adc #0")
			r.push(l.storeA(op.Dst, 1))
		} else {
			panic("invalid index")
		}
	} else {
		// インデックスのサイズが２
		// TODO: ちゃんとする、テスト作る
		if ir.ValType(op.In(0)).Kind == types.Array {
			r.push(l.loadA(op.In(1), 0))
			r.push("sta <reg+0")
			r.push(l.loadA(op.In(1), 1))
			r.push("sta <reg+1")

			if ir.ValType(op.In(0)).Base.Size == 2 {
				r.push("clc")
				r.push("rol <reg+0")
				r.push("rol <reg+1")
			}

			if ir.ValLocation(op.In(0)) == ir.LocFrame {
				// フレーム上のローカル配列 (上記と同じ。インデックスの上位は 0 とみなす)
				r.push("txa")
				r.push("clc")
				r.push(fmt.Sprintf("adc #.LOBYTE(S+%d)", ir.ValAddress(op.In(0))))
				r.push("clc")
				r.push("adc <reg+0")
				r.push(l.storeA(op.Dst, 0))
				r.push("lda <reg+1")
				r.push("adc #0")
				r.push(l.storeA(op.Dst, 1))
			} else {
				r.push("lda <reg+0")
				r.push("clc")
				r.push(fmt.Sprintf("adc #.LOBYTE(%s)", l.addrExpr(op.In(0))))
				r.push(l.storeA(op.Dst, 0))
				r.push("lda <reg+1")
				r.push(fmt.Sprintf("adc #.HIBYTE(%s)", l.addrExpr(op.In(0))))
				r.push(l.storeA(op.Dst, 1))
			}
		} else if ir.ValType(op.In(0)).Kind == types.Pointer {
			// ポインタ + 16 ビットの添字 (fc 3 の広い slice の範囲・添字)
			r.push(l.loadA(op.In(1), 0))
			r.push("sta <reg+0")
			r.push(l.loadA(op.In(1), 1))
			r.push("sta <reg+1")
			if ir.ValType(op.In(0)).Base.Size == 2 {
				r.push("asl <reg+0")
				r.push("rol <reg+1")
			}
			r.push("clc")
			r.push(l.loadA(op.In(0), 0))
			r.push("adc <reg+0")
			r.push(l.storeA(op.Dst, 0))
			r.push(l.loadA(op.In(0), 1))
			r.push("adc <reg+1")
			r.push(l.storeA(op.Dst, 1))
		} else {
			panic("invalid index")
		}
	}
}

// genRef は Ref のコード生成。
func (l *funcGen) genRef() {
	r, op := l.r, l.op
	if ir.ValLocation(op.In(0)) == ir.LocFrame {
		r.push("txa")
		r.push("clc")
		r.push(fmt.Sprintf("adc #.LOBYTE(S+%d)", ir.ValAddress(op.In(0))))
		r.push(l.storeA(op.Dst, 0))
		r.push("lda #0")
		r.push(l.storeA(op.Dst, 1))
	} else {
		r.push(fmt.Sprintf("lda #.LOBYTE(%s)", l.addrExpr(op.In(0))))
		r.push(l.storeA(op.Dst, 0))
		r.push(fmt.Sprintf("lda #.HIBYTE(%s)", l.addrExpr(op.In(0))))
		r.push(l.storeA(op.Dst, 1))
	}
}

// genLoadMem は LoadMem のコード生成 (doc/v4_memops.md の表)。
func (l *funcGen) genLoadMem() {
	r, op, lmd, ops, opNo := l.r, l.op, l.lmd, l.ops, l.opNo
	restore := &l.restore
	m := op.Mem()
	if m.Index == nil {
		if m.BaseIsArray() {
			// 添字の無いグローバルの配列: 絶対番地
			for i := 0; i < m.Width; i++ {
				r.push(fmt.Sprintf("lda %s+%d", l.toAsm(m.Base), m.Disp+i), l.storeA(op.Dst, i))
			}
			return
		}
		r.push(l.pointerRead(m.Base, m.Disp, op.Dst, m.Width))
		return
	}
	if !isByteInt(ir.ValType(m.Index)) {
		panic(&diag.Error{Msg: "16-bit index is not supported here (use a 1-byte index)"})
	}
	if !m.BaseIsArray() {
		// ポインタ + 添字 (+ ずれ): ldy idx; lda (p),y (ずれがあれば Y = idx * scale + disp を A で計算)
		base, setup := l.pointerBase(m.Base)
		if m.Width > 1 && sameStorage(m.Base, op.Dst) {
			base, setup = "reg", []any{l.loadA(m.Base, 0), "sta <reg+0", l.loadA(m.Base, 1), "sta <reg+1"}
		}
		// 添字を先に Y へ (添字が A にあるとき、ポインタを reg に写す setup が A を壊す。fuzz で発覚)
		r.push(l.loadYIdxDisp(m.Index, m.Scale, m.Disp))
		r.push(setup)
		for i := 0; i < m.Width; i++ {
			if i > 0 {
				r.push("iny")
			}
			r.push(fmt.Sprintf("lda (%s),y", base))
			r.push(l.storeA(op.Dst, i))
		}
		if m.Disp == 0 {
			r.push(l.restoreY(m.Index, m.Width))
		}
		return
	}
	// グローバルの配列 + 添字: lda a+disp,y (添字が X に常駐していれば ,x)
	// Y の常駐変数をこの命令の最後で復帰するなら融合しない (添字を入れた Y が直後の命令の前に常駐の値に戻って、
	// `tab+0,y` が常駐の値で読んでいた: `sty k; ldy #2; ldy k; sbc tab+0,y`。fuzz で発覚)
	if reg, ok := l.fusableIndex(ops, opNo); ok && !(reg == "y" && restore[ir.RegY]) && !lmd.Cfg().Disabled("fuse-index") {
		// 直後の sub / lt の第 2 入力に融合: 添字をレジスタに用意して、結果の一時変数を `tab+0,y` として読ませる
		if reg == "y" {
			r.push(l.loadYIdx(m.Index, m.Scale))
		}
		l.fused = map[*ir.Value]string{op.Dst.(*ir.Value): fmt.Sprintf("%s+%d,%s", l.toAsm(m.Base), m.Disp, reg)}
		l.fusedAt = opNo
		return
	}
	store := func(i int) any { return l.storeA(op.Dst, i) }
	if fnPtrToReg(lmd, ops, opNo) >= 0 {
		store = func(i int) any { return fmt.Sprintf("sta <reg+%d", i) } // 呼び出しが reg から飛ぶ (写しを省く)
	}
	if l.inX(m.Index) {
		// 添字が X に常駐 (グローバル配列、要素 1 バイトかバイト単位の添字)
		for i := 0; i < m.Width; i++ {
			r.push(fmt.Sprintf("lda %s+%d,x", l.toAsm(m.Base), m.Disp+i))
			r.push(store(i))
		}
		return
	}
	r.push(l.loadYIdx(m.Index, m.Scale))
	for i := 0; i < m.Width; i++ {
		r.push(fmt.Sprintf("lda %s+%d,y", l.toAsm(m.Base), m.Disp+i))
		r.push(store(i))
	}
}

// genStoreMem は StoreMem のコード生成 (doc/v4_memops.md の表)。書く幅は m.Width (値が小さいリテラルでも上位まで書く)。
func (l *funcGen) genStoreMem() {
	r, op := l.r, l.op
	m := op.Mem()
	val := op.MemValue()
	if m.Index == nil {
		if m.BaseIsArray() {
			for i := 0; i < m.Width; i++ {
				r.push(l.loadA(val, i), fmt.Sprintf("sta %s+%d", l.toAsm(m.Base), m.Disp+i))
			}
			return
		}
		r.push(l.pointerWrite(m.Base, m.Disp, val, m.Width))
		return
	}
	if !isByteInt(ir.ValType(m.Index)) {
		panic(&diag.Error{Msg: "16-bit index is not supported here (use a 1-byte index)"})
	}
	if !m.BaseIsArray() {
		base, setup := l.pointerBase(m.Base)
		pre := append(l.loadYIdxDisp(m.Index, m.Scale, m.Disp), setup...) // 添字を先に Y へ (load_mem と同じ)
		if m.Scale == 1 && m.Disp == 0 && len(setup) == 0 {
			r.push(pre) // ldy だけなら A は壊れない
		} else {
			r.push(l.keepA(val, pre))
		}
		for i := 0; i < m.Width; i++ {
			if i > 0 {
				r.push("iny")
			}
			r.push(l.loadA(val, i))
			r.push(fmt.Sprintf("sta (%s),y", base))
		}
		if m.Disp == 0 {
			r.push(l.restoreY(m.Index, m.Width))
		}
		return
	}
	if l.inX(m.Index) {
		for i := 0; i < m.Width; i++ {
			r.push(l.loadA(val, i))
			r.push(fmt.Sprintf("sta %s+%d,x", l.toAsm(m.Base), m.Disp+i))
		}
		return
	}
	if m.Scale == 1 {
		r.push(l.loadYIdx(m.Index, m.Scale))
	} else {
		r.push(l.keepA(val, l.loadYIdx(m.Index, m.Scale))) // lda idx; asl; tay は A を壊す
	}
	for i := 0; i < m.Width; i++ {
		r.push(l.loadA(val, i))
		r.push(fmt.Sprintf("sta %s+%d,y", l.toAsm(m.Base), m.Disp+i))
	}
}
