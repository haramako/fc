package ir

import "github.com/haramako/fc/internal/types"

// 命令ごとの生存解析 (ローカル変数)。regalloc の LiveRangeCalculator は結果を区間に潰すが、こちらは集合のまま持つ
// (ループ内の A 常駐の判定に使う。doc/v2_regalloc.md)。

// Liveness は各命令の入口 / 出口で生きているローカル変数。
type Liveness struct {
	Vars  []*Value
	index map[*Value]int
	in    [][]bool // [op][var]
	out   [][]bool
}

// BuildLiveness は標準の後ろ向きデータフロー (ブロック単位で反復してから命令単位に展開)。
// 変数の一部への書き込み (IsPartialDef) は使用でもある。引数は入口で定義済み扱い。
func BuildLiveness(lmd *Lambda) *Liveness { return buildLiveness(lmd, false) }

// BuildLivenessWithGlobals はグローバルのスカラ変数 (配列でない) も対象にする。呼び出し・インラインアセンブラ・
// ポインタ経由の書き込みは全グローバルを読んで書くとみなし、return は全グローバルを読むとみなす
// (関数の外から見える値なので、ループ内でレジスタに置いた値はそこで書き戻す必要がある)。
func BuildLivenessWithGlobals(lmd *Lambda) *Liveness { return buildLiveness(lmd, true) }

// MayTouchGlobals は op が (オペランドに現れない) グローバル変数を読み書きしうるか。
func MayTouchGlobals(op *Op) bool {
	switch op.Code {
	case OpCall, OpFastcall, OpAsm, OpPset, OpIndexPset, OpFieldPset:
		return true
	}
	return false
}

func buildLiveness(lmd *Lambda, globals bool) *Liveness {
	lv := &Liveness{index: map[*Value]int{}}
	local := func(o Operand) *Value {
		if o == nil {
			return nil
		}
		if pa, ok := o.(*PointeredArray); ok {
			o = pa.From
		}
		v := UnderlyingValue(o)
		if v == nil {
			return nil
		}
		if v.Kind == KindLocal {
			return v
		}
		if globals && v.Kind == KindGlobal && v.Symbol != "" && v.Type.Kind != types.Array && v.Type.Kind != types.Func {
			return v
		}
		return nil
	}
	add := func(v *Value) int {
		if i, ok := lv.index[v]; ok {
			return i
		}
		lv.index[v] = len(lv.Vars)
		lv.Vars = append(lv.Vars, v)
		return len(lv.Vars) - 1
	}
	n := len(lmd.Ops)
	type du struct{ defs, uses []int }
	opDU := make([]du, n)
	for i, op := range lmd.Ops {
		if op == nil {
			continue
		}
		defs, uses := DefUse(op)
		for _, d := range defs {
			if v := local(d); v != nil {
				k := add(v)
				if IsPartialDef(d) {
					opDU[i].uses = append(opDU[i].uses, k)
				} else {
					opDU[i].defs = append(opDU[i].defs, k)
				}
			}
		}
		for _, u := range uses {
			if v := local(u); v != nil {
				opDU[i].uses = append(opDU[i].uses, add(v))
			}
		}
	}
	if globals {
		// 呼び出しなどは全グローバルを読んで書く、return は全グローバルを読む
		for i, op := range lmd.Ops {
			if op == nil {
				continue
			}
			if MayTouchGlobals(op) || op.Code == OpReturn {
				for k, v := range lv.Vars {
					if v.Kind != KindGlobal {
						continue
					}
					opDU[i].uses = append(opDU[i].uses, k)
					if op.Code != OpReturn {
						opDU[i].defs = append(opDU[i].defs, k)
					}
				}
			}
		}
	}
	nv := len(lv.Vars)
	cfg := BuildCFG(lmd)
	// ブロックの gen / kill
	gen := map[*Block][]bool{}
	kill := map[*Block][]bool{}
	for _, b := range cfg.Blocks {
		g, k := make([]bool, nv), make([]bool, nv)
		for _, i := range cfg.Ops(b) {
			for _, u := range opDU[i].uses {
				if !k[u] {
					g[u] = true
				}
			}
			for _, d := range opDU[i].defs {
				k[d] = true
			}
		}
		gen[b], kill[b] = g, k
	}
	bin := map[*Block][]bool{}
	bout := map[*Block][]bool{}
	for _, b := range cfg.Blocks {
		bin[b] = make([]bool, nv)
		bout[b] = make([]bool, nv)
	}
	for changed := true; changed; {
		changed = false
		for bi := len(cfg.Blocks) - 1; bi >= 0; bi-- {
			b := cfg.Blocks[bi]
			out := bout[b]
			for _, s := range b.Succs {
				for k, v := range bin[s] {
					if v && !out[k] {
						out[k] = true
						changed = true
					}
				}
			}
			for k := 0; k < nv; k++ {
				v := gen[b][k] || (out[k] && !kill[b][k])
				if v != bin[b][k] {
					bin[b][k] = v
					changed = true
				}
			}
		}
	}
	// 命令単位に展開
	lv.in = make([][]bool, n)
	lv.out = make([][]bool, n)
	for _, b := range cfg.Blocks {
		cur := append([]bool{}, bout[b]...)
		ops := cfg.Ops(b)
		for j := len(ops) - 1; j >= 0; j-- {
			i := ops[j]
			lv.out[i] = append([]bool{}, cur...)
			for _, d := range opDU[i].defs {
				cur[d] = false
			}
			for _, u := range opDU[i].uses {
				cur[u] = true
			}
			lv.in[i] = append([]bool{}, cur...)
		}
	}
	return lv
}

// LiveIn / LiveOut は命令 i の入口 / 出口で v が生きているか。
func (lv *Liveness) LiveIn(i int, v *Value) bool {
	k, ok := lv.index[v]
	return ok && lv.in[i] != nil && lv.in[i][k]
}

func (lv *Liveness) LiveOut(i int, v *Value) bool {
	k, ok := lv.index[v]
	return ok && lv.out[i] != nil && lv.out[i][k]
}
