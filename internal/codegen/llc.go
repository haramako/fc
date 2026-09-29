package codegen

// LLC: 中間コード (ir.go) → ca65 アセンブリ。命令選択 (llc.go / arith.go / operand.go / call.go / data.go) と
// asm テキストの後処理 (peephole.go、extendjump.go)。IR→IR の最適化は internal/opt、置き場所の決定は internal/regalloc。

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/regalloc"
	"github.com/haramako/fc/internal/types"
)

type Llc struct {
	farPointerSymbols map[string]bool // Link-time bank range assertions for this module.

	OptimizeLevel int
	Limits        regalloc.Limits // レジスタ領域の大きさ (base.asm と一致させる)
	FarCall       bool            // far call が有効 (各モジュールに farcall / FC_FARCALL の import を出す)
	labelCount    int
	ResidentFixes int // 常駐を触らないはずの命令が書いていて、退避 / 復帰に直した数 (regalloc の見積もりの外れ。CompileLambda)
	// MisclassifyResident はテスト用: 常駐レジスタを退避する (ResClobber) と見積もった命令を「触らない」(ResFree) に
	// する。本体がそのレジスタを書いていれば CompileLambda が見つけて退避 / 復帰に直す (自己修正が働くことを確かめる。
	// 普段のビルドでは見積もりが外れないので、この経路は通らない)
	MisclassifyResident bool
	codeSegment         string
	curLambda           *ir.Lambda            // 処理中の関数 (エラー位置の補完用)
	curOp               *ir.Op                // 処理中の命令 (エラー位置の補完用)
	zero                *ir.Value             // 定数 0 (mul の 0 倍の最適化用)
	Lambdas             map[string]*ir.Lambda // Id → 関数 (全モジュール。呼び先の呼び出し規約を引く。frames.Analyze の結果。SetLambdas)

	// DebugFile が nil でなければ、命令ごとに fc のソース位置を `.dbg line, "file", N` で .s に埋める (fcc build -g)。
	// ld65 の --dbgfile に載り、Mesen が fc のソースをステップ実行できる。DebugFile は sema のファイル参照
	// (Dir 相対) を .dbg に書く名前 (ROM の隣から辿れる相対パス) にする
	DebugFile func(ref string) string
	// LogSites は -g のときの @log の地点 (log.go。Compile の順に増える)
	LogSites []*LogSite
	dbgFiles map[string]bool // このモジュールで宣言済みの .dbg file
	dbgLast  string          // 直前に出した .dbg line (同じ行の命令の間では出さない)

	// ループ内の常駐 (doc/v2_regalloc.md): 処理中の命令でレジスタ (ir.Reg) を占有している変数と、その扱い
	res    [ir.NumRegs]*ir.Value // op.Res[reg].V
	resMem [ir.NumRegs]bool      // 退避中: res[reg] をメモリ (Home) として参照する
	holdA  bool                  // 呼び出しの最後の引数を A に置いてから call まで (A の常駐は退避済みで、call では退避しない)
	holdX  int                   // stack 系の呼び出しの push_result (ldx FC_SP) から call まで (入れ子の深さ): X = FC_SP のまま。X の常駐はメモリ側で扱い、復帰しない
	aHeld  bool                  // A は res[A] で塞がっていて、この命令は res[A] を触らない (Y で代用する)

	// 添字付きオペランドの融合: `sub d = x, t` / `lt d = x, t` の t が直前の 添字付きの load_mem (グローバルの 1 バイト配列) の結果なら、
	// 添字付きの load_mem は Y (または X) を用意するだけにして、t を `tab+0,y` として読む (sta t; lda x; sbc t → lda x; sbc tab,y)。
	// 可換な演算は opt.commuteTemp が t を第 1 入力にするので、ここは非可換な sub / lt の第 2 入力だけ
	fused   map[*ir.Value]string // t → オペランドの表記
	fusedAt int                  // 融合した 添字付きの load_mem の命令番号 (直後の命令でだけ有効)
}

func NewLlc(optimizeLevel int, u *types.Universe) *Llc {
	return &Llc{OptimizeLevel: optimizeLevel, Limits: regalloc.DefaultLimits, zero: ir.NewIntLiteral("", u.IntType(1, false), 0)}
}

// ldReg / stReg はレジスタに読む / から書く命令。
var (
	ldReg = [ir.NumRegs]string{ir.RegA: "lda", ir.RegY: "ldy", ir.RegX: "ldx"}
	stReg = [ir.NumRegs]string{ir.RegA: "sta", ir.RegY: "sty", ir.RegX: "stx"}
)

// beginOp は命令の処理の前に常駐の状態を op の印から取る (退避は無し)。
func (l *Llc) beginOp(op *ir.Op) {
	for reg := range op.Res {
		l.res[reg] = op.Res[reg].V
	}
	l.resMem = [ir.NumRegs]bool{}
	l.aHeld = false
}

// endOp は命令の処理の後に常駐の状態を消す。
func (l *Llc) endOp() {
	l.res = [ir.NumRegs]*ir.Value{}
	l.resMem = [ir.NumRegs]bool{}
	l.aHeld = false
}

// spillResident は常駐の変数をメモリ側 (Home) に書く命令。
func (l *Llc) spillResident(op *ir.Op, reg ir.Reg) string {
	return stReg[reg] + " " + l.byte(op.Res[reg].V.Home, 0)
}

