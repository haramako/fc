package opt

// 小さなループの完全展開 (doc/v2_ssa.md §7)。回数がリテラルから決まる短いループ (crc8 / crc16 の `for (j = 8; j; j--)`) を
// 本体の写しの並びにする。Oscar64 の crc8 が fc の 3 倍速いのはこれ (8 回の `asl; bcc; eor` を直線に並べる)。
//
//	k = k0                              k = k0
//	L: if !cond(k) goto E               L:    if !cond(k) goto E     ← 定数になるので SSA が消す
//	   body (k += s を含む)      →            body (k += s)
//	   goto L                           L_u1: if !cond(k) goto E
//	E:                                        body
//	                                    ...
//	                                    L_uN: if !cond(k) goto E     ← 必ず成立 → jump E
//	                                    E:
//
// 各写しにヘッダの検査を残すので、k が畳めなくても正しい (畳めるのが前提だが)。本体の中の `goto L` (continue) は
// 次の写しのヘッダへ。ループの外へのジャンプ (break、出口) はそのまま。ループの中で定義される一時変数は写しごとに
// 別の変数にする (同じ一時変数を何度も定義すると live range が伸びて A / フラグへの割付が外れる)。
//
// 条件: ヘッダは「ラベル + [lt / eq t = k, リテラル] + 分岐」、k はループ内で毎周 1 回 `add / sub k = k, リテラル` で
// しか変わらず、初期値がリテラル。回数はシミュレーションで求め、8 回まで。本体の命令数 × 回数 ≤ 64。
// ループの範囲にあるループ外のブロック (for の出口) からループへ入らないこと。asm から参照されるラベルを含まないこと。

import (
	"fmt"
	"os"

	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/types"
)

const (
	unrollMaxTrips = 8
	unrollMaxOps   = 64
)

// unrollLoops は展開できるループを 1 つずつ展開する (変換したら true)。
func unrollLoops(lmd *ir.Lambda) bool {
	changed := false
	for n := 0; n < 8; n++ {
		s := buildSSA(lmd)
		if s == nil || !s.unrollOne() {
			return changed
		}
		compact(lmd)
		changed = true
	}
	return changed
}

