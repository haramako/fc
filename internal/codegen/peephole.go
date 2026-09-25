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
//   - 検査のためだけのロード (codegen が testMark を付けた `lda x` / `ldy x`) は、N/Z が既に x の値を映していて
//     (直前が `inc x` / `dec x` で、間にフラグを変える命令もラベルも無い) 直後が beq / bne / bmi / bpl なら削除
//
// 対象は fc が生成する形だけ (オペランドは文字列として同じかどうかで比べる。`0+<L+0` と `<L+0` は別扱い)。
// 副作用が無いことを確認済みの命令だけ扱い、知らない命令は全部を空にする。
// 追跡するのはゼロページの局所 (レジスタ領域 / フレーム / fastcall 領域 / reg) と即値だけ。グローバル変数は
// options(address:) の I/O レジスタ ($2002 など。読むたびに値が変わる) と区別できないので追跡しない。

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// aState は「A の値と等しいことが分かっている場所」の集合。
type aState map[string]bool

type peepState struct {
	a          aState
	flagsFromA bool   // N/Z が A の値を反映しているか
	flagsFromY bool   // N/Z が Y の値を反映しているか (ldy / iny / dey / tay の直後)
	flagsFromX bool   // N/Z が X の値を反映しているか (ldx / inx / dex / tax の直後)
	y          string // Y の値と等しいことが分かっている場所 / 即値 ("" なら不明)
	flagsMem   string // N/Z がこのメモリ位置の値を映している (inc / dec の直後。"" なら不明)
}

// canonAddr はオペランドの表記を正規化する: `k+<L+n` (byte の表記) と `<L+(n+k)` は同じ場所。別の綴りの同じ番地への
// 書き込みで追跡が無効にならず (`sty 0+<F+6` の後の `sta 1+<F+5`)、必要な `ldy 0+<F+6` を消していた (fuzz で発覚)。
func canonAddr(arg string) string {
	m := addrOffsetRe.FindStringSubmatch(arg)
	if m == nil {
		return strings.TrimPrefix(arg, "0+")
	}
	k := 0
	if m[1] != "" {
		k, _ = strconv.Atoi(m[1])
	}
	n := 0
	if m[4] != "" {
		n, _ = strconv.Atoi(m[4])
	}
	if k+n == 0 {
		return m[2] + m[3]
	}
	return fmt.Sprintf("%s%s+%d", m[2], m[3], k+n)
}

// addrOffsetRe: `k+<SYM+n` / `<SYM+n` / `SYM` (k, n は 10 進。`+0` は落とす)。
var addrOffsetRe = regexp.MustCompile(`^(?:([0-9]+)\+)?(<?)([A-Za-z_][A-Za-z0-9_]*)(?:\+([0-9]+))?$`)

func (s *peepState) reset() {
	s.a = aState{}
	s.flagsFromA = false
	s.flagsFromY = false
	s.flagsFromX = false
	s.y = ""
	s.flagsMem = ""
}

// keepsFlagsMem は命令が flagsMem (N/Z がそのメモリ位置の値を映していること) を保つか。
// フラグを変えない命令のうち、その位置を書かないと分かるものだけ (インデックス付き・間接の書き込みはどこを書くか分からない)。
func (s *peepState) keepsFlagsMem(mnem, arg string, kind operandKind) bool {
	switch mnem {
	case "bcc", "bcs", "beq", "bne", "bmi", "bpl", "bvc", "bvs", "clc", "sec", "cli", "sei", "cld", "nop":
		return true
	case "sta", "stx", "sty":
		return (kind == opLocal || kind == opOther) && !strings.Contains(arg, ",") && arg != s.flagsMem
	}
	return false
}

// branchOnNZ は行が N / Z だけを見る分岐か。
func branchOnNZ(line string) bool {
	f := strings.Fields(line)
	if len(f) == 0 {
		return false
	}
	switch f[0] {
	case "beq", "bne", "bmi", "bpl":
		return true
	}
	return false
}

