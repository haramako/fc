package opt

import (
	"fmt"
	"strings"

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/ir"
)

// InlineProgram は `options(inline: true)` の関数の呼び出しを、呼び出し側に本体を写して置き換える
// (codegen.PrepareProgram で frames.Analyze の前に 1 回。展開された関数はどこからも呼ばれなければ出力されない)。
//
//	push_result; push_arg a; call t = f     →  load f.i = a; <f の本体 (変数とラベルは付け替え、return v は load f.$result = v;
//	                                              jump @iN_end)>; @iN_end:; load t = f.$result
//
// 展開しない場合 (呼び出しのまま残す): 別のモジュール (= 別のセグメント) の関数で本体に呼び出しがあるもの
// (far call の判定は呼び先のモジュール基準で付いているので、写すと狂う)、別の呼び出しの引数の並びの中
// (push_result と call の間。fastcall の引数領域を壊す)、自分自身。
// エラー: extern / interrupt / 再帰の関数への options(inline: true)。
func InlineProgram(mods []*ir.Module) error {
	inl := map[string]*ir.Lambda{} // シンボル → inline 関数
	for _, m := range mods {
		for _, d := range m.Defs {
			if d.Kind != ir.DefCode || !d.Lambda.Options.Has("inline") {
				continue
			}
			lmd := d.Lambda
			switch {
			case lmd.Extern:
				return &diag.Error{Msg: fmt.Sprintf("inline function %s has no body", lmd.Name), Pos: lmd.Pos}
			case lmd.Options.Has("interrupt"):
				return &diag.Error{Msg: fmt.Sprintf("inline function %s cannot be an interrupt handler", lmd.Name), Pos: lmd.Pos}
			case callsTo(lmd, lmd.Id):
				return &diag.Error{Msg: fmt.Sprintf("inline function %s is recursive", lmd.Name), Pos: lmd.Pos}
			}
			inl[lmd.Id] = lmd
		}
	}
	if len(inl) == 0 {
		return nil
	}
	for _, m := range mods {
		for _, d := range m.Defs {
			if d.Kind != ir.DefCode || d.Lambda.Extern {
				continue
			}
			// inline 関数が inline 関数を呼ぶ形は繰り返して展開する (相互再帰は回数で打ち切る)
			for depth := 0; depth < 8; depth++ {
				if !inlineCalls(d.Lambda, inl) {
					break
				}
			}
		}
	}
	return nil
}

// callsTo は lmd の本体に sym への直接呼び出しがあるか。
func callsTo(lmd *ir.Lambda, sym string) bool {
	for _, op := range lmd.Ops {
		if calleeSym(op) == sym {
			return true
		}
	}
	return false
}

// calleeSym は call / fastcall の直接の呼び先のシンボル ("" なら関数ポインタ経由など)。
func calleeSym(op *ir.Op) string {
	if op == nil || (op.Code != ir.OpCall && op.Code != ir.OpFastcall) {
		return ""
	}
	lit := ir.ValLiteral(op.Src[0])
	if lit == nil || lit.Kind != ir.KindLiteral || lit.IsInt {
		return ""
	}
	return lit.Symbol
}

func isPushResult(op *ir.Op) bool {
	return op != nil && (op.Code == ir.OpPushResult || op.Code == ir.OpPushFastcallResult)
}

func isPushArg(op *ir.Op) bool {
	return op != nil && (op.Code == ir.OpPushArg || op.Code == ir.OpPushFastcallArg)
}

func isCall(op *ir.Op) bool {
	return op != nil && (op.Code == ir.OpCall || op.Code == ir.OpFastcall)
}

// hasCalls は本体に呼び出し (またはインラインアセンブラ) があるか。
func hasCalls(lmd *ir.Lambda) bool {
	for _, op := range lmd.Ops {
		if isCall(op) || (op != nil && op.Code == ir.OpAsm) {
			return true
		}
	}
	return false
}

