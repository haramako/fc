package codegen

// LLC: 中間コード (ir.go) → ca65 アセンブリ。lib/fc/llc.rb 由来。
// 行の生成順・インデント規則・extend_jump のサイズ表まで旧実装と同一。

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/ir"
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
}

func NewLlc(optimizeLevel int, u *types.Universe) *Llc {
	return &Llc{OptimizeLevel: optimizeLevel, Limits: regalloc.DefaultLimits, zero: ir.NewIntLiteral("", u.IntType(1, false), 0)}
}

// asmLines は文字列/ nil / ネストした配列を保持する行バッファ (Ruby の Array 相当)。
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
			asm.push(anyList(l.CompileLambda(d.Sym, lmd)))
		default:
			panic(fmt.Sprintf("invalid def kind %s", d.Kind))
		}
	}

	// fastcall 関数が使う FC_FASTCALL_REG の大きさを、base.asm (プロジェクトが自前で持つこともある) の .res とリンク時に突き合わせる
	fastcallNeed := 0
	for _, d := range mod.Defs {
		if d.Kind == ir.DefCode && !d.Lambda.Extern && d.Lambda.Type.Fastcall() {
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

// CompileLambda は関数1つ分のアセンブリを生成する。
func (l *Llc) CompileLambda(sym string, lmd *ir.Lambda) []string {
	l.curLambda = lmd // エラー位置の補完用 (Compile の回復点で参照するので、ここでは戻さない)
	l.curOp = nil
	l.allocRegister(lmd)
	ops := lmd.Ops
	if l.OptimizeLevel > 0 {
		ops = l.optimizePointer(lmd, ops)
	}
	lmd.Ops = ops

	r := &asmLines{}

	r.push(";;;=============================")
	r.push(fmt.Sprintf(";;; function %s", lmd.Id))
	r.push(";;;=============================")

	if seg := lmd.Segment(); seg != "" {
		r.push(fmt.Sprintf(".segment \"%s\"", seg))
	} else {
		r.push(fmt.Sprintf(".segment \"%s\"", l.codeSegment))
	}
	r.push(fmt.Sprintf(".proc %s", mangle(sym)))

	pushArgSize := 0
	pushFastcallArgSize := 0

	for opNo, op := range ops {
		if op == nil {
			continue
		}
		l.curOp = op
		// IRコメント (golden比較では除去されるため、Go版独自の形式でよい)
		cm := ir.DumpOp(op, nil)
		if len(cm) > 120 {
			cm = cm[:120]
		}
		r.push(fmt.Sprintf("; %04d: %s", opNo, cm))

		switch op.Code {

		case ir.OpLabel:
			r.push(op.Label + ":")

		case ir.OpIf:
			if ir.ValLocation(op.In(0)) == ir.LocCond {
				// コンディションレジスタの場合
				v := ir.UnderlyingValue(op.In(0))
				var asmOp string
				switch v.CondReg {
				case ir.CondZero:
					asmOp = ifElse(v.CondPositive, "bne", "beq")
				case ir.CondCarry:
					asmOp = ifElse(v.CondPositive, "bcs", "bcc")
				case ir.CondNegative:
					asmOp = ifElse(v.CondPositive, "bpl", "bmi")
				default:
					panic("invalid cond_reg")
				}
				r.push(fmt.Sprintf("%s %s", asmOp, op.Label))
			} else {
				// コンディションレジスタでない場合
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

		case ir.OpJump:
			r.push(fmt.Sprintf("jmp %s", op.Label))

		case ir.OpReturn:
			if op.In(0) != nil {
				r.push(l.load(lmd.Result, op.In(0)))
			}
			r.push("rts")

		case ir.OpPushResult:
			pushArgSize += op.Type.Size

		case ir.OpPushArg:
			for i := 0; i < op.Type.Size; i++ {
				r.push(l.loadA(op.In(0), i))
				r.push(fmt.Sprintf("sta <S+%d,x", lmd.FrameSize+pushArgSize))
				pushArgSize++
			}

		case ir.OpCall:
			for _, a := range ir.ValType(op.In(0)).Params {
				pushArgSize -= a.Size
			}
			pushArgSize -= ir.ValType(op.In(0)).Base.Size

			if op.Far {
				// 別バンクの関数: 呼び先とバンクを FC_FARCALL に置いて farcall (ターゲット側のトランポリン) を呼ぶ
				r.push(l.farCallSetup(ir.ValLiteral(op.In(0)).Symbol))
				r.push(l.callSubroutine("farcall", lmd.FrameSize+pushArgSize))
			} else if ir.ValKind(op.In(0)) == ir.KindLiteral {
				// 関数を直に呼ぶ
				r.push(l.callSubroutine(mangle(ir.ValLiteral(op.In(0)).Symbol), lmd.FrameSize+pushArgSize))
			} else {
				// 関数ポインタから呼ぶ
				r.push(l.loadA(op.In(0), 0))
				r.push("sta <reg+0")
				r.push(l.loadA(op.In(0), 1))
				r.push("sta <reg+1")
				r.push(l.callSubroutine("jsr_reg", lmd.FrameSize+pushArgSize))
			}

			// 帰り値を格納する
			if op.Dst != nil {
				for i := 0; i < ir.ValType(op.Dst).Size; i++ {
					r.push(fmt.Sprintf("lda <%d+S+%d,x", i, lmd.FrameSize))
					r.push(l.storeA(op.Dst, i))
				}
			}

		case ir.OpPushFastcallResult:
			pushFastcallArgSize += op.Type.Size

		case ir.OpPushFastcallArg:
			for i := 0; i < op.Type.Size; i++ {
				r.push(l.loadA(op.In(0), i))
				r.push(fmt.Sprintf("sta <FC_FASTCALL_REG+%d", pushFastcallArgSize))
				pushFastcallArgSize++
			}

		case ir.OpFastcall:
			pushFastcallArgSize = 0

			if op.Far {
				r.push(l.farCallSetup(ir.ValLiteral(op.In(0)).Symbol))
				r.push("jsr farcall")
			} else if ir.ValKind(op.In(0)) == ir.KindLiteral {
				// 関数を直に呼ぶ
				r.push(fmt.Sprintf("jsr %s", mangle(ir.ValLiteral(op.In(0)).Symbol)))
			} else {
				// 関数ポインタから呼ぶ
				r.push(l.loadA(op.In(0), 0))
				r.push("sta <reg+0")
				r.push(l.loadA(op.In(0), 1))
				r.push("sta <reg+1")
				r.push("jsr jsr_reg")
			}

			// 帰り値を格納する
			if op.Dst != nil {
				for i := 0; i < ir.ValType(op.Dst).Size; i++ {
					r.push(fmt.Sprintf("lda <%d+FC_FASTCALL_REG", i))
					r.push(l.storeA(op.Dst, i))
				}
			}

		case ir.OpLoad:
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

		case ir.OpAdd:
			for i := 0; i < ir.ValType(op.Dst).Size; i++ {
				if i == 0 {
					r.push("clc")
				}
				r.push(l.loadA(op.In(0), i))
				r.push(fmt.Sprintf("adc %s", l.byte(op.In(1), i)))
				r.push(l.storeA(op.Dst, i))
			}

		case ir.OpSub:
			for i := 0; i < ir.ValType(op.Dst).Size; i++ {
				if i == 0 {
					r.push("sec")
				}
				r.push(l.loadA(op.In(0), i))
				r.push(fmt.Sprintf("sbc %s", l.byte(op.In(1), i)))
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
				if ir.ValType(op.Dst).Size == 1 {
					// サイズが1
					r.push(l.loadA(op.In(0), 0))
					for k := 0; k < n; k++ {
						if signed {
							r.push("cmp #128")
						} else {
							r.push("clc")
						}
						r.push(fmt.Sprintf("%s a", rotate))
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
					r.push(fmt.Sprintf("lda #.LOBYTE(%s)", l.toAsm(op.In(0))))
					r.push("adc <reg+0")
					r.push(l.storeA(op.Dst, 0))
					r.push(fmt.Sprintf("lda #.HIBYTE(%s)", l.toAsm(op.In(0))))
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
						r.push(fmt.Sprintf("adc #.LOBYTE(%s)", l.toAsm(op.In(0))))
						r.push(l.storeA(op.Dst, 0))
						r.push("lda <reg+1")
						r.push(fmt.Sprintf("adc #.HIBYTE(%s)", l.toAsm(op.In(0))))
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
				r.push(fmt.Sprintf("lda #.LOBYTE(%s)", l.toAsm(op.In(0))))
				r.push(l.storeA(op.Dst, 0))
				r.push(fmt.Sprintf("lda #.HIBYTE(%s)", l.toAsm(op.In(0))))
				r.push(l.storeA(op.Dst, 1))
			}

		case ir.OpPget:
			r.push(l.loadA(op.In(0), 0))
			r.push("sta <reg+0")
			r.push(l.loadA(op.In(0), 1))
			r.push("sta <reg+1")
			for i := 0; i < ir.ValType(op.Dst).Size; i++ {
				r.push(fmt.Sprintf("ldy #%d", i))
				r.push("lda (reg),y")
				r.push(l.storeA(op.Dst, i))
			}

		case ir.OpPset:
			r.push(l.loadA(op.In(0), 0))
			r.push("sta <reg+0")
			r.push(l.loadA(op.In(0), 1))
			r.push("sta <reg+1")
			for i := 0; i < ir.ValType(op.In(0)).Base.Size; i++ {
				r.push(l.loadA(op.In(1), i))
				r.push(fmt.Sprintf("ldy #%d", i))
				r.push("sta (reg),y")
			}

			// 最適化後のオペレータ
		case ir.OpIndexPget:
			if !isByteInt(ir.ValType(op.In(1))) {
				panic(&diag.Error{Msg: "16-bit index is not supported here (use a 1-byte index)"})
			}
			if ir.ValType(op.In(0)).Kind != types.Array {
				panic("index_pget with non-array")
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
			if ir.ValType(op.In(0)).Kind != types.Array {
				panic("index_pset with non-array")
			}
			r.push(l.loadYIdx(op.In(1), op.In(0)))
			for i := 0; i < ir.ValType(op.In(2)).Size; i++ {
				r.push(l.loadA(op.In(2), i))
				r.push(fmt.Sprintf("sta %s+%d,y", l.toAsm(op.In(0)), i))
			}

		case ir.OpFieldPget:
			// ポインタ + 定数オフセット経由の読み出し (struct のフィールド)
			off, _ := ir.ValIntLiteral(op.In(1))
			r.push(l.loadA(op.In(0), 0))
			r.push("sta <reg+0")
			r.push(l.loadA(op.In(0), 1))
			r.push("sta <reg+1")
			for i := 0; i < ir.ValType(op.Dst).Size; i++ {
				r.push(fmt.Sprintf("ldy #%d", off+i))
				r.push("lda (reg),y")
				r.push(l.storeA(op.Dst, i))
			}

		case ir.OpFieldPset:
			off, _ := ir.ValIntLiteral(op.In(1))
			r.push(l.loadA(op.In(0), 0))
			r.push("sta <reg+0")
			r.push(l.loadA(op.In(0), 1))
			r.push("sta <reg+1")
			for i := 0; i < op.Type.Size; i++ { // Type はフィールドの型 (値が小さいリテラルでもフィールド全体を書く)
				r.push(l.loadA(op.In(2), i))
				r.push(fmt.Sprintf("ldy #%d", off+i))
				r.push("sta (reg),y")
			}

		default:
			panic(fmt.Sprintf("unknow op %s", ir.DumpOp(op, nil)))
		}
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

	lines = l.extendJump(lines)

	lmd.Asm = lines
	return lines
}

var reIndentExempt = regexp.MustCompile(`^([.@_a-zA-Z0-9][_a-zA-Z0-9]+:|\.segment|\.proc)`)

// isByteInt は 1 バイトの整数型か (旧実装の `type == int || type == sint8`)。SoA のハンドル (1 バイトのインデックス) も含む。
func isByteInt(t *types.Type) bool {
	return (t.Kind == types.Int || t.Kind == types.SoaRef) && t.Size == 1
}

func ifElse(cond bool, a, b string) string {
	if cond {
		return a
	}
	return b
}

func anyIfy(v []any) []any { return v }

func (l *Llc) loadYIdx(idx, ptr ir.Operand) []any {
	r := []any{}
	if ir.ValType(ptr).Base.Size == 1 {
		r = append(r, fmt.Sprintf("ldy %s", l.byte(idx, 0)))
	} else {
		r = append(r, fmt.Sprintf("lda %s", l.byte(idx, 0)))
		for i := 0; i < ir.ValType(ptr).Base.Size-1; i++ {
			r = append(r, "asl a")
		}
		r = append(r, "tay")
	}
	return r
}

func (l *Llc) load(to, from ir.Operand) []any {
	r := []any{}
	voidPtr := ir.ValType(to).Kind == types.Pointer && ir.ValType(to).Base.Kind == types.Void // *void にはどのポインタも入る
	if ir.ValType(to).Kind == types.Pointer && ir.ValType(from).Kind == types.Array {
		if ir.ValType(from).Base != ir.ValType(to).Base && !voidPtr {
			panic(fmt.Sprintf("can't convert from %s to %s", ir.OperandString(from), ir.OperandString(to)))
		}
		// 配列からポインタに変換
		if ir.ValLocation(from) == ir.LocFrame {
			// フレーム上のローカル配列: S + addr + X (OpRef と同じ)
			r = append(r, "txa")
			r = append(r, "clc")
			r = append(r, fmt.Sprintf("adc #.LOBYTE(S+%d)", ir.ValAddress(from)))
			r = append(r, fmt.Sprintf("sta %s", l.byte(to, 0)))
			r = append(r, "lda #0")
			r = append(r, fmt.Sprintf("sta %s", l.byte(to, 1)))
		} else {
			r = append(r, fmt.Sprintf("lda #.LOBYTE(%s)", l.toAsm(from)))
			r = append(r, fmt.Sprintf("sta %s", l.byte(to, 0)))
			r = append(r, fmt.Sprintf("lda #.HIBYTE(%s)", l.toAsm(from)))
			r = append(r, fmt.Sprintf("sta %s", l.byte(to, 1)))
		}
	} else {
		// 通常の代入
		if ir.ValType(from).Kind != types.Int && !voidPtr {
			if ir.ValType(from).Base != ir.ValType(to).Base {
				panic(fmt.Sprintf("can't convert from %s to %s", ir.OperandString(from), ir.OperandString(to)))
			}
		}
		for i := 0; i < ir.ValType(to).Size; i++ {
			r = append(r, l.loadA(from, i))
			r = append(r, l.storeA(to, i))
		}
	}
	return r
}

// loadA は Aレジスタへのロード。戻り値は string / nil / []string。
func (l *Llc) loadA(v ir.Operand, n int) any {
	if uv, ok := v.(*ir.Value); ok && uv.Location == ir.LocCond {
		if n != 0 {
			panic("load_a cond with n != 0")
		}
		switch uv.CondReg {
		case ir.CondZero:
			labels := l.newLabels(2)
			trueLabel, endLabel := labels[0], labels[1]
			return []string{
				fmt.Sprintf("%s %s", ifElse(uv.CondPositive, "beq", "bne"), trueLabel),
				"lda #0",
				fmt.Sprintf("jmp %s", endLabel),
				trueLabel + ":",
				"lda #1",
				endLabel + ":",
			}
		case ir.CondCarry:
			if uv.CondPositive {
				return []string{"lda #0", "rol a", "eor #1"}
			}
			return []string{"lda #0", "rol a"}
		case ir.CondNegative:
			labels := l.newLabels(2)
			trueLabel, endLabel := labels[0], labels[1]
			return []string{
				fmt.Sprintf("%s %s", ifElse(uv.CondPositive, "bmi", "bpl"), trueLabel),
				"lda #0",
				fmt.Sprintf("jmp %s", endLabel),
				trueLabel + ":",
				"lda #1",
				endLabel + ":",
			}
		default:
			panic("invalid cond_reg")
		}
	}
	if isValueOrCasted(v) && ir.ValLocation(v) == ir.LocA {
		if n != 0 {
			return "lda #0"
		}
		return nil
	}
	if pa, ok := v.(*ir.PointeredArray); ok && ir.ValLocation(pa.From) == ir.LocFrame {
		// フレーム上のローカル配列をポインタとして使う: 先頭は S + addr + X (上位は 0)
		if n == 0 {
			return []string{"txa", "clc", fmt.Sprintf("adc #.LOBYTE(S+%d)", ir.ValAddress(pa.From))}
		}
		return "lda #0"
	}
	return fmt.Sprintf("lda %s", l.byte(v, n))
}

func isValueOrCasted(v ir.Operand) bool {
	switch v.(type) {
	case *ir.Value, *ir.CastedValue:
		return true
	}
	return false
}

// storeA は Aレジスタからのストア。
// (Ruby版は ir.CastedValue の location==:a を考慮しない — 忠実に再現)
func (l *Llc) storeA(v ir.Operand, n int) any {
	if uv, ok := v.(*ir.Value); ok && uv.Location == ir.LocA {
		if n != 0 {
			panic("store_a with n != 0")
		}
		return nil
	}
	return fmt.Sprintf("sta %s", l.byte(v, n))
}

// farCallSetup は far call の呼び先アドレスとバンク番号 (ld65.cfg の bank = N を .bank で引く) を FC_FARCALL に置く。
func (l *Llc) farCallSetup(sym string) []any {
	s := mangle(sym)
	return []any{
		fmt.Sprintf("lda #<%s", s), "sta FC_FARCALL+0",
		fmt.Sprintf("lda #>%s", s), "sta FC_FARCALL+1",
		fmt.Sprintf("lda #<.bank(%s)", s), "sta FC_FARCALL+2",
	}
}

// callSubroutine はスタックポインタ(X)を進めて jsr する。
func (l *Llc) callSubroutine(addr string, frameSize int) any {
	if frameSize <= 4 {
		r := []string{}
		for i := 0; i < frameSize; i++ {
			r = append(r, "inx")
		}
		r = append(r, fmt.Sprintf("jsr %s", addr))
		for i := 0; i < frameSize; i++ {
			r = append(r, "dex")
		}
		return r
	}
	return fmt.Sprintf("call %s, #%d", addr, frameSize)
}

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

func (l *Llc) toAsm(v ir.Operand) string {
	if isValueOrCasted(v) {
		switch ir.ValKind(v) {
		case ir.KindLocal:
			switch ir.ValLocation(v) {
			case ir.LocFrame:
				return fmt.Sprintf("<S+%d,x", ir.ValAddress(v))
			case ir.LocReg:
				return fmt.Sprintf("<L+%d", ir.ValAddress(v))
			case ir.LocFastcallReg:
				return fmt.Sprintf("<FC_FASTCALL_REG+%d", ir.ValAddress(v))
			default:
				panic(fmt.Sprintf("invalid location %s of %s", ir.ValLocation(v), ir.OperandString(v)))
			}
		case ir.KindGlobal:
			lv := ir.ValLiteral(v)
			if lv.Symbol == "" {
				panic(fmt.Sprintf("invalid %s", ir.OperandString(v)))
			}
			if off := ir.ValOffset(v); off != 0 {
				// struct のフィールド
				return fmt.Sprintf("%s+%d", mangle(lv.Symbol), off)
			}
			return mangle(lv.Symbol)
		case ir.KindLiteral:
			lv := ir.ValLiteral(v)
			if lv.IsInt {
				return fmt.Sprintf("#%d", lv.Int)
			}
			return "#" + lv.Symbol
		default:
			panic(fmt.Sprintf("invalid v %s", ir.OperandString(v)))
		}
	}
	panic(fmt.Sprintf("invalid v = %v", v))
}

// mangle は名前をアセンブラ用の表現に変更する。
func mangle(str string) string {
	return strings.ReplaceAll(str, "$", "_D")
}

// byte は値からn番目のbyteを取得する。
func (l *Llc) byte(v ir.Operand, n int) string {
	if pa, ok := v.(*ir.PointeredArray); ok {
		if ir.ValLocation(pa.From) == ir.LocFrame {
			// 即値では表せない (loadA が扱う)。ここに来るのは未対応の経路
			panic(&diag.Error{Msg: "local array used as pointer in an unsupported position"})
		}
		switch n {
		case 0:
			return fmt.Sprintf("#.LOBYTE(%s)", l.toAsm(pa.From))
		case 1:
			return fmt.Sprintf("#.HIBYTE(%s)", l.toAsm(pa.From))
		default:
			panic("invalid byte index for ir.PointeredArray")
		}
	}
	if cv, ok := v.(*ir.CastedValue); ok {
		if lv := ir.ValLiteral(cv); lv != nil && lv.Kind == ir.KindLiteral {
			// リテラルの一部 (struct のフィールド / SoA のバイト分割): リテラルそのものの n+offset バイト目
			if n < cv.Type.Size {
				return l.byte(lv, n+ir.ValOffset(cv))
			}
			return "#0"
		}
		if n < cv.Type.Size && n+cv.Offset < ir.ValType(cv.From).Size {
			return fmt.Sprintf("%d+%s", n, l.toAsm(cv))
		}
		return "#0" // 符号拡張は、:sign_extension オペレータで行うので、存在しないbyteは0扱い
	}
	if ir.ValKind(v) == ir.KindLiteral {
		lv := ir.ValLiteral(v)
		if lv.IsInt {
			return fmt.Sprintf("#%d", ir.FloorMod(ir.Shr(lv.Int, n*8), 256))
		}
		switch n {
		case 0:
			return fmt.Sprintf("#.LOBYTE(%s)", mangle(lv.Symbol))
		case 1:
			return fmt.Sprintf("#.HIBYTE(%s)", mangle(lv.Symbol))
		default:
			panic("invalid byte index for symbol")
		}
	}
	if n < ir.ValType(v).Size {
		return fmt.Sprintf("%d+%s", n, l.toAsm(v))
	}
	return "#0" // 符号拡張は、:sign_extension オペレータで行うので、存在しないbyteは0扱い
}

// emitBlock は v を .db/.dw に変換する。
func (l *Llc) emitBlock(sym string, typ *types.Type, val []ir.Operand) []any {
	r := []any{}
	r = append(r, mangle(sym)+":")
	if typ.Kind == types.Struct || (typ.Kind == types.Array && (typ.Base.Kind == types.Struct || typ.Base.Kind == types.Array)) {
		// struct / 入れ子の配列: 要素ごとに型に従って .byte / .word を出す
		return append(r, l.emitData(typ, val)...)
	}
	var op string
	var limit int
	switch typ.Base.Size {
	case 1:
		op = ".byte"
		limit = 256
	case 2:
		op = ".word"
		limit = 65536
	default:
		panic("invalid block base size")
	}
	for s := 0; s < len(val); s += 16 {
		e := min(s+16, len(val))
		parts := make([]string, 0, e-s)
		for _, elem := range val[s:e] {
			lv := ir.ValLiteral(elem)
			switch {
			case lv != nil && lv.Kind == ir.KindLiteral && lv.IsInt:
				parts = append(parts, fmt.Sprintf("%d", ir.FloorMod(lv.Int, limit)))
			case lv != nil && lv.Kind == ir.KindLiteral:
				parts = append(parts, lv.Symbol)
			default:
				parts = append(parts, l.toAsm(elem))
			}
		}
		r = append(r, fmt.Sprintf("\t%s %s", op, strings.Join(parts, ",")))
	}
	return r
}

// emitData は struct / 配列の定数データを型に従って平坦に出力する (1 要素 1 行)。
func (l *Llc) emitData(typ *types.Type, val []ir.Operand) []any {
	r := []any{}
	scalar := func(t *types.Type, v ir.Operand) {
		op, limit := ".byte", 256
		if t.Size == 2 {
			op, limit = ".word", 65536
		} else if t.Size != 1 {
			panic(fmt.Sprintf("invalid data element size %d", t.Size))
		}
		lv := ir.ValLiteral(v)
		switch {
		case lv != nil && lv.Kind == ir.KindLiteral && lv.IsInt:
			r = append(r, fmt.Sprintf("\t%s %d", op, ir.FloorMod(lv.Int, limit)))
		case lv != nil && lv.Kind == ir.KindLiteral:
			r = append(r, fmt.Sprintf("\t%s %s", op, lv.Symbol))
		default:
			r = append(r, fmt.Sprintf("\t%s %s", op, l.toAsm(v)))
		}
	}
	var elem func(t *types.Type, v ir.Operand)
	elem = func(t *types.Type, v ir.Operand) {
		switch t.Kind {
		case types.Struct:
			lv := ir.ValLiteral(v)
			if lv == nil || lv.Kind != ir.KindArrayLiteral || len(lv.Elems) != len(t.Fields) {
				panic(&diag.Error{Msg: fmt.Sprintf("invalid constant for struct %s", t.Name)})
			}
			for i, f := range t.Fields {
				elem(f.Type, lv.Elems[i])
			}
		case types.Array:
			lv := ir.ValLiteral(v)
			if lv == nil || lv.Kind != ir.KindArrayLiteral {
				panic(&diag.Error{Msg: fmt.Sprintf("invalid constant for %s", t)})
			}
			if t.Length >= 0 && len(lv.Elems) != t.Length {
				panic(&diag.Error{Msg: fmt.Sprintf("array %s has %d elements but %d given", t, t.Length, len(lv.Elems))})
			}
			for _, e := range lv.Elems {
				elem(t.Base, e)
			}
		default:
			scalar(t, v)
		}
	}
	if typ.Kind == types.Struct {
		elem(typ, ir.NewArrayLiteral("", typ, val))
	} else {
		for _, v := range val {
			elem(typ.Base, v)
		}
	}
	return r
}

// indexLarge は要素サイズが 1・2 以外 (struct の配列) の index: Dst = 先頭 + idx * size。
// idx * size は reg+2,reg+3 に 16 ビットで、size のビットごとの shift-add で求める (size は定数)。
func (l *Llc) indexLarge(op *ir.Op) []any {
	arr, idx := op.In(0), op.In(1)
	size := ir.ValType(arr).Base.Size
	r := []any{}
	// reg+0,1 = idx (16 ビット)。reg+2,3 = 積
	r = append(r, l.loadA(idx, 0), "sta <reg+0", l.loadA(idx, 1), "sta <reg+1")
	top := 15
	for top >= 0 && size&(1<<top) == 0 {
		top--
	}
	r = append(r, "lda <reg+0", "sta <reg+2", "lda <reg+1", "sta <reg+3")
	for bit := top - 1; bit >= 0; bit-- {
		r = append(r, "asl <reg+2", "rol <reg+3")
		if size&(1<<bit) != 0 {
			r = append(r, "clc", "lda <reg+2", "adc <reg+0", "sta <reg+2", "lda <reg+3", "adc <reg+1", "sta <reg+3")
		}
	}
	// 先頭アドレスを足す
	switch {
	case ir.ValType(arr).Kind == types.Array && ir.ValLocation(arr) == ir.LocFrame:
		r = append(r, "txa", "clc", fmt.Sprintf("adc #.LOBYTE(S+%d)", ir.ValAddress(arr)), "clc", "adc <reg+2",
			l.storeA(op.Dst, 0), "lda <reg+3", "adc #0", l.storeA(op.Dst, 1))
	case ir.ValType(arr).Kind == types.Array:
		r = append(r, "clc", "lda <reg+2", fmt.Sprintf("adc #.LOBYTE(%s)", l.toAsm(arr)), l.storeA(op.Dst, 0),
			"lda <reg+3", fmt.Sprintf("adc #.HIBYTE(%s)", l.toAsm(arr)), l.storeA(op.Dst, 1))
	case ir.ValType(arr).Kind == types.Pointer:
		r = append(r, "clc", l.loadA(arr, 0), "adc <reg+2", l.storeA(op.Dst, 0),
			l.loadA(arr, 1), "adc <reg+3", l.storeA(op.Dst, 1))
	default:
		panic("invalid index")
	}
	return r
}

// ---------------------------------------------------------------
// 一部の複雑なオペレータ用
// ---------------------------------------------------------------

// mulDivMod は mul, div, mod のコード生成 (定数の場合の最適化つき)。
func (l *Llc) mulDivMod(op *ir.Op) []any {
	r := []any{}
	dst, s0, s1 := op.Dst, op.In(0), op.In(1)
	op3int, op3IsInt := ir.ValIntLiteral(s1)
	isPow2 := false
	if op3IsInt {
		switch op3int {
		case 1, 2, 4, 8, 16, 32, 64, 128:
			isPow2 = true
		}
	}
	if op3IsInt && op3int == 0 {
		// 0の場合
		switch op.Code {
		case ir.OpMul:
			r = append(r, anyIfy(l.load(dst, l.zero)))
		case ir.OpDiv, ir.OpMod:
			panic(&diag.Error{Msg: "div by 0"})
		}
	} else if op3IsInt && isPow2 && ir.ValType(dst).Size == 1 {
		// 定数(1byte)の場合の最適化
		n := log2(op3int)
		r = append(r, l.loadA(s0, 0))
		switch op.Code {
		case ir.OpMul:
			for i := 0; i < n; i++ {
				r = append(r, "asl a")
			}
		case ir.OpDiv:
			if ir.ValType(s0).Signed {
				labels := l.newLabels(2)
				negativeLabel, endLabel := labels[0], labels[1]
				r = append(r, fmt.Sprintf("bmi %s", negativeLabel))
				for i := 0; i < n; i++ {
					r = append(r, "lsr a")
				}
				r = append(r, fmt.Sprintf("jmp %s", endLabel))
				r = append(r, negativeLabel+":")
				for i := 0; i < n; i++ {
					r = append(r, "lsr a")
				}
				r = append(r, fmt.Sprintf("ora #%d", 256-pow2(8-n)))
				r = append(r, endLabel+":")
			} else {
				for i := 0; i < n; i++ {
					r = append(r, "lsr a")
				}
			}
		case ir.OpMod:
			r = append(r, fmt.Sprintf("and #%d", op3int-1))
		}
		r = append(r, l.storeA(dst, 0))
	} else if op3IsInt && isPow2 && ir.ValType(dst).Size > 1 {
		// 定数(2byte以上)の場合の最適化
		n := log2(op3int)
		size := ir.ValType(dst).Size
		switch op.Code {
		case ir.OpMul:
			r = append(r, anyIfy(l.load(dst, s0)))
			for k := 0; k < n; k++ {
				for i := 0; i < size; i++ {
					rot := ifElse(i == 0, "asl", "rol")
					r = append(r, fmt.Sprintf("%s %s", rot, l.byte(dst, i)))
				}
			}
		case ir.OpDiv:
			r = append(r, anyIfy(l.load(dst, s0)))
			for k := 0; k < n; k++ {
				for i := size - 1; i >= 0; i-- {
					var rot string
					if ir.ValType(dst).Signed {
						rot = "ror"
						if i == 0 {
							r = append(r, "cmp $80")
						}
					} else {
						rot = ifElse(i == size-1, "lsr", "ror")
					}
					r = append(r, fmt.Sprintf("%s %s", rot, l.byte(dst, i)))
				}
			}
		case ir.OpMod:
			for i := 0; i < size; i++ {
				r = append(r, l.loadA(s0, i))
				r = append(r, fmt.Sprintf("and #%d", ir.FloorMod(ir.Shr(op3int-1, i*8), 256)))
				r = append(r, l.storeA(dst, i))
			}
		}
	} else {
		// 定数でない場合
		for i := 0; i < ir.ValType(dst).Size; i++ {
			r = append(r, l.loadA(s0, i))
			r = append(r, fmt.Sprintf("sta <reg+0+%d", i))
			r = append(r, l.loadA(s1, i))
			r = append(r, fmt.Sprintf("sta <reg+2+%d", i))
		}
		if ir.ValType(dst).Size == 1 {
			if ir.ValType(dst).Signed {
				r = append(r, fmt.Sprintf("jsr __%s_8s", op.Code.String()))
			} else {
				r = append(r, fmt.Sprintf("jsr __%s_8", op.Code.String()))
			}
		} else {
			r = append(r, fmt.Sprintf("jsr __%s_16", op.Code.String()))
		}
		for i := 0; i < ir.ValType(dst).Size; i++ {
			r = append(r, fmt.Sprintf("lda <reg+4+%d", i))
			r = append(r, l.storeA(dst, i))
		}
	}
	return r
}

func log2(n int) int {
	r := 0
	for n > 1 {
		n >>= 1
		r++
	}
	return r
}

func pow2(n int) int {
	return 1 << uint(n)
}

// ---------------------------------------------------------------
// レジスター割り当て
// ---------------------------------------------------------------

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
// オプティマイザ
// ---------------------------------------------------------------

// optimizePointer は index->pget, index->pset の組み合わせを合成する。
func (l *Llc) optimizePointer(lmd *ir.Lambda, ops []*ir.Op) []*ir.Op {
	ops = append([]*ir.Op{}, ops...)

	// add(ポインタ, 定数) + pget/pset の最適化 (struct のフィールドをポインタ経由で触る): ldy #off; lda (reg),y
	for i, op := range ops {
		if op == nil || op.Code != ir.OpAdd || i+1 >= len(ops) || ops[i+1] == nil {
			continue
		}
		nextOp := ops[i+1]
		ptr, off := op.Src[0], op.Src[1]
		k, isLit := ir.ValIntLiteral(off)
		tmp := ir.UnderlyingValue(op.Dst)
		if !isLit || k < 0 || k > 255 || ir.ValType(ptr).Kind != types.Pointer || ir.ValType(ptr).Size != 2 ||
			tmp == nil || tmp.LocalType != ir.LTTemp || tmp.LiveRange == nil || tmp.LiveRange.Max-tmp.LiveRange.Min != 1 {
			continue
		}
		switch nextOp.Code {
		case ir.OpPget:
			if isSameOperand(op.Dst, nextOp.Src[0]) && k+ir.ValType(nextOp.Dst).Size <= 256 {
				ops[i] = &ir.Op{Code: ir.OpFieldPget, Dst: nextOp.Dst, Src: []ir.Operand{ptr, off}, Pos: nextOp.Pos}
				ops[i+1] = nil
			}
		case ir.OpPset:
			if isSameOperand(op.Dst, nextOp.Src[0]) && k+ir.ValType(op.Dst).Base.Size <= 256 {
				ops[i] = &ir.Op{Code: ir.OpFieldPset, Src: []ir.Operand{ptr, off, nextOp.Src[1]}, Type: ir.ValType(op.Dst).Base, Pos: nextOp.Pos}
				ops[i+1] = nil
			}
		}
	}

	// index + pget/pset の最適化
	for i, op := range ops {
		if op == nil || op.Code != ir.OpIndex {
			continue
		}
		if i+1 >= len(ops) {
			continue
		}
		nextOp := ops[i+1]
		if nextOp == nil {
			continue
		}
		arr, idx := op.Src[0], op.Src[1]
		if es := ir.ValType(arr).Base.Size; es != 1 && es != 2 {
			continue // struct の配列 (要素サイズが 1・2 以外) は sym+i,y の形にできない
		}
		switch nextOp.Code {
		case ir.OpPget:
			if isSameOperand(op.Dst, nextOp.Src[0]) && // 同じ変数を連続で使っていて
				ir.ValKind(arr) == ir.KindGlobal && // 単純なシンボルで
				ir.ValType(arr).Kind == types.Array && // 配列 (グローバルのポインタ変数は sym+i,y では読めない)
				ir.ValLocalType(op.Dst) == ir.LTTemp && // その変数をそこでしか使っていない
				ir.ValType(idx).Size == 1 { // インデックスのサイズが1byte
				ops[i] = &ir.Op{Code: ir.OpIndexPget, Dst: nextOp.Dst, Src: []ir.Operand{arr, idx}, Pos: nextOp.Pos}
				ops[i+1] = nil
			}
		case ir.OpPset:
			if isSameOperand(op.Dst, nextOp.Src[0]) &&
				ir.ValKind(arr) == ir.KindGlobal &&
				ir.ValType(arr).Kind == types.Array &&
				ir.ValLocalType(op.Dst) == ir.LTTemp &&
				ir.ValType(idx).Size == 1 {
				ops[i] = &ir.Op{Code: ir.OpIndexPset, Src: []ir.Operand{arr, idx, nextOp.Src[1]}, Pos: nextOp.Pos}
				ops[i+1] = nil
			}
		}
	}
	return ops
}

// isSameOperand は Ruby の == (ir.CastedValue は Delegator 経由で from と比較) 相当。
func isSameOperand(a, b ir.Operand) bool {
	ua, ub := ir.UnderlyingValue(a), ir.UnderlyingValue(b)
	if ua != nil && ub != nil {
		return ua == ub
	}
	return a == b
}

// ---------------------------------------------------------------
// extend_jump
// ---------------------------------------------------------------

var (
	reEjLabel  = regexp.MustCompile(`^([@._a-zA-Z0-9][_a-zA-Z0-9]+):`)
	reEjInstr  = regexp.MustCompile(`^\s+(\w+)`)
	reEjBranch = regexp.MustCompile(`^\s+(\w+)\s+([@._a-zA-Z0-9][_a-zA-Z0-9]+)`)
)

// extendJump はブランチ命令のジャンプ先が +-127 より遠いかもしれない場合、2段階ジャンプに変換する。
func (l *Llc) extendJump(asm []string) []string {
	opSize := map[string]int{"call": 10}
	branchOps := map[string]string{
		"bcc": "bcs", "bcs": "bcc", "beq": "bne", "bne": "beq", "bmi": "bpl", "bpl": "bmi",
	}
	for _, op := range []string{"brk", "clc", "cld", "clv", "dex", "dey", "inx", "iny", "nop", "pha", "php", "pla", "plp", "rti", "rts",
		"sec", "sec", "sed", "sei", "tax", "tay", "tsx", "txa", "txs", "tya"} {
		opSize[op] = 1
	}
	for _, op := range []string{"adc", "and", "asl", "cmp", "cpx", "cpy", "dec", "eor", "inc", "jmp", "jsr",
		"lda", "ldx", "ldy", "lsr", "ora", "rol", "ror", "sbc", "sta", "stx", "sty"} {
		opSize[op] = 3
	}
	for _, op := range []string{"bcc", "bcs", "beq", "bit", "bmi", "bne", "bpl", "bvc", "bvs"} {
		opSize[op] = 5 // 分割して増えるかもしれない
	}

	// 各ラベルのアドレス候補を求める
	var addrs []int
	labels := map[string]int{}
	n := 0
	for _, line := range asm {
		addrs = append(addrs, n)
		if m := reEjLabel.FindStringSubmatch(line); m != nil {
			labels[m[1]] = n
		} else if m := reEjInstr.FindStringSubmatch(line); m != nil {
			if size, ok := opSize[m[1]]; ok {
				n += size
			} else {
				n += 10 // 知らない命令は、とりえあず10byteとする
			}
		}
	}

	// 書き換えが必要なジャンプを書き換える
	result := &asmLines{}
	for i, line := range asm {
		addr := addrs[i]
		replaced := false
		if m := reEjBranch.FindStringSubmatch(line); m != nil {
			if inv, ok := branchOps[m[1]]; ok {
				jumpTo, hasLabel := labels[m[2]]
				if hasLabel && abs(jumpTo-addr) >= 127 {
					label := l.newLabel()
					result.push([]string{
						fmt.Sprintf("\t%s %s", inv, label),
						fmt.Sprintf("\tjmp %s", m[2]),
						label + ":",
					})
					replaced = true
				}
			}
		}
		if !replaced {
			result.push(line)
		}
	}

	return result.flatten()
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
