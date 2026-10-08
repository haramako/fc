package opt

// ループ不変の計算をループの前へ出す (Agent/wiki/design/ssa.md §12):
//
//	L: .. index t = &a[y] ..             mul j = y, #10              (入口の辺の末尾で 1 度だけ)
//	   .. load_mem d = a, k ..     →     L: ..
//	   if_true c goto L
//
// 対象は入力が全部ループ不変の純粋な計算 (add / sub / and / or / xor / mul / shift_left / index) で、結果が一時変数
// (定義は関数に 1 つ)、または定義が関数に 1 つでどの使用もその定義の後 (支配される) の変数 (fieldindex の `i * s`) のもの。ループ不変の入力は、リテラル、ループの中で定義されない SSA の対象の変数、index の配列
// (グローバルの配列の番地は変わらない)。純粋な計算は止まらず副作用も無いので、ループが 1 度も回らない・その経路を通らない
// ときに前もって計算しても結果は変わらない (結果の一時変数は元の位置より後でしか読まれない)。
// 2 次元の配列の内側のループの `a[y][x]` の行 (y * s) を毎周計算し直していた (fieldindex の後の mul j = y, #s)。
// 入れ子のループは 1 つずつ外へ出る (内側の前へ出した計算が、外側でも不変ならさらに外側の前へ)。

import (
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/types"
)

// hoistInvariants は関数の全てのループについて、ループ不変の計算をループの前へ出す (出したら true)。
func hoistInvariants(lmd *ir.Lambda) bool {
	return untilFixed(lmd, "licm", func() bool {
		s := buildSSA(lmd)
		return s != nil && s.hoistOneInvariant()
	})
}

// hoistable は前へ出せる計算の種類か。
func hoistable(c ir.OpCode) bool {
	switch c {
	case ir.OpAdd, ir.OpSub, ir.OpAnd, ir.OpOr, ir.OpXor, ir.OpMul, ir.OpShiftLeft, ir.OpIndex:
		return true
	}
	return false
}

func (s *ssaForm) hoistOneInvariant() bool {
	cfg := s.cfg
	ops := s.lmd.Ops
	var dom *ir.DomTree
	ndefs := map[*ir.Value]int{}
	for _, op := range ops {
		if op == nil {
			continue
		}
		defs, _ := ir.DefUse(op)
		for _, d := range defs {
			if v := ir.UnderlyingValue(d); v != nil {
				ndefs[v]++
			}
		}
	}
	for _, lp := range cfg.Loops() {
		pre := cfg.Preheader(lp)
		if pre == nil {
			continue
		}
		if last := cfg.Last(pre); last != nil && (ir.IsCondBranch(last) || last.Code == ir.OpSwitch || last.Code == ir.OpReturn) {
			continue
		}
		defs := s.loopDefs(lp)
		for i, op := range ops { // 命令の順 (lp.Blocks は map なので、その順だと出す順がビルドごとに変わる)
			if op == nil || s.blockOf[i] == nil || !lp.Contains(s.blockOf[i]) || !hoistable(op.Code) || ir.FeedsCarry(ops, i) {
				continue
			}
			d, ok := op.Dst.(*ir.Value)
			if !ok || d.Volatile || !s.vars[d] || ndefs[d] != 1 || !s.invariantOperands(op, defs) {
				continue
			}
			if d.LocalType != ir.LTTemp {
				if dom == nil {
					dom = cfg.DomTree()
				}
				if !s.defDominatesUses(d, i, dom) {
					continue
				}
			}
			// 入口の辺の末尾 (jump の前) へ移す
			at := pre.End
			if last := cfg.Last(pre); last != nil && last.Code == ir.OpJump {
				for at = pre.End - 1; ops[at] != last; at-- {
				}
			}
			// @log の注釈は元の位置に残す (次の命令へ。注釈で生成コードが変わらないように)
			moved := *op
			moved.Logs = nil
			ir.DropOp(ops, i)
			out := make([]*ir.Op, 0, len(ops))
			out = append(out, ops[:at]...)
			out = append(out, &moved)
			out = append(out, ops[at:]...)
			s.lmd.Ops = out
			return true
		}
	}
	return false
}

// defDominatesUses は ops[at] (v の定義) がどの v の使用より先に通るか (前の周の値・ループの前の値を読む使用が無い)。
func (s *ssaForm) defDominatesUses(v *ir.Value, at int, dom *ir.DomTree) bool {
	db := s.blockOf[at]
	for i, op := range s.lmd.Ops {
		if op == nil || s.blockOf[i] == nil {
			continue
		}
		_, uses := ir.DefUse(op)
		for _, u := range uses {
			if ir.UnderlyingValue(u) != v {
				continue
			}
			ub := s.blockOf[i]
			if (ub == db && i <= at) || (ub != db && !dom.Dominates(db, ub)) {
				return false
			}
		}
	}
	return true
}

// invariantOperands は op の入力が全部ループ不変か (defs はループの中の定義)。
func (s *ssaForm) invariantOperands(op *ir.Op, defs map[*ir.Value][]int) bool {
	for k, src := range op.Src {
		if _, lit := ir.ValIntLiteral(src); lit {
			continue
		}
		v := ir.UnderlyingValue(src)
		if v == nil {
			return false
		}
		if op.Code == ir.OpIndex && k == 0 && v.Kind == ir.KindGlobal && ir.ValType(src).Kind == types.Array {
			continue // グローバルの配列の番地は動かない
		}
		if !s.vars[v] || v.Volatile || len(defs[v]) > 0 {
			return false
		}
	}
	return true
}
