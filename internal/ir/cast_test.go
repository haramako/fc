package ir

import (
	"testing"

	"github.com/haramako/fc/internal/types"
)

// 入れ子の cast は 1 段に畳まれ、内側の切り詰めは Width に残る (`((x as uint8) as int16)` の上位は 0)。
func TestCastedValueCanonical(t *testing.T) {
	u := types.NewUniverse()
	u8, u16, u32 := u.IntType(1, false), u.IntType(2, false), u.IntType(4, false)
	x16 := NewLocal("x", u16, LTNone)
	b8 := NewLocal("b", u8, LTNone)
	s32 := NewLocal("s", u32, LTNone)

	cases := []struct {
		name          string
		cv            *CastedValue
		from          *Value
		offset, width int
		plain, trunc  bool
	}{
		{"widen byte", NewCastedValue(b8, u16, 0), b8, 0, 1, false, false},
		{"narrow then widen", NewCastedValue(NewCastedValue(x16, u8, 0), u16, 0), x16, 0, 1, false, true},
		{"same size", NewCastedValue(NewCastedValue(x16, u16, 0), u16, 0), x16, 0, 2, true, false},
		{"field then byte", NewCastedValue(NewCastedValue(s32, u16, 1), u8, 1), s32, 2, 1, true, false},
		{"field widened", NewCastedValue(NewCastedValue(s32, u8, 3), u16, 0), s32, 3, 1, false, false},
		{"past the end", NewCastedValue(NewCastedValue(x16, u8, 0), u8, 1), x16, 1, 0, false, true},
	}
	for _, c := range cases {
		if c.cv.From != Operand(c.from) || c.cv.Offset != c.offset || c.cv.Width != c.width {
			t.Errorf("%s: got from=%v offset=%d width=%d, want from=%v offset=%d width=%d",
				c.name, c.cv.From, c.cv.Offset, c.cv.Width, c.from, c.offset, c.width)
		}
		if c.cv.Plain() != c.plain || c.cv.Truncated() != c.trunc {
			t.Errorf("%s: plain=%v truncated=%v, want %v %v", c.name, c.cv.Plain(), c.cv.Truncated(), c.plain, c.trunc)
		}
	}

	// リテラルは値のビット列を全部持つので、1 段目では切り詰めない (2 段目の cast が狭めたときだけ)
	lit := NewIntLiteral("", u16, 300)
	if w := NewCastedValue(lit, u16, 0).Width; w != 2 {
		t.Errorf("literal width = %d, want 2", w)
	}
	if w := NewCastedValue(NewCastedValue(lit, u8, 0), u16, 0).Width; w != 1 {
		t.Errorf("narrowed literal width = %d, want 1", w)
	}

	// 差し替えても元の変数で切り詰めた幅は保つ (大きい変数に差し替えても上位を読み出さない)
	y32 := NewLocal("y", u32, LTNone)
	if r := RebaseCast(NewCastedValue(b8, u16, 0), y32); r.From != Operand(y32) || r.Width != 1 {
		t.Errorf("rebase: from=%v width=%d, want y width=1", r.From, r.Width)
	}
	// cast への差し替えは畳む
	if r := RebaseCast(NewCastedValue(b8, u16, 0), NewCastedValue(s32, u8, 2)); r.From != Operand(s32) || r.Offset != 2 || r.Width != 1 {
		t.Errorf("rebase onto cast: from=%v offset=%d width=%d, want s offset=2 width=1", r.From, r.Offset, r.Width)
	}
}
