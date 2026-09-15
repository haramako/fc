package codegen

import (
	"fmt"
	"strings"

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/types"
)

// オペランドの読み書き: 値の置き場所 (フレーム / レジスタ領域 / A / グローバル) に応じたアドレス表記と lda / sta。

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

// storeA は A レジスタからのストア (置き場所が A の値なら何も出さない)。
func (l *Llc) storeA(v ir.Operand, n int) any {
	if uv, ok := v.(*ir.Value); ok && uv.Location == ir.LocA {
		if n != 0 {
			panic("store_a with n != 0")
		}
		return nil
	}
	return fmt.Sprintf("sta %s", l.byte(v, n))
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
