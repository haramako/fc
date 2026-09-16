package codegen

// LLC: 中間コード (ir.go) → ca65 アセンブリ。命令選択 (llc.go / arith.go / operand.go / call.go / data.go) と
// asm テキストの後処理 (peephole.go、extendjump.go)。IR→IR の最適化は internal/opt、置き場所の決定は internal/regalloc。

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/frames"
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/opt"
	"github.com/haramako/fc/internal/regalloc"
	"github.com/haramako/fc/internal/types"
)

type Llc struct {
	OptimizeLevel int
	Limits        regalloc.Limits // レジスタ領域の大きさ (base.asm と一致させる)
	FarCall       bool            // far call が有効 (各モジュールに farcall / FC_FARCALL の import を出す)
	labelCount    int
	codeSegment   string
	curLambda     *ir.Lambda // 処理中の関数 (エラー位置の補完用)
	curOp         *ir.Op     // 処理中の命令 (エラー位置の補完用)
	zero          *ir.Value  // 定数 0 (mul の 0 倍の最適化用)
	types         *types.Universe
	Lambdas       map[string]*ir.Lambda // Id → 関数 (全モジュール。呼び先の呼び出し規約を引く。frames.Analyze の結果)

	// ループ内の A / Y 常駐 (doc/v2_regalloc.md): 処理中の命令でレジスタを占有している変数と、その扱い
	res     *ir.Value // op.Resident (A)
	resMem  bool      // 退避中: res をメモリ (Home) として参照する
	resY    *ir.Value // op.ResidentY
	resYMem bool      // 退避中: resY をメモリ (Home) として参照する
	aHeld   bool      // A は res で塞がっていて、この命令は res を触らない (Y で代用する)
}

func NewLlc(optimizeLevel int, u *types.Universe) *Llc {
	return &Llc{OptimizeLevel: optimizeLevel, Limits: regalloc.DefaultLimits, zero: ir.NewIntLiteral("", u.IntType(1, false), 0), types: u}
}

// asmLines は文字列 / nil / ネストした配列を保持する行バッファ。
type asmLines struct {
	lines []any
}

func (a *asmLines) push(xs ...any) {
	a.lines = append(a.lines, xs...)
}

// flatten は flatten + delete(nil) 相当。
func (a *asmLines) flatten() []string {
	var r []string
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case nil:
			// delete(nil)
		case string:
			r = append(r, x)
		case []any:
			for _, e := range x {
				walk(e)
			}
		case []string:
			for _, e := range x {
				walk(e)
			}
		default:
			panic(fmt.Sprintf("invalid asm line %T", v))
		}
	}
	for _, e := range a.lines {
		walk(e)
	}
	return r
}

