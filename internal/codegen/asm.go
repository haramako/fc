package codegen

// 生成したアセンブリの 1 行の構造化: ニーモニック + アドレッシングモード + 記号のオペランド (base + 定数のずれ)。
//
// codegen は命令を文字列として組み立てる (llc.go / operand.go / arith.go)。後処理 (ピープホール peephole.go、レジスタの
// 書き込みの検査 verify.go、分岐の延長 extendjump.go、@log の地点 log.go) は以前それぞれが文字列を切って判断していて、
// ニーモニックの性質の表も 4 つあった。ここで 1 度だけ解析して asmLine にし、後処理はその上で書く。
//
// 番地の同一性は (Base, Off, 添字レジスタ) で決まる: `1+<F+5` と `0+<F+6`、`<1+S+3,x` と `<S+4,x`、`<FC_FASTCALL_REG+2` と
// `<2+FC_FASTCALL_REG` は同じ場所。以前は綴りの正規表現 (canonAddr) で一部の形だけを揃えていて、揃わない形で必要な
// `ldy` を消すバグが出た (TestPeepholeAddressSpelling)。

import (
	"fmt"
	"strconv"
	"strings"
)

// lineKind は行の種類。
type lineKind uint8

const (
	lkBlank     lineKind = iota
	lkComment            // `; ...`
	lkLabel              // `name:`
	lkDirective          // `.byte` / `.dbg` / `.proc` / `sym = expr` など
	lkInstr              // ニーモニック [オペランド] (ca65 のマクロも含む)
)

// addrMode はアドレッシングモード (オペランドの形)。
type addrMode uint8

const (
	amNone  addrMode = iota // implied / accumulator (オペランド無し、または `a`)
	amImm                   // `#expr`
	amMem                   // `expr` (ゼロページか絶対。分岐のラベルも)
	amMemX                  // `expr,x`
	amMemY                  // `expr,y`
	amIndY                  // `(expr),y`
	amIndX                  // `(expr,x)`
	amInd                   // `(expr)` (jmp)
	amOther                 // 解析できない形 (`.LOBYTE(S+3)`、`<(L-1)` など。Raw だけ)
)

// operand はオペランド。Base は記号 (無ければ "")、Off は定数の和、Lo は `<` (ゼロページ / 下位バイト) が付いているか。
type operand struct {
	Mode addrMode
	Base string
	Off  int
	Lo   bool
	Raw  string // 元の綴り (出力はこれをそのまま使う)
}

// key は場所の同一性の鍵 (Mode / Base / Off / 添字)。amImm と amOther は綴りそのもの。
func (o operand) key() string {
	switch o.Mode {
	case amNone:
		return ""
	case amImm, amOther:
		return o.Raw
	}
	var b strings.Builder
	if o.Mode == amIndY || o.Mode == amIndX || o.Mode == amInd {
		b.WriteByte('(')
	}
	b.WriteString(o.Base)
	if o.Off != 0 || o.Base == "" {
		fmt.Fprintf(&b, "+%d", o.Off)
	}
	switch o.Mode {
	case amMemX:
		b.WriteString(",x")
	case amMemY:
		b.WriteString(",y")
	case amIndY:
		b.WriteString("),y")
	case amIndX:
		b.WriteString(",x)")
	case amInd:
		b.WriteByte(')')
	}
	return b.String()
}

// indexed は添字レジスタ付きか (X / Y が変わると別の場所を指す)。
func (o operand) indexedX() bool { return o.Mode == amMemX || o.Mode == amIndX }

// asmLine は解析した 1 行。
type asmLine struct {
	Kind  lineKind
	Mnem  string  // lkInstr: ニーモニック (小文字)
	Arg   operand // lkInstr
	Test  bool    // lkInstr: 検査のためだけのロードの印 (testMark)
	Label string  // lkLabel: 名前
	Text  string  // 元の行 (インデント・コメントを含む)。出力はこれ
}

