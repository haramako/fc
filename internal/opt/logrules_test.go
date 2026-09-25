package opt

// @log の注釈 (ir.Op.Logs。doc/development_notes.md「@log の注釈とパス」) を落とさないための規則の検査:
// 命令列の要素を直接 nil にしたり別の命令に差し替えたりせず、ir.DropOp / ir.DiscardOp / ir.ReplaceOp / ir.MergeDrop を
// 通す (注釈を次に実行される命令へ移す)。命令列を作り直すパス (out を組み立てて lmd.Ops = out) は検査できないので、
// そのパスが自分で Logs を引き継ぐ。

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// opWrite は `ops[...] = ...` / `lmd.Ops[...] = ...` (左辺が命令列の要素そのもの) の行。
var opWrite = regexp.MustCompile(`^\s*(?:[A-Za-z_][\w.]*\.)?(?:ops|Ops)\[`)

// directOpWrite は行が命令列の要素への直接の代入か (`ops[i].Src[k] = x` のようにフィールドへの代入は除く)。
func directOpWrite(line string) bool {
	lhs, _, ok := strings.Cut(line, " = ")
	if !ok || !opWrite.MatchString(lhs) {
		return false
	}
	lhs = strings.TrimSpace(lhs)
	start := strings.Index(lhs, "[")
	depth := 0
	for i := start; i < len(lhs); i++ {
		switch lhs[i] {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return i == len(lhs)-1 // 最初の [ が左辺の末尾で閉じる
			}
		}
	}
	return false
}

func TestNoDirectOpWrites(t *testing.T) {
	for _, dir := range []string{".", "../regalloc", "../codegen"} {
		files, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") {
				continue
			}
			b, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			for n, line := range strings.Split(string(b), "\n") {
				if directOpWrite(line) {
					t.Errorf("%s:%d: 命令を直接書き換えている (@log の注釈が落ちる。ir.DropOp / ir.DiscardOp / ir.ReplaceOp / ir.MergeDrop を使う): %s", f, n+1, strings.TrimSpace(line))
				}
			}
		}
	}
}

func TestDirectOpWriteDetection(t *testing.T) {
	for line, want := range map[string]bool{
		"\tops[i] = nil":                   true,
		"\t\tlmd.Ops[i] = &ir.Op{Code: x}": true,
		"\ts.lmd.Ops[jops[0]] = nil":       true,
		"\tops[hops[1]] = newCmp":          true,
		"\tops[u].Src[k] = x":              false,
		"\tlabelIn[ops[i].Label] = true":   false,
		"\tops := lmd.Ops":                 false,
		"\tir.DropOp(ops, i)":              false,
	} {
		if got := directOpWrite(line); got != want {
			t.Errorf("%q: got %v, want %v", line, got, want)
		}
	}
}
