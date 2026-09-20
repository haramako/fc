package opt

import (
	"fmt"
	"strings"

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/types"
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
	if ir.Disabled("inline") {
		return nil
	}
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
	if !ir.Disabled("autoinline") {
		// 自動インライン: 印が無くても小さい関数 (autoInlinable) は展開する。呼び出し 1 回あたり 20〜30 サイクル
		// (引数の受け渡し + jsr / rts + 戻り値) が消える。呼び出し箇所が多い関数は本体が特に小さいときだけ (ROM)
		sites := map[string]int{}
		for _, m := range mods {
			for _, d := range m.Defs {
				if d.Kind == ir.DefCode {
					for _, op := range d.Lambda.Ops {
						if sym := calleeSym(op); sym != "" {
							sites[sym]++
						}
					}
				}
			}
		}
		for _, m := range mods {
			for _, d := range m.Defs {
				if d.Kind != ir.DefCode || inl[d.Sym] != nil {
					continue
				}
				if n, ok := autoInlinable(d.Lambda); ok && (n <= autoInlineSmall || sites[d.Sym] <= autoInlineFewSites) {
					inl[d.Lambda.Id] = d.Lambda
				}
			}
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

const (
	autoInlineMaxOps   = 12 // 自動インラインする本体の命令数の上限 (ラベル・jump を除く)
	autoInlineSmall    = 6  // これ以下なら呼び出し箇所がいくつあっても展開する
	autoInlineFewSites = 2  // それより大きい本体は呼び出し箇所がこれ以下のときだけ
)

// autoInlinable は印の無い関数を自動で展開してよいか (本体の命令数も返す): 本体が小さく、ループ・呼び出し・asm・
// アドレス取得・配列 / struct のローカルが無く、interrupt / 再帰 / extern でないもの。
func autoInlinable(lmd *ir.Lambda) (int, bool) {
	if lmd.Extern || lmd.Options.Has("interrupt") || lmd.Options.Has("inline") || lmd.Options.Has("noinline") || len(lmd.Ops) == 0 {
		return 0, false
	}
	if lmd.Options.Has("segment") || lmd.Options.Has("symbol") || lmd.Options.Has("abi") {
		return 0, false // 置き場所や規約を指定した関数はそのまま
	}
	n := 0
	for _, op := range lmd.Ops {
		if op == nil {
			continue
		}
		switch op.Code {
		case ir.OpLabel, ir.OpJump:
		case ir.OpAsm, ir.OpCall, ir.OpFastcall, ir.OpRef:
			return 0, false
		default:
			n++
		}
	}
	if n > autoInlineMaxOps {
		return 0, false
	}
	for _, v := range lmd.Vars {
		if v.Kind == ir.KindLocal && (v.Type.Kind == types.Array || v.Type.Kind == types.Struct) {
			return 0, false
		}
	}
	if len(ir.BuildCFG(lmd).Loops()) > 0 {
		return 0, false
	}
	return n, true
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
	// Far pointers preserve mapping/restoration even when the symbol is known.
	if ir.ValType(op.Src[0]).IsFarFunc() {
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
		argLoads, body := expand(caller, callee, k, args)
		// ops[p..k] を置き換える: push_result は消し、push_arg はその場で引数の変数への代入に、それ以外 (引数の式の計算や
		// 引数の中の別の呼び出し) はそのまま残し、call の位置に本体を置く
		argAt := map[int]int{}
		for i, j := range args {
			argAt[j] = i
		}
		out := make([]*ir.Op, 0, len(caller.Ops)+len(body))
		out = append(out, caller.Ops[:p]...)
		for j := p + 1; j < k; j++ {
			if i, ok := argAt[j]; ok {
				out = append(out, argLoads[i])
			} else {
				out = append(out, caller.Ops[j])
			}
		}
		out = append(out, body...)
		next := len(out) - 1
		out = append(out, caller.Ops[k+1:]...)
		caller.Ops = out
		k = next
		done = true
	}
	return done
}

// expand は callee の本体を caller 用に写した命令列: 引数の代入 (push_arg ごとに 1 つ。呼び出し側がその push_arg の位置に
// 置く) と、本体 + 終端ラベル + 戻り値の取り出し。
func expand(caller, callee *ir.Lambda, callIdx int, args []int) (argLoads, body []*ir.Op) {
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

	for i, j := range args {
		a := caller.Ops[j]
		argLoads = append(argLoads, &ir.Op{Code: ir.OpLoad, Dst: vmap[callee.Args[i]], Src: []ir.Operand{a.Src[0]}, Pos: a.Pos})
	}
	var out []*ir.Op
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
		if len(op.Labels) > 0 {
			no.Labels = make([]string, len(op.Labels))
			for k, l := range op.Labels {
				no.Labels[k] = label(l)
			}
		}
		out = append(out, &no)
	}
	out = append(out, &ir.Op{Code: ir.OpLabel, Label: end, Pos: call.Pos})
	if call.Dst != nil && result != nil {
		out = append(out, &ir.Op{Code: ir.OpLoad, Dst: call.Dst, Src: []ir.Operand{result}, Pos: call.Pos})
	}
	return argLoads, out
}
