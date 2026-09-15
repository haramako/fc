package opt

// パスごとの単体テスト。小さな IR を組んで、書き換え後の命令列を短い表記 (fmtOps) で比べる。
// 実行結果での検証は internal/driver (ops_test.go など) と bench にある。

import (
	"fmt"
	"strings"
	"testing"

	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/types"
)

var tu = types.NewUniverse()

func u8() *types.Type  { return tu.IntType(1, false) }
func u16() *types.Type { return tu.IntType(2, false) }

func local(name string, t *types.Type) *ir.Value { return ir.NewLocal(name, t, ir.LTNone) }
func tmp(name string, t *types.Type) *ir.Value   { return ir.NewLocal(name, t, ir.LTTemp) }
func lit(n int, t *types.Type) *ir.Value         { return ir.NewIntLiteral("", t, n) }

func opnd(o ir.Operand) string {
	if o == nil {
		return "nil"
	}
	if n, ok := ir.ValIntLiteral(o); ok {
		return fmt.Sprintf("#%d", n)
	}
	v := ir.UnderlyingValue(o)
	if cv, ok := o.(*ir.CastedValue); ok {
		return fmt.Sprintf("%s.%d:%d", v.Name, ir.ValOffset(cv), cv.Type.Size)
	}
	return v.Name
}

// fmtOps は命令列を "code dst = src, src" / "code src, src" / "if src L" の形で 1 行ずつ返す。
func fmtOps(lmd *ir.Lambda) []string {
	var r []string
	for _, op := range lmd.Ops {
		if op == nil {
			continue
		}
		var srcs []string
		for _, s := range op.Src {
			srcs = append(srcs, opnd(s))
		}
		switch op.Code {
		case ir.OpLabel, ir.OpJump:
			r = append(r, fmt.Sprintf("%s %s", op.Code, op.Label))
		case ir.OpIf, ir.OpIfTrue:
			r = append(r, fmt.Sprintf("%s %s %s", op.Code, srcs[0], op.Label))
		case ir.OpReturn, ir.OpPset, ir.OpIndexPset, ir.OpFieldPset:
			r = append(r, strings.TrimSpace(fmt.Sprintf("%s %s", op.Code, strings.Join(srcs, ", "))))
		default:
			r = append(r, fmt.Sprintf("%s %s = %s", op.Code, opnd(op.Dst), strings.Join(srcs, ", ")))
		}
	}
	return r
}

func check(t *testing.T, lmd *ir.Lambda, want ...string) {
	t.Helper()
	got := fmtOps(lmd)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got:\n  %s\nwant:\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}

func lambda(ops ...*ir.Op) *ir.Lambda { return &ir.Lambda{Ops: ops} }

func TestCoalesceCopies(t *testing.T) {
	a, b, x, tv := local("a", u16()), local("b", u16()), local("x", u16()), tmp("t", u16())
	lmd := lambda(
		&ir.Op{Code: ir.OpAdd, Dst: tv, Src: []ir.Operand{a, b}},
		&ir.Op{Code: ir.OpLoad, Dst: x, Src: []ir.Operand{tv}},
	)
	coalesceCopies(lmd)
	compact(lmd)
	check(t, lmd, "add x = a, b")

	// t を 2 回使うなら消せない
	lmd = lambda(
		&ir.Op{Code: ir.OpAdd, Dst: tv, Src: []ir.Operand{a, b}},
		&ir.Op{Code: ir.OpLoad, Dst: x, Src: []ir.Operand{tv}},
		&ir.Op{Code: ir.OpReturn, Src: []ir.Operand{tv}},
	)
	coalesceCopies(lmd)
	compact(lmd)
	check(t, lmd, "add t = a, b", "load x = t", "return t")

	// 型が違えば消せない (1 バイトの結果を 2 バイトに入れる)
	t8 := tmp("t8", u8())
	lmd = lambda(
		&ir.Op{Code: ir.OpAdd, Dst: t8, Src: []ir.Operand{lit(1, u8()), lit(2, u8())}},
		&ir.Op{Code: ir.OpLoad, Dst: x, Src: []ir.Operand{t8}},
	)
	coalesceCopies(lmd)
	compact(lmd)
	check(t, lmd, "add t8 = #1, #2", "load x = t8")
}

