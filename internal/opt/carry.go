package opt

import "github.com/haramako/fc/internal/ir"

// carryBranch は「最上位 (最下位) ビットを検査して、両方の枝で同じ 1 ビットシフトをする」形を、
// 先にシフトして C フラグで分岐する形にする (CRC の内側ループ):
//
//	and t = x, #0x80; if t goto L        (t は一時変数でここでだけ使う。L の直前は jump で抜ける then 側)
//	  <then> shift_left x = x, #1; ...
//	L: shift_left x = x, #1; ...
//	→
//	shift_left x = x, #1; if_not_carry goto L
//	  <then> ...
//	L: ...
//
// asl / lsr は押し出したビットを C に残すので、検査とシフトが 1 命令になる (6502 の crc は `asl; bcc; eor`)。
// 条件: マスクが左シフトなら最上位ビット (1 バイトなら 0x80、2 バイト値の上位バイトの 0x80)、右シフトなら
// 最下位ビット (1 バイトなら 1、2 バイト値の下位バイトの 1)。両方の枝の先頭の命令が同じ変数の同じ 1 ビットシフト。
// 分岐先のブロックはこの分岐からしか入らない (先頭の命令を消すため)。
func carryBranch(lmd *ir.Lambda) {
	ud := ir.BuildUseDef(lmd)
	cfg := ir.BuildCFG(lmd)
	ops := lmd.Ops
	for i, op := range ops {
		if op == nil || op.Code != ir.OpAnd || i+1 >= len(ops) || ops[i+1] == nil {
			continue
		}
		cond := ops[i+1]
		if cond.Code != ir.OpIf && cond.Code != ir.OpIfTrue {
			continue
		}
		t, ok := op.Dst.(*ir.Value)
		if !ok || t.LocalType != ir.LTTemp || len(ud.Defs[t]) != 1 {
			continue
		}
		if u, single := ud.SingleUse(t); !single || u != i+1 || cond.Src[0] != ir.Operand(t) {
			continue
		}
		mask, isLit := ir.ValIntLiteral(op.Src[1])
		if !isLit {
			continue
		}
		x := ir.UnderlyingValue(op.Src[0])
		if x == nil {
			continue
		}
		// マスクがどのビットか: 検査している値のバイト位置 (CastedValue の Offset) とマスクから
		var shiftCode ir.OpCode
		byteNo := ir.ValOffset(op.Src[0])
		size := ir.ValType(op.Src[0]).Size
		switch {
		case size == 1 && mask == 0x80 && byteNo == x.Type.Size-1:
			shiftCode = ir.OpShiftLeft
		case size == 1 && mask == 1 && byteNo == 0:
			shiftCode = ir.OpShiftRight
		case size == 2 && mask == 0x8000 && byteNo == 0 && x.Type.Size == 2:
			shiftCode = ir.OpShiftLeft
		case size == 2 && mask == 1 && byteNo == 0 && x.Type.Size == 2:
			shiftCode = ir.OpShiftRight
		default:
			continue
		}
		if shiftCode == ir.OpShiftRight && x.Type.Signed && x.Type.Size == 1 {
			// 符号付き 1 バイトの右シフトは lda; cmp #128; ror x で、C は cmp の結果になる → 対象外
			continue
		}
		// 分岐の両側の先頭の命令
		bi := cfg.BlockOf(cond.Label)
		var fall *ir.Block
		for _, b := range cfg.Blocks {
			if b.Start <= i && i < b.End && b.Index+1 < len(cfg.Blocks) {
				fall = cfg.Blocks[b.Index+1]
			}
		}
		if bi == nil || fall == nil || len(bi.Preds) != 1 || len(fall.Preds) != 1 {
			continue
		}
		first := func(b *ir.Block) (int, *ir.Op) {
			for _, k := range cfg.Ops(b) {
				if ops[k].Code != ir.OpLabel {
					return k, ops[k]
				}
			}
			return -1, nil
		}
		ki, si := first(bi)
		kf, sf := first(fall)
		if si == nil || sf == nil {
			continue
		}
		ti, oki := isShiftOne(lmd, ud, ki, x, shiftCode)
		tf, okf := isShiftOne(lmd, ud, kf, x, shiftCode)
		if !oki || !okf {
			continue
		}
		// 書き換え: and → シフト (x 自身に)、if → C の分岐、両方の枝の先頭のシフトを消す。
		// 枝のシフトが一時変数 t に入れる形 (`shl t = x; xor x = t, k`) なら、t の使用を x に置き換える
		ir.ReplaceOp(ops, i, &ir.Op{Code: shiftCode, Dst: x, Src: []ir.Operand{x, si.Src[1]}, Pos: si.Pos})
		if cond.Code == ir.OpIf {
			cond.Code = ir.OpIfNotCarry // ビットが 0 (C クリア) なら L へ
		} else {
			cond.Code = ir.OpIfCarry
		}
		cond.Src = nil
		for _, tv := range []*ir.Value{ti, tf} {
			if tv == nil {
				continue
			}
			u, _ := ud.SingleUse(tv)
			for k, src := range ops[u].Src {
				if src == ir.Operand(tv) {
					ops[u].Src[k] = x
				}
			}
		}
		ir.DropOp(ops, ki)
		ir.DropOp(ops, kf)
		return // CFG と use/def が変わったので 1 回 1 箇所 (呼び出し側が繰り返す)
	}
}

// isShiftOne は ops[k] が `shift x = x, #1`、または `shift t = x, #1` (t は一時変数で、この直後の命令でだけ使い、
// その命令が x を書く前に t を読む) か。後者なら t を返す (呼び出し側が t の使用を x に置き換える)。
func isShiftOne(lmd *ir.Lambda, ud *ir.UseDef, k int, x *ir.Value, code ir.OpCode) (*ir.Value, bool) {
	op := lmd.Ops[k]
	if op.Code != code || len(op.Src) != 2 {
		return nil, false
	}
	if n, ok := ir.ValIntLiteral(op.Src[1]); !ok || n != 1 {
		return nil, false
	}
	s := op.Src[0]
	if ir.UnderlyingValue(s) != x || ir.ValOffset(s) != 0 || ir.ValType(s).Size != x.Type.Size || !ir.PlainOperand(s) {
		return nil, false
	}
	d, ok := op.Dst.(*ir.Value)
	if !ok {
		return nil, false
	}
	if d == x {
		return nil, true
	}
	if d.LocalType != ir.LTTemp || d.Type != x.Type || len(ud.Defs[d]) != 1 {
		return nil, false
	}
	u, single := ud.SingleUse(d)
	if !single || u != k+1 || !readsBeforeWrite(lmd.Ops[u].Code) {
		return nil, false
	}
	// 置き換え後 `op x = x, b` になる。b が x を読むなら値が変わるので不可 (chainInPlace と同じ条件)
	next := lmd.Ops[u]
	for _, b := range next.Src {
		if b != ir.Operand(d) && ir.UnderlyingValue(b) == x {
			return nil, false
		}
	}
	return d, true
}
