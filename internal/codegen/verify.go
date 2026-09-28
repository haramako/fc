package codegen

// レジスタの書き込みを実際に出した asm から数える。
//   - 常駐 (常に): regalloc.Classify が「この命令はレジスタに触らない」(ResFree) とした常駐レジスタを、命令の本体が
//     書いていたら、その命令だけ退避 / 復帰にして出し直す (compileLambda)。常駐の形は regalloc と codegen が同じ表
//     (regalloc/forms.go) を引くので食い違わないが、表の外の予測 (汎用の出力が A / X / Y を使うか: needsX / needsY、
//     A の値をそのまま扱う形) は外れうる。テストと fuzz (FC_VERIFY_REGS) では食い違いそのものをコンパイルエラーにする
//   - 引数の保持 (FC_VERIFY_REGS=1。テストと fuzz で有効): 呼び出しの引数を A / Y に置いてから call まで、stack 系の
//     push_result (ldx FC_SP) から call まで、間の命令が (退避・復帰も含めて) そのレジスタを書いていないか。codegen の
//     中の約束事なので、破っていればコンパイルエラー
//
// 命令がどのレジスタを書くかは internal/m6502 の表 (asmLine.writes)。

import (
	"fmt"
	"strings"

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/ir"
)

// regsKept は書いてはいけないレジスタ。
type regsKept struct{ a, x, y bool }

func (k regsKept) any() bool { return k.a || k.x || k.y }

func (k regsKept) and(o regsKept) regsKept { return regsKept{k.a && o.a, k.x && o.x, k.y && o.y} }

func (k regsKept) or(o regsKept) regsKept { return regsKept{k.a || o.a, k.x || o.x, k.y || o.y} }

// eachWrite は lines の命令ごとに書くレジスタを f に渡す。`pha … pla` の間の A の書き込みは pla で元に戻るので数えない。
// f が false を返したら止める。
func eachWrite(lines []any, f func(a asmLine, wa, wx, wy bool) bool) {
	depth := 0
	for _, line := range (&asmLines{lines: lines}).flatten() {
		a := parseAsmLine(line)
		if a.Kind != lkInstr {
			continue
		}
		switch {
		case a.Mnem == "pha":
			depth++
			continue
		case a.Mnem == "pla" && depth > 0:
			depth--
			continue
		}
		wa, wx, wy := a.writes()
		if !f(a, wa && depth == 0, wx, wy) {
			return
		}
	}
}

// regsWritten は lines が書くレジスタ。
func regsWritten(lines []any) regsKept {
	var w regsKept
	eachWrite(lines, func(_ asmLine, wa, wx, wy bool) bool {
		w.a, w.x, w.y = w.a || wa, w.x || wx, w.y || wy
		return true
	})
	return w
}

// verifyRegs は lines (1 つの命令が出した行) が keep のレジスタを書いていないか確かめ、書いていればコンパイルエラーにする。
func (l *Llc) verifyRegs(op *ir.Op, what string, keep regsKept, lines []any) {
	if !keep.any() {
		return
	}
	eachWrite(lines, func(a asmLine, wa, wx, wy bool) bool {
		var bad string
		switch {
		case keep.a && wa:
			bad = "A"
		case keep.x && wx:
			bad = "X"
		case keep.y && wy:
			bad = "Y"
		}
		if bad != "" {
			panic(&diag.Error{Msg: fmt.Sprintf("internal: register verify (%s): `%s` writes %s in %s", what, strings.TrimSpace(a.Text), bad, ir.DumpOp(op, nil))})
		}
		return true
	})
}
