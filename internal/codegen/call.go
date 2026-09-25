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

// callKind は呼び出し 1 つの引数の渡し方 (呼び先の種類で決まる。doc/v2_frame_alloc.md §6-1)。
type callKind uint8

const (
	ckStack  callKind = iota // S+k,x に積む (stack / entry / 関数ポインタ経由)
	ckStatic                 // 呼び先の静的フレーム F_g+k に直接書く
	ckFastcallReg
	ckCc65 // cc65 の __fastcall__: 引数は FC_FASTCALL_REG に置いてから A / X に、戻り値は A / X (doc/language_reference.md §4.5)                 // FC_FASTCALL_REG に積む (extern の fastcall)
)

// pendingCall は push_result から call までの 1 つの呼び出し。
type pendingCall struct {
	callee *ir.Lambda // 分かっているとき
	kind   callKind
	argOff int    // ckStatic: 次の引数バイトのフレーム内オフセット
	callOp *ir.Op // 対応する call
	far    bool   // far call (トランポリンが A を壊すのでレジスタ渡しは使えない)
	inA    bool   // ckStatic: 最後の引数を A に置いた (フレームには書いていない)
	inY    bool   // ckStatic: 最後から 2 つ目の引数を Y に置いた (push_arg の ArgY)
}

// directSym は Entry 関数をプロローグ (スタックからの引数コピー) を飛ばして直接呼ぶときの入口シンボル
// (RegArg / RegArgY なら最後の引数を A / その前を Y に置いて入る)。
func directSym(sym string) string { return sym + "__direct" }

// frameSym は RegArg / RegArgY の関数を、レジスタ渡しの引数もフレームに書いてから呼ぶときの入口 (入口の `sty` / `sta` の後ろ)。
func frameSym(sym string) string { return sym + "__frame" }

// aSym は RegArg と RegArgY の両方ある関数を、Y の引数はフレームに書き、A の引数だけ A に置いて呼ぶときの入口
// (`sty` の後ろ、`sta` の前)。
func aSym(sym string) string { return sym + "__a" }

