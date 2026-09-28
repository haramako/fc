package ir

import (
	"reflect"
	"testing"
)

func TestAsmSymbols(t *testing.T) {
	text := "_top:\n\tjsr _foo\n\tlda #<_bar+1\n\tsta (_ptr),y\n\tjsr @jsr_on_vsync\n; void __fastcall__ nsd_main(void);\n\tlda F_lzw_unpack__dest\n"
	got := AsmSymbols(text)
	want := []string{"_top", "_foo", "_bar", "_ptr", "__fastcall__"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("AsmSymbols = %q, want %q", got, want)
	}
}
