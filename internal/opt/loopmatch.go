package opt

// ループの形の照合 (induction / unroll が共有する部品)。以前はそれぞれがヘッダの形、ループ内の定義の収集、
// 「毎周必ず通る」(back edge の元を支配する) の判定を別々に書いていた。

import "github.com/haramako/fc/internal/ir"

// loopHeader はループのヘッダが「ラベル; [比較 t = …;] 条件分岐 (ループの外へ)」の形ならその部品を返す。比較があれば
// その結果は一時変数で、分岐がそれを読む (cmp は比較が無ければ nil)。hops はヘッダの命令の添字。
func (s *ssaForm) loopHeader(lp *ir.Loop) (cmp, br *ir.Op, hops []int, ok bool) {
	hops = s.cfg.Ops(lp.Header)
	ops := s.lmd.Ops
	if len(hops) < 2 || len(hops) > 3 || ops[hops[0]].Code != ir.OpLabel {
		return nil, nil, nil, false
	}
	br = ops[hops[len(hops)-1]]
	if (br.Code != ir.OpIf && br.Code != ir.OpIfTrue) || lp.Contains(s.cfg.BlockOf(br.Label)) {
		return nil, nil, nil, false
	}
	if len(hops) == 3 {
		cmp = ops[hops[1]]
		t, isV := cmp.Dst.(*ir.Value)
		if !isV || t.LocalType != ir.LTTemp || br.Src[0] != ir.Operand(t) {
			return nil, nil, nil, false
		}
	}
	return cmp, br, hops, true
}

// loopDefs はループの中で定義される変数 → 定義の命令の添字 (SSA の対象の変数だけ)。
func (s *ssaForm) loopDefs(lp *ir.Loop) map[*ir.Value][]int {
	defs := map[*ir.Value][]int{}
	for b := range lp.Blocks {
		for _, i := range s.cfg.Ops(b) {
			if d := s.defAt[i]; d != nil {
				defs[d.v] = append(defs[d.v], i)
			}
		}
	}
	return defs
}

// singleStep はループの中の v の定義が「毎周必ず通る `add / sub v = v, step` の 1 つ」ならその命令の添字と、歩幅の Src の添字。
// sub を許すかは allowSub。
func (s *ssaForm) singleStep(lp *ir.Loop, dom *ir.DomTree, defs map[*ir.Value][]int, v *ir.Value, allowSub bool) (i, stepIdx int, ok bool) {
	ds := defs[v]
	if len(ds) != 1 {
		return 0, 0, false
	}
	i = ds[0]
	op := s.lmd.Ops[i]
	if op.Dst != ir.Operand(v) || (op.Code != ir.OpAdd && !(allowSub && op.Code == ir.OpSub)) || !lp.EveryIteration(dom, s.blockOf[i]) {
		return 0, 0, false
	}
	switch {
	case op.Src[0] == ir.Operand(v):
		return i, 1, true
	case op.Code == ir.OpAdd && op.Src[1] == ir.Operand(v):
		return i, 0, true
	}
	return 0, 0, false
}
