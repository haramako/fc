package driver

// fastcall の呼び出し規約: 引数に呼び出し (fastcall / 普通の関数) を含む場合。

import "testing"

func TestFastcallNestedArgs(t *testing.T) {
	t.Parallel()
	out := runEmu(t, `function inc(a:int):int options(fastcall: true) { return a + 1; }
function add(a:int, b:int):int options(fastcall: true) { return a + b; }
function add16(a:int16, b:int16):int16 options(fastcall: true) { return a + b; }
function twice(a:int):int { return inc(a) + inc(a) - 2; }   // 普通の関数だが中で fastcall を使う
function main():void
{
	var x = 5;
	printf(add(inc(x), inc(inc(x))), " ",        // fastcall の引数に fastcall (入れ子も)
		add(twice(x), inc(x)), " ",              // fastcall の引数に、中で fastcall を使う普通の関数
		add(x, twice(add(x, 1))), " ",           // 混在
		add16(add16(300, 1), add16(2, twice(x)) as int16), "\n");
	exit(0);
}
`)
	// 6+7=13、10+6=16、5+12=17、301+12=313
	if out != "13 16 17 313\n" {
		t.Errorf("got %q", out)
	}
}
