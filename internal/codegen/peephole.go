package codegen

// アセンブリ行のピープホール: A レジスタに何が入っているかを追跡して、不要な lda を消す。
//
// 命令列を上から順に見て「A の値 = このメモリ位置 / この即値」の集合を持ち、
//
//   - `lda X` で X が集合にあり、かつ N/Z フラグが今の A の値を反映していれば削除
//     (lda は N/Z を立てるので、間に ldy / cmp などフラグを別の値で上書きする命令があったら消せない)
//   - `sta X` は A を変えないので X を集合に足す
//   - A を書く命令 (adc / and / lda / pla / txa …) で集合を空にする
//   - メモリを書く命令は書いた先を集合から外す。間接・インデックス付きの書き込みと jsr / call は
//     どこを書いたか分からないので全部外す
//   - ラベル (合流点) と分岐命令の後は空にする
//
// 対象は fc が生成する形だけ (オペランドは文字列として同じかどうかで比べる。`0+<L+0` と `<L+0` は別扱い)。
// 副作用が無いことを確認済みの命令だけ扱い、知らない命令は全部を空にする。

import (
	"strings"
)

// aState は「A の値と等しいことが分かっている場所」の集合。
type aState map[string]bool

func peepholeA(lines []string) []string {
	out := make([]string, 0, len(lines))
	state := aState{}
	flagsFromA := false // N/Z が A の値を反映しているか
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, ";") {
			out = append(out, line)
			continue
		}
		if strings.HasSuffix(t, ":") || strings.HasPrefix(t, ".") {
			// ラベル・ディレクティブ: 合流点なので何も分からない
			state = aState{}
			flagsFromA = false
			out = append(out, line)
			continue
		}
		mnem, arg := t, ""
		if i := strings.IndexAny(t, " \t"); i >= 0 {
			mnem, arg = t[:i], strings.TrimSpace(t[i+1:])
		}
		indexed := strings.Contains(arg, ",") || strings.HasPrefix(arg, "(")
		// 追跡するのはゼロページの局所 (レジスタ領域 / フレーム / fastcall 領域 / reg) だけ。グローバル変数は
		// options(address:) の I/O レジスタ ($2002 など。読むたびに値が変わる) と区別できないので追跡しない
		trackable := !indexed && strings.Contains(arg, "<")
		switch mnem {
		case "lda":
			if trackable && state[arg] && flagsFromA {
				continue // A は既にこの値で、フラグもそれを反映している
			}
			state = aState{}
			if trackable {
				state[arg] = true
			}
			flagsFromA = true
		case "sta":
			if indexed {
				state = aState{} // どこを書いたか分からない (ポインタ経由でゼロページの変数を書くこともある)
			} else if trackable {
				state[arg] = true
			}
		case "stx", "sty":
			// メモリを書く。A もフラグも変わらない
			if indexed {
				state = aState{}
			} else {
				delete(state, arg)
			}
		case "inc", "dec", "asl", "lsr", "rol", "ror":
			if arg == "a" {
				state = aState{}
				flagsFromA = true
			} else {
				if indexed {
					state = aState{}
				} else {
					delete(state, arg)
				}
				flagsFromA = false
			}
		case "clc", "sec", "cli", "sei", "cld", "nop", "txs":
			// A もメモリもフラグ (N/Z) も変えない
		case "ldx", "ldy", "iny", "inx", "dey", "dex", "cmp", "cpx", "cpy", "bit":
			// A もメモリも変えないが N/Z は別の値になる
			flagsFromA = false
		case "tay", "tax":
			// A の値を写すので N/Z は A を反映する
			flagsFromA = true
		case "bcc", "bcs", "beq", "bne", "bmi", "bpl", "bvc", "bvs":
			// 分岐: 落ちてくる側では状態はそのまま (飛び先はラベルで空になる)
		default:
			// A を書く (adc / sbc / and / ora / eor / pla / txa / tya ...)、
			// jmp / jsr / rts / call マクロ / farcall など: 何も分からない。
			// A を書く命令は N/Z を A から立てるが、jsr 等は分からないので安全側 (false)
			state = aState{}
			flagsFromA = false
		}
		out = append(out, line)
	}
	return out
}
