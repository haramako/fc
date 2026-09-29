package driver

// @format (Agent/wiki/plans/v4-stdlib.md §4) のテスト: 書式をコンパイル時に分解して fmt の関数の呼び出しにする。

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

// TestTryFormat: @try_format は @format と同じに書き、書き先が足りなければ止まらずに長さ 0 の slice を返す (途中の数・文字列・
// 1 文字のどこで足りなくなっても)。失敗の後の @format / printf は普通に動く (状態を begin が戻す)。
func TestTryFormat(t *testing.T) {
	t.Parallel()
	out, err := buildBothLevels(t, map[string]string{"t.fc": `#fc 4
use console;
var buf:[8]u8;
var big:[300]u8;
function main():void
{
	var hp:u16 = 1234;
	var s = @try_format(buf, "HP {}", hp);
	console.write(s);
	printf(" {}", @len(s));
	printf(" {}", @len(@try_format(buf, "HP {:5}/{}", hp, 99)));   // 数で足りない
	printf(" {}", @len(@try_format(buf, "abcdefgh{}", "x")));      // 文字列で足りない
	printf(" {}", @len(@try_format(buf, "abcdefgh{:c}", 65 as u8)));   // 1 文字で足りない
	printf(" {}", @len(@try_format(buf[..3], "{}{}{}{}", 1, 2, 3, 4)));
	for (var i:u16 = 0; i < 300; i += 1) {
		big[i] = 65;
	}
	printf(" {}", @len(@try_format(buf, "{}", big))); // 256 バイトを超える文字列 (u8 の配列は中の最初の 0 まで)
	printf(" [{}]\n", @format(buf, "ok {}", 7));
	console.exit(0);
}
`})
	if err != nil {
		t.Fatal(err)
	}
	if want := "HP 1234 7 0 0 0 0 0 [ok 7]\n"; out != want {
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

// TestFormatTextmap: 書式に textmap の変換器の呼び出しを書くと、`{…}` を解析してから文字の部分を文字表のコードにし、数字も
// 文字表のコード (fmt.codes) で書く (実行時の数も、コンパイル時に畳み込む定数も)。
func TestFormatTextmap(t *testing.T) {
	t.Parallel()
	// 表: ＿=0 　=1 ０..９=2..11 Ａ..Ｆ=12..17 ａ..ｆ=18..23 ー=24 Ｈ=25 Ｐ=26 (textmap は ASCII を全角にしてから引く。- は長音の ー)
	table := "＿　０１２３４５６７８９ＡＢＣＤＥＦａｂｃｄｅｆーＨＰ"
	out, err := buildBothLevels(t, map[string]string{
		"t.txt": table,
		"t.fc": `#fc 4
use console;
const _T = @textmap("t.txt");
var buf:[16]u8;
function dump(s:[]const u8):void
{
	for (var c in s) {
		printf("{} ", c);
	}
	printf("\n");
}
function main():void
{
	var hp:u8 = 7;
	var w:i16 = -26;
	dump(@format(buf, _T("HP {:3}"), hp));
	dump(@format(buf, _T("{:x}{}"), 171 as u8, w));
	dump(@format(buf, _T("{}"), 5));
	console.exit(0);
}
`})
	if err != nil {
		t.Fatal(err)
	}
	want := "25 26 1 1 1 9 \n18 19 24 4 8 \n7 \n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// TestFormatTextmapPO: 書式は .po で `{…}` を含んだ全体を翻訳してから解析する (訳文の {1} / {0} で引数の順を変えられる)。
func TestFormatTextmapPO(t *testing.T) {
	t.Parallel()
	table := "＿　０１２３４５６７８９ＡＢＣＤＥＦａｂｃｄｅｆーＨＰＭ"
	// msgid は原文を全角にしたもの (textmap の照合のキー。英数字と / が全角、{ } と空白はそのまま)
	po := "msgid \"\"\nmsgstr \"\"\n\nmsgid \"ＨＰ {}／{}\"\nmsgstr \"{1}M{0}\"\n"
	out, err := buildBothLevels(t, map[string]string{
		"t.txt": table,
		"t.po":  po,
		"t.fc": `#fc 4
use console;
const _T = @textmap("t.txt", "t.po");
var buf:[16]u8;
function main():void
{
	var hp:u8 = 7;
	var mx:u8 = 30;
	for (var c in @format(buf, _T("HP {}/{}"), hp, mx)) {
		printf("{} ", c);
	}
	printf("\n");
	console.exit(0);
}
`})
	if err != nil {
		t.Fatal(err)
	}
	// "{1}M{0}" → 30 M 7 → ３ ０ Ｍ ７ = 5 2 27 9
	if want := "5 2 27 9 \n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}
