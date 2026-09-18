package codegen

import (
	"fmt"
	"regexp"
	"strings"
)

// extend_jump: 分岐命令の飛び先が ±127 バイトを超えるとき 2 段階ジャンプ (`bne L` → `beq @n; jmp L; @n:`) に
// 変換する (asm テキスト上の後処理)。
//
// 各命令のサイズをアドレッシングモードから求め (即値 / ゼロページ = 2、絶対 = 3、implied = 1、call マクロ = 11、
// 知らない命令 = 10 の安全側)、分岐は「短い (2)」から始めて届かないものを「長い (5)」に変えながら固定点まで繰り返す。
// 長くすると距離が伸びて別の分岐が届かなくなることがあるので、届く分岐を長くすることはあっても逆は無い (単調)。

var (
	reEjLabel  = regexp.MustCompile(`^([@._a-zA-Z0-9][_a-zA-Z0-9]+):`)
	reEjInstr  = regexp.MustCompile(`^\s+(\w+)\s*(.*)$`)
	reEjBranch = regexp.MustCompile(`^\s+(\w+)\s+([@._a-zA-Z0-9][_a-zA-Z0-9]+)`)
)

var ejBranchInverse = map[string]string{
	"bcc": "bcs", "bcs": "bcc", "beq": "bne", "bne": "beq", "bmi": "bpl", "bpl": "bmi", "bvc": "bvs", "bvs": "bvc",
}

// instrSize は命令 1 つのサイズ (バイト)。分岐は呼び出し側が決める。
func instrSize(mnem, arg string) int {
	switch mnem {
	case "brk", "clc", "cld", "cli", "clv", "dex", "dey", "inx", "iny", "nop", "pha", "php", "pla", "plp", "rti", "rts",
		"sec", "sed", "sei", "tax", "tay", "tsx", "txa", "txs", "tya":
		return 1
	case "jmp", "jsr":
		return 3
	case "call":
		return 11 // txa pha clc adc# tax jsr pla tax
	case "adc", "and", "asl", "bit", "cmp", "cpx", "cpy", "dec", "eor", "inc", "lda", "ldx", "ldy", "lsr", "ora",
		"rol", "ror", "sbc", "sta", "stx", "sty":
		switch {
		case arg == "" || arg == "a":
			return 1
		case strings.HasPrefix(arg, "#"), strings.HasPrefix(arg, "("), strings.Contains(arg, "<"):
			return 2 // 即値 / 間接 / ゼロページ
		}
		return 3 // 絶対 (ゼロページのシンボルなら ca65 が 2 にするが、大きく見積もる分には安全)
	}
	return 10 // 知らない命令 (インラインアセンブラのマクロなど) は安全側
}

// extendJump はブランチ命令のジャンプ先が +-127 より遠い場合、2段階ジャンプに変換する。
func (l *Llc) extendJump(asm []string) []string {
	type instr struct {
		branch bool
		mnem   string // branch のとき
		target string // branch のとき
		size   int
	}
	instrs := make([]instr, len(asm))
	for i, line := range asm {
		if m := reEjLabel.FindStringSubmatch(line); m != nil {
			continue
		}
		if m := reEjBranch.FindStringSubmatch(line); m != nil {
			if _, ok := ejBranchInverse[m[1]]; ok {
				instrs[i] = instr{branch: true, mnem: m[1], target: m[2], size: 2}
				continue
			}
		}
		if m := reEjInstr.FindStringSubmatch(line); m != nil {
			instrs[i] = instr{size: instrSize(m[1], strings.TrimSpace(m[2]))}
		}
		// ジャンプテーブルのデータ (.byte / .word の並び)
		if t := strings.TrimSpace(line); strings.HasPrefix(t, ".byte ") {
			instrs[i] = instr{size: strings.Count(t, ",") + 1}
		} else if strings.HasPrefix(t, ".word ") {
			instrs[i] = instr{size: 2 * (strings.Count(t, ",") + 1)}
		}
	}

	// 届かない分岐を長くしながら固定点まで
	for {
		addrs := make([]int, len(asm))
		labels := map[string]int{}
		n := 0
		for i, line := range asm {
			addrs[i] = n
			if m := reEjLabel.FindStringSubmatch(line); m != nil {
				labels[m[1]] = n
			}
			n += instrs[i].size
		}
		changed := false
		for i := range instrs {
			in := &instrs[i]
			if !in.branch || in.size != 2 {
				continue
			}
			to, ok := labels[in.target]
			if !ok {
				continue // 外部のラベル (無いはず)
			}
			if d := to - (addrs[i] + 2); d < -128 || d > 127 {
				in.size = 5
				changed = true
			}
		}
		if !changed {
			break
		}
	}

	result := &asmLines{}
	for i, line := range asm {
		in := instrs[i]
		if in.branch && in.size == 5 {
			label := l.newLabel()
			result.push([]string{
				fmt.Sprintf("\t%s %s", ejBranchInverse[in.mnem], label),
				fmt.Sprintf("\tjmp %s", in.target),
				label + ":",
			})
			continue
		}
		result.push(line)
	}
	return result.flatten()
}
