// fcc は FCコンパイラの Go 実装 (bin/fcc 互換 CLI)。
// 移植中: Phase 6 でオプション互換を実装する。
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "fcc (go): not implemented yet")
	os.Exit(1)
}
