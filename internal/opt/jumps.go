package opt

import "github.com/haramako/fc/internal/ir"

// simplifyJumps は制御フローの整理。hlc は if / while / for を素直に (ラベルとジャンプの連鎖で) 出すので、
//
//  1. ジャンプの連鎖: 飛び先がラベルだけ、またはラベル + jump のブロックなら、その先へ直接飛ぶ
//  2. 直後への jump を消す
//  3. 到達できないブロックを消す
//  4. 参照されないラベルを消す
//  5. 分岐の反転: `if !c goto L1; jump L2; L1:` → `if c goto L2`
//  6. ループの回転: 先頭で条件を見て末尾で先頭に戻る形を、末尾で条件を見て本体の先頭へ戻る形にする
//     (1 周あたり jmp 1 つ = 3 サイクル減り、分岐が短くなる)
//
// 変化がなくなるまで繰り返す。
func simplifyJumps(lmd *ir.Lambda) {
	for iter := 0; iter < 20; iter++ {
		changed := threadJumps(lmd)
		changed = removeUnreachable(lmd) || changed
		changed = invertBranches(lmd) || changed
		changed = rotateLoops(lmd) || changed
		compact(lmd)
		if !changed {
			return
		}
	}
}

func isBranch(op *ir.Op) bool {
	return op != nil && (op.Code == ir.OpIf || op.Code == ir.OpIfTrue || op.Code == ir.OpJump)
}

// threadJumps は 1・2 (ジャンプの連鎖と直後への jump)。
func threadJumps(lmd *ir.Lambda) bool {
	cfg := ir.BuildCFG(lmd)
	ops := lmd.Ops
	changed := false
	// ブロックの「実質の飛び先」: ラベルだけなら次のブロック、ラベル + jump ならその先
	resolve := func(label string) string {
		for n := 0; n < 20; n++ {
			b := cfg.BlockOf(label)
			if b == nil {
				return label
			}
			var body []*ir.Op
			for _, i := range cfg.Ops(b) {
				if ops[i].Code != ir.OpLabel {
					body = append(body, ops[i])
				}
			}
			switch {
			case len(body) == 0 && b.Index+1 < len(cfg.Blocks) && cfg.Blocks[b.Index+1].Label != "":
				label = cfg.Blocks[b.Index+1].Label
			case len(body) == 1 && body[0].Code == ir.OpJump && body[0].Label != label:
				label = body[0].Label
			default:
				return label
			}
		}
		return label
	}
	for _, b := range cfg.Blocks {
		for _, i := range cfg.Ops(b) {
			op := ops[i]
			if !isBranch(op) {
				continue
			}
			if l := resolve(op.Label); l != op.Label {
				op.Label = l
				changed = true
			}
			// 直後のブロックへの jump は不要
			if op.Code == ir.OpJump && b.Index+1 < len(cfg.Blocks) && cfg.Blocks[b.Index+1].Label == op.Label {
				ops[i] = nil
				changed = true
			}
		}
	}
	return changed
}

// removeUnreachable は 3・4 (到達不能なブロックと、参照されないラベル)。
func removeUnreachable(lmd *ir.Lambda) bool {
	cfg := ir.BuildCFG(lmd)
	ops := lmd.Ops
	changed := false
	reach := cfg.Reachable()
	refs := map[string]int{}
	for _, b := range cfg.Blocks {
		if !reach[b] {
			for _, i := range cfg.Ops(b) {
				ops[i] = nil
				changed = true
			}
			continue
		}
		for _, i := range cfg.Ops(b) {
			if isBranch(ops[i]) {
				refs[ops[i].Label]++
			}
		}
	}
	for i, op := range ops {
		if op != nil && op.Code == ir.OpLabel && refs[op.Label] == 0 && !usedByAsm(lmd, op.Label) {
			ops[i] = nil
			changed = true
		}
	}
	return changed
}