// restoreResident は restore の立っているレジスタに常駐の値をメモリ側 (Home) から読み戻す命令 (order の順)。
func (l *Llc) restoreResident(op *ir.Op, restore [ir.NumRegs]bool, order ...ir.Reg) []any {
	var r []any
	for _, reg := range order {
		if restore[reg] {
			r = append(r, ldReg[reg]+" "+l.byte(op.Res[reg].V.Home, 0))
		}
	}
	return r
}

// asmLines は文字列 / nil / ネストした配列を保持する行バッファ。
type asmLines struct {
	lines []any
}

func (a *asmLines) push(xs ...any) {
	a.lines = append(a.lines, xs...)
}

// flatten は flatten + delete(nil) 相当。
func (a *asmLines) flatten() []string {
	var r []string
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case nil:
			// delete(nil)
		case string:
			r = append(r, x)
		case []any:
			for _, e := range x {
				walk(e)
			}
		case []string:
			for _, e := range x {
				walk(e)
			}
		default:
			panic(fmt.Sprintf("invalid asm line %T", v))
		}
	}
	for _, e := range a.lines {
		walk(e)
	}
	return r
}

// Compile はモジュールをアセンブラに変換する。(asm, inc) の行リストを返す。
// コード生成中の CompileError は処理中の関数の宣言位置を補完して返す (回復点)。
func (l *Llc) Compile(mod *ir.Module) (asmOut, incOut []string, err error) {
	defer func() {
		if r := recover(); r != nil {
			if ce, ok := r.(*diag.Error); ok {
				if !ce.Pos.IsValid() && l.curOp != nil && l.curOp.Pos.IsValid() {
					ce.Pos = l.curOp.Pos
				}
				if !ce.Pos.IsValid() && l.curLambda != nil {
					ce.Pos = l.curLambda.Pos
				}
				err = ce
				return
			}
			panic(r)
		}
	}()
	l.farPointerSymbols = map[string]bool{}
	l.labelCount = 0
	l.codeSegment = mod.Id
	l.curLambda = nil
	l.curOp = nil

	inc := &asmLines{}
	asm := &asmLines{}
	l.dbgFiles, l.dbgLast = map[string]bool{}, ""
	asm.push("\t.setcpu \"6502\"")
	asm.push("\t.include \"macro.inc\"")
	asm.push("\t.include \"_frames.inc\"") // 静的フレームの配置 (frames.Place が生成)
	asm.push("\t.importzp FC_SP")          // スタックの空き先頭 (base.asm)
	asm.push(fmt.Sprintf("__MODULE_%s__ = 1", strings.ToUpper(mod.Id)))

	inc.push(fmt.Sprintf(".ifndef __MODULE_%s__", strings.ToUpper(mod.Id)))
	inc.push(fmt.Sprintf("__MODULE_%s__ = 1", strings.ToUpper(mod.Id)))

	asm.push(fmt.Sprintf(".segment \"%s\"", l.codeSegment)) // dummy

	for _, m := range mod.Uses {
		inc.push(fmt.Sprintf("\t.include \"_%s.inc\"", m.Id))
		asm.push(fmt.Sprintf("\t.include \"_%s.inc\"", m.Id))
	}
	if l.FarCall {
		asm.push("\t.global farcall") // トランポリンを include したモジュールでは export、それ以外では import になる
		asm.push("\t.import FC_FARCALL")
	}

	// include(.asm)の処理
	for _, file := range mod.IncludeAsms {
		asm.push(fmt.Sprintf("\t.include \"%s\"", file))
	}

	for _, d := range mod.Defs {
		switch d.Kind {
		case ir.DefEqu:
			var val string
			if d.Equ.IsInt {
				val = strconv.Itoa(d.Equ.Int)
			} else {
				val = mangle(d.Equ.Symbol)
				if d.Equ.Type.IsFarFunc() {
					l.farPointerSymbols[d.Equ.Symbol] = true
				}
			}
			inc.push(fmt.Sprintf("%s = %s", mangle(d.Sym), val))
			asm.push(fmt.Sprintf("%s = %s", mangle(d.Sym), val))
		case ir.DefBss:
			if d.Unused {
				continue // どこからも参照されない private な変数は領域を取らない (pipeline.markUnusedGlobals)
			}
			inc.push(fmt.Sprintf("\t.import %s", mangle(d.Sym)))
			asm.push(fmt.Sprintf("\t.export %s", mangle(d.Sym)))
			if d.Segment != "" {
				asm.push(fmt.Sprintf(".segment \"%s\"", d.Segment))
			} else {
				asm.push(".segment \"BSS\"")
			}
			asm.push(fmt.Sprintf("%s: .res %d", mangle(d.Sym), d.Type.Size))
		case ir.DefBlock:
			inc.push(fmt.Sprintf("\t.import %s", mangle(d.Sym)))
			asm.push(fmt.Sprintf("\t.export %s", mangle(d.Sym)))
			asm.push(fmt.Sprintf(".segment \"%s\"", l.codeSegment))
			asm.push(l.emitBlock(d.Sym, d.Type, d.Elems))
		case ir.DefExtern:
			// asm 側の定義の参照 (値なしの const の options(symbol:)): `.global` は定義があれば export、無ければ import
			// になるので、同じモジュールに include した asm で定義していても別のオブジェクトファイルでもよい
			inc.push(fmt.Sprintf("\t.global %s", mangle(d.Sym)))
			asm.push(fmt.Sprintf("\t.global %s", mangle(d.Sym)))
		case ir.DefCode:
			lmd := d.Lambda
			if lmd.Unused {
				continue // どこからも届かない関数は出力しない (frames.Analyze)
			}
			if lmd.Extern {
				// 本体なし (asm 側の定義の参照): DefExtern と同じく `.global` (以前は `.export` で、同じモジュールに
				// include した asm で定義したものしかリンクできなかった)
				inc.push(fmt.Sprintf("\t.global %s", mangle(d.Sym)))
				asm.push(fmt.Sprintf("\t.global %s", mangle(d.Sym)))
				continue
			}
			inc.push(fmt.Sprintf("\t.import %s", mangle(d.Sym)))
			asm.push(fmt.Sprintf("\t.export %s", mangle(d.Sym)))
			if lmd.Entry {
				inc.push(fmt.Sprintf("\t.import %s", directSym(mangle(d.Sym))))
				asm.push(fmt.Sprintf("\t.export %s", directSym(mangle(d.Sym))))
			}
			if lmd.RegArg || lmd.RegArgY {
				inc.push(fmt.Sprintf("\t.import %s", frameSym(mangle(d.Sym))))
				asm.push(fmt.Sprintf("\t.export %s", frameSym(mangle(d.Sym))))
			}
			if lmd.RegArg && lmd.RegArgY {
				inc.push(fmt.Sprintf("\t.import %s", aSym(mangle(d.Sym))))
				asm.push(fmt.Sprintf("\t.export %s", aSym(mangle(d.Sym))))
			}
			asm.push(anyList(l.CompileLambda(d.Sym, lmd)))
		default:
			panic(fmt.Sprintf("invalid def kind %s", d.Kind))
		}
	}

	// fastcall 関数が使う FC_FASTCALL_REG の大きさを、base.asm (プロジェクトが自前で持つこともある) の .res とリンク時に突き合わせる
	fastcallNeed := 0
	for _, d := range mod.Defs {
		if d.Kind == ir.DefCode && ((!d.Lambda.Extern && d.Lambda.ABI == ir.ABIFastcall) || d.Lambda.ABI == ir.ABICc65) {
			fastcallNeed = max(fastcallNeed, d.Lambda.ZpUsed)
		}
	}
	if fastcallNeed > 0 {
		asm.push("	.import FC_FASTCALL_REG_SIZE")
		asm.push(fmt.Sprintf("	.assert FC_FASTCALL_REG_SIZE >= %d, error, \"fastcall functions of module %s need %d bytes of FC_FASTCALL_REG (raise .res of FC_FASTCALL_REG and FC_FASTCALL_REG_SIZE in base.asm)\"", fastcallNeed, mod.Id, fastcallNeed))
	}

	// include header(.asm)の処理
	for _, file := range mod.IncludeHeaders {
		asm.push(fmt.Sprintf("\t.include \"%s\"", file))
	}

	// include(.chr)の処理
	for _, file := range mod.IncludeChrs {
		asm.push(".segment \"CHARS\"")
		asm.push(fmt.Sprintf("\t.incbin \"%s\"", file))
	}

	var farSymbols []string
	for sym := range l.farPointerSymbols {
		farSymbols = append(farSymbols, sym)
	}
	sort.Strings(farSymbols)
	for _, sym := range farSymbols {
		asm.push(fmt.Sprintf(".assert .bank(%s) >= 0, lderror, \"farfn bank must be in 0..255\"", mangle(sym)))
		asm.push(fmt.Sprintf(".assert .bank(%s) <= 255, lderror, \"farfn bank must be in 0..255\"", mangle(sym)))
	}
	inc.push(".endif")

	return asm.flatten(), inc.flatten(), nil
}

