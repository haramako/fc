package opt

import (
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/types"
)

// scaleIndex は要素が 2 バイトの配列 / ポインタの添字 i (1 バイト) を、基本ブロックごとに 1 度だけ 2 倍した
// 変数 i*2 に置き換える (`op.Scaled`: 添字はバイト単位):
//
//	index_pget a = px[i]; index_pget b = vx[i]        (codegen は毎回 lda i; asl a; tay)
//	→ shift_left i*2 = i, #1; index_pget a = px[i*2] (scaled); index_pget b = vx[i*2] (scaled)
//
// i*2 は普通のローカル変数なので、ループ内なら regalloc が Y に常駐させて `lda px,y` だけになる。
// i が書き換えられたら (定義があったら) 次の使用で作り直す。ブロックをまたいでは共有しない。
func scaleIndex(lmd *ir.Lambda, u *types.Universe) {
	cfg := ir.BuildCFG(lmd)
	u8 := u.IntType(1, false)
	var out []*ir.Op
	scaled := map[*ir.Value]*ir.Value{} // i → i*2 (作った変数。関数内で共有)
	for _, b := range cfg.Blocks {
		valid := map[*ir.Value]bool{} // このブロックで i*2 が i と一致している
		for _, k := range cfg.Ops(b) {
			op := lmd.Ops[k]
			if op.Code == ir.OpIndexPget || op.Code == ir.OpIndexPset {
				arr, idx := op.In(0), op.In(1)
				iv, ok := idx.(*ir.Value)
				if !op.Scaled && ok && iv.Kind == ir.KindLocal && iv.Type.Size == 1 && iv.Type.Kind == types.Int &&
					ir.ValType(arr).Base.Size == 2 && (ir.ValKind(arr) == ir.KindGlobal || ir.ValType(arr).Kind == types.Pointer) {
					i2 := scaled[iv]
					if i2 == nil {
						i2 = ir.NewLocal(iv.Name+"*2", u8, ir.LTNone)
						scaled[iv] = i2
						lmd.Vars = append(lmd.Vars, i2)
					}
					if !valid[iv] {
						out = append(out, &ir.Op{Code: ir.OpShiftLeft, Dst: i2, Src: []ir.Operand{iv, ir.NewIntLiteral("", u8, 1)}, Pos: op.Pos})
						valid[iv] = true
					}
					op.Src[1] = i2
					op.Scaled = true
				}
			}
			out = append(out, op)
			// i を書き換えたら i*2 は古くなる
			defs, _ := ir.DefUse(op)
			for _, d := range defs {
				if v := ir.UnderlyingValue(d); v != nil {
					delete(valid, v)
				}
			}
		}
	}
	lmd.Ops = out
}
