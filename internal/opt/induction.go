package opt

// 誘導変数の統合 (doc/v2_ssa.md §5): ループの中で同じ歩幅で進む 2 つの変数のうち、比較にしか使われないカウンタを
// もう一方 (ポインタ) の比較に置き換えて消す。
//
//	k = k0; q = q0                       k = k0; q = q0
//	L: if !(k < LIM) goto E              if !(k < LIM) goto E        (k0 がリテラルでなければ入口で 1 度だけ検査)
//	   *q = ..; q += s; k += s     →     lim = q + (LIM - k)
//	   goto L                            L: if !(q < lim) goto E
//	E:                                      *q = ..; q += s
//	                                        goto L
//
// 条件 (どれも sieve の内側 / 初期化のループが満たす):
//   - k は符号なしの整数、ループ内の定義は毎周 1 回の `add k = k, s` だけ、使用はその加算と、ヘッダの
//     `lt t = k, LIM` (t は分岐だけが使う) だけで、ループの外では使われない
//   - q はポインタ、ループ内の定義は毎周 1 回の `add q = q, s` だけ (歩幅 s は同じリテラル、または同じ版のループ不変の
//     ローカル変数)。ヘッダの比較の時点で k も q も加算前の版 (φ)
//   - LIM はリテラル。k が折り返さないこと: LIM + (s の上限) が k の型に収まる。s の上限はリテラルの値、型の最大値、
//     加算なら入力の上限の和、ループの変数なら支配する比較 `v < LIM'` から (maxValue。sieve の prime = i + i + 3 は
//     外側の `i < 8191` から 16383 以下)
//   - q + (LIM - k0) がポインタとして折り返さないことは「ポインタ演算が配列の末尾 + 1 を超えるのは未定義」
//     (C と同じ規則。language_reference.md §6) から従う。k0 > LIM のとき LIM - k0 が負になって lim が q の下に
//     回り込むのは、入口の検査でループに入らないので起きない
//   - ヘッダは「ラベル + 比較 + 分岐」だけ、外からの入口は 1 つで、その辺は jump か fallthrough (分岐ではない)

import (
	"fmt"
	"os"

	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/types"
)

// eliminateInduction は関数の全てのループについて誘導変数の統合を試みる (変換したら true)。
func eliminateInduction(lmd *ir.Lambda) bool {
	changed := false
	for n := 0; n < 8; n++ {
		s := buildSSA(lmd)
		if s == nil {
			return changed
		}
		if !s.eliminateOneInduction() {
			return changed
		}
		compact(lmd)
		changed = true
	}
	return changed
}

// ivDef はループ内の誘導変数の情報。
type ivDef struct {
	v       *ir.Value
	def     int        // `add v = v, step` の添字
	step    ir.Operand // 歩幅 (リテラル、またはループ不変のローカル変数)
	stepIdx int        // 歩幅の Src の添字
	phi     *ssaVal    // ヘッダの入口での版
}

