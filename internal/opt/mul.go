package opt

// 定数との乗算をシフトと加減算に展開する (2 のべき乗は codegen が asl にするので対象外):
//
//	mul d = x, 10   → shift_left t1 = x, 3; shift_left t2 = x, 1; add d = t1, t2   (立っているビットが 3 つまで)
//	mul d = x, 7    → shift_left t1 = x, 3; sub d = t1, x                          (2^n - 1)
//	mul d = x, 1    → load d = x
//	mul d = x, 0    → load d = #0
//
// __mul_8 / __mul_16 (share/runtime.asm) は 100〜300 サイクルで、1 バイトなら展開した方が常に速い。2 バイトでも
// シフト 1 段が 2 命令 (asl lo; rol hi) なので、項が 3 つまでなら速い。castle は `__mul_8` に 1〜2% を使っていた。

import (
	"math/bits"

	"github.com/haramako/fc/internal/ir"
)

func expandMul(lmd *ir.Lambda) {
	var out []*ir.Op
	changed := false
	for _, op := range lmd.Ops {
		if op == nil {
			continue
		}
		if op.Code != ir.OpMul {
			out = append(out, op)
			continue
		}
		x, k := op.Src[0], op.Src[1]
		if _, lit := ir.ValIntLiteral(x); lit {
			x, k = k, x
		}
		n, ok := ir.ValIntLiteral(k)
		t := ir.ValType(op.Dst)
		if !ok || !isIntLike(t) || !isIntLike(ir.ValType(x)) || ir.ValType(x).Size != t.Size || n < 0 || n >= 1<<(8*uint(t.Size)) {
			out = append(out, op)
			continue
		}
		newOp := func(code ir.OpCode, dst ir.Operand, a, b ir.Operand) *ir.Op {
			return &ir.Op{Code: code, Dst: dst, Src: []ir.Operand{a, b}, Pos: op.Pos}
		}
		tmp := func() *ir.Value {
			v := ir.NewLocal("$mul", t, ir.LTTemp)
			lmd.Vars = append(lmd.Vars, v)
			return v
		}
		lit := func(m int) ir.Operand { return ir.NewIntLiteral("", ir.ValType(k), m) }
		switch {
		case n == 0:
			out = append(out, &ir.Op{Code: ir.OpLoad, Dst: op.Dst, Src: []ir.Operand{lit(0)}, Pos: op.Pos})
		case n == 1:
			out = append(out, &ir.Op{Code: ir.OpLoad, Dst: op.Dst, Src: []ir.Operand{x}, Pos: op.Pos})
		case n&(n-1) == 0:
			out = append(out, op) // 2 のべき乗は codegen が asl にする
			continue
		case (n+1)&n == 0:
			// 2^m - 1: (x << m) - x
			t1 := tmp()
			out = append(out, newOp(ir.OpShiftLeft, t1, x, lit(bits.Len(uint(n))))) // n+1 = 2^m なので m = Len(n)
			out = append(out, newOp(ir.OpSub, op.Dst, t1, x))
		case bits.OnesCount(uint(n)) <= 3:
			// 立っているビットごとのシフトの和 (上から)
			var acc ir.Operand
			for b := bits.Len(uint(n)) - 1; b >= 0; b-- {
				if n&(1<<uint(b)) == 0 {
					continue
				}
				var term ir.Operand = x
				if b > 0 {
					t1 := tmp()
					out = append(out, newOp(ir.OpShiftLeft, t1, x, lit(b)))
					term = t1
				}
				if acc == nil {
					acc = term
					continue
				}
				dst := ir.Operand(op.Dst)
				if n&(1<<uint(b)-1) != 0 {
					dst = tmp() // まだ項が残る
				}
				out = append(out, newOp(ir.OpAdd, dst, acc, term))
				acc = dst
			}
		default:
			out = append(out, op)
			continue
		}
		changed = true
	}
	if changed {
		lmd.Ops = out
	}
}
