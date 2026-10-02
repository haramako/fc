package driver

// パッケージ間の import 方向を固定する (Agent/discussions/2026-09-12-v2-plan.md R3-a / C3)。
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
		"syntax":    {},
		"types":     {},
		"diag":      {"syntax"},
		"ir":        {"syntax", "types", "diag"},
		"m6502":     {},
		"regalloc":  {"ir", "types", "diag", "m6502"},
		"opt":       {"ir", "types", "diag"},
		"frames":    {"ir", "diag", "types"},
		"codegen":   {"ir", "types", "diag", "regalloc", "m6502"},
		"pipeline":  {"ir", "types", "diag", "opt", "regalloc", "frames"},
		"cc65":      {},
		"emu":       {"r6502"},
		"fchome":    {},
		"project":   {"sema", "diag", "extmacro", "starmacro"},
		"fclog":     {"codegen", "cc65", "ir", "diag", "types"},
		"interp":    {"ir", "types"},
		"sema":      {"syntax", "types", "ir", "diag", "lz4", "rle", "extmacro"},
		"lz4":       {}, // コンパイル時の圧縮 (@lz4 / @rle): 何にも依存しない葉
		"rle":       {},
		"extmacro":  {},           // 外部コマンドの定数マクロのプロセスとプロトコル: 何にも依存しない葉
		"starmacro": {"extmacro"}, // Starlark の定数マクロ (外の依存は go.starlark.net だけ)
		"migrate":   {"syntax", "types", "ir", "sema"},
		"r6502":     {},
		"nes":       {"r6502"},
		"quicknes":  {}, // libretro の QuickNES のコア (画面を確かめるテスト用)
		"doccheck":  {},
		"fcdoc":     {"syntax"}, // fcc doc (モジュールのドキュメント) // 文書を確かめるテストだけ (テストは driver と syntax を使う)
		"driver":    {"syntax", "types", "ir", "diag", "sema", "codegen", "pipeline", "regalloc", "opt", "r6502", "frames", "cc65", "project", "fclog", "emu", "migrate", "extmacro", "starmacro"},
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
