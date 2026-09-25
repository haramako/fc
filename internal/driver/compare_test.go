package driver

import "testing"

// TestBugSignedCompare: 比較のコード生成 (v1 は左辺の符号しか見ない、符号付きのオーバーフロー未補正、
// 16 ビットで上位バイトの結果を下位で上書き)。関数を分けているのはレジスタ領域 (16 バイト) の都合。
func TestBugSignedCompare(t *testing.T) {
	t.Parallel()
	out := runEmu(t, "function u16():void\n"+
		"{\n"+
		"\tvar a:int16 = 0x0201;\n"+
		"\tvar b:int16 = 0x0102;\n"+
		"\tvar r1 = a < b;\n"+
		"\tvar r2 = b < a;\n"+
		"\tprintf(\"u16 \", r1, \" \", r2, \"\\n\");\n"+
		"\tif (a < b) { printf(\"WRONG \"); }\n"+
		"}\n"+
		"function s8():void\n"+
		"{\n"+
		"\tvar s:sint = -100;\n"+
		"\tvar t:sint = 100;\n"+
		"\tvar r3 = s < t;\n"+
		"\tvar r4 = t < s;\n"+
		"\tprintf(\"s8 \", r3, \" \", r4, \"\\n\");\n"+
		"\tif (s < t) { printf(\"c1 \"); }\n"+
		"\tif (t < s) { printf(\"WRONG \"); }\n"+
		"}\n"+
		"function s8b():void\n"+
		"{\n"+
		"\tvar u:sint = -1;\n"+
		"\tvar r5 = u < 0;\n"+
		"\tvar r6 = u > 0;\n"+
		"\tvar r7 = 0 < u;\n"+
		"\tprintf(\"s8b \", r5, \" \", r6, \" \", r7, \"\\n\");\n"+
		"\tif (u >= 0) { printf(\"WRONG \"); }\n"+
		"}\n"+
		"function s16():void\n"+
		"{\n"+
		"\tvar s16:i16 = -300;\n"+
		"\tvar t16:i16 = 300;\n"+
		"\tvar r8 = s16 < t16;\n"+
		"\tvar r9 = t16 < s16;\n"+
		"\tvar r10 = s16 < 0;\n"+
		"\tprintf(\"s16 \", r8, \" \", r9, \" \", r10, \"\\n\");\n"+
		"\tif (s16 < t16) { printf(\"c2\\n\"); }\n"+
		"}\n"+
		"function main():void\n"+
		"{\n"+
		"\tu16();\n"+
		"\ts8();\n"+
		"\ts8b();\n"+
		"\ts16();\n"+
		"\texit(0);\n"+
		"}\n")
	want := "u16 0 1\ns8 1 0\nc1 s8b 1 0 0\ns16 1 0 1\nc2\n"
	if out != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}
}