func (s *ssaForm) eliminateOneInduction() bool {
	cfg := s.cfg
	idom := cfg.Dominators()
	dominates := func(a, b *ir.Block) bool {
		for x := b; x != nil; x = idom[x] {
			if x == a {
				return true
			}
			if idom[x] == x {
				return false
			}
		}
		return false
	}
	ops := s.lmd.Ops
	for _, lp := range cfg.Loops() {
		h := lp.Header
		// ヘッダ: ラベル + `lt t = k, LIM` + `if t goto E` (E はループの外)
		hops := cfg.Ops(h)
		if len(hops) != 3 || ops[hops[0]].Code != ir.OpLabel || ops[hops[1]].Code != ir.OpLt || ops[hops[2]].Code != ir.OpIf {
			ivTrace(1)
			continue
		}
		cmp, br := ops[hops[1]], ops[hops[2]]
		t, ok := cmp.Dst.(*ir.Value)
		if !ok || t.LocalType != ir.LTTemp || br.Src[0] != ir.Operand(t) || lp.Contains(cfg.BlockOf(br.Label)) {
			ivTrace(2)
			continue
		}
		// 入口は 1 つで、その辺は分岐ではない
		entries := cfg.Entries(lp)
		if len(entries) != 1 {
			ivTrace(3)
			continue
		}
		pre := entries[0]
		if last := cfg.Last(pre); last != nil && (isCond(last) || last.Code == ir.OpSwitch || last.Code == ir.OpReturn) {
			ivTrace(4)
			continue
		}
		// 比較の向き: `k < LIM` または `LIM < k`
		kIdx := 0
		if _, lit := ir.ValIntLiteral(cmp.Src[1]); !lit {
			kIdx = 1
		}
		lim, lit := ir.ValIntLiteral(cmp.Src[1-kIdx])
		if !lit {
			ivTrace(5)
			continue
		}
		k, ok := cmp.Src[kIdx].(*ir.Value)
		if !ok || !s.vars[k] || k.Type.Kind != types.Int || k.Type.Signed {
			ivTrace(6)
			continue
		}
		ivs := s.inductionVars(lp, dominates)
		kiv := ivs[k]
		if kiv == nil || len(s.useAt[hops[1]]) == 0 || resolve(s.useAt[hops[1]][kIdx]) != kiv.phi {
			ivTrace(7)
			continue
		}
		// k の使用は加算と比較だけ (ループの外の使用は、ループを通った版を読まないものに限る: 次のループで
		// 初期化し直して使うのはよい)、t の使用は分岐だけ
		loopVals := map[*ssaVal]bool{kiv.phi: true, s.defAt[kiv.def]: true}
		if !s.onlyUses(k, lp, loopVals, kiv.def, hops[1]) || !s.onlyUses(t, lp, nil, hops[2]) {
			ivTrace(8)
			continue
		}
		// 折り返さないこと: LIM + (歩幅の最大値) が k の型に収まる (歩幅の上限は maxValue: リテラル、型、支配する比較から)
		kmax := 1<<(8*uint(k.Type.Size)) - 1
		stepMax, ok := s.maxValue(kiv.def, kiv.stepIdx, idom)
		if !ok || stepMax <= 0 || lim+stepMax > kmax {
			ivTrace(11)
			continue
		}
		// 相方: 同じ歩幅のポインタ
		var q *ivDef
		for v, iv := range ivs {
			if v == k || v.Type.Kind != types.Pointer || v.Type.Size != k.Type.Size {
				continue
			}
			if !s.sameStep(iv.step, kiv.step, iv.def, kiv.def) {
				continue
			}
			if q == nil || iv.def < q.def {
				q = iv
			}
		}
		if q == nil {
			ivTrace(9)
			continue
		}
		// ヘッダの時点で q も加算前の版
		if s.valueAt(q.v, hops[1]) != q.phi {
			ivTrace(10)
			continue
		}
		// 変換: 入口の辺の末尾に [検査] と lim の計算、ヘッダの比較を q < lim に
		limV := ir.NewLocal("$lim", q.v.Type, ir.LTTemp)
		diff := ir.NewLocal("$diff", k.Type, ir.LTTemp)
		s.lmd.Vars = append(s.lmd.Vars, limV, diff)
		var pro []*ir.Op
		if _, k0lit := ir.ValIntLiteral(s.initialOperand(k, pre)); !k0lit {
			g := ir.NewLocal("$guard", t.Type, ir.LTTemp)
			s.lmd.Vars = append(s.lmd.Vars, g)
			pro = append(pro, &ir.Op{Code: ir.OpLt, Dst: g, Src: []ir.Operand{cmp.Src[0], cmp.Src[1]}, Pos: cmp.Pos},
				&ir.Op{Code: ir.OpIf, Src: []ir.Operand{g}, Label: br.Label, Pos: br.Pos})
		}
		pro = append(pro,
			&ir.Op{Code: ir.OpSub, Dst: diff, Src: []ir.Operand{ir.NewIntLiteral("", k.Type, lim), k}, Pos: cmp.Pos},
			&ir.Op{Code: ir.OpAdd, Dst: limV, Src: []ir.Operand{q.v, diff}, Pos: cmp.Pos})
		newCmp := &ir.Op{Code: ir.OpLt, Dst: t, Src: []ir.Operand{q.v, limV}, Pos: cmp.Pos}
		if kIdx == 1 {
			newCmp.Src = []ir.Operand{limV, q.v}
		}
		// 入口の辺の末尾 (jump の前) に挿す
		at := pre.End
		if last := cfg.Last(pre); last != nil && last.Code == ir.OpJump {
			for at = pre.End - 1; ops[at] != last; at-- {
			}
		}
		ops[hops[1]] = newCmp
		out := append([]*ir.Op{}, ops[:at]...)
		out = append(out, pro...)
		out = append(out, ops[at:]...)
		s.lmd.Ops = out
		return true
	}
	return false
}