// parseAsmLine は 1 行を解析する。
func parseAsmLine(line string) asmLine {
	t := strings.TrimSpace(line)
	a := asmLine{Text: line}
	switch {
	case t == "":
		a.Kind = lkBlank
		return a
	case strings.HasPrefix(t, ";"):
		a.Kind = lkComment
		return a
	case strings.HasPrefix(t, "."):
		a.Kind = lkDirective
		return a
	}
	if name, ok := strings.CutSuffix(t, ":"); ok && !strings.ContainsAny(name, " \t") {
		a.Kind = lkLabel
		a.Label = name
		return a
	}
	if strings.Contains(t, " = ") || strings.Contains(t, ":=") {
		a.Kind = lkDirective // `sym = expr`
		return a
	}
	a.Kind = lkInstr
	if strings.HasSuffix(t, testMark) {
		a.Test = true
		t = strings.TrimSuffix(t, testMark)
	}
	if k := strings.IndexByte(t, ';'); k >= 0 {
		t = strings.TrimSpace(t[:k])
	}
	mnem, arg := t, ""
	if k := strings.IndexAny(t, " \t"); k >= 0 {
		mnem, arg = t[:k], strings.TrimSpace(t[k+1:])
	}
	a.Mnem = strings.ToLower(mnem)
	a.Arg = parseOperand(arg)
	return a
}

// parseOperand はオペランドの綴りを解析する。
func parseOperand(arg string) operand {
	o := operand{Raw: arg}
	switch {
	case arg == "" || arg == "a" || arg == "A":
		o.Mode = amNone
		return o
	case strings.HasPrefix(arg, "#"):
		o.Mode = amImm
		if base, off, lo, ok := parseExpr(arg[1:]); ok {
			o.Base, o.Off, o.Lo = base, off, lo
		}
		return o
	}
	expr := arg
	switch {
	case strings.HasPrefix(arg, "(") && strings.HasSuffix(arg, "),y"):
		o.Mode, expr = amIndY, arg[1:len(arg)-3]
	case strings.HasPrefix(arg, "(") && strings.HasSuffix(arg, ",x)"):
		o.Mode, expr = amIndX, arg[1:len(arg)-3]
	case strings.HasPrefix(arg, "(") && strings.HasSuffix(arg, ")"):
		o.Mode, expr = amInd, arg[1:len(arg)-1]
	case strings.HasSuffix(arg, ",x"):
		o.Mode, expr = amMemX, arg[:len(arg)-2]
	case strings.HasSuffix(arg, ",y"):
		o.Mode, expr = amMemY, arg[:len(arg)-2]
	default:
		o.Mode = amMem
	}
	base, off, lo, ok := parseExpr(expr)
	if !ok {
		return operand{Mode: amOther, Raw: arg}
	}
	o.Base, o.Off, o.Lo = base, off, lo
	return o
}

// parseExpr は `k+<SYM+n` のような、記号 1 つ以下と整数の和 (各項に `<` が付いてもよい) を解析する。
// 記号が 2 つ以上、減算、括弧、関数呼び出しは解析しない (ok = false)。
func parseExpr(expr string) (base string, off int, lo bool, ok bool) {
	if expr == "" {
		return "", 0, false, false
	}
	for _, term := range strings.Split(expr, "+") {
		term = strings.TrimSpace(term)
		if strings.HasPrefix(term, "<") {
			lo = true
			term = term[1:]
		}
		if term == "" {
			return "", 0, false, false
		}
		if n, err := strconv.Atoi(term); err == nil {
			off += n
			continue
		}
		if strings.HasPrefix(term, "$") {
			n, err := strconv.ParseInt(term[1:], 16, 32)
			if err != nil {
				return "", 0, false, false
			}
			off += int(n)
			continue
		}
		if !isAsmIdent(term) || base != "" {
			return "", 0, false, false
		}
		base = term
	}
	return base, off, lo, true
}

