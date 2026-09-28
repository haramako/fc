// Package m6502 は 6502 の命令の性質の表 (書くレジスタ・フラグ・サイクル数)。
//
// codegen の後処理 (ピープホール・レジスタの書き込みの検査・分岐の延長) と、regalloc の常駐の見積もり (regalloc/forms.go。
// 命令列からサイクル数と壊すレジスタを計算する) が同じ表を引く。以前は codegen/asm.go の mnemTable にあり、見積もりの
// サイクル数 (3 / 6 / 1 …) は regalloc に手で書いていた。
package m6502

// Mode はアドレッシングモード (サイクル数の表の添字)。
type Mode uint8

const (
	Imp  Mode = iota // implied / accumulator (オペランド無し、または `a`)
	Imm              // `#expr`
	ZP               // ゼロページ
	ZPX              // ゼロページ,x
	ZPY              // ゼロページ,y
	Abs              // 絶対
	AbsX             // 絶対,x (ページをまたぐと読み出しは +1)
	AbsY             // 絶対,y (同上)
	IndX             // (zp,x)
	IndY             // (zp),y (ページをまたぐと読み出しは +1)
	Ind              // (abs) (jmp)
	Rel              // 相対分岐 (成立しないとき 2。成立 +1、ページをまたぐとさらに +1)
	numModes
)

// Info は命令 1 つの性質。
type Info struct {
	WA, WX, WY bool   // 書くレジスタ (asl / lsr / rol / ror の A は accumulator モードのときだけ: Writes)
	NZ         bool   // N / Z を自分の結果で立てる (accumulator / レジスタ / メモリのどれでも)
	KeepsNZ    bool   // N / Z を変えない (sta / 分岐 / clc など)
	Rel        bool   // 相対分岐
	Implied    bool   // オペランド無し (1 バイト)
	Inverse    string // Rel: 条件を反転した分岐
	cycles     [numModes]uint8
}

// Cycles はモード m のときのサイクル数 (ページまたぎと分岐の成立の加算は含まない)。そのモードが無ければ 0。
func (i Info) Cycles(m Mode) int { return int(i.cycles[m]) }

// cyc はモードとサイクル数の組を表にする。
func cyc(pairs ...int) [numModes]uint8 {
	var c [numModes]uint8
	for k := 0; k+1 < len(pairs); k += 2 {
		c[pairs[k]] = uint8(pairs[k+1])
	}
	return c
}

var (
	load   = cyc(int(Imm), 2, int(ZP), 3, int(ZPX), 4, int(ZPY), 4, int(Abs), 4, int(AbsX), 4, int(AbsY), 4, int(IndX), 6, int(IndY), 5)
	alu    = load
	store  = cyc(int(ZP), 3, int(ZPX), 4, int(ZPY), 4, int(Abs), 4, int(AbsX), 5, int(AbsY), 5, int(IndX), 6, int(IndY), 6)
	cmpXY  = cyc(int(Imm), 2, int(ZP), 3, int(Abs), 4)
	rmw    = cyc(int(Imp), 2, int(ZP), 5, int(ZPX), 6, int(Abs), 6, int(AbsX), 7) // asl / lsr / rol / ror (Imp = accumulator)
	incdec = cyc(int(ZP), 5, int(ZPX), 6, int(Abs), 6, int(AbsX), 7)
	imp2   = cyc(int(Imp), 2)
	branch = cyc(int(Rel), 2)
)

