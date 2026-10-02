package opt

// 3 バイト以上の値 (slice・struct) の写しの伝播。

import (
	"github.com/haramako/fc/internal/ir"
)

// propagateAggregateCopies は `load L ← P` (L と P は同じ型で 3 バイト以上のローカル) で、P が関数の中で一度も書かれず、L が
// ほかで書かれないとき、L を読む所 (部分を読む cast も) を P にして写しを消す。インライン展開が呼び先の slice の引数を一時変数へ
// 写し直すもの (`lz4.unpack` が `try_unpack` を inline で呼ぶと 155 バイト、直に呼ぶと 123 バイト) を無くす。SSA (2 バイトまで)
// の外の大きさの値のため。アドレスを取られた変数と @log が読む変数は扱わない。
func propagateAggregateCopies(lmd *ir.Lambda) bool {
	writes := map[*ir.Value]int{}
	addr := map[*ir.Value]bool{}
	defAt := map[*ir.Value]int{} // L → 丸ごと書く load の位置 (1 つだけのとき)
	for i, op := range lmd.Ops {
		if op == nil {
			continue
		}
		if op.Dst != nil {
			if v := ir.UnderlyingValue(op.Dst); v != nil {
				writes[v]++
				if dv, ok := op.Dst.(*ir.Value); ok && op.Code == ir.OpLoad {
					defAt[dv] = i
				}
			}
		}
		if op.Code == ir.OpRef || op.Code == ir.OpAsm {
			for _, s := range op.Src {
				if v := ir.UnderlyingValue(s); v != nil {
					addr[v] = true
				}
			}
		}
		for _, s := range op.Src {
			if pa, ok := s.(*ir.PointeredArray); ok {
				if v := ir.UnderlyingValue(pa.From); v != nil {
					addr[v] = true // (配列の先頭の番地として使う)
				}
			}
		}
		for _, lp := range op.Logs {
			for _, a := range lp.Args {
				if v := ir.UnderlyingValue(a.Val); v != nil {
					addr[v] = true
				}
				for _, b := range a.Bytes {
					if v := ir.UnderlyingValue(b); v != nil {
						addr[v] = true
					}
				}
			}
		}
	}
	local := func(v *ir.Value) bool {
		return v != nil && v.Kind == ir.KindLocal && !addr[v] && v.Type.Size >= 3 && v.Home == nil
	}
	repl := map[*ir.Value]*ir.Value{}
	for l, i := range defAt {
		op := lmd.Ops[i]
		p, ok := op.Src[0].(*ir.Value)
		if !ok || !local(l) || !local(p) || writes[l] != 1 || writes[p] != 0 || p.Type != l.Type || p == l {
			continue
		}
		// L を読む所はすべて写しの後ろ (写しより前で読むのは未定義の値: 置き換えない)
		early := false
		for _, o := range lmd.Ops[:i] {
			if o != nil && opReads(o, l) {
				early = true
				break
			}
		}
		if !early {
			repl[l] = p
		}
	}
	if len(repl) == 0 {
		return false
	}
	var subst func(o ir.Operand) ir.Operand
	subst = func(o ir.Operand) ir.Operand {
		switch x := o.(type) {
		case *ir.Value:
			if p, ok := repl[x]; ok {
				return p
			}
		case *ir.CastedValue:
			if f := subst(x.From); f != x.From {
				c := *x
				c.From = f
				return &c
			}
		case *ir.PointeredArray:
			if f := subst(x.From); f != x.From {
				c := *x
				c.From = f
				return &c
			}
		}
		return o
	}
	for i, op := range lmd.Ops {
		if op == nil {
			continue
		}
		if dv, ok := op.Dst.(*ir.Value); ok && op.Code == ir.OpLoad && repl[dv] != nil && defAt[dv] == i {
			ir.DropOp(lmd.Ops, i) // 写しそのもの
			continue
		}
		for k, s := range op.Src {
			op.Src[k] = subst(s)
		}
	}
	return true
}

// opReads は op が v を (部分でも) 読むか。
func opReads(op *ir.Op, v *ir.Value) bool {
	for _, s := range op.Src {
		if ir.UnderlyingValue(s) == v {
			return true
		}
	}
	return false
}

