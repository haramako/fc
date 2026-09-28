package ir

// メモリアクセス命令 (load_mem / store_mem) の番地の形 (doc/ir_memops.md)。

import "github.com/haramako/fc/internal/types"

// NoIndex は添字の無い load_mem / store_mem の Src[1] に置く番兵 (リテラルの 0。Op.Mem() は Index を nil にして返す)。
// 定数の添字 0 とは区別する (定数の添字は codegen が `ldy #0` で読む。Disp に畳むのは別の最適化)。
var NoIndex = &Value{Kind: KindLiteral, IsInt: true, Int: 0, Type: types.NewUniverse().IntType(1, false)}

// MemRef は load_mem / store_mem の番地 `Base + Index * Scale + Disp`。Index が無ければ nil (Scale は 0)。
type MemRef struct {
	Base  Operand // グローバルの配列 (KindGlobal、Array 型) か 2 バイトのポインタ値
	Index Operand // 1 バイトの添字 (無ければ nil)
	Scale int     // 添字 1 につき進むバイト数 (要素の大きさ。添字がバイト単位なら 1)
	Disp  int     // 定数のずれ (バイト)
	Width int     // 読む / 書く幅 (バイト)
}

// IsMem は load_mem / store_mem か。
func (op *Op) IsMem() bool { return op.Code == OpLoadMem || op.Code == OpStoreMem }

// Mem は load_mem / store_mem の番地と幅。
func (op *Op) Mem() MemRef {
	m := MemRef{Base: op.Src[0], Scale: op.Scale, Disp: op.Disp, Width: op.Width}
	if op.Src[1] != Operand(NoIndex) {
		m.Index = op.Src[1]
	} else {
		m.Scale = 0
	}
	if op.Code == OpLoadMem {
		m.Width = ValType(op.Dst).Size
	}
	return m
}

// MemValue は store_mem の書く値。
func (op *Op) MemValue() Operand { return op.Src[2] }

// MemBaseIsArray は Base がグローバルの配列か (そうでなければポインタ値)。
func (m MemRef) BaseIsArray() bool {
	return ValKind(m.Base) == KindGlobal && ValType(m.Base).Kind == types.Array
}

// NewLoadMem は `dst = mem[base + index*scale + disp]` (index が nil なら添字無し)。
func NewLoadMem(dst, base, index Operand, scale, disp int) *Op {
	if index == nil {
		index, scale = NoIndex, 0
	}
	return &Op{Code: OpLoadMem, Dst: dst, Src: []Operand{base, index}, Scale: scale, Disp: disp}
}

// NewStoreMem は `mem[base + index*scale + disp .. +width) = val`。
func NewStoreMem(base, index Operand, scale, disp, width int, val Operand) *Op {
	if index == nil {
		index, scale = NoIndex, 0
	}
	return &Op{Code: OpStoreMem, Src: []Operand{base, index, val}, Scale: scale, Disp: disp, Width: width}
}
