package codegen

import "testing"

// TestOperandIdentity: 番地の同一性は綴りでなく (記号, ずれ, 添字) で決まる (asm.go)。以前の canonAddr は `k+<L+n` の形しか
// 揃えず、`<1+S+3,x` や `<2+FC_FASTCALL_REG` の綴りで同じ番地を別物と見ていた。
func TestOperandIdentity(t *testing.T) {
	same := [][2]string{
		{"1+<F+5", "0+<F+6"}, {"1+<F+5", "<F+6"}, {"<1+S+3,x", "<S+4,x"}, {"0+<S+4,x", "<S+4,x"},
		{"<FC_FASTCALL_REG+2", "<2+FC_FASTCALL_REG"}, {"<reg+0+1", "<reg+1"}, {"_g+0,y", "_g,y"}, {"F_t_f+2", "2+F_t_f"},
		{"1+_g", "_g+1"}, {"<L+$10", "<L+16"},
	}
	for _, c := range same {
		if parseOperand(c[0]).key() != parseOperand(c[1]).key() {
			t.Errorf("%q と %q は同じ場所のはず (%q / %q)", c[0], c[1], parseOperand(c[0]).key(), parseOperand(c[1]).key())
		}
	}
	diff := [][2]string{
		{"<S+4,x", "<S+4"}, {"<S+4,x", "<S+4,y"}, {"<L+1", "<L+2"}, {"_g+1", "_h+1"}, {"(<reg),y", "<reg"}, {"#1", "#2"},
		{"<S+4,x", "(S+4,x)"},
	}
	for _, c := range diff {
		if parseOperand(c[0]).key() == parseOperand(c[1]).key() {
			t.Errorf("%q と %q は別の場所のはず", c[0], c[1])
		}
	}
	modes := map[string]addrMode{
		"": amNone, "a": amNone, "#3": amImm, "#<_sym": amImm, "<L+1": amMem, "_g": amMem, "_g+0,y": amMemY, "<S+3,x": amMemX,
		"(<reg),y": amIndY, "(S+3,x)": amIndX, "(_tbl)": amInd, ".LOBYTE(S+3)": amOther, "<(@1-1)": amOther, "@end_3": amMem,
	}
	for arg, want := range modes {
		if got := parseOperand(arg).Mode; got != want {
			t.Errorf("%q: mode %d, want %d", arg, got, want)
		}
	}
}

// TestAsmLineKinds: 行の種類とサイズ。
func TestAsmLineKinds(t *testing.T) {
	cases := []struct {
		line string
		kind lineKind
		size int
	}{
		{"", lkBlank, 0}, {"; 0001: load", lkComment, 0}, {"@1:", lkLabel, 0}, {"_t_f__direct:", lkLabel, 0},
		{"\t.dbg line, \"t.fc\", 3", lkDirective, 0}, {"\t.byte <(@1-1), <(@2-1)", lkDirective, 2}, {"\t.word 1, 2, 3", lkDirective, 6},
		{"F_t_f = FC_SZP+3", lkDirective, 0},
		{"\tlda <L+1", lkInstr, 2}, {"\tlda _g", lkInstr, 3}, {"\tlda #1", lkInstr, 2}, {"\tlda (<reg),y", lkInstr, 2},
		{"\tinx", lkInstr, 1}, {"\tasl", lkInstr, 1}, {"\tasl a", lkInstr, 1}, {"\tjmp @1", lkInstr, 3}, {"\tbne @1", lkInstr, 2},
		{"\tcall _f", lkInstr, 11}, {"\tfarcall _f, 3", lkInstr, 10}, {"\tlda <L+1" + testMark, lkInstr, 2},
	}
	for _, c := range cases {
		a := parseAsmLine(c.line)
		if a.Kind != c.kind || a.size() != c.size {
			t.Errorf("%q: kind %d size %d, want %d %d", c.line, a.Kind, a.size(), c.kind, c.size)
		}
	}
	if a := parseAsmLine("\tlda <L+1" + testMark); !a.Test || a.Mnem != "lda" || a.Arg.key() != "L+1" {
		t.Errorf("testMark: %+v", a)
	}
	if wa, wx, wy := parseAsmLine("\tjsr __mul_8").writes(); !wa || wx || !wy {
		t.Errorf("__mul_8 writes A and Y: %v %v %v", wa, wx, wy)
	}
	if wa, wx, wy := parseAsmLine("\tasl <L+1").writes(); wa || wx || wy {
		t.Errorf("asl mem writes nothing: %v %v %v", wa, wx, wy)
	}
	if got := parseAsmLine("\t\tldy <L+2").withInstr("tay", "").Text; got != "\t\ttay" {
		t.Errorf("withInstr: %q", got)
	}
}
