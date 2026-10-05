package ir

import "sync/atomic"

// 命令の同一性は *Op。lmd.Ops の中の位置は Lambda.IndexOf で引く: 命令に持たせた添字の手がかり (Op.pos) を確かめ、合わなければ
// 関数の命令を数え直す。命令を挿す・動かす・消す (nil にする / 詰める / 新しいスライスを作る) のどれをしても、位置を引く側に
// 知らせなくてよい (解析 (UseDef など) は *Op を鍵に持ち、位置は使うときに引く)。
//
// 消した命令は数え直しても見つからない (-1)。同じ命令列のまま同じ命令を何度引いても数え直さないよう、数え直したときの
// 命令列 (スライスの先頭と長さ) と回数を覚え、見つからなかった命令にはその回数を記す。消した *Op をスライスへ直接書き戻す
// (ReplaceOp を通さない) と、命令列が同じままなら -1 のままになる: 戻すときは ReplaceOp か新しいスライスで。

// posEpoch は数え直しの回数の通し番号 (関数をまたいで一意。インライン展開の写しの手がかりが別の関数で合わないように)。
var posEpoch atomic.Int64

// IndexOf は op の lmd.Ops の中の添字 (消した命令・ほかの関数の命令なら -1)。
func (lmd *Lambda) IndexOf(op *Op) int {
	ops := lmd.Ops
	if i := op.pos; i >= 0 && i < len(ops) && ops[i] == op {
		return i
	}
	if op.pos < 0 && op.posEpoch == lmd.posEpoch && lmd.sameOps() {
		return -1 // この命令列で数え直したときに無かった
	}
	lmd.renumber()
	if i := op.pos; i >= 0 && i < len(ops) && ops[i] == op {
		return i
	}
	op.pos, op.posEpoch = -1, lmd.posEpoch
	return -1
}

// Has は op が lmd.Ops にあるか (消されていないか)。
func (lmd *Lambda) Has(op *Op) bool { return lmd.IndexOf(op) >= 0 }

// renumber は lmd.Ops の全命令の位置の手がかりを付け直す。
func (lmd *Lambda) renumber() {
	for i, op := range lmd.Ops {
		if op != nil {
			op.pos = i
		}
	}
	lmd.posEpoch = posEpoch.Add(1)
	lmd.posLen = len(lmd.Ops)
	if len(lmd.Ops) > 0 {
		lmd.posHead = &lmd.Ops[0]
	} else {
		lmd.posHead = nil
	}
}

// sameOps は lmd.Ops が最後に数え直したときと同じスライス (先頭と長さ) か。
func (lmd *Lambda) sameOps() bool {
	if len(lmd.Ops) != lmd.posLen {
		return false
	}
	if len(lmd.Ops) == 0 {
		return lmd.posHead == nil
	}
	return lmd.posHead == &lmd.Ops[0]
}

// NextOp は ops[i] の次の (nil でない) 命令の添字 (無ければ -1)。消した命令の穴をはさんでも「直後の命令」を見るために使う。
func NextOp(ops []*Op, i int) int {
	for j := i + 1; j < len(ops); j++ {
		if ops[j] != nil {
			return j
		}
	}
	return -1
}

// PrevOp は ops[i] の前の (nil でない) 命令の添字 (無ければ -1)。
func PrevOp(ops []*Op, i int) int {
	for j := i - 1; j >= 0; j-- {
		if ops[j] != nil {
			return j
		}
	}
	return -1
}

// NextOpOf は ops[i] の次の (nil でない) 命令 (無ければ nil)。
func NextOpOf(ops []*Op, i int) *Op {
	if j := NextOp(ops, i); j >= 0 {
		return ops[j]
	}
	return nil
}

// MoveOpBefore は ops[i] を ops[j] の直前へ動かす (i < j。間の命令を 1 つ前に詰める。同じスライスの中で)。@log の注釈は
// 命令と一緒に動くので、元の位置に残すなら呼び出し側が先に移す。
func MoveOpBefore(ops []*Op, i, j int) {
	op := ops[i]
	copy(ops[i:j-1], ops[i+1:j])
	ops[j-1] = op
	for k := i; k < j; k++ {
		if ops[k] != nil {
			ops[k].pos = k
		}
	}
}
