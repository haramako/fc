package opt

import (
	"testing"

	"github.com/haramako/fc/internal/ir"
)

// gridColumnLoop は 2 次元の配列 arr[20][10] の列 x を y で回すループ (fieldindex の後の形: mul j = y, #10; add k = j, x)。
func gridColumnLoop() (*ir.Lambda, *ir.Value) {
	arr := ir.NewGlobal("arr", tu.ArrayOf(tu.ArrayOf(u8(), 10), 20), "_arr")
	x, y, acc := local("x", u8()), local("y", u8()), local("acc", u8())
	j, k, d, tv := local("j", u8()), tmp("k", u8()), tmp("d", u8()), tmp("t", u8())
	lmd := lambda(
		op(ir.OpLoad, y, lit(0, u8())),
		op(ir.OpLoad, acc, lit(0, u8())),
		label("@begin"),
		op(ir.OpLt, tv, y, lit(20, u8())),
		ifz(tv, "@end"),
		op(ir.OpMul, j, y, lit(10, u8())),
		op(ir.OpAdd, k, j, x),
		ir.NewLoadMem(d, arr, k, 1, 0),
		op(ir.OpAdd, acc, acc, d),
		op(ir.OpAdd, y, y, lit(1, u8())),
		jump("@begin"),
		label("@end"),
		ret(acc),
	)
	lmd.Args = []*ir.Value{x}
	return lmd, x
}

// TestStrengthGridColumn: y * 10 + x を、y と一緒に 10 ずつ進む変数に置き換える (掛け算と加算の 2 段。y * 10 の変数は
// 加算の唯一の使用なので消える)。初期値は y = 0 なので 0 * 10 + x。
func TestStrengthGridColumn(t *testing.T) {
	lmd, _ := gridColumnLoop()
	propagateSSA(lmd)
	if !reduceStrength(lmd) {
		t.Fatal("not transformed")
	}
	propagateSSA(lmd)
	check(t, lmd,
		"load y = #0",
		"load acc = #0",
		"add k' = #0, x",
		"label @begin",
		"lt t = y, #20",
		"if t @end",
		"load_mem d = arr, k'",
		"add acc = acc, d",
		"add y = y, #1",
		"add k' = k', #10",
		"jump @begin",
		"label @end",
		"return acc",
	)
}

// TestStrengthSkipsCheap: ほかにも使われる誘導変数の x + 1 は置き換えない (毎周の加算と変数が増えるだけ)。2 のべきの
// 掛け算も、唯一の使用でなければ置き換えない (シフトのほうが安い)。
func TestStrengthSkipsCheap(t *testing.T) {
	arr := ir.NewGlobal("arr", tu.ArrayOf(u8(), 64), "_arr")
	x, acc := local("x", u8()), local("acc", u8())
	k, m, d1, d2, tv := tmp("k", u8()), tmp("m", u8()), tmp("d1", u8()), tmp("d2", u8()), tmp("t", u8())
	lmd := lambda(
		op(ir.OpLoad, x, lit(0, u8())),
		op(ir.OpLoad, acc, lit(0, u8())),
		label("@begin"),
		op(ir.OpLt, tv, x, lit(15, u8())),
		ifz(tv, "@end"),
		op(ir.OpAdd, k, x, lit(1, u8())),
		ir.NewLoadMem(d1, arr, k, 1, 0),
		op(ir.OpMul, m, x, lit(4, u8())),
		ir.NewLoadMem(d2, arr, m, 1, 0),
		op(ir.OpAdd, acc, d1, d2),
		op(ir.OpAdd, x, x, lit(1, u8())),
		jump("@begin"),
		label("@end"),
		ret(acc),
	)
	propagateSSA(lmd)
	if reduceStrength(lmd) {
		t.Errorf("transformed:\n%v", fmtOps(lmd))
	}
}

// TestStrengthTwoDefs: 本体の途中でも進める変数 (定義が 2 つ) は誘導変数ではないので置き換えない。
func TestStrengthTwoDefs(t *testing.T) {
	lmd, x := gridColumnLoop()
	// if x goto @skip; add y = y, #1; @skip: を本体の頭に
	y := lmd.Ops[0].Dst.(*ir.Value)
	body := append([]*ir.Op{}, lmd.Ops[:5]...)
	body = append(body, ifz(x, "@skip"), op(ir.OpAdd, y, y, lit(1, u8())), label("@skip"))
	lmd.Ops = append(body, lmd.Ops[5:]...)
	propagateSSA(lmd)
	if reduceStrength(lmd) {
		t.Errorf("transformed:\n%v", fmtOps(lmd))
	}
}

// TestLicmGridRow: 内側のループで変わらない行 y * 16 を、ループの前へ出す。ループの変数を読む x + j は残す。
func TestLicmGridRow(t *testing.T) {
	arr := ir.NewGlobal("arr", tu.ArrayOf(tu.ArrayOf(u8(), 16), 16), "_arr")
	x, y, acc := local("x", u8()), local("y", u8()), local("acc", u8())
	j, k, d, tv := local("j", u8()), tmp("k", u8()), tmp("d", u8()), tmp("t", u8())
	lmd := lambda(
		op(ir.OpLoad, x, lit(0, u8())),
		op(ir.OpLoad, acc, lit(0, u8())),
		label("@begin"),
		op(ir.OpLt, tv, x, lit(16, u8())),
		ifz(tv, "@end"),
		op(ir.OpMul, j, y, lit(16, u8())),
		op(ir.OpAdd, k, j, x),
		ir.NewLoadMem(d, arr, k, 1, 0),
		op(ir.OpAdd, acc, acc, d),
		op(ir.OpAdd, x, x, lit(1, u8())),
		jump("@begin"),
		label("@end"),
		ret(acc),
	)
	lmd.Args = []*ir.Value{y}
	propagateSSA(lmd)
	if !hoistInvariants(lmd) {
		t.Fatal("not transformed")
	}
	lmd.Compact()
	check(t, lmd,
		"load x = #0",
		"load acc = #0",
		"mul j = y, #16",
		"label @begin",
		"lt t = x, #16",
		"if t @end",
		"add k = j, x",
		"load_mem d = arr, k",
		"add acc = acc, d",
		"add x = x, #1",
		"jump @begin",
		"label @end",
		"return acc",
	)
}

// TestLicmUseBeforeDef: 定義が 1 つでも、前の周の値を読む変数 (使用が定義より前) は前へ出さない (1 周目は別の値を読む)。
func TestLicmUseBeforeDef(t *testing.T) {
	x, y, acc, j, tv := local("x", u8()), local("y", u8()), local("acc", u8()), local("j", u8()), tmp("t", u8())
	lmd := lambda(
		op(ir.OpLoad, x, lit(0, u8())),
		op(ir.OpLoad, acc, lit(0, u8())),
		label("@begin"),
		op(ir.OpLt, tv, x, lit(16, u8())),
		ifz(tv, "@end"),
		op(ir.OpAdd, acc, acc, j),
		op(ir.OpMul, j, y, lit(16, u8())),
		op(ir.OpAdd, x, x, lit(1, u8())),
		jump("@begin"),
		label("@end"),
		ret(acc),
	)
	lmd.Args = []*ir.Value{y}
	propagateSSA(lmd)
	if hoistInvariants(lmd) {
		t.Errorf("transformed:\n%v", fmtOps(lmd))
	}
}