func TestCoalesceReturn(t *testing.T) {
	a, b, tv := local("a", u16()), local("b", u16()), tmp("t", u16())
	res := ir.NewLocal("$result", u16(), ir.LTResult)
	lmd := lambda(
		&ir.Op{Code: ir.OpAdd, Dst: tv, Src: []ir.Operand{a, b}},
		&ir.Op{Code: ir.OpReturn, Src: []ir.Operand{tv}},
	)
	lmd.Result = res
	coalesceCopies(lmd)
	compact(lmd)
	check(t, lmd, "add $result = a, b", "return $result")

	// 型が違えば (return (a + b) as int) そのまま
	t8 := tmp("t8", u8())
	lmd = lambda(
		&ir.Op{Code: ir.OpAdd, Dst: t8, Src: []ir.Operand{lit(1, u8()), lit(2, u8())}},
		&ir.Op{Code: ir.OpReturn, Src: []ir.Operand{t8}},
	)
	lmd.Result = res
	coalesceCopies(lmd)
	compact(lmd)
	check(t, lmd, "add t8 = #1, #2", "return t8")
}

func TestChainInPlace(t *testing.T) {
	x, k, tv := local("x", u16()), local("k", u16()), tmp("t", u16())
	one := lit(1, u8())
	// x = (x << 1) ^ k → 両方 x の上で
	lmd := lambda(
		&ir.Op{Code: ir.OpShiftLeft, Dst: tv, Src: []ir.Operand{x, one}},
		&ir.Op{Code: ir.OpXor, Dst: x, Src: []ir.Operand{tv, k}},
	)
	chainInPlace(lmd)
	check(t, lmd, "shift_left x = x, #1", "xor x = x, k")

	// x = (x << 1) ^ x は 2 つ目の x が古い値なので書き換えない (2026-09-16 のバグ)
	lmd = lambda(
		&ir.Op{Code: ir.OpShiftLeft, Dst: tv, Src: []ir.Operand{x, one}},
		&ir.Op{Code: ir.OpXor, Dst: x, Src: []ir.Operand{tv, x}},
	)
	chainInPlace(lmd)
	check(t, lmd, "shift_left t = x, #1", "xor x = t, x")

	// x の一部 (CastedValue) を読む場合も同じ
	lmd = lambda(
		&ir.Op{Code: ir.OpShiftLeft, Dst: tv, Src: []ir.Operand{x, one}},
		&ir.Op{Code: ir.OpXor, Dst: x, Src: []ir.Operand{tv, ir.NewCastedValue(x, u16(), 0)}},
	)
	chainInPlace(lmd)
	check(t, lmd, "shift_left t = x, #1", "xor x = t, x.0:2")

	// 1 バイトは対象外 (A に置く方が速い)
	x8, t8 := local("x8", u8()), tmp("t8", u8())
	lmd = lambda(
		&ir.Op{Code: ir.OpShiftLeft, Dst: t8, Src: []ir.Operand{x8, one}},
		&ir.Op{Code: ir.OpXor, Dst: x8, Src: []ir.Operand{t8, lit(7, u8())}},
	)
	chainInPlace(lmd)
	check(t, lmd, "shift_left t8 = x8, #1", "xor x8 = t8, #7")
}

