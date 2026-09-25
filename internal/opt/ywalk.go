package opt

import (
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/types"
)

// walkPointerY は 1 ずつ進むポインタの下位バイトを添字 k (regalloc が Y に常駐させる) に移す (ループの回転の後):
//
//	jump B                               k = p.lo
//	L: .. *p .. (pset p / pget *p)       if !(p < lim) goto X      (入口の検査は元のまま)
//	   add p = p, #1                     p.lo = 0
//	B: lt t = p, lim                →    L: .. p[k] ..             (lda (p),y)
//	   if_true t goto L                     add k = k, #1          (iny)
//	                                        if_true k goto S       (bne)
//	                                        add p.hi = p.hi, #1    (inc p+1)
//	                                     S:
//	                                     B: eq t1 = k, lim.lo; if t1 goto L        (cpy lim; bne)
//	                                        eq t2 = p.hi, lim.hi; if t2 goto L
//	                                     X: p.lo = k
//
// ループの間 p の下位は 0 で、本当の値は (p.hi, k)。帰りの検査を `!=` にしてよいのは、p がループの中で毎周ちょうど 1 回
// `p += 1` され、前の周の検査で p < lim だったので、検査の時点で p <= lim だから (入口は元の `<` で検査する)。
// crc8 / crc16 / sieve の初期化のループ: `ldy #0; lda (p),y; inc p; bne; inc p+1` と 16 ビットの比較が
// `lda (p),y; iny; bne; …; cpy lim; bne` になる。
//
// 条件: p はアドレスを取られないローカルの 1 バイト要素のポインタ (呼び先からは見えないので、ループの中の呼び出しは
// よい)。ループ (L から帰りの分岐まで) の中の p の使用は `pget v = *p` / `pset *p = v`、定義は `add p = p, #1` の 1 つだけで、
// 毎周必ず通る (ywalkBody)。ループから出る分岐 (break)・return・switch・asm が無い。lim はリテラルかループの中で
// 定義されないローカル変数。
func walkPointerY(lmd *ir.Lambda, u *types.Universe) bool {
	if ir.Disabled("ywalk") {
		return false
	}
	changed := false
	for n := 0; n < 8; n++ {
		if !walkPointerYOne(lmd, u) {
			break
		}
		changed = true
	}
	return changed
}

func walkPointerYOne(lmd *ir.Lambda, u *types.Universe) bool {
	ops := lmd.Ops
	labelAt := map[string]int{}
	for i, op := range ops {
		if op != nil && op.Code == ir.OpLabel {
			labelAt[op.Label] = i
		}
	}
	refered := map[*ir.Value]bool{}
	for _, op := range ops {
		if op != nil && op.Code == ir.OpRef {
			refered[ir.UnderlyingValue(op.Src[0])] = true
		}
	}
	u8 := u.IntType(1, false)
	for c, br := range ops {
		// 帰りの分岐: `B: lt t = p, lim; if_true t goto L` (L は前)
		if br == nil || br.Code != ir.OpIfTrue || c < 2 {
			continue
		}
		a, ok := labelAt[br.Label]
		if !ok || a >= c {
			continue
		}
		cmp, lb := ops[c-1], ops[c-2]
		if cmp == nil || cmp.Code != ir.OpLt || lb == nil || lb.Code != ir.OpLabel || br.Src[0] != cmp.Dst {
			continue
		}
		t, _ := cmp.Dst.(*ir.Value)
		p, _ := cmp.Src[0].(*ir.Value)
		lim := cmp.Src[1]
		if t == nil || t.LocalType != ir.LTTemp || p == nil || p.Kind != ir.KindLocal || refered[p] || p.Volatile ||
			p.Type.Kind != types.Pointer || p.Type.Size != 2 || p.Type.Base == nil || p.Type.Base.Size != 1 {
			continue
		}
		if !ywalkLimit(lim, p) {
			continue
		}
		b := c - 2
		// 入口: L の直前が `jump B`
		if a == 0 || ops[a-1] == nil || ops[a-1].Code != ir.OpJump || ops[a-1].Label != lb.Label {
			continue
		}
		if !ywalkBody(ops, labelAt, a, b, c, p, t, lim, lb.Label) {
			continue
		}
		ywalkRewrite(lmd, u8, a, b, c, p, t, lim)
		return true
	}
	return false
}

