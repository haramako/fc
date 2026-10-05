package regalloc

// 添字付きの読み出しを直後の演算に融合する (`t = p[i]; crc = crc ^ t` → `ldy i; eor (p),y`)。
//
// A に常駐する変数がある間は、読み出し (lda (p),y; sta t) が A を壊すので常駐の退避と復帰が要った (crc8 の内側のループで
// `stx k; sta crc; ldy k; lda (p),y; sta t; lda crc; ldx k; eor t`)。融合すると読み出しは Y に添字を用意するだけで A を
// 触らない。regalloc (Classify) と codegen (genLoadMem) が同じ FusedLoad で判断する (食い違うと FC_VERIFY_REGS のエラー)。

import "github.com/haramako/fc/internal/ir"

// fuseNextOps は融合できる直後の命令 (第 2 入力を `adc (p),y` などのオペランドにできる 1 バイトの演算)。
// 比較 (eq / lt) は A が塞がっているときの Y での代用 (`ldy a; cpy b`) と組めないので入れない。
var fuseNextOps = map[ir.OpCode]bool{ir.OpAdd: true, ir.OpSub: true, ir.OpAnd: true, ir.OpOr: true, ir.OpXor: true}

// MarkFusedLoads は融合の候補の load_mem に FuseNext を付ける (opt の後、AllocateResident の前): 添字付きで 1 バイトの
// 読み出しの結果が 1 バイトの一時変数で、直後の 1 バイトの演算の第 2 入力でだけ使われるもの。添字はバイト単位、
// ポインタ経由ならずれが無い (Y = 添字 + ずれ は A で計算するので)。
func MarkFusedLoads(lmd *ir.Lambda) {
	if lmd.Cfg().Disabled("fuse-load") {
		return
	}
	var ud *ir.UseDef
	for i, op := range lmd.Ops {
		if op == nil || op.Code != ir.OpLoadMem || i+1 >= len(lmd.Ops) {
			continue
		}
		m := op.Mem()
		if m.Index == nil || m.Scale != 1 || m.Width != 1 || (!m.BaseIsArray() && m.Disp != 0) || ir.ValType(m.Index).Size != 1 {
			continue
		}
		t, ok := op.Dst.(*ir.Value)
		if !ok || t.LocalType != ir.LTTemp || t.Type.Size != 1 {
			continue
		}
		next := lmd.Ops[i+1]
		if next == nil || !fuseNextOps[next.Code] || len(next.Src) != 2 || ir.ValType(next.Dst).Size != 1 {
			continue
		}
		// 交換できる演算は読んだ値が第 1 入力でもよい (第 2 入力が A に常駐する変数のときだけ融合する: FusedLoad)
		other := next.Src[0]
		if next.Src[1] != ir.Operand(t) {
			if next.Code == ir.OpSub || next.Src[0] != ir.Operand(t) {
				continue
			}
			other = next.Src[1]
		}
		if ir.UnderlyingValue(other) == t || ir.ValType(other).Size != 1 {
			continue
		}
		if ud == nil {
			ud = ir.BuildUseDef(lmd)
		}
		if j, ok := ud.SingleUse(t); !ok || j != i+1 {
			continue
		}
		op.FuseNext = true
	}
}

// FusedLoad は lmd.Ops[i] (FuseNext の load_mem) を直後の命令に融合するか。vA / vY は A / Y に常駐している変数 (nil 可)。
// 読んだ値は直後の命令の第 2 入力 (交換できる演算で第 1 入力なら、第 2 入力が vA のとき: codegen が入れ替えて出す。
// それ以外の第 1 入力は A に読む割付 (allocateA) のほうが同じかよい)。Y が空いているか、添字そのものが Y に常駐していること
// (ポインタ経由で、ポインタがゼロページに無ければ reg に写すのに X を使う: stack 関数は X がフレームの底なので除く)。
// 呼び出しの引数を Y に保持している間はしない。
func FusedLoad(lmd *ir.Lambda, i int, vA, vY *ir.Value) bool {
	op := lmd.Ops[i]
	if op == nil || !op.FuseNext || i+1 >= len(lmd.Ops) {
		return false
	}
	next := lmd.Ops[i+1]
	if next == nil || !fuseNextOps[next.Code] || len(next.Src) != 2 || op.HoldY || op.ArgY || next.HoldY {
		return false
	}
	if next.Src[1] != op.Dst && !(next.Src[0] == op.Dst && isV(next.Src[1], vA)) {
		return false
	}
	if vY == nil {
		return true
	}
	m := op.Mem()
	if !isV(m.Index, vY) {
		return false
	}
	return m.BaseIsArray() || (lmd.ABI != ir.ABIStack && !op.HoldX)
}

// FusedOperands は融合した読み出しの結果 t を使う命令 op の入力を、A に読むもの・第 2 オペランドの順で返す
// (交換できる演算で t が第 1 入力なら入れ替える)。
func FusedOperands(op *ir.Op, t ir.Operand) (ir.Operand, ir.Operand) {
	if op.Src[0] == t && op.Code != ir.OpSub {
		return op.Src[1], op.Src[0]
	}
	return op.Src[0], op.Src[1]
}

// fusedXSetup は融合した読み出しがポインタを reg に写すのに X を使うことがあるか (添字が Y に常駐していて Y を使えない)。
func fusedXSetup(op *ir.Op, vY *ir.Value) bool {
	return vY != nil && !op.Mem().BaseIsArray()
}
