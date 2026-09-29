package opt

// slice の引数を部品のまま積む (doc/roadmap.md の「slice の引数の受け渡しを縮める」)。

import (
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/types"
)

// splitSliceArgs は、sema が作る「slice の一時変数のポインタと長さを書き、その変数を push_arg する」形を、部品ごとの push_arg
// (長さの側は ArgCont) にする。slice の並びはポインタ → 長さなので、呼び先のフレーム (やスタック) の中の並びは同じ。呼び出し側の
// フレームに組み立ててから呼び先へ写し直していたのが、呼び先へ直に書くだけになる (vram.put(a, "...") の呼び出しが約 20 バイト
// 縮む)。部品の load と push_arg の間に置けるのは、何も書き換えない push_result / push_arg だけ (部品の値を push_arg の位置で
// 読んでも同じ)。一時変数がほかで使われていれば (読まれる・丸ごと書かれる・アドレスを取られる) 何もしない。
func splitSliceArgs(lmd *ir.Lambda) bool {
	uses := map[*ir.Value]int{}  // 読む回数 (部分を読むのも)
	parts := map[*ir.Value]int{} // 部分を書く回数
	whole := map[*ir.Value]int{} // 丸ごと書く回数
	for _, op := range lmd.Ops {
		if op == nil {
			continue
		}
		for _, s := range op.Src {
			if v := ir.UnderlyingValue(s); v != nil {
				uses[v]++
			}
		}
		if op.Dst != nil {
			if c, ok := op.Dst.(*ir.CastedValue); ok {
				if v := ir.UnderlyingValue(c); v != nil {
					parts[v]++
				}
			} else if v := ir.UnderlyingValue(op.Dst); v != nil {
				whole[v]++
			}
		}
		for _, lp := range op.Logs {
			for _, a := range lp.Args {
				if v := ir.UnderlyingValue(a.Val); v != nil {
					uses[v] += 2 // @log が読む値は残す
				}
			}
		}
	}
	ops := lmd.Ops
	cont := map[*ir.Op]*ir.Op{} // p0 → その後ろに差し込む p1
	for k, op := range ops {
		if op == nil || op.Code != ir.OpPushArg || op.Type == nil || !op.Type.IsSlice() || op.ArgY || op.ArgCont {
			continue
		}
		tmp, ok := op.Src[0].(*ir.Value)
		if !ok || tmp.Kind != ir.KindLocal || tmp.LocalType != ir.LTTemp || tmp.Type.Size != op.Type.Size ||
			uses[tmp] != 1 || parts[tmp] != 2 || whole[tmp] != 0 {
			continue
		}
		// 後ろから、何も書き換えない命令を飛ばしながら tmp の部品の load を 2 つ探す
		var loads []int
		for i := k - 1; i >= 0 && len(loads) < 2; i-- {
			o := ops[i]
			switch {
			case o == nil:
			case slicePart(o, tmp) != nil:
				loads = append(loads, i)
			case o.Code == ir.OpPushResult || o.Code == ir.OpPushFastcallResult || (o.Code == ir.OpPushArg && !o.ArgY):
			default:
				i = -1 // 止める
			}
		}
		if len(loads) != 2 {
			continue
		}
		ptr, n := slicePart(ops[loads[0]], tmp), slicePart(ops[loads[1]], tmp)
		if ptr.Offset != 0 {
			ptr, n = n, ptr
			loads[0], loads[1] = loads[1], loads[0]
		}
		if ptr.Offset != 0 || n.Offset != ptr.Type.Size || ptr.Type.Size+n.Type.Size != tmp.Type.Size {
			continue
		}
		p0 := &ir.Op{Code: ir.OpPushArg, Type: ptr.Type, Src: []ir.Operand{partValue(ops[loads[0]], ptr)}, Pos: op.Pos}
		p1 := &ir.Op{Code: ir.OpPushArg, Type: n.Type, Src: []ir.Operand{partValue(ops[loads[1]], n)}, ArgCont: true, Pos: op.Pos}
		// push_arg の位置に p0、続けて p1 (注釈は ReplaceOp / MergeDrop で p0 が引き取る)
		ir.ReplaceOp(ops, k, p0)
		ir.MergeDrop(ops, k, loads[0])
		ir.MergeDrop(ops, k, loads[1])
		cont[p0] = p1
	}
	if len(cont) == 0 {
		return false
	}
	out := make([]*ir.Op, 0, len(ops)+len(cont))
	for _, op := range ops {
		out = append(out, op)
		if p1, ok := cont[op]; ok {
			out = append(out, p1)
		}
	}
	lmd.Ops = out
	return true
}

// slicePart は op が「tmp の部分に、同じ大きさの値 (か整数のリテラル) をそのまま書く load」なら、その部分 (無ければ nil)。
func slicePart(op *ir.Op, tmp *ir.Value) *ir.CastedValue {
	if op == nil || op.Code != ir.OpLoad {
		return nil
	}
	c, ok := op.Dst.(*ir.CastedValue)
	if !ok || c.From != ir.Operand(tmp) || c.Width != 0 && c.Width != c.Type.Size || !partType(c.Type) {
		return nil
	}
	if _, lit := ir.ValIntLiteral(op.Src[0]); lit {
		return c // `[:u16]` の長さを u8 のリテラルで書くもの (partValue が部分の型のリテラルにする)
	}
	if t := ir.ValType(op.Src[0]); t == nil || t.Size != c.Type.Size {
		return nil
	}
	return c
}

// partValue は部品の load の値 (整数のリテラルは部分の型に作り直す)。
func partValue(load *ir.Op, part *ir.CastedValue) ir.Operand {
	if v, lit := ir.ValIntLiteral(load.Src[0]); lit && ir.ValType(load.Src[0]).Size != part.Type.Size {
		return ir.NewIntLiteral("", part.Type, v)
	}
	return load.Src[0]
}

func partType(t *types.Type) bool {
	return t.Kind == types.Pointer || t.Kind == types.Int
}