// fusableIndex は ops[i] (添字付きの load_mem、グローバル配列) の結果を直後の sub / lt の第 2 入力に融合できるか。
// 結果は 1 バイトの一時変数で、その命令でしか使われない (live range が直後まで)。添字は Y に入れる (X に常駐していれば X)。
// 直後の命令の第 1 入力が A に常駐している / 添字が A にある組み合わせは、tay が A を壊すので除く。
func (l *Llc) fusableIndex(ops []*ir.Op, i int) (string, bool) {
	op := ops[i]
	if i+1 >= len(ops) || ops[i+1] == nil || op.Code != ir.OpLoadMem {
		return "", false
	}
	if m := op.Mem(); !m.BaseIsArray() || m.Index == nil || m.Scale != 1 {
		return "", false
	}
	t, ok := op.Dst.(*ir.Value)
	if !ok || t.LocalType != ir.LTTemp || t.Type.Size != 1 || t.LiveRange == nil || t.LiveRange.Max != i+1 || ir.ValLocation(t) == ir.LocA {
		return "", false
	}
	next := ops[i+1]
	if next.Code != ir.OpSub && next.Code != ir.OpLt || len(next.Src) != 2 || next.Src[1] != ir.Operand(t) || next.Src[0] == ir.Operand(t) {
		return "", false
	}
	if ir.ValType(next.Src[0]).Size != 1 {
		return "", false
	}
	if ir.ValLocation(next.Src[0]) == ir.LocX || ir.ValLocation(next.Src[0]) == ir.LocY || next.Res[ir.RegA].V != nil {
		// cpx / cpy に添字付きのオペランドは無い (`cpx tab+0,y` を出していた。fuzz で発覚)。A に常駐変数があると
		// 次の命令が Y で代用 (UseY: `ldy a; cpy b`) されることがあるので、それも融合しない (`cpy seq+0,y`)
		return "", false
	}
	if l.inA(op.In(1)) {
		return "", false
	}
	if l.inX(op.In(1)) {
		return "x", true
	}
	return "y", true
}

