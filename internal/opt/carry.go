package opt

import (
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/types"
)

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
		if !oki || !okf || si.Sign != sf.Sign {
			continue // 両方の枝が同じシフト (右シフトは符号も同じ) のときだけ 1 つにまとめられる
		}
		if si.IsSigned() && x.Type.Size == 1 {
			// 符号付き 1 バイトの右シフトは lda; cmp #128; ror x で、C は cmp の結果になる → 対象外
			continue
		}
		// 書き換え: and → シフト (x 自身に)、if → C の分岐、両方の枝の先頭のシフトを消す。
		// 枝のシフトが一時変数 t に入れる形 (`shl t = x; xor x = t, k`) なら、t の使用を x に置き換える
		ir.ReplaceOp(ops, i, &ir.Op{Code: shiftCode, Dst: x, Src: []ir.Operand{x, si.Src[1]}, Sign: si.Sign, Pos: si.Pos})
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
	if !single || u != k+1 || !lmd.Ops[u].Code.ReadsBeforeWrite() {
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

// averageBytes は 1 バイトどうしを 16 ビットで足して 1 ビット右へずらした値の下位 1 バイトだけを使う形 (平均
// `((a as u16 + b) / 2) as u8`) を、8 ビットの足し算と、その C (9 ビット目) を上に入れる右回転にする:
//
//	add y = <u16>a, b; shift_right x = y, #1 (か div x = y, #2); ... <u8>x ...
//	→ add t = a, b; rorc s = t; ... s ...                               (clc; lda a; adc b; ror a)
//
// 条件: a / b は 1 バイトの変数 (または 1 バイトの変数をゼロ拡張して読む cast) でリテラルでない (x += 1 は inc になって C を
// 作らない)。y は右シフトでだけ使い、x は下位 1 バイトを読む cast でだけ使う。足し算と右シフトは隣 (間に C を変える命令が無い)。
func averageBytes(lmd *ir.Lambda, u *types.Universe) bool {
	ud := ir.BuildUseDef(lmd)
	ops := lmd.Ops
	u8 := u.IntType(1, false)
	byteOf := func(o ir.Operand) (ir.Operand, bool) {
		switch x := o.(type) {
		case *ir.CastedValue:
			if v, ok := x.From.(*ir.Value); ok && x.Width == 1 && x.Offset == 0 && v.Type.Size == 1 && v.Kind != ir.KindLiteral {
				return v, true
			}
		case *ir.Value:
			if x.Type.Size == 1 && x.Kind != ir.KindLiteral && isIntLike(x.Type) {
				return x, true
			}
		}
		return nil, false
	}
	lowByte := func(o ir.Operand, x *ir.Value) bool {
		cv, ok := o.(*ir.CastedValue)
		return ok && cv.From == ir.Operand(x) && cv.Offset == 0 && cv.Type.Size == 1 && cv.Width == 1
	}
	changed := false
	for i := 0; i+1 < len(ops); i++ {
		add, sh := ops[i], ops[i+1]
		if add == nil || sh == nil || add.Code != ir.OpAdd {
			continue
		}
		y, ok := add.Dst.(*ir.Value)
		if !ok || y.LocalType != ir.LTTemp || y.Type.Size != 2 || y.Type.Signed || len(ud.Defs[y]) != 1 {
			continue
		}
		a, okA := byteOf(add.Src[0])
		b, okB := byteOf(add.Src[1])
		if !okA || !okB {
			continue
		}
		if sh.Code != ir.OpShiftRight && sh.Code != ir.OpDiv {
			continue
		}
		k, lit := ir.ValIntLiteral(sh.Src[1])
		if !lit || sh.IsSigned() || !(sh.Code == ir.OpShiftRight && k == 1 || sh.Code == ir.OpDiv && k == 2) || sh.Src[0] != ir.Operand(y) {
			continue
		}
		if u, single := ud.SingleUse(y); !single || u != i+1 {
			continue
		}
		x, ok := sh.Dst.(*ir.Value)
		if !ok || x.LocalType != ir.LTTemp || x.Type.Size != 2 || len(ud.Defs[x]) != 1 {
			continue
		}
		good := len(ud.Uses[x]) > 0
		for _, j := range ud.Uses[x] {
			if j <= i+1 || ops[j] == nil {
				good = false
				break
			}
			for _, s := range ops[j].Src {
				if ir.UnderlyingValue(s) == x && !lowByte(s, x) {
					good = false
				}
			}
			if ops[j].Dst != nil && ir.UnderlyingValue(ops[j].Dst) == x {
				good = false
			}
		}
		if !good {
			continue
		}
		t := ir.NewLocal("$avg", u8, ir.LTTemp)
		s := ir.NewLocal("$avg", u8, ir.LTTemp)
		lmd.Vars = append(lmd.Vars, t, s)
		ir.ReplaceOp(ops, i, &ir.Op{Code: ir.OpAdd, Dst: t, Src: []ir.Operand{a, b}, Pos: add.Pos})
		ir.ReplaceOp(ops, i+1, &ir.Op{Code: ir.OpRorC, Dst: s, Src: []ir.Operand{t}, Pos: sh.Pos})
		for _, j := range ud.Uses[x] {
			for k, o := range ops[j].Src {
				if lowByte(o, x) {
					ops[j].Src[k] = s
				}
			}
		}
		changed = true
	}
	return changed
}