func TestFusePointer(t *testing.T) {
	p := local("p", tu.PointerTo(u8()))
	d, v := local("d", u8()), local("v", u8())
	tv := tmp("t", tu.PointerTo(u8()))
	lmd := lambda(
		&ir.Op{Code: ir.OpAdd, Dst: tv, Src: []ir.Operand{p, lit(3, u8())}},
		&ir.Op{Code: ir.OpPget, Dst: d, Src: []ir.Operand{tv}},
		&ir.Op{Code: ir.OpAdd, Dst: tv, Src: []ir.Operand{p, lit(4, u8())}},
		&ir.Op{Code: ir.OpPset, Src: []ir.Operand{tv, v}},
	)
	// t は 2 回定義されるので融合しない (定義が 1 つで直後にだけ使う一時変数のみ)
	fusePointer(lmd)
	compact(lmd)
	check(t, lmd, "add t = p, #3", "pget d = t", "add t = p, #4", "pset t, v")

	t1, t2 := tmp("t1", tu.PointerTo(u8())), tmp("t2", tu.PointerTo(u8()))
	arr := ir.NewGlobal("arr", tu.ArrayOf(u8(), 10), "_arr")
	i := local("i", u8())
	lmd = lambda(
		&ir.Op{Code: ir.OpAdd, Dst: t1, Src: []ir.Operand{p, lit(3, u8())}},
		&ir.Op{Code: ir.OpPget, Dst: d, Src: []ir.Operand{t1}},
		&ir.Op{Code: ir.OpIndex, Dst: t2, Src: []ir.Operand{arr, i}},
		&ir.Op{Code: ir.OpPset, Src: []ir.Operand{t2, v}},
	)
	fusePointer(lmd)
	compact(lmd)
	check(t, lmd, "field_pget d = p, #3", "index_pset arr, i, v")
}

func TestSinkAddress(t *testing.T) {
	arr := ir.NewGlobal("arr", tu.ArrayOf(u8(), 10), "_arr")
	i, v, w := local("i", u8()), tmp("v", u8()), tmp("w", u8())
	tp := tmp("t", tu.PointerTo(u8()))
	// hlc の `a[i] += 1`: 左辺のアドレスが先に出る → pset の直前へ → fusePointer で index_pset に
	build := func() *ir.Lambda {
		return lambda(
			&ir.Op{Code: ir.OpIndex, Dst: tp, Src: []ir.Operand{arr, i}},
			&ir.Op{Code: ir.OpIndexPget, Dst: v, Src: []ir.Operand{arr, i}},
			&ir.Op{Code: ir.OpAdd, Dst: w, Src: []ir.Operand{v, lit(1, u8())}},
			&ir.Op{Code: ir.OpPset, Src: []ir.Operand{tp, w}},
		)
	}
	lmd := build()
	sinkAddress(lmd)
	check(t, lmd, "index_pget v = arr, i", "add w = v, #1", "index t = arr, i", "pset t, w")
	fusePointer(lmd)
	compact(lmd)
	check(t, lmd, "index_pget v = arr, i", "add w = v, #1", "index_pset arr, i, w")

	// 間で添字が書き換わるなら動かさない
	lmd = build()
	lmd.Ops[2] = &ir.Op{Code: ir.OpAdd, Dst: i, Src: []ir.Operand{i, lit(1, u8())}}
	sinkAddress(lmd)
	check(t, lmd, "index t = arr, i", "index_pget v = arr, i", "add i = i, #1", "pset t, w")

	// ラベルをまたがない
	lmd = build()
	lmd.Ops[2] = &ir.Op{Code: ir.OpLabel, Label: "L"}
	sinkAddress(lmd)
	check(t, lmd, "index t = arr, i", "index_pget v = arr, i", "label L", "pset t, w")

	// グローバルのポインタ変数 + 定数は呼び出しをまたがない (呼び出し先が書き換えるかもしれない)
	gp := ir.NewGlobal("gp", tu.PointerTo(u8()), "_gp")
	fn := ir.NewGlobal("f", tu.Func(nil, tu.Void(), false), "_f")
	lmd = lambda(
		&ir.Op{Code: ir.OpAdd, Dst: tp, Src: []ir.Operand{gp, lit(2, u8())}},
		&ir.Op{Code: ir.OpCall, Src: []ir.Operand{fn}},
		&ir.Op{Code: ir.OpPset, Src: []ir.Operand{tp, w}},
	)
	sinkAddress(lmd)
	check(t, lmd, "add t = gp, #2", "call nil = f", "pset t, w")
}