// Compile はモジュールをアセンブラに変換する。(asm, inc) の行リストを返す。
// コード生成中の CompileError は処理中の関数の宣言位置を補完して返す (回復点)。
func (l *Llc) Compile(mod *ir.Module) (asmOut, incOut []string, err error) {
	defer func() {
		if r := recover(); r != nil {
			if ce, ok := r.(*diag.Error); ok {
				if !ce.Pos.IsValid() && l.curOp != nil && l.curOp.Pos.IsValid() {
					ce.Pos = l.curOp.Pos
				}
				if !ce.Pos.IsValid() && l.curLambda != nil {
					ce.Pos = l.curLambda.Pos
				}
				err = ce
				return
			}
			panic(r)
		}
	}()
	l.labelCount = 0
	l.codeSegment = mod.Id
	l.curLambda = nil
	l.curOp = nil

	inc := &asmLines{}
	asm := &asmLines{}
	asm.push("\t.setcpu \"6502\"")
	asm.push("\t.include \"macro.inc\"")
	asm.push("\t.include \"_frames.inc\"") // 静的フレームの配置 (frames.Place が生成)
	asm.push(fmt.Sprintf("__MODULE_%s__ = 1", strings.ToUpper(mod.Id)))

	inc.push(fmt.Sprintf(".ifndef __MODULE_%s__", strings.ToUpper(mod.Id)))
	inc.push(fmt.Sprintf("__MODULE_%s__ = 1", strings.ToUpper(mod.Id)))

	asm.push(fmt.Sprintf(".segment \"%s\"", l.codeSegment)) // dummy

	for _, m := range mod.Uses {
		inc.push(fmt.Sprintf("\t.include \"_%s.inc\"", m.Id))
		asm.push(fmt.Sprintf("\t.include \"_%s.inc\"", m.Id))
	}
	if l.FarCall {
		asm.push("\t.global farcall") // トランポリンを include したモジュールでは export、それ以外では import になる
		asm.push("\t.import FC_FARCALL")
	}

	// include(.asm)の処理
	for _, file := range mod.IncludeAsms {
		asm.push(fmt.Sprintf("\t.include \"%s\"", file))
	}

	for _, d := range mod.Defs {
		switch d.Kind {
		case ir.DefEqu:
			var val string
			if d.Equ.IsInt {
				val = strconv.Itoa(d.Equ.Int)
			} else {
				val = mangle(d.Equ.Symbol)
			}
			inc.push(fmt.Sprintf("%s = %s", mangle(d.Sym), val))
			asm.push(fmt.Sprintf("%s = %s", mangle(d.Sym), val))
		case ir.DefBss:
			inc.push(fmt.Sprintf("\t.import %s", mangle(d.Sym)))
			asm.push(fmt.Sprintf("\t.export %s", mangle(d.Sym)))
			if d.Segment != "" {
				asm.push(fmt.Sprintf(".segment \"%s\"", d.Segment))
			} else {
				asm.push(".segment \"BSS\"")
			}
			asm.push(fmt.Sprintf("%s: .res %d", mangle(d.Sym), d.Type.Size))
		case ir.DefBlock:
			inc.push(fmt.Sprintf("\t.import %s", mangle(d.Sym)))
			asm.push(fmt.Sprintf("\t.export %s", mangle(d.Sym)))
			asm.push(fmt.Sprintf(".segment \"%s\"", l.codeSegment))
			asm.push(l.emitBlock(d.Sym, d.Type, d.Elems))
		case ir.DefCode:
			inc.push(fmt.Sprintf("\t.import %s", mangle(d.Sym)))
			asm.push(fmt.Sprintf("\t.export %s", mangle(d.Sym)))
			lmd := d.Lambda
			if lmd.Extern {
				continue
			}
			if lmd.Entry {
				inc.push(fmt.Sprintf("\t.import %s", directSym(mangle(d.Sym))))
				asm.push(fmt.Sprintf("\t.export %s", directSym(mangle(d.Sym))))
			}
			asm.push(anyList(l.CompileLambda(d.Sym, lmd)))
		default:
			panic(fmt.Sprintf("invalid def kind %s", d.Kind))
		}
	}

	// fastcall 関数が使う FC_FASTCALL_REG の大きさを、base.asm (プロジェクトが自前で持つこともある) の .res とリンク時に突き合わせる
	fastcallNeed := 0
	for _, d := range mod.Defs {
		if d.Kind == ir.DefCode && !d.Lambda.Extern && d.Lambda.ABI == ir.ABIFastcall {
			fastcallNeed = max(fastcallNeed, d.Lambda.ZpUsed)
		}
	}
	if fastcallNeed > 0 {
		asm.push("	.import FC_FASTCALL_REG_SIZE")
		asm.push(fmt.Sprintf("	.assert FC_FASTCALL_REG_SIZE >= %d, error, \"fastcall functions of module %s need %d bytes of FC_FASTCALL_REG (raise .res of FC_FASTCALL_REG and FC_FASTCALL_REG_SIZE in base.asm)\"", fastcallNeed, mod.Id, fastcallNeed))
	}

	// include header(.asm)の処理
	for _, file := range mod.IncludeHeaders {
		asm.push(fmt.Sprintf("\t.include \"%s\"", file))
	}

	// include(.chr)の処理
	for _, file := range mod.IncludeChrs {
		asm.push(".segment \"CHARS\"")
		asm.push(fmt.Sprintf("\t.incbin \"%s\"", file))
	}

	inc.push(".endif")

	return asm.flatten(), inc.flatten(), nil
}

func anyList(ss []string) []any {
	r := make([]any, len(ss))
	for i, s := range ss {
		r[i] = s
	}
	return r
}

// Prepare は関数 1 つの最適化とレジスタ割付 (frames.Analyze の後、frames.Place の前に全関数について呼ぶ)。
func (l *Llc) Prepare(lmd *ir.Lambda) {
	l.curLambda = lmd
	l.curOp = nil
	opt.Optimize(lmd, l.OptimizeLevel, l.types)
	if l.OptimizeLevel > 0 {
		regalloc.AllocateResident(lmd)
	}
	l.allocRegister(lmd)
}