// markArgY は lmd の呼び出しのうち、最後から 2 つ目の引数を Y で渡せるもの (push_arg の ArgY) に印を付ける
// (最適化の後、割付の前。regalloc は印の付いた push_arg を Y を壊す命令と見て、Y の常駐をその前で書き戻す)。
// 条件: 呼び先が分かっていて static で RegArgY、far でなく、最後の引数の push_arg の直後が call で、その 2 つの push_arg の
// 間の命令 (最後の引数の式) が Y を使わない (最後の引数の読み出しは変数か cast か定数)。
func markArgY(lmd *ir.Lambda, lambdas map[string]*ir.Lambda) {
	type pending struct {
		callOp *ir.Op
		args   []int
	}
	var stack []*pending
	ops := lmd.Ops
	for i, op := range ops {
		if op == nil {
			continue
		}
		switch op.Code {
		case ir.OpPushResult, ir.OpPushFastcallResult:
			stack = append(stack, &pending{})
		case ir.OpPushArg, ir.OpPushFastcallArg:
			if len(stack) > 0 {
				p := stack[len(stack)-1]
				p.args = append(p.args, i)
			}
		case ir.OpCall, ir.OpFastcall:
			if len(stack) == 0 {
				continue
			}
			p := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if op.Far {
				continue // far call はトランポリンが A / Y を壊す
			}
			v := ir.ValLiteral(op.Src[0])
			if v == nil || v.Kind != ir.KindLiteral || v.Symbol == "" {
				continue
			}
			callee, ok := lambdas[v.Symbol]
			if !ok || callee.ABI != ir.ABIStatic || !callee.RegArgY {
				continue
			}
			np := len(callee.Type.Params)
			if len(p.args) != np {
				continue
			}
			iy, il := p.args[np-2], p.args[np-1]
			py, pl := ops[iy], ops[il]
			if !isPushArg(py) || !isPushArg(pl) || nextOp(ops, il) != op {
				continue
			}
			if !isValueOrCasted(py.In(0)) || !isValueOrCasted(pl.In(0)) || py.Type.Size != 1 {
				continue
			}
			// 間の命令 (最後の引数の式の計算) は Y を使わないものだけ (push_arg をその下に沈めると、間の演算の結果が A に
			// 残らなくなって (sta t; ldy; lda t) 損: oam で +0.7%)
			ok = true
			for j := iy + 1; j < il && ok; j++ {
				if ops[j] != nil && !keepsY(ops[j]) {
					ok = false
				}
			}
			if !ok {
				continue
			}
			py.ArgY = true
			for j := iy + 1; j <= i; j++ {
				if ops[j] != nil {
					ops[j].HoldY = true
				}
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

// isPushArg は push_arg (static の呼び先なら fastcall の印の付いた関数の呼び出しも同じ形)。
func isPushArg(op *ir.Op) bool { return op.Code == ir.OpPushArg || op.Code == ir.OpPushFastcallArg }

// isCallOp は call / fastcall。
func isCallOp(op *ir.Op) bool { return op.Code == ir.OpCall || op.Code == ir.OpFastcall }

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

// resolveCall は push_result (添字 i) に対応する call を探して、呼び出しの種類を決める。
func (l *Llc) resolveCall(ops []*ir.Op, i int) *pendingCall {
	depth := 0
	var callOp *ir.Op
	for j := i; j < len(ops) && callOp == nil; j++ {
		op := ops[j]
		if op == nil {
			continue
		}
		switch op.Code {
		case ir.OpPushResult, ir.OpPushFastcallResult:
			depth++
		case ir.OpCall, ir.OpFastcall:
			depth--
			if depth == 0 {
				callOp = op
			}
		}
	}
	if callOp == nil {
		panic(&diag.Error{Msg: "push_result without call"})
	}
	pc := &pendingCall{kind: ckStack, callOp: callOp, far: callOp.Far}
	// Even a known farfn target uses its ordinary stack/Entry entry point.
	if ir.ValType(callOp.Src[0]).IsFarFunc() {
		pc.far = true
		return pc
	}
	if ops[i].Code == ir.OpPushFastcallResult {
		pc.kind = ckFastcallReg
	}
	if v := ir.ValLiteral(callOp.Src[0]); v != nil && v.Kind == ir.KindLiteral && v.Symbol != "" {
		if callee, ok := l.Lambdas[v.Symbol]; ok {
			pc.callee = callee
			switch {
			case callee.ABI == ir.ABIStatic:
				// Entry でも呼び先が分かっていればフレームに直接書き、プロローグの後ろ (__direct) から入る
				pc.kind = ckStatic
				pc.argOff = callee.Type.Base.Size
			case callee.ABI == ir.ABIFastcall:
				pc.kind = ckFastcallReg
			case callee.ABI == ir.ABICc65:
				pc.kind = ckCc65
			default:
				pc.kind = ckStack
			}
		}
	}
	return pc
}

// staticAddr は静的フレーム上のオフセット off のアドレス表記 (ゼロページなら `<F_g+off`)。
func staticAddr(lmd *ir.Lambda, off int) string {
	if lmd.FrameZp {
		return fmt.Sprintf("<%s+%d", lmd.FrameSym(), off)
	}
	return fmt.Sprintf("%s+%d", lmd.FrameSym(), off)
}

// checkStackPush は、スタックに積む引数 (S+k,x の k = 基点 + 積んでいる途中の引数と戻り値) が FC_STACK に収まるか
// (k は ゼロページの番地の定数なので、超えると ca65 / ld65 の範囲エラーになる)。フレームだけの検査
// (regalloc の FC_STACK) では、stack 系の関数が大きいフレームの後ろに引数を積むときに漏れていた (inline 関数を
// 展開した再帰関数で `<S+128,x`。fuzz で発覚)。frame size over なので、-O 2 なら driver が展開を止めてやり直す。
func (l *Llc) checkStackPush(lmd *ir.Lambda) {
	ops := lmd.Ops
	var pending []*pendingCall
	var marks []int
	cur, peak := 0, 0
	for i, op := range ops {
		if op == nil {
			continue
		}
		switch op.Code {
		case ir.OpPushResult, ir.OpPushFastcallResult:
			pc := l.resolveCall(ops, i)
			pending = append(pending, pc)
			marks = append(marks, cur)
			if pc.kind == ckStack {
				cur += op.Type.Size
			}
		case ir.OpPushArg, ir.OpPushFastcallArg:
			if len(pending) > 0 && pending[len(pending)-1].kind == ckStack {
				cur += op.Type.Size
			}
		case ir.OpCall, ir.OpFastcall:
			if len(pending) > 0 {
				cur = marks[len(marks)-1]
				pending, marks = pending[:len(pending)-1], marks[:len(marks)-1]
			}
		}
		peak = max(peak, cur)
	}
	if need := l.stackBase(lmd) + peak; peak > 0 && need > regalloc.StackSize {
		panic(&diag.Error{Msg: fmt.Sprintf("frame size over on %s: stack frame and call arguments need %d bytes but FC_STACK has %d (split the function or reduce locals)", lmd, need, regalloc.StackSize)})
	}
}

// stackBase は現在の関数がスタックに引数を積むときの基点 (stack 関数は自分のフレームの後ろ、static / entry は X の指す位置)。
func (l *Llc) stackBase(lmd *ir.Lambda) int {
	if lmd.ABI == ir.ABIStack {
		return lmd.FrameSize
	}
	return 0
}

// スタックの空き先頭はゼロページの FC_SP が持つ (doc/v2_regalloc.md §4)。X はレジスタとして自由に使える。
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
	if lmd.ABI != ir.ABIStack || lmd.FrameSize == 0 {
		if lmd.ABI == ir.ABIStack {
			return []any{"ldx FC_SP"}
		}
		return nil
	}
	return []any{"lda FC_SP", "sec", fmt.Sprintf("sbc #%d", lmd.FrameSize), "tax"}
}

// loadSP は static 関数が stack 系の呼び先の引数 / 戻り値を S+k,x で触る前に X をスタックの空き先頭にする。
func (l *Llc) loadSP(lmd *ir.Lambda) any {
	if lmd.ABI == ir.ABIStack {
		return nil // 自分の X (フレームの底) からの相対で書く
	}
	return "ldx FC_SP"
}
