package fc

import (
	"testing"
)

// golden データは testdata/golden/ (リポジトリルート) にあり、
// tools/gen_golden.rb で Ruby版(オラクル)から生成される。
// 形式は doc/go_port_dump_format.md を参照。

// Phase 1: test/test_*.fc + fclib/**/*.fc (x6502除く) のAST一致
func TestGoldenAST(t *testing.T) {
	t.Skip("Phase 1 で実装")
}

// Phase 3: 全テストの HLC 出力(IR)一致
func TestGoldenIR(t *testing.T) {
	t.Skip("Phase 3 で実装")
}

// Phase 4: レジスタ割付+delete_unuse 後のIR一致
func TestGoldenAllocIR(t *testing.T) {
	t.Skip("Phase 4 で実装")
}

// Phase 5: 正規化済みアセンブラ(.s/.inc)一致
func TestGoldenAsm(t *testing.T) {
	t.Skip("Phase 5 で実装")
}

// Phase 6: リンク済みバイナリ(a.bin/a.nes)のバイト一致
func TestGoldenBinary(t *testing.T) {
	t.Skip("Phase 6 で実装")
}

// Phase 7: エミュレータ実行の stdout / 終了コード一致
func TestGoldenStdout(t *testing.T) {
	t.Skip("Phase 7 で実装")
}
