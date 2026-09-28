package ir

// IR の検証器: 命令列が守るべき形を確かめる (調査用。FC_VERIFY_IR=1 で opt の各段・常駐・割付の後に走る。テストと fuzz では
// 常に有効)。以前は「一時変数は定義 1 つ」「ラベルは関数の中にある」「オペランドの数」のような前提がコメントにしかなく、
// 破れは fuzz の差分 (出力の食い違い) か codegen の panic として遠くで見つかっていた。

import (
	"fmt"
	"strings"

	"github.com/haramako/fc/internal/types"
)

// srcCount は命令が持つ Src の数 (-1 なら 1 以上、-2 なら 0 か 1)。
var srcCount = [opCodeCount]int8{
	OpLabel: 0, OpJump: 0, OpAsm: 0, OpPushResult: 0, OpPushFastcallResult: 0, OpIfCarry: 0, OpIfNotCarry: 0,
	OpIf: -1, OpIfTrue: -1, OpPushArg: -1, OpPushFastcallArg: -1, OpCall: -1, OpFastcall: -1,
	OpSwitch: 2, OpReturn: -2,
	OpLoad: 1, OpUminus: 1, OpNot: 1, OpBitNot: 1, OpSignExtension: 1, OpRef: 1, OpRolC: 1, OpRorC: 1,
	OpAdd: 2, OpSub: 2, OpAnd: 2, OpOr: 2, OpXor: 2, OpMul: 2, OpDiv: 2, OpMod: 2, OpEq: 2, OpLt: 2,
	OpShiftLeft: 2, OpShiftRight: 2, OpIndex: 2, OpLoadMem: 2, OpStoreMem: 3,
}

// Verify は lmd の命令列を検査し、最初に見つけた問題を返す (無ければ nil)。
//   - 命令の種類が有効で、Src の数が種類に合い、オペランドが nil でない
//   - 副作用の無い命令は Dst を持ち、結果を持たない命令 (分岐・store・push・asm) は Dst を持たない
//   - ラベルは関数の中で一意で、分岐・switch の飛び先は関数の中にある
//   - push_result / push_arg は Type を持つ
//
// 一時変数の定義が 1 つ、という前提は置いていない (sema の `||` / `&&` は両方の枝で同じ一時変数に書き、常駐の一時変数はループで更新する)。
func Verify(lmd *Lambda) error {
	labels := map[string]int{}
	for i, op := range lmd.Ops {
		if op != nil && op.Code == OpLabel {
			if j, dup := labels[op.Label]; dup {
				return fmt.Errorf("op %d: label %s is also defined at op %d", i, op.Label, j)
			}
			labels[op.Label] = i
		}
	}
	for i, op := range lmd.Ops {
		if op == nil {
			continue
		}
		if op.Code <= opInvalid || op.Code >= opCodeCount {
			return fmt.Errorf("op %d: invalid opcode %d", i, op.Code)
		}
		if err := verifyOp(op, labels); err != nil {
			return fmt.Errorf("op %d (%s): %v", i, strings.TrimSpace(DumpOp(op, nil)), err)
		}
	}
	return nil
}

func verifyOp(op *Op, labels map[string]int) error {
	switch n := srcCount[op.Code]; {
	case n >= 0 && len(op.Src) != int(n):
		return fmt.Errorf("expected %d operands, got %d", n, len(op.Src))
	case n == -1 && len(op.Src) < 1:
		return fmt.Errorf("expected an operand")
	case n == -2 && len(op.Src) > 1:
		return fmt.Errorf("expected at most 1 operand, got %d", len(op.Src))
	}
	for k, s := range op.Src {
		if err := verifyOperand(s); err != nil {
			return fmt.Errorf("operand %d: %v", k, err)
		}
	}
	if op.Dst != nil {
		if err := verifyOperand(op.Dst); err != nil {
			return fmt.Errorf("dst: %v", err)
		}
	}
	switch {
	case op.Code.IsPure() && op.Dst == nil:
		return fmt.Errorf("no dst")
	case (op.Code.IsTerminator() || op.Code.IsPushArg() || op.Code.IsPushResult() || op.Code == OpLabel || op.Code == OpAsm ||
		op.Code == OpStoreMem) && op.Dst != nil:
		return fmt.Errorf("unexpected dst")
	}
	if op.IsMem() {
		m := op.Mem()
		if m.Width <= 0 {
			return fmt.Errorf("memory access without a width")
		}
		if m.Index != nil && (m.Scale <= 0 || ValType(m.Index).Size != 1) {
			return fmt.Errorf("memory index must be 1 byte with a positive scale (scale %d)", m.Scale)
		}
		if m.Index != nil && m.Scale > 2 {
			if _, lit := ValIntLiteral(m.Index); !lit {
				return fmt.Errorf("memory index scale must be 1 or 2 (got %d; codegen scales with asl)", m.Scale)
			}
		}
		if m.Disp < 0 {
			return fmt.Errorf("negative memory displacement %d", m.Disp)
		}
		if bt := ValType(m.Base); !m.BaseIsArray() && (bt.Kind != types.Pointer || bt.Size != 2) {
			return fmt.Errorf("memory base must be a global array or a 2-byte pointer (got %s)", bt)
		}
		if !m.BaseIsArray() && m.Disp+m.Width > 256 {
			// (p),y: ずれと幅は Y に収まる (添字があれば、添字 * scale + ずれ + 幅 <= 256 を作る側が保証する)
			return fmt.Errorf("displacement %d + width %d through a pointer does not fit in Y", m.Disp, m.Width)
		}
	}
	// 幅と符号 (sign.go)
	switch {
	case op.Code.IsCompare() && op.Width <= 0:
		return fmt.Errorf("comparison without a width")
	case op.Code.HasSign() && op.Sign == SignNone:
		return fmt.Errorf("%s without a sign", op.Code)
	case !op.Code.HasSign() && op.Sign != SignNone:
		return fmt.Errorf("sign on %s", op.Code)
	case op.Width != 0 && !op.Code.IsCompare() && op.Code != OpStoreMem:
		return fmt.Errorf("width on %s", op.Code)
	}
	if op.Code.IsBranch() {
		if _, ok := labels[op.Label]; !ok {
			return fmt.Errorf("label %s is not in the function", op.Label)
		}
	}
	if op.Code == OpSwitch {
		for _, l := range op.Labels {
			if _, ok := labels[l]; !ok {
				return fmt.Errorf("label %s is not in the function", l)
			}
		}
	}
	if (op.Code.IsPushArg() || op.Code.IsPushResult()) && op.Type == nil {
		return fmt.Errorf("no type")
	}
	return nil
}

func verifyOperand(o Operand) error {
	switch x := o.(type) {
	case nil:
		return fmt.Errorf("nil")
	case *Value:
		if x.Type == nil {
			return fmt.Errorf("%s has no type", x.Name)
		}
	case *CastedValue:
		if x.From == nil {
			return fmt.Errorf("cast without a value")
		}
		if x.Type == nil {
			return fmt.Errorf("cast without a type")
		}
		return verifyOperand(x.From)
	case *PointeredArray:
		if x.From == nil {
			return fmt.Errorf("pointered array without a value")
		}
		return verifyOperand(x.From)
	default:
		return fmt.Errorf("unknown operand %T", o)
	}
	return nil
}
