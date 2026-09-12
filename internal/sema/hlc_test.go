package sema

// HLC (構文木 → IR) の単体テスト。golden が網羅しない「保存すべき挙動」を小さなソースで固定する。

import (
	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/ir"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// repoRoot はリポジトリルート (fclib を参照するため)。
var repoRoot = func() string {
	p, _ := filepath.Abs(filepath.Join("..", ".."))
	return p
}()

// compileSrc はソース文字列を HLC コンパイルして IR ダンプを返す (失敗時は err)。
func compileSrc(t *testing.T, src string) (string, error) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "t.fc")
	if err := os.WriteFile(path, []byte(src), 0o666); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	prog, err := Compile([]string{".", filepath.ToSlash(filepath.Join(repoRoot, "fclib")), filepath.ToSlash(filepath.Join(repoRoot, "fclib", "emu"))}, "t.fc")
	if err != nil {
		return "", err
	}
	return ir.DumpProgram(prog.Options, prog.Modules.List()), nil
}

func mustCompileSrc(t *testing.T, src string) string {
	t.Helper()
	ir, err := compileSrc(t, src)
	if err != nil {
		t.Fatalf("コンパイル失敗: %v", err)
	}
	return ir
}

func countOps(ir, op string) int {
	return strings.Count(ir, " (:"+op+" ")
}

// TestHlcCompoundAssignEvaluatesLhsTwice: `a[i] += 8` は旧実装の脱糖 (load X (add X 8)) に従い、
// 左辺の index を 2 回計算する (Ruby 由来の挙動。R1-c の純関数化後も維持)。
func TestHlcCompoundAssignEvaluatesLhsTwice(t *testing.T) {
	ir := mustCompileSrc(t, `
var a:int[4];
function main():void { var i:int; i = 1; a[i] += 8; }
`)
	if n := countOps(ir, "index"); n != 2 {
		t.Errorf("index ops = %d, want 2\n%s", n, ir)
	}
	if n := countOps(ir, "pget"); n != 1 {
		t.Errorf("pget ops = %d, want 1\n%s", n, ir)
	}
	if n := countOps(ir, "pset"); n != 1 {
		t.Errorf("pset ops = %d, want 1\n%s", n, ir)
	}
}

// TestHlcConstFold: 定数式は畳み込まれ、Ruby の floor 除算・真偽値 0/1 に従う。
func TestHlcConstFold(t *testing.T) {
	ir := mustCompileSrc(t, `
const A = (7 + 3) * 2;
const B = -7 / 2;
const C = -7 % 3;
const D = 3 < 5;
const E = !0;
const F = 1 << 3 | 2;
function main():void {}
`)
	for _, want := range []string{
		"(def _t_A equ", "val=20", "val=-4", "val=2", "val=1 ", "val=10",
	} {
		if !strings.Contains(ir, want) {
			t.Errorf("IR に %q が無い\n%s", want, ir)
		}
	}
}

// TestHlcWhileForDesugar: while / for は loop+if / 代入+while に脱糖される。
func TestHlcWhileForDesugar(t *testing.T) {
	ir := mustCompileSrc(t, `
var s:int;
function main():void {
  var i:int;
  for (i, 0, 3) { s = s + i; }
  while (i) { i = i - 1; }
}
`)
	// for: load(i,0), while→ lt + if + jump(break) ; while: if + jump
	if n := countOps(ir, "lt"); n != 1 {
		t.Errorf("lt ops = %d, want 1\n%s", n, ir)
	}
	if n := countOps(ir, "if"); n != 2 {
		t.Errorf("if ops = %d, want 2\n%s", n, ir)
	}
}

// TestHlcErrors: 旧実装がランタイムパニックで落ちていた入力も含め、CompileError で報告される。
func TestHlcErrors(t *testing.T) {
	cases := []struct{ src, want string }{
		{"var a:int = 1;\nfunction main():void {}", "can't init global variable"},
		{"function main():void { hoge = 1; }", "hoge not found"},
		{"function main():void { break; }", "cannot break without loop"},
		{"function f():int { return; }\nfunction main():void {}", "can't return without value"},
		{"var x:int;\nconst a = x + 1;\nfunction main():void {}", "must be constant"},
		{"function main():void { var a:int; var b:int[a+1]; }", "array size must be constant"},
		{"function main():void { var p:int*; p = &1; }", "is not left value"},
		{"function main():void { var a:int; *a = 1; }", "is not pointer"},
		{"function main():void { var a:int; a[0] = 1; }", "index must be pointer or array"},
	}
	for _, c := range cases {
		_, err := compileSrc(t, c.src)
		if err == nil {
			t.Errorf("%q: エラーになるべき", c.src)
			continue
		}
		ce, ok := err.(*diag.Error)
		if !ok {
			t.Errorf("%q: *diag.Error であるべき: %T", c.src, err)
			continue
		}
		if !strings.Contains(ce.Msg, c.want) {
			t.Errorf("%q: メッセージ %q に %q が無い", c.src, ce.Msg, c.want)
		}
		if !ce.Pos.IsValid() || ce.Pos.Col == 0 || ce.Pos.Filename == "" {
			t.Errorf("%q: 位置情報が無い: %+v", c.src, ce)
		}
	}
}

// TestHlcErrorPosition: エラー位置は原因となった式 (識別子など) の file:line:col を指す。
// 式に位置が無い場合 (合成ノード) は文の開始位置にフォールバックする。
func TestHlcErrorPosition(t *testing.T) {
	cases := []struct {
		src       string
		line, col int
	}{
		// 未定義識別子: その識別子の位置 (5 行 7 列)
		{"function main():void\n{\n  var a:int;\n  a = 1 +\n      hoge;\n}\n", 5, 7},
		// 型不一致: 代入式の位置 (`a = p` の a)
		{"function main():void\n{\n  var a:int; var p:int*;\n  a = p;\n}\n", 4, 3},
		// 文レベルのエラー: 文の先頭
		{"function main():void\n{\n  break;\n}\n", 3, 3},
		// 式の評価後に文レベルで検出されるエラー (var の初期化禁止): 文の先頭
		{"var a:int = 1;\nfunction main():void {}\n", 1, 1},
	}
	for _, c := range cases {
		_, err := compileSrc(t, c.src)
		ce, ok := err.(*diag.Error)
		if !ok {
			t.Fatalf("%q: CompileError であるべき: %v", c.src, err)
		}
		if ce.Pos.Line != c.line || ce.Pos.Col != c.col {
			t.Errorf("%q: 位置 %d:%d, want %d:%d (%s)", c.src, ce.Pos.Line, ce.Pos.Col, c.line, c.col, ce.Msg)
		}
		if !strings.HasSuffix(ce.Pos.Filename, "t.fc") {
			t.Errorf("ファイル名: %s", ce.Pos.Filename)
		}
	}
}
