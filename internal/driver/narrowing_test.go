package driver

// fc 2 / fc 3 の暗黙の縮小と、fc 3 → 4 の migrate が足す `as` の IR をそろえる (sema.convert)。

import (
	"bytes"
	"testing"
)

// TestV4MigrateNarrowingROM: fc 3 の暗黙の縮小 (初期化・代入・引数・戻り値の u16 / i16 → u8) を fc 4 に migrate すると `as u8` が
// 付くが、ROM は -O 0 / -O 2 とも同じ。暗黙の縮小は値のまま、`as` は cast の形で IR が違い、-O 2 のレジスタの割付が変わって
// ROM が変わっていた (fuzz の TestRandomMigrate の種 53252143。暗黙の縮小も cast の形にそろえた)。
func TestV4MigrateNarrowingROM(t *testing.T) {
	t.Parallel()
	src := `#fc 3
public var out:[8]u8;
public var w:i16;
public var u:u16;
function take(x:u8):u8 @(noinline) { return x + 1; }
function ret(a:u16):u8 @(noinline) { return a + 3; }
function main():void
{
	var l5:u8 = w * 3 + 7;
	var s:u8 = 0;
	for (var i:u8 = 0; i < 10; i += 1) {
		s += l5;
		l5 = u + i;
		out[i & 7] = l5;
	}
	out[0] = s;
	out[1] = take(u);
	out[2] = ret(u);
	out[3] = w;
	while (true) {
	}
}
`
	v4, err := migrateToLatest(t, map[string]string{"t.fc": src})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains([]byte(v4["t.fc"]), []byte("as u8")) {
		t.Fatalf("migrate が as を足していない:\n%s", v4["t.fc"])
	}
	for _, level := range []int{-1, 0} {
		want, err := romBuild(t, map[string]string{"t.fc": src}, level)
		if err != nil {
			t.Fatal(err)
		}
		got, err := romBuild(t, v4, level)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("-O %d: migrate で ROM が変わった\n%s", level, v4["t.fc"])
		}
	}
}
