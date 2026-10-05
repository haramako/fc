package opt

import (
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/types"
)

// sinkAddress は単一使用のアドレス計算 (`index` / ポインタ + 定数の `add`) を、その使用 (添字の無い `load_mem` / `store_mem`) の直前へ動かす。
//
// hlc は `p.x += 1` / SoA の `e.anim++` で左辺のアドレスを右辺の読み出しより先に出す:
//
//	index t = &a[i]; load_mem v = a, i; add w = v, #1; store_mem t, w
//
// t の定義と使用が離れているので fusePointer が `store_mem a, i, w` にできない。使用の直前に寄せれば融合できる
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
	// 動かしても定義と使用は変わらないので UseDef は 1 回だけ作る (*Op を鍵にしているので位置が変わってもよい)。
	// 動かすと前の命令の間が空くことがあるので、先頭から見直す
	ud := ir.BuildUseDef(lmd)
	for changed := true; changed; {
		changed = false
		ops := lmd.Ops
		for i, op := range ops {
			if op == nil || (op.Code != ir.OpIndex && op.Code != ir.OpAdd) {
				continue
			}
			t, ok := op.Dst.(*ir.Value)
			if !ok || t.LocalType != ir.LTTemp || ud.NumDefs(t) != 1 {
				continue
			}
			use, single := ud.SingleUse(t)
			if !single {
				continue
			}
			j := lmd.IndexOf(use)
			if j <= ir.NextOp(ops, i) || !plainDeref(use, t) {
				continue
			}
			if !canSink(op, ops[i+1:j], refered) {
				continue
			}
			// op を j の直前へ
			if len(op.Logs) > 0 {
				// @log の注釈は元の位置に残す (次の命令へ。ir/log.go)
				ir.PrependLogs(ir.NextOpOf(ops, i), op.Logs)
				op.Logs = nil
			}
			ir.MoveOpBefore(ops, i, j)
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
		if b.Code.IsBlockBoundary() || b.Code.IsOpaque() || b.Code.UsesCarry() {
			return false // 直線の区間の外、asm、C を受け取る命令 (前の命令と離せない) の手前まで
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
			case ir.OpStoreMem:
				if uv.Kind == ir.KindGlobal && uv.Type.Kind != types.Array || refered[uv] {
					return false
				}
			}
		}
	}
	return true
}
