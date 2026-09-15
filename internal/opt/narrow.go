package opt

import (
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/types"
)

// narrowBitTest は 2 バイト値のビット検査を 1 バイトに狭める:
//
//	and t = x, #k; if t goto L      (t は一時変数で、この if でだけ使う)
//
// k の下位バイトが 0 なら結果の下位バイトは常に 0 なので、上位バイトだけ検査すればよい:
//
//	and t8 = x.hi, #(k >> 8); if t8 goto L
//
// 上位バイトが 0 なら下位バイトだけ。`if (crc & 0x8000)` のような形が対象で、2 バイトの and (6 命令) + 2 段の分岐が
// 1 バイトの and (A に置かれて `and #128; beq`) になる。
func narrowBitTest(lmd *ir.Lambda, u *types.Universe) {
	ud := ir.BuildUseDef(lmd)
	ops := lmd.Ops
	for i, op := range ops {
		if op == nil || op.Code != ir.OpAnd || i+1 >= len(ops) || ops[i+1] == nil {
			continue
		}
		next := ops[i+1]
		if next.Code != ir.OpIf && next.Code != ir.OpIfTrue {
			continue
		}
		t, ok := op.Dst.(*ir.Value)
		if !ok || t.LocalType != ir.LTTemp || t.Type.Size != 2 || len(ud.Defs[t]) != 1 {
			continue
		}
		if use, single := ud.SingleUse(t); !single || use != i+1 || next.Src[0] != ir.Operand(t) {
			continue
		}
		x := op.Src[0]
		k, lit := ir.ValIntLiteral(op.Src[1])
		if !lit || ir.ValType(x).Size != 2 || ir.ValKind(x) != ir.KindLocal && ir.ValKind(x) != ir.KindGlobal {
			continue
		}
		var byteNo int
		switch {
		case k&0xff == 0:
			byteNo = 1
		case k&0xff00 == 0:
			byteNo = 0
		default:
			continue
		}
		u8 := u.IntType(1, false)
		t.Type = u8
		op.Src = []ir.Operand{ir.NewCastedValue(x, u8, byteNo), ir.NewIntLiteral("", u8, (k>>(8*byteNo))&0xff)}
	}
}
