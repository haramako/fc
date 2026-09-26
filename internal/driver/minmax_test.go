package driver

// 組み込みの min / max / clamp。

import (
	"strings"
	"testing"
)

func TestMinMaxClamp(t *testing.T) {
	t.Parallel()
	out := runEmu(t, `var s:sint;
var w:int16;
function main():void
{
	var a = 3;
	var b = 200;
	s = -5;
	w = 1000;
	printf(min(a, b), " ", max(a, b), " ", clamp(b, 10, 100), " ", clamp(a, 10, 100), " ", clamp(50, 10, 100), "\n");
	printf(min(s, 3 as sint), " ", max(s, 3 as sint), " ", clamp(s, -2 as sint, 2 as sint), " ", clamp(s, -10 as sint, -7 as sint), "\n");
	printf(min(w, 300 as int16), " ", max(w, 3000 as int16), " ", clamp(w, 0 as int16, 500 as int16), " ", min(a, w), "\n");
	printf(min(7, 3), " ", max(7, 3), " ", clamp(7, 0, 5), "\n");
	s = clamp(s + 20 as sint, -8 as sint, 8 as sint);
	printf(s, "\n");
	exit(0);
}
`)
	want := "3 200 100 10 50\n" +
		"-5 3 -2 -7\n" + // -5, 3, -2, -7 (print_int16 は符号なし表示)
		"300 3000 500 3\n" +
		"3 7 5\n" +
		"8\n"
	if out != want {
		t.Errorf("got %q\nwant %q", out, want)
	}
	got := compileErr(t, "function main():void { var p:*int; var q = min(p, p); }\n")
	if !strings.Contains(got, "arguments must be integers") {
		t.Errorf("pointer: %q", got)
	}
	got = compileErr(t, "function main():void { var q = clamp(1, 2); }\n")
	if !strings.Contains(got, "clamp takes 3 arguments") {
		t.Errorf("arity: %q", got)
	}
}