func (s *ssaForm) unrollOne() bool {
	cfg := s.cfg
	idom := cfg.Dominators()
	ops := s.lmd.Ops
	for _, lp := range cfg.Loops() {
		h := lp.Header
		hops := cfg.Ops(h)
		if len(hops) < 2 || len(hops) > 3 || ops[hops[0]].Code != ir.OpLabel {
			unrollTrace("bail1", h, 0, 0)
			continue
		}
		br := ops[hops[len(hops)-1]]
		if (br.Code != ir.OpIf && br.Code != ir.OpIfTrue) || lp.Contains(cfg.BlockOf(br.Label)) {
			unrollTrace("bail2", h, 0, 0)
			continue
		}
		// ヘッダの条件が読むカウンタ k
		var k *ir.Value
		var cmp *ir.Op
		if len(hops) == 3 {
			cmp = ops[hops[1]]
			if (cmp.Code != ir.OpLt && cmp.Code != ir.OpEq) || cmp.Dst != br.Src[0] {
				continue
			}
			t, ok := cmp.Dst.(*ir.Value)
			if !ok || t.LocalType != ir.LTTemp {
				continue
			}
			for _, o := range cmp.Src {
				if v, ok := o.(*ir.Value); ok && s.vars[v] {
					k = v
				}
			}
		} else if v, ok := br.Src[0].(*ir.Value); ok && s.vars[v] {
			k = v
		}
		if k == nil || k.Type.Kind != types.Int {
			unrollTrace("bail3", h, 0, 0)
			continue
		}
		// ループのブロックは連続
		first, last := h, h
		for b := range lp.Blocks {
			if b.Index < first.Index {
				first = b
			}
			if b.Index > last.Index {
				last = b
			}
		}
		if first != h {
			unrollTrace("bail4", h, 0, 0)
			continue
		}
		// 範囲 [first, last] にループ外のブロック (for の出口 `@else: jump @end` は本体と step の間に置かれる) が
		// あってもよいが、そこからループの中へ入ってはいけない (写しごとに一緒に写す)
		enters := false
		for _, b := range cfg.Blocks[first.Index : last.Index+1] {
			if lp.Contains(b) {
				continue
			}
			for _, x := range b.Succs {
				if lp.Contains(x) {
					enters = true
				}
			}
		}
		if enters {
			unrollTrace("bail5", h, 0, 0)
			continue
		}
		// k の更新: ループ内の唯一の定義が毎周 1 回の add / sub k = k, リテラル
		step, ok := s.counterStep(lp, k, idom)
		if !ok {
			unrollTrace("step", h, 0, 0)
			unrollTrace("bail6", h, 0, 0)
			continue
		}
		// 入口: 1 つで、そこでの k がリテラル
		entries := cfg.Entries(lp)
		if len(entries) != 1 {
			unrollTrace("bail7", h, 0, 0)
			continue
		}
		k0, ok := ir.ValIntLiteral(s.initialOperand(k, entries[0]))
		if !ok {
			unrollTrace("bail8", h, 0, 0)
			continue
		}
		// 回数のシミュレーション
		trips := -1
		kv := k0
		for n := 0; n <= unrollMaxTrips; n++ {
			exit, ok := s.headerExits(cmp, br, k, kv)
			if !ok {
				break
			}
			if exit {
				trips = n
				break
			}
			kv = normInt(kv+step, k.Type)
		}
		if trips <= 0 {
			unrollTrace("trips", h, trips, 0)
			unrollTrace("bail9", h, 0, 0)
			continue
		}
		// 大きさ: コードになる命令だけ数える (ラベル、ヘッダへの jump、畳まれるカウンタの更新と検査は除く)
		bodyOps := 0
		for _, b := range cfg.Blocks[first.Index : last.Index+1] {
			for _, i := range cfg.Ops(b) {
				switch ops[i].Code {
				case ir.OpLabel, ir.OpJump:
				default:
					bodyOps++
				}
			}
		}
		bodyOps -= len(hops) // ヘッダの検査 (ラベルは数えていないので 1 多く引くが目安なのでよい)
		if bodyOps*trips > unrollMaxOps {
			unrollTrace("too big", h, bodyOps, trips)
			unrollTrace("bail10", h, 0, 0)
			continue
		}
		// asm から参照されるラベル、asm 命令があれば諦める
		bad := false
		for _, b := range cfg.Blocks[first.Index : last.Index+1] {
			for _, i := range cfg.Ops(b) {
				if ops[i].Code == ir.OpAsm || (ops[i].Code == ir.OpLabel && usedByAsm(s.lmd, ops[i].Label)) {
					bad = true
				}
			}
		}
		if bad {
			unrollTrace("bail11", h, 0, 0)
			continue
		}
		// ループ内で定義される一時変数 (使用もループ内だけ) は写しごとに別の変数に
		ud := ir.BuildUseDef(s.lmd)
		inLoop := func(i int) bool { return i >= h.Start && i < last.End }
		var renamable []*ir.Value // 定義の順 (Vars に足す順を決定的に: golden / ROM の一致)
		seen := map[*ir.Value]bool{}
		for i := h.Start; i < last.End; i++ {
			if ops[i] == nil {
				continue
			}
			defs, _ := ir.DefUse(ops[i])
			for _, d := range defs {
				v := ir.UnderlyingValue(d)
				if v == nil || v.Kind != ir.KindLocal || v.LocalType != ir.LTTemp || seen[v] {
					continue
				}
				seen[v] = true
				ok := true
				for _, j := range ud.Uses[v] {
					if !inLoop(j) {
						ok = false
					}
				}
				for _, j := range ud.Defs[v] {
					if !inLoop(j) {
						ok = false
					}
				}
				if ok {
					renamable = append(renamable, v)
				}
			}
		}
		if os.Getenv("FC_TRACE_UNROLL") != "" {
			fmt.Fprintf(os.Stderr, "unroll: %s trips=%d range=[%d,%d)"+string(rune(10)), h.Label, trips, h.Start, last.End)
			for i := h.Start; i < last.End; i++ {
				if ops[i] != nil {
					fmt.Fprintf(os.Stderr, "   %04d %s"+string(rune(10)), i, ir.DumpOp(ops[i], nil))
				}
			}
		}
		// 写しを作る
		var out []*ir.Op
		out = append(out, ops[:h.Start]...)
		labelIn := map[string]bool{}
		for i := h.Start; i < last.End; i++ {
			if ops[i] != nil && ops[i].Code == ir.OpLabel {
				labelIn[ops[i].Label] = true
			}
		}
		// 写し n のラベルは `元のラベル_<番号>` (番号は関数内で使われていないもの。入れ子のループを先に展開した写しの
		// ラベルと衝突しないように)
		base := maxLabelNumber(s.lmd)
		suffix := func(n int, l string) string {
			if n == 0 || !labelIn[l] {
				return l
			}
			return fmt.Sprintf("%s_%d", l, base+n)
		}
		for n := 0; n <= trips; n++ {
			rename := map[*ir.Value]*ir.Value{}
			if n > 0 {
				for _, v := range renamable {
					nv := ir.NewLocal(fmt.Sprintf("%s_u%d", v.Name, n), v.Type, ir.LTTemp)
					s.lmd.Vars = append(s.lmd.Vars, nv)
					rename[v] = nv
				}
			}
			mapOperand := func(o ir.Operand) ir.Operand { return renameOperand(o, rename) }
			// ヘッダ (最後の写しは検査だけ)
			end := last.End
			if n == trips {
				end = h.End
			}
			for i := h.Start; i < end; i++ {
				op := ops[i]
				if op == nil {
					continue
				}
				no := *op
				no.Src = make([]ir.Operand, len(op.Src))
				for j, o := range op.Src {
					no.Src[j] = mapOperand(o)
				}
				if op.Dst != nil {
					no.Dst = mapOperand(op.Dst)
				}
				if n > 0 {
					no.Logs = ir.CloneLogs(op.Logs, mapOperand) // 最初の写しは元の注釈のまま (ir.KeepLogs が付け替えない)
				}
				// 最後の写し (検査だけ) の飛び先は元のブロック (出口の経路はどの写しでも同じ)
				target := func(l string) string {
					if l == h.Label {
						return suffix(n+1, l) // continue: 次の写しのヘッダへ
					}
					if n == trips {
						return l
					}
					return suffix(n, l)
				}
				if op.Code == ir.OpLabel {
					no.Label = suffix(n, op.Label)
				} else if op.Label != "" {
					no.Label = target(op.Label)
				}
				if len(op.Labels) > 0 {
					no.Labels = make([]string, len(op.Labels))
					for j, l := range op.Labels {
						no.Labels[j] = target(l)
					}
				}
				out = append(out, &no)
			}
		}
		out = append(out, ops[last.End:]...)
		s.lmd.Ops = out
		return true
	}
	return false
}

