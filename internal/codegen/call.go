package codegen

import (
	"fmt"

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/regalloc"
)

// 関数呼び出し (call マクロ / fastcall / far call)。

// farCallSetup は far call の呼び先アドレスとバンク番号 (ld65.cfg の bank = N を .bank で引く) を FC_FARCALL に置く。
func (l *Llc) farCallSetup(sym string) []any {
	s := mangle(sym)
	return []any{
		fmt.Sprintf("lda #<%s", s), "sta FC_FARCALL+0",
		fmt.Sprintf("lda #>%s", s), "sta FC_FARCALL+1",
		fmt.Sprintf("lda #<.bank(%s)", s), "sta FC_FARCALL+2",
	}
}

// pipeline.Backend の実装 (pipeline.Prepare が最適化と割付の間で呼ぶ)。

// SetLambdas は全関数の表 (frames.Analyze の結果) を受け取る。
func (l *Llc) SetLambdas(lambdas map[string]*ir.Lambda) { l.Lambdas = lambdas }

// MarkCalls は割付の前の呼び出しの印 (markArgY と markHoldX、markResultArg。全関数の表は SetLambdas で受け取ったもの)。
func (l *Llc) MarkCalls(lmd *ir.Lambda) {
	p := l.planCalls(lmd.Ops)
	markArgY(lmd.Ops, p)
	markHoldX(lmd.Ops, p)
	markResultArg(lmd.Ops, p)
}

// markResultArg は、static な関数の 3 バイト以上の戻り値を受けた一時変数 T をすぐ次の呼び出しの最初の引数にするだけの
// push_arg (`sum(get())`: `call T ← get; push_result; push_arg T`) に ResultArg を付ける。codegen は T に受けずに、get の
// フレームの戻り値を引数の場所へ写す (genCall / genPushArg)。兄弟の関数のフレームは重なりうるので、全部のバイトを読んでから
// 書く (A・Y・X とスタック。regalloc は印の付いた push_arg を Y を壊す命令と見る)。call と push_arg の間が push_result だけで
// ない (前の引数の push_arg がある: 呼び先のフレームに書くので戻り値を壊しうる) なら付けない。
func markResultArg(ops []*ir.Op, p *callPlans) {
	reads := map[*ir.Value]int{}
	for _, op := range ops {
		if op == nil {
			continue
		}
		op.ResultArg = false
		for _, s := range op.Src {
			if v := ir.UnderlyingValue(s); v != nil {
				reads[v]++
			}
		}
		for _, lp := range op.Logs {
			for _, a := range lp.Args {
				if v := ir.UnderlyingValue(a.Val); v != nil {
					reads[v] += 2 // @log が読む
				}
				for _, b := range a.Bytes {
					if v := ir.UnderlyingValue(b); v != nil {
						reads[v] += 2
					}
				}
			}
		}
	}
	for _, c := range p.list {
		call := c.call
		t, ok := call.Dst.(*ir.Value)
		if c.push.Code != ir.OpPushResult || c.kind != ckStatic || c.far || call.Code != ir.OpCall || !ok || t.Kind != ir.KindLocal ||
			t.LocalType != ir.LTTemp || t.Type.Size < 3 || reads[t] != 1 {
			continue
		}
		i := c.callAt
		j := i + 1
		for j < len(ops) && (ops[j] == nil || ops[j].Code == ir.OpPushResult) {
			j++
		}
		if j == i+1 || j >= len(ops) {
			continue // 次の呼び出しの引数でない
		}
		if a := ops[j]; a.Code == ir.OpPushArg && a.In(0) == ir.Operand(t) && a.Type.Size == t.Type.Size && !a.ArgY && !a.HoldY && !a.ArgCont {
			a.ResultArg = true
		}
	}
}

// resultArgAt は call の命令 opNo の戻り値を、一時変数に受けずに次の push_arg (ResultArg) が写すならその位置 (無ければ -1)。
// 間は push_result だけ (割付が命令を挟んだら写さない: 印は Y を壊すという見積もりとしてだけ残る)。
func resultArgAt(ops []*ir.Op, opNo int) int {
	op := ops[opNo]
	j := opNo + 1
	for j < len(ops) && (ops[j] == nil || ops[j].Code == ir.OpPushResult) {
		j++
	}
	if j == opNo+1 || j >= len(ops) || !ops[j].ResultArg || op.Dst == nil || ops[j].In(0) != op.Dst {
		return -1
	}
	return j
}

