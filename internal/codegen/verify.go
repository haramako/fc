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
	"regexp"
	"strings"

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/m6502"
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

// flagBits はフラグの組 (N / Z / C / V)。
type flagBits uint8

const (
	flagN flagBits = 1 << iota
	flagZ
	flagC
	flagV
)

// flagUse は命令が読むフラグと書くフラグ (verifyCompareFlags 用。N / Z を書くかは m6502 の表)。
func flagUse(mnem string) (reads, writes flagBits) {
	switch mnem {
	case "beq", "bne":
		reads = flagZ
	case "bmi", "bpl":
		reads = flagN
	case "bcc", "bcs", "adc", "sbc", "rol", "ror":
		reads = flagC
	case "bvc", "bvs":
		reads = flagV
	case "php":
		reads = flagN | flagZ | flagC | flagV
	}
	if m6502.Table[mnem].NZ {
		writes |= flagN | flagZ
	}
	switch mnem {
	case "cmp", "cpx", "cpy", "adc", "sbc", "asl", "lsr", "rol", "ror", "clc", "sec":
		writes |= flagC
	}
	switch mnem {
	case "adc", "sbc", "bit", "clv":
		writes |= flagV
	}
	return reads, writes
}

// reOpComment は codegen が IR の命令ごとに出すコメント (`; 0221: (:lt {l? $899 #"ubool8"} …)`)。
var reOpComment = regexp.MustCompile(`^\s*; \d{4}: \(:\S+ (.*)$`)

// opComment は lines[k] の命令が属する IR の命令のコメントの中身 (オペランドの並び)。無ければ "" 。
func opComment(lines []string, k int) string {
	for ; k >= 0; k-- {
		if m := reOpComment.FindStringSubmatch(lines[k]); m != nil {
			return m[1]
		}
	}
	return ""
}

var reOpDst = regexp.MustCompile(`^\{\S+ (\S+) `)

// opDst は IR の命令のコメントの最初のオペランド (結果) の名前 (`{l? $899 …}` の $899)。
func opDst(c string) string {
	if m := reOpDst.FindStringSubmatch(c); m != nil {
		return m[1]
	}
	return ""
}

// opReads は IR の命令のコメントのオペランドに name の値があるか。
func opReads(c, name string) bool {
	return regexp.MustCompile(`\{\S+ ` + regexp.QuoteMeta(name) + ` `).MatchString(c)
}

// verifyCompareFlags は、比較 (cmp / cpx / cpy / bit) の立てたフラグがまだどれも読まれないうちに別の命令が上書きし、その後で
// 分岐などが上書きされたフラグを読む所があればコンパイルエラーにする (FC_VERIFY_REGS。テストと fuzz で有効)。比較と分岐の
// 間にフラグを壊す命令 (常駐の復帰の ldx など) が挟まったバグの形 (`cmp; ldx; bne`。castle の踏むスイッチ・梯子バグ・
// TestIfCallResultFlags・TestResidentCondRestore の系統)。直線の範囲だけ見て、ラベル・jsr / jmp / rts / rti / brk / plp・
// 知らない命令 (マクロ) で止める (読まれるとみなす)。
//
// フラグが 1 つも読まれずに全部上書きされる比較 (使われない比較) もエラーにする: 比較の結果を使う命令が畳まれて比較だけが
// 残る最適化の取りこぼしで、出力は正しいが無駄 (2026-10-18 は fuzz の 4000 本で 37 本あり、ssa の `x & 0` の畳み込みと
// jumps の dropDeadTemps で 0 本にしてからエラーにした。Agent/issues/local-2026-10-18-dead-compare.md)。
func verifyCompareFlags(fn string, lines []string) {
	for i, line := range lines {
		a := parseAsmLine(line)
		if a.Kind != lkInstr || (a.Mnem != "cmp" && a.Mnem != "cpx" && a.Mnem != "cpy" && a.Mnem != "bit") {
			continue
		}
		_, live := flagUse(a.Mnem)
		var killed flagBits // 比較が立て、まだ読まれないうちに上書きされたフラグ
	scan:
		for j := i + 1; j < len(lines); j++ {
			b := parseAsmLine(lines[j])
			if b.Kind == lkLabel {
				break
			}
			if b.Kind != lkInstr {
				continue
			}
			switch b.Mnem {
			case "jsr", "jmp", "rts", "rti", "brk", "plp":
				break scan
			}
			if _, ok := m6502.Table[b.Mnem]; !ok {
				break
			}
			reads, writes := flagUse(b.Mnem)
			if reads&live != 0 {
				break // 比較の結果が使われた
			}
			if reads&killed != 0 {
				// 上書きした命令の結果を読む分岐もある (`cmp` (使われない); `dec l1`; `beq`)。比較の IR の命令の結果を、分岐の IR の
				// 命令が入力にしているときだけ誤り (codegen が命令ごとに出す `; 0221: (:lt {l? $899 …} …)` のコメントで見る)
				if dst := opDst(opComment(lines, i)); dst != "" && opReads(opComment(lines, j), dst) {
					panic(&diag.Error{Msg: fmt.Sprintf("internal: flag verify: `%s` reads flags of `%s` that were overwritten in between in %s",
						strings.TrimSpace(b.Text), strings.TrimSpace(a.Text), fn)})
				}
				break
			}
			killed |= live & writes
			if live &^= writes; live == 0 {
				panic(&diag.Error{Msg: fmt.Sprintf("internal: flag verify: the flags of `%s` are all overwritten before any use (by `%s`) in %s",
					strings.TrimSpace(a.Text), strings.TrimSpace(b.Text), fn)})
			}
		}
	}
}