// assembleInPlace は部分ごとに組み立てた一時の値を丸ごと写すのをやめ、部分を写し先へ直に書く:
//
//	index $12 ← s.ptr, n          index s.ptr ← s.ptr, n
//	sub   $13 ← s.len, n          sub   s.len ← s.len, n
//	load  $14.ptr ← $12      →
//	load  $14.len ← $13
//	load  s ← $14
//
// sema は `s = s[n..]` / `t = s[..n]` / struct の値をこの形に作る (3 バイト以上の値は SSA の外)。写し先 D はアドレスを取られない
// ローカルか volatile でないグローバル (`w = @format(buf, …)` の w)。組み立ての間 (最初の部分の load から丸ごとの写しまで) は
// 副作用の無い命令だけ (D がグローバルなら、ポインタの先を読む load_mem も無いこと: D を指しているかもしれない)。部分を D に書く
// 所から写しまでに D のそのバイトを読み書きする命令が無いこと。さらに部分の値が 1 度だけ使う一時の値なら、それを計算する命令の
// 結果を直に書く (間に D のそのバイトに触れる命令が無く、副作用の無い命令だけのとき)。
func assembleInPlace(lmd *ir.Lambda) bool {
	ops := lmd.Ops
	pinned := map[*ir.Value]bool{} // アドレスを取られた・asm と @log が触る変数
	pinnedSym := map[string]bool{} // その中のグローバル (インライン展開した関数の命令は同じグローバルを別の Value で持つ)
	pin := func(v *ir.Value) {
		pinned[v] = true
		if v.Kind == ir.KindGlobal {
			pinnedSym[v.Symbol] = true
		}
	}
	defs := map[*ir.Value][]int{}
	uses := map[*ir.Value][]int{}
	for i, op := range ops {
		if op == nil {
			continue
		}
		if op.Code == ir.OpRef || op.Code == ir.OpAsm {
			for _, s := range op.Src {
				if v := ir.UnderlyingValue(s); v != nil {
					pin(v)
				}
			}
		}
		for _, s := range append([]ir.Operand{op.Dst}, op.Src...) {
			if pa, ok := s.(*ir.PointeredArray); ok {
				if v := ir.UnderlyingValue(pa.From); v != nil {
					pin(v)
				}
			}
		}
		for _, lp := range op.Logs {
			for _, a := range lp.Args {
				if v := ir.UnderlyingValue(a.Val); v != nil {
					pin(v)
				}
				for _, b := range a.Bytes {
					if v := ir.UnderlyingValue(b); v != nil {
						pin(v)
					}
				}
			}
		}
		if v := ir.UnderlyingValue(op.Dst); v != nil && op.Dst != nil {
			defs[v] = append(defs[v], i)
		}
		for _, s := range op.Src {
			if v := ir.UnderlyingValue(s); v != nil {
				uses[v] = append(uses[v], i)
			}
		}
	}
	// pure は (a, b) の命令がすべて副作用も分岐も無いか (d がグローバルなら、ポインタの先を読む命令も無いか)
	pure := func(a, b int, d *ir.Value) bool {
		for _, o := range ops[a+1 : b] {
			if o != nil && (!o.Code.IsPure() || d.Kind == ir.KindGlobal && (o.Code == ir.OpLoadMem || o.Code.MayTouchGlobals())) {
				return false
			}
		}
		return true
	}
	// clear は (a, b) の命令が d のバイト [lo, hi) を読み書きしないか
	clear := func(a, b int, d *ir.Value, lo, hi int) bool {
		for _, o := range ops[a+1 : b] {
			if o == nil {
				continue
			}
			for _, s := range append([]ir.Operand{o.Dst}, o.Src...) {
				if overlaps(s, d, lo, hi) {
					return false
				}
			}
		}
		return true
	}
	changed := false
	for i, op := range ops {
		if op == nil || op.Code != ir.OpLoad && (op.Code != ir.OpReturn || len(op.Src) == 0 || lmd.Result == nil) {
			continue
		}
		t, ok := op.Src[0].(*ir.Value)
		d, ok2 := op.Dst.(*ir.Value)
		if op.Code == ir.OpReturn {
			d, ok2 = lmd.Result, true // `return s[..n]`: 戻り値の領域に直に組み立てる
		}
		dOK := d != nil && (d.Kind == ir.KindLocal && !pinned[d] ||
			d.Kind == ir.KindGlobal && d.Symbol != "" && !d.Volatile && !d.ReadOnly && !pinnedSym[d.Symbol])
		if !ok || !ok2 || t == d || t.LocalType != ir.LTTemp || t.Kind != ir.KindLocal || !dOK ||
			t.Type.Size < 3 || t.Type.Size != d.Type.Size || pinned[t] || t.Home != nil || d.Home != nil ||
			len(uses[t]) != 1 || uses[t][0] != i {
			continue
		}
		// t の定義はすべて i より前の部分の load で、重ならずに全部のバイトを覆う
		var parts []*ir.Op
		covered := make([]bool, t.Type.Size)
		first := i
		good := true
		for _, k := range defs[t] {
			p := ops[k]
			cv, ok := p.Dst.(*ir.CastedValue)
			if k > i || p.Code != ir.OpLoad || !ok || !cv.Plain() || cv.From != ir.Operand(t) {
				good = false
				break
			}
			for b := cv.Offset; b < cv.Offset+cv.Type.Size; b++ {
				if b >= len(covered) || covered[b] {
					good = false
					break
				}
				covered[b] = true
			}
			first = min(first, k)
			parts = append(parts, p)
		}
		if !good || len(parts) == 0 || !pure(first-1, i, d) {
			continue
		}
		for _, c := range covered {
			good = good && c
		}
		if !good {
			continue
		}
		for _, k := range defs[t] {
			cv := ops[k].Dst.(*ir.CastedValue)
			if !clear(k, i, d, cv.Offset, cv.Offset+cv.Type.Size) {
				good = false
				break
			}
		}
		if !good {
			continue
		}
		for _, k := range defs[t] {
			p := ops[k]
			cv := p.Dst.(*ir.CastedValue)
			dst := ir.NewCastedValue(d, cv.Type, cv.Offset)
			p.Dst = dst
			// 部分の値が 1 度だけ使う一時の値なら、それを計算する命令に直に書かせる
			x, ok := p.Src[0].(*ir.Value)
			if !ok || x.LocalType != ir.LTTemp || x.Kind != ir.KindLocal || pinned[x] || len(defs[x]) != 1 || len(uses[x]) != 1 ||
				uses[x][0] != k {
				continue
			}
			j := defs[x][0]
			q := ops[j]
			if j > k || q.Dst != ir.Operand(x) || !q.Code.ReadsBeforeWrite() || !sameBits(q.Code, cv.Type, x.Type) ||
				!pure(j, k, d) || !clear(j, k, d, cv.Offset, cv.Offset+cv.Type.Size) {
				continue
			}
			q.Dst = dst
			ir.DropOp(ops, k)
		}
		if op.Code == ir.OpReturn {
			op.Src[0] = d
		} else {
			ir.DropOp(ops, i)
		}
		changed = true
	}
	return changed
}

// overlaps は o が変数 v のバイト [lo, hi) に触れうるか (ポインタで指す先の配列は v の中身を読むものとみなす)。
func overlaps(o ir.Operand, v *ir.Value, lo, hi int) bool {
	switch x := o.(type) {
	case *ir.Value:
		return sameVar(x, v)
	case *ir.CastedValue:
		if !sameVar(ir.UnderlyingValue(x), v) {
			return overlaps(x.From, v, lo, hi)
		}
		if _, ok := x.From.(*ir.Value); !ok {
			return true
		}
		return x.Offset < hi && lo < x.Offset+x.Type.Size
	case *ir.PointeredArray:
		return overlaps(x.From, v, lo, hi)
	}
	return false
}

// sameVar は a と b が同じ変数か (グローバルはシンボルで比べる: インライン展開した関数の命令は同じグローバルを別の Value で持つ)。
func sameVar(a, b *ir.Value) bool {
	return a == b || a != nil && b != nil && a.Kind == ir.KindGlobal && b.Kind == ir.KindGlobal && a.Symbol != "" && a.Symbol == b.Symbol
}