// PrepareProgram はコード生成の前にプログラム全体で 1 回行う処理 (doc/v2_frame_alloc.md §6-4):
// 呼び出し規約の決定 (frames.Analyze) → 全関数の最適化と割付 (Prepare) → 静的フレームの配置 (frames.Place)。
// 戻り値の Plan.Inc を `_frames.inc` として書き、各モジュールの asm が include する。
func (l *Llc) PrepareProgram(mods []*ir.Module, staticZp, staticRam int) (*frames.Plan, error) {
	graph, err := frames.Analyze(mods)
	if err != nil {
		return nil, err
	}
	l.Lambdas = graph.ByID
	if err := l.PrepareAll(graph.Lambdas); err != nil {
		return nil, err
	}
	return frames.Place(graph, staticZp, staticRam)
}

// PrepareAll は全関数の Prepare (エラーは関数の位置を補完して返す)。
func (l *Llc) PrepareAll(lmds []*ir.Lambda) (err error) {
	defer func() {
		if r := recover(); r != nil {
			if ce, ok := r.(*diag.Error); ok {
				if !ce.Pos.IsValid() && l.curLambda != nil {
					ce.Pos = l.curLambda.Pos
				}
				err = ce
				return
			}
			panic(r)
		}
	}()
	for _, lmd := range lmds {
		l.Prepare(lmd)
	}
	return nil
}

