package driver

// レジスタ割付 (doc/v2_frame_alloc.md §3): バイト単位の詰め込み、フレームへのあふれ、fastcall 領域の大きさ。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFrameSpill(t *testing.T) {
	t.Parallel()
	// 同時に生きる変数が 16 バイトを超える普通の関数: レジスタから外れた分はフレームに置かれ、正しく動く
	out := runEmu(t, `function many(a:int):int16
{
	var v0:int16 = a as int16 + 1;
	var v1:int16 = a as int16 + 2;
	var v2:int16 = a as int16 + 3;
	var v3:int16 = a as int16 + 4;
	var v4:int16 = a as int16 + 5;
	var v5:int16 = a as int16 + 6;
	var v6:int16 = a as int16 + 7;
	var v7:int16 = a as int16 + 8;
	var v8:int16 = a as int16 + 9;
	var v9:int16 = a as int16 + 10;
	var v10:int16 = a as int16 + 11;
	var v11:int16 = a as int16 + 12;
	return v0 + v1 + v2 + v3 + v4 + v5 + v6 + v7 + v8 + v9 + v10 + v11;
}
function bytes(a:int):int
{
	var b0 = a + 1; var b1 = a + 2; var b2 = a + 3; var b3 = a + 4; var b4 = a + 5; var b5 = a + 6;
	var b6 = a + 7; var b7 = a + 8; var b8 = a + 9; var b9 = a + 10; var b10 = a + 11; var b11 = a + 12;
	var b12 = a + 13; var b13 = a + 14; var b14 = a + 15; var b15 = a + 16; var b16 = a + 17; var b17 = a + 18;
	return b0 + b1 + b2 + b3 + b4 + b5 + b6 + b7 + b8 + b9 + b10 + b11 + b12 + b13 + b14 + b15 + b16 + b17;
}
function fc(a:int, b:int16, c:int):int16 options(fastcall: true)
{
	var v0:int16 = b + 1;
	var v1:int16 = b + 2;
	var v2:int16 = b + 3;
	var v3:int16 = b + 4;
	var v4:int16 = b + 5;
	var v5:int16 = b + 6;
	var v6:int16 = b + 7;
	return v0 + v1 + v2 + v3 + v4 + v5 + v6 + a as int16 + c as int16;
}
function main():void
{
	printf(many(1), " ", bytes(1), " ", fc(1, 100, 2), "\n");
	exit(0);
}
`)
	// many: 12 + 78 = 90、bytes: 18 + 171 = 189、fc: 700 + 28 + 3 = 731
	if out != "90 189 731\n" {
		t.Errorf("got %q", out)
	}
}

// TestFastcallStatic: fc で本体を持つ fastcall 関数は静的フレーム (FC_FASTCALL_REG の 16 バイト制限は extern だけ) なので、
// ローカルが多くてもコンパイルでき、呼び出しは呼び先のフレーム F_<sym> に直接書く。
func TestFastcallStatic(t *testing.T) {
	t.Parallel()
	src := `#fc 2
options(fastcall_reg: 16);
function fc(a:int, b:int16, c:int):int16 options(fastcall: true)
{
	var v0:int16 = b + 1;
	var v1:int16 = b + 2;
	var v2:int16 = b + 3;
	var v3:int16 = b + 4;
	var v4:int16 = b + 5;
	var v5:int16 = b + 6;
	var v6:int16 = b + 7;
	return v0 + v1 + v2 + v3 + v4 + v5 + v6 + a as int16 + c as int16;
}
function main():void { fc(1, 2, 3); }
`
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "t.fc"), []byte(src), 0o666); err != nil {
		t.Fatal(err)
	}
	if _, err := NewCompiler(absRepoRoot).Build("t.fc", &BuildOptions{Dir: dir, BuildDir: filepath.Join(dir, "b"), Out: filepath.Join(dir, "a.bin"), CompileOnly: true}); err != nil {
		t.Fatalf("got %v", err)
	}
	mod, _ := os.ReadFile(filepath.Join(dir, "b", "_t.s"))
	frames, _ := os.ReadFile(filepath.Join(dir, "b", "_frames.inc"))
	if !strings.Contains(string(mod), "sta <F_t_fc+3") || !strings.Contains(string(frames), "F_t_fc = FC_SZP+") {
		t.Errorf("_t.s / _frames.inc:\n%s\n%s", mod, frames)
	}
	// base.s に静的フレームの領域と大きさが出る
	base, _ := os.ReadFile(filepath.Join(dir, "b", "base.s"))
	if len(base) > 0 && !strings.Contains(string(base), "FC_SZP_SIZE = 48") {
		t.Errorf("base.s:\n%s", base)
	}
}