// inductionVars はループ内で「毎周 1 回だけ `add v = v, s` (s はループ不変) で更新される」ローカル変数。
func (s *ssaForm) inductionVars(lp *ir.Loop, dominates func(a, b *ir.Block) bool) map[*ir.Value]*ivDef {
	defs := map[*ir.Value][]int{}
	for b := range lp.Blocks {
		for _, i := range s.cfg.Ops(b) {
			if d := s.defAt[i]; d != nil {
				defs[d.v] = append(defs[d.v], i)
			}
		}
	}
	everyIteration := func(b *ir.Block) bool {
		for _, t := range lp.Tails {
			if !dominates(b, t) {
				return false
			}
		}
		return true
	}
	r := map[*ir.Value]*ivDef{}
	for v, ds := range defs {
		if len(ds) != 1 {
			continue
		}
		i := ds[0]
		op := s.lmd.Ops[i]
		if op.Code != ir.OpAdd || op.Dst != ir.Operand(v) || !everyIteration(s.blockOf[i]) {
			continue
		}
		self, step := 0, 1
		if op.Src[0] != ir.Operand(v) {
			self, step = 1, 0
			if op.Src[1] != ir.Operand(v) {
				continue
			}
		}
		st := op.Src[step]
		if _, lit := ir.ValIntLiteral(st); !lit {
			sv, ok := st.(*ir.Value)
			if !ok || !s.vars[sv] || len(defs[sv]) > 0 {
				continue
			}
		}
		phi := resolve(s.useAt[i][self])
		if !phi.phi || phi.block != lp.Header {
			continue
		}
		r[v] = &ivDef{v: v, def: i, step: st, stepIdx: step, phi: phi}
	}
	return r
}

// sameStep は 2 つの歩幅が同じか (同じリテラル、または同じ変数で両方の加算の位置で同じ版)。
func (s *ssaForm) sameStep(a, b ir.Operand, ai, bi int) bool {
	na, la := ir.ValIntLiteral(a)
	nb, lb := ir.ValIntLiteral(b)
	if la || lb {
		return la && lb && na == nb
	}
	va, _ := a.(*ir.Value)
	vb, _ := b.(*ir.Value)
	if va == nil || va != vb {
		return false
	}
	return s.valueAt(va, ai) == s.valueAt(vb, bi)
}

// onlyUses は v の使用が allowed の命令だけか (到達できる命令の中で)。ループ lp の外の使用は、読む版が
// loopVals (ループの中の版) に (φ を通しても) 依存しなければよい。
func (s *ssaForm) onlyUses(v *ir.Value, lp *ir.Loop, loopVals map[*ssaVal]bool, allowed ...int) bool {
	for i, op := range s.lmd.Ops {
		if op == nil || s.blockOf[i] == nil {
			continue
		}
		_, uses := ir.DefUse(op)
		for k, u := range uses {
			if ir.UnderlyingValue(u) != v {
				continue
			}
			ok := false
			for _, a := range allowed {
				if a == i {
					ok = true
				}
			}
			if !ok && loopVals != nil && !lp.Contains(s.blockOf[i]) && k < len(s.useAt[i]) && s.useAt[i][k] != nil {
				ok = !dependsOn(resolve(s.useAt[i][k]), loopVals, map[*ssaVal]bool{})
			}
			if !ok {
				return false
			}
		}
	}
	return true
}

// dependsOn は版 val が set のどれかに (φ を辿って) 依存するか。
func dependsOn(val *ssaVal, set, seen map[*ssaVal]bool) bool {
	val = resolve(val)
	if set[val] {
		return true
	}
	if !val.phi || seen[val] {
		return false
	}
	seen[val] = true
	for _, a := range val.args {
		if dependsOn(a, set, seen) {
			return true
		}
	}
	return false
}

// initialOperand は入口ブロック pre の末尾での v の値 (直前の定義がリテラルの load ならそのリテラル、それ以外は v)。
func (s *ssaForm) initialOperand(v *ir.Value, pre *ir.Block) ir.Operand {
	val := resolve(s.readVariable(v, pre))
	if val.def >= 0 {
		if op := s.lmd.Ops[val.def]; op.Code == ir.OpLoad {
			if _, lit := ir.ValIntLiteral(op.Src[0]); lit {
				return op.Src[0]
			}
		}
	}
	return v
}

