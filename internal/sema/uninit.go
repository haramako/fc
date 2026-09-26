package sema

// 初期化していないローカル変数の読み出しの警告 (doc/roadmap.md の v3、2026-09-26 決定)。静的フレームはほかの関数と
// 重なるので、代入する前に読むと前の関数の値が見え、-O で結果も変わる (`var y:u8; y += 1;`)。ゼロで初期化はしない。
// 意味解析の直後の命令列で、代入されていないかもしれない変数を前向きに辿る (最適化の前なので -O で変わらない)。
// 対象はユーザーが宣言したスカラーのローカル変数。アドレスを取られた変数 (`f(&x)` で初期化しうる) と struct・配列は除く。

import (
	"fmt"

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/types"
)

func (h *Hlc) warnUninitialized(lmd *ir.Lambda) {
	idx := map[*ir.Value]int{}
	var vars []*ir.Value
	for _, v := range lmd.Vars {
		if v.Kind != ir.KindLocal || v.LocalType != ir.LTNone || v.Name == "" || v.Name[0] == '$' {
			continue
		}
		switch v.Type.Kind {
		case types.Int, types.Bool, types.Pointer, types.Func:
			idx[v] = len(vars)
			vars = append(vars, v)
		}
	}
	for _, op := range lmd.Ops {
		if op != nil && op.Code == ir.OpRef {
			if v := ir.UnderlyingValue(op.Src[0]); v != nil {
				if i, ok := idx[v]; ok {
					vars[i] = nil // アドレスを取られた
				}
			}
		}
	}
	live := false
	for _, v := range vars {
		live = live || v != nil
	}
	if !live {
		return
	}
	cfg := ir.BuildCFG(lmd)
	if len(cfg.Blocks) == 0 {
		return
	}
	reach := cfg.Reachable()
	n := len(vars)
	// maybe[b][i]: ブロック b の入口で vars[i] が代入されていないかもしれない
	maybe := map[*ir.Block][]bool{}
	transfer := func(b *ir.Block, cur []bool, report func(i int, op *ir.Op)) {
		for _, k := range cfg.Ops(b) {
			op := lmd.Ops[k]
			if op == nil {
				continue
			}
			defs, uses := ir.DefUse(op)
			if report != nil {
				for _, u := range uses {
					if i, ok := idx[ir.UnderlyingValue(u)]; ok && vars[i] != nil && cur[i] {
						report(i, op)
					}
				}
			}
			for _, d := range defs {
				if i, ok := idx[ir.UnderlyingValue(d)]; ok {
					cur[i] = false // 一部への書き込みも代入とみなす (誤検出を避ける)
				}
			}
		}
	}
	for changed := true; changed; {
		changed = false
		for _, b := range cfg.Blocks {
			if !reach[b] {
				continue
			}
			in := make([]bool, n)
			if b == cfg.Blocks[0] {
				for i := range in {
					in[i] = true
				}
			}
			for _, p := range b.Preds {
				if !reach[p] {
					continue
				}
				out := append([]bool(nil), maybe[p]...)
				if out == nil {
					continue // まだ入口を計算していないブロック
				}
				transfer(p, out, nil)
				for i := range in {
					in[i] = in[i] || out[i]
				}
			}
			old := maybe[b]
			if old == nil || !equalBools(old, in) {
				if old == nil || anyNew(old, in) {
					changed = true
				}
				maybe[b] = in
			}
		}
	}
	warned := make([]bool, n)
	for _, b := range cfg.Blocks {
		if !reach[b] || maybe[b] == nil {
			continue
		}
		cur := append([]bool(nil), maybe[b]...)
		transfer(b, cur, func(i int, op *ir.Op) {
			if !warned[i] {
				warned[i] = true
				h.prog.Warnings = append(h.prog.Warnings, diag.Warning{Pos: op.Pos,
					Msg: fmt.Sprintf("`%s` may be read before it is assigned (local variables start with an unknown value)", vars[i].Name)})
			}
		})
	}
}

func equalBools(a, b []bool) bool {
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// anyNew は b に a に無い true があるか (集合は広がるだけなので、これが無くなれば固定点)。
func anyNew(a, b []bool) bool {
	for i := range a {
		if b[i] && !a[i] {
			return true
		}
	}
	return false
}
