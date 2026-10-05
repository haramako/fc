package regalloc

import (
	"testing"

	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/types"
)

// TestFormGains: 常駐の得 (Classify の gain) は形・規則の命令列から m6502 のサイクル数で数える。以前 regalloc に手で
// 書いていた数字 (ゼロページのサイクル数の差) と同じになる。
func TestFormGains(t *testing.T) {
	u := types.NewUniverse()
	u8, u16 := u.IntType(1, false), u.IntType(2, false)
	lit := func(n int) *ir.Value { return ir.NewIntLiteral("", u8, n) }
	v := ir.NewLocal("v", u8, ir.LTNone)
	x := ir.NewLocal("x", u8, ir.LTNone)
	w := ir.NewLocal("w", u16, ir.LTNone)
	arr := ir.NewGlobal("arr", u.ArrayOf(u8, 16), "_arr")
	tc := ir.NewLocal("$c", u8, ir.LTTemp) // 比較の結果 (直後の if だけが読む)
	op := func(code ir.OpCode, dst ir.Operand, srcs ...ir.Operand) *ir.Op {
		return ir.InferWidthSign(&ir.Op{Code: code, Dst: dst, Src: srcs})
	}
	ifc := &ir.Op{Code: ir.OpIf, Src: []ir.Operand{tc}, Label: "L"}
	cases := []struct {
		name string
		ops  []*ir.Op // 見る命令は先頭 (比較は 2 つ目に if を置く)
		reg  ir.Reg
		want int
	}{
		{"ldy x", []*ir.Op{op(ir.OpLoad, v, x)}, ir.RegY, 3},                                // lda x; sta v → ldy x
		{"sty x", []*ir.Op{op(ir.OpLoad, x, v)}, ir.RegY, 3},                                // lda v; sta x → sty x
		{"ldx #5", []*ir.Op{op(ir.OpLoad, v, lit(5))}, ir.RegX, 3},                          // lda #5; sta v → ldx #5
		{"cpy #0", []*ir.Op{{Code: ir.OpIf, Src: []ir.Operand{v}, Label: "L"}}, ir.RegY, 2}, // lda v → cpy #0 (+1: ピープホール)
		{"cmp #0", []*ir.Op{{Code: ir.OpIf, Src: []ir.Operand{v}, Label: "L"}}, ir.RegA, 1}, // lda v → cmp #0
		{"cpy k", []*ir.Op{op(ir.OpEq, tc, v, x), ifc}, ir.RegY, 3},                         // lda v; cmp x → cpy x
		{"cpx a (可換)", []*ir.Op{op(ir.OpEq, tc, x, v), ifc}, ir.RegX, 3},                    // lda x; cmp v → cpx x
		{"cmp a (可換)", []*ir.Op{op(ir.OpEq, tc, x, v), ifc}, ir.RegA, 3},                    // lda x; cmp v → cmp x
		{"lda v; cmp k (A)", []*ir.Op{op(ir.OpLt, tc, v, x), ifc}, ir.RegA, 3},              // lda v が消える
		{"iny", []*ir.Op{op(ir.OpAdd, v, v, lit(1))}, ir.RegY, 3},                           // inc v (5) → iny (2)
		{"iny × 2", []*ir.Op{op(ir.OpAdd, v, v, lit(2))}, ir.RegY, 6},                       // lda; clc; adc #2; sta (10) → iny × 2 (4)
		{"dex × 4", []*ir.Op{op(ir.OpSub, v, v, lit(4))}, ir.RegX, 2},                       // 10 → dex × 4 (8)
		{"clc; adc #1 (A)", []*ir.Op{op(ir.OpAdd, v, v, lit(1))}, ir.RegA, 1},               // inc v (5) → clc; adc #1 (4)
		{"adc x (A、v に戻す)", []*ir.Op{op(ir.OpAdd, v, v, x)}, ir.RegA, 6},                    // lda v … sta v が消える
		{"asl a", []*ir.Op{op(ir.OpShiftLeft, v, v, lit(1))}, ir.RegA, 3},                   // asl v (5) → asl a (2)
		{"rol a", []*ir.Op{op(ir.OpRolC, v, v)}, ir.RegA, 3},                                // rol v (5) → rol a (2)
		{"lda arr,y", []*ir.Op{ir.NewLoadMem(x, arr, v, 1, 0)}, ir.RegY, 3},                 // ldy v が消える
		{"lda arr,x", []*ir.Op{ir.NewLoadMem(x, arr, v, 1, 0)}, ir.RegX, 3},
		{"tay (A)", []*ir.Op{ir.NewLoadMem(x, arr, v, 1, 0)}, ir.RegA, 1}, // ldy v (3) → tay (2)
		{"sta arr,y (A)", []*ir.Op{ir.NewStoreMem(arr, x, 1, 0, 1, v)}, ir.RegA, 3},
		{"sta w (A)", []*ir.Op{op(ir.OpLoad, w, v)}, ir.RegA, 0}, // 2 バイトの書き込みは A のままでは出せない
	}
	for _, c := range cases {
		lmd := &ir.Lambda{Id: "_f", Type: u.Func(nil, u8, false), Vars: []*ir.Value{v, x, w, tc}, Ops: c.ops}
		var ok bool
		var gain int
		switch c.reg {
		case ir.RegA:
			ok, gain = friendlyA(lmd, 0, v, false, false)
		case ir.RegY:
			ok, gain = friendlyY(lmd, 0, v)
		case ir.RegX:
			ok, gain = friendlyX(lmd, 0, v)
		}
		if c.want == 0 {
			if ok {
				t.Errorf("%s: friendly になった (得 %d)", c.name, gain)
			}
			continue
		}
		if !ok || gain != c.want {
			t.Errorf("%s: friendly=%v 得 %d, want %d", c.name, ok, gain, c.want)
		}
	}
}

