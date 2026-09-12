package fc

// test/fc/test_allocator.rb の移植。

import "testing"

func lr(min, max int) *LiveRange { return &LiveRange{Min: min, Max: max} }

func TestAllocatorUnit(t *testing.T) {
	flow := [][]int{{}, {}, {}, {}, {1}, {}}

	t.Run("should calc live range", func(t *testing.T) {
		lrc := NewLiveRangeCalculator(flow)
		cases := []struct {
			defines, uses []int
			want          *LiveRange
		}{
			{[]int{0, 3}, []int{1, 4}, lr(0, 4)}, // a
			{[]int{1}, []int{3}, lr(1, 3)},       // b
			{[]int{3}, []int{3, 5}, lr(0, 5)},    // c
		}
		for i, c := range cases {
			got := lrc.CalcLiveRange(c.defines, c.uses)
			if got == nil || got.Min != c.want.Min || got.Max != c.want.Max {
				t.Errorf("case %d: got %+v want %+v", i, got, c.want)
			}
		}
	})

	liverange := func(flow [][]int, infos [][2][]int) []*LiveRange {
		lrc := NewLiveRangeCalculator(flow)
		var rs []*LiveRange
		for _, info := range infos {
			if l := lrc.CalcLiveRange(info[0], info[1]); l != nil {
				rs = append(rs, l)
			}
		}
		return rs
	}

	t.Run("should allocate 3 registers", func(t *testing.T) {
		rs := liverange(flow, [][2][]int{
			{{0, 3}, {1, 4}}, // a
			{{1}, {3}},       // b
			{{3}, {3, 5}},    // c
		})
		if regs := allocRanges(rs); len(regs) != 3 {
			t.Errorf("regs: got %d want 3", len(regs))
		}
	})

	t.Run("should allocate 3 registers (with d)", func(t *testing.T) {
		rs := liverange(flow, [][2][]int{
			{{0, 3}, {1, 4}}, // a
			{{1}, {3}},       // b
			{{3}, {3, 5}},    // c
			{{0}, {0}},       // d
		})
		if regs := allocRanges(rs); len(regs) != 3 {
			t.Errorf("regs: got %d want 3", len(regs))
		}
	})
}
