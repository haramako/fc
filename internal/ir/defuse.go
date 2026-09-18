package ir

// 命令のオペランドの役割 (定義 / 使用) と制御フロー。regalloc と opt (最適化) が共通に使う。

import "fmt"

// DefUse は op が定義する値と使う値を返す (OpCode ごとの表)。
// 戻り値の要素は Op のフィールドそのもの (CastedValue / PointeredArray のまま。元の変数は UnderlyingValue で辿る)。
// Dst が nil の命令 (戻り値を捨てた call など) は defs に含めない。
func DefUse(op *Op) (defs, uses []Operand) {
	switch op.Code {
	case OpLabel, OpJump, OpAsm, OpPushResult, OpPushFastcallResult, OpIfCarry, OpIfNotCarry:
	case OpIf, OpIfTrue, OpPushArg, OpPushFastcallArg:
		uses = op.Src[:1]
	case OpSwitch:
		uses = op.Src
	case OpReturn:
		if len(op.Src) > 0 {
			uses = op.Src[:1]
		}
	case OpLoad, OpUminus, OpNot, OpBitNot, OpSignExtension, OpRef, OpCall, OpFastcall, OpRolC, OpRorC:
		if op.Dst != nil {
			defs = []Operand{op.Dst}
		}
		uses = op.Src[:1]
	case OpAdd, OpSub, OpAnd, OpOr, OpXor, OpMul, OpDiv, OpMod, OpEq, OpLt,
		OpShiftLeft, OpShiftRight, OpIndex, OpPget, OpIndexPget, OpFieldPget:
		defs = []Operand{op.Dst}
		uses = op.Src
	case OpPset, OpIndexPset, OpFieldPset:
		uses = op.Src
	default:
		panic(fmt.Sprintf("DefUse: invalid op %v", DumpOp(op, nil)))
	}
	return
}

// IsPartialDef は Dst が変数の一部 (CastedValue の Offset つき、または元より小さい型) への書き込みか。
// 一部への書き込みは残りのバイトを保つので、値の使用でもある。
func IsPartialDef(v Operand) bool {
	cv, ok := v.(*CastedValue)
	if !ok {
		return false
	}
	uv := UnderlyingValue(cv)
	return uv != nil && (ValOffset(cv) != 0 || cv.Type.Size < uv.Type.Size)
}

// Block は基本ブロック: 先頭 (ラベルか、分岐・ジャンプ・return の直後) から次の先頭の手前まで。
type Block struct {
	Index int    // CFG.Blocks の添字 (命令の並び順)
	Label string // 先頭が OpLabel ならそのラベル。"" なら前のブロックからの流れ込みだけ
	Start int    // lmd.Ops の添字 (この範囲の nil は削除済みの命令)
	End   int    // 終端の添字 + 1
	Succs []*Block
	Preds []*Block
}

// CFG は関数 1 つの制御フローグラフ。命令列 (lmd.Ops) 自体は変えない。
type CFG struct {
	Lambda  *Lambda
	Blocks  []*Block
	byLabel map[string]*Block
}

