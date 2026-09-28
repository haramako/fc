package ir

// 支配木と自然ループ (レジスタ割付のループ検出と、opt のループの変換 (induction / unroll / split) が使う。doc/v2_regalloc.md)。
// CFG に付いてキャッシュされる (CFG は命令列を変えたら作り直すもの)。

import "sort"

// DomTree は支配木。
type DomTree struct {
	idom map[*Block]*Block // 直接支配ブロック (入口は自分自身。到達できないブロックは無い)
}

// IDom は b の直接支配ブロック (入口なら b 自身、到達できなければ nil)。
func (d *DomTree) IDom(b *Block) *Block { return d.idom[b] }

// Dominates は a が b を支配するか (a == b も含む)。到達できないブロックは何も支配せず、支配もされない。
func (d *DomTree) Dominates(a, b *Block) bool {
	for x := b; x != nil; x = d.idom[x] {
		if x == a {
			return true
		}
		if d.idom[x] == x {
			return false
		}
	}
	return false
}

// DomTree は支配木 (Cooper / Harvey / Kennedy の反復法)。
func (c *CFG) DomTree() *DomTree {
	if c.dom != nil {
		return c.dom
	}
	idom := map[*Block]*Block{}
	c.dom = &DomTree{idom: idom}
	if len(c.Blocks) == 0 {
		return c.dom
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
	return c.dom
}

// Loop は自然ループ。
type Loop struct {
	Header *Block
	Blocks map[*Block]bool // ヘッダを含む
	Tails  []*Block        // back edge の元 (Tail → Header)
	Parent *Loop           // このループを含む最も内側のループ (最外なら nil)
	Depth  int             // 入れ子の深さ (最外は 1)
}

// Contains はブロックがループに含まれるか。
func (l *Loop) Contains(b *Block) bool { return l.Blocks[b] }

// EveryIteration はループの中のブロック b を毎周必ず通るか (全ての back edge の元を支配する)。
func (l *Loop) EveryIteration(d *DomTree, b *Block) bool {
	for _, t := range l.Tails {
		if !d.Dominates(b, t) {
			return false
		}
	}
	return true
}

// Loops は自然ループの一覧 (同じヘッダの back edge は 1 つのループにまとめる。並びはヘッダの順)。到達できないブロックは無視。
// 入れ子は Parent / Depth に入る。戻り値の並びは呼び出し側が並べ替えてよい (写し)。
func (c *CFG) Loops() []*Loop {
	if c.loops == nil {
		c.loops = c.findLoops()
	}
	return append([]*Loop(nil), c.loops...)
}

func (c *CFG) findLoops() []*Loop {
	d := c.DomTree()
	byHeader := map[*Block]*Loop{}
	loops := []*Loop{}
	for _, b := range c.Blocks {
		if d.idom[b] == nil {
			continue
		}
		for _, s := range b.Succs {
			if !d.Dominates(s, b) {
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
	// 入れ子: ヘッダを含む他のループのうち最も小さいものが親 (ブロック数の昇順に見て最初に見つかるもの)
	bySize := append([]*Loop(nil), loops...)
	sort.SliceStable(bySize, func(i, j int) bool { return len(bySize[i].Blocks) < len(bySize[j].Blocks) })
	for _, lp := range bySize {
		for _, outer := range bySize {
			if outer != lp && len(outer.Blocks) > len(lp.Blocks) && outer.Blocks[lp.Header] {
				lp.Parent = outer
				break
			}
		}
	}
	for _, lp := range loops {
		lp.Depth = 1
		for p := lp.Parent; p != nil; p = p.Parent {
			lp.Depth++
		}
	}
	return loops
}

// Entries はループへ入る辺の元 (ループ外のブロックからヘッダへ)。
func (c *CFG) Entries(l *Loop) []*Block {
	var r []*Block
	for _, p := range l.Header.Preds {
		if !l.Blocks[p] {
			r = append(r, p)
		}
	}
	return r
}

// Preheader はループの入口が 1 つならそのブロック (無ければ nil)。ループに入る前に 1 度だけ行う処理はその末尾に置ける。
func (c *CFG) Preheader(l *Loop) *Block {
	if e := c.Entries(l); len(e) == 1 {
		return e[0]
	}
	return nil
}
