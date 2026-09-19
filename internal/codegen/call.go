package codegen

import (
	"fmt"

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/ir"
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
}

// directSym は Entry 関数をプロローグ (スタックからの引数コピー) を飛ばして直接呼ぶときの入口シンボル
// (RegArg なら最後の引数を A に置いて入る)。
func directSym(sym string) string { return sym + "__direct" }

// frameSym は RegArg の関数を、最後の引数もフレームに書いてから呼ぶときの入口 (入口の `sta` の後ろ)。
func frameSym(sym string) string { return sym + "__frame" }

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
