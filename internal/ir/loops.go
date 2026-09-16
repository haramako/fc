package ir

// 支配木と自然ループ (レジスタ割付のループ検出用。doc/v2_regalloc.md)。

// Dominators は各ブロックの直接支配ブロック (idom) を返す (入口は自分自身。到達できないブロックは nil)。
// Cooper / Harvey / Kennedy の反復法。
func (c *CFG) Dominators() map[*Block]*Block {
	idom := map[*Block]*Block{}
	if len(c.Blocks) == 0 {
		return idom
	}
	// 逆後順 (reverse postorder) の番号
	order := map[*Block]int{}
	var post []*Block
	seen := map[*Block]bool{}
	var walk func(b *Block)
	walk = func(b *Block) {
		seen[b] = true
		for _, s := range b.Succs {
			if !seen[s] {
				walk(s)
			}
		}
		post = append(post, b)
	}
	entry := c.Blocks[0]
	walk(entry)
	rpo := make([]*Block, 0, len(post))
	for i := len(post) - 1; i >= 0; i-- {
		order[post[i]] = len(rpo)
		rpo = append(rpo, post[i])
	}
	intersect := func(a, b *Block) *Block {
		for a != b {
			for order[a] > order[b] {
				a = idom[a]
			}
			for order[b] > order[a] {
				b = idom[b]
			}
		}
		return a
	}
	idom[entry] = entry
	for changed := true; changed; {
		changed = false
		for _, b := range rpo[1:] {
			var newIdom *Block
			for _, p := range b.Preds {
				if idom[p] == nil {
					continue
				}
				if newIdom == nil {
					newIdom = p
				} else {
					newIdom = intersect(p, newIdom)
				}
			}
			if newIdom != nil && idom[b] != newIdom {
				idom[b] = newIdom
				changed = true
			}
		}
	}
	return idom
}

// Loop は自然ループ。
type Loop struct {
	Header *Block
	Blocks map[*Block]bool // ヘッダを含む
	Tails  []*Block        // back edge の元 (Tail → Header)
}

// Contains はブロックがループに含まれるか。
func (l *Loop) Contains(b *Block) bool { return l.Blocks[b] }

// Loops は自然ループの一覧 (同じヘッダの back edge は 1 つのループにまとめる)。到達できないブロックは無視。
func (c *CFG) Loops() []*Loop {
	idom := c.Dominators()
	dominates := func(a, b *Block) bool {
		for x := b; ; x = idom[x] {
			if x == a {
				return true
			}
			if x == nil || idom[x] == x {
				return x == a
			}
		}
	}
	byHeader := map[*Block]*Loop{}
	var loops []*Loop
	for _, b := range c.Blocks {
		if idom[b] == nil {
			continue
		}
		for _, s := range b.Succs {
			if !dominates(s, b) {
				continue
			}
			lp := byHeader[s]
			if lp == nil {
				lp = &Loop{Header: s, Blocks: map[*Block]bool{s: true}}
				byHeader[s] = lp
				loops = append(loops, lp)
			}
			lp.Tails = append(lp.Tails, b)
			// ヘッダを通らずに tail へ戻れるブロックが本体
			stack := []*Block{b}
			for len(stack) > 0 {
				x := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				if lp.Blocks[x] {
					continue
				}
				lp.Blocks[x] = true
				stack = append(stack, x.Preds...)
			}
		}
	}
	return loops
}

// Innermost は他のループを含まないループだけを返す。
func Innermost(loops []*Loop) []*Loop {
	var r []*Loop
	for _, a := range loops {
		inner := true
		for _, b := range loops {
			if a == b || len(b.Blocks) >= len(a.Blocks) {
				continue
			}
			nested := true
			for x := range b.Blocks {
				if !a.Blocks[x] {
					nested = false
					break
				}
			}
			if nested {
				inner = false
				break
			}
		}
		if inner {
			r = append(r, a)
		}
	}
	return r
}

// Exits はループから外へ出る辺 (from はループ内、to はループ外)。
func (c *CFG) Exits(l *Loop) [][2]*Block {
	var r [][2]*Block
	for b := range l.Blocks {
		for _, s := range b.Succs {
			if !l.Blocks[s] {
				r = append(r, [2]*Block{b, s})
			}
		}
	}
	return r
}

// Entries はループへ入る辺 (from はループ外、to はヘッダ)。
func (c *CFG) Entries(l *Loop) []*Block {
	var r []*Block
	for _, p := range l.Header.Preds {
		if !l.Blocks[p] {
			r = append(r, p)
		}
	}
	return r
}