// dbgLine は ソース位置 pos の `.dbg line` (初出のファイルは `.dbg file` も)。同じ位置が続く間は空。
func (l *Llc) dbgLine(file string, line int) []any {
	name := l.DebugFile(file)
	key := fmt.Sprintf("%s:%d", name, line)
	if key == l.dbgLast {
		return nil
	}
	l.dbgLast = key
	var r []any
	if !l.dbgFiles[name] {
		l.dbgFiles[name] = true
		r = append(r, fmt.Sprintf(".dbg file, \"%s\", 0, 0", name))
	}
	return append(r, fmt.Sprintf(".dbg line, \"%s\", %d", name, line))
}

func anyList(ss []string) []any {
	r := make([]any, len(ss))
	for i, s := range ss {
		r[i] = s
	}
	return r
}

// testA は値の i バイト目を A に読んで N / Z を立てる。値がすでに A にある (loadA が何も出さない) ときは `cmp #0`
// (直前が A を書いた命令ならピープホールが消す)。呼び出しの戻り値 (A) を if で見るとき、call の後の常駐の復帰 (`ldy home`)
// がフラグを壊していた (`if ((f(x)) as sint16)`。fuzz で発覚)。
//
// 検査のためだけの lda には testMark を付ける。直前の命令がフラグをその場所の値で立てていれば (`dec x` の直後など)
// ピープホールが消す。IR の命令の単位ではなく実際に出た命令列で判断するので、間に挟まる常駐の復帰や、2 バイトの inc の
// 中の分岐 (`inc lo; bne @s; inc hi; @s:`) を見落とさない (codegen の flagsFromIncDec でこの形のバグが続いた)。
func (l *Llc) testA(v ir.Operand, i int) any {
	if code := l.loadA(v, i); code != nil {
		return markTest(code)
	}
	return "cmp #0"
}

// testMark は「フラグを立てるためだけのロード」の印 (行末のコメント)。ピープホールが消すか、最後に印だけ外す。
const testMark = " ; tst"

// markTest は code がメモリからの 1 命令の lda / ldy なら testMark を付ける。
func markTest(code any) any {
	s, ok := code.(string)
	if !ok || !(strings.HasPrefix(s, "lda ") || strings.HasPrefix(s, "ldy ")) || strings.HasPrefix(s[4:], "#") {
		return code
	}
	return s + testMark
}

// stripTestMarks は残った testMark を外す。
func stripTestMarks(lines []string) []string {
	for i, line := range lines {
		lines[i] = strings.TrimSuffix(line, testMark)
	}
	return lines
}

// nextOp は i の次の (nil でない) 命令。
func nextOp(ops []*ir.Op, i int) *ir.Op {
	for j := i + 1; j < len(ops); j++ {
		if ops[j] != nil {
			return ops[j]
		}
	}
	return nil
}

// restoreY は要素 size バイトのポインタ参照 (`lda (p),y; iny; lda (p),y`) の後で、添字が Y に常駐しているなら Y を戻す
// (`q0[i] += 1` の store_mem が load_mem の iny でずれた Y で書いていた。fuzz で発覚)。
func (l *Llc) restoreY(idx ir.Operand, size int) []any {
	var r []any
	if l.inY(idx) {
		for i := 1; i < size; i++ {
			r = append(r, "dey")
		}
	}
	return r
}

// CompileLambda は関数1つ分のアセンブリを生成する (Prepare 済みであること)。
//
// 常駐レジスタの扱い (置いたまま実行する / 触らない / 退避する) は regalloc.Classify が決める。その「形」(Y に常駐する変数の
// ldy / cpy / iny、A を使わないメモリ上の inc …) は regalloc の形の表 (regalloc/forms.go) で、codegen は同じ表で命令を出す
// (以前は codegen の出力を手で写した予測表で、食い違いを関数ごと最大 8 回コンパイルし直して吸収していた)。表の外の予測
// (汎用の出力が A / X / Y を使うか: needsX / needsY、A の値をそのまま扱う形) が外れて、触らないとした命令の本体が常駐
// レジスタを書いていたら、その命令だけ退避 / 復帰にして出し直す (compileLambda)。テストと fuzz (FC_VERIFY_REGS) では
// 食い違いそのものをコンパイルエラーにする。FC_TRACE_RESIDENT で出し直した命令を stderr に出す。
func (l *Llc) CompileLambda(sym string, lmd *ir.Lambda) []string {
	g := &funcGen{Llc: l}
	return g.compileLambda(sym, lmd)
}

