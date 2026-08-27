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

	liverange := func(flow [][]int, keys []any, infos [][2][]int) ([]any, []*LiveRange) {
		lrc := NewLiveRangeCalculator(flow)
		var ks []any
		var rs []*LiveRange
		for i, k := range keys {
			l := lrc.CalcLiveRange(infos[i][0], infos[i][1])
			if l != nil {
				ks = append(ks, k)
				rs = append(rs, l)
			}
		}
		return ks, rs
	}

	t.Run("should allocate 3 registers", func(t *testing.T) {
		ks, rs := liverange(flow,
			[]any{"a", "b", "c"},
			[][2][]int{
				{{0, 3}, {1, 4}},
				{{1}, {3}},
				{{3}, {3, 5}},
			})
		alloc := NewAllocatorGeneric(ks, rs)
		if len(alloc.Regs) != 3 {
			t.Errorf("regs: got %d want 3", len(alloc.Regs))
		}
	})

	t.Run("should allocate 3 registers (with d)", func(t *testing.T) {
		ks, rs := liverange(flow,
			[]any{"a", "b", "c", "d"},
			[][2][]int{
				{{0, 3}, {1, 4}},
				{{1}, {3}},
				{{3}, {3, 5}},
				{{0}, {0}},
			})
		alloc := NewAllocatorGeneric(ks, rs)
		if len(alloc.Regs) != 3 {
			t.Errorf("regs: got %d want 3", len(alloc.Regs))
		}
	})
}
