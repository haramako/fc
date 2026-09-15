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
