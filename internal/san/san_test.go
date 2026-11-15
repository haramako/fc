package san

import (
	"strings"
	"testing"

	"github.com/haramako/fc/internal/r6502"
)

// run は code を $1000 に置いて、$1000 の rts で S が戻るまで走らせ、検査の誤り (無ければ "") を返す。
func run(t *testing.T, info *Info, code map[int][]byte) string {
	t.Helper()
	mem := r6502.NewMemory()
	for at, bs := range code {
		for i, b := range bs {
			mem.Set(at+i, int(b))
		}
	}
	cpu := r6502.NewCpu(mem)
	chk := NewChecker(mem, info)
	cpu.Mem = chk
	cpu.Pc, cpu.S = 0x1000, 0xfd
	for i := 0; i < 1000 && cpu.S <= 0xfd; i++ {
		chk.Step(cpu)
		cpu.StepSilent()
		if chk.Err() != nil {
			return chk.Err().Error()
		}
	}
	return ""
}

// TestChecker: 手で組んだ小さな 6502 のコードで、検査の 1 つ 1 つを確かめる (コンパイラを通さない)。
func TestChecker(t *testing.T) {
	a := &Func{Name: "a", Entries: []int{0x1000}, Frame: 0x200, Size: 4}
	b := &Func{Name: "b", Entries: []int{0x1010}, Frame: 0x204, Size: 4, Params: []Range{{1, 1}}, Result: 1}
	info := &Info{Funcs: []*Func{a, b}, OutLo: 0xfff0, OutHi: 0x10000}
	const (
		jsr, rts, ldaAbs, staAbs, ldaImm, beq, ldxAbs, ldaAbsX, adcAbs, clc = 0x20, 0x60, 0xad, 0x8d, 0xa9, 0xf0, 0xae, 0xbd, 0x6d, 0x18
	)
	cases := []struct {
		name string
		info *Info
		code map[int][]byte
		want string // 誤りに含む言葉 ("" なら誤りなし)
	}{
		{"読むだけ・写すだけは誤りにしない", info, map[int][]byte{0x1000: {ldaAbs, 0x00, 0x02, staAbs, 0x01, 0x02, rts}}, ""},
		{"未初期化の値で分岐", info, map[int][]byte{0x1000: {ldaAbs, 0x00, 0x02, beq, 0x00, rts}}, "a branch depends on an uninitialized value from the frame of a +0"},
		{"写した先から読んで分岐 (出どころは元の番地)", info, map[int][]byte{0x1000: {ldaAbs, 0x02, 0x02, staAbs, 0x00, 0x03, ldaAbs, 0x00, 0x03, beq, 0x00, rts}},
			"a branch depends on an uninitialized value from the frame of a +2"},
		{"計算に混ぜてから分岐", info, map[int][]byte{0x1000: {ldaImm, 1, clc, adcAbs, 0x03, 0x02, beq, 0x00, rts}}, "from the frame of a +3"},
		{"書いてから読めばよい", info, map[int][]byte{0x1000: {ldaImm, 1, staAbs, 0x00, 0x02, ldaAbs, 0x00, 0x02, beq, 0x00, rts}}, ""},
		{"添字に使う", info, map[int][]byte{0x1000: {ldxAbs, 0x01, 0x02, ldaAbsX, 0x00, 0x30, rts}}, "an address or index is made from an uninitialized value"},
		{"出力に書く", info, map[int][]byte{0x1000: {ldaAbs, 0x00, 0x02, staAbs, 0xf2, 0xff, rts}}, "the program outputs an uninitialized value"},
		// a が引数 (b の +1) を積んで b を呼ぶ。b は引数を読んで戻り値 (+0) に書く。a は戻り値を読む
		{"引数と戻り値", info, map[int][]byte{
			0x1000: {ldaImm, 7, staAbs, 0x05, 0x02, jsr, 0x10, 0x10, ldaAbs, 0x04, 0x02, beq, 0x00, rts},
			0x1010: {ldaAbs, 0x05, 0x02, staAbs, 0x04, 0x02, rts}}, ""},
		{"積んでいない引数", info, map[int][]byte{
			0x1000: {jsr, 0x10, 0x10, rts},
			0x1010: {ldaAbs, 0x05, 0x02, beq, 0x00, rts}}, "from the frame of b +1 ($0205): an argument that was not stored"},
		{"戻った関数のローカル変数は未初期化", info, map[int][]byte{
			0x1000: {jsr, 0x10, 0x10, ldaAbs, 0x06, 0x02, beq, 0x00, rts},
			0x1010: {ldaImm, 1, staAbs, 0x06, 0x02, rts}}, "from the frame of b +2"},
		{"フレームの重なり", &Info{Funcs: []*Func{a, {Name: "c", Entries: []int{0x1010}, Frame: 0x202, Size: 4}}}, map[int][]byte{
			0x1000: {jsr, 0x10, 0x10, rts}, 0x1010: {rts}}, "the static frame of c overlaps the live frame of a at $0202"},
		{"入り直し", info, map[int][]byte{0x1000: {jsr, 0x00, 0x10, rts}}, "a is entered again while it is running"},
	}
	for _, c := range cases {
		got := run(t, c.info, c.code)
		if c.want == "" && got != "" || c.want != "" && !strings.Contains(got, c.want) {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}
