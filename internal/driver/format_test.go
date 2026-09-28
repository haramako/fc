package driver

// @format (doc/v4_stdlib.md §4) のテスト: 書式をコンパイル時に分解して fmt の関数の呼び出しにする。

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
)

// TestFormatBasic: 幅・0 埋め・位置指定・16 進 / 2 進・符号・文字列 (定数・配列・slice・*u8)・enum・bool・文字・{{ }}・定数の畳み込み。
func TestFormatBasic(t *testing.T) {
	t.Parallel()
	out, err := buildBothLevels(t, map[string]string{"t.fc": `#fc 4
use console;
use fmt;
enum Color { RED, GREEN }
const LABEL = "lit";
var buf:[40]u8;
function main():void
{
	var hp:u8 = 7;
	var max_hp:u16 = 300;
	var s:i8 = -5;
	var name = "joe";
	var sl:[]const u8 = "slice";
	var p:*const u8 = "ptr";
	var c = Color.GREEN;
	var ok = true;
	console.write(@format(buf, "HP {:3}/{}\n", hp, max_hp));
	console.write(@format(buf, "{0:03} {0:02x} {0:b} {1:X}\n", hp, max_hp));
	console.write(@format(buf, "[{:4}] [{:04}] {} {} {}\n", s, s, name, LABEL, sl));
	console.write(@format(buf, "{} {} {} {:d} {:c}{{}}\n", p, c, ok, ok, 65 as u8));
	console.write(@format(buf, "const {} {:x} {:5}|{}\n", 42, 255, -3, false));
	var r = @format(buf, "{}", max_hp);
	console.write(@format(buf[10..], "len {}\n", @len(r)));
	console.exit(0);
}
`})
	if err != nil {
		t.Fatal(err)
	}
	want := "HP   7/300\n007 07 111 12C\n[  -5] [-005] joe lit slice\nptr 1 true 1 A{}\nconst 42 ff    -3|false\nlen 3\n"
	if out != want {
		t.Errorf("got %q\nwant %q", out, want)
	}
}

// TestFormatReference: @format の数を Go の fmt と比べる (乱数の値・種類・幅・埋め。変数と、コンパイル時に畳み込む定数の両方)。
func TestFormatReference(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewSource(2))
	types := []struct {
		name   string
		lo, hi int64
	}{{"u8", 0, 255}, {"u16", 0, 65535}, {"i8", -128, 127}, {"i16", -32768, 32767}}
	verbs := []string{"", "d", "x", "X", "b"}
	var src, want strings.Builder
	src.WriteString("#fc 4\nuse console;\nuse fmt;\nvar buf:[40]u8;\nvar vu8:u8;\nvar vu16:u16;\nvar vi8:i8;\nvar vi16:i16;\n")
	const perFunc = 16
	const n = 96 // 1 回の @format は数十〜100 バイトのコードになる (emu の ROM に収める)
	for i := 0; i < n; i++ {
		if i%perFunc == 0 {
			if i > 0 {
				src.WriteString("}\n")
			}
			fmt.Fprintf(&src, "function part%d():void\n{\n", i/perFunc)
		}
		ty := types[r.Intn(len(types))]
		v := ty.lo + r.Int63n(ty.hi-ty.lo+1)
		verb := verbs[r.Intn(len(verbs))]
		width, zero := r.Intn(9), r.Intn(2) == 0
		spec := ""
		if zero && width > 0 {
			spec += "0"
		}
		if width > 0 {
			spec += fmt.Sprint(width)
		}
		spec += verb
		if spec != "" {
			spec = ":" + spec
		}
		if i%2 == 0 {
			fmt.Fprintf(&src, "\tv%s = %d;\n\tconsole.write(@format(buf, \"<{%s}>\\n\", v%s));\n", ty.name, v, spec, ty.name)
		} else {
			fmt.Fprintf(&src, "\tconsole.write(@format(buf, \"<{%s}>\\n\", %d as %s));\n", spec, v, ty.name)
		}
		// Go: 16 進・2 進は同じ大きさの符号なしとして
		gv := v
		if verb == "x" || verb == "X" || verb == "b" {
			if ty.name == "i8" || ty.name == "u8" {
				gv = v & 0xff
			} else {
				gv = v & 0xffff
			}
		}
		gs := "%"
		if zero && width > 0 {
			gs += "0"
		}
		if width > 0 {
			gs += fmt.Sprint(width)
		}
		if verb == "" {
			gs += "d"
		} else {
			gs += verb
		}
		fmt.Fprintf(&want, "<"+gs+">\n", gv)
	}
	src.WriteString("}\nfunction main():void\n{\n")
	for k := 0; k < n/perFunc; k++ {
		fmt.Fprintf(&src, "\tpart%d();\n", k)
	}
	src.WriteString("\tconsole.exit(0);\n}\n")
	out, err := buildBothLevels(t, map[string]string{"t.fc": src.String()})
	if err != nil {
		t.Fatal(err)
	}
	gl, wl := strings.Split(out, "\n"), strings.Split(want.String(), "\n")
	for i := range wl {
		if i >= len(gl) || gl[i] != wl[i] {
			t.Fatalf("%d 行目: got %q, want %q", i+1, gl[min(i, len(gl)-1)], wl[i])
		}
	}
}

// TestFormatErrors: @format のコンパイルエラー。
func TestFormatErrors(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, body, want string }{
		{"引数が足りない", `console.write(@format(buf, "{} {}", 1));`, "there is no argument 1"},
		{"使わない引数", `console.write(@format(buf, "x", 1));`, "is not used in the format"},
		{"文字列に幅", `console.write(@format(buf, "{:5}", "ab"));`, "a width is for numbers"},
		{"書き先が読み取り専用", `console.write(@format(RO, "x"));`, "the destination is read-only"},
		{"書き先が広い slice", `console.write(@format(big, "x"));`, "at most 255 bytes"},
		{"書けない型", `var q:P; console.write(@format(buf, "{}", q));`, "cannot format a value of type"},
		{"書式が定数でない", `var f:*const u8 = "{}"; console.write(@format(buf, f, 1));`, "the format must be a constant string"},
	}
	for i, c := range cases {
		c := c
		t.Run(fmt.Sprintf("err%02d", i+1), func(t *testing.T) {
			t.Parallel()
			src := "#fc 4\nuse console;\nuse fmt;\nstruct P { x:u8; }\nconst RO = [1, 2, 3];\nvar buf:[16]u8;\nvar big:[300]u8;\nfunction main():void\n{\n\t" + c.body + "\n}\n"
			_, err := buildFiles(t, map[string]string{"t.fc": src})
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("%s: err = %v, want %q", c.name, err, c.want)
			}
		})
	}
	// fmt は use しなくても組み込みが読み込む
	out, err := buildBothLevels(t, map[string]string{"t.fc": "#fc 4\nuse console;\nvar buf:[8]u8;\nfunction main():void { var s = @format(buf, \"x{}\", 1 as u8); printf(\"{}\\n\", s); console.exit(0); }\n"})
	if err != nil || out != "x1\n" {
		t.Errorf("use なし: out = %q, err = %v", out, err)
	}
}