// compileLambda は CompileLambda の本体。
func (l *funcGen) compileLambda(sym string, lmd *ir.Lambda) []string {
	l.lmd = lmd
	l.curLambda = lmd // エラー位置の補完用 (Compile の回復点で参照するので、ここでは戻さない)
	l.curOp = nil
	l.ops = lmd.Ops
	ops := l.ops
	siteStart := len(l.LogSites)
	var lv *ir.Liveness
	liveness := func() *ir.Liveness { // @log の値の生存 (地点がある関数だけ作る)
		if lv == nil {
			lv = ir.BuildLiveness(lmd)
		}
		return lv
	}

	l.r = &asmLines{}
	r := l.r

	r.push(";;;=============================")
	r.push(fmt.Sprintf(";;; function %s", lmd.Id))
	r.push(";;;=============================")

	if seg := lmd.Segment(); seg != "" {
		r.push(fmt.Sprintf(".segment \"%s\"", seg))
	} else {
		r.push(fmt.Sprintf(".segment \"%s\"", l.codeSegment))
	}
	if lmd.RegArg || lmd.RegArgY {
		// レジスタ渡しの入口 (doc/v2_frame_alloc.md §7): 呼び出し側は最後の引数を A、その前を Y に置いて `sym` / `sym__direct`
		// から入り、`sty` / `sta` でフレームに写す。レジスタに置けなかった引数はフレームに書いてあるので、その前の入口
		// (`sym__frame`: 両方フレーム、`sym__a`: Y だけフレーム) がレジスタに読んでから同じ `sty` / `sta` に落ちる
		// (本体の先頭では常に A / Y に引数があり、ピープホールが先頭の lda / ldy を消す)。.proc の中のラベルは同じファイルの
		// 別の .proc から見えない (castle の text モジュールで未定義になった) ので、入口ごとに .proc を閉じる (.endproc は
		// コードを出さないのでそのまま落ちる)
		if lmd.RegArg {
			r.push(fmt.Sprintf(".proc %s", frameSym(mangle(sym))), fmt.Sprintf("lda %s", staticAddr(lmd, regArgOffset(lmd))), ".endproc")
		}
		if lmd.RegArgY {
			name := frameSym(mangle(sym))
			if lmd.RegArg {
				name = aSym(mangle(sym))
			}
			r.push(fmt.Sprintf(".proc %s", name), fmt.Sprintf("ldy %s", staticAddr(lmd, regArgYOffset(lmd))), ".endproc")
		}
		if lmd.Entry {
			r.push(fmt.Sprintf("jmp %s", directSym(mangle(sym)))) // 間にスタックからのコピーが入る
		}
	}
	if lmd.Entry {
		// アドレスを取られた関数: 関数ポインタ経由の呼び出し側はスタック (X の指す位置) に引数を積むので、
		// 自分のフレームに写してから本体 (__direct。呼び先が分かっている呼び出しはここから入る) へ
		r.push(mangle(sym) + ":")
		for k := lmd.Type.Base.Size; k < lmd.Type.Base.Size+argBytes(lmd); k++ {
			if lmd.RegArg && k == regArgOffset(lmd) {
				r.push(fmt.Sprintf("lda <S+%d,x", k)) // 最後の引数は A のまま __direct の sta へ
				continue
			}
			if lmd.RegArgY && k == regArgYOffset(lmd) {
				r.push(fmt.Sprintf("ldy <S+%d,x", k)) // その前の引数は Y のまま __direct の sty へ
				continue
			}
			r.push(fmt.Sprintf("lda <S+%d,x", k), fmt.Sprintf("sta %s", staticAddr(lmd, k)))
		}
		r.push(fmt.Sprintf(".proc %s", directSym(mangle(sym))))
	} else {
		r.push(fmt.Sprintf(".proc %s", mangle(sym)))
	}
	if lmd.RegArgY {
		r.push(fmt.Sprintf("sty %s", staticAddr(lmd, regArgYOffset(lmd)))) // Y の最後から 2 つ目の引数をフレームに
	}
	if lmd.RegArg {
		r.push(fmt.Sprintf("sta %s", staticAddr(lmd, regArgOffset(lmd)))) // A の最後の引数をフレームに
	}
	if lmd.ABI == ir.ABIStack && lmd.FrameSize > 0 {
		// stack 関数: X = フレームの底 (呼び出し側が FC_SP にした)。空き先頭をフレームの後ろへ
		r.push("txa", "clc", fmt.Sprintf("adc #%d", lmd.FrameSize), "sta FC_SP")
	}

	verify := lmd.Cfg().VerifyRegs() // テストと fuzz で有効 (verifyRegs)

	for opNo, op := range ops {
		if op == nil {
			continue
		}
		l.curOp = op
		l.op, l.opNo = op, opNo
		// IRコメント (golden比較では除去されるため、Go版独自の形式でよい)
		cm := ir.DumpOp(op, nil)
		if len(cm) > 120 {
			cm = cm[:120]
		}
		comment := fmt.Sprintf("; %04d: %s", opNo, cm)
		if l.DebugFile != nil && len(op.Logs) > 0 {
			comment += l.logMarkers(lmd, opNo, op, liveness) // @log の地点 (log.go)
		}
		r.push(comment)
		if l.DebugFile != nil && op.Pos.IsValid() && op.Code != ir.OpLabel {
			r.push(l.dbgLine(op.Pos.Filename, op.Pos.Line))
		}

		if l.fused != nil && opNo != l.fusedAt+1 {
			l.fused = nil
		}
		// 常駐を触らないはずの本体がそのレジスタを書いていたら、この命令だけ退避 / 復帰にして出し直す
		// (regalloc の形の表・規則と codegen の出力の食い違い。テストと fuzz (FC_VERIFY_REGS) ではコンパイルエラー)
		var snap opState
		if op.HasResident() {
			snap = l.saveOp()
		}
		var forced regsKept
		for try := 0; ; try++ {
			w := l.compileOp(opNo, op, forced, verify)
			if !w.any() {
				break
			}
			if verify && !l.MisclassifyResident || try >= int(ir.NumRegs) {
				panic(&diag.Error{Msg: fmt.Sprintf("internal: resident register written by an op classified as not touching it (%+v; regalloc forms / rules disagree with codegen) in %s: %s", w, lmd.Id, ir.DumpOp(op, nil))})
			}
			if lmd.Cfg().Trace("resident") != "" {
				fmt.Fprintf(os.Stderr, "resident: %s op %d writes %+v; spill instead: %s\n", lmd.Id, opNo, w, ir.DumpOp(op, nil))
			}
			l.ResidentFixes++
			l.restoreOp(snap)
			forced = forced.or(w)
		}
	}

	// 関数の中の const の表・文字列 (.proc の中のラベルなので、この関数の中からしか参照されない)。コード (インラインアセンブラを
	// 含む) とほかの表から参照されているものだけ出す (インライン展開で写した表が、最適化で使われなくなることがある)
	blocks := make([][]any, len(lmd.Defs))
	for i, d := range lmd.Defs {
		if d.Kind != ir.DefBlock {
			panic(fmt.Sprintf("invalid lambda def kind %s", d.Kind))
		}
		blocks[i] = l.emitBlock(d.Sym, d.Type, d.Elems)
	}
	for _, i := range referencedDefs(lmd.Defs, blocks, r.flatten()) {
		r.push(blocks[i])
	}

	lines := r.flatten() // まとめた行を展開 + 空の行を削除
	// ラベル行,コメント行以外はインデントする
	for i, line := range lines {
		if reIndentExempt.MatchString(line) {
			// そのまま
		} else {
			lines[i] = "\t" + line
		}
	}

	lines = append(lines, ".endproc")

	if l.OptimizeLevel > 0 && !lmd.Cfg().Disabled("peephole") {
		lines = peepholeA(lines)
	}
	lines = stripTestMarks(lines)
	lines = l.extendJump(lines)
	if len(l.LogSites) > siteStart {
		lines = l.placeLogLabels(lines, l.LogSites[siteStart:])
	}

	lmd.Asm = lines
	return lines
}