// setsNZ は命令 (行) が N/Z を A 以外の (自分の) 結果で立てるか。直前の lda のフラグが要らないことの判定に使う
// (分岐・sta・clc などは N/Z を保つので、その前の lda は消せない)。
func setsNZ(line string) bool {
	m := line
	if k := strings.IndexAny(m, " \t"); k >= 0 {
		m = m[:k]
	}
	switch m {
	case "adc", "sbc", "and", "ora", "eor", "cmp", "cpx", "cpy", "asl", "lsr", "rol", "ror", "inc", "dec",
		"inx", "iny", "dex", "dey", "ldx", "ldy", "lda", "tax", "tay", "txa", "tya", "pla":
		return true
	}
	return false
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
	case strings.Contains(arg, "<"), strings.HasPrefix(strings.TrimLeft(arg, "0123456789+"), "F_"):
		return opLocal // ゼロページの局所 / RAM 上の静的フレーム (I/O レジスタではないので追跡してよい)
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
			if t != "" && !strings.HasPrefix(t, ";") && !strings.HasPrefix(t, ".dbg") {
				return t
			}
		}
		return ""
	}
	for i, line := range lines {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, ";") || strings.HasPrefix(t, ".dbg") {
			out = append(out, line) // .dbg (fcc build -g のソース位置) は命令の並びに影響しない
			continue
		}
		if strings.HasSuffix(t, ":") || strings.HasPrefix(t, ".") {
			// ラベル・ディレクティブ: 合流点なので何も分からない
			s.reset()
			out = append(out, line)
			continue
		}
		isTest := strings.HasSuffix(t, testMark)
		t = strings.TrimSuffix(t, testMark)
		mnem, arg := t, ""
		if k := strings.IndexAny(t, " \t"); k >= 0 {
			mnem, arg = t[:k], strings.TrimSpace(t[k+1:])
		}
		arg = canonAddr(arg)
		kind := classify(arg)
		trackable := kind == opLocal
		if isTest && s.flagsMem != "" && arg == s.flagsMem && branchOnNZ(next(i)) {
			continue // N/Z は既に arg の値 (`dec x; lda x; bne` の lda)。A / Y は変えないまま
		}
		setFlagsMem := "" // この命令の後で N/Z が映すメモリ位置 (下の switch の後で入れる)
		if (mnem == "inc" || mnem == "dec") && arg != "a" && (kind == opLocal || kind == opOther) && !strings.Contains(arg, ",") {
			setFlagsMem = arg
		} else if !s.keepsFlagsMem(mnem, arg, kind) {
			s.flagsMem = ""
		}
		switch mnem {
		case "ldy", "iny", "dey", "tay", "sta", "sty", "stx", "clc", "sec", "cli", "sei", "cld", "nop", "txs",
			"bcc", "bcs", "beq", "bne", "bmi", "bpl", "bvc", "bvs":
			// Y のフラグを保つ (ldy / iny / dey / tay は下で立てる)
		case "cpy":
			if arg == "#0" && s.flagsFromY && branchOnNZ(next(i)) {
				continue // N/Z は Y そのもの (`iny; cpy #0; bne` の cpy。以前は下の case に届く前にここで flagsFromY を落としていた)
			}
			s.flagsFromY = false
		default:
			s.flagsFromY = false // フラグを別の値で立てる命令
		}
		switch mnem {
		case "ldx", "inx", "dex", "tax", "sta", "sty", "stx", "clc", "sec", "cli", "sei", "cld", "nop", "txs",
			"bcc", "bcs", "beq", "bne", "bmi", "bpl", "bvc", "bvs":
			// X のフラグを保つ (ldx / inx / dex / tax は下で立てる)
		case "cpx":
			if arg == "#0" && s.flagsFromX && branchOnNZ(next(i)) {
				continue // N/Z は X そのもの (`dex; cpx #0; bne` の cpx)
			}
			s.flagsFromX = false
		default:
			s.flagsFromX = false
		}
		switch mnem {
		case "lda":
			if (trackable || kind == opImmediate) && s.a[arg] && (s.flagsFromA || setsNZ(next(i))) {
				// A は既にこの値で、フラグもそれを反映している (または次の命令がフラグを別の値で立て直すので要らない。
				// ラベルの直後の `sta x; sec; lda x; sbc #32` など)
				continue
			}
			s.a = aState{}
			if trackable || kind == opImmediate {
				s.a[arg] = true // 即値は変わらないので無効化は要らない
			}
			s.flagsFromA = true
		case "ldy":
			if (trackable || kind == opImmediate) && s.y == arg {
				// Y は既にこの値。直後が分岐 (ldy x; bne) ならフラグも Y を反映していることが要る
				f := strings.Fields(next(i))
				if s.flagsFromY || len(f) == 0 || !strings.HasPrefix(f[0], "b") {
					continue
				}
			}
			s.y = ""
			if trackable || kind == opImmediate {
				s.y = arg
			}
			if (trackable || kind == opImmediate) && s.a[arg] {
				// A が既にこの値 (sta x; ldy x): tay (3 → 2 サイクル)。A は変わらずフラグは A = Y を反映する
				line = strings.Replace(line, t, "tay", 1)
				s.flagsFromY = true
				break
			}
			s.flagsFromA = false
			s.flagsFromY = true
		case "cpy":
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
		case "cpx", "bit":
			// A も Y もメモリも変えないが N/Z は別の値になる
			s.flagsFromA = false
		case "ldx", "inx", "dex", "tax":
			if mnem == "ldx" && (trackable || kind == opImmediate) && s.a[arg] {
				// A が既にこの値 (sta x; ldx x): tax (3 → 2 サイクル)
				line = strings.Replace(line, t, "tax", 1)
				mnem = "tax"
			}
			// X が変わるとフレーム (`<S+n,x`) の指す先が変わるので、",x" を含む追跡は捨てる
			for k := range s.a {
				if strings.Contains(k, ",x") {
					delete(s.a, k)
				}
			}
			if strings.Contains(s.y, ",x") {
				s.y = ""
			}
			s.flagsFromA = mnem == "tax" // tax は A の値を写すので N/Z は A を反映する
			s.flagsFromX = true
		case "iny", "dey":
			s.y = ""
			s.flagsFromA = false
			s.flagsFromY = true
		case "tay":
			s.y = ""
			s.flagsFromA = true // A の値を写すので N/Z は A を反映する
			s.flagsFromY = true
		case "bcc", "bcs", "beq", "bne", "bmi", "bpl", "bvc", "bvs":
			// 分岐: 落ちてくる側では状態はそのまま (飛び先はラベルで空になる)
		case "adc", "sbc", "and", "ora", "eor", "txa", "tya", "pla":
			// A を書き、N/Z は A から立つ
			s.a = aState{}
			s.flagsFromA = true
		default:
			// A を書く (adc / sbc / and / ora / eor / pla / txa / tya ...)、
			// jmp / jsr / rts / call マクロ / farcall など: 何も分からない。
			// A を書く命令は N/Z を A から立てるが、jsr 等は分からないので安全側 (false)
			s.reset()
		}
		if setFlagsMem != "" {
			s.flagsMem = setFlagsMem
		}
		out = append(out, line)
	}
	return out
}
