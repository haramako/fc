package codegen

import (
	"fmt"

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/ir"
)

// 呼び出しの計画: 関数の中の呼び出し (push_result から call まで) ごとに、呼び先・渡し方 (callKind)・引数の push_arg を
// 1 回で組にする (planCalls)。割付の前の印 (markArgY / markHoldX / markResultArg)、スタックの大きさの検査
// (CheckStackPush)、コード生成 (layoutCalls: 引数ごとの置き場所・レジスタ渡し・入口・X の保持) が同じ計画を見る。
// 呼び先の側の規約 (引数の置き場所・受け取るレジスタ・入口) は ir.CallConv (frames.Analyze が作る)。

// callKind は呼び出し 1 つの引数の渡し方 (呼び先の種類で決まる。Agent/wiki/design/frame-alloc.md §6-1)。
type callKind uint8

const (
	ckStack       callKind = iota // S+k,x に積む (stack / entry / 関数ポインタ経由)
	ckStatic                      // 呼び先の静的フレーム F_g+k に直接書く
	ckFastcallReg                 // FC_FASTCALL_REG に積む (extern の fastcall)
	ckCc65                        // cc65 の __fastcall__: 引数は FC_FASTCALL_REG に置いてから A / X に、戻り値は A / X (docs/reference/assembly.md の「cc65」)
)

// callPlan は呼び出し 1 つ (push_result から call まで)。
type callPlan struct {
	push, call     *ir.Op
	pushAt, callAt int        // 命令列の中の位置 (計画を作ったとき)
	callee         *ir.Lambda // 呼び先が分かっているとき (関数のシンボルを直接呼ぶ)
	kind           callKind
	far            bool     // far call・far な関数ポインタ (トランポリンが A / Y を壊すのでレジスタ渡しは使えない)
	args           []*ir.Op // push_arg (並びの順。slice を分けた続き ArgCont も含む)
	argAt          []int    // args の位置

	// コード生成の段で決めるもの (layoutCalls)
	inA, inY  bool // static: 最後の引数を A に / 最後から 2 つ目の引数を Y に置いて呼ぶ (入口は ir.CallConv.DirectEntry)
	resultOff int  // ckStack: 戻り値の S+k,x の k
}

// argPlan は push_arg 1 つの置き場所 (コード生成の段。layoutCalls)。
type argPlan struct {
	call *callPlan
	off  int    // ckStatic: 呼び先のフレームの中、ckStack: S+k,x の k、ckFastcallReg: FC_FASTCALL_REG の中
	reg  ir.Reg // RegA / RegY: レジスタに置いて呼ぶ (フレームには書かない)。NoReg: メモリ
	// from は直前の呼び出しの戻り値を、一時変数に受けずに呼び先のフレームから写す (markResultArg の ResultArg)
	from *callPlan
}

// callPlans は関数 1 つの呼び出しの計画。
type callPlans struct {
	list []*callPlan
	byOp map[*ir.Op]*callPlan // push_result・push_arg・call → 呼び出し
	// コード生成の段 (layoutCalls)
	args      map[*ir.Op]*argPlan
	holdX     map[*ir.Op]int // 命令の入口で開いている stack 系の呼び出しの数 (push_result の ldx FC_SP から call まで X = FC_SP)
	stackPeak int            // stack 系の呼び出しに積む戻り値と引数の最大 (stackBase から。CheckStackPush)
}

// planCalls は ops の呼び出しを組にして、呼び先と渡し方を決める。
func (l *Llc) planCalls(ops []*ir.Op) *callPlans {
	p := &callPlans{byOp: map[*ir.Op]*callPlan{}}
	var open []*callPlan
	for i, op := range ops {
		if op == nil {
			continue
		}
		switch op.Code {
		case ir.OpPushResult, ir.OpPushFastcallResult:
			c := &callPlan{push: op, pushAt: i}
			p.list = append(p.list, c)
			p.byOp[op] = c
			open = append(open, c)
		case ir.OpPushArg, ir.OpPushFastcallArg:
			if len(open) > 0 {
				c := open[len(open)-1]
				c.args, c.argAt = append(c.args, op), append(c.argAt, i)
				p.byOp[op] = c
			}
		case ir.OpCall, ir.OpFastcall:
			if len(open) == 0 {
				continue
			}
			c := open[len(open)-1]
			open = open[:len(open)-1]
			c.call, c.callAt = op, i
			p.byOp[op] = c
			l.resolveCall(c)
		}
	}
	if len(open) > 0 {
		panic(&diag.Error{Msg: "push_result without call"})
	}
	return p
}