// ywalkLimit は lim が帰りの検査に使える上限か (リテラル、または p と同じ幅の値)。
func ywalkLimit(lim ir.Operand, p *ir.Value) bool {
	if _, lit := ir.ValIntLiteral(lim); lit {
		return true
	}
	v, ok := lim.(*ir.Value)
	return ok && v != p && v.Kind == ir.KindLocal && !v.Volatile && v.Type.Size == 2
}

// ywalkBody はループ ops[a..c] (a: L、b: B、c: 帰りの分岐) が変換の条件を満たすか。加算 `p += 1` は毎周必ず通ること:
// 加算が本体の先頭にある (L から加算までにラベルも分岐も無い。このときは continue = B への分岐もよい: 加算の後なので) か、
// 加算の後ろから B までにラベルが無く B へ飛ぶのが入口の jump だけ。
func ywalkBody(ops []*ir.Op, labelAt map[string]int, a, b, c int, p, t *ir.Value, lim ir.Operand, bLabel string) bool {
	inc := -1
	top := true         // L から加算までにラベルも分岐も無い
	labelAfter := false // 加算の後ろにラベルがある
	toB := false        // ループの中から B へ飛ぶ (continue)
	limV, _ := lim.(*ir.Value)
	for i := a + 1; i < b; i++ {
		op := ops[i]
		if op == nil {
			continue
		}
		switch op.Code {
		case ir.OpReturn, ir.OpSwitch, ir.OpAsm:
			return false
		case ir.OpJump, ir.OpIf, ir.OpIfTrue, ir.OpIfCarry, ir.OpIfNotCarry:
			// 中の分岐はループの中 (L の後ろ、B まで) へだけ (break = 外へ、は出口で p を戻せないので対象外)
			j, ok := labelAt[op.Label]
			if !ok || j <= a || j > b {
				return false
			}
			if j == b {
				toB = true
			}
			if inc < 0 {
				top = false
			}
		case ir.OpLabel:
			if inc < 0 {
				top = false
			} else {
				labelAfter = true
			}
		}
		defs, uses := ir.DefUse(op)
		for _, d := range defs {
			if ir.UnderlyingValue(d) == p {
				if op.Code != ir.OpAdd || op.Dst != ir.Operand(p) || op.Src[0] != ir.Operand(p) || inc >= 0 {
					return false
				}
				if n, lit := ir.ValIntLiteral(op.Src[1]); !lit || n != 1 {
					return false
				}
				inc = i
			}
			if limV != nil && ir.UnderlyingValue(d) == limV {
				return false
			}
			if ir.UnderlyingValue(d) == t {
				return false
			}
		}
		for k, s := range uses {
			if ir.UnderlyingValue(s) != p {
				continue
			}
			if i == inc {
				continue
			}
			// 1 バイトの読み書きの番地としてだけ
			if (op.Code == ir.OpPget || op.Code == ir.OpPset) && k == 0 && op.Src[0] == ir.Operand(p) {
				continue
			}
			return false
		}
	}
	if inc < 0 || !(top || (!labelAfter && !toB)) {
		return false
	}
	// ループの外から B / L へ飛ぶのは入口の jump と帰りの分岐だけ
	for i, op := range ops {
		if op == nil || i == a-1 || i == c || (i > a && i < b) {
			continue
		}
		switch op.Code {
		case ir.OpJump, ir.OpIf, ir.OpIfTrue, ir.OpIfCarry, ir.OpIfNotCarry:
			if op.Label == bLabel || op.Label == ops[a].Label {
				return false
			}
		case ir.OpSwitch:
			return false // 飛び先の表 (念のため関数ごと対象外)
		case ir.OpAsm:
			if contains(op.Text, bLabel) || contains(op.Text, ops[a].Label) {
				return false
			}
		}
	}
	return true
}