// copyResultArg は呼び先 src のフレームの戻り値 n バイトを dst(i) へ写す。src と dst は重なりうる (兄弟の関数のフレーム)
// ので、全部を読んでから書く: 0 バイト目は A、1 バイト目は Y、2 バイト目は X (useX のとき)、残りはスタック。
func copyResultArg(src *ir.Lambda, n int, dst func(i int) string, useX bool) []any {
	regs := map[int]string{1: "y"}
	if useX {
		regs[2] = "x"
	}
	var r []any
	var stacked []int
	for i := n - 1; i >= 1; i-- {
		if _, ok := regs[i]; !ok {
			r = append(r, "lda "+staticAddr(src, i), "pha")
			stacked = append(stacked, i)
		}
	}
	for i := 1; i < n; i++ {
		if reg, ok := regs[i]; ok {
			r = append(r, "ld"+reg+" "+staticAddr(src, i))
		}
	}
	r = append(r, "lda "+staticAddr(src, 0), "sta "+dst(0))
	for i := 1; i < n; i++ {
		if reg, ok := regs[i]; ok {
			r = append(r, "st"+reg+" "+dst(i))
		}
	}
	for k := len(stacked) - 1; k >= 0; k-- {
		r = append(r, "pla", "sta "+dst(stacked[k]))
	}
	return r
}

// markArgY は lmd の呼び出しのうち、最後から 2 つ目の引数を Y で渡せるもの (push_arg の ArgY) に印を付ける
// (最適化の後、割付の前。regalloc は印の付いた push_arg を Y を壊す命令と見て、Y の常駐をその前で書き戻す)。
// 条件: 呼び先が分かっていて static で Y で受け取る引数があり (CallConv.RegParam)、far でなく、最後の引数の push_arg の直後が call で、その 2 つの push_arg の
// 間の命令 (最後の引数の式) が Y を使わない (最後の引数の読み出しは変数か cast か定数)。
// markHoldX は stack 系の呼び出し (push_result で X = FC_SP にし、call までそのまま) の間の命令に HoldX を付ける。codegen は
// その間 X の常駐を退避してメモリ側で扱う (callPlans.holdX) ので、regalloc の見積もりも同じに見る
// (`load l0 = l0.lo@X` を stx と見積もって A を触らないとしたのに、codegen はメモリから A で写していた。fuzz で発覚)。
func markHoldX(ops []*ir.Op, p *callPlans) {
	for _, op := range ops {
		if op != nil {
			op.HoldX = false
		}
	}
	for _, c := range p.list {
		if c.kind != ckStack {
			continue
		}
		for k := c.pushAt + 1; k < c.callAt; k++ {
			if ops[k] != nil {
				ops[k].HoldX = true
			}
		}
	}
}

func markArgY(ops []*ir.Op, p *callPlans) {
	for _, c := range p.list {
		op := c.call
		if c.far || c.callee == nil || c.callee.Conv.ABI != ir.ABIStatic || c.callee.Conv.RegParam(ir.RegY) == nil {
			continue // far call はトランポリンが A / Y を壊す
		}
		var at []int // 引数ごとの push_arg の位置 (slice を分けた続きは数えない)
		for k, a := range c.args {
			if !a.ArgCont {
				at = append(at, c.argAt[k])
			}
		}
		np := len(c.callee.Type.Params)
		if len(at) != np {
			continue
		}
		ky, kl := at[np-2], at[np-1]
		py, pl := ops[ky], ops[kl]
		if !ir.IsPushArg(py) || !ir.IsPushArg(pl) || nextOp(ops, kl) != op {
			continue
		}
		if !isValueOrCasted(py.In(0)) || !isValueOrCasted(pl.In(0)) || py.Type.Size != 1 {
			continue
		}
		// 間の命令 (最後の引数の式の計算) は Y を使わないものだけ (push_arg をその下に沈めると、間の演算の結果が A に
		// 残らなくなって (sta t; ldy; lda t) 損: oam で +0.7%)
		ok := true
		for j := ky + 1; j < kl && ok; j++ {
			if ops[j] != nil && !keepsY(ops[j]) {
				ok = false
			}
		}
		if !ok {
			continue
		}
		py.ArgY = true
		for j := ky + 1; j <= c.callAt; j++ {
			if ops[j] != nil {
				ops[j].HoldY = true
			}
		}
	}
}

// keepsY は op の codegen が Y を使わないか (Y に呼び出しの引数を保持したまま実行できる。常駐変数は保持中はメモリ側で扱う
// ので、常駐の friendly な形 (iny / cpy / lda a,y) は考えなくてよい)。分岐・ラベル・呼び出し・添字・ポインタ・乗除算・
// 変数シフト・asm は使う。
func keepsY(op *ir.Op) bool {
	switch op.Code {
	case ir.OpLoad, ir.OpSignExtension, ir.OpAdd, ir.OpSub, ir.OpAnd, ir.OpOr, ir.OpXor, ir.OpRolC, ir.OpRorC,
		ir.OpUminus, ir.OpEq, ir.OpLt, ir.OpNot, ir.OpBitNot:
		for _, o := range op.Src {
			if !isValueOrCasted(o) {
				return false
			}
		}
		return true
	case ir.OpShiftLeft, ir.OpShiftRight:
		_, lit := ir.ValIntLiteral(op.Src[1])
		return lit && isValueOrCasted(op.Src[0])
	case ir.OpPushArg, ir.OpPushFastcallArg:
		return !op.ArgY && isValueOrCasted(op.Src[0])
	}
	return false
}