// ivTrace は調査用 (FC_TRACE_INDUCTION=1 で、対象外になった条件の番号を出す)。
func ivTrace(n int) {
	if os.Getenv("FC_TRACE_INDUCTION") != "" {
		fmt.Fprintf(os.Stderr, "induction: bail %d\n", n)
	}
}

// maxValue は命令 i の入力 k の値の上限 (符号なし)。リテラルはその値、型の最大値、定義が加算なら入力の上限の和、
// φ (ループの変数) は「その使用位置を支配する `lt t = v, LIM; if t goto E` の真の側」にあれば LIM - 1。
func (s *ssaForm) maxValue(i, k int, idom map[*ir.Block]*ir.Block) (int, bool) {
	return s.maxValueDepth(i, k, idom, 4)
}

func (s *ssaForm) maxValueDepth(i, k int, idom map[*ir.Block]*ir.Block, depth int) (int, bool) {
	o := s.lmd.Ops[i].Src[k]
	t := ir.ValType(o)
	if t.Kind != types.Int || t.Signed || ir.ValOffset(o) != 0 {
		return 0, false
	}
	if n, lit := ir.ValIntLiteral(o); lit {
		return n, n >= 0
	}
	us := s.useAt[i]
	if k >= len(us) || us[k] == nil {
		return 0, false
	}
	return s.maxOfVal(resolve(us[k]), s.blockOf[i], idom, depth), true
}

func (s *ssaForm) maxOfVal(val *ssaVal, at *ir.Block, idom map[*ir.Block]*ir.Block, depth int) int {
	tmax := 1<<(8*uint(val.v.Type.Size)) - 1
	if val.v.Type.Kind != types.Int || val.v.Type.Signed {
		return tmax
	}
	if s.konst(val) {
		return val.bits
	}
	if depth == 0 {
		return tmax
	}
	if val.phi {
		// 支配する比較 `lt t = v, LIM` の真の側 (if は落ちる先、if_true は飛び先) から at が支配されていれば LIM - 1
		best := tmax
		for d := at; d != nil; d = idom[d] {
			ops := s.cfg.Ops(d)
			if br := s.cfg.Last(d); br != nil && (br.Code == ir.OpIf || br.Code == ir.OpIfTrue) && len(ops) >= 2 {
				ci := ops[len(ops)-2]
				cmp := s.lmd.Ops[ci]
				if cmp.Code == ir.OpLt && cmp.Dst == br.Src[0] && len(s.useAt[ci]) == 2 && s.useAt[ci][0] != nil && resolve(s.useAt[ci][0]) == val {
					if lim, lit := ir.ValIntLiteral(cmp.Src[1]); lit && lim > 0 {
						var trueSide *ir.Block
						if br.Code == ir.OpIf {
							if d.Index+1 < len(s.cfg.Blocks) {
								trueSide = s.cfg.Blocks[d.Index+1]
							}
						} else {
							trueSide = s.cfg.BlockOf(br.Label)
						}
						if trueSide != nil && trueSide != d && s.dominated(trueSide, at, idom) && lim-1 < best {
							best = lim - 1
						}
					}
				}
			}
			if idom[d] == d {
				break
			}
		}
		return best
	}
	if val.def < 0 {
		return tmax
	}
	op := s.lmd.Ops[val.def]
	switch op.Code {
	case ir.OpLoad:
		if m, ok := s.maxValueDepth(val.def, 0, idom, depth-1); ok {
			return min(m, tmax)
		}
	case ir.OpAdd:
		a, oka := s.maxValueDepth(val.def, 0, idom, depth-1)
		b, okb := s.maxValueDepth(val.def, 1, idom, depth-1)
		if oka && okb && a+b <= tmax {
			return a + b
		}
	case ir.OpAnd, ir.OpMod:
		if m, lit := ir.ValIntLiteral(op.Src[1]); lit && m > 0 {
			if op.Code == ir.OpAnd {
				return min(m, tmax)
			}
			return min(m-1, tmax)
		}
	}
	return tmax
}

// dominated は a が b を支配するか。
func (s *ssaForm) dominated(a, b *ir.Block, idom map[*ir.Block]*ir.Block) bool {
	for x := b; x != nil; x = idom[x] {
		if x == a {
			return true
		}
		if idom[x] == x {
			return false
		}
	}
	return false
}
