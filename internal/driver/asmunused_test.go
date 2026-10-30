package driver

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// segSize は ld65 のマップ (out と同じ名前の .map) のセグメント name の大きさ (無ければ 0)。
func segSize(t *testing.T, out, name string) int {
	t.Helper()
	b, err := os.ReadFile(strings.TrimSuffix(out, filepath.Ext(out)) + ".map")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(name) + `\s+\w+\s+\w+\s+(\w+)`).FindSubmatch(b)
	if m == nil {
		return 0
	}
	n, _ := strconv.ParseInt(string(m[1]), 16, 32)
	return int(n)
}

// TestAsmUnusedFunctions: include した asm の関数を `.if FC_USED_…` で包めば、使わない関数は ROM に入らない (コンパイラが印を
// 置く。GitHub の Issue #20)。float.asm は内部のルーチンを使う入口の論理和で包むので、どの関数を 1 つだけ使っても組み立て・
// リンクでき、値が正しいことを関数ごとに見る (包みの条件の漏れはここで落ちる)。
func TestAsmUnusedFunctions(t *testing.T) {
	t.Parallel()
	f := func(x float32) uint32 { return math.Float32bits(x) }
	cases := []struct {
		name, expr string
		want       uint32
	}{
		{"add32", fmt.Sprintf("float.add32(%d, %d)", f(1.5), f(2.25)), f(3.75)},
		{"sub32", fmt.Sprintf("float.sub32(%d, %d)", f(1.5), f(2.25)), f(-0.75)},
		{"mul32", fmt.Sprintf("float.mul32(%d, %d)", f(1.5), f(2.25)), f(3.375)},
		{"div32", fmt.Sprintf("float.div32(%d, %d)", f(3), f(2)), f(1.5)},
		{"sqrt32", fmt.Sprintf("float.sqrt32(%d)", f(2.25)), f(1.5)},
		{"eq32", fmt.Sprintf("float.eq32(%d, %d) as u32", f(1), f(1)), 1},
		{"lt32", fmt.Sprintf("float.lt32(%d, %d) as u32", f(1), f(2)), 1},
		{"le32", fmt.Sprintf("float.le32(%d, %d) as u32", f(2), f(1)), 0},
		{"from_i32", "float.from_i32(-3)", f(-3)},
		{"from_u32", "float.from_u32(70000)", f(70000)},
		{"to_i32", fmt.Sprintf("float.to_i32(%d) as u32", f(-7.75)), uint32(0xfffffff9)},
		{"to_u32", fmt.Sprintf("float.to_u32(%d)", f(70000.5)), 70000},
		{"rint32", fmt.Sprintf("float.rint32(%d)", f(2.5)), f(2)},
		{"floor32", fmt.Sprintf("float.floor32(%d)", f(-2.5)), f(-3)},
		{"ceil32", fmt.Sprintf("float.ceil32(%d)", f(-2.5)), f(-2)},
		{"trunc32", fmt.Sprintf("float.trunc32(%d)", f(-2.5)), f(-2)},
		{"round32", fmt.Sprintf("float.round32(%d)", f(2.5)), f(3)},
		{"ldexp32", fmt.Sprintf("float.ldexp32(%d, 3)", f(1.5)), f(12)},
		{"to_sat_i", fmt.Sprintf("float.to_sat_i(%d, 0, false, 8) as u32", f(300)), 127},
		{"to_sat_u", fmt.Sprintf("float.to_sat_u(%d, 0, false, 8)", f(300)), 255},
		{"from16", "float.from16(0x3e00)", f(1.5)},
		{"to16", fmt.Sprintf("float.to16(%d) as u32", f(1.5)), 0x3e00},
	}
	for _, c := range cases {
		src := fmt.Sprintf("#fc 4\nuse float;\nvar r:u32;\nfunction main():void\n{\n\tr = %s;\n\t@printf(\"{}\\n\", r);\n}\n", c.expr)
		r := testBuild(t, buildSpec{Files: map[string]string{"t.fc": src}, Run: true})
		if r.Err != nil {
			t.Errorf("%s: %v", c.name, r.Err)
			continue
		}
		if r.Stdout != fmt.Sprintf("%d\n", c.want) {
			t.Errorf("%s: got %q, want %d", c.name, r.Stdout, c.want)
		}
		// 全部入れると 4 KB あまり。1 つだけなら 1 KB 前後
		if n := segSize(t, r.Out, "float"); n > 1500 {
			t.Errorf("%s: float のセグメントが %d バイト (使わない関数が入っている)", c.name, n)
		}
	}

	// NES の oam: begin / end だけなら asm の spr / meta / meta_pal は入らない
	r := testBuild(t, buildSpec{Files: map[string]string{"t.fc": "#fc 4\nuse oam;\nfunction main():void\n{\n\toam.begin();\n\toam.end();\n}\n"}, Target: "nes"})
	if r.Err != nil {
		t.Fatal(r.Err)
	}
	if n := segSize(t, r.Out, "oam"); n > 150 {
		t.Errorf("oam のセグメントが %d バイト (使わない asm の関数が入っている)", n)
	}
}
