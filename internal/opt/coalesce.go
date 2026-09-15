package opt

import "github.com/haramako/fc/internal/ir"

// coalesceCopies は一時変数を経由するコピーを消す:
//
//	op t = a, b        (t は一時変数で、ここで定義され、直後の load でだけ使われる)
//	load x = t
//	→ op x = a, b
//
// hlc は `x = a + b` / `x += b` をこの形に脱糖するので、加減算・論理演算の直後の代入で毎回起きる。
// 1 バイトの場合は regalloc が t を A に置くので既に無駄は無いが、2 バイト以上は sta / lda の往復
// (2 バイトで 4 命令) になっていた。
//
// 安全条件: x と t の型が同じ。op が Dst を書く前に Src を読み切る種類 (codegen を確認したもの) だけ。
// x が a / b に現れてもよい (codegen はバイトごとに読んでから書き、上位バイトを読む前に下位バイトしか書かない)。
func coalesceCopies(lmd *ir.Lambda) {
	ud := ir.BuildUseDef(lmd)
	ops := lmd.Ops
	for i, op := range ops {
		if op == nil || i+1 >= len(ops) || ops[i+1] == nil || op.Dst == nil {
			continue
		}
		switch op.Code {
		case ir.OpLoad, ir.OpAdd, ir.OpSub, ir.OpAnd, ir.OpOr, ir.OpXor, ir.OpMul, ir.OpDiv, ir.OpMod,
			ir.OpShiftLeft, ir.OpShiftRight, ir.OpUminus, ir.OpBitNot, ir.OpNot, ir.OpEq, ir.OpLt,
			ir.OpSignExtension, ir.OpIndex, ir.OpPget, ir.OpIndexPget, ir.OpFieldPget:
		default:
			continue
		}
		next := ops[i+1]
		if next.Code != ir.OpLoad {
			continue
		}
		t, ok := op.Dst.(*ir.Value)
		if !ok || t.LocalType != ir.LTTemp || len(ud.Defs[t]) != 1 {
			continue
		}
		if u, single := ud.SingleUse(t); !single || u != i+1 || next.Src[0] != ir.Operand(t) {
			continue
		}
		x := next.Dst
		if ir.ValType(x) != t.Type || !ir.ValAssignable(x) {
			continue
		}
		// x が変数の一部 (CastedValue) なら、書き込み先として codegen が扱える単純な形に限る
		if _, isValue := x.(*ir.Value); !isValue {
			continue
		}
		op.Dst = x // 位置は演算の方を残す (0 除算などのエラー位置)
		ops[i+1] = nil
	}
}