// inlineCalls は caller の中の inline 関数の呼び出しを 1 巡展開する (展開したら true)。
func inlineCalls(caller *ir.Lambda, inl map[string]*ir.Lambda) bool {
	done := false
	for k := 0; k < len(caller.Ops); k++ {
		op := caller.Ops[k]
		callee := inl[calleeSym(op)]
		if callee == nil || callee == caller {
			continue
		}
		if callee.Module != caller.Module && hasCalls(callee) {
			continue // far call の判定が呼び先のモジュール基準なので、呼び出しを含む本体は別モジュールに写せない
		}
		// この呼び出しの push_result と引数を探す (引数の評価の中の呼び出しは深さで飛ばす)
		p, depth := -1, 0
		var args []int
		for j := k - 1; j >= 0; j-- {
			o := caller.Ops[j]
			switch {
			case isCall(o):
				depth++
			case isPushResult(o) && depth > 0:
				depth--
			case isPushResult(o):
				p = j
			case isPushArg(o) && depth == 0:
				args = append(args, j)
			}
			if p >= 0 {
				break
			}
		}
		if p < 0 || len(args) != len(callee.Args) {
			continue
		}
		// 別の呼び出しの引数の並びの中 (push_result … call の間) なら展開しない
		pending := 0
		for j := p - 1; j >= 0; j-- {
			o := caller.Ops[j]
			if isCall(o) {
				pending++
			} else if isPushResult(o) {
				if pending == 0 {
					pending = -1
					break
				}
				pending--
			}
		}
		if pending < 0 {
			continue
		}
		// args は後ろから集めたので前から並べ直す
		for i, j := 0, len(args)-1; i < j; i, j = i+1, j-1 {
			args[i], args[j] = args[j], args[i]
		}
		expanded := expand(caller, callee, k, args)
		// ops[p..k] を expanded で置き換える
		out := make([]*ir.Op, 0, len(caller.Ops)+len(expanded))
		out = append(out, caller.Ops[:p]...)
		out = append(out, expanded...)
		out = append(out, caller.Ops[k+1:]...)
		caller.Ops = out
		k = p + len(expanded) - 1
		done = true
	}
	return done
}

// expand は callee の本体を caller 用に写した命令列 (引数の代入 + 本体 + 終端ラベル + 戻り値の取り出し)。
func expand(caller, callee *ir.Lambda, callIdx int, args []int) []*ir.Op {
	call := caller.Ops[callIdx]
	// 展開ごとの番号 (変数名とラベルを、呼び出し側や前の展開と衝突させない)
	n := 0
	for _, op := range caller.Ops {
		if op != nil && op.Code == ir.OpLabel && strings.HasPrefix(op.Label, "@i") && strings.HasSuffix(op.Label, "_end") {
			n++
		}
	}
	tag := fmt.Sprintf("@i%d_", n)
	// 変数の写し (引数・戻り値も普通のローカルに)
	vmap := map[*ir.Value]*ir.Value{}
	prefix := fmt.Sprintf("%s%d.", callee.Name, n)
	for _, v := range callee.Vars {
		if v.Kind != ir.KindLocal {
			continue
		}
		lt := v.LocalType
		if lt == ir.LTArg || lt == ir.LTResult {
			lt = ir.LTNone
		}
		nv := ir.NewLocal(prefix+v.Name, v.Type, lt)
		nv.Volatile = v.Volatile
		vmap[v] = nv
		caller.Vars = append(caller.Vars, nv)
	}
	var mapOperand func(o ir.Operand) ir.Operand
	mapOperand = func(o ir.Operand) ir.Operand {
		switch x := o.(type) {
		case nil:
			return nil
		case *ir.Value:
			if nv, ok := vmap[x]; ok {
				return nv
			}
			return x
		case *ir.CastedValue:
			return ir.NewCastedValue(mapOperand(x.From), x.Type, x.Offset)
		case *ir.PointeredArray:
			return ir.NewPointeredArray(mapOperand(x.From), x.Type)
		}
		return o
	}
	label := func(l string) string {
		if l == "" {
			return ""
		}
		return tag + l[1:]
	}
	end := tag + "end"

	var out []*ir.Op
	for i, j := range args {
		a := caller.Ops[j]
		out = append(out, &ir.Op{Code: ir.OpLoad, Dst: vmap[callee.Args[i]], Src: []ir.Operand{a.Src[0]}, Pos: a.Pos})
	}
	var result *ir.Value
	if callee.Result != nil {
		result = vmap[callee.Result]
	}
	for i, op := range callee.Ops {
		if op == nil {
			continue
		}
		if op.Code == ir.OpReturn {
			if len(op.Src) > 0 && result != nil {
				if v := mapOperand(op.Src[0]); v != ir.Operand(result) {
					out = append(out, &ir.Op{Code: ir.OpLoad, Dst: result, Src: []ir.Operand{v}, Pos: op.Pos})
				}
			}
			if i != len(callee.Ops)-1 {
				out = append(out, &ir.Op{Code: ir.OpJump, Label: end, Pos: op.Pos})
			}
			continue
		}
		no := *op
		no.Src = make([]ir.Operand, len(op.Src))
		for s, o := range op.Src {
			no.Src[s] = mapOperand(o)
		}
		no.Dst = mapOperand(op.Dst)
		no.Label = label(op.Label)
		out = append(out, &no)
	}
	out = append(out, &ir.Op{Code: ir.OpLabel, Label: end, Pos: call.Pos})
	if call.Dst != nil && result != nil {
		out = append(out, &ir.Op{Code: ir.OpLoad, Dst: call.Dst, Src: []ir.Operand{result}, Pos: call.Pos})
	}
	return out
}
