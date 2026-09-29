package regalloc

import (
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/types"
)

// PropagateLiteralLoads は常駐の割付の後、同じブロックの中で `load v = K` (K は整数のリテラル) の後に v をそのまま読む所を K に
// し、どこからも読まれなくなった v への `load v = K` を消す。ループの回転 (opt/jumps.go) が前に出した最初の検査と、常駐の入口の
// 写し (`load i@Y = i`) が読むループの変数の初期値のためで、どちらもレジスタから読むのでメモリへの書き込み (`lda #0; sta i`)
// だけが残っていた (SSA はループの回転と常駐の割付より前に走る)。
//
// 対象はアドレスを取られない・volatile でない 1〜2 バイトのローカル変数。変わらないこと (Clean) を前提に退避を省く常駐の
// 退避先は、中身が要るので消さない。@log が読む変数の書き込みを消したら LogStale の印を立てる (@log の有無で消し方を変えない)。
func PropagateLiteralLoads(lmd *ir.Lambda) {
	ops := lmd.Ops
	pinned := map[*ir.Value]bool{}
	for _, op := range ops {
		if op == nil {
			continue
		}
		if op.Code == ir.OpRef || op.Code == ir.OpAsm {
			for _, s := range op.Src {
				if v := ir.UnderlyingValue(s); v != nil {
					pinned[v] = true
				}
			}
		}
		for _, s := range append([]ir.Operand{op.Dst}, op.Src...) {
			if pa, ok := s.(*ir.PointeredArray); ok {
				if v := ir.UnderlyingValue(pa.From); v != nil {
					pinned[v] = true
				}
			}
		}
	}
	for _, v := range lmd.Vars {
		if v.Home != nil && v.Clean {
			pinned[v.Home] = true // 退避を省く常駐は、退避先から読み戻す
		}
	}
	target := func(v *ir.Value) bool {
		// 常駐の一時変数 (Home がある) は除く: 関数・ループの出口の書き戻し (sty home) は codegen が出すので、IR の上では読まれて
		// いないように見える (castle の my_process.anchor_throw の anchor_vx@Y への書き込みを消していた)
		return v != nil && v.Kind == ir.KindLocal && v.Home == nil && !isResident(v) && !v.Volatile && !pinned[v] && v.LocalType != ir.LTResult &&
			v.Type.Size <= 2 && (v.Type.Kind == types.Int || v.Type.Kind == types.Bool)
	}
	cfg := ir.BuildCFG(lmd)
	out := map[*ir.Block]map[*ir.Value]*ir.Value{} // ブロックの終わりで分かっている値
	for _, b := range cfg.Blocks {
		known := map[*ir.Value]*ir.Value{}
		if len(b.Preds) == 1 && out[b.Preds[0]] != nil && b.Preds[0].Index < b.Index {
			// 入口が 1 つ (前のブロックから: 分岐の落ちる側) なら、その終わりの値を引き継ぐ (ループの最初の検査の後の入口の写し)
			for v, lit := range out[b.Preds[0]] {
				known[v] = lit
			}
		}
		for _, i := range cfg.Ops(b) {
			op := ops[i]
			if op == nil {
				continue
			}
			if len(known) > 0 && op.Code != ir.OpRef && op.Code != ir.OpAsm {
				for k, s := range op.Src {
					if op.IsMem() && k == 0 {
						continue // 番地 (ポインタ) はリテラルにしない
					}
					if v, ok := s.(*ir.Value); ok {
						if lit := known[v]; lit != nil {
							op.Src[k] = lit
						}
					}
				}
			}
			if op.Dst == nil {
				continue
			}
			dv := ir.UnderlyingValue(op.Dst)
			delete(known, dv)
			if v, ok := op.Dst.(*ir.Value); ok && op.Code == ir.OpLoad && target(v) {
				if k, lit := ir.ValIntLiteral(op.Src[0]); lit {
					known[v] = ir.NewIntLiteral("", v.Type, k)
				}
			}
		}
		out[b] = known
	}
	ud := ir.BuildUseDef(lmd)
	logged := map[*ir.Value]bool{}
	for _, op := range ops {
		if op == nil {
			continue
		}
		for _, p := range op.Logs {
			for _, a := range p.Args {
				if v := ir.UnderlyingValue(a.Val); v != nil {
					logged[v] = true
				}
				for _, bv := range a.Bytes {
					if v := ir.UnderlyingValue(bv); v != nil {
						logged[v] = true
					}
				}
			}
		}
	}
	for i, op := range ops {
		if op == nil || op.Code != ir.OpLoad {
			continue
		}
		v, ok := op.Dst.(*ir.Value)
		if !ok || !target(v) || len(ud.Uses[v]) != 0 {
			continue
		}
		if _, lit := ir.ValIntLiteral(op.Src[0]); !lit {
			continue
		}
		if logged[v] {
			v.LogStale = true
		}
		ir.DropOp(ops, i)
	}
	compactOps(lmd)
}

// compactOps は消した命令 (nil) を詰める。
func compactOps(lmd *ir.Lambda) {
	out := lmd.Ops[:0]
	for _, op := range lmd.Ops {
		if op != nil {
			out = append(out, op)
		}
	}
	lmd.Ops = out
}