// compileOp は命令 1 つを出す (常駐の扱いの決定・退避と復帰・本体・レジスタの検査)。forced のレジスタは常駐を退避 / 復帰する。
// 戻り値は、常駐を触らないとした本体が書いていたレジスタ (空ならこの命令は正しい)。
func (l *funcGen) compileOp(opNo int, op *ir.Op, forced regsKept, verify bool) regsKept {
	r, lmd := l.r, l.lmd
	// A / Y 常駐: この命令の扱い (friendly / 触らない / 退避)
	l.beginOp(op)
	l.restore = [ir.NumRegs]bool{} // 命令の後でレジスタに戻す常駐
	restore := &l.restore
	opStart, holdAIn, holdXIn := len(r.lines), l.holdA, l.holdX
	var d regalloc.Decision
	if op.HasResident() {
		d, _ = regalloc.Classify(lmd, opNo, op.Res[ir.RegA].V, op.Res[ir.RegY].V, op.Res[ir.RegX].V, op.Res[ir.RegA].In || op.Res[ir.RegA].Out, op.Res[ir.RegA].Out, op.Res[ir.RegY].In || op.Res[ir.RegY].Out)
		if l.MisclassifyResident {
			// テスト用: 見積もりをわざと外す (下の forced が直す)。常駐の変数そのものを読み書きする命令は、退避して
			// メモリ側で扱うしかない (レジスタのまま出せない形がある) ので対象外。実際に外れるのも「変数を触らない
			// 命令がレジスタを書いていた」形
			if d.A == regalloc.ResClobber && !opInvolves(op, op.Res[ir.RegA].V) {
				d.A, d.UseY = regalloc.ResFree, false
			}
			if d.Y == regalloc.ResClobber && !opInvolves(op, op.Res[ir.RegY].V) {
				d.Y = regalloc.ResFree
			}
			if d.X == regalloc.ResClobber && !opInvolves(op, op.Res[ir.RegX].V) {
				d.X = regalloc.ResFree
			}
		}
		// 前の試みで、触らないはずの本体が書いていた常駐レジスタは退避 / 復帰する (compileLambda)
		if f := forced; f.any() {
			if f.a && d.A == regalloc.ResFree {
				d.A, d.UseY = regalloc.ResClobber, false
			}
			if f.y && d.Y == regalloc.ResFree {
				d.Y = regalloc.ResClobber
			}
			if f.x && d.X == regalloc.ResFree {
				d.X = regalloc.ResClobber
			}
		}
		// stack 系の呼び出しの引数を積んでいる間 (push_result の ldx FC_SP から call まで) は X = FC_SP のまま:
		// X の常駐はメモリ側で扱い、退避も復帰もしない (push_result の直後の復帰 `ldx g1` で X が常駐の値に戻り、
		// `sta <S+1,x` が別の場所に引数を書いていた。fuzz で発覚)。call の後で復帰する
		if op.Res[ir.RegX].V != nil && (d.X == regalloc.ResClobber || l.holdX > 0) {
			if op.Res[ir.RegX].In && !op.Res[ir.RegX].V.Clean && l.holdX == 0 {
				r.push(l.spillResident(op, ir.RegX))
			}
			l.resMem[ir.RegX] = true
			restore[ir.RegX] = op.Res[ir.RegX].Out && (l.holdX == 0 || (ir.IsCall(op) && l.holdX == 1))
		}
		// 呼び出しの引数を Y に保持中 (markArgY: ArgY の push_arg から call まで) は Y を代用にも常駐にも使わない。
		// 常駐変数は ArgY の push_arg で退避してメモリ側で扱い、call の後で復帰する (間の命令と call では退避も復帰もしない)。
		// A の最後の引数も同じ (push_arg で退避、call では退避しない)
		holdY := op.ArgY || op.HoldY
		if holdY && d.UseY {
			d.UseY, d.A = false, regalloc.ResClobber
		}
		if op.Res[ir.RegA].V != nil && (d.A == regalloc.ResClobber || l.holdA) {
			if op.Res[ir.RegA].In && !op.Res[ir.RegA].V.Clean && !l.holdA {
				r.push(l.spillResident(op, ir.RegA))
			}
			l.resMem[ir.RegA] = true
			restore[ir.RegA] = op.Res[ir.RegA].Out
		}
		if op.Res[ir.RegY].V != nil && (d.Y == regalloc.ResClobber || holdY) {
			if op.Res[ir.RegY].In && !op.Res[ir.RegY].V.Clean && !op.HoldY {
				r.push(l.spillResident(op, ir.RegY))
			}
			l.resMem[ir.RegY] = true
			restore[ir.RegY] = op.Res[ir.RegY].Out && !op.ArgY && !(op.HoldY && !ir.IsCall(op))
		}
		l.aHeld = d.UseY
	}
	bodyStart := len(r.lines)
	keep := regsKept{
		a: op.Res[ir.RegA].V != nil && !l.resMem[ir.RegA] && d.A == regalloc.ResFree,
		y: op.Res[ir.RegY].V != nil && !l.resMem[ir.RegY] && d.Y == regalloc.ResFree,
		x: op.Res[ir.RegX].V != nil && !l.resMem[ir.RegX] && d.X == regalloc.ResFree,
	}

	switch op.Code {
	case ir.OpLabel:
		l.genLabel()
	case ir.OpIf, ir.OpIfTrue:
		l.genIf()
	case ir.OpIfCarry:
		l.genIfCarry()
	case ir.OpIfNotCarry:
		l.genIfNotCarry()
	case ir.OpJump:
		l.genJump()
	case ir.OpSwitch:
		l.genSwitch()
	case ir.OpReturn:
		l.genReturn()
	case ir.OpPushResult, ir.OpPushFastcallResult:
		l.genPushResult()
	case ir.OpPushArg, ir.OpPushFastcallArg:
		l.genPushArg()
	case ir.OpCall, ir.OpFastcall:
		l.genCall()
	case ir.OpLoad:
		l.genLoad()
	case ir.OpSignExtension:
		l.genSignExtension()
	case ir.OpAdd, ir.OpSub:
		l.genAddSub()
	case ir.OpAnd, ir.OpOr, ir.OpXor:
		l.genBitwise()
	case ir.OpMul, ir.OpDiv, ir.OpMod:
		l.genMulDivMod()
	case ir.OpRolC, ir.OpRorC:
		l.genRotateCarry()
	case ir.OpShiftLeft, ir.OpShiftRight:
		l.genShift()
	case ir.OpUminus:
		l.genUminus()
	case ir.OpEq:
		l.genEq()
	case ir.OpLt:
		l.genLt()
	case ir.OpNot:
		l.genNot()
	case ir.OpBitNot:
		l.genBitNot()
	case ir.OpAsm:
		l.genAsm()
	case ir.OpIndex:
		l.genIndex()
	case ir.OpRef:
		l.genRef()
	case ir.OpLoadMem:
		l.genLoadMem()
	case ir.OpStoreMem:
		l.genStoreMem()
	default:
		panic(fmt.Sprintf("unknow op %s", ir.DumpOp(op, nil)))
	}
	bodyEnd := len(r.lines)
	if *restore != ([ir.NumRegs]bool{}) {
		// 結果がコンディションレジスタ (次の if が見るフラグ) なら、復帰の lda / ldy / ldx で N / Z を壊さないように
		// php / plp で挟む (castle の `on_idx == i` で i@X の復帰 ldx が Z を消して踏むスイッチが効かなかった)。
		// C (符号なしの lt) はロードで変わらないので挟まない
		cond := ir.CondRestoreNeedsFlags(op)
		if cond {
			r.push("php")
		}
		r.push(l.restoreResident(op, *restore, ir.RegA, ir.RegY, ir.RegX)...)
		if cond {
			r.push("plp")
		}
	}
	written := keep.and(regsWritten(r.lines[bodyStart:bodyEnd])) // 常駐を触らないはずの本体が書いたレジスタ
	if verify {
		// 呼び出しの引数の保持中 (A の最後の引数、Y の引数、stack 系の X = FC_SP) は、退避・復帰も含めて命令全体で触らない
		// (codegen の中の約束事なので、破っていたらコンパイルエラー)
		hold := regsKept{
			a: holdAIn && !ir.IsCall(op),
			y: op.HoldY && !ir.IsCall(op),
			x: holdXIn > 0 && !ir.IsCall(op) && op.Code != ir.OpPushResult,
		}
		l.verifyRegs(op, "引数の保持", hold, r.lines[opStart:])
	}
	l.endOp()
	return written
}

