package opt

// 強さの低減 (Agent/wiki/design/ssa.md §12): ループの中で毎周 1 回 `add i = i, s` で進む変数 i から作る値 f(i) を、
// ループの前で 1 度だけ計算して i と同じ所で進める変数 r に置き換える。
//
//	L: mul j = y, #10                    r = y * 10                   (入口の辺の末尾)
//	   add k = j, x                      L: load j = r
//	   load_mem d = a, k          →         ...
//	   add y = y, #1                        add y = y, #1
//	   ...                                  add r = r, #10            (i の加算の直後)
//
// f は `mul d = i, #c` (歩幅 s * c)、`shift_left d = i, #c` (s << c)、`add d = i, v` / `add d = v, i` (s)、
// `sub d = i, v` (s)、`sub d = v, i` (-s)。v はリテラルかループ不変の変数、c はリテラル。結果は i と同じ大きさの整数で、
// i の読み方 (cast) もずれ・幅を変えないもの。同じ大きさの整数の加算と乗算は 2^n を法として f(i + s) = f(i) + Δ が
// 折り返しても成り立つので、r はループのどこでも f(i) に等しい (i の定義はループの中にこの加算 1 つだけで、r はその直後に
// 進む。ループの外では r を読まない)。2 次元の配列 a[y][x] を y で回すループの添字 y * 10 + x が、毎周の掛け算から 10 の
// 加算になる (fieldindex の mul と add の 2 段を順に置き換える。置き換えた後の r が次の段の i になる)。
// 置き換えた `load d = r` の写しと、使われなくなった前の段の r (自分の加算だけに使われる) は後の propagateSSA が消す。
// 置き換えで毎周の命令が増えないときだけする: 2 のべきでない定数の掛け算 (シフトと加算の展開より 1 回の加算が速い) か、
// その命令が i の唯一の使用 (i の加算と入れ替わる: mul の後の add k = r, x)。y - 1 や、ほかにも使われる i の x + j
// (2 次元の配列の内側のループの添字) を置き換えると、毎周の加算と変数が増えるだけ。

import (
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/types"
)

// reduceStrength は関数の全てのループについて強さの低減をする (変えたら true)。
func reduceStrength(lmd *ir.Lambda) bool {
	return untilFixed(lmd, "strength", func() bool {
		s := buildSSA(lmd)
		if s == nil || !s.reduceOneStrength() {
			return false
		}
		propagateSSA(lmd) // 写し load j = r を畳む (次の段の add k = j, x が r を読むように)
		return true
	})
}

func (s *ssaForm) reduceOneStrength() bool {
	cfg := s.cfg
	dom := cfg.DomTree()
	ops := s.lmd.Ops
	for _, lp := range cfg.Loops() {
		pre := cfg.Preheader(lp)
		if pre == nil {
			continue
		}
		if last := cfg.Last(pre); last != nil && (ir.IsCondBranch(last) || last.Code == ir.OpSwitch || last.Code == ir.OpReturn) {
			continue
		}
		ivs := s.inductionVars(lp, dom)
		if len(ivs) == 0 {
			continue
		}
		defs := s.loopDefs(lp)
		for i, op := range ops { // 命令の順 (決定的に)
			if op == nil || s.blockOf[i] == nil || !lp.Contains(s.blockOf[i]) || ir.FeedsCarry(ops, i) {
				continue
			}
			d, ok := op.Dst.(*ir.Value)
			if !ok || d.Volatile || d.Type.Kind != types.Int { // その位置で f(i) を写すだけなので、d は一時変数でなくてよい
				continue
			}
			iv, delta := s.strengthStep(op, d, ivs, defs)
			if iv == nil || !(s.costlyMul(op) || s.onlyUseOf(iv, i)) {
				continue
			}
			r := ir.NewLocal(d.Name+"'", d.Type, ir.LTNone)
			s.lmd.Vars = append(s.lmd.Vars, r)
			// 入口の辺の末尾 (jump の前) で r = f(i)
			at := pre.End
			if last := cfg.Last(pre); last != nil && last.Code == ir.OpJump {
				for at = pre.End - 1; ops[at] != last; at-- {
				}
			}
			init := *op
			init.Dst = r
			init.Logs = nil
			init.Src = append([]ir.Operand(nil), op.Src...)
			step := ir.InferWidthSign(&ir.Op{Code: ir.OpAdd, Dst: r, Src: []ir.Operand{r, delta}, Pos: ops[iv.def].Pos})
			if _, lit := ir.ValIntLiteral(delta); !lit && op.Code == ir.OpSub && op.Src[1] != nil && ir.UnderlyingValue(op.Src[1]) == iv.v {
				step.Code = ir.OpSub // r = v - i: 歩幅が変数なら引く
			}
			ir.ReplaceOp(ops, i, &ir.Op{Code: ir.OpLoad, Dst: d, Src: []ir.Operand{r}, Pos: op.Pos})
			out := make([]*ir.Op, 0, len(ops)+2)
			for k, o := range ops {
				if k == at {
					out = append(out, &init)
				}
				out = append(out, o)
				if k == iv.def {
					out = append(out, step)
				}
			}
			if at == len(ops) {
				out = append(out, &init)
			}
			s.lmd.Ops = out
			return true
		}
	}
	return false
}

