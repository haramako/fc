package fc

// lib/fc/llc.rb の Low-Level コンパイラ (中間コード → ca65アセンブリ) の厳密移植。
// 行の生成順・インデント規則・extend_jump のサイズ表まで 1:1 で再現する。

import (
	"fmt"
	"regexp"
	"strings"
)

type Llc struct {
	OptimizeLevel int
	labelCount    int
	codeSegment   string
}

func NewLlc(optimizeLevel int) *Llc {
	return &Llc{OptimizeLevel: optimizeLevel}
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
func (l *Llc) Compile(mod *Module) ([]string, []string) {
	l.labelCount = 0
	l.codeSegment = ToS(mod.Id)

	inc := &asmLines{}
	asm := &asmLines{}
	asm.push("\t.setcpu \"6502\"")
	asm.push("\t.include \"macro.inc\"")
	asm.push(fmt.Sprintf("__MODULE_%s__ = 1", strings.ToUpper(ToS(mod.Id))))

	inc.push(fmt.Sprintf(".ifndef __MODULE_%s__", strings.ToUpper(ToS(mod.Id))))
	inc.push(fmt.Sprintf("__MODULE_%s__ = 1", strings.ToUpper(ToS(mod.Id))))

	asm.push(fmt.Sprintf(".segment \"%s\"", l.codeSegment)) // dummy

	for _, e := range mod.Modules.Entries() {
		m := e.Val.(*Module)
		inc.push(fmt.Sprintf("\t.include \"_%s.inc\"", ToS(m.Id)))
		asm.push(fmt.Sprintf("\t.include \"_%s.inc\"", ToS(m.Id)))
	}

	// include(.asm)の処理
	for _, file := range mod.IncludeAsms {
		asm.push(fmt.Sprintf("\t.include \"%s\"", file))
	}

	for _, d := range mod.Defs {
		switch d.Kind {
		case "equ":
			val := d.Val
			if s, ok := val.(Sym); ok {
				val = mangle(string(s))
			}
			inc.push(fmt.Sprintf("%s = %s", mangle(ToS(d.Sym)), ToS(val)))
			asm.push(fmt.Sprintf("%s = %s", mangle(ToS(d.Sym)), ToS(val)))
		case "bss":
			inc.push(fmt.Sprintf("\t.import %s", mangle(ToS(d.Sym))))
			asm.push(fmt.Sprintf("\t.export %s", mangle(ToS(d.Sym))))
			seg := d.Val.(*OMap).GetOr(Sym("segment"))
			if seg != nil {
				asm.push(fmt.Sprintf(".segment \"%s\"", ToS(seg)))
			} else {
				asm.push(".segment \"BSS\"")
			}
			asm.push(fmt.Sprintf("%s: .res %d", mangle(ToS(d.Sym)), d.Type.Size))
		case "block":
			inc.push(fmt.Sprintf("\t.import %s", mangle(ToS(d.Sym))))
			asm.push(fmt.Sprintf("\t.export %s", mangle(ToS(d.Sym))))
			asm.push(fmt.Sprintf(".segment \"%s\"", l.codeSegment))
			asm.push(l.emitBlock(d.Sym, d.Type, d.Val.([]any)))
		case "code":
			inc.push(fmt.Sprintf("\t.import %s", mangle(ToS(d.Sym))))
			asm.push(fmt.Sprintf("\t.export %s", mangle(ToS(d.Sym))))
			lmd := d.Val.(*Lambda)
			if truthy(lmd.Opt.GetOr(Sym("extern"))) {
				continue
			}
			asm.push(anyList(l.CompileLambda(d.Sym, lmd)))
		default:
			panic(fmt.Sprintf("invalid def kind %s", d.Kind))
		}
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

	return asm.flatten(), inc.flatten()
}

func anyList(ss []string) []any {
	r := make([]any, len(ss))
	for i, s := range ss {
		r[i] = s
	}
	return r
}

// CompileLambda は関数1つ分のアセンブリを生成する。
func (l *Llc) CompileLambda(sym any, lmd *Lambda) []string {
	l.allocRegister(lmd)
	ops := lmd.Ops
	if l.OptimizeLevel > 0 {
		ops = l.optimizePointer(lmd, ops)
	}
	lmd.Ops = ops

	r := &asmLines{}

	r.push(";;;=============================")
	r.push(fmt.Sprintf(";;; function %s", ToS(lmd.Id)))
	r.push(";;;=============================")

	if seg := lmd.Opt.GetOr(Sym("segment")); seg != nil {
		r.push(fmt.Sprintf(".segment \"%s\"", ToS(seg)))
	} else {
		r.push(fmt.Sprintf(".segment \"%s\"", l.codeSegment))
	}
	r.push(fmt.Sprintf(".proc %s", mangle(ToS(sym))))

	pushArgSize := 0
	pushFastcallArgSize := 0

	for opNo, op := range ops {
		if op == nil {
			continue
		}
		// IRコメント (golden比較では除去されるため、Go版独自の形式でよい)
		cm := dumpOp(op, nil)
		if len(cm) > 120 {
			cm = cm[:120]
		}
		r.push(fmt.Sprintf("; %04d: %s", opNo, cm))

		switch op[0] {

		case Sym("label"):
			r.push(op[1].(string) + ":")

		case Sym("if"):
			if ValLocation(op[1]) == "cond" {
				// コンディションレジスタの場合
				v := UnderlyingValue(op[1])
				var asmOp string
				switch v.CondReg {
				case "zero":
					asmOp = ifElse(v.CondPositive, "bne", "beq")
				case "carry":
					asmOp = ifElse(v.CondPositive, "bcs", "bcc")
				case "negative":
					asmOp = ifElse(v.CondPositive, "bpl", "bmi")
				default:
					panic("invalid cond_reg")
				}
				r.push(fmt.Sprintf("%s %s", asmOp, op[2]))
			} else {
				// コンディションレジスタでない場合
				thenLabel := l.newLabel()
				size := ValType(op[1]).Size
				for i := 0; i < size; i++ {
					r.push(l.loadA(op[1], i))
					if i == size-1 {
						r.push(fmt.Sprintf("beq %s", op[2]))
					} else {
						r.push(fmt.Sprintf("bne %s", thenLabel))
					}
				}
				r.push(thenLabel + ":")
			}

		case Sym("jump"):
			r.push(fmt.Sprintf("jmp %s", op[1]))

		case Sym("return"):
			if at(op, 1) != nil {
				r.push(l.load(lmd.Result, op[1]))
			}
			r.push("rts")

		case Sym("push_result"):
			pushArgSize += op[1].(*Type).Size

		case Sym("push_arg"):
			for i := 0; i < op[1].(*Type).Size; i++ {
				r.push(l.loadA(op[2], i))
				r.push(fmt.Sprintf("sta <S+%d,x", lmd.FrameSize+pushArgSize))
				pushArgSize++
			}

		case Sym("call"):
			for _, a := range ValType(op[2]).Args {
				pushArgSize -= a.Size
			}
			pushArgSize -= ValType(op[2]).Base.Size

			if ValKind(op[2]) == "literal" {
				// 関数を直に呼ぶ
				r.push(l.callSubroutine(mangle(ToS(ValVal(op[2]))), lmd.FrameSize+pushArgSize))
			} else {
				// 関数ポインタから呼ぶ
				r.push(l.loadA(op[2], 0))
				r.push("sta <reg+0")
				r.push(l.loadA(op[2], 1))
				r.push("sta <reg+1")
				r.push(l.callSubroutine("jsr_reg", lmd.FrameSize+pushArgSize))
			}

			// 帰り値を格納する
			if at(op, 1) != nil {
				for i := 0; i < ValType(op[1]).Size; i++ {
					r.push(fmt.Sprintf("lda <%d+S+%d,x", i, lmd.FrameSize))
					r.push(l.storeA(op[1], i))
				}
			}

		case Sym("push_fastcall_result"):
			pushFastcallArgSize += op[1].(*Type).Size

		case Sym("push_fastcall_arg"):
			for i := 0; i < op[1].(*Type).Size; i++ {
				r.push(l.loadA(op[2], i))
				r.push(fmt.Sprintf("sta <FC_FASTCALL_REG+%d", pushFastcallArgSize))
				pushFastcallArgSize++
			}

		case Sym("fastcall"):
			pushFastcallArgSize = 0

			if ValKind(op[2]) == "literal" {
				// 関数を直に呼ぶ
				r.push(fmt.Sprintf("jsr %s", mangle(ToS(ValVal(op[2])))))
			} else {
				// 関数ポインタから呼ぶ
				r.push(l.loadA(op[2], 0))
				r.push("sta <reg+0")
				r.push(l.loadA(op[2], 1))
				r.push("sta <reg+1")
				r.push("jsr jsr_reg")
			}

			// 帰り値を格納する
			if at(op, 1) != nil {
				for i := 0; i < ValType(op[1]).Size; i++ {
					r.push(fmt.Sprintf("lda <%d+FC_FASTCALL_REG", i))
					r.push(l.storeA(op[1], i))
				}
			}

		case Sym("load"):
			r.push(l.load(op[1], op[2]))

		case Sym("sign_extension"):
			labels := l.newLabels(2)
			plsLabel, endLabel := labels[0], labels[1]
			// TODO: サイズ1->2以上の場合を実装すること、いまはそれしかないから十分だけど
			r.push(l.loadA(op[2], 0))
			r.push(l.storeA(op[1], 0))
			r.push(fmt.Sprintf("bpl %s", plsLabel))
			r.push("lda #255")
			r.push(fmt.Sprintf("jmp %s", endLabel))
			r.push(plsLabel + ":")
			r.push("lda #0")
			r.push(endLabel + ":")
			r.push(l.storeA(op[1], 1))

		case Sym("add"):
			for i := 0; i < ValType(op[1]).Size; i++ {
				if i == 0 {
					r.push("clc")
				}
				r.push(l.loadA(op[2], i))
				r.push(fmt.Sprintf("adc %s", l.byte(op[3], i)))
				r.push(l.storeA(op[1], i))
			}

		case Sym("sub"):
			for i := 0; i < ValType(op[1]).Size; i++ {
				if i == 0 {
					r.push("sec")
				}
				r.push(l.loadA(op[2], i))
				r.push(fmt.Sprintf("sbc %s", l.byte(op[3], i)))
				r.push(l.storeA(op[1], i))
			}

		case Sym("and"), Sym("or"), Sym("xor"):
			for i := 0; i < ValType(op[1]).Size; i++ {
				r.push(l.loadA(op[2], i))
				as := map[Sym]string{"and": "and", "or": "ora", "xor": "eor"}[op[0].(Sym)]
				r.push(fmt.Sprintf("%s %s", as, l.byte(op[3], i)))
				r.push(l.storeA(op[1], i))
			}

		case Sym("mul"), Sym("div"), Sym("mod"):
			r.push(l.mulDivMod(op))

		case Sym("shift_left"), Sym("shift_right"):
			signed := ValType(op[2]).Signed
			rotate := ifElse(eqAny(op[0], Sym("shift_left")), "rol", "ror")
			if n, ok := ValVal(op[3]).(int); ok {
				// 定数の場合
				if ValType(op[1]).Size == 1 {
					// サイズが1
					r.push(l.loadA(op[2], 0))
					for k := 0; k < n; k++ {
						if signed {
							r.push("cmp #128")
						} else {
							r.push("clc")
						}
						r.push(fmt.Sprintf("%s a", rotate))
					}
					r.push(l.storeA(op[1], 0))
				} else {
					// サイズが２以上
					r.push(anyIfy(l.load(op[1], op[2])))
					for k := 0; k < n; k++ {
						if eqAny(op[0], Sym("shift_left")) {
							// 左シフト
							for i := 0; i < ValType(op[1]).Size; i++ {
								r.push(l.loadA(op[1], i))
								if i == 0 {
									r.push("clc")
								}
								r.push("rol a")
								r.push(l.storeA(op[1], i))
							}
						} else {
							// 右シフト
							for i := ValType(op[1]).Size - 1; i >= 0; i-- {
								r.push(l.loadA(op[1], i))
								if i == ValType(op[1]).Size-1 {
									if signed {
										r.push("cmp #128")
									} else {
										r.push("clc")
									}
								}
								r.push("ror a")
								r.push(l.storeA(op[1], i))
							}
						}
					}
				}
			} else {
				// 定数でない場合
				// TODO: もうちょっと整理して効率よくできるはず
				if ValType(op[1]).Size != 1 {
					panic("shift with non-const count and size != 1")
				}
				labels := l.newLabels(2)
				loopLabel, endLabel := labels[0], labels[1]
				r.push(l.loadA(op[3], 0))
				r.push("tay")
				r.push(l.loadA(op[2], 0))
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
				r.push(l.storeA(op[1], 0))
			}

		case Sym("uminus"):
			if ValType(op[1]).Kind != "int" {
				panic("uminus with non-int")
			}
			for i := 0; i < ValType(op[1]).Size; i++ {
				if i == 0 {
					r.push("sec")
				}
				r.push("lda #0")
				if ValType(op[2]).Size > i {
					r.push(fmt.Sprintf("sbc %s", l.byte(op[2], i)))
				}
				r.push(l.storeA(op[1], i))
			}

		case Sym("eq"):
			labels := l.newLabels(2)
			falseLabel, endLabel := labels[0], labels[1]
			size := max(ValType(op[2]).Size, ValType(op[3]).Size)
			for i := 0; i < size; i++ {
				r.push(l.loadA(op[2], i))
				r.push(fmt.Sprintf("cmp %s", l.byte(op[3], i)))
				r.push(fmt.Sprintf("bne %s", falseLabel))
			}
			if ValLocation(op[1]) == "cond" {
				r.lines = r.lines[:len(r.lines)-1] // 最後のbneを消す
				if size != 1 {
					r.push(falseLabel + ":")
				}
			} else {
				// falseのとき
				r.push("lda #1")
				r.push(l.storeA(op[1], 0))
				r.push(fmt.Sprintf("jmp %s", endLabel))
				// trueのとき
				r.push(falseLabel + ":")
				r.push("lda #0")
				r.push(l.storeA(op[1], 0))
				r.push(endLabel + ":")
			}

		case Sym("lt"):
			labels := l.newLabels(2)
			trueLabel, endLabel := labels[0], labels[1]
			size := max(ValType(op[2]).Size, ValType(op[3]).Size)
			// Ruby版は `signed = op[2].type.signed or op[3].type.signed` で、
			// `or` の優先順位により op[3] 側は代入に含まれない (バグの忠実な再現)
			signed := ValType(op[2]).Signed
			for i := size - 1; i >= 0; i-- {
				r.push(l.loadA(op[2], i))
				r.push(fmt.Sprintf("cmp %s", l.byte(op[3], i)))
				if signed && i == size-1 {
					r.push(fmt.Sprintf("bmi %s", trueLabel))
				} else {
					r.push(fmt.Sprintf("bcc %s", trueLabel))
				}
			}
			if ValLocation(op[1]) == "cond" {
				r.lines = r.lines[:len(r.lines)-1] // 最後のbccを消す
				if size != 1 {
					r.push(trueLabel + ":")
				}
			} else {
				// falseのとき
				r.push("lda #0")
				r.push(l.storeA(op[1], 0))
				r.push(fmt.Sprintf("jmp %s", endLabel))
				// trueのとき
				r.push(trueLabel + ":")
				r.push("lda #1")
				r.push(l.storeA(op[1], 0))
				r.push(endLabel + ":")
			}

		case Sym("not"):
			if ValLocation(op[1]) != "cond" {
				labels := l.newLabels(2)
				trueLabel, endLabel := labels[0], labels[1]
				for i := 0; i < ValType(op[2]).Size; i++ {
					r.push(l.loadA(op[2], i))
					r.push(fmt.Sprintf("beq %s", trueLabel))
				}
				// falseのとき
				r.push("lda #0")
				r.push(l.storeA(op[1], 0))
				r.push(fmt.Sprintf("jmp %s", endLabel))
				// trueのとき
				r.push(trueLabel + ":")
				r.push("lda #1")
				r.push(l.storeA(op[1], 0))
				r.push(endLabel + ":")
			}

		case Sym("asm"):
			r.push(op[1].(string))

		case Sym("index"):
			if ValType(op[3]).Size == 1 {
				// インデックスのサイズが１
				if ValType(op[2]).Kind == "array" {
					r.push(l.loadYIdx(op[3], op[2]))
					r.push("sty <reg+0")
					r.push("clc")
					r.push(fmt.Sprintf("lda #.LOBYTE(%s)", l.toAsm(op[2])))
					r.push("adc <reg+0")
					r.push(l.storeA(op[1], 0))
					r.push(fmt.Sprintf("lda #.HIBYTE(%s)", l.toAsm(op[2])))
					r.push("adc #0")
					r.push(l.storeA(op[1], 1))
				} else if ValType(op[2]).Kind == "pointer" {
					r.push(l.loadYIdx(op[3], op[2]))
					r.push("sty <reg+0")
					r.push("clc")
					r.push(l.loadA(op[2], 0))
					r.push("adc <reg+0")
					r.push(l.storeA(op[1], 0))
					r.push(l.loadA(op[2], 1))
					r.push("adc #0")
					r.push(l.storeA(op[1], 1))
				} else {
					panic("invalid index")
				}
			} else {
				// インデックスのサイズが２
				// TODO: ちゃんとする、テスト作る
				if ValType(op[2]).Kind == "array" {
					r.push(l.loadA(op[3], 0))
					r.push("sta <reg+0")
					r.push(l.loadA(op[3], 1))
					r.push("sta <reg+1")

					if ValType(op[2]).Base.Size == 2 {
						r.push("clc")
						r.push("rol <reg+0")
						r.push("rol <reg+1")
					}

					r.push("lda <reg+0")
					r.push("clc")
					r.push(fmt.Sprintf("adc #.LOBYTE(%s)", l.toAsm(op[2])))
					r.push(l.storeA(op[1], 0))
					r.push("lda <reg+1")
					r.push(fmt.Sprintf("adc #.HIBYTE(%s)", l.toAsm(op[2])))
					r.push(l.storeA(op[1], 1))
				} else {
					panic("invalid index")
				}
			}

		case Sym("ref"):
			if ValLocation(op[2]) == "frame" {
				r.push("txa")
				r.push("clc")
				r.push(fmt.Sprintf("adc #.LOBYTE(S+%s)", ToS(ValAddress(op[2]))))
				r.push(l.storeA(op[1], 0))
				r.push("lda #0")
				r.push(l.storeA(op[1], 1))
			} else {
				r.push(fmt.Sprintf("lda #.LOBYTE(%s)", l.toAsm(op[2])))
				r.push(l.storeA(op[1], 0))
				r.push(fmt.Sprintf("lda #.HIBYTE(%s)", l.toAsm(op[2])))
				r.push(l.storeA(op[1], 1))
			}

		case Sym("pget"):
			r.push(fmt.Sprintf("lda %s", l.byte(op[2], 0)))
			r.push("sta <reg+0")
			r.push(fmt.Sprintf("lda %s", l.byte(op[2], 1)))
			r.push("sta <reg+1")
			for i := 0; i < ValType(op[1]).Size; i++ {
				r.push(fmt.Sprintf("ldy #%d", i))
				r.push("lda (reg),y")
				r.push(l.storeA(op[1], i))
			}

		case Sym("pset"):
			r.push(fmt.Sprintf("lda %s", l.byte(op[1], 0)))
			r.push("sta <reg+0")
			r.push(fmt.Sprintf("lda %s", l.byte(op[1], 1)))
			r.push("sta <reg+1")
			for i := 0; i < ValType(op[1]).Base.Size; i++ {
				r.push(l.loadA(op[2], i))
				r.push(fmt.Sprintf("ldy #%d", i))
				r.push("sta (reg),y")
			}

			// 最適化後のオペレータ
		case Sym("index_pget"):
			if ValType(op[3]) != TypeOf(Sym("int")) && ValType(op[3]) != TypeOf(Sym("sint8")) {
				panic(&CompileError{Msg: "2byte index not supported"})
			}
			if ValType(op[2]).Kind != "array" {
				panic("index_pget with non-array")
			}
			r.push(l.loadYIdx(op[3], op[2]))
			for i := 0; i < ValType(op[1]).Size; i++ {
				r.push(fmt.Sprintf("lda %s+%d,y", l.toAsm(op[2]), i))
				r.push(l.storeA(op[1], i))
			}

		case Sym("index_pset"):
			if ValType(op[2]) != TypeOf(Sym("int")) && ValType(op[2]) != TypeOf(Sym("sint8")) {
				panic(&CompileError{Msg: "2byte index not supported"})
			}
			if ValType(op[1]).Kind != "array" {
				panic("index_pset with non-array")
			}
			r.push(l.loadYIdx(op[2], op[1]))
			for i := 0; i < ValType(op[3]).Size; i++ {
				r.push(l.loadA(op[3], i))
				r.push(fmt.Sprintf("sta %s+%d,y", l.toAsm(op[1]), i))
			}

		default:
			panic(fmt.Sprintf("unknow op %s", dumpOp(op, nil)))
		}
	}

	for _, d := range lmd.Defs {
		switch d.Kind {
		case "block":
			r.push(l.emitBlock(d.Sym, d.Type, d.Val.([]any)))
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

func ifElse(cond bool, a, b string) string {
	if cond {
		return a
	}
	return b
}

func anyIfy(v []any) []any { return v }

func (l *Llc) loadYIdx(idx, ptr any) []any {
	r := []any{}
	if ValType(ptr).Base.Size == 1 {
		r = append(r, fmt.Sprintf("ldy %s", l.byte(idx, 0)))
	} else {
		r = append(r, fmt.Sprintf("lda %s", l.byte(idx, 0)))
		for i := 0; i < ValType(ptr).Base.Size-1; i++ {
			r = append(r, "asl a")
		}
		r = append(r, "tay")
	}
	return r
}

func (l *Llc) load(to, from any) []any {
	r := []any{}
	if ValType(to).Kind == "pointer" && ValType(from).Kind == "array" {
		if ValType(from).Base != ValType(to).Base {
			panic(fmt.Sprintf("can't convert from %s to %s", valToS(from), valToS(to)))
		}
		// 配列からポインタに変換
		r = append(r, fmt.Sprintf("lda #.LOBYTE(%s)", l.toAsm(from)))
		r = append(r, fmt.Sprintf("sta %s", l.byte(to, 0)))
		r = append(r, fmt.Sprintf("lda #.HIBYTE(%s)", l.toAsm(from)))
		r = append(r, fmt.Sprintf("sta %s", l.byte(to, 1)))
	} else {
		// 通常の代入
		if ValType(from).Kind != "int" {
			if ValType(from).Base != ValType(to).Base {
				panic(fmt.Sprintf("can't convert from %s to %s", valToS(from), valToS(to)))
			}
		}
		for i := 0; i < ValType(to).Size; i++ {
			r = append(r, l.loadA(from, i))
			r = append(r, l.storeA(to, i))
		}
	}
	return r
}

// loadA は Aレジスタへのロード。戻り値は string / nil / []string。
func (l *Llc) loadA(v any, n int) any {
	if uv, ok := v.(*Value); ok && uv.Location == "cond" {
		if n != 0 {
			panic("load_a cond with n != 0")
		}
		switch uv.CondReg {
		case "zero":
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
		case "carry":
			if uv.CondPositive {
				return []string{"lda #0", "rol a", "eor #1"}
			}
			return []string{"lda #0", "rol a"}
		case "negative":
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
	if isValueOrCasted(v) && ValLocation(v) == "a" {
		if n != 0 {
			return "lda #0"
		}
		return nil
	}
	return fmt.Sprintf("lda %s", l.byte(v, n))
}

func isValueOrCasted(v any) bool {
	switch v.(type) {
	case *Value, *CastedValue:
		return true
	}
	return false
}

// storeA は Aレジスタからのストア。
// (Ruby版は CastedValue の location==:a を考慮しない — 忠実に再現)
func (l *Llc) storeA(v any, n int) any {
	if uv, ok := v.(*Value); ok && uv.Location == "a" {
		if n != 0 {
			panic("store_a with n != 0")
		}
		return nil
	}
	return fmt.Sprintf("sta %s", l.byte(v, n))
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

func (l *Llc) toAsm(v any) string {
	if isValueOrCasted(v) {
		switch ValKind(v) {
		case "local":
			switch ValLocation(v) {
			case "frame":
				return fmt.Sprintf("<S+%s,x", ToS(ValAddress(v)))
			case "reg":
				return fmt.Sprintf("<L+%s", ToS(ValAddress(v)))
			case "fastcall_reg":
				return fmt.Sprintf("<FC_FASTCALL_REG+%s", ToS(ValAddress(v)))
			default:
				panic(fmt.Sprintf("invalid location %s of %s", ValLocation(v), valToS(v)))
			}
		case "global":
			if ValVal(v) == nil {
				panic(fmt.Sprintf("invalid %s, %s", valToS(v), ToS(ValVal(v))))
			}
			return mangle(ToS(ValVal(v)))
		case "literal":
			return fmt.Sprintf("#%s", ToS(ValVal(v)))
		default:
			panic(fmt.Sprintf("invalid v %s", valToS(v)))
		}
	}
	panic(fmt.Sprintf("invalid v = %v", v))
}

// mangle は名前をアセンブラ用の表現に変更する。
func mangle(str string) string {
	return strings.ReplaceAll(str, "$", "_D")
}

// byte は値からn番目のbyteを取得する。
func (l *Llc) byte(v any, n int) string {
	if pa, ok := v.(*PointeredArray); ok {
		switch n {
		case 0:
			return fmt.Sprintf("#.LOBYTE(%s)", l.toAsm(pa.From))
		case 1:
			return fmt.Sprintf("#.HIBYTE(%s)", l.toAsm(pa.From))
		default:
			panic("invalid byte index for PointeredArray")
		}
	}
	if cv, ok := v.(*CastedValue); ok {
		if n < cv.Type.Size && n < ValType(cv.From).Size {
			return fmt.Sprintf("%d+%s", n, l.toAsm(cv))
		}
		return "#0" // 符号拡張は、:sign_extension オペレータで行うので、存在しないbyteは0扱い
	}
	if ValKind(v) == "literal" {
		switch val := ValVal(v).(type) {
		case int:
			return fmt.Sprintf("#%d", rubyMod(rubyShr(val, n*8), 256))
		case Sym:
			switch n {
			case 0:
				return fmt.Sprintf("#.LOBYTE(%s)", mangle(string(val)))
			case 1:
				return fmt.Sprintf("#.HIBYTE(%s)", mangle(string(val)))
			default:
				panic("invalid byte index for symbol")
			}
		default:
			panic(fmt.Sprintf("invalid literal val %T", ValVal(v)))
		}
	}
	if n < ValType(v).Size {
		return fmt.Sprintf("%d+%s", n, l.toAsm(v))
	}
	return "#0" // 符号拡張は、:sign_extension オペレータで行うので、存在しないbyteは0扱い
}

// emitBlock は v を .db/.dw に変換する。
func (l *Llc) emitBlock(sym any, typ *Type, val []any) []any {
	r := []any{}
	r = append(r, mangle(ToS(sym))+":")
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
			switch x := ValVal(elem).(type) {
			case int:
				parts = append(parts, fmt.Sprintf("%d", rubyMod(x, limit)))
			case Sym:
				parts = append(parts, string(x))
			default:
				parts = append(parts, l.toAsm(elem))
			}
		}
		r = append(r, fmt.Sprintf("\t%s %s", op, strings.Join(parts, ",")))
	}
	return r
}

// ---------------------------------------------------------------
// 一部の複雑なオペレータ用
// ---------------------------------------------------------------

// mulDivMod は mul, div, mod のコード生成 (定数の場合の最適化つき)。
func (l *Llc) mulDivMod(op []any) []any {
	r := []any{}
	op3val := ValVal(op[3])
	op3int, op3IsInt := op3val.(int)
	isPow2 := false
	if op3IsInt {
		switch op3int {
		case 1, 2, 4, 8, 16, 32, 64, 128:
			isPow2 = true
		}
	}
	if op3IsInt && op3int == 0 {
		// 0の場合
		switch op[0] {
		case Sym("mul"):
			r = append(r, anyIfy(l.load(op[1], NewIntValue(0))))
		case Sym("div"), Sym("mod"):
			panic(&CompileError{Msg: "div by 0"})
		}
	} else if op3IsInt && isPow2 && ValType(op[1]).Size == 1 {
		// 定数(1byte)の場合の最適化
		n := log2(op3int)
		r = append(r, l.loadA(op[2], 0))
		switch op[0] {
		case Sym("mul"):
			for i := 0; i < n; i++ {
				r = append(r, "asl a")
			}
		case Sym("div"):
			if ValType(op[2]).Signed {
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
		case Sym("mod"):
			r = append(r, fmt.Sprintf("and #%d", op3int-1))
		}
		r = append(r, l.storeA(op[1], 0))
	} else if op3IsInt && isPow2 && ValType(op[1]).Size > 1 {
		// 定数(2byte以上)の場合の最適化
		n := log2(op3int)
		size := ValType(op[1]).Size
		switch op[0] {
		case Sym("mul"):
			r = append(r, anyIfy(l.load(op[1], op[2])))
			for k := 0; k < n; k++ {
				for i := 0; i < size; i++ {
					rot := ifElse(i == 0, "asl", "rol")
					r = append(r, fmt.Sprintf("%s %s", rot, l.byte(op[1], i)))
				}
			}
		case Sym("div"):
			r = append(r, anyIfy(l.load(op[1], op[2])))
			for k := 0; k < n; k++ {
				for i := size - 1; i >= 0; i-- {
					var rot string
					if ValType(op[1]).Signed {
						rot = "ror"
						if i == 0 {
							r = append(r, "cmp $80")
						}
					} else {
						rot = ifElse(i == size-1, "lsr", "ror")
					}
					r = append(r, fmt.Sprintf("%s %s", rot, l.byte(op[1], i)))
				}
			}
		case Sym("mod"):
			for i := 0; i < size; i++ {
				r = append(r, l.loadA(op[2], i))
				r = append(r, fmt.Sprintf("and #%d", rubyMod(rubyShr(op3int-1, i*8), 256)))
				r = append(r, l.storeA(op[1], i))
			}
		}
	} else {
		// 定数でない場合
		for i := 0; i < ValType(op[1]).Size; i++ {
			r = append(r, l.loadA(op[2], i))
			r = append(r, fmt.Sprintf("sta <reg+0+%d", i))
			r = append(r, l.loadA(op[3], i))
			r = append(r, fmt.Sprintf("sta <reg+2+%d", i))
		}
		if ValType(op[1]).Size == 1 {
			if ValType(op[1]).Signed {
				r = append(r, fmt.Sprintf("jsr __%s_8s", ToS(op[0])))
			} else {
				r = append(r, fmt.Sprintf("jsr __%s_8", ToS(op[0])))
			}
		} else {
			r = append(r, fmt.Sprintf("jsr __%s_16", ToS(op[0])))
		}
		for i := 0; i < ValType(op[1]).Size; i++ {
			r = append(r, fmt.Sprintf("lda <reg+4+%d", i))
			r = append(r, l.storeA(op[1], i))
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

func (l *Llc) allocRegister(lmd *Lambda) {
	if l.OptimizeLevel > 0 {
		AllocateRegister(lmd)
		DeleteUnuse(lmd)
	} else {
		// 単純なバージョンのアロケータ(debug用)
		size := 0
		for _, v := range lmd.Vars {
			if v.Kind == "local" {
				v.Location = "frame"
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
func (l *Llc) optimizePointer(lmd *Lambda, ops [][]any) [][]any {
	ops = append([][]any{}, ops...)

	// index + pget/pset の最適化
	for i, op := range ops {
		if op == nil {
			continue
		}
		if !eqAny(op[0], Sym("index")) {
			continue
		}
		if i+1 >= len(ops) {
			continue
		}
		nextOp := ops[i+1]
		if nextOp == nil {
			continue
		}
		switch nextOp[0] {
		case Sym("pget"):
			if isSameAny(op[1], nextOp[2]) && // 同じ変数を連続で使っていて
				ValKind(op[2]) == "global" && // 単純なシンボルで
				eqAny(ValOpt(op[1]).GetOr(Sym("local_type")), Sym("temp")) && // その変数をそこでしか使っていない
				ValType(op[3]).Size == 1 { // インデックスのサイズが1byte
				ops[i] = []any{Sym("index_pget"), nextOp[1], op[2], op[3]}
				ops[i+1] = nil
			}
		case Sym("pset"):
			if isSameAny(op[1], nextOp[1]) &&
				ValKind(op[2]) == "global" &&
				eqAny(ValOpt(op[1]).GetOr(Sym("local_type")), Sym("temp")) &&
				ValType(op[3]).Size == 1 {
				ops[i] = []any{Sym("index_pset"), op[2], op[3], nextOp[2]}
				ops[i+1] = nil
			}
		}
	}
	return ops
}

// isSameAny は Ruby の == (CastedValue は Delegator 経由で from と比較) 相当。
func isSameAny(a, b any) bool {
	ua, ub := UnderlyingValue(a), UnderlyingValue(b)
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
