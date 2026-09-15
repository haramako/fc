package codegen

// アセンブリ行のピープホール: A / Y レジスタに何が入っているかを追跡して、不要な lda / ldy / cmp #0 を消す。
//
// 命令列を上から順に見て「A の値 = このメモリ位置 / この即値」の集合と「Y の値 = この場所 / 即値」を持ち、
//
//   - `lda X` で X が集合にあり、かつ N/Z フラグが今の A の値を反映していれば削除
//     (lda は N/Z を立てるので、間に ldy / cmp などフラグを別の値で上書きする命令があったら消せない)
//   - `ldy X` で Y が既に X なら削除 (fc の codegen は ldy のフラグで分岐しない)
//   - `cmp #0` の直後が beq / bne / bmi / bpl で、フラグが既に A を反映していれば cmp を削除
//     (bcc / bcs は cmp の C を見るので消せない)
//   - `sta X` は A を変えないので X を集合に足す
//   - A を書く命令 (adc / and / lda / pla / txa …) で集合を空にする
//   - メモリを書く命令は書いた先を集合から外す (Y が指す場所なら Y も忘れる)。間接・インデックス付きの書き込みと
//     jsr / call はどこを書いたか分からないので全部外す (Y が即値ならそのまま)
//   - ラベル (合流点) と分岐命令の後は空にする
//
// 対象は fc が生成する形だけ (オペランドは文字列として同じかどうかで比べる。`0+<L+0` と `<L+0` は別扱い)。
// 副作用が無いことを確認済みの命令だけ扱い、知らない命令は全部を空にする。
// 追跡するのはゼロページの局所 (レジスタ領域 / フレーム / fastcall 領域 / reg) と即値だけ。グローバル変数は
// options(address:) の I/O レジスタ ($2002 など。読むたびに値が変わる) と区別できないので追跡しない。

import (
	"strings"
)

// aState は「A の値と等しいことが分かっている場所」の集合。
type aState map[string]bool

type peepState struct {
	a          aState
	flagsFromA bool   // N/Z が A の値を反映しているか
	y          string // Y の値と等しいことが分かっている場所 / 即値 ("" なら不明)
}

func (s *peepState) reset() {
	s.a = aState{}
	s.flagsFromA = false
	s.y = ""
}

// operandKind はオペランドの種類。
//   - opIndirect: `(p),y` / `(S+n,x)`。どこを書くか分からない (ポインタ経由でゼロページの変数を書くこともある)
//   - opGlobalIndexed: `sym+0,y` / `sym,x`。グローバル配列への参照。ゼロページの局所とは重ならない
//   - opLocal: `<L+n` / `0+<S+n,x` / `<reg+0` / `<FC_FASTCALL_REG+n`。ゼロページの局所 (フレームは X が関数内で一定なので
//     文字列で同一性が決まる。X を変える命令で状態を捨てる)
//   - opImmediate: `#n`
//   - opOther: グローバル変数など (I/O レジスタと区別できないので追跡しない)
type operandKind uint8

const (
	opOther operandKind = iota
	opIndirect
	opGlobalIndexed
	opLocal
	opImmediate
)

func classify(arg string) operandKind {
	switch {
	case strings.HasPrefix(arg, "("):
		return opIndirect
	case strings.HasPrefix(arg, "#"):
		return opImmediate
	case strings.Contains(arg, "<"):
		return opLocal
	case strings.Contains(arg, ","):
		return opGlobalIndexed
	}
	return opOther
}

// wrote はメモリ位置 arg への書き込み (A は変わらない)。
func (s *peepState) wrote(arg string, kind operandKind) {
	switch kind {
	case opIndirect:
		s.a = aState{}
		if !strings.HasPrefix(s.y, "#") {
			s.y = ""
		}
	case opGlobalIndexed, opOther:
		// グローバルは追跡していないので影響なし
	default:
		delete(s.a, arg)
		if s.y == arg {
			s.y = ""
		}
	}
}

func peepholeA(lines []string) []string {
	out := make([]string, 0, len(lines))
	s := &peepState{}
	s.reset()
	next := func(i int) string {
		for j := i + 1; j < len(lines); j++ {
			t := strings.TrimSpace(lines[j])
			if t != "" && !strings.HasPrefix(t, ";") {
				return t
			}
		}
		return ""
	}
	for i, line := range lines {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, ";") {
			out = append(out, line)
			continue
		}
		if strings.HasSuffix(t, ":") || strings.HasPrefix(t, ".") {
			// ラベル・ディレクティブ: 合流点なので何も分からない
			s.reset()
			out = append(out, line)
			continue
		}
		mnem, arg := t, ""
		if k := strings.IndexAny(t, " \t"); k >= 0 {
			mnem, arg = t[:k], strings.TrimSpace(t[k+1:])
		}
		kind := classify(arg)
		trackable := kind == opLocal
		switch mnem {
		case "lda":
			if trackable && s.a[arg] && s.flagsFromA {
				continue // A は既にこの値で、フラグもそれを反映している
			}
			s.a = aState{}
			if trackable {
				s.a[arg] = true
			}
			s.flagsFromA = true
		case "ldy":
			if (trackable || kind == opImmediate) && s.y == arg {
				continue // Y は既にこの値
			}
			s.y = ""
			if trackable || kind == opImmediate {
				s.y = arg
			}
			s.flagsFromA = false
		case "cmp":
			if arg == "#0" && s.flagsFromA {
				if f := strings.Fields(next(i)); len(f) > 0 {
					switch f[0] {
					case "beq", "bne", "bmi", "bpl":
						continue // N/Z は A そのもの (C は見ない)
					}
				}
			}
			s.flagsFromA = false
		case "sta":
			if trackable {
				s.a[arg] = true
				if s.y == arg {
					s.y = ""
				}
			} else {
				s.wrote(arg, kind)
			}
		case "stx":
			s.wrote(arg, kind)
		case "sty":
			// Y の値を書く。Y == arg になるが、Y が即値ならそのままの方が使いやすい
			s.wrote(arg, kind)
			if trackable && s.y == "" {
				s.y = arg
			}
		case "inc", "dec", "asl", "lsr", "rol", "ror":
			if arg == "a" {
				s.a = aState{}
				s.flagsFromA = true
			} else {
				s.wrote(arg, kind)
				s.flagsFromA = false
			}
		case "clc", "sec", "cli", "sei", "cld", "nop", "txs":
			// A も Y もメモリもフラグ (N/Z) も変えない
		case "cpx", "cpy", "bit":
			// A も Y もメモリも変えないが N/Z は別の値になる
			s.flagsFromA = false
		case "ldx", "inx", "dex", "tax":
			// X が変わるとフレーム (`<S+n,x`) の指す先が変わるので、A / Y の追跡は捨てる
			s.a = aState{}
			s.y = ""
			s.flagsFromA = mnem == "tax" // tax は A の値を写すので N/Z は A を反映する
		case "iny", "dey":
			s.y = ""
			s.flagsFromA = false
		case "tay":
			s.y = ""
			s.flagsFromA = true // A の値を写すので N/Z は A を反映する
		case "bcc", "bcs", "beq", "bne", "bmi", "bpl", "bvc", "bvs":
			// 分岐: 落ちてくる側では状態はそのまま (飛び先はラベルで空になる)
		default:
			// A を書く (adc / sbc / and / ora / eor / pla / txa / tya ...)、
			// jmp / jsr / rts / call マクロ / farcall など: 何も分からない。
			// A を書く命令は N/Z を A から立てるが、jsr 等は分からないので安全側 (false)
			s.reset()
		}
		out = append(out, line)
	}
	return out
}
