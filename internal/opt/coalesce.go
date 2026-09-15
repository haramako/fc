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

// chainInPlace は `x = (x op1 a) op2 b` の中間の一時変数を x 自身にする:
//
//	op1 t = x, a; op2 x = t, b    (t は一時変数で op2 でだけ使う。b は x を参照しない)
//	→ op1 x = x, a; op2 x = x, b
//
// `crc = (crc << 1) ^ k` のような形。coalesceCopies の後に走らせる (op2 の Dst が x になっている必要がある)。
func chainInPlace(lmd *ir.Lambda) {
	ud := ir.BuildUseDef(lmd)
	ops := lmd.Ops
	for i, op := range ops {
		if op == nil || i+1 >= len(ops) || ops[i+1] == nil || op.Dst == nil || len(op.Src) == 0 {
			continue
		}
		if !readsBeforeWrite(op.Code) {
			continue
		}
		next := ops[i+1]
		if next.Dst == nil || len(next.Src) == 0 || !readsBeforeWrite(next.Code) {
			continue
		}
		t, ok := op.Dst.(*ir.Value)
		if !ok || t.LocalType != ir.LTTemp || len(ud.Defs[t]) != 1 || t.Type.Size < 2 {
			continue // 1 バイトは t が A に置かれる (lda; op; op; sta) 方が速いので対象外
		}
		if u, single := ud.SingleUse(t); !single || u != i+1 || next.Src[0] != ir.Operand(t) {
			continue
		}
		x := next.Dst
		if _, isValue := x.(*ir.Value); !isValue || ir.ValType(x) != t.Type || ir.ValType(op.Src[0]) != t.Type {
			continue
		}
		if !sameStorage(op.Src[0], x) || readsValue(next.Src[1:], x) {
			continue // op2 の他の入力が x を読むなら、x を先に書き換えてはいけない (`x = (x << 1) ^ x`)
		}
		op.Dst = x
		next.Src[0] = x
	}
}

// readsBeforeWrite は「全ての入力を読んでから結果を書く」ことが codegen で保証されている命令か
// (Dst と Src が同じ場所でもよい)。
func readsBeforeWrite(c ir.OpCode) bool {
	switch c {
	case ir.OpLoad, ir.OpAdd, ir.OpSub, ir.OpAnd, ir.OpOr, ir.OpXor, ir.OpMul, ir.OpDiv, ir.OpMod,
		ir.OpShiftLeft, ir.OpShiftRight, ir.OpUminus, ir.OpBitNot, ir.OpNot, ir.OpEq, ir.OpLt,
		ir.OpSignExtension, ir.OpIndex, ir.OpPget, ir.OpIndexPget, ir.OpFieldPget:
		return true
	}
	return false
}

// readsValue は srcs のどれかが x と同じ変数を (一部でも) 読むか。
func readsValue(srcs []ir.Operand, x ir.Operand) bool {
	ux := ir.UnderlyingValue(x)
	for _, s := range srcs {
		if ux != nil && ir.UnderlyingValue(s) == ux {
			return true
		}
	}
	return false
}

// sameStorage は同じ変数 (の同じ位置) を指すか。
func sameStorage(a, b ir.Operand) bool {
	ua, ub := ir.UnderlyingValue(a), ir.UnderlyingValue(b)
	if ua == nil || ub == nil || ua != ub {
		return false
	}
	return ir.ValOffset(a) == ir.ValOffset(b)
}