// ywalkRewrite は条件を満たしたループを書き換える。
func ywalkRewrite(lmd *ir.Lambda, u8 *types.Type, a, b, c int, p, t *ir.Value, lim ir.Operand) {
	ops := lmd.Ops
	pos := ops[c].Pos
	k := ir.NewLocal("$k", u8, ir.LTNone) // 常駐の候補にする (regalloc は一時変数を常駐させない)
	g := ir.NewLocal("$yguard", t.Type, ir.LTTemp)
	t1 := ir.NewLocal("$ylo", t.Type, ir.LTTemp)
	t2 := ir.NewLocal("$yhi", t.Type, ir.LTTemp)
	lmd.Vars = append(lmd.Vars, k, g, t1, t2)
	p.LogNoValue = true // ループの中の p の値は p と k の組 (@log では読めない)
	lo := ir.NewCastedValue(p, u8, 0)
	hi := ir.NewCastedValue(p, u8, 1)
	limByte := func(n int) ir.Operand {
		if v, lit := ir.ValIntLiteral(lim); lit {
			return ir.NewIntLiteral("", u8, (v>>(8*n))&0xff)
		}
		return ir.NewCastedValue(lim, u8, n)
	}
	xLabel := newLabel(lmd, "ywalk_exit")
	sLabel := newLabel(lmd, "ywalk_skip")
	if sLabel == xLabel {
		sLabel = xLabel + "_s"
	}
	// @log の注釈: 入口の jump B のものは入口の検査へ、ループの条件 (B の後ろ) のものは新しい比較へ (ir/log.go)
	var condLogs []*ir.LogPoint
	for i := b + 1; i <= c; i++ {
		if ops[i] != nil {
			condLogs = append(condLogs, ops[i].Logs...)
		}
	}
	var out []*ir.Op
	out = append(out, ops[:a-1]...)
	// 入口 (jump B の代わり): 元の検査、下位を k へ
	out = append(out,
		&ir.Op{Code: ir.OpLoad, Dst: k, Src: []ir.Operand{lo}, Pos: pos, Logs: ops[a-1].Logs},
		&ir.Op{Code: ir.OpLt, Dst: g, Src: []ir.Operand{p, lim}, Pos: pos},
		&ir.Op{Code: ir.OpIf, Src: []ir.Operand{g}, Label: xLabel, Pos: pos},
		&ir.Op{Code: ir.OpLoad, Dst: lo, Src: []ir.Operand{ir.NewIntLiteral("", u8, 0)}, Pos: pos},
	)
	for i := a; i < b; i++ {
		op := ops[i]
		if op == nil {
			continue
		}
		switch {
		case op.Code == ir.OpPget && op.Src[0] == ir.Operand(p):
			out = append(out, &ir.Op{Code: ir.OpIndexPget, Dst: op.Dst, Src: []ir.Operand{p, k}, Pos: op.Pos, Logs: op.Logs})
		case op.Code == ir.OpPset && op.Src[0] == ir.Operand(p):
			out = append(out, &ir.Op{Code: ir.OpIndexPset, Src: []ir.Operand{p, k, op.Src[1]}, Pos: op.Pos, Logs: op.Logs})
		case op.Code == ir.OpAdd && op.Dst == ir.Operand(p):
			out = append(out,
				&ir.Op{Code: ir.OpAdd, Dst: k, Src: []ir.Operand{k, ir.NewIntLiteral("", u8, 1)}, Pos: op.Pos, Logs: op.Logs},
				&ir.Op{Code: ir.OpIfTrue, Src: []ir.Operand{k}, Label: sLabel, Pos: op.Pos},
				&ir.Op{Code: ir.OpAdd, Dst: hi, Src: []ir.Operand{hi, ir.NewIntLiteral("", u8, 1)}, Pos: op.Pos},
				&ir.Op{Code: ir.OpLabel, Label: sLabel, Pos: op.Pos},
			)
		default:
			out = append(out, op)
		}
	}
	L := ops[a].Label
	out = append(out,
		ops[b], // B
		&ir.Op{Code: ir.OpEq, Dst: t1, Src: []ir.Operand{k, limByte(0)}, Pos: pos, Logs: condLogs},
		&ir.Op{Code: ir.OpIf, Src: []ir.Operand{t1}, Label: L, Pos: pos},
		&ir.Op{Code: ir.OpEq, Dst: t2, Src: []ir.Operand{hi, limByte(1)}, Pos: pos},
		&ir.Op{Code: ir.OpIf, Src: []ir.Operand{t2}, Label: L, Pos: pos},
		&ir.Op{Code: ir.OpLabel, Label: xLabel, Pos: pos},
		&ir.Op{Code: ir.OpLoad, Dst: lo, Src: []ir.Operand{k}, Pos: pos},
	)
	out = append(out, ops[c+1:]...)
	lmd.Ops = out
}