// Table はニーモニック (小文字) → 性質。ca65 のマクロ (call など) は載っていない (知らない命令として扱う)。
var Table = map[string]Info{
	"lda": {WA: true, NZ: true, cycles: load}, "ldx": {WX: true, NZ: true, cycles: load}, "ldy": {WY: true, NZ: true, cycles: load},
	"sta": {KeepsNZ: true, cycles: store}, "stx": {KeepsNZ: true, cycles: store}, "sty": {KeepsNZ: true, cycles: store},
	"adc": {WA: true, NZ: true, cycles: alu}, "sbc": {WA: true, NZ: true, cycles: alu}, "and": {WA: true, NZ: true, cycles: alu},
	"ora": {WA: true, NZ: true, cycles: alu}, "eor": {WA: true, NZ: true, cycles: alu},
	"cmp": {NZ: true, cycles: alu}, "cpx": {NZ: true, cycles: cmpXY}, "cpy": {NZ: true, cycles: cmpXY},
	"bit": {NZ: true, cycles: cyc(int(ZP), 3, int(Abs), 4)},
	"asl": {NZ: true, cycles: rmw}, "lsr": {NZ: true, cycles: rmw}, "rol": {NZ: true, cycles: rmw}, "ror": {NZ: true, cycles: rmw},
	"inc": {NZ: true, cycles: incdec}, "dec": {NZ: true, cycles: incdec},
	"inx": {WX: true, NZ: true, Implied: true, cycles: imp2}, "dex": {WX: true, NZ: true, Implied: true, cycles: imp2},
	"iny": {WY: true, NZ: true, Implied: true, cycles: imp2}, "dey": {WY: true, NZ: true, Implied: true, cycles: imp2},
	"tax": {WX: true, NZ: true, Implied: true, cycles: imp2}, "tay": {WY: true, NZ: true, Implied: true, cycles: imp2},
	"txa": {WA: true, NZ: true, Implied: true, cycles: imp2}, "tya": {WA: true, NZ: true, Implied: true, cycles: imp2},
	"tsx": {WX: true, NZ: true, Implied: true, cycles: imp2}, "txs": {KeepsNZ: true, Implied: true, cycles: imp2},
	"pha": {KeepsNZ: true, Implied: true, cycles: cyc(int(Imp), 3)}, "php": {KeepsNZ: true, Implied: true, cycles: cyc(int(Imp), 3)},
	"pla": {WA: true, NZ: true, Implied: true, cycles: cyc(int(Imp), 4)}, "plp": {Implied: true, cycles: cyc(int(Imp), 4)},
	"clc": {KeepsNZ: true, Implied: true, cycles: imp2}, "sec": {KeepsNZ: true, Implied: true, cycles: imp2},
	"cli": {KeepsNZ: true, Implied: true, cycles: imp2}, "sei": {KeepsNZ: true, Implied: true, cycles: imp2},
	"cld": {KeepsNZ: true, Implied: true, cycles: imp2}, "sed": {KeepsNZ: true, Implied: true, cycles: imp2},
	"clv": {KeepsNZ: true, Implied: true, cycles: imp2}, "nop": {KeepsNZ: true, Implied: true, cycles: imp2},
	"brk": {Implied: true, cycles: cyc(int(Imp), 7)}, "rti": {Implied: true, cycles: cyc(int(Imp), 6)},
	"rts": {KeepsNZ: true, Implied: true, cycles: cyc(int(Imp), 6)},
	"jmp": {KeepsNZ: true, cycles: cyc(int(Abs), 3, int(Ind), 5)}, "jsr": {cycles: cyc(int(Abs), 6)},
	"bcc": {KeepsNZ: true, Rel: true, Inverse: "bcs", cycles: branch}, "bcs": {KeepsNZ: true, Rel: true, Inverse: "bcc", cycles: branch},
	"beq": {KeepsNZ: true, Rel: true, Inverse: "bne", cycles: branch}, "bne": {KeepsNZ: true, Rel: true, Inverse: "beq", cycles: branch},
	"bmi": {KeepsNZ: true, Rel: true, Inverse: "bpl", cycles: branch}, "bpl": {KeepsNZ: true, Rel: true, Inverse: "bmi", cycles: branch},
	"bvc": {KeepsNZ: true, Rel: true, Inverse: "bvs", cycles: branch}, "bvs": {KeepsNZ: true, Rel: true, Inverse: "bvc", cycles: branch},
}

// Writes は mnem をモード m で実行したときに書くレジスタ。知らない命令 (マクロ) と jsr は全部を書くと見る
// (share/runtime.asm の乗除算の例外は codegen の asmLine.writes が扱う)。
func Writes(mnem string, m Mode) (a, x, y bool) {
	info, ok := Table[mnem]
	if !ok || mnem == "jsr" {
		return true, true, true
	}
	switch mnem {
	case "asl", "lsr", "rol", "ror":
		return m == Imp, false, false
	}
	return info.WA, info.WX, info.WY
}

// Cycles は mnem をモード m で実行したときのサイクル数 (知らない命令・無いモードは 0)。
func Cycles(mnem string, m Mode) int { return Table[mnem].Cycles(m) }
