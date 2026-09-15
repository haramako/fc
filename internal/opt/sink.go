package opt

import (
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/types"
)

// sinkAddress は単一使用のアドレス計算 (`index` / ポインタ + 定数の `add`) を、その使用 (`pget` / `pset`) の直前へ動かす。
//
// hlc は `p.x += 1` / SoA の `e.anim++` で左辺のアドレスを右辺の読み出しより先に出す:
//
//	index t = &a[i]; index_pget v = a[i]; add w = v, #1; pset *t = w
//
// t の定義と使用が離れているので fusePointer が `index_pset a[i] = w` にできない。使用の直前に寄せれば融合できる
// (fusePointer の前に走らせる)。動かしてよい条件:
//   - 同じ基本ブロック内 (間にラベル・分岐・return・インラインアセンブラが無い)
//   - 間の命令が計算の入力を書き換えない (入力の変数への定義。グローバル変数の入力は呼び出しをまたがない。
//     アドレスを取られた局所変数の入力はポインタ経由の書き込みをまたがない)
func sinkAddress(lmd *ir.Lambda) {
	refered := map[*ir.Value]bool{}
	for _, op := range lmd.Ops {
		if op != nil && op.Code == ir.OpRef {
			refered[ir.UnderlyingValue(op.Src[0])] = true
		}
	}
	for changed := true; changed; {
		changed = false
		ud := ir.BuildUseDef(lmd)
		ops := lmd.Ops
		for i, op := range ops {
			if op == nil || (op.Code != ir.OpIndex && op.Code != ir.OpAdd) {
				continue
			}
			t, ok := op.Dst.(*ir.Value)
			if !ok || t.LocalType != ir.LTTemp || len(ud.Defs[t]) != 1 {
				continue
			}
			j, single := ud.SingleUse(t)
			if !single || j <= i+1 {
				continue
			}
			use := ops[j]
			if (use.Code != ir.OpPget && use.Code != ir.OpPset) || use.Src[0] != ir.Operand(t) {
				continue
			}
			if !canSink(op, ops[i+1:j], refered) {
				continue
			}
			// op を j の直前へ
			moved := append([]*ir.Op{}, ops[:i]...)
			moved = append(moved, ops[i+1:j]...)
			moved = append(moved, op)
			moved = append(moved, ops[j:]...)
			lmd.Ops = moved
			changed = true
			break
		}
	}
}

// canSink は op を between の後ろへ動かしてよいか。
func canSink(op *ir.Op, between []*ir.Op, refered map[*ir.Value]bool) bool {
	for _, b := range between {
		if b == nil {
			continue
		}
		switch b.Code {
		case ir.OpLabel, ir.OpIf, ir.OpIfTrue, ir.OpJump, ir.OpReturn, ir.OpAsm:
			return false
		}
		defs, _ := ir.DefUse(b)
		for _, src := range op.Src {
			uv := ir.UnderlyingValue(src)
			if uv == nil {
				continue
			}
			for _, d := range defs {
				if ir.UnderlyingValue(d) == uv {
					return false
				}
			}
			switch b.Code {
			case ir.OpCall, ir.OpFastcall:
				// 呼び出し先がグローバル変数を書き換えるかもしれない (配列そのもの = アドレス定数は別)
				if uv.Kind == ir.KindGlobal && uv.Type.Kind != types.Array {
					return false
				}
			case ir.OpPset, ir.OpIndexPset, ir.OpFieldPset:
				if uv.Kind == ir.KindGlobal && uv.Type.Kind != types.Array || refered[uv] {
					return false
				}
			}
		}
	}
	return true
}
