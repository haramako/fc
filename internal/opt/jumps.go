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
	return op != nil && (isCond(op) || op.Code == ir.OpJump)
}

// isCond は条件分岐 (if / if_true / if_carry / if_not_carry) か。
func isCond(op *ir.Op) bool {
	if op == nil {
		return false
	}
	switch op.Code {
	case ir.OpIf, ir.OpIfTrue, ir.OpIfCarry, ir.OpIfNotCarry:
		return true
	}
	return false
}

// invertCond は条件分岐の向きを反転する。
func invertCond(op *ir.Op) {
	switch op.Code {
	case ir.OpIf:
		op.Code = ir.OpIfTrue
	case ir.OpIfTrue:
		op.Code = ir.OpIf
	case ir.OpIfCarry:
		op.Code = ir.OpIfNotCarry
	case ir.OpIfNotCarry:
		op.Code = ir.OpIfCarry
	}
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
			if op != nil && op.Code == ir.OpSwitch {
				for k, l := range op.Labels {
					if nl := resolve(l); nl != l {
						op.Labels[k] = nl
						changed = true
					}
				}
				continue
			}
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
			} else if ops[i] != nil && ops[i].Code == ir.OpSwitch {
				for _, l := range ops[i].Labels {
					refs[l]++
				}
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
		if !isCond(last) {
			continue
		}
		jops := cfg.Ops(jb)
		if len(jops) != 1 || ops[jops[0]].Code != ir.OpJump || jb.Label != "" || lb.Label != last.Label {
			continue
		}
		invertCond(last)
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
		if !isCond(cond) {
			continue
		}
		end := cfg.BlockOf(cond.Label)
		if end == nil || end.Index <= b0.Index+1 {
			continue
		}
		// 戻りの辺: jump L_begin で終わる最後のブロック (L_end の直前とは限らない。ループの出口のラベルが外側の if の
		// 終端と同じラベルに畳まれていると、L_end は別のコードの後ろにある)
		var back *ir.Block
		for _, b := range cfg.Blocks[b0.Index+1 : end.Index] {
			if l := cfg.Last(b); l != nil && l.Code == ir.OpJump && l.Label == b0.Label {
				back = b
			}
		}
		if back == nil {
			continue
		}
		bj := cfg.Last(back)
		// B0 に流れ込むのは直前からの fallthrough と本体からの jump L_begin だけ (if の飛び先には使われていない)
		ok := true
		for _, p := range b0.Preds {
			if l := cfg.Last(p); p.Index != b0.Index-1 && (l == nil || l.Code != ir.OpJump || l.Label != b0.Label || p.Index > back.Index) {
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
		if dup := cloneCond(lmd, cfg, b0); dup != nil {
			// 条件が短い (比較 1 つ + 分岐) ときは、入口の条件はそのまま残して本体の末尾に条件の写しを置く (テストの複製):
			//   L_begin: <cond>; if c goto L_end; L_body: <body>; <cond'>; if_true c' goto L_body; [jump L_end]
			// 末尾の条件の直前にラベルが無いので、`dey` の直後の `cpy #0` がピープホールで消える (crc8 / crc16 の内側ループ)。
			// 入口の jump も要らない
			out = append(out, ops[b0.Start:b0.End]...)
			out = append(out, &ir.Op{Code: ir.OpLabel, Label: bodyLabel, Pos: cond.Pos})
			for i := b0.End; i < back.End; i++ {
				if ops[i] != bj {
					out = append(out, ops[i])
				}
			}
			last := dup[len(dup)-1]
			invertCond(last)
			last.Label = bodyLabel
			out = append(out, dup...)
		} else {
			out = append(out, &ir.Op{Code: ir.OpJump, Label: b0.Label, Pos: cond.Pos})
			out = append(out, &ir.Op{Code: ir.OpLabel, Label: bodyLabel, Pos: cond.Pos})
			for i := b0.End; i < back.End; i++ {
				if ops[i] != bj {
					out = append(out, ops[i])
				}
			}
			out = append(out, ops[b0.Start:b0.End]...)
			invertCond(cond)
			cond.Label = bodyLabel
		}
		if back.Index+1 != end.Index {
			out = append(out, &ir.Op{Code: ir.OpJump, Label: end.Label, Pos: cond.Pos}) // 落ちる先が L_end でないなら飛ぶ
		}
		out = append(out, ops[back.End:]...)
		lmd.Ops = out
		return true // CFG が変わったので作り直す
	}
	return false
}

// cloneCond はループの条件ブロック (ラベル + 条件 + 分岐) の写し (ラベル抜き)。条件が「分岐だけ」か「比較 / 演算 1 つ + 分岐」
// で、定義する一時変数が分岐でしか使われないときだけ (それ以外は nil)。一時変数は新しく作る (同じ一時変数を 2 度定義すると
// live range がループ全体に伸びて A / コンディションフラグへの割付が外れる)。
func cloneCond(lmd *ir.Lambda, cfg *ir.CFG, b0 *ir.Block) []*ir.Op {
	var body []*ir.Op
	for _, i := range cfg.Ops(b0) {
		if lmd.Ops[i].Code != ir.OpLabel {
			body = append(body, lmd.Ops[i])
		}
	}
	if len(body) < 1 || len(body) > 2 || !isCond(body[len(body)-1]) {
		return nil
	}
	if last := body[len(body)-1]; len(last.Src) > 0 && ir.ValType(last.Src[0]).Size != 1 {
		return nil
	}
	rename := map[*ir.Value]*ir.Value{}
	if len(body) == 2 {
		def := body[0]
		switch def.Code {
		case ir.OpEq, ir.OpLt, ir.OpAnd, ir.OpLoad:
		default:
			return nil
		}
		t, ok := def.Dst.(*ir.Value)
		if !ok || t.LocalType != ir.LTTemp || body[1].Src[0] != ir.Operand(t) {
			return nil
		}
		for _, o := range def.Src {
			if ir.ValType(o).Size != 1 {
				return nil // 2 バイトの比較の写しは大きいだけで速くならない (cmp の連鎖にフラグの再利用は無い)
			}
		}
		nt := ir.NewLocal(t.Name+"'", t.Type, ir.LTTemp)
		lmd.Vars = append(lmd.Vars, nt)
		rename[t] = nt
	}
	var out []*ir.Op
	for _, op := range body {
		no := *op
		no.Src = make([]ir.Operand, len(op.Src))
		for k, o := range op.Src {
			if v, ok := o.(*ir.Value); ok && rename[v] != nil {
				o = rename[v]
			}
			no.Src[k] = o
		}
		if v, ok := op.Dst.(*ir.Value); ok && rename[v] != nil {
			no.Dst = rename[v]
		}
		out = append(out, &no)
	}
	return out
}
