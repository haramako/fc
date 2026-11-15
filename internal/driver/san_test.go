package driver

import (
	"os"
	"strings"
	"testing"
)

// TestSanitizer: 実行のときの検査 (internal/san) が、出力には出ないかもしれない誤りを見つける。
func TestSanitizer(t *testing.T) {
	t.Parallel()
	if os.Getenv("FC_NO_SAN") != "" {
		t.Skip("FC_NO_SAN: 検査を切っている")
	}
	run := func(src string, level int) error {
		r := testBuild(t, buildSpec{Files: map[string]string{"t.fc": src}, Run: true, Level: level})
		return r.Err
	}
	// 初期化していないローカル変数の配列を読む (コンパイルのときの警告は単純な変数だけ)
	uninit := `#fc 4
function sum(n:u8):u8 @(noinline)
{
	var buf:[4]u8;
	buf[0] = n;
	var s:u8 = 0;
	for (var i:u8 = 0; i < 4; i += 1) {
		s += buf[i];
	}
	return s;
}
function main():void
{
	@printf("{}\n", sum(3));
}
`
	for _, level := range []int{-1, 0} {
		err := run(uninit, level)
		if err == nil || !strings.Contains(err.Error(), "sanitizer: ") || !strings.Contains(err.Error(), "an uninitialized value from the frame of _t_sum") {
			t.Errorf("-O %d: 初期化していない配列: %v", level, err)
		}
	}
	// 正しいプログラムは通る: 引数・戻り値・ポインタで渡したローカル変数・再帰・関数ポインタ
	ok := `#fc 4
struct P { x:u8; y:u16; }
function fill(p:*[4]u8, v:u8):void @(noinline)
{
	for (var i:u8 = 0; i < 4; i += 1) {
		(*p)[i] = v + i;
	}
}
function add(a:u8, b:u16):u16 @(noinline) { return a + b; }
function mk(a:u8):P @(noinline) { return {a, 300}; }
function fact(n:u8):u16 { return n <= 1 ? 1 : n * fact(n - 1); }
function twice(f:fn(u8):u8, v:u8):u8 @(noinline) { return f(f(v)); }
function inc(v:u8):u8 { return v + 1; }
function main():void
{
	var buf:[4]u8;
	fill(&buf, 10);
	var p = mk(buf[3]);
	@printf("{} {} {} {} {}\n", buf[0] + buf[3], add(buf[1], 1000), p.x + p.y, fact(5), twice(inc, 7));
}
`
	for _, level := range []int{-1, 0} {
		if err := run(ok, level); err != nil {
			t.Errorf("-O %d: 正しいプログラムで: %v", level, err)
		}
	}
}
