package ir

// 命令のオペランドの役割 (定義 / 使用) と制御フロー。regalloc と opt (最適化) が共通に使う。

import (
	"fmt"
	"slices"
)

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
		OpShiftLeft, OpShiftRight, OpIndex:
		defs = []Operand{op.Dst}
		uses = op.Src
	case OpLoadMem:
		defs = []Operand{op.Dst}
		uses = memUses(op)
	case OpStoreMem:
		uses = memUses(op)
	default:
		panic(fmt.Sprintf("DefUse: invalid op %v", DumpOp(op, nil)))
	}
	return
}

// memUses は load_mem / store_mem が使うオペランド。添字の番兵 NoIndex はリテラルなので変数の使用にはならないが、
// SSA など「Src の添字順」で使用を引く側のために Src をそのまま返す (要素を抜くと位置がずれる)。
func memUses(op *Op) []Operand { return op.Src }

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
	dom     *DomTree // DomTree のキャッシュ
	loops   []*Loop  // Loops のキャッシュ

	// 作ったときの命令列の長さと制御の命令 (ラベル・分岐・終端) の並び: 同じなら作り直さずに使える (BuildCFG)
	n     int
	marks []cfgMark
}

// cfgMark は CFG を作ったときの制御の命令 1 つ (位置・命令・中身)。
type cfgMark struct {
	i      int
	op     *Op
	code   OpCode
	label  string
	labels []string
	gap    int // 終端の直後の消した命令の穴の終わり (次のブロックの先頭。穴に命令が戻ると形が変わる)
}

// isControl は CFG の形を決める命令 (ブロックの先頭・末尾になる) か。
func isControl(c OpCode) bool {
	return c == OpLabel || c.IsTerminator() || c.IsCondBranch() || c == OpJump || c == OpSwitch || c == OpReturn
}

// BuildCFG は lmd の制御フローグラフ。前に作った CFG が今の命令列でも同じ形 (長さと、ラベル・分岐・終端の命令の位置・
// 中身が同じ) なら、それを返す (支配木・ループのキャッシュも使い回す)。ブロックの範囲は添字なので、ほかの命令を置き換える・
// 消す (nil) のはよく、挿す・詰めると作り直す。CFG は読むだけにする (書き換えない)。
func BuildCFG(lmd *Lambda) *CFG {
	if c := lmd.cfg; c != nil && c.current() {
		return c
	}
	c := buildCFG(lmd)
	lmd.cfg = c
	return c
}

// current は c が今の lmd.Ops でも同じ形か。
func (c *CFG) current() bool {
	ops := c.Lambda.Ops
	if len(ops) != c.n {
		return false
	}
	k := 0
	for i, op := range ops {
		if op == nil || !isControl(op.Code) {
			continue
		}
		if k >= len(c.marks) {
			return false
		}
		m := c.marks[k]
		if m.i != i || m.op != op || m.code != op.Code || m.label != op.Label || !slices.Equal(m.labels, op.Labels) {
			return false
		}
		for j := i + 1; j < m.gap; j++ {
			if ops[j] != nil {
				return false
			}
		}
		k++
	}
	return k == len(c.marks)
}