func TestNarrowBitTest(t *testing.T) {
	x, tv := local("x", u16()), tmp("t", u16())
	lmd := lambda(
		&ir.Op{Code: ir.OpAnd, Dst: tv, Src: []ir.Operand{x, lit(0x8000, u16())}},
		&ir.Op{Code: ir.OpIf, Src: []ir.Operand{tv}, Label: "L"},
		&ir.Op{Code: ir.OpLabel, Label: "L"},
	)
	narrowBitTest(lmd, tu)
	check(t, lmd, "and t = x.1:1, #128", "if t L", "label L")
	if tv.Type.Size != 1 {
		t.Errorf("t should be narrowed to 1 byte, got %s", tv.Type)
	}

	// 両方のバイトにビットがあれば対象外
	tv2 := tmp("t2", u16())
	lmd = lambda(
		&ir.Op{Code: ir.OpAnd, Dst: tv2, Src: []ir.Operand{x, lit(0x0101, u16())}},
		&ir.Op{Code: ir.OpIf, Src: []ir.Operand{tv2}, Label: "L"},
		&ir.Op{Code: ir.OpLabel, Label: "L"},
	)
	narrowBitTest(lmd, tu)
	check(t, lmd, "and t2 = x, #257", "if t2 L", "label L")
}

func TestSimplifyJumps(t *testing.T) {
	c := local("c", u8())
	x := local("x", u8())
	// ジャンプの連鎖と直後への jump、参照されないラベル
	lmd := lambda(
		&ir.Op{Code: ir.OpJump, Label: "A"},
		&ir.Op{Code: ir.OpLabel, Label: "A"},
		&ir.Op{Code: ir.OpLabel, Label: "B"},
		&ir.Op{Code: ir.OpJump, Label: "C"},
		&ir.Op{Code: ir.OpLoad, Dst: x, Src: []ir.Operand{lit(1, u8())}}, // 到達不能
		&ir.Op{Code: ir.OpLabel, Label: "C"},
		&ir.Op{Code: ir.OpReturn},
	)
	simplifyJumps(lmd)
	check(t, lmd, "return")

	// 分岐の反転: if c goto L1; jump L2; L1: → if_true c goto L2
	lmd = lambda(
		&ir.Op{Code: ir.OpIf, Src: []ir.Operand{c}, Label: "L1"},
		&ir.Op{Code: ir.OpJump, Label: "L2"},
		&ir.Op{Code: ir.OpLabel, Label: "L1"},
		&ir.Op{Code: ir.OpLoad, Dst: x, Src: []ir.Operand{lit(1, u8())}},
		&ir.Op{Code: ir.OpLabel, Label: "L2"},
		&ir.Op{Code: ir.OpReturn},
	)
	simplifyJumps(lmd)
	check(t, lmd, "if_true c L2", "load x = #1", "label L2", "return")

	// ループの回転: hlc の while の形
	lmd = lambda(
		&ir.Op{Code: ir.OpLabel, Label: "@begin_1"},
		&ir.Op{Code: ir.OpLt, Dst: c, Src: []ir.Operand{x, lit(10, u8())}},
		&ir.Op{Code: ir.OpIf, Src: []ir.Operand{c}, Label: "@end_2"},
		&ir.Op{Code: ir.OpAdd, Dst: x, Src: []ir.Operand{x, lit(1, u8())}},
		&ir.Op{Code: ir.OpJump, Label: "@begin_1"},
		&ir.Op{Code: ir.OpLabel, Label: "@end_2"},
		&ir.Op{Code: ir.OpReturn},
	)
	simplifyJumps(lmd)
	check(t, lmd,
		"jump @begin_1",
		"label @body_3",
		"add x = x, #1",
		"label @begin_1",
		"lt c = x, #10",
		"if_true c @body_3",
		"return")
}
