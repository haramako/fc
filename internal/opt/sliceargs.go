package opt

// slice の引数を部品のまま積む (doc/roadmap.md の「slice の引数の受け渡しを縮める」)。

import (
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/types"
)

// splitSliceArgs は、sema が作る「slice の一時変数のポインタと長さを書き、その変数を push_arg する」形を、部品ごとの push_arg
// (2 つ目からは ArgCont) にする。slice の並びはポインタ → 長さなので、呼び先のフレーム (やスタック) の中の並びは同じ。呼び出し側の
// フレームに組み立ててから呼び先へ写し直していたのが、呼び先へ直に書くだけになる (vram.put(a, "...") の呼び出しが約 20 バイト
// 縮む)。
//   - 部品の load と push_arg の間に置けるのは、何も書き換えない push_result / push_arg だけ (部品の値を push_arg の位置で読んでも同じ)
//   - `[]T` → `[:u16]T` の変換 (別の slice の一時変数の部品を写すだけ) は元の値までたどる。u8 の長さを u16 の部分に積むときは、
//     長さの 1 バイトと上の桁の 0 の 2 つの push_arg にする (push_arg は値の大きさを超えるバイトを読めないので)
//   - 一時変数がほかで使われていれば (読まれる・丸ごと書かれる・アドレスを取られる) 何もしない
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
	// sliceTemp は v が「2 つの部品の load だけで書かれ、読まれるのが n 回の slice の一時変数」か。
	sliceTemp := func(o ir.Operand, n int) *ir.Value {
		v, ok := o.(*ir.Value)
		if !ok || v.Kind != ir.KindLocal || v.LocalType != ir.LTTemp || !v.Type.IsSlice() || uses[v] != n || parts[v] != 2 || whole[v] != 0 {
			return nil
		}
		return v
	}
	ops := lmd.Ops
	after := map[*ir.Op][]*ir.Op{} // 置き換えた push_arg → その後ろに差し込む push_arg
	for k, op := range ops {
		if op == nil || op.Code != ir.OpPushArg || op.Type == nil || !op.Type.IsSlice() || op.ArgY || op.ArgCont {
			continue
		}
		tmp := sliceTemp(op.Src[0], 1)
		if tmp == nil || tmp.Type.Size != op.Type.Size {
			continue
		}
		// 後ろから、何も書き換えない命令を飛ばしながら tmp の部品の load を 2 つ探す
		loads := partLoads(ops, k, tmp, true)
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
		srcs := []ir.Operand{ops[loads[0]].Src[0], ops[loads[1]].Src[0]}
		drop := append([]int(nil), loads...)
		// 変換の写し: 部品の値が別の slice の一時変数 y の部品なら、y の部品の値にする (y の 2 つの load はすぐ前にある)
		if c0, ok := srcs[0].(*ir.CastedValue); ok {
			if c1, ok := srcs[1].(*ir.CastedValue); ok && c0.From == c1.From {
				if y := sliceTemp(c0.From, 2); y != nil {
					if yl := partLoads(ops, min(loads[0], loads[1]), y, false); len(yl) == 2 {
						for pi, c := range []*ir.CastedValue{c0, c1} {
							for _, l := range yl {
								if yp := slicePart(ops[l], y); yp.Offset == c.Offset && yp.Type.Size == c.Type.Size {
									srcs[pi] = ops[l].Src[0]
								}
							}
						}
						drop = append(drop, yl...)
					}
				}
			}
		}
		var pushes []*ir.Op
		ok := true
		for pi, part := range []*ir.CastedValue{ptr, n} {
			ps := partPushes(srcs[pi], part.Type, op)
			if ps == nil {
				ok = false
				break
			}
			pushes = append(pushes, ps...)
		}
		if !ok {
			continue
		}
		for _, p := range pushes[1:] {
			p.ArgCont = true
		}
		// push_arg の位置に最初の部品、続けて残り (注釈は ReplaceOp / MergeDrop で最初の部品が引き取る)
		ir.ReplaceOp(ops, k, pushes[0])
		for _, d := range drop {
			ir.MergeDrop(ops, k, d)
		}
		after[pushes[0]] = pushes[1:]
	}
	if len(after) == 0 {
		return false
	}
	out := make([]*ir.Op, 0, len(ops)+2*len(after))
	for _, op := range ops {
		out = append(out, op)
		out = append(out, after[op]...)
	}
	lmd.Ops = out
	return true
}

// partLoads は ops[k] の前の、tmp の部品の load の位置 (近い順に 2 つまで)。skipPush なら何も書き換えない push_result / push_arg を
// 飛ばす。そのほかの命令があれば止める (tmp のほかの load は探さない)。
func partLoads(ops []*ir.Op, k int, tmp *ir.Value, skipPush bool) []int {
	var loads []int
	for i := k - 1; i >= 0 && len(loads) < 2; i-- {
		o := ops[i]
		switch {
		case o == nil:
		case slicePart(o, tmp) != nil:
			loads = append(loads, i)
		case skipPush && (o.Code == ir.OpPushResult || o.Code == ir.OpPushFastcallResult || (o.Code == ir.OpPushArg && !o.ArgY)):
		case !skipPush && o.Code == ir.OpLoad:
			// (変換の写し: tmp の load の後ろにある、写し先の load)
			if c, ok := o.Dst.(*ir.CastedValue); !ok || ir.UnderlyingValue(c.From) == tmp {
				return nil
			}
		default:
			return loads
		}
	}
	return loads
}

// slicePart は op が「tmp の部分 (ポインタか整数) に値を書く load」なら、その部分 (無ければ nil)。
func slicePart(op *ir.Op, tmp *ir.Value) *ir.CastedValue {
	if op == nil || op.Code != ir.OpLoad {
		return nil
	}
	c, ok := op.Dst.(*ir.CastedValue)
	if !ok || c.From != ir.Operand(tmp) || c.Width != 0 && c.Width != c.Type.Size || !partType(c.Type) {
		return nil
	}
	return c
}

// partPushes は部分の型 t に値 src を積む push_arg (積めない形なら nil)。整数のリテラルは t のリテラルに作り直し、符号なしの
// 狭い値は値の大きさの push_arg と上の桁の 0 の push_arg にする。
func partPushes(src ir.Operand, t *types.Type, at *ir.Op) []*ir.Op {
	push := func(v ir.Operand, typ *types.Type) *ir.Op {
		return &ir.Op{Code: ir.OpPushArg, Type: typ, Src: []ir.Operand{v}, Pos: at.Pos}
	}
	st := ir.ValType(src)
	if v, lit := ir.ValIntLiteral(src); lit {
		if st.Size != t.Size {
			src = ir.NewIntLiteral("", t, v)
		}
		return []*ir.Op{push(src, t)}
	}
	switch {
	case st == nil:
		return nil
	case st.Size == t.Size:
		return []*ir.Op{push(src, t)}
	case st.Size == 1 && t.Size == 2 && t.Kind == types.Int && st.Kind == types.Int && !st.Signed:
		return []*ir.Op{push(src, st), push(ir.NewIntLiteral("", st, 0), st)}
	}
	return nil
}

func partType(t *types.Type) bool {
	return t.Kind == types.Pointer || t.Kind == types.Int
}
