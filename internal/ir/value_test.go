package ir

import (
	"testing"

	"github.com/haramako/fc/internal/types"
)

// ValIntLiteral はリテラルの一部を読む cast (オフセット付き、または Type.Size より狭い Width) でもそのバイトの値を
// 返す。リテラルの値そのものを返すと、split が 16 ビットのリテラルの上位を 255 でなく 65535 と読んで壊れる
// (d643a74 で直したバグ。testdata/bugzoo/valint-width.patch はこれ。2026-10-18 に、この形を通るプログラムが
// 今のテストにも fuzz も一切通っていないと分かったので、ヘルパーの契約をここで直接見張る)。
func TestValIntLiteralPartialCast(t *testing.T) {
	u := types.NewUniverse()
	i8, u8, i16, u16 := u.IntType(1, true), u.IntType(1, false), u.IntType(2, true), u.IntType(2, false)
	lit := func(typ *types.Type, n int) Operand { return NewIntLiteral("", typ, n) }

	cases := []struct {
		name string
		op   Operand
		want int
	}{
		{"リテラルそのもの", lit(u16, 65534), 65534},
		{"下位バイト", NewCastedValue(lit(u16, 65534), u8, 0), 254},
		{"上位バイト", NewCastedValue(lit(u16, 65534), u8, 1), 255},
		{"上位バイト (65535)", NewCastedValue(lit(u16, 65535), u8, 1), 255},
		{"符号違いへの同じ幅の cast", NewCastedValue(lit(i16, -47), u8, 0), 209},
		{"符号付きへの同じ幅の cast は符号拡張", NewCastedValue(lit(i16, -47), i8, 0), -47},
		{"下位バイトの符号付き", NewCastedValue(lit(u16, 0x1FE), i8, 0), -2},
		{"入れ子の cast は畳まれる", NewCastedValue(NewCastedValue(lit(u16, 0xFF00), u16, 1), u8, 0), 255},
	}
	for _, c := range cases {
		got, ok := ValIntLiteral(c.op)
		if !ok {
			t.Errorf("%s: ValIntLiteral が値を返さなかった", c.name)
			continue
		}
		if got != c.want {
			t.Errorf("%s: %d が欲しかったが %d", c.name, c.want, got)
		}
	}
	// リテラルでないと値を返さないこと
	if _, ok := ValIntLiteral(NewLocal("x", u16, LTNone)); ok {
		t.Errorf("ローカル変数がリテラルと読まれた")
	}
}
