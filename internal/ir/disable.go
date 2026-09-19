package ir

// 調査用: 環境変数 FC_DISABLE=名前,名前,... で最適化のパスを個別に切る (doc/development_notes.md (7): 退行やバグは
// 切って比べる)。名前: ssa induction sink fuse coalesce chain narrow scale commute carry split rotate dup inline resident func-resident
// step shift8 fuse-index switch peephole。FC_NO_RESIDENT=1 は resident と同じ。

import (
	"os"
	"strings"
	"sync"
)

var (
	disabledOnce sync.Once
	disabledSet  map[string]bool
)

// Disabled は FC_DISABLE に name が含まれるか。
func Disabled(name string) bool {
	disabledOnce.Do(func() {
		disabledSet = map[string]bool{}
		for _, n := range strings.Split(os.Getenv("FC_DISABLE"), ",") {
			if n = strings.TrimSpace(n); n != "" {
				disabledSet[n] = true
			}
		}
		if os.Getenv("FC_NO_RESIDENT") != "" {
			disabledSet["resident"] = true
		}
	})
	return disabledSet[name]
}
