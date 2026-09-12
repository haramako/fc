package driver

// パッケージ間の import 方向を固定する (doc/v2_plan.md R3-a / C3)。
// 特に syntax は他の internal パッケージに依存しないこと (フォーマッタが sema 無しで動くため)。

import (
	"os/exec"
	"strings"
	"testing"
)

func internalImports(t *testing.T, pkg string) []string {
	t.Helper()
	out, err := exec.Command("go", "list", "-f", `{{join .Imports "\n"}}`, "github.com/haramako/fc/internal/"+pkg).Output()
	if err != nil {
		t.Fatalf("go list %s: %v", pkg, err)
	}
	var r []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if strings.HasPrefix(line, "github.com/haramako/fc/internal/") {
			r = append(r, strings.TrimPrefix(line, "github.com/haramako/fc/internal/"))
		}
	}
	return r
}

func TestImportDirection(t *testing.T) {
	allowed := map[string][]string{
		"syntax":   {},
		"types":    {},
		"diag":     {"syntax"},
		"ir":       {"syntax", "types", "diag"},
		"regalloc": {"ir", "types", "diag"},
		"codegen":  {"ir", "types", "diag", "regalloc"},
		"sema":     {"syntax", "types", "ir", "diag"},
		"driver":   {"syntax", "types", "ir", "diag", "sema", "codegen", "regalloc", "r6502"},
	}
	for pkg, ok := range allowed {
		okSet := map[string]bool{}
		for _, o := range ok {
			okSet[o] = true
		}
		for _, imp := range internalImports(t, pkg) {
			if !okSet[imp] {
				t.Errorf("%s が %s を import している (許可: %v)", pkg, imp, ok)
			}
		}
	}
}