// BuildCFG は命令列からブロックと辺を作る。OpIf は「条件が 0 なら Label へ」なので飛び先と直後の両方が後続。
// OpAsm は分岐しないものとみなす。Ops の nil 要素は飛ばす。
func BuildCFG(lmd *Lambda) *CFG {
	ops := lmd.Ops
	leader := make([]bool, len(ops)+1)
	leader[0] = true
	for i, op := range ops {
		if op == nil {
			continue
		}
		switch op.Code {
		case OpLabel:
			leader[i] = true
		case OpIf, OpIfTrue, OpIfCarry, OpIfNotCarry, OpJump, OpReturn, OpSwitch:
			leader[i+1] = true
		}
	}
	c := &CFG{Lambda: lmd, byLabel: map[string]*Block{}}
	var cur *Block
	for i, op := range ops {
		if leader[i] || cur == nil {
			if cur != nil {
				cur.End = i
			}
			cur = &Block{Index: len(c.Blocks), Start: i}
			c.Blocks = append(c.Blocks, cur)
		}
		if op != nil && op.Code == OpLabel {
			cur.Label = op.Label
			c.byLabel[op.Label] = cur
		}
	}
	if cur != nil {
		cur.End = len(ops)
	}
	for bi, b := range c.Blocks {
		last := b.lastOp(ops)
		next := func() *Block {
			if bi+1 < len(c.Blocks) {
				return c.Blocks[bi+1]
			}
			return nil
		}
		var succs []*Block
		switch {
		case last == nil:
			succs = []*Block{next()}
		case last.Code == OpJump:
			succs = []*Block{c.byLabel[last.Label]}
		case last.Code == OpIf || last.Code == OpIfTrue || last.Code == OpIfCarry || last.Code == OpIfNotCarry:
			succs = []*Block{next(), c.byLabel[last.Label]}
		case last.Code == OpSwitch:
			succs = []*Block{next()}
			seen := map[string]bool{}
			for _, l := range last.Labels {
				if !seen[l] {
					seen[l] = true
					succs = append(succs, c.byLabel[l])
				}
			}
		case last.Code == OpReturn:
		default:
			succs = []*Block{next()}
		}
		for _, s := range succs {
			if s == nil {
				continue
			}
			b.Succs = append(b.Succs, s)
			s.Preds = append(s.Preds, b)
		}
	}
	return c
}

// lastOp はブロックの末尾の (nil でない) 命令。
func (b *Block) lastOp(ops []*Op) *Op {
	for i := b.End - 1; i >= b.Start; i-- {
		if ops[i] != nil {
			return ops[i]
		}
	}
	return nil
}

// Last はブロックの末尾の命令 (無ければ nil)。
func (c *CFG) Last(b *Block) *Op { return b.lastOp(c.Lambda.Ops) }

// BlockOf はラベルのブロック。
func (c *CFG) BlockOf(label string) *Block { return c.byLabel[label] }

// Ops はブロックの命令 (nil を除く) の添字を順に返す。
func (c *CFG) Ops(b *Block) []int {
	var r []int
	for i := b.Start; i < b.End; i++ {
		if c.Lambda.Ops[i] != nil {
			r = append(r, i)
		}
	}
	return r
}

// Reachable は入口から到達できるブロックの集合。
func (c *CFG) Reachable() map[*Block]bool {
	seen := map[*Block]bool{}
	if len(c.Blocks) == 0 {
		return seen
	}
	stack := []*Block{c.Blocks[0]}
	for len(stack) > 0 {
		b := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if seen[b] {
			continue
		}
		seen[b] = true
		stack = append(stack, b.Succs...)
	}
	return seen
}

// UseDef は関数内の各ローカル変数の定義位置と使用位置 (lmd.Ops の添字、昇順)。
// 変数の一部への書き込み (IsPartialDef) は定義と使用の両方に数える。
type UseDef struct {
	Defs map[*Value][]int
	Uses map[*Value][]int
}

// BuildUseDef は命令列を 1 回走査して UseDef を作る。ローカル変数 (KindLocal) だけを対象にする。
func BuildUseDef(lmd *Lambda) *UseDef {
	ud := &UseDef{Defs: map[*Value][]int{}, Uses: map[*Value][]int{}}
	local := func(o Operand) *Value {
		if ValKind(o) != KindLocal {
			return nil
		}
		if pa, ok := o.(*PointeredArray); ok {
			o = pa.From
		}
		return UnderlyingValue(o)
	}
	for i, op := range lmd.Ops {
		if op == nil {
			continue
		}
		defs, uses := DefUse(op)
		for _, d := range defs {
			if v := local(d); v != nil {
				ud.Defs[v] = append(ud.Defs[v], i)
				if IsPartialDef(d) {
					ud.Uses[v] = append(ud.Uses[v], i)
				}
			}
		}
		for _, u := range uses {
			if v := local(u); v != nil {
				ud.Uses[v] = append(ud.Uses[v], i)
			}
		}
	}
	return ud
}

// SingleUse は v の使用が 1 箇所だけならその添字を返す。
func (ud *UseDef) SingleUse(v *Value) (int, bool) {
	if u := ud.Uses[v]; len(u) == 1 {
		return u[0], true
	}
	return 0, false
}
