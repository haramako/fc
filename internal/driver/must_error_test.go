package driver

// 「通ってはいけない」プログラムの表 (エラーになる / 警告が出る)。機能ごとのテストにばらばらにあった検査のうち、2026-09-26・27 の
// 調査で「黙って通っていた」と分かったものを中心に 1 か所に集める (検査が緩んだ退行を見つけるため)。新しく検査を
// 足したら、ここにも 1 行足す。

import (
	"fmt"
	"strings"
	"testing"
)

// TestMustError は、各プログラムがコンパイルエラーになり、文言に want を含むことを確かめる。
func TestMustError(t *testing.T) {
	t.Parallel()
	pre := "#fc 3\nstruct S { a:u8; b:u8; }\nenum D { N, E, S }\nconst T:[3]u8 = [1, 2, 3];\nvar A:[4]u8;\nvar g:u8;\nsoa Es:[4]S;\n"
	cases := []struct{ name, src, want string }{
		// 型のない定数・比較
		{"比較で収まらない定数", "function f(s:i8):bool { return s < 200; }", "200 does not fit in i8"},
		{"u8 と -1 の比較", "function f(x:u8):bool { return x == -1; }", "-1 does not fit in u8"},
		{"@min の定数", "function f(s:i8):i8 { return @min(s, 200); }", "200 does not fit in i8"},
		{"case の値 (符号)", "function f(u:u8):void { switch (u) { case -1: g = 1; } }", "case value -1 does not fit in u8"},
		{"case の値 (大きさ)", "function f(u:u8):void { switch (u) { case 300: g = 1; } }", "case value 300 does not fit in u8"},
		{"case の範囲 (空)", "function f(u:u8):void { switch (u) { case 3..3: g = 1; } }", "is empty"},
		{"case の範囲 (重なり)", "function f(u:u8):void { switch (u) { case 0..5: g = 1; case 4: g = 2; } }", "duplicate case value 4"},
		{"case の範囲 (型)", "function f(u:u8):void { switch (u) { case 200..=300: g = 1; } }", "does not fit in u8"},
		// 配列の定数の添字 (範囲外は隣の変数を黙って壊す)
		{"配列の定数の添字 (書く)", "function f():void { A[4] = 1; }", "index 4 is out of range for `A` (type [4]u8: 0..3)"},
		{"配列の定数の添字 (const の表)", "function f():u8 { return T[3]; }", "index 3 is out of range for `T`"},
		{"配列の定数の添字 (負)", "function f():u8 { return A[-1]; }", "index -1 is out of range"},
		{"配列の定数の添字 (struct の配列のフィールド)", "struct Q { v:[2]u8; }\nvar q:Q;\nfunction f():u8 { return q.v[2]; }", "index 2 is out of range"},
		// 読み取り専用
		{"const の表への書き込み", "function f():void { var p = &T[0]; *p = 1; }", "read-only"},
		{"*const の算術で const が外れない", "function f():void { var p:*const u8 = T; var q = p + 1; *q = 2; }", "read-only"},
		{"for-each の値", "function f():void { for (var x in A) { x = 1; } }", "cannot assign to for-each variable `x`"},
		{"for-each の添字", "function f():void { for (var i, x in A) { i += 1; } }", "cannot assign to for-each variable `i`"},
		{"for-each の範囲の変数", "function f():void { for (var i in 0..3) { i++; } }", "cannot assign to for-each variable `i`"},
		{"for-each の要素のポインタ", "function f():void { for (var p in &A) { p++; } }", "cannot assign to for-each variable `p`"},
		{"for-each で const の表に書く", "function f():void { for (var p in &T) { *p = 5; } }", "read-only"},
		{"for-each でポインタの変数から const の表に書く", "function f():void { var q = &T; for (var p in q) { *p = 5; } }", "read-only"},
		// for-each の形
		{"範囲で 2 変数", "function f():void { for (var i, x in 0..3) { } }", "a range gives one variable"},
		{"配列の要素に型", "function f():void { for (var x:u16 in A) { } }", "a type can be written only for a range"},
		{"整数を回す", "function f():void { var n:u8 = 3; for (var x in n) { } }", "cannot iterate over u8"},
		{"範囲を式に", "function f():void { var r = 1..3; }", "parse error"},
		// return 忘れ
		{"return 忘れ", "function f(x:u8):u8 { if (x > 0) { return 1; } }", "missing return"},
		{"ラベル付き switch の中のループから抜ける", "function f(x:u8):u8 { L: switch (x) { case 1: loop { break L; } default: return 2; } }", "missing return"},
		{"case の中の break", "function f(x:u8):u8 { switch (x) { case 1: if (x > 0) { break; } return 1; default: return 2; } }", "missing return"},
		{"default の無い enum の switch", "function f(d:D):u8 { switch (d) { case .N: return 1; case .E: return 2; case .S: return 3; } }", "the switch has no default"},
		{"@if の選ばれた側に return が無い", "function f():u8 { @if (false) { return 1; } }", "missing return"},
		// 配列・リテラル
		{"長さの揃わない配列リテラル", "const M = [[1, 2, 3], [4, 5]];", "different lengths"},
		{"負の長さ", "var z:[-1]u8;", "must not be negative"},
		{"大きすぎる配列", "var z:[70000]u8;", "too large"},
		{"const の表の要素が多すぎる", "const Z:[2]u8 = [1, 2, 3];", ""},
		// struct・ポインタの演算
		{"struct の加算", "function f():void { var a:S; var b:S; var c = a + b; }", "cannot apply +"},
		{"ポインタの乗算", "function f():void { var p:*u8; var q:*u8; var r = p * q; }", "cannot apply *"},
		{"*void の算術", "function f():void { var p:*void; var q = p + 1; }", "no arithmetic on *void"},
		// soa (42f5734)
		{"soa の名前を変数の型に", "var v:Es;", "is not a value type"},
		{"soa の名前を引数の型に", "function f(a:Es):void { }", "is not a value type"},
		// 値でない名前 (モジュール・マクロ・型名) を値に (2026-10-05 まで `var v = math;`・`var v = @lz4;` が黙って通っていた)
		{"モジュールを値に", "use math; function f():void { var v = math; }", "math is a module, not a value"},
		{"マクロを値に", "function f():void { var v = @lz4; }", "@lz4 is a macro, not a value"},
		{"モジュールを呼ぶ", "use math; function f():void { math(1); }", "math is a module, not a value"},
		{"型名を値に", "function f():void { var v = S; }", "S is a type, not a value"},
		{"モジュールを const に", "use math; const X = math;", "math is a module, not a value"},
	}
	for i, c := range cases {
		c := c
		// サブテストの名前は ASCII に (t.TempDir() のパスに入り、英語の Windows の ca65 は日本語のパスを開けない)
		t.Run(fmt.Sprintf("err%02d", i+1), func(t *testing.T) {
			t.Parallel()
			_, err := buildFiles(t, map[string]string{"t.fc": pre + c.src + "\nfunction main():void { }\n"})
			if err == nil {
				t.Fatalf("%s: エラーにならない: %s", c.name, c.src)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("%s: %s: got %v, want %q", c.name, c.src, err, c.want)
			}
		})
	}
}

