package codegen

// アセンブリ行のピープホール: A / Y レジスタに何が入っているかを追跡して、不要な lda / ldy / cmp #0 を消す。
//
// 命令列を上から順に見て「A の値 = このメモリ位置 / この即値」の集合と「Y の値 = この場所 / 即値」を持ち、
//
//   - `lda X` で X が集合にあり、かつ N/Z フラグが今の A の値を反映しているか、その後で N/Z が読まれる前に立て直されるなら削除
//     (lda は N/Z を立てるので、間に ldy / cmp などフラグを別の値で上書きする命令があって、後で N/Z を見るなら消せない)
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
// 対象は fc が生成する形だけ。場所の同一性は asm.go の operand.key() (記号 + 定数のずれ + 添字) で見る。
// 副作用が無いことを確認済みの命令だけ扱い、知らない命令は全部を空にする。
// 追跡するのはゼロページの局所 (レジスタ領域 / フレーム / fastcall 領域 / reg) と静的フレーム、即値だけ。グローバル変数は
// options(address:) の I/O レジスタ ($2002 など。読むたびに値が変わる) と区別できないので追跡しない。

import "strings"

// aState は「A の値と等しいことが分かっている場所」の集合 (operand.key())。
type aState map[string]bool

type peepState struct {
	a          aState
	flagsFromA bool   // N/Z が A の値を反映しているか
	flagsFromY bool   // N/Z が Y の値を反映しているか (ldy / iny / dey / tay の直後)
	flagsFromX bool   // N/Z が X の値を反映しているか (ldx / inx / dex / tax の直後)
	y          string // Y の値と等しいことが分かっている場所 / 即値 ("" なら不明)
	flagsMem   string // N/Z がこのメモリ位置の値を映している (inc / dec の直後。"" なら不明)
}

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
func (s *peepState) keepsFlagsMem(a asmLine, key string, kind operandKind) bool {
	switch a.Mnem {
	case "sta", "stx", "sty":
		return (kind == opLocal || kind == opOther) && a.Arg.Mode == amMem && key != s.flagsMem
	}
	return a.keepsNZ()
}

// operandKind はオペランドの種類。
//   - opIndirect: `(p),y` / `(S+n,x)`。どこを書くか分からない (ポインタ経由でゼロページの変数を書くこともある)
//   - opGlobalIndexed: `sym+0,y` / `sym,x`。グローバル配列への参照。ゼロページの局所とは重ならない
//   - opLocal: `<L+n` / `0+<S+n,x` / `<reg+0` / `<FC_FASTCALL_REG+n` / `F_f+n`。ゼロページの局所と静的フレーム (フレームは X が
//     関数内で一定なので (記号, ずれ, 添字) で同一性が決まる。X を変える命令で状態を捨てる)
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

func classify(o operand) operandKind {
	switch o.Mode {
	case amIndY, amIndX, amInd:
		return opIndirect
	case amImm:
		return opImmediate
	case amOther:
		return opOther
	}
	if o.Lo || strings.HasPrefix(o.Base, "F_") {
		return opLocal // ゼロページの局所 / RAM 上の静的フレーム (I/O レジスタではないので追跡してよい)
	}
	if o.Mode == amMemX || o.Mode == amMemY {
		return opGlobalIndexed
	}
	return opOther
}

// wrote はメモリ位置 key への書き込み (A は変わらない)。
func (s *peepState) wrote(key string, kind operandKind) {
	switch kind {
	case opIndirect:
		s.a = aState{}
		if !strings.HasPrefix(s.y, "#") {
			s.y = ""
		}
	case opGlobalIndexed, opOther:
		// グローバルは追跡していないので影響なし
	default:
		delete(s.a, key)
		if s.y == key {
			s.y = ""
		}
	}
}

// forgetX は X が変わったとき: フレーム (`<S+n,x`) の指す先が変わるので、X の添字付きの追跡は捨てる。
func (s *peepState) forgetX() {
	for k := range s.a {
		if strings.Contains(k, ",x") {
			delete(s.a, k)
		}
	}
	if strings.Contains(s.y, ",x") {
		s.y = ""
	}
}

