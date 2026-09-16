package codegen

import (
	"fmt"

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/types"
)

// 乗除算・大きな要素の添字計算 (定数の 2 のべき乗はシフト、それ以外は share/runtime.asm のルーチン呼び出し)。

// indexLarge は要素サイズが 1・2 以外 (struct の配列) の index: Dst = 先頭 + idx * size。
// idx * size は reg+2,reg+3 に 16 ビットで、size のビットごとの shift-add で求める (size は定数)。
func (l *Llc) indexLarge(op *ir.Op) []any {
	arr, idx := op.In(0), op.In(1)
	size := ir.ValType(arr).Base.Size
	r := []any{}
	top := 15
	for top >= 0 && size&(1<<top) == 0 {
		top--
	}
	if at := ir.ValType(arr); at.Kind == types.Array && at.Size > 0 && at.Size <= 256 && ir.ValType(idx).Size == 1 &&
		ir.ValLocation(arr) != ir.LocFrame {
		// 配列全体が 256 バイト以内で添字が 1 バイト: 積も 1 バイトに収まるので A だけで計算する
		// (&objs[i] の struct 配列。16 ビットの積より 3 倍ほど速い)
		r = append(r, l.loadA(idx, 0), "sta <reg+0")
		for bit := top - 1; bit >= 0; bit-- {
			r = append(r, "asl a")
			if size&(1<<bit) != 0 {
				r = append(r, "clc", "adc <reg+0")
			}
		}
		r = append(r, "clc", fmt.Sprintf("adc #.LOBYTE(%s)", l.toAsm(arr)), l.storeA(op.Dst, 0),
			fmt.Sprintf("lda #.HIBYTE(%s)", l.toAsm(arr)), "adc #0", l.storeA(op.Dst, 1))
		return r
	}
	// reg+0,1 = idx (16 ビット)。reg+2,3 = 積
	r = append(r, l.loadA(idx, 0), "sta <reg+0", l.loadA(idx, 1), "sta <reg+1")
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
			// 2 のべき乗の除算は右シフト。符号付きは算術シフト (床除算: __div_16s と同じ丸め)。
			// 最上位バイトを lda して cmp #128 で C に符号を立ててから ror する
			r = append(r, anyIfy(l.load(dst, s0)))
			for k := 0; k < n; k++ {
				for i := size - 1; i >= 0; i-- {
					if i == size-1 {
						if ir.ValType(dst).Signed {
							r = append(r, l.loadA(dst, i), "cmp #128", fmt.Sprintf("ror %s", l.byte(dst, i)))
						} else {
							r = append(r, fmt.Sprintf("lsr %s", l.byte(dst, i)))
						}
					} else {
						r = append(r, fmt.Sprintf("ror %s", l.byte(dst, i)))
					}
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
		// __mul_8 / __div_8s / __mod_16s など (share/runtime.asm)。mul は符号で結果が変わらないので __mul_16 のみ
		suffix := ifElse(ir.ValType(dst).Size == 1, "8", "16")
		if ir.ValType(dst).Signed && op.Code != ir.OpMul {
			suffix += "s"
		}
		r = append(r, fmt.Sprintf("jsr __%s_%s", op.Code.String(), suffix))
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

// incDec は `x = x ± 1` (x はメモリ上の 1 / 2 バイトの変数) を inc / dec で出す。
//
//	1 バイト: inc x                          (5 サイクル。clc; lda; adc #1; sta の 10 から)
//	2 バイト +1: inc x; bne @s; inc x+1; @s:  (7〜13 サイクル。18 から)
//	2 バイト -1: lda x; bne @s; dec x+1; @s: dec x
//
// 結果を A に置く割付 (LocA) のときは A に値が要るので使わない。
// flagsFromIncDec は直前の命令が `inc x` / `dec x` (1 バイト、メモリ上) で、いま x を検査するなら Z フラグが
// その値を反映しているか (`dec x; lda x; bne` の lda を省く)。
func (l *Llc) flagsFromIncDec(prev *ir.Op, v ir.Operand) bool {
	if prev == nil || (prev.Code != ir.OpAdd && prev.Code != ir.OpSub) || ir.ValType(v).Size != 1 {
		return false
	}
	if _, ok := l.incDec(prev); !ok {
		return false
	}
	if l.inY(v) {
		return l.inY(prev.Dst) // iny / dey の直後の cpy #0 は要らない
	}
	if l.inX(v) {
		return l.inX(prev.Dst)
	}
	return isValueOrCasted(v) && ir.ValKind(v) != ir.KindLiteral && !l.inY(prev.Dst) && !l.inX(prev.Dst) && l.byte(v, 0) == l.byte(prev.Dst, 0)
}

func (l *Llc) incDec(op *ir.Op) ([]any, bool) {
	k, lit := ir.ValIntLiteral(op.In(1))
	size := ir.ValType(op.Dst).Size
	if !lit || k != 1 || size > 2 || !isValueOrCasted(op.Dst) || !isValueOrCasted(op.In(0)) {
		return nil, false
	}
	if l.inY(op.Dst) && l.inY(op.In(0)) && size == 1 {
		return []any{ifElse(op.Code == ir.OpAdd, "iny", "dey")}, true // Y に常駐するカウンタ
	}
	if l.inX(op.Dst) && l.inX(op.In(0)) && size == 1 {
		return []any{ifElse(op.Code == ir.OpAdd, "inx", "dex")}, true // X に常駐するカウンタ
	}
	for _, v := range []ir.Operand{op.Dst, op.In(0)} {
		if ir.ValKind(v) == ir.KindLiteral || l.inA(v) || l.inY(v) || l.inX(v) || ir.ValLocation(v) == ir.LocCond {
			return nil, false
		}
	}
	for i := 0; i < size; i++ {
		if l.byte(op.Dst, i) != l.byte(op.In(0), i) {
			return nil, false
		}
	}
	lo, hi := l.byte(op.Dst, 0), ""
	if size == 2 {
		hi = l.byte(op.Dst, 1)
	}
	if op.Code == ir.OpAdd {
		if size == 1 {
			return []any{"inc " + lo}, true
		}
		skip := l.newLabel()
		return []any{"inc " + lo, "bne " + skip, "inc " + hi, skip + ":"}, true
	}
	if size == 1 {
		return []any{"dec " + lo}, true
	}
	skip := l.newLabel()
	return []any{"lda " + lo, "bne " + skip, "dec " + hi, skip + ":", "dec " + lo}, true
}

// shiftInMemory は定数シフトをメモリ上で行う (Dst がメモリにある 1 / 2 バイトのとき)。
//
//	1 バイト:  asl x / lsr x                 (lda; clc; rol a; sta の 10 サイクルが 5 に)
//	2 バイト:  asl lo; rol hi / lsr hi; ror lo (バイトごとに lda / sta していた 20 サイクルが 10 に)
//	          8 以上は先にバイトを動かす (<< 8 は hi = lo; lo = 0)
//
// 符号付きの右シフトは最上位バイトを A に読んで C に符号を立てる (lda hi; cmp #128; ror hi; ror lo)。
// Dst が A / コンディション (LocA / LocCond) なら対象外。In(0) が Dst と別の場所ならまず load でコピーする。
func (l *Llc) shiftInMemory(op *ir.Op, n int, signed bool) ([]any, bool) {
	size := ir.ValType(op.Dst).Size
	if size > 2 || !isValueOrCasted(op.Dst) || ir.ValKind(op.Dst) == ir.KindLiteral {
		return nil, false
	}
	if l.inA(op.Dst) || l.inY(op.Dst) || l.inX(op.Dst) || ir.ValLocation(op.Dst) == ir.LocCond {
		return nil, false
	}
	left := op.Code == ir.OpShiftLeft
	if size == 1 && !l.sameByte(op.Dst, op.In(0), 0) {
		return nil, false // 1 バイトで別の場所へ: A 経由 (lda; clc; rol a; sta) の方が短い
	}
	var r []any
	r = append(r, anyIfy(l.load(op.Dst, op.In(0))))
	if size == 1 {
		x := l.byte(op.Dst, 0)
		for k := 0; k < n; k++ {
			if signed && !left {
				r = append(r, "lda "+x, "cmp #128", "ror "+x)
			} else if left {
				r = append(r, "asl "+x)
			} else {
				r = append(r, "lsr "+x)
			}
		}
		return r, true
	}
	lo, hi := l.byte(op.Dst, 0), l.byte(op.Dst, 1)
	for n >= 8 && !(signed && !left) {
		if left {
			r = append(r, "lda "+lo, "sta "+hi, "lda #0", "sta "+lo)
		} else {
			r = append(r, "lda "+hi, "sta "+lo, "lda #0", "sta "+hi)
		}
		n -= 8
	}
	for k := 0; k < n; k++ {
		switch {
		case left:
			r = append(r, "asl "+lo, "rol "+hi)
		case signed:
			r = append(r, "lda "+hi, "cmp #128", "ror "+hi, "ror "+lo)
		default:
			r = append(r, "lsr "+hi, "ror "+lo)
		}
	}
	return r, true
}