// renameOperand は rename にある変数を差し替える (CastedValue / PointeredArray の中も)。
func renameOperand(o ir.Operand, rename map[*ir.Value]*ir.Value) ir.Operand {
	switch x := o.(type) {
	case *ir.Value:
		if nv := rename[x]; nv != nil {
			return nv
		}
	case *ir.CastedValue:
		return ir.RebaseCast(x, renameOperand(x.From, rename))
	case *ir.PointeredArray:
		return ir.NewPointeredArray(renameOperand(x.From, rename), x.Type)
	}
	return o
}

// counterStep はループ内での k の唯一の定義が毎周 1 回の `add / sub k = k, リテラル` ならその歩幅 (sub は負)。
func (s *ssaForm) counterStep(lp *ir.Loop, k *ir.Value, idom map[*ir.Block]*ir.Block) (int, bool) {
	var defs []int
	for b := range lp.Blocks {
		for _, i := range s.cfg.Ops(b) {
			if d := s.defAt[i]; d != nil && d.v == k {
				defs = append(defs, i)
			}
		}
	}
	if len(defs) != 1 {
		return 0, false
	}
	i := defs[0]
	op := s.lmd.Ops[i]
	if (op.Code != ir.OpAdd && op.Code != ir.OpSub) || op.Dst != ir.Operand(k) || op.Src[0] != ir.Operand(k) {
		return 0, false
	}
	n, ok := ir.ValIntLiteral(op.Src[1])
	if !ok {
		return 0, false
	}
	for _, t := range lp.Tails {
		if !s.dominated(s.blockOf[i], t, idom) {
			return 0, false
		}
	}
	if op.Code == ir.OpSub {
		n = -n
	}
	return n, true
}

// headerExits はカウンタが kv のときヘッダの検査でループを抜けるか。
func (s *ssaForm) headerExits(cmp, br *ir.Op, k *ir.Value, kv int) (bool, bool) {
	cond := kv != 0
	if cmp != nil {
		vals := [2]int{}
		for i, o := range cmp.Src {
			if o == ir.Operand(k) {
				vals[i] = kv
			} else if n, ok := ir.ValIntLiteral(o); ok {
				vals[i] = normInt(n, k.Type)
			} else {
				return false, false
			}
		}
		if cmp.Code == ir.OpLt {
			cond = vals[0] < vals[1]
		} else {
			cond = vals[0] == vals[1]
		}
	} else if br.Src[0] != ir.Operand(k) {
		return false, false
	}
	// if は条件が偽 (0) のとき飛ぶ = 抜ける。if_true は真のとき抜ける
	if br.Code == ir.OpIf {
		return !cond, true
	}
	return cond, true
}

// unrollTrace は調査用 (FC_TRACE_UNROLL=1)。
func unrollTrace(why string, h *ir.Block, a, b int) {
	if os.Getenv("FC_TRACE_UNROLL") != "" {
		fmt.Fprintf(os.Stderr, "unroll: %s %s %d %d"+string(rune(10)), h.Label, why, a, b)
	}
}
