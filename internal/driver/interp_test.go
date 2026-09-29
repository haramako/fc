package driver

import "testing"

// TestInterpArrayValueLoad: 初期値つきのローカル配列 (ROM の表からの配列の値の load) をインタプリタが中身のコピーとして
// 実行する (番地の 2 バイトを書いていて、-O 0 / -O 2 と食い違っていた。fuzz の生成器に実行時の値の配列リテラルを
// 足すときに発覚)。
func TestInterpArrayValueLoad(t *testing.T) {
	t.Parallel()
	src := "#fc 2\nuse * from stdio;\nvar g:int16;\nfunction main():void {\n" +
		"\tvar a:[2]int = [3, 5];\n" +
		"\tg = 300;\n" +
		"\tvar b:[3]int16 = [g, 7, g + 1];\n" +
		"\tprintf(a[0], \" \", a[1], \" \", b[0], \" \", b[1], \" \", b[2], \"\\n\");\n" +
		"\texit(0);\n}\n"
	out, ok, err := rpInterp(t, map[string]string{"t.fc": src})
	if !ok || err != nil {
		t.Fatalf("interp: ok=%v err=%v", ok, err)
	}
	if want := "3 5 300 7 301\n"; out != want {
		t.Errorf("interp の出力 %q, 期待 %q", out, want)
	}
}