func peepholeA(lines []string) []string {
	out := make([]string, 0, len(lines))
	parsed := make([]asmLine, len(lines))
	for i, line := range lines {
		parsed[i] = parseAsmLine(line)
	}
	skip := func(a asmLine) bool { // 命令の並びに影響しない行 (.dbg は fcc build -g のソース位置)
		return a.Kind == lkBlank || a.Kind == lkComment || (a.Kind == lkDirective && strings.HasPrefix(strings.TrimSpace(a.Text), ".dbg"))
	}
	next := func(i int) asmLine {
		for j := i + 1; j < len(parsed); j++ {
			if !skip(parsed[j]) {
				return parsed[j]
			}
		}
		return asmLine{}
	}
	// nzDead は i の命令の後の N/Z が読まれないか: 先の命令を (ラベルの合流も越えて) 順に見て、N/Z を読む前に立て直す命令が
	// 来れば読まれない。N/Z の分岐・php と、どこへ行くか分からない命令 (分岐・jmp・jsr・rts・知らない命令・ディレクティブ) は読む
	// ものとみなす
	nzDead := func(i int) bool {
		for j := i + 1; j < len(parsed); j++ {
			b := parsed[j]
			switch {
			case skip(b) || b.Kind == lkLabel:
				continue
			case b.Kind != lkInstr || b.isBranch() || b.Mnem == "php":
				return false
			case b.setsNZ():
				return true
			case !b.keepsNZ():
				return false
			}
		}
		return false
	}
	s := &peepState{}
	s.reset()
	for i, a := range parsed {
		if skip(a) {
			out = append(out, a.Text)
			continue
		}
		if a.Kind != lkInstr {
			// ラベル・ディレクティブ: 合流点なので何も分からない
			s.reset()
			out = append(out, a.Text)
			continue
		}
		mnem := a.Mnem
		key := a.Arg.key()
		kind := classify(a.Arg)
		trackable := kind == opLocal
		if a.Test && s.flagsMem != "" && key == s.flagsMem && next(i).branchOnNZ() {
			continue // N/Z は既に arg の値 (`dec x; lda x; bne` の lda)。A / Y は変えないまま
		}
		setFlagsMem := "" // この命令の後で N/Z が映すメモリ位置 (下の switch の後で入れる)
		if (mnem == "inc" || mnem == "dec") && a.Arg.Mode == amMem && (kind == opLocal || kind == opOther) {
			setFlagsMem = key
		} else if !s.keepsFlagsMem(a, key, kind) {
			s.flagsMem = ""
		}
		switch mnem {
		case "ldy", "iny", "dey", "tay", "sta", "sty", "stx", "clc", "sec", "cli", "sei", "cld", "nop", "txs",
			"bcc", "bcs", "beq", "bne", "bmi", "bpl", "bvc", "bvs":
			// Y のフラグを保つ (ldy / iny / dey / tay は下で立てる)
		case "cpy":
			if key == "#0" && s.flagsFromY && next(i).branchOnNZ() {
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
			if key == "#0" && s.flagsFromX && next(i).branchOnNZ() {
				continue // N/Z は X そのもの (`dex; cpx #0; bne` の cpx)
			}
			s.flagsFromX = false
		default:
			s.flagsFromX = false
		}
		switch mnem {
		case "lda":
			if (trackable || kind == opImmediate) && s.a[key] && (s.flagsFromA || nzDead(i)) {
				// A は既にこの値で、フラグもそれを反映している (またはフラグが読まれる前に別の値で立て直すので要らない。
				// ラベルの直後の `sta x; sec; lda x; sbc #32`、上限で切る `lda #k; cmp x; bcs @l; lda #k; sta x; @l: lda y` など)
				continue
			}
			s.a = aState{}
			if trackable || kind == opImmediate {
				s.a[key] = true // 即値は変わらないので無効化は要らない
			}
			s.flagsFromA = true
		case "ldy":
			if (trackable || kind == opImmediate) && s.y == key {
				// Y は既にこの値。直後が分岐 (ldy x; bne) ならフラグも Y を反映していることが要る
				if s.flagsFromY || !next(i).isBranch() {
					continue
				}
			}
			s.y = ""
			if trackable || kind == opImmediate {
				s.y = key
			}
			if (trackable || kind == opImmediate) && s.a[key] {
				// A が既にこの値 (sta x; ldy x): tay (3 → 2 サイクル)。A は変わらずフラグは A = Y を反映する
				a = a.withInstr("tay", "")
				s.flagsFromY = true
				break
			}
			s.flagsFromA = false
			s.flagsFromY = true
		case "cpy":
			s.flagsFromA = false
		case "cmp":
			if key == "#0" && s.flagsFromA && next(i).branchOnNZ() {
				continue // N/Z は A そのもの (C は見ない)
			}
			s.flagsFromA = false
		case "sta":
			if trackable {
				s.a[key] = true
				if s.y == key {
					s.y = ""
				}
			} else {
				s.wrote(key, kind)
			}
		case "stx":
			s.wrote(key, kind)
		case "sty":
			// Y の値を書く。Y == arg になるが、Y が即値ならそのままの方が使いやすい
			s.wrote(key, kind)
			if trackable && s.y == "" {
				s.y = key
			}
		case "inc", "dec", "asl", "lsr", "rol", "ror":
			if a.Arg.Mode == amNone {
				s.a = aState{}
				s.flagsFromA = true
			} else {
				s.wrote(key, kind)
				s.flagsFromA = false
			}
		case "clc", "sec", "cli", "sei", "cld", "nop", "txs":
			// A も Y もメモリもフラグ (N/Z) も変えない
		case "cpx", "bit":
			// A も Y もメモリも変えないが N/Z は別の値になる
			s.flagsFromA = false
		case "ldx", "inx", "dex", "tax":
			if mnem == "ldx" && (trackable || kind == opImmediate) && s.a[key] {
				// A が既にこの値 (sta x; ldx x): tax (3 → 2 サイクル)
				a = a.withInstr("tax", "")
				mnem = "tax"
			}
			s.forgetX()
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
			// jmp / jsr / rts / call マクロ / farcall など: 何も分からない (jsr 等は N/Z も分からないので安全側の false)
			s.reset()
		}
		if setFlagsMem != "" {
			s.flagsMem = setFlagsMem
		}
		out = append(out, a.Text)
	}
	return out
}
