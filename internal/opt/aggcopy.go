package opt

// 3 バイト以上の値 (slice・struct) の写しの伝播。

import (
	"github.com/haramako/fc/internal/ir"
)

// propagateAggregateCopies は `load L ← P` (L と P は同じ型で 3 バイト以上のローカル) で、P が関数の中で一度も書かれず、L が
// ほかで書かれないとき、L を読む所 (部分を読む cast も) を P にして写しを消す。インライン展開が呼び先の slice の引数を一時変数へ
// 写し直すもの (`lz4.unpack` が `try_unpack` を inline で呼ぶと 155 バイト、直に呼ぶと 123 バイト) を無くす。SSA (2 バイトまで)
// の外の大きさの値のため。アドレスを取られた変数と @log が読む変数は扱わない。
func propagateAggregateCopies(lmd *ir.Lambda) bool {
	writes := map[*ir.Value]int{}
	addr := map[*ir.Value]bool{}
	defAt := map[*ir.Value]int{} // L → 丸ごと書く load の位置 (1 つだけのとき)
	for i, op := range lmd.Ops {
		if op == nil {
			continue
		}
		if op.Dst != nil {
			if v := ir.UnderlyingValue(op.Dst); v != nil {
				writes[v]++
				if dv, ok := op.Dst.(*ir.Value); ok && op.Code == ir.OpLoad {
					defAt[dv] = i
				}
			}
		}
		if op.Code == ir.OpRef || op.Code == ir.OpAsm {
			for _, s := range op.Src {
				if v := ir.UnderlyingValue(s); v != nil {
					addr[v] = true
				}
			}
		}
		for _, s := range op.Src {
			if pa, ok := s.(*ir.PointeredArray); ok {
				if v := ir.UnderlyingValue(pa.From); v != nil {
					addr[v] = true // (配列の先頭の番地として使う)
				}
			}
		}
		for _, lp := range op.Logs {
			for _, a := range lp.Args {
				if v := ir.UnderlyingValue(a.Val); v != nil {
					addr[v] = true
				}
				for _, b := range a.Bytes {
					if v := ir.UnderlyingValue(b); v != nil {
						addr[v] = true
					}
				}
			}
		}
	}
	local := func(v *ir.Value) bool {
		return v != nil && v.Kind == ir.KindLocal && !addr[v] && v.Type.Size >= 3 && v.Home == nil
	}
	repl := map[*ir.Value]*ir.Value{}
	for l, i := range defAt {
		op := lmd.Ops[i]
		p, ok := op.Src[0].(*ir.Value)
		if !ok || !local(l) || !local(p) || writes[l] != 1 || writes[p] != 0 || p.Type != l.Type || p == l {
			continue
		}
		// L を読む所はすべて写しの後ろ (写しより前で読むのは未定義の値: 置き換えない)
		early := false
		for _, o := range lmd.Ops[:i] {
			if o != nil && opReads(o, l) {
				early = true
				break
			}
		}
		if !early {
			repl[l] = p
		}
	}
	if len(repl) == 0 {
		return false
	}
	var subst func(o ir.Operand) ir.Operand
	subst = func(o ir.Operand) ir.Operand {
		switch x := o.(type) {
		case *ir.Value:
			if p, ok := repl[x]; ok {
				return p
			}
		case *ir.CastedValue:
			if f := subst(x.From); f != x.From {
				c := *x
				c.From = f
				return &c
			}
		case *ir.PointeredArray:
			if f := subst(x.From); f != x.From {
				c := *x
				c.From = f
				return &c
			}
		}
		return o
	}
	for i, op := range lmd.Ops {
		if op == nil {
			continue
		}
		if dv, ok := op.Dst.(*ir.Value); ok && op.Code == ir.OpLoad && repl[dv] != nil && defAt[dv] == i {
			ir.DropOp(lmd.Ops, i) // 写しそのもの
			continue
		}
		for k, s := range op.Src {
			op.Src[k] = subst(s)
		}
	}
	return true
}

// opReads は op が v を (部分でも) 読むか。
func opReads(op *ir.Op, v *ir.Value) bool {
	for _, s := range op.Src {
		if ir.UnderlyingValue(s) == v {
			return true
		}
	}
	return false
}
