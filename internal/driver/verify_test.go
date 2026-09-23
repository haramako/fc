package driver

import "os"

// テストでは codegen のレジスタの検査 (FC_VERIFY_REGS。internal/codegen/verify.go) を常に有効にする。
// fuzz (TestRandomPrograms) で regalloc と codegen の食い違いをコンパイルエラーとして見つける。
func init() {
	os.Setenv("FC_VERIFY_REGS", "1")
}
