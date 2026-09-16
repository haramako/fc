package regalloc

import (
	"fmt"
	"strings"
	"testing"

	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/types"
)

// TestAllocateResident: crc8 の内側ループの形で crc が A に、カウンタ i が Y に常駐し、入口 / 出口に写しが入る。
//
//	0: load i = #8
//	1: jump @begin
//	2: label @body
//	3: shift_left crc = crc, #1
//	4: if_not_carry @end
//	5: xor crc = crc, #29
//	6: label @end
//	7: sub i = i, #1
//	8: label @begin
//	9: if_true i @body
//	10: return crc
func TestAllocateResident(t *testing.T) {
	u := types.NewUniverse()
	u8 := u.IntType(1, false)
	lit := func(n int) *ir.Value { return ir.NewIntLiteral("", u8, n) }
	crc := ir.NewLocal("crc", u8, ir.LTNone)
	i := ir.NewLocal("i", u8, ir.LTNone)
	lmd := &ir.Lambda{Id: "_f", Type: u.Func(nil, u8, false), Vars: []*ir.Value{crc, i}, Ops: []*ir.Op{
		{Code: ir.OpLoad, Dst: i, Src: []ir.Operand{lit(8)}},
		{Code: ir.OpJump, Label: "@begin"},
		{Code: ir.OpLabel, Label: "@body"},
		{Code: ir.OpShiftLeft, Dst: crc, Src: []ir.Operand{crc, lit(1)}},
		{Code: ir.OpIfNotCarry, Label: "@end"},
		{Code: ir.OpXor, Dst: crc, Src: []ir.Operand{crc, lit(29)}},
		{Code: ir.OpLabel, Label: "@end"},
		{Code: ir.OpSub, Dst: i, Src: []ir.Operand{i, lit(1)}},
		{Code: ir.OpLabel, Label: "@begin"},
		{Code: ir.OpIfTrue, Src: []ir.Operand{i}, Label: "@body"},
		{Code: ir.OpReturn, Src: []ir.Operand{crc}},
	}}
	AllocateResident(lmd)
	var got []string
	for _, op := range lmd.Ops {
		res := ""
		if op.Resident != nil {
			res += fmt.Sprintf(" [a in=%v out=%v]", op.ResIn, op.ResOut)
		}
		if op.ResidentY != nil {
			res += fmt.Sprintf(" [y in=%v out=%v]", op.ResYIn, op.ResYOut)
		}
		got = append(got, ir.DumpOp(op, nil)+res)
	}
	want := []string{
		`(:load {l? i #"uint8"} {lit nil 8 #"uint8"})`,
		`(:load {l? crc@A #"uint8"} {l? crc #"uint8"} "a=crc@A") [a in=false out=true]`,
		`(:load {l? i@Y #"uint8"} {l? i #"uint8"} "y=i@Y") [y in=false out=true]`,
		`(:jump "@begin")`,
		`(:label "@body" "a=crc@A" "y=i@Y") [a in=true out=true] [y in=true out=true]`,
		`(:shift_left {l? crc@A #"uint8"} {l? crc@A #"uint8"} {lit nil 1 #"uint8"} "a=crc@A" "y=i@Y") [a in=true out=true] [y in=true out=true]`,
		`(:if_not_carry "@end" "a=crc@A" "y=i@Y") [a in=true out=true] [y in=true out=true]`,
		`(:xor {l? crc@A #"uint8"} {l? crc@A #"uint8"} {lit nil 29 #"uint8"} "a=crc@A" "y=i@Y") [a in=true out=true] [y in=true out=true]`,
		`(:label "@end" "a=crc@A" "y=i@Y") [a in=true out=true] [y in=true out=true]`,
		`(:sub {l? i@Y #"uint8"} {l? i@Y #"uint8"} {lit nil 1 #"uint8"} "a=crc@A" "y=i@Y") [a in=true out=true] [y in=true out=true]`,
		`(:label "@begin" "a=crc@A" "y=i@Y") [a in=true out=true] [y in=true out=true]`,
		`(:if_true {l? i@Y #"uint8"} "@body" "a=crc@A" "y=i@Y") [a in=true out=true] [y in=true out=true]`,
		`(:load {l? crc #"uint8"} {l? crc@A #"uint8"} "a=crc@A") [a in=true out=false]`,
		`(:return {l? crc #"uint8"})`,
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	// 分類: shift / xor は A friendly、sub (dey) / if_true (cpy) は Y friendly、if_not_carry / ラベルは free
	rA := lmd.Ops[1].Dst.(*ir.Value)
	rY := lmd.Ops[2].Dst.(*ir.Value)
	want2 := map[int]Decision{
		5:  {A: ResFriendly, Y: ResFree, X: ResFree},
		6:  {A: ResFree, Y: ResFree, X: ResFree},
		7:  {A: ResFriendly, Y: ResFree, X: ResFree},
		9:  {A: ResFree, Y: ResFriendly, X: ResFree},
		11: {A: ResFree, Y: ResFriendly, X: ResFree},
	}
	for k, w := range want2 {
		if d, _ := Classify(lmd, k, rA, rY, nil, true, true, true); d != w {
			t.Errorf("op %d: %+v, want %+v", k, d, w)
		}
	}
}
