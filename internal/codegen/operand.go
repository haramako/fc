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

// loadYIdx は添字を Y に入れる。scaled なら添字はすでにバイト単位 (opt.scaleIndex)。
func (l *Llc) loadYIdx(idx, ptr ir.Operand, scaled bool) []any {
	r := []any{}
	if ir.ValType(ptr).Base.Size == 1 || scaled {
		if l.inY(idx) {
			// 添字が Y に常駐している
		} else if l.inA(idx) {
			r = append(r, "tay") // 添字が A にある
		} else {
			r = append(r, fmt.Sprintf("ldy %s", l.byte(idx, 0)))
		}
	} else {
		r = append(r, l.loadA(idx, 0))
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
			r = append(r, fmt.Sprintf("lda #.LOBYTE(%s)", l.addrExpr(from)))
			r = append(r, fmt.Sprintf("sta %s", l.byte(to, 0)))
			r = append(r, fmt.Sprintf("lda #.HIBYTE(%s)", l.addrExpr(from)))
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
			if l.sameByte(to, from, i) {
				continue // 自分自身への代入 (x = x) は何もしない
			}
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
	if l.inA(v) {
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

// addrExpr は配列・変数の先頭アドレスの式 (`#.LOBYTE(...)` の中身)。静的フレーム上のローカルは `F_f+addr`。
func (l *Llc) addrExpr(v ir.Operand) string {
	if isValueOrCasted(v) && ir.ValKind(v) == ir.KindLocal && ir.ValLocation(v) == ir.LocStatic {
		return fmt.Sprintf("%s+%d", l.curLambda.FrameSym(), ir.ValAddress(v))
	}
	return l.toAsm(v)
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
	if l.inA(v) {
		if n != 0 {
			panic("store_a with n != 0")
		}
		return nil
	}
	return fmt.Sprintf("sta %s", l.byte(v, n))
}

// pointerBase はポインタ値 v を (P),y の形で参照するための P を返す。
//   - ゼロページ (レジスタ領域 / fastcall 領域) にあるポインタ変数: そのまま "(L+4),y" で使える → ("L+4", 0 命令)
//   - それ以外 (フレーム / グローバル / 即値など): reg にコピーしてから "(reg),y" → ("reg", コピー命令)
func (l *Llc) pointerBase(v ir.Operand) (string, []any) {
	if isValueOrCasted(v) && ir.ValKind(v) == ir.KindLocal && ir.ValType(v).Size == 2 {
		switch ir.ValLocation(v) {
		case ir.LocReg, ir.LocFastcallReg:
			return strings.TrimPrefix(l.toAsm(v), "<"), nil
		case ir.LocStatic:
			if l.curLambda.FrameZp {
				return strings.TrimPrefix(l.toAsm(v), "<"), nil
			}
		}
	}
	return "reg", []any{l.loadA(v, 0), "sta <reg+0", l.loadA(v, 1), "sta <reg+1"}
}

// pointerRead は *(p + off) から size バイトを dst に読む。
// p がゼロページなら (p),y、フレーム上で 1 バイト・オフセット 0 なら (S+n,x)、それ以外は reg にコピーして (reg),y。
func (l *Llc) pointerRead(p ir.Operand, off int, dst ir.Operand, size int) []any {
	var r []any
	if fp, ok := l.framePointer(p); ok && off == 0 && size == 1 {
		return []any{fmt.Sprintf("lda %s", fp), l.storeA(dst, 0)}
	}
	base, setup := l.pointerBase(p)
	if size > 1 && sameStorage(p, dst) {
		// p = p.next のように読み先がポインタ自身: 下位バイトを書いた後に上位バイトを読むと壊れるので reg 経由
		base, setup = "reg", []any{l.loadA(p, 0), "sta <reg+0", l.loadA(p, 1), "sta <reg+1"}
	}
	r = append(r, setup)
	for i := 0; i < size; i++ {
		r = append(r, fmt.Sprintf("ldy #%d", off+i), fmt.Sprintf("lda (%s),y", base), l.storeA(dst, i))
	}
	return r
}

// pointerWrite は val の size バイトを *(p + off) に書く (pointerRead の逆)。
func (l *Llc) pointerWrite(p ir.Operand, off int, val ir.Operand, size int) []any {
	var r []any
	if fp, ok := l.framePointer(p); ok && off == 0 && size == 1 {
		return []any{l.loadA(val, 0), fmt.Sprintf("sta %s", fp)}
	}
	base, setup := l.pointerBase(p)
	r = append(r, l.keepA(val, setup))
	for i := 0; i < size; i++ {
		r = append(r, l.loadA(val, i), fmt.Sprintf("ldy #%d", off+i), fmt.Sprintf("sta (%s),y", base))
	}
	return r
}

// inX は値がいま X レジスタにあるか (LocX。退避中 (resXMem) ならメモリ側 Home にある)。
func (l *Llc) inX(v ir.Operand) bool {
	if !isValueOrCasted(v) || ir.ValKind(v) != ir.KindLocal || ir.ValLocation(v) != ir.LocX {
		return false
	}
	uv := ir.UnderlyingValue(v)
	return !(l.resXMem && uv.Home != nil && uv == l.resX)
}

// inY は値がいま Y レジスタにあるか (LocY。退避中 (resYMem) ならメモリ側 Home にある)。
func (l *Llc) inY(v ir.Operand) bool {
	if !isValueOrCasted(v) || ir.ValKind(v) != ir.KindLocal || ir.ValLocation(v) != ir.LocY {
		return false
	}
	uv := ir.UnderlyingValue(v)
	return !(l.resYMem && uv.Home != nil && uv == l.resY)
}

// inA は値がいま A レジスタにあるか (LocA。ただしループ内の常駐変数を退避中 (resMem) ならメモリ側 Home にある)。
func (l *Llc) inA(v ir.Operand) bool {
	if !isValueOrCasted(v) || ir.ValKind(v) != ir.KindLocal || ir.ValLocation(v) != ir.LocA {
		return false
	}
	uv := ir.UnderlyingValue(v)
	return !(l.resMem && uv.Home != nil && uv == l.res)
}

// keepA は書く値 val が A にあり、その前に出す準備 pre (ポインタの reg へのコピーなど) が A を壊すとき、
// A を reg+2 に退避して pre の後で戻す (pre が空なら pre のまま)。
func (l *Llc) keepA(val ir.Operand, pre []any) []any {
	if len(pre) == 0 || !l.inA(val) {
		return pre
	}
	r := []any{"sta <reg+2"}
	r = append(r, pre...)
	return append(r, "lda <reg+2")
}

// sameByte は 2 つのメモリ上のオペランドの i バイト目が同じ場所か (アドレス表記が同じ)。A / 即値なら false。
func (l *Llc) sameByte(a, b ir.Operand, i int) bool {
	for _, v := range []ir.Operand{a, b} {
		if !isValueOrCasted(v) || ir.ValKind(v) == ir.KindLiteral || l.inA(v) || l.inY(v) || l.inX(v) || ir.ValLocation(v) == ir.LocCond {
			return false
		}
	}
	return l.byte(a, i) == l.byte(b, i)
}

// sameStorage は 2 つのオペランドが同じ変数 (の一部) を指すか。
func sameStorage(a, b ir.Operand) bool {
	if ua, ub := ir.UnderlyingValue(a), ir.UnderlyingValue(b); ua != nil && ub != nil {
		return ua == ub
	}
	return false
}

// framePointer はポインタ値 v がフレーム上の変数なら "(S+n,x)" (indexed indirect) で 1 バイト目を直接参照できる。
func (l *Llc) framePointer(v ir.Operand) (string, bool) {
	if isValueOrCasted(v) && ir.ValKind(v) == ir.KindLocal && ir.ValType(v).Size == 2 && ir.ValLocation(v) == ir.LocFrame {
		return fmt.Sprintf("(S+%d,x)", ir.ValAddress(v)), true
	}
	return "", false
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
			case ir.LocStatic:
				return staticAddr(l.curLambda, ir.ValAddress(v))
			case ir.LocA:
				if h := ir.UnderlyingValue(v).Home; h != nil && l.resMem {
					return l.toAsm(h) // 常駐変数の退避中: メモリ側
				}
				panic(fmt.Sprintf("invalid location %s of %s", ir.ValLocation(v), ir.OperandString(v)))
			case ir.LocY:
				if h := ir.UnderlyingValue(v).Home; h != nil && l.resYMem {
					return l.toAsm(h)
				}
				panic(fmt.Sprintf("invalid location %s of %s", ir.ValLocation(v), ir.OperandString(v)))
			case ir.LocX:
				if h := ir.UnderlyingValue(v).Home; h != nil && l.resXMem {
					return l.toAsm(h)
				}
				panic(fmt.Sprintf("invalid location %s of %s", ir.ValLocation(v), ir.OperandString(v)))
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
func mangle(str string) string { return ir.Mangle(str) }

// argBytes は引数の合計バイト数。
func argBytes(lmd *ir.Lambda) int {
	n := 0
	for _, p := range lmd.Type.Params {
		n += p.Size
	}
	return n
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
			return fmt.Sprintf("#.LOBYTE(%s)", l.addrExpr(pa.From))
		case 1:
			return fmt.Sprintf("#.HIBYTE(%s)", l.addrExpr(pa.From))
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