// isAsmIdent は ca65 の識別子 (ラベルの `@` / `.` の接頭辞も) か。
func isAsmIdent(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '_' || c == '$' || 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z':
		case '0' <= c && c <= '9' && i > 0:
		case (c == '@' || c == '.') && i == 0:
		default:
			return false
		}
	}
	return s != ""
}

// withInstr は Kind / Text を同じ字下げで命令 mnem [arg] に置き換えた行。
func (a asmLine) withInstr(mnem, arg string) asmLine {
	indent := a.Text[:len(a.Text)-len(strings.TrimLeft(a.Text, " \t"))]
	t := mnem
	if arg != "" {
		t += " " + arg
	}
	return asmLine{Kind: lkInstr, Mnem: mnem, Arg: parseOperand(arg), Text: indent + t}
}

// ---------------------------------------------------------------
// ニーモニックの性質
// ---------------------------------------------------------------

// mnemInfo は 6502 の命令 1 つの性質。
type mnemInfo struct {
	wA, wX, wY bool   // 書くレジスタ (asl / lsr / rol / ror の A は accumulator モードのときだけ)
	nz         bool   // N / Z を自分の結果で立てる (accumulator / レジスタ / メモリのどれでも)
	keepsNZ    bool   // N / Z を変えない (sta / 分岐 / clc など)
	rel        bool   // 相対分岐
	implied    bool   // オペランド無し (1 バイト)
	inverse    string // rel: 条件を反転した分岐
}

var mnemTable = map[string]mnemInfo{
	"lda": {wA: true, nz: true}, "ldx": {wX: true, nz: true}, "ldy": {wY: true, nz: true},
	"sta": {keepsNZ: true}, "stx": {keepsNZ: true}, "sty": {keepsNZ: true},
	"adc": {wA: true, nz: true}, "sbc": {wA: true, nz: true}, "and": {wA: true, nz: true}, "ora": {wA: true, nz: true}, "eor": {wA: true, nz: true},
	"cmp": {nz: true}, "cpx": {nz: true}, "cpy": {nz: true}, "bit": {nz: true},
	"asl": {nz: true}, "lsr": {nz: true}, "rol": {nz: true}, "ror": {nz: true}, // A は accumulator モードのとき (mnemWrites)
	"inc": {nz: true}, "dec": {nz: true},
	"inx": {wX: true, nz: true, implied: true}, "dex": {wX: true, nz: true, implied: true},
	"iny": {wY: true, nz: true, implied: true}, "dey": {wY: true, nz: true, implied: true},
	"tax": {wX: true, nz: true, implied: true}, "tay": {wY: true, nz: true, implied: true},
	"txa": {wA: true, nz: true, implied: true}, "tya": {wA: true, nz: true, implied: true},
	"tsx": {wX: true, nz: true, implied: true}, "txs": {keepsNZ: true, implied: true},
	"pha": {keepsNZ: true, implied: true}, "php": {keepsNZ: true, implied: true},
	"pla": {wA: true, nz: true, implied: true}, "plp": {implied: true},
	"clc": {keepsNZ: true, implied: true}, "sec": {keepsNZ: true, implied: true}, "cli": {keepsNZ: true, implied: true},
	"sei": {keepsNZ: true, implied: true}, "cld": {keepsNZ: true, implied: true}, "sed": {keepsNZ: true, implied: true},
	"clv": {keepsNZ: true, implied: true}, "nop": {keepsNZ: true, implied: true},
	"brk": {implied: true}, "rti": {implied: true}, "rts": {keepsNZ: true, implied: true},
	"jmp": {keepsNZ: true}, "jsr": {},
	"bcc": {keepsNZ: true, rel: true, inverse: "bcs"}, "bcs": {keepsNZ: true, rel: true, inverse: "bcc"},
	"beq": {keepsNZ: true, rel: true, inverse: "bne"}, "bne": {keepsNZ: true, rel: true, inverse: "beq"},
	"bmi": {keepsNZ: true, rel: true, inverse: "bpl"}, "bpl": {keepsNZ: true, rel: true, inverse: "bmi"},
	"bvc": {keepsNZ: true, rel: true, inverse: "bvs"}, "bvs": {keepsNZ: true, rel: true, inverse: "bvc"},
}