// CompileLambda は関数1つ分のアセンブリを生成する (Prepare 済みであること)。
func (l *Llc) CompileLambda(sym string, lmd *ir.Lambda) []string {
	l.curLambda = lmd // エラー位置の補完用 (Compile の回復点で参照するので、ここでは戻さない)
	l.curOp = nil
	ops := lmd.Ops

	r := &asmLines{}

	r.push(";;;=============================")
	r.push(fmt.Sprintf(";;; function %s", lmd.Id))
	r.push(";;;=============================")

	if seg := lmd.Segment(); seg != "" {
		r.push(fmt.Sprintf(".segment \"%s\"", seg))
	} else {
		r.push(fmt.Sprintf(".segment \"%s\"", l.codeSegment))
	}
	if lmd.Entry {
		// アドレスを取られた関数: 関数ポインタ経由の呼び出し側はスタック (X の指す位置) に引数を積むので、
		// 自分のフレームに写してから本体 (__direct。呼び先が分かっている呼び出しはここから入る) へ
		r.push(mangle(sym) + ":")
		for k := lmd.Type.Base.Size; k < lmd.Type.Base.Size+argBytes(lmd); k++ {
			r.push(fmt.Sprintf("lda <S+%d,x", k), fmt.Sprintf("sta %s", staticAddr(lmd, k)))
		}
		r.push(fmt.Sprintf(".proc %s", directSym(mangle(sym))))
	} else {
		r.push(fmt.Sprintf(".proc %s", mangle(sym)))
	}

	pushArgSize := 0
	pushFastcallArgSize := 0
	var calls []*pendingCall // 積んでいる途中の呼び出し (内側が末尾)
	var prevOp *ir.Op        // 直前に生成した命令 (フラグの再利用の判定用)

	for opNo, op := range ops {
		if op == nil {
			continue
		}
		l.curOp = op
		if opNo > 0 {
			prevOp = ops[opNo-1]
		}
		// IRコメント (golden比較では除去されるため、Go版独自の形式でよい)
		cm := ir.DumpOp(op, nil)
		if len(cm) > 120 {
			cm = cm[:120]
		}
		r.push(fmt.Sprintf("; %04d: %s", opNo, cm))

		// A / Y 常駐: この命令の扱い (friendly / 触らない / 退避)
		l.res, l.resMem, l.resY, l.resYMem, l.aHeld = op.Resident, false, op.ResidentY, false, false
		restoreA, restoreY := false, false
		if op.Resident != nil || op.ResidentY != nil {
			d, _ := regalloc.Classify(lmd, opNo, op.Resident, op.ResidentY, op.ResIn || op.ResOut, op.ResOut, op.ResYIn || op.ResYOut)
			if op.Resident != nil && d.A == regalloc.ResClobber {
				if op.ResIn {
					r.push("sta " + l.byte(op.Resident.Home, 0))
				}
				l.resMem = true
				restoreA = op.ResOut
			}
			if op.ResidentY != nil && d.Y == regalloc.ResClobber {
				if op.ResYIn {
					r.push("sty " + l.byte(op.ResidentY.Home, 0))
				}
				l.resYMem = true
				restoreY = op.ResYOut
			}
			l.aHeld = d.UseY
		}

		switch op.Code {

		case ir.OpLabel:
			r.push(op.Label + ":")

		case ir.OpIf, ir.OpIfTrue:
			// OpIf は値が 0 のとき、OpIfTrue は 0 でないときに Label へ
			onTrue := op.Code == ir.OpIfTrue
			if ir.ValLocation(op.In(0)) == ir.LocCond {
				// コンディションレジスタの場合。CondPositive のとき「真 ⇔ フラグがセット」、ただし C だけは
				// 「真 ⇔ C クリア」(比較 a < b は C クリアで真。regalloc.allocateCond 参照)
				v := ir.UnderlyingValue(op.In(0))
				trueIsSet := v.CondPositive
				if v.CondReg == ir.CondCarry {
					trueIsSet = !v.CondPositive
				}
				jumpIfSet := trueIsSet == onTrue
				var asmOp string
				switch v.CondReg {
				case ir.CondZero:
					asmOp = ifElse(jumpIfSet, "beq", "bne")
				case ir.CondCarry:
					asmOp = ifElse(jumpIfSet, "bcs", "bcc")
				case ir.CondNegative:
					asmOp = ifElse(jumpIfSet, "bmi", "bpl")
				default:
					panic("invalid cond_reg")
				}
				r.push(fmt.Sprintf("%s %s", asmOp, op.Label))
			} else if l.flagsFromIncDec(prevOp, op.In(0)) {
				// 直前の inc / dec が Z を残している (`dec x; bne L`)
				r.push(fmt.Sprintf("%s %s", ifElse(onTrue, "bne", "beq"), op.Label))
			} else if l.inA(op.In(0)) && ir.ValType(op.In(0)).Size == 1 {
				// A に常駐している値: フラグが A を反映しているとは限らない (直前が A の演算ならピープホールが cmp を消す)
				r.push("cmp #0")
				r.push(fmt.Sprintf("%s %s", ifElse(onTrue, "bne", "beq"), op.Label))
			} else if l.inY(op.In(0)) && ir.ValType(op.In(0)).Size == 1 {
				r.push("cpy #0")
				r.push(fmt.Sprintf("%s %s", ifElse(onTrue, "bne", "beq"), op.Label))
			} else if l.aHeld && ir.ValType(op.In(0)).Size == 1 {
				// A は常駐変数で塞がっている: Y で検査する
				r.push(fmt.Sprintf("ldy %s", l.byte(op.In(0), 0)))
				r.push(fmt.Sprintf("%s %s", ifElse(onTrue, "bne", "beq"), op.Label))
			} else if onTrue {
				// 値のどれかのバイトが 0 でなければ飛ぶ
				for i := 0; i < ir.ValType(op.In(0)).Size; i++ {
					r.push(l.loadA(op.In(0), i))
					r.push(fmt.Sprintf("bne %s", op.Label))
				}
			} else {
				// 全バイトが 0 なら飛ぶ
				thenLabel := l.newLabel()
				size := ir.ValType(op.In(0)).Size
				for i := 0; i < size; i++ {
					r.push(l.loadA(op.In(0), i))
					if i == size-1 {
						r.push(fmt.Sprintf("beq %s", op.Label))
					} else {
						r.push(fmt.Sprintf("bne %s", thenLabel))
					}
				}
				r.push(thenLabel + ":")
			}

		case ir.OpIfCarry:
			r.push(fmt.Sprintf("bcs %s", op.Label))

		case ir.OpIfNotCarry:
			r.push(fmt.Sprintf("bcc %s", op.Label))

		case ir.OpJump:
			r.push(fmt.Sprintf("jmp %s", op.Label))

		case ir.OpReturn:
			if op.In(0) != nil {
				r.push(l.load(lmd.Result, op.In(0)))
			}
			if lmd.Entry && lmd.Type.Base.Size > 0 {
				// 呼び出し側はスタック (X の指す位置) から戻り値を読む
				for i := 0; i < lmd.Type.Base.Size; i++ {
					r.push(fmt.Sprintf("lda %s", staticAddr(lmd, i)), fmt.Sprintf("sta <S+%d,x", i))
				}
			}
			r.push("rts")

		case ir.OpPushResult, ir.OpPushFastcallResult:
			// 呼び出しの開始。呼び先の種類 (static / stack / fastcall) で引数の置き場所が決まる (IR の flavor は見ない)
			pc := l.resolveCall(ops, opNo)
			calls = append(calls, pc)
			switch pc.kind {
			case ckStack:
				pushArgSize += op.Type.Size
			case ckFastcallReg:
				pushFastcallArgSize += op.Type.Size
			}

		case ir.OpPushArg, ir.OpPushFastcallArg:
			pc := calls[len(calls)-1]
			for i := 0; i < op.Type.Size; i++ {
				r.push(l.loadA(op.In(0), i))
				switch pc.kind {
				case ckStatic:
					r.push(fmt.Sprintf("sta %s", staticAddr(pc.callee, pc.argOff)))
					pc.argOff++
				case ckStack:
					r.push(fmt.Sprintf("sta <S+%d,x", l.stackBase(lmd)+pushArgSize))
					pushArgSize++
				case ckFastcallReg:
					r.push(fmt.Sprintf("sta <FC_FASTCALL_REG+%d", pushFastcallArgSize))
					pushFastcallArgSize++
				}
			}

		case ir.OpCall, ir.OpFastcall:
			pc := calls[len(calls)-1]
			calls = calls[:len(calls)-1]
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
				if op.Far {
					r.push(l.farCallSetup(target))
					r.push(l.jsrOrCall(lmd, "farcall", lmd.FrameSize))
				} else {
					r.push(l.jsrOrCall(lmd, target, lmd.FrameSize))
				}
				if op.Dst != nil {
					for i := 0; i < ir.ValType(op.Dst).Size; i++ {
						r.push(fmt.Sprintf("lda %s", staticAddr(pc.callee, i)))
						r.push(l.storeA(op.Dst, i))
					}
				}
			case ckStack:
				for _, a := range fnType.Params {
					pushArgSize -= a.Size
				}
				pushArgSize -= fnType.Base.Size
				base := l.stackBase(lmd) + pushArgSize
				if op.Far {
					// 別バンクの関数: 呼び先とバンクを FC_FARCALL に置いて farcall (ターゲット側のトランポリン) を呼ぶ
					r.push(l.farCallSetup(ir.ValLiteral(op.In(0)).Symbol))
					r.push(l.jsrOrCall(lmd, "farcall", base))
				} else if sym != "" {
					r.push(l.jsrOrCall(lmd, sym, base))
				} else {
					// 関数ポインタから呼ぶ
					r.push(l.loadA(op.In(0), 0))
					r.push("sta <reg+0")
					r.push(l.loadA(op.In(0), 1))
					r.push("sta <reg+1")
					r.push(l.jsrOrCall(lmd, "jsr_reg", base))
				}
				if op.Dst != nil {
					for i := 0; i < ir.ValType(op.Dst).Size; i++ {
						r.push(fmt.Sprintf("lda <%d+S+%d,x", i, base))
						r.push(l.storeA(op.Dst, i))
					}
				}
			case ckFastcallReg:
				pushFastcallArgSize = 0
				if op.Far {
					r.push(l.farCallSetup(ir.ValLiteral(op.In(0)).Symbol))
					r.push("jsr farcall")
				} else if sym != "" {
					r.push(fmt.Sprintf("jsr %s", sym))
				} else {
					r.push(l.loadA(op.In(0), 0))
					r.push("sta <reg+0")
					r.push(l.loadA(op.In(0), 1))
					r.push("sta <reg+1")
					r.push("jsr jsr_reg")
				}
				if op.Dst != nil {
					for i := 0; i < ir.ValType(op.Dst).Size; i++ {
						r.push(fmt.Sprintf("lda <%d+FC_FASTCALL_REG", i))
						r.push(l.storeA(op.Dst, i))
					}
				}
			}

		case ir.OpLoad:
			if l.inY(op.Dst) {
				// Y に常駐する変数への代入: ldy x (A の一時変数なら tay)
				if l.inA(op.In(0)) {
					r.push("tay")
				} else {
					r.push(fmt.Sprintf("ldy %s", l.byte(op.In(0), 0)))
				}
				break
			}
			if l.inY(op.In(0)) {
				r.push(fmt.Sprintf("sty %s", l.byte(op.Dst, 0)))
				break
			}
			if l.aHeld {
				// A は常駐変数で塞がっている: Y で写す
				for i := 0; i < ir.ValType(op.Dst).Size; i++ {
					if !l.sameByte(op.Dst, op.In(0), i) {
						r.push(fmt.Sprintf("ldy %s", l.byte(op.In(0), i)), fmt.Sprintf("sty %s", l.byte(op.Dst, i)))
					}
				}
				break
			}
			r.push(l.load(op.Dst, op.In(0)))

		case ir.OpSignExtension:
			labels := l.newLabels(2)
			plsLabel, endLabel := labels[0], labels[1]
			// TODO: サイズ1->2以上の場合を実装すること、いまはそれしかないから十分だけど
			r.push(l.loadA(op.In(0), 0))
			r.push(l.storeA(op.Dst, 0))
			r.push(fmt.Sprintf("bpl %s", plsLabel))
			r.push("lda #255")
			r.push(fmt.Sprintf("jmp %s", endLabel))
			r.push(plsLabel + ":")
			r.push("lda #0")
			r.push(endLabel + ":")
			r.push(l.storeA(op.Dst, 1))

		case ir.OpAdd, ir.OpSub:
			if lines, ok := l.incDec(op); ok {
				r.push(lines)
				break
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

		case ir.OpAnd, ir.OpOr, ir.OpXor:
			for i := 0; i < ir.ValType(op.Dst).Size; i++ {
				r.push(l.loadA(op.In(0), i))
				as := map[ir.OpCode]string{ir.OpAnd: "and", ir.OpOr: "ora", ir.OpXor: "eor"}[op.Code]
				r.push(fmt.Sprintf("%s %s", as, l.byte(op.In(1), i)))
				r.push(l.storeA(op.Dst, i))
			}

		case ir.OpMul, ir.OpDiv, ir.OpMod:
			r.push(l.mulDivMod(op))

		case ir.OpShiftLeft, ir.OpShiftRight:
			signed := ir.ValType(op.In(0)).Signed
			rotate := ifElse(op.Code == ir.OpShiftLeft, "rol", "ror")
			if n, ok := ir.ValIntLiteral(op.In(1)); ok {
				// 定数の場合
				if lines, ok := l.shiftInMemory(op, n, signed); ok {
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
				if ir.ValType(op.Dst).Size != 1 {
					panic(&diag.Error{Msg: "shift of 2-byte value by non-constant count is not supported"})
				}
				labels := l.newLabels(2)
				loopLabel, endLabel := labels[0], labels[1]
				r.push(l.loadA(op.In(1), 0))
				r.push("tay")
				r.push(l.loadA(op.In(0), 0))
				r.push(loopLabel + ":")
				r.push("cpy #0")
				r.push(fmt.Sprintf("beq %s", endLabel))
				if signed {
					r.push("cmp #128")
				} else {
					r.push("clc")
				}
				r.push(fmt.Sprintf("%s a", rotate))
				r.push("dey")
				r.push(fmt.Sprintf("jmp %s", loopLabel))
				r.push(endLabel + ":")
				r.push(l.storeA(op.Dst, 0))
			}

		case ir.OpUminus:
			if ir.ValType(op.Dst).Kind != types.Int {
				panic(&diag.Error{Msg: fmt.Sprintf("cannot negate non-integer type %s", ir.ValType(op.Dst))})
			}
			if l.inA(op.In(0)) && ir.ValType(op.Dst).Size == 1 {
				// A にある値の 2 の補数
				r.push("eor #255", "clc", "adc #1", l.storeA(op.Dst, 0))
				break
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

		case ir.OpEq:
			if l.inY(op.In(0)) && ir.ValLocation(op.Dst) == ir.LocCond {
				r.push(fmt.Sprintf("cpy %s", l.byte(op.In(1), 0)))
				break
			}
			if l.aHeld && ir.ValLocation(op.Dst) == ir.LocCond {
				r.push(fmt.Sprintf("ldy %s", l.byte(op.In(0), 0)), fmt.Sprintf("cpy %s", l.byte(op.In(1), 0)))
				break
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

		case ir.OpLt:
			// a < b。フラグの意味 (LocCond のとき OpIf が見る):
			//   符号なし: 多バイトの減算の借り = C クリア ⇔ a < b
			//   符号付き: 減算結果の符号 (V でオーバーフローを補正) = N セット ⇔ a < b
			// どちらか一方でも符号付きなら符号付き比較 (v1 は左辺しか見ておらず、多バイトや
			// オーバーフローのある符号付き比較も壊れていた。2026-09-14 に書き直し)
			if l.inY(op.In(0)) && ir.ValLocation(op.Dst) == ir.LocCond {
				r.push(fmt.Sprintf("cpy %s", l.byte(op.In(1), 0)))
				break
			}
			if l.aHeld && ir.ValLocation(op.Dst) == ir.LocCond {
				r.push(fmt.Sprintf("ldy %s", l.byte(op.In(0), 0)), fmt.Sprintf("cpy %s", l.byte(op.In(1), 0)))
				break
			}
			labels := l.newLabels(3)
			trueLabel, endLabel, skipLabel := labels[0], labels[1], labels[2]
			size := max(ir.ValType(op.In(0)).Size, ir.ValType(op.In(1)).Size)
			signed := ir.ValType(op.In(0)).Signed || ir.ValType(op.In(1)).Signed
			if os.Getenv("FC_TRACE_SIGNED") != "" && signed {
				// 調査用: 符号付き比較の場所を列挙する
				lit0, ok0 := ir.ValIntLiteral(op.In(0))
				lit1, ok1 := ir.ValIntLiteral(op.In(1))
				fmt.Fprintf(os.Stderr, "SIGNED_LT %s mixed=%v bigliteral=%v %s:%s < %s:%s cond=%v\n", op.Pos,
					ir.ValType(op.In(0)).Signed != ir.ValType(op.In(1)).Signed, ok0 && lit0 >= 128 || ok1 && lit1 >= 128,
					ir.OperandString(op.In(0)), ir.ValType(op.In(0)), ir.OperandString(op.In(1)), ir.ValType(op.In(1)),
					ir.ValLocation(op.Dst) == ir.LocCond)
			}
			if lit, ok := ir.ValIntLiteral(op.In(1)); signed && ok && lit == 0 {
				// a < 0 (符号付き) は a の最上位バイトの符号ビットそのもの
				r.push(l.loadA(op.In(0), size-1))
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

		case ir.OpNot:
			if ir.ValLocation(op.Dst) != ir.LocCond {
				labels := l.newLabels(2)
				trueLabel, endLabel := labels[0], labels[1]
				for i := 0; i < ir.ValType(op.In(0)).Size; i++ {
					r.push(l.loadA(op.In(0), i))
					r.push(fmt.Sprintf("beq %s", trueLabel))
				}
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

		case ir.OpBitNot:
			for i := 0; i < ir.ValType(op.Dst).Size; i++ {
				r.push(l.loadA(op.In(0), i))
				r.push("eor #255")
				r.push(l.storeA(op.Dst, i))
			}

		case ir.OpAsm:
			r.push(op.Text)

		case ir.OpIndex:
			if es := ir.ValType(op.In(0)).Base.Size; es != 1 && es != 2 {
				r.push(l.indexLarge(op))
			} else if ir.ValType(op.In(1)).Size == 1 {
				// インデックスのサイズが１
				if ir.ValType(op.In(0)).Kind == types.Array && ir.ValLocation(op.In(0)) == ir.LocFrame {
					// フレーム上のローカル配列: 先頭は S + addr + X (ゼロページなので上位は 0。OpRef と同じ)
					r.push(l.loadYIdx(op.In(1), op.In(0)))
					r.push("sty <reg+0")
					r.push("txa")
					r.push("clc")
					r.push(fmt.Sprintf("adc #.LOBYTE(S+%d)", ir.ValAddress(op.In(0))))
					r.push("clc")
					r.push("adc <reg+0")
					r.push(l.storeA(op.Dst, 0))
					r.push("lda #0")
					r.push(l.storeA(op.Dst, 1))
				} else if ir.ValType(op.In(0)).Kind == types.Array {
					r.push(l.loadYIdx(op.In(1), op.In(0)))
					r.push("sty <reg+0")
					r.push("clc")
					r.push(fmt.Sprintf("lda #.LOBYTE(%s)", l.addrExpr(op.In(0))))
					r.push("adc <reg+0")
					r.push(l.storeA(op.Dst, 0))
					r.push(fmt.Sprintf("lda #.HIBYTE(%s)", l.addrExpr(op.In(0))))
					r.push("adc #0")
					r.push(l.storeA(op.Dst, 1))
				} else if ir.ValType(op.In(0)).Kind == types.Pointer {
					r.push(l.loadYIdx(op.In(1), op.In(0)))
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
				} else {
					panic("invalid index")
				}
			}

		case ir.OpRef:
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

		case ir.OpPget:
			r.push(l.pointerRead(op.In(0), 0, op.Dst, ir.ValType(op.Dst).Size))

		case ir.OpPset:
			r.push(l.pointerWrite(op.In(0), 0, op.In(1), ir.ValType(op.In(0)).Base.Size))

			// 最適化後のオペレータ
		case ir.OpIndexPget:
			if !isByteInt(ir.ValType(op.In(1))) {
				panic(&diag.Error{Msg: "16-bit index is not supported here (use a 1-byte index)"})
			}
			if ir.ValType(op.In(0)).Kind == types.Pointer {
				// ポインタ + 添字: ldy idx; lda (p),y
				base, setup := l.pointerBase(op.In(0))
				if ir.ValType(op.Dst).Size > 1 && sameStorage(op.In(0), op.Dst) {
					base, setup = "reg", []any{l.loadA(op.In(0), 0), "sta <reg+0", l.loadA(op.In(0), 1), "sta <reg+1"}
				}
				r.push(setup)
				r.push(l.loadYIdx(op.In(1), op.In(0)))
				for i := 0; i < ir.ValType(op.Dst).Size; i++ {
					if i > 0 {
						r.push("iny")
					}
					r.push(fmt.Sprintf("lda (%s),y", base))
					r.push(l.storeA(op.Dst, i))
				}
				break
			}
			r.push(l.loadYIdx(op.In(1), op.In(0)))
			for i := 0; i < ir.ValType(op.Dst).Size; i++ {
				r.push(fmt.Sprintf("lda %s+%d,y", l.toAsm(op.In(0)), i))
				r.push(l.storeA(op.Dst, i))
			}

		case ir.OpIndexPset:
			if !isByteInt(ir.ValType(op.In(1))) {
				panic(&diag.Error{Msg: "16-bit index is not supported here (use a 1-byte index)"})
			}
			if ir.ValType(op.In(0)).Kind == types.Pointer {
				base, setup := l.pointerBase(op.In(0))
				pre := append(setup, l.loadYIdx(op.In(1), op.In(0))...)
				if ir.ValType(op.In(0)).Base.Size == 1 && len(setup) == 0 {
					r.push(pre) // ldy だけなら A は壊れない
				} else {
					r.push(l.keepA(op.In(2), pre))
				}
				for i := 0; i < ir.ValType(op.In(0)).Base.Size; i++ {
					if i > 0 {
						r.push("iny")
					}
					r.push(l.loadA(op.In(2), i))
					r.push(fmt.Sprintf("sta (%s),y", base))
				}
				break
			}
			if ir.ValType(op.In(0)).Base.Size == 1 {
				r.push(l.loadYIdx(op.In(1), op.In(0)))
			} else {
				r.push(l.keepA(op.In(2), l.loadYIdx(op.In(1), op.In(0)))) // lda idx; asl; tay は A を壊す
			}
			for i := 0; i < ir.ValType(op.In(2)).Size; i++ {
				r.push(l.loadA(op.In(2), i))
				r.push(fmt.Sprintf("sta %s+%d,y", l.toAsm(op.In(0)), i))
			}

		case ir.OpFieldPget:
			// ポインタ + 定数オフセット経由の読み出し (struct のフィールド)
			off, _ := ir.ValIntLiteral(op.In(1))
			r.push(l.pointerRead(op.In(0), off, op.Dst, ir.ValType(op.Dst).Size))

		case ir.OpFieldPset:
			off, _ := ir.ValIntLiteral(op.In(1))
			r.push(l.pointerWrite(op.In(0), off, op.In(2), op.Type.Size)) // Type はフィールドの型 (値が小さいリテラルでもフィールド全体を書く)

		default:
			panic(fmt.Sprintf("unknow op %s", ir.DumpOp(op, nil)))
		}
		if restoreA {
			r.push("lda " + l.byte(op.Resident.Home, 0))
		}
		if restoreY {
			r.push("ldy " + l.byte(op.ResidentY.Home, 0))
		}
		l.res, l.resMem, l.resY, l.resYMem, l.aHeld = nil, false, nil, false, false
	}

	for _, d := range lmd.Defs {
		switch d.Kind {
		case ir.DefBlock:
			r.push(l.emitBlock(d.Sym, d.Type, d.Elems))
		default:
			panic(fmt.Sprintf("invalid lambda def kind %s", d.Kind))
		}
	}

	lines := r.flatten() // まとめた行を展開 + 空の行を削除
	// ラベル行,コメント行以外はインデントする
	for i, line := range lines {
		if reIndentExempt.MatchString(line) {
			// そのまま
		} else {
			lines[i] = "\t" + line
		}
	}

	lines = append(lines, ".endproc")

	if l.OptimizeLevel > 0 {
		lines = peepholeA(lines)
	}
	lines = l.extendJump(lines)

	lmd.Asm = lines
	return lines
}

var reIndentExempt = regexp.MustCompile(`^([.@_a-zA-Z0-9][_a-zA-Z0-9]+:|\.segment|\.proc)`)

func (l *Llc) newLabel() string {
	l.labelCount++
	return fmt.Sprintf("@%d", l.labelCount)
}

func (l *Llc) newLabels(n int) []string {
	r := make([]string, n)
	for i := range r {
		r[i] = l.newLabel()
	}
	return r
}

func (l *Llc) allocRegister(lmd *ir.Lambda) {
	if l.OptimizeLevel > 0 {
		regalloc.AllocateRegister(lmd, l.Limits)
		regalloc.DeleteUnuse(lmd)
	} else {
		// 単純なバージョンのアロケータ(debug用)
		size := 0
		for _, v := range lmd.Vars {
			if v.Kind == ir.KindLocal {
				v.Location = ir.LocFrame
				v.Address = size
				size += v.Type.Size
			}
		}
		lmd.FrameSize = size
	}
}

// ---------------------------------------------------------------
// extend_jump
// ---------------------------------------------------------------