// resolveCall は呼び出しの呼び先と渡し方を決める。
func (l *Llc) resolveCall(c *callPlan) {
	c.kind, c.far = ckStack, c.call.Far
	// Even a known farfn target uses its ordinary stack/Entry entry point.
	if ir.ValType(c.call.Src[0]).IsFarFunc() {
		c.far = true
		return
	}
	if c.push.Code == ir.OpPushFastcallResult {
		c.kind = ckFastcallReg
	}
	if v := ir.ValLiteral(c.call.Src[0]); v != nil && v.Kind == ir.KindLiteral && v.Symbol != "" {
		if callee, ok := l.Lambdas[v.Symbol]; ok {
			c.callee = callee
			switch callee.Conv.ABI {
			case ir.ABIStatic:
				// スタックの入口があっても、呼び先が分かっていればフレームに直接書き、プロローグの後ろ (__direct) から入る
				c.kind = ckStatic
			case ir.ABIFastcall:
				c.kind = ckFastcallReg
			case ir.ABICc65:
				c.kind = ckCc65
			default:
				c.kind = ckStack
			}
		}
	}
}

// regParam は static の呼び先がレジスタ r で受け取る引数 (far call はトランポリンが A / Y を壊すので無し)。
func (c *callPlan) regParam(r ir.Reg) *ir.ArgLoc {
	if c.kind != ckStatic || c.far {
		return nil
	}
	return c.callee.Conv.RegParam(r)
}

// layoutCalls はコード生成の段 (割付の後。命令の並びはもう変わらない) で、引数ごとの置き場所とレジスタ渡し、stack 系の
// 呼び出しの X の保持を決める。stack 系の引数は入れ子の呼び出しの分も続けて積む (S+k,x の k は stackBase からの通し)。
func (p *callPlans) layoutCalls(ops []*ir.Op, stackBase int) {
	p.args = map[*ir.Op]*argPlan{}
	p.holdX = map[*ir.Op]int{}
	stack, fastcall, holdX := 0, 0, 0 // 積んだバイト数 (stack 系・fastcall)、開いている stack 系の呼び出しの数
	var resultFrom *callPlan          // 戻り値を呼び先のフレームに置いたままの呼び出し (次の push_arg が写す)
	argOff := map[*callPlan]int{}     // static: 次の引数のバイトの位置
	for i, op := range ops {
		if op == nil {
			continue
		}
		p.holdX[op] = holdX
		c := p.byOp[op]
		switch op.Code {
		case ir.OpPushResult, ir.OpPushFastcallResult:
			switch c.kind {
			case ckStack:
				stack += op.Type.Size
				p.stackPeak = max(p.stackPeak, stack)
				holdX++
			case ckFastcallReg:
				fastcall += op.Type.Size
			case ckStatic:
				argOff[c] = c.callee.Type.Base.Size
			}
		case ir.OpPushArg, ir.OpPushFastcallArg:
			if c == nil {
				continue
			}
			a := &argPlan{call: c, reg: ir.NoReg}
			p.args[op] = a
			n := op.Type.Size
			if f := resultFrom; f != nil && op.In(0) == f.call.Dst {
				a.from, resultFrom = f, nil
			}
			switch c.kind {
			case ckStatic:
				a.off = argOff[c]
				argOff[c] += n
				if y := c.regParam(ir.RegY); a.from == nil && op.ArgY && y != nil && a.off == y.Off {
					// 最後から 2 つ目の引数は Y に置いて呼ぶ (markArgY: 直後が最後の引数の push_arg、その直後が call)
					a.reg, c.inY = ir.RegY, true
				} else if r := c.regParam(ir.RegA); a.from == nil && r != nil && a.off+n-1 == r.Off && nextOp(ops, i) == c.call {
					a.reg, c.inA = ir.RegA, true // 最後の引数は A のまま呼ぶ (直後が call のときだけ)
				}
			case ckStack:
				a.off = stack
				stack += n
				p.stackPeak = max(p.stackPeak, stack)
			case ckFastcallReg:
				a.off = fastcall
				fastcall += n
			case ckCc65:
				if a.from != nil {
					panic(fmt.Sprintf("result argument for call kind %d", c.kind))
				}
			}
		case ir.OpCall, ir.OpFastcall:
			if c == nil {
				continue
			}
			switch c.kind {
			case ckStack:
				fn := ir.ValType(op.In(0))
				for _, a := range fn.Params {
					stack -= a.Size
				}
				stack -= fn.Base.Size
				c.resultOff = stack
				if holdX > 0 {
					holdX--
				}
			case ckFastcallReg:
				fastcall = 0
			case ckStatic:
				if op.Dst != nil && !op.Far && resultArgAt(ops, i) >= 0 {
					resultFrom = c // 次の push_arg が呼び先のフレームから写す
				}
			}
		}
	}
	for _, c := range p.list {
		c.resultOff += stackBase
		for _, op := range c.args {
			if a := p.args[op]; a != nil && c.kind == ckStack {
				a.off += stackBase
			}
		}
	}
}