// isBranch は相対分岐か。
func (a asmLine) isBranch() bool { return a.Kind == lkInstr && mnemTable[a.Mnem].rel }

// isJump は jmp か相対分岐か (ラベルへ飛ぶ)。
func (a asmLine) isJump() bool { return a.Kind == lkInstr && (a.Mnem == "jmp" || mnemTable[a.Mnem].rel) }

// branchOnNZ は N / Z だけを見る分岐か (beq / bne / bmi / bpl。bcc / bcs は C を見る)。
func (a asmLine) branchOnNZ() bool {
	switch a.Mnem {
	case "beq", "bne", "bmi", "bpl":
		return a.Kind == lkInstr
	}
	return false
}

// setsNZ は命令が N / Z を自分の結果で立てるか (知らない命令 (マクロ) は立てないと見る: 直前の lda のフラグが要るかの
// 判定に使うので、安全側)。
func (a asmLine) setsNZ() bool { return a.Kind == lkInstr && mnemTable[a.Mnem].nz }

// keepsNZ は命令が N / Z を変えないか (知らない命令は変えると見る)。
func (a asmLine) keepsNZ() bool { return a.Kind == lkInstr && mnemTable[a.Mnem].keepsNZ }

// writes は命令が書くレジスタ。jsr と知らない命令 (マクロ) は全部を書くと見る。ただし share/runtime.asm の乗除算は
// A と Y を使い X は保つ (__div_16 は退避して戻す)。
func (a asmLine) writes() (wa, wx, wy bool) {
	if a.Kind != lkInstr {
		return
	}
	m, ok := mnemTable[a.Mnem]
	if !ok || a.Mnem == "jsr" {
		if a.Mnem == "jsr" {
			for _, p := range []string{"__mul_", "__div_", "__mod_"} {
				if strings.HasPrefix(a.Arg.Raw, p) {
					return true, false, true
				}
			}
		}
		return true, true, true
	}
	switch a.Mnem {
	case "asl", "lsr", "rol", "ror":
		return a.Arg.Mode == amNone, false, false
	}
	return m.wA, m.wX, m.wY
}

// size は命令のサイズ (バイト)。分岐は 2 (延長すれば 5: extendJump が決める)。call マクロは 11
// (txa pha clc adc# tax jsr pla tax)、知らない命令 (インラインアセンブラのマクロなど) は 10 の安全側。
func (a asmLine) size() int {
	switch a.Kind {
	case lkInstr:
	case lkDirective:
		t := strings.TrimSpace(a.Text)
		switch {
		case strings.HasPrefix(t, ".byte "):
			return strings.Count(t, ",") + 1
		case strings.HasPrefix(t, ".word "):
			return 2 * (strings.Count(t, ",") + 1)
		}
		return 0
	default:
		return 0
	}
	m, ok := mnemTable[a.Mnem]
	switch {
	case !ok:
		if a.Mnem == "call" {
			return 11
		}
		return 10
	case m.rel:
		return 2
	case m.implied || a.Arg.Mode == amNone:
		return 1
	case a.Mnem == "jmp" || a.Mnem == "jsr":
		return 3
	case a.Arg.Mode == amImm, a.Arg.Mode == amIndY, a.Arg.Mode == amIndX, a.Arg.Lo:
		return 2 // 即値 / 間接 / ゼロページ
	case a.Arg.Mode == amOther && strings.Contains(a.Arg.Raw, "<"):
		return 2
	}
	return 3 // 絶対 (ゼロページのシンボルなら ca65 が 2 にするが、大きく見積もる分には安全)
}
