package regalloc

// test/fc/test_allocator.rb の移植。

import (
	"testing"

	"github.com/haramako/fc/internal/ir"
)

func lr(min, max int) *ir.LiveRange { return &ir.LiveRange{Min: min, Max: max} }

func TestAllocatorUnit(t *testing.T) {
	flow := [][]int{{}, {}, {}, {}, {1}, {}}

	t.Run("should calc live range", func(t *testing.T) {
		lrc := NewLiveRangeCalculator(flow)
		cases := []struct {
			defines, uses []int
			want          *ir.LiveRange
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

	liverange := func(flow [][]int, infos [][2][]int) []*ir.LiveRange {
		lrc := NewLiveRangeCalculator(flow)
		var rs []*ir.LiveRange
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

// allocRanges は live range の重ならないものを同じレジスタにまとめる (テスト用。生存区間の計算と overlapRange を確かめる)。
// 返り値は各レジスタに入るキーの index のリスト。
func allocRanges(ranges []*ir.LiveRange) [][]int {
	var regs [][]int
	var regRanges []*ir.LiveRange
	for i, lr := range ranges {
		found := false
		for j := range regs {
			if !overlapRange(regRanges[j], lr) {
				regRanges[j] = joinRange(regRanges[j], lr)
				regs[j] = append(regs[j], i)
				found = true
				break
			}
		}
		if !found {
			regs = append(regs, []int{i})
			regRanges = append(regRanges, lr)
		}
	}
	return regs
}

func joinRange(r1, r2 *ir.LiveRange) *ir.LiveRange {
	r := &ir.LiveRange{Min: min(r1.Min, r2.Min), Max: max(r1.Max, r2.Max)}
	for _, w := range append(append([]int{}, r1.Writes...), r2.Writes...) {
		if w < r.Min || w > r.Max {
			r.Writes = append(r.Writes, w)
		}
	}
	return r
}