// usedByAsm はインラインアセンブラがラベル名を参照していそうか (安全側)。
func usedByAsm(lmd *ir.Lambda, label string) bool {
	for _, op := range lmd.Ops {
		if op != nil && op.Code == ir.OpAsm && contains(op.Text, label) {
			return true
		}
	}
	return false
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// invertBranches は 5: `if c goto L1; jump L2; L1:` → `if !c goto L2` (if_true と if を入れ替える)。
func invertBranches(lmd *ir.Lambda) bool {
	cfg := ir.BuildCFG(lmd)
	ops := lmd.Ops
	changed := false
	for bi := 0; bi+2 < len(cfg.Blocks); bi++ {
		b, jb, lb := cfg.Blocks[bi], cfg.Blocks[bi+1], cfg.Blocks[bi+2]
		last := cfg.Last(b)
		if last == nil || (last.Code != ir.OpIf && last.Code != ir.OpIfTrue) {
			continue
		}
		jops := cfg.Ops(jb)
		if len(jops) != 1 || ops[jops[0]].Code != ir.OpJump || jb.Label != "" || lb.Label != last.Label {
			continue
		}
		if last.Code == ir.OpIf {
			last.Code = ir.OpIfTrue
		} else {
			last.Code = ir.OpIf
		}
		last.Label = ops[jops[0]].Label
		ops[jops[0]] = nil
		changed = true
	}
	return changed
}

// rotateLoops は 6。対象の形 (hlc の while / for):
//
//	L_begin: <cond>; if c goto L_end     (ブロック B0。他からの流入は jump L_begin だけ)
//	<body>                               (B1 …)
//	jump L_begin                         (L_end の直前のブロックの末尾)
//	L_end:
//
// これを
//
//	jump L_begin
//	L_body: <body>
//	L_begin: <cond>; if_true c goto L_body
//	L_end:
//
// にする。本体の中の continue (jump L_begin) はそのまま条件へ飛ぶ。
func rotateLoops(lmd *ir.Lambda) bool {
	cfg := ir.BuildCFG(lmd)
	ops := lmd.Ops
	for _, b0 := range cfg.Blocks {
		if b0.Label == "" {
			continue
		}
		cond := cfg.Last(b0)
		if cond == nil || (cond.Code != ir.OpIf && cond.Code != ir.OpIfTrue) {
			continue
		}
		end := cfg.BlockOf(cond.Label)
		if end == nil || end.Index <= b0.Index+1 {
			continue
		}
		back := cfg.Blocks[end.Index-1]
		bj := cfg.Last(back)
		if bj == nil || bj.Code != ir.OpJump || bj.Label != b0.Label {
			continue
		}
		// B0 に流れ込むのは直前からの fallthrough と jump L_begin だけ (if の飛び先には使われていない)
		ok := true
		for _, p := range b0.Preds {
			if l := cfg.Last(p); p.Index != b0.Index-1 && (l == nil || l.Code != ir.OpJump || l.Label != b0.Label) {
				ok = false
			}
		}
		if !ok {
			continue
		}
		// 本体 (B1 .. back) の中に L_end へ抜けるブロック以外の出口があってもよい (break はそのまま)
		bodyLabel := newLabel(lmd, "body")
		var out []*ir.Op
		out = append(out, ops[:b0.Start]...)
		out = append(out, &ir.Op{Code: ir.OpJump, Label: b0.Label, Pos: cond.Pos})
		out = append(out, &ir.Op{Code: ir.OpLabel, Label: bodyLabel, Pos: cond.Pos})
		for i := b0.End; i < back.End; i++ {
			if ops[i] != bj {
				out = append(out, ops[i])
			}
		}
		out = append(out, ops[b0.Start:b0.End]...)
		if cond.Code == ir.OpIf {
			cond.Code = ir.OpIfTrue
		} else {
			cond.Code = ir.OpIf
		}
		cond.Label = bodyLabel
		out = append(out, ops[end.Start:]...)
		lmd.Ops = out
		return true // CFG が変わったので作り直す
	}
	return false
}
