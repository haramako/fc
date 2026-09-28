package driver

import "os"

// テストでは codegen のレジスタの検査 (FC_VERIFY_REGS。internal/codegen/verify.go) と IR の検査 (FC_VERIFY_IR。
// internal/ir/verify.go) を常に有効にする。fuzz (TestRandomPrograms) で regalloc と codegen の食い違いや、最適化が壊した
// IR の形をコンパイルエラーとして見つける。
func init() {
	os.Setenv("FC_VERIFY_REGS", "1")
	os.Setenv("FC_VERIFY_IR", "1")
}