// opState は命令 1 つを出し直すときに戻す状態 (出力の行、ラベルの番号、融合、呼び出しの引数の保持、.dbg)。
type opState struct {
	lines               int
	labelCount          int
	fused               map[*ir.Value]string
	fusedAt             int
	holdA               bool
	holdX               int
	calls               []pendingCall
	pushArgSize         int
	pushFastcallArgSize int
	dbgLast             string
	logSites            int
}

func (l *funcGen) saveOp() opState {
	s := opState{lines: len(l.r.lines), labelCount: l.labelCount, fused: l.fused, fusedAt: l.fusedAt, holdA: l.holdA, holdX: l.holdX,
		pushArgSize: l.pushArgSize, pushFastcallArgSize: l.pushFastcallArgSize, dbgLast: l.dbgLast, logSites: len(l.LogSites)}
	for _, pc := range l.calls {
		s.calls = append(s.calls, *pc)
	}
	return s
}

func (l *funcGen) restoreOp(s opState) {
	l.r.lines = l.r.lines[:s.lines]
	l.labelCount, l.fused, l.fusedAt, l.holdA, l.holdX = s.labelCount, s.fused, s.fusedAt, s.holdA, s.holdX
	l.pushArgSize, l.pushFastcallArgSize, l.dbgLast = s.pushArgSize, s.pushFastcallArgSize, s.dbgLast
	l.LogSites = l.LogSites[:s.logSites]
	l.calls = l.calls[:0]
	for i := range s.calls {
		pc := s.calls[i]
		l.calls = append(l.calls, &pc)
	}
}

