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
	ckStack       callKind = iota // S+k,x に積む (stack / entry / 関数ポインタ経由)
	ckStatic                      // 呼び先の静的フレーム F_g+k に直接書く
	ckFastcallReg                 // FC_FASTCALL_REG に積む (extern の fastcall)
)

// pendingCall は push_result から call までの 1 つの呼び出し。
type pendingCall struct {
	callee *ir.Lambda // 分かっているとき
	kind   callKind
	argOff int // ckStatic: 次の引数バイトのフレーム内オフセット
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
	pc := &pendingCall{kind: ckStack}
	if ops[i].Code == ir.OpPushFastcallResult {
		pc.kind = ckFastcallReg
	}
	if v := ir.ValLiteral(callOp.Src[0]); v != nil && v.Kind == ir.KindLiteral && v.Symbol != "" {
		if callee, ok := l.Lambdas[v.Symbol]; ok {
			pc.callee = callee
			switch {
			case callee.ABI == ir.ABIStatic && !callee.Entry:
				pc.kind = ckStatic
				pc.argOff = callee.Type.Base.Size
			case callee.ABI == ir.ABIFastcall:
				pc.kind = ckFastcallReg
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

// jsrOrCall は呼び出し側の種類に応じて jsr するか X を進める call マクロを使うか。
func (l *Llc) jsrOrCall(lmd *ir.Lambda, addr string, frameSize int) any {
	if lmd.ABI == ir.ABIStack {
		return l.callSubroutine(addr, frameSize)
	}
	return fmt.Sprintf("jsr %s", addr)
}

// callSubroutine はスタックポインタ(X)を進めて jsr する。
func (l *Llc) callSubroutine(addr string, frameSize int) any {
	if frameSize <= 4 {
		r := []string{}
		for i := 0; i < frameSize; i++ {
			r = append(r, "inx")
		}
		r = append(r, fmt.Sprintf("jsr %s", addr))
		for i := 0; i < frameSize; i++ {
			r = append(r, "dex")
		}
		return r
	}
	return fmt.Sprintf("call %s, #%d", addr, frameSize)
}
