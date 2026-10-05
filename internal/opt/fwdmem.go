package opt

import (
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/types"
)

// forwardFields は同じ基本ブロックの中で、struct を指すポインタのフィールドの読み出しを、直前に読んだ値・書いた値で置き換える
// (`load_mem d = p, disp=k` → `load d = v`)。
//
//	var pressed = now & ~p.held; p.released = p.held & ~now;   // 2 回目の p.held は 1 回目の値
//	p.timer = 16; … if (p.timer == 0) { … }      // 書いた定数 (比較が畳まれる)
//
// `(p),y` の読み出しは `ldy #k; lda (p),y` の 4 バイト 7 サイクルで、置き換えると一時変数の読み出し (2 バイト 3 サイクル) か、
// 定数なら畳まれる (pad.update)。書いた値が書く所だけで使う一時変数なら置き換えない (下)。
//
// 覚えた値を捨てる所: ラベル (合流)、呼び出し・インラインアセンブラ・グローバル変数への書き込み・アドレスを取られた局所変数への
// 書き込み (どれもポインタの先を書きうる)、ほかのポインタ・配列への store_mem (同じ所を指しうる)、同じポインタへの重なる
// store_mem、ポインタ自身・覚えた値の変数への書き込み。
// 対象は struct を指すポインタの、添字の無いフィールドだけ: I/O のレジスタを指すポインタ (`*u8`) の読み出しは読むたびに値が
// 変わりうるので残す (struct で I/O のレジスタを表す書き方はしない前提。docs/reference/language.md の「ポインタ」)。
func forwardFields(lmd *ir.Lambda) bool {
	refered := map[*ir.Value]bool{}
	uses := map[*ir.Value]int{} // 変数を読む命令の数
	for _, op := range lmd.Ops {
		if op == nil {
			continue
		}
		if op.Code == ir.OpRef {
			refered[ir.UnderlyingValue(op.Src[0])] = true
		}
		_, us := ir.DefUse(op)
		for _, u := range us {
			if v := ir.UnderlyingValue(u); v != nil {
				uses[v]++
			}
		}
	}
	type key struct {
		base *ir.Value
		disp int
	}
	type known struct {
		val   ir.Operand
		width int
	}
	mem := map[key]known{}
	// base は op の番地が対象の形なら、そのポインタの変数
	base := func(op *ir.Op) *ir.Value {
		m := op.Mem()
		if m.Index != nil {
			return nil
		}
		b := ir.UnderlyingValue(m.Base)
		if b == nil || b.Kind != ir.KindLocal || refered[b] || b.Type.Size != 2 {
			return nil
		}
		if c, ok := m.Base.(*ir.CastedValue); ok && (c.Offset != 0 || c.Width != 0 && c.Width != 2) {
			return nil
		}
		if b.Type.Kind != types.Pointer || b.Type.Base == nil || b.Type.Base.Kind != types.Struct {
			return nil
		}
		return b
	}
	// forget は変数 v を書いたとき: v をポインタか値に持つものを捨てる
	forget := func(v *ir.Value) {
		if v == nil {
			return
		}
		if v.Kind != ir.KindLocal || refered[v] {
			clear(mem) // グローバル・アドレスを取られた局所: ポインタの先かもしれない
			return
		}
		for k, kn := range mem {
			if k.base == v || ir.UnderlyingValue(kn.val) == v {
				delete(mem, k)
			}
		}
	}
	changed := false
	for _, op := range lmd.Ops {
		if op == nil {
			continue
		}
		switch {
		case op.Code == ir.OpLabel || op.Code.IsCall() || op.Code.IsOpaque() || op.Code.IsTerminator() && !op.Code.IsCondBranch():
			clear(mem)
			continue
		case op.Code == ir.OpLoadMem:
			b := base(op)
			w := ir.ValType(op.Dst).Size
			disp := op.Disp // 置き換えると op.Disp は 0 になるので先に取る (ps.f1 の値を disp=0 の ps.f0 として覚えていた。seed 60443000)
			if _, plain := op.Dst.(*ir.Value); b != nil && plain {
				if kn, ok := mem[key{b, disp}]; ok && kn.width == w && ir.ValType(kn.val).Size == w {
					*op = ir.Op{Code: ir.OpLoad, Dst: op.Dst, Src: []ir.Operand{kn.val}, Pos: op.Pos, Logs: op.Logs}
					if v := ir.UnderlyingValue(kn.val); v != nil {
						uses[v]++
					}
					changed = true
				}
			}
			d := ir.UnderlyingValue(op.Dst)
			forget(d)
			if b != nil && d != nil && d != b && d.Kind == ir.KindLocal && !refered[d] {
				if _, ok := op.Dst.(*ir.Value); ok {
					mem[key{b, disp}] = known{op.Dst, w}
				}
			}
			continue
		case op.Code == ir.OpStoreMem:
			b := base(op)
			m := op.Mem()
			for k := range mem {
				if b == nil || k.base != b || k.disp < m.Disp+m.Width && m.Disp < k.disp+mem[k].width {
					delete(mem, k)
				}
			}
			if b == nil {
				continue
			}
			v := op.MemValue()
			if ir.ValType(v).Size != m.Width {
				continue
			}
			if lit := ir.ValLiteral(v); lit != nil && lit.Kind == ir.KindLiteral && lit.IsInt {
				mem[key{b, m.Disp}] = known{v, m.Width}
			} else if uv, ok := v.(*ir.Value); ok && uv.Kind == ir.KindLocal && !refered[uv] && (uv.LocalType != ir.LTTemp || uses[uv] > 1) {
				// 書いた値の一時変数が書く所だけで使われていれば、A に置いたまま書いて (p),y で読み直す方が短い
				// (`sbc #1; sta (p),y; lda (p),y` の 2 バイト。置き換えると一時変数がメモリに出て `sta t; …; lda t` の 4 バイト)
				mem[key{b, m.Disp}] = known{v, m.Width}
			}
			continue
		}
		if op.Code.MayTouchGlobals() {
			clear(mem)
			continue
		}
		defs, _ := ir.DefUse(op)
		for _, d := range defs {
			forget(ir.UnderlyingValue(d))
		}
	}
	return changed
}