// TestFormFreeA: A を触らない形 (命令列が A を書かない) だけが free。2 バイトの x -= 1 は lda lo で下位を見るので A を壊す。
func TestFormFreeA(t *testing.T) {
	u := types.NewUniverse()
	u8, u16 := u.IntType(1, false), u.IntType(2, false)
	lit := func(n int) *ir.Value { return ir.NewIntLiteral("", u8, n) }
	x := ir.NewLocal("x", u8, ir.LTNone)
	w := ir.NewLocal("w", u16, ir.LTNone)
	cases := []struct {
		name string
		op   *ir.Op
		want bool
	}{
		{"inc x", &ir.Op{Code: ir.OpAdd, Dst: x, Src: []ir.Operand{x, lit(1)}}, true},
		{"dec x", &ir.Op{Code: ir.OpSub, Dst: x, Src: []ir.Operand{x, lit(1)}}, true},
		{"inc w (2 バイト)", &ir.Op{Code: ir.OpAdd, Dst: w, Src: []ir.Operand{w, lit(1)}}, true},
		{"dec w (2 バイト)", &ir.Op{Code: ir.OpSub, Dst: w, Src: []ir.Operand{w, lit(1)}}, false},
		{"x += 2", &ir.Op{Code: ir.OpAdd, Dst: x, Src: []ir.Operand{x, lit(2)}}, false},
		{"asl x", &ir.Op{Code: ir.OpShiftLeft, Dst: x, Src: []ir.Operand{x, lit(2)}}, true},
		{"asl w; rol w+1", &ir.Op{Code: ir.OpShiftLeft, Dst: w, Src: []ir.Operand{w, lit(1)}}, true},
		{"w >> 8 (バイトの移動は A)", ir.InferWidthSign(&ir.Op{Code: ir.OpShiftRight, Dst: w, Src: []ir.Operand{w, lit(8)}}), false},
		{"rol x", &ir.Op{Code: ir.OpRolC, Dst: x, Src: []ir.Operand{x}}, true},
		{"label", &ir.Op{Code: ir.OpLabel, Label: "L"}, true},
		{"load (A を通す)", &ir.Op{Code: ir.OpLoad, Dst: x, Src: []ir.Operand{lit(3)}}, false},
	}
	for _, c := range cases {
		lmd := &ir.Lambda{Id: "_f", Type: u.Func(nil, u8, false), Vars: []*ir.Value{x, w}, Ops: []*ir.Op{c.op}}
		if got := freeA(lmd, 0, resPlace{}); got != c.want {
			t.Errorf("%s: freeA = %v, want %v", c.name, got, c.want)
		}
	}
}