// buildCFG は命令列からブロックと辺を作る。OpIf は「条件が 0 なら Label へ」なので飛び先と直後の両方が後続。
// OpAsm は分岐しないものとみなす。Ops の nil 要素は飛ばす。
func buildCFG(lmd *Lambda) *CFG {
	ops := lmd.Ops
	leader := make([]bool, len(ops)+1)
	leader[0] = true
	for i, op := range ops {
		if op == nil {
			continue
		}
		if op.Code == OpLabel {
			leader[i] = true
		} else if op.Code.IsTerminator() {
			// 次の (消されていない) 命令から新しいブロック。消した命令の穴だけの空のブロックを作らない (ブロックの並びで
			// 「次のブロック」を見る判定が、穴をはさむと黙って効かなかった)
			if j := NextOp(ops, i); j >= 0 {
				leader[j] = true
			} else {
				leader[i+1] = true
			}
		}
	}
	c := &CFG{Lambda: lmd, byLabel: map[string]*Block{}, n: len(ops)}
	for i, op := range ops {
		if op != nil && isControl(op.Code) {
			gap := i + 1
			if op.Code.IsTerminator() {
				if j := NextOp(ops, i); j >= 0 {
					gap = j
				}
			}
			c.marks = append(c.marks, cfgMark{i, op, op.Code, op.Label, append([]string(nil), op.Labels...), gap})
		}
	}
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
		case last.Code.IsCondBranch():
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

// UseDef は関数内の各ローカル変数を定義する命令と使う命令 (*Op を鍵にする。位置は Lambda.IndexOf で引く)。
// 変数の一部への書き込み (IsPartialDef) は定義と使用の両方に数える。1 つの命令が同じ変数を 2 回使えば使用も 2 つ。
//
// 差分で保つ: 命令を消す (DropOp などで nil にする・詰める)・動かす・並べ替えるのは知らせなくてよい (引くときに lmd.Ops に
// 無い命令を外す)。命令を足したら Add、命令の Dst / Src を書き換えたら Update を呼ぶ。
type UseDef struct {
	lmd  *Lambda
	defs map[*Value][]*Op
	uses map[*Value][]*Op
	reg  map[*Op]udReg // Add で登録した変数 (Update / Remove で外す)
}

type udReg struct{ defs, uses []*Value }

// udLocal は UseDef の対象 (ローカル変数) の元の変数 (対象外なら nil)。
func udLocal(o Operand) *Value {
	if ValKind(o) != KindLocal {
		return nil
	}
	if pa, ok := o.(*PointeredArray); ok {
		o = pa.From
	}
	return UnderlyingValue(o)
}

// BuildUseDef は命令列を 1 回走査して UseDef を作る。ローカル変数 (KindLocal) だけを対象にする。
func BuildUseDef(lmd *Lambda) *UseDef {
	ud := &UseDef{lmd: lmd, defs: map[*Value][]*Op{}, uses: map[*Value][]*Op{}, reg: map[*Op]udReg{}}
	for _, op := range lmd.Ops {
		if op != nil {
			ud.Add(op)
		}
	}
	return ud
}

// Add は命令 op (lmd.Ops に足したもの) の定義と使用を登録する。
func (ud *UseDef) Add(op *Op) {
	if _, ok := ud.reg[op]; ok {
		return
	}
	var r udReg
	defs, uses := DefUse(op)
	for _, d := range defs {
		if v := udLocal(d); v != nil {
			ud.defs[v] = append(ud.defs[v], op)
			r.defs = append(r.defs, v)
			if IsPartialDef(d) {
				ud.uses[v] = append(ud.uses[v], op)
				r.uses = append(r.uses, v)
			}
		}
	}
	for _, u := range uses {
		if v := udLocal(u); v != nil {
			ud.uses[v] = append(ud.uses[v], op)
			r.uses = append(r.uses, v)
		}
	}
	ud.reg[op] = r
}

// Remove は命令 op の登録を外す (lmd.Ops から消した命令は外さなくても引くときに除かれる)。
func (ud *UseDef) Remove(op *Op) {
	r, ok := ud.reg[op]
	if !ok {
		return
	}
	delete(ud.reg, op)
	for _, v := range r.defs {
		ud.defs[v] = removeOp(ud.defs[v], op)
	}
	for _, v := range r.uses {
		ud.uses[v] = removeOp(ud.uses[v], op)
	}
}

// Update は命令 op の Dst / Src を書き換えた後に登録をやり直す。
func (ud *UseDef) Update(op *Op) {
	ud.Remove(op)
	ud.Add(op)
}

// removeOp は list から op を 1 つ除く (同じ命令が 2 回使う変数は 2 つ並ぶので、1 回の登録につき 1 つ)。
func removeOp(list []*Op, op *Op) []*Op {
	for i, o := range list {
		if o == op {
			return append(list[:i:i], list[i+1:]...)
		}
	}
	return list
}

// live は list のうち lmd.Ops にある命令を位置の順に返し、消えた命令の登録を外す。
func (ud *UseDef) live(list []*Op) []*Op {
	var gone []*Op
	out := make([]*Op, 0, len(list))
	for _, op := range list {
		if ud.lmd.IndexOf(op) >= 0 {
			out = append(out, op)
		} else {
			gone = append(gone, op)
		}
	}
	for _, op := range gone {
		ud.Remove(op)
	}
	sortByIndex(ud.lmd, out)
	return out
}

// sortByIndex は ops を lmd.Ops の中の位置の順に並べる (挿入ソート: 短い)。
func sortByIndex(lmd *Lambda, ops []*Op) {
	for i := 1; i < len(ops); i++ {
		for j := i; j > 0 && lmd.IndexOf(ops[j-1]) > lmd.IndexOf(ops[j]); j-- {
			ops[j-1], ops[j] = ops[j], ops[j-1]
		}
	}
}

// Defs は v を定義する命令 (位置の順)。
func (ud *UseDef) Defs(v *Value) []*Op { return ud.live(ud.defs[v]) }

// Uses は v を使う命令 (位置の順。同じ命令が 2 回使えば 2 つ)。
func (ud *UseDef) Uses(v *Value) []*Op { return ud.live(ud.uses[v]) }

// NumDefs / NumUses は v の定義 / 使用の数。
func (ud *UseDef) NumDefs(v *Value) int { return len(ud.Defs(v)) }
func (ud *UseDef) NumUses(v *Value) int { return len(ud.Uses(v)) }

// SingleUse は v の使用が 1 箇所だけならその命令を返す。
func (ud *UseDef) SingleUse(v *Value) (*Op, bool) {
	if u := ud.Uses(v); len(u) == 1 {
		return u[0], true
	}
	return nil, false
}

// SingleDef は v の定義が 1 箇所だけならその命令を返す。
func (ud *UseDef) SingleDef(v *Value) (*Op, bool) {
	if d := ud.Defs(v); len(d) == 1 {
		return d[0], true
	}
	return nil, false
}

// UseIndexes / DefIndexes は v を使う / 定義する命令の lmd.Ops の中の添字 (昇順)。
func (ud *UseDef) UseIndexes(v *Value) []int { return ud.indexes(ud.Uses(v)) }
func (ud *UseDef) DefIndexes(v *Value) []int { return ud.indexes(ud.Defs(v)) }

func (ud *UseDef) indexes(ops []*Op) []int {
	r := make([]int, len(ops))
	for i, op := range ops {
		r[i] = ud.lmd.IndexOf(op)
	}
	return r
}

// Involves は op が v を読むか書くか (オペランドの元の値で比べる。v が nil なら false)。
func Involves(op *Op, v *Value) bool {
	if v == nil {
		return false
	}
	defs, uses := DefUse(op)
	for _, o := range defs {
		if UnderlyingValue(o) == v {
			return true
		}
	}
	for _, o := range uses {
		if UnderlyingValue(o) == v {
			return true
		}
	}
	return false
}