// TestMustWarn は、各プログラムが警告を出し、文言に want を含むことを確かめる。
func TestMustWarn(t *testing.T) {
	t.Parallel()
	pre := "#fc 3\nuse * from stdio;\nstruct S { a:u8; b:u8; }\nvar A:[4]u8;\n"
	cases := []struct{ name, src, want string }{
		{"必ず真の比較 (u8 < 256)", "function f():void { for (var i = 0; i < 256; i++) { } }", "always has the same result"},
		{"符号なしの < 0", "function f(a:u8, b:u8):bool { return a - b < 0; }", "never negative"},
		{"ローカルのアドレスを返す", "function f():*u8 { var x:u8 = 1; return &x; }", "returning the address of local variable"},
		{"ローカルの配列を返す", "function f():*u8 { var a:[4]u8; a[0] = 1; return a; }", "returning the address of local variable"},
		{"値渡しの引数のアドレスを返す", "function f(s:S):*u8 { return &s.b; }", "returning the address of local variable"},
		{"for-each のポインタを返す", "function f():*u8 { var l:[4]u8; l[0] = 1; for (var p in &l) { return p; } return null; }", "returning the address of local variable"},
		{"初期化していないローカル", "function f():u8 { var x:u8; return x; }", "read before"},
		{"未知の属性", "function f():void @(noinlin: true) { }", "did you mean"},
	}
	for i, c := range cases {
		c := c
		t.Run(fmt.Sprintf("warn%02d", i+1), func(t *testing.T) { // ASCII の名前 (TestMustError と同じ理由)
			t.Parallel()
			_, res, err := buildFilesDefs(t, map[string]string{"t.fc": pre + c.src + "\nfunction main():void { exit(0); }\n"}, nil)
			if err != nil {
				t.Fatalf("%s: ビルド失敗: %v", c.name, err)
			}
			for _, w := range res.Warnings {
				if strings.Contains(w.Msg, c.want) {
					return
				}
			}
			var ws []string
			for _, w := range res.Warnings {
				ws = append(ws, w.Msg)
			}
			t.Errorf("%s: 警告 %q が出ない: %s\n出た警告: %v", c.name, c.want, c.src, ws)
		})
	}
}
