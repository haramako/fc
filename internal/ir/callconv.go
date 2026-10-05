package ir

import "fmt"

// CallConv は関数の呼び出し規約: 引数と戻り値の置き場所と、入口 (frames.Analyze が決める。Agent/wiki/design/frame-alloc.md)。
// 呼ぶ側 (codegen の呼び出しの計画)、呼ばれる側 (入口・return)、regalloc、frames (戻り値を A だけで返せるか) が同じものを見る。
type CallConv struct {
	ABI ABI
	// Params は引数の置き場所 (宣言の順)。Off は戻り値の後ろからのバイトの位置 (static は自分のフレーム F_<sym>+Off、
	// stack 系はスタック S+Off,x。同じ並び)。Reg は static の関数がレジスタの入口で受け取る引数 (最後の 1 バイトの引数は A、
	// その前の 1 バイトの引数は Y。それ以外は NoReg)
	Params []ArgLoc
	Result ResultLoc
	// Entries は入口。先頭が関数のシンボルそのもの (Suffix "")。static でアドレスを取られた関数 (Stack の入口がある) は
	// スタックの引数を自分のフレームに写してから __direct に入る。レジスタの引数のある関数は、レジスタの引数をフレームに
	// 書いた呼び出しのための入口 (__frame: 全部フレーム、__a: Y の引数だけフレーム) を前に持ち、レジスタに読んでから落ちる
	Entries []Entry
}

// NoReg はレジスタで受け取らない引数の ArgLoc.Reg。
const NoReg Reg = 0xff

// ArgLoc は引数 1 つの置き場所。
type ArgLoc struct {
	Off  int
	Size int
	Reg  Reg // RegA / RegY: レジスタの入口で受け取る。NoReg: フレーム (スタック) だけ
}

// ResultLoc は戻り値の置き場所 (フレーム・スタックの先頭)。
type ResultLoc struct {
	Size int
	// InA は 1 バイトの戻り値をフレームに書いたうえで A にも置いて返す (static の関数。呼び出し側はフレームを読まなくてよい)
	InA bool
	// OnlyA は InA のうち、フレームに書かずに A だけで返す: どの呼び出しもフレームから読まない (far call・stack の関数からの
	// 呼び出し・アドレスを取られた・asm から参照される・別名・options(symbol:) の関数でない。frames.Analyze)
	OnlyA bool
}

// Entry は入口 1 つ (シンボルは関数のシンボル + Suffix)。
type Entry struct {
	Suffix string
	Stack  bool // 引数をスタック (S+k,x) で受け取る (関数ポインタ・asm からの呼び出し)
	InA    bool // A で受け取る引数が A にある (false ならフレームに書いてある)
	InY    bool // Y で受け取る引数が Y にある
}

// 入口のシンボルの接尾辞。
const (
	EntryDirect = "__direct" // アドレスを取られた static の関数の、プロローグ (スタックからの写し) の後ろ
	EntryFrame  = "__frame"  // レジスタの引数もフレームに書いた呼び出し (入口の sty / sta を飛ばす)
	EntryA      = "__a"      // Y の引数はフレーム、A の引数は A (sty だけ飛ばす)
)

// RegParam はレジスタ r で受け取る引数 (無ければ nil)。
func (c *CallConv) RegParam(r Reg) *ArgLoc {
	for i := range c.Params {
		if c.Params[i].Reg == r {
			return &c.Params[i]
		}
	}
	return nil
}

// HasStackEntry はスタックで引数を受け取る入口 (static でアドレスを取られた関数の `sym`) があるか。
func (c *CallConv) HasStackEntry() bool {
	for _, e := range c.Entries {
		if e.Stack && c.ABI == ABIStatic {
			return true
		}
	}
	return false
}

// DirectEntry は static の関数を、レジスタの引数のうち A / Y に置けたものを inA / inY として呼ぶ入口の接尾辞
// (引数はフレームに書いて入る。スタックの入口は使わない)。
func (c *CallConv) DirectEntry(inA, inY bool) string {
	for _, e := range c.Entries {
		if !e.Stack && e.InA == inA && e.InY == inY {
			return e.Suffix
		}
	}
	panic(fmt.Sprintf("no entry with A=%v Y=%v", inA, inY))
}

// ResultFromA は caller が call で呼んだこの関数の戻り値を A から受け取れるか (受け取れなければフレームから読む)。far call は
// トランポリンが A を壊し、stack の関数は呼び出しの後で X (フレームの底) を戻すのに A を使う。
func (c *CallConv) ResultFromA(caller *Lambda, call *Op) bool {
	return c.Result.InA && !call.Far && caller.Conv.ABI != ABIStack
}

// SetStatic は static の関数の規約を作る (frames.Analyze)。entry はアドレスを取られた (スタックの入口が要る)、regs はレジスタ
// 渡し (最後の 1 バイトの引数を A、その前の 1 バイトの引数を Y、1 バイトの戻り値を A にも) をするか。
func (c *CallConv) SetStatic(fn *Lambda, entry, regs bool) {
	c.ABI = ABIStatic
	c.setLocs(fn)
	np := len(c.Params)
	if regs {
		if np > 0 && c.Params[np-1].Size == 1 {
			c.Params[np-1].Reg = RegA
		}
		if np > 1 && c.Params[np-2].Size == 1 {
			c.Params[np-2].Reg = RegY
		}
		c.Result.InA = c.Result.Size == 1
	}
	hasA, hasY := c.RegParam(RegA) != nil, c.RegParam(RegY) != nil
	if entry {
		c.Entries = []Entry{{Suffix: "", Stack: true}, {Suffix: EntryDirect, InA: hasA, InY: hasY}}
	} else {
		c.Entries = []Entry{{Suffix: "", InA: hasA, InY: hasY}}
	}
	if hasA || hasY {
		c.Entries = append(c.Entries, Entry{Suffix: EntryFrame})
	}
	if hasA && hasY {
		c.Entries = append(c.Entries, Entry{Suffix: EntryA, InA: true})
	}
}

// SetOther は static でない関数 (stack・fastcall・cc65) の規約を作る。
func (c *CallConv) SetOther(fn *Lambda, abi ABI) {
	*c = CallConv{ABI: abi}
	c.setLocs(fn)
	c.Entries = []Entry{{Suffix: "", Stack: abi == ABIStack}}
}

func (c *CallConv) setLocs(fn *Lambda) {
	c.Params = c.Params[:0]
	c.Result = ResultLoc{Size: fn.Type.Base.Size}
	off := fn.Type.Base.Size
	for _, p := range fn.Type.Params {
		c.Params = append(c.Params, ArgLoc{Off: off, Size: p.Size, Reg: NoReg})
		off += p.Size
	}
}