// loadY は 1 バイトの値を Y に読む (Y にある値なら何もしない。A にある値や融合した添字付きオペランドは A 経由で tay)。
func (l *Llc) loadY(v ir.Operand) []any {
	switch {
	case l.inY(v):
		return nil
	case l.inX(v):
		return []any{"txa", "tay"}
	case l.inA(v) || ir.ValLocation(v) == ir.LocCond:
		return []any{l.loadA(v, 0), "tay"}
	}
	if tv, ok := v.(*ir.Value); ok && l.fused != nil && l.fused[tv] != "" {
		return []any{l.loadA(v, 0), "tay"} // `tab+0,y` は ldy では読めない
	}
	return []any{fmt.Sprintf("ldy %s", l.byte(v, 0))}
}

// staticAddr は静的フレーム上のオフセット off のアドレス表記 (ゼロページなら `<F_g+off`)。
func staticAddr(lmd *ir.Lambda, off int) string {
	if lmd.FrameZp {
		return fmt.Sprintf("<%s+%d", lmd.FrameSym(), off)
	}
	return fmt.Sprintf("%s+%d", lmd.FrameSym(), off)
}

// CheckStackPush は、スタックに積む引数 (S+k,x の k = 基点 + 積んでいる途中の引数と戻り値) が FC_STACK に収まるか
// (k は ゼロページの番地の定数なので、超えると ca65 / ld65 の範囲エラーになる)。フレームだけの検査
// (regalloc の FC_STACK) では、stack 系の関数が大きいフレームの後ろに引数を積むときに漏れていた (inline 関数を
// 展開した再帰関数で `<S+128,x`。fuzz で発覚)。frame size over なので、-O 2 なら driver が展開を止めてやり直す。
func (l *Llc) CheckStackPush(lmd *ir.Lambda) {
	p := l.planCalls(lmd.Ops)
	p.layoutCalls(lmd.Ops, 0)
	if need := l.stackBase(lmd) + p.stackPeak; p.stackPeak > 0 && need > regalloc.StackSize {
		panic(&diag.Error{Msg: fmt.Sprintf("frame size over on %s: stack frame and call arguments need %d bytes but FC_STACK has %d (split the function or reduce locals)", lmd, need, regalloc.StackSize)})
	}
}

// stackBase は現在の関数がスタックに引数を積むときの基点 (stack 関数は自分のフレームの後ろ、static / entry は X の指す位置)。
func (l *Llc) stackBase(lmd *ir.Lambda) int {
	if lmd.Conv.ABI == ir.ABIStack {
		return lmd.FrameSize
	}
	return 0
}

// スタックの空き先頭はゼロページの FC_SP が持つ (Agent/wiki/design/regalloc.md §3 の「X の開放」)。X はレジスタとして自由に使える。
//   - static / entry 関数: X を使わない (使うのはループ内の常駐)。stack 系の呼び先には `ldx FC_SP` してから
//     S+k,x に引数を書いて jsr し、戻り値を読む前にもう一度 `ldx FC_SP` (呼び先が X を壊しうる)
//   - stack (再帰) 関数: X = 自分のフレームの底。入口で FC_SP = X + FrameSize、return で戻す。呼び出しの後は
//     X を FC_SP - FrameSize から戻す (呼び先が X を壊しうる)

// callStackish は S+k,x に引数を積む呼び先 (stack / entry / extern) を呼ぶ。
func (l *Llc) callStackish(lmd *ir.Lambda, addr string) []any {
	r := []any{"ldx FC_SP", fmt.Sprintf("jsr %s", addr)}
	return append(r, l.restoreX(lmd)...)
}

// callStatic は静的フレームの呼び先を呼ぶ (引数はフレームに書いてある)。
func (l *Llc) callStatic(lmd *ir.Lambda, addr string) []any {
	return append([]any{fmt.Sprintf("jsr %s", addr)}, l.restoreX(lmd)...)
}

// restoreX は stack 関数が呼び出しの後で X (フレームの底) を FC_SP から戻す。static 関数は何もしない。
func (l *Llc) restoreX(lmd *ir.Lambda) []any {
	if lmd.Conv.ABI != ir.ABIStack || lmd.FrameSize == 0 {
		if lmd.Conv.ABI == ir.ABIStack {
			return []any{"ldx FC_SP"}
		}
		return nil
	}
	return []any{"lda FC_SP", "sec", fmt.Sprintf("sbc #%d", lmd.FrameSize), "tax"}
}

// loadSP は static 関数が stack 系の呼び先の引数 / 戻り値を S+k,x で触る前に X をスタックの空き先頭にする。
func (l *Llc) loadSP(lmd *ir.Lambda) any {
	if lmd.Conv.ABI == ir.ABIStack {
		return nil // 自分の X (フレームの底) からの相対で書く
	}
	return "ldx FC_SP"
}
