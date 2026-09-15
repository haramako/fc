package codegen

import (
	"fmt"
	"regexp"
)

// extend_jump: 分岐命令の飛び先が ±127 バイトを超えうるとき 2 段階ジャンプに変換する (asm テキスト上の後処理)。

var (
	reEjLabel  = regexp.MustCompile(`^([@._a-zA-Z0-9][_a-zA-Z0-9]+):`)
	reEjInstr  = regexp.MustCompile(`^\s+(\w+)`)
	reEjBranch = regexp.MustCompile(`^\s+(\w+)\s+([@._a-zA-Z0-9][_a-zA-Z0-9]+)`)
)

// extendJump はブランチ命令のジャンプ先が +-127 より遠いかもしれない場合、2段階ジャンプに変換する。
func (l *Llc) extendJump(asm []string) []string {
	opSize := map[string]int{"call": 10}
	branchOps := map[string]string{
		"bcc": "bcs", "bcs": "bcc", "beq": "bne", "bne": "beq", "bmi": "bpl", "bpl": "bmi",
	}
	for _, op := range []string{"brk", "clc", "cld", "clv", "dex", "dey", "inx", "iny", "nop", "pha", "php", "pla", "plp", "rti", "rts",
		"sec", "sec", "sed", "sei", "tax", "tay", "tsx", "txa", "txs", "tya"} {
		opSize[op] = 1
	}
	for _, op := range []string{"adc", "and", "asl", "cmp", "cpx", "cpy", "dec", "eor", "inc", "jmp", "jsr",
		"lda", "ldx", "ldy", "lsr", "ora", "rol", "ror", "sbc", "sta", "stx", "sty"} {
		opSize[op] = 3
	}
	for _, op := range []string{"bcc", "bcs", "beq", "bit", "bmi", "bne", "bpl", "bvc", "bvs"} {
		opSize[op] = 5 // 分割して増えるかもしれない
	}

	// 各ラベルのアドレス候補を求める
	var addrs []int
	labels := map[string]int{}
	n := 0
	for _, line := range asm {
		addrs = append(addrs, n)
		if m := reEjLabel.FindStringSubmatch(line); m != nil {
			labels[m[1]] = n
		} else if m := reEjInstr.FindStringSubmatch(line); m != nil {
			if size, ok := opSize[m[1]]; ok {
				n += size
			} else {
				n += 10 // 知らない命令は、とりえあず10byteとする
			}
		}
	}

	// 書き換えが必要なジャンプを書き換える
	result := &asmLines{}
	for i, line := range asm {
		addr := addrs[i]
		replaced := false
		if m := reEjBranch.FindStringSubmatch(line); m != nil {
			if inv, ok := branchOps[m[1]]; ok {
				jumpTo, hasLabel := labels[m[2]]
				if hasLabel && abs(jumpTo-addr) >= 127 {
					label := l.newLabel()
					result.push([]string{
						fmt.Sprintf("\t%s %s", inv, label),
						fmt.Sprintf("\tjmp %s", m[2]),
						label + ":",
					})
					replaced = true
				}
			}
		}
		if !replaced {
			result.push(line)
		}
	}

	return result.flatten()
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
