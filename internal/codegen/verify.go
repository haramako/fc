package codegen

// レジスタの検査 (FC_VERIFY_REGS=1。テストと fuzz で有効): 命令ごとに実際に出した asm を見て、
//   - 常駐: regalloc.Classify が「この命令はレジスタに触らない」(ResFree) とした常駐レジスタを、命令の本体が書いていないか
//   - 引数の保持: 呼び出しの引数を A / Y に置いてから call まで、stack 系の push_result (ldx FC_SP) から call まで、
//     間の命令が (退避・復帰も含めて) そのレジスタを書いていないか
// を確かめる。regalloc の「どの命令が A / X / Y を壊すか」(freeA / needsY / needsX …) と codegen の実際の命令列は
// 別々に書かれていて、食い違いが fuzz で何度も出た (2 バイトの dec が A を壊す、push_result の後の ldx など)。
// 食い違いを値の違いになる前に、コンパイルエラーとして見つける。

import (
	"fmt"
	"strings"

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/ir"
)

// regsKept は書いてはいけないレジスタ。
type regsKept struct{ a, x, y bool }

func (k regsKept) any() bool { return k.a || k.x || k.y }

// regWrites は 1 行の命令が書くレジスタ。ラベル・コメント・ディレクティブは何も書かない。
// jsr や知らない命令 (マクロ) は全部を書くと見る。
func regWrites(line string) (a, x, y bool) {
	t := strings.TrimSpace(line)
	if t == "" || strings.HasPrefix(t, ";") || strings.HasPrefix(t, ".") || strings.HasSuffix(t, ":") {
		return
	}
	if k := strings.Index(t, ";"); k >= 0 {
		t = strings.TrimSpace(t[:k])
	}
	mnem, arg := t, ""
	if k := strings.IndexAny(t, " \t"); k >= 0 {
		mnem, arg = t[:k], strings.TrimSpace(t[k+1:])
	}
	switch mnem {
	case "lda", "txa", "tya", "pla", "adc", "sbc", "and", "ora", "eor":
		return true, false, false
	case "asl", "lsr", "rol", "ror":
		return arg == "" || arg == "a", false, false
	case "ldx", "tax", "inx", "dex", "tsx":
		return false, true, false
	case "ldy", "tay", "iny", "dey":
		return false, false, true
	case "sta", "stx", "sty", "inc", "dec", "cmp", "cpx", "cpy", "bit",
		"bcc", "bcs", "beq", "bne", "bmi", "bpl", "bvc", "bvs", "jmp", "rts", "rti",
		"clc", "sec", "cli", "sei", "cld", "sed", "clv", "nop", "pha", "php", "plp", "txs":
		return false, false, false
	case "jsr":
		if strings.HasPrefix(arg, "__mul_") || strings.HasPrefix(arg, "__div_") || strings.HasPrefix(arg, "__mod_") {
			return true, false, true // share/runtime.asm の乗除算は A と Y を使い、X は保つ (__div_16 は退避して戻す)
		}
	}
	return true, true, true
}

// verifyRegs は lines (1 つの命令が出した行) が keep のレジスタを書いていないか確かめ、書いていればコンパイルエラーにする。
// `pha … pla` の間の A の書き込みは pla で元に戻るので数えない。
func (l *Llc) verifyRegs(op *ir.Op, what string, keep regsKept, lines []any) {
	if !keep.any() {
		return
	}
	depth := 0
	for _, line := range (&asmLines{lines: lines}).flatten() {
		t := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(t, "pha"):
			depth++
			continue
		case strings.HasPrefix(t, "pla") && depth > 0:
			depth--
			continue
		}
		a, x, y := regWrites(t)
		var bad string
		switch {
		case keep.a && a && depth == 0:
			bad = "A"
		case keep.x && x:
			bad = "X"
		case keep.y && y:
			bad = "Y"
		}
		if bad != "" {
			panic(&diag.Error{Msg: fmt.Sprintf("internal: register verify (%s): `%s` writes %s in %s", what, t, bad, ir.DumpOp(op, nil))})
		}
	}
}
