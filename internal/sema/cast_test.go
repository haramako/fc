package sema

import (
	"strings"
	"testing"
)

// TestCastV2: `x as T` (数値変換) と `bitcast<T>(x)` (ビット読み替え)。doc/v2_types_struct.md §3.5
func TestCastV2(t *testing.T) {
	v2 := func(body string) map[string]string {
		return map[string]string{"t.fc": "#fc 2\nvar a:[4]int;\nvar p:*int;\nvar w:u16;\nvar s:i8;\nvar f:fn(int):void;\nfunction g(x:int):void {}\nfunction main():void {\n" + body + "\n}\n"}
	}
	ok := []string{
		"var x = s as int;",              // 同サイズの符号読み替え
		"var x:int16 = s as int16;",      // 符号拡張
		"var x:int = w as int;",          // 縮小
		"var q:*int = a as *int;",        // 配列 → 同じ要素型のポインタ
		"var x = -s as int + 1;",         // 優先順位: (-s) as int、その後 + 1
		"var x = bitcast<u16>(p);",       // ポインタ → uint16
		"var q = bitcast<*int>(w);",      // uint16 → ポインタ
		"var q = bitcast<*u16>(p);",      // ポインタ → 別のポインタ
		"var q = bitcast<*u16>(a);",      // 配列 → 別の要素型のポインタ
		"var x = bitcast<u16>(f);",       // 関数ポインタ → uint16
		"var x = bitcast<i8>(w as int);", // 同サイズ整数
		"var q = bitcast<*int>(0x2000);", // リテラル
	}
	bad := []struct{ src, want string }{
		{"var q = p as *u16;", "cannot convert *u8 to *u16 with `as`"},
		{"var x = p as u16;", "cannot convert *u8 to u16 with `as`"},
		{"var q = a as *u16;", "cannot convert [4]u8 to *u16 with `as`"},
		{"var x = bitcast<int16>(s);", "sizes differ"},
		{"var x = bitcast<int>(p);", "sizes differ"},
	}
	for _, src := range ok {
		if err := compileFiles(t, v2(src), "t.fc"); err != nil {
			t.Errorf("%s: %v", src, err)
		}
	}
	for _, c := range bad {
		err := compileFiles(t, v2(c.src), "t.fc")
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want /%s/", c.src, err, c.want)
		}
	}
	// as の符号拡張は sign_extension 命令になる
	ir := mustCompileFiles(t, v2("var x:int16 = s as int16;"), "t.fc")
	if !strings.Contains(ir, "sign_extension") {
		t.Errorf("s as int16 が符号拡張になっていない:\n%s", ir)
	}

}