// costlyMul は op が 2 のべきでない定数の掛け算か (展開するとシフトと加算が 2 つ以上)。
func (s *ssaForm) costlyMul(op *ir.Op) bool {
	if op.Code != ir.OpMul {
		return false
	}
	for _, src := range op.Src {
		if c, lit := ir.ValIntLiteral(src); lit && c > 0 && c&(c-1) != 0 {
			return true
		}
	}
	return false
}

// onlyUseOf は ops[at] が誘導変数 iv の、自分の加算のほかの唯一の使用か (置き換えると iv が要らなくなる)。
func (s *ssaForm) onlyUseOf(iv *ivDef, at int) bool {
	for i, op := range s.lmd.Ops {
		if op == nil || i == at || i == iv.def {
			continue
		}
		_, uses := ir.DefUse(op)
		for _, u := range uses {
			if ir.UnderlyingValue(u) == iv.v {
				return false
			}
		}
	}
	return true
}

// strengthStep は op が誘導変数 i の f(i) (この段の対象の形) なら i と、i が s 進むときの r の進み (r に足すもの) を返す。
func (s *ssaForm) strengthStep(op *ir.Op, d *ir.Value, ivs map[*ir.Value]*ivDef, defs map[*ir.Value][]int) (*ivDef, ir.Operand) {
	if len(op.Src) != 2 {
		return nil, nil
	}
	n := d.Type.Size
	mask := 1<<(8*uint(n)) - 1
	// ivAt は Src[k] が誘導変数をそのままの大きさで読むならその情報
	ivAt := func(k int) *ivDef {
		src := op.Src[k]
		v := ir.UnderlyingValue(src)
		if v == nil || ivs[v] == nil || ir.ValOffset(src) != 0 || ir.ValType(src).Size != n || v.Type.Size != n || v.Type.Kind != types.Int {
			return nil
		}
		return ivs[v]
	}
	// invariant は Src[k] がリテラルかループ不変の変数 (同じ大きさで読む) か
	invariant := func(k int) bool {
		src := op.Src[k]
		if _, lit := ir.ValIntLiteral(src); lit {
			return true
		}
		v := ir.UnderlyingValue(src)
		return v != nil && s.vars[v] && !v.Volatile && len(defs[v]) == 0 && ir.ValType(src).Size == n
	}
	lit := func(x int) ir.Operand { return ir.NewIntLiteral("", d.Type, x&mask) }
	for k := 0; k < 2; k++ {
		iv := ivAt(k)
		if iv == nil || !invariant(1-k) {
			continue
		}
		st, stLit := ir.ValIntLiteral(iv.step)
		c, cLit := ir.ValIntLiteral(op.Src[1-k])
		switch op.Code {
		case ir.OpMul:
			if stLit && cLit {
				return iv, lit(st * c)
			}
		case ir.OpShiftLeft:
			if k == 0 && stLit && cLit && c >= 0 && c < 8*n {
				return iv, lit(st << uint(c))
			}
		case ir.OpAdd:
			if stLit {
				return iv, lit(st)
			}
			if ir.ValType(iv.step).Size == n {
				return iv, iv.step
			}
		case ir.OpSub:
			switch {
			case k == 0 && stLit:
				return iv, lit(st)
			case k == 0 && ir.ValType(iv.step).Size == n:
				return iv, iv.step
			case k == 1 && stLit:
				return iv, lit(-st)
			case k == 1 && ir.ValType(iv.step).Size == n:
				return iv, iv.step // reduceOneStrength が sub r = r, s にする
			}
		}
	}
	return nil, nil
}
