package codegen

import (
	"fmt"

	"github.com/haramako/fc/internal/m6502"
)

// extend_jump: 分岐命令の飛び先が ±127 バイトを超えるとき 2 段階ジャンプ (`bne L` → `beq @n; jmp L; @n:`) に
// 変換する (asm の後処理)。
//
// 各命令のサイズは asmLine.size() (即値 / ゼロページ = 2、絶対 = 3、implied = 1、call マクロ = 11、
// 知らない命令 = 10 の安全側)。分岐は「短い (2)」から始めて届かないものを「長い (5)」に変えながら固定点まで繰り返す。
// 長くすると距離が伸びて別の分岐が届かなくなることがあるので、届く分岐を長くすることはあっても逆は無い (単調)。

// extendJump はブランチ命令のジャンプ先が +-127 より遠い場合、2段階ジャンプに変換する。
func (l *Llc) extendJump(asm []string) []string {
	parsed := make([]asmLine, len(asm))
	sizes := make([]int, len(asm))
	for i, line := range asm {
		parsed[i] = parseAsmLine(line)
		sizes[i] = parsed[i].size()
	}

	// 届かない分岐を長くしながら固定点まで
	for {
		addrs := make([]int, len(asm))
		labels := map[string]int{}
		n := 0
		for i, a := range parsed {
			addrs[i] = n
			if a.Kind == lkLabel {
				labels[a.Label] = n
			}
			n += sizes[i]
		}
		changed := false
		for i, a := range parsed {
			if !a.isBranch() || sizes[i] != 2 {
				continue
			}
			to, ok := labels[a.Arg.Raw]
			if !ok {
				continue // 外部のラベル (無いはず)
			}
			if d := to - (addrs[i] + 2); d < -128 || d > 127 {
				sizes[i] = 5
				changed = true
			}
		}
		if !changed {
			break
		}
	}

	result := &asmLines{}
	for i, a := range parsed {
		if a.isBranch() && sizes[i] == 5 {
			label := l.newLabel()
			result.push([]string{
				fmt.Sprintf("\t%s %s", m6502.Table[a.Mnem].Inverse, label),
				fmt.Sprintf("\tjmp %s", a.Arg.Raw),
				label + ":",
			})
			continue
		}
		result.push(asm[i])
	}
	return result.flatten()
}