var reIndentExempt = regexp.MustCompile(`^([.@_a-zA-Z0-9][_a-zA-Z0-9]+:|\.segment|\.proc)`)

func (l *Llc) newLabel() string {
	l.labelCount++
	return fmt.Sprintf("@%d", l.labelCount)
}

// condJump はコンディションレジスタの値 v が真 (onTrue) / 偽のときに飛ぶ分岐命令。CondPositive のとき「真 ⇔ フラグが
// セット」、ただし C だけは「真 ⇔ C クリア」(比較 a < b は C クリアで真。regalloc.allocateCond 参照)。
func condJump(v *ir.Value, onTrue bool) string { return regalloc.CondBranch(v, onTrue) }

func (l *Llc) newLabels(n int) []string {
	r := make([]string, n)
	for i := range r {
		r[i] = l.newLabel()
	}
	return r
}

// ---------------------------------------------------------------
// extend_jump
// ---------------------------------------------------------------

// fnPtrToReg は ops[i] (グローバルの表の 添字付きの load_mem) が読む関数ポインタを、一時変数でなく reg に直接書いてよいとき、
// それを使う呼び出しの命令番号を返す (だめなら -1)。条件: 結果が near の関数ポインタの一時変数で、関数の中でその
// 呼び出しにしか使われず、間にあるのが reg を使わない引数の積み込み (1〜2 バイトの値の push_result / push_arg) と
// 引数の式の単純な演算 (load / add / sub / and / or / xor) だけ。
// 呼び出しの側 (fnPtrInReg) も同じ判定で reg への写しを省く (castle の en.process の `PROCESS[t](i)`: 表 → 一時変数 →
// reg の写し 4 命令、12 サイクルが消える)。
func fnPtrToReg(lmd *ir.Lambda, ops []*ir.Op, i int) int {
	op := ops[i]
	if op == nil || op.Code != ir.OpLoadMem || op.Mem().Index == nil || lmd.Cfg().Disabled("fnptr-reg") {
		return -1
	}
	t, ok := op.Dst.(*ir.Value)
	if !ok || t.LocalType != ir.LTTemp || t.Type.Kind != types.Func || t.Type.IsFarFunc() || t.Type.Size != 2 {
		return -1
	}
	if ir.ValType(op.In(0)).Kind == types.Pointer {
		return -1 // ポインタ経由の表は (reg),y で読むことがある
	}
	call := -1
	for j := i + 1; j < len(ops) && call < 0; j++ {
		b := ops[j]
		if b == nil {
			continue
		}
		switch b.Code {
		case ir.OpPushResult, ir.OpPushFastcallResult:
		case ir.OpPushArg, ir.OpPushFastcallArg, ir.OpLoad, ir.OpAdd, ir.OpSub, ir.OpAnd, ir.OpOr, ir.OpXor:
			// 引数の積み込みと、引数の式の単純な演算 (lda / adc / sta … だけで reg を使わない)
			for _, s := range append([]ir.Operand{b.Dst}, b.Src...) {
				if s == ir.Operand(t) || !simpleArg(s) {
					return -1
				}
			}
		case ir.OpCall, ir.OpFastcall:
			if b.Src[0] != ir.Operand(t) {
				return -1 // 引数の中の呼び出し (reg を壊す)
			}
			call = j
		default:
			return -1
		}
	}
	if call < 0 {
		return -1
	}
	uses := 0
	for _, o := range lmd.Ops {
		if o == nil {
			continue
		}
		defs, us := ir.DefUse(o)
		for _, u := range us {
			if ir.UnderlyingValue(u) == t {
				uses++
			}
		}
		for _, d := range defs {
			if ir.UnderlyingValue(d) == t && o != op {
				return -1
			}
		}
	}
	if uses != 1 {
		return -1
	}
	return call
}

// fnPtrInReg は ops[i] (関数ポインタの一時変数からの呼び出し) の呼び先を、それを定義した 添字付きの load_mem が reg に直接
// 書いたか (fnPtrToReg)。
func fnPtrInReg(lmd *ir.Lambda, ops []*ir.Op, i int) bool {
	for j := i - 1; j >= 0; j-- {
		if b := ops[j]; b != nil && b.Code == ir.OpLoadMem && b.Dst == ops[i].Src[0] {
			return fnPtrToReg(lmd, ops, j) == i
		}
	}
	return false
}

// simpleArg は積み込みの codegen が lda / sta だけで reg を使わない引数 (リテラルか、2 バイト以下の値)。
func simpleArg(s ir.Operand) bool {
	if s == nil || ir.ValKind(s) == ir.KindLiteral {
		return true
	}
	switch s.(type) {
	case *ir.Value, *ir.CastedValue:
		tp := ir.ValType(s)
		return tp.Size <= 2 && tp.Kind != types.Struct && tp.Kind != types.Array
	}
	return false
}

// opInvolves は op が v を読むか書くか (regalloc.involves と同じ。MisclassifyResident 用)。
func opInvolves(op *ir.Op, v *ir.Value) bool {
	if v == nil {
		return false
	}
	defs, uses := ir.DefUse(op)
	for _, o := range append(defs, uses...) {
		if ir.UnderlyingValue(o) == v {
			return true
		}
	}
	return false
}
