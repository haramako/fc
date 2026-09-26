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
			if d.Kind != ir.DefCode || !d.Lambda.Options.Flag("inline") {
				continue
			}
			lmd := d.Lambda
			switch {
			case lmd.Extern:
				return &diag.Error{Msg: fmt.Sprintf("inline function %s has no body", lmd.Name), Pos: lmd.Pos}
			case lmd.Options.Flag("interrupt"):
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
	owners := defOwners(mods)
	for _, m := range mods {
		for _, d := range m.Defs {
			if d.Kind != ir.DefCode || d.Lambda.Extern {
				continue
			}
			// inline 関数が inline 関数を呼ぶ形は繰り返して展開する (相互再帰は回数で打ち切る)
			for depth := 0; depth < 8; depth++ {
				if !inlineCalls(d.Lambda, inl, owners) {
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
	if lmd.Extern || lmd.Options.Flag("interrupt") || lmd.Options.Flag("inline") || lmd.Options.Flag("noinline") || len(lmd.Ops) == 0 {
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

// hasCallsOrOpaqueAsm は本体に呼び出しか、flagOnlyAsm でない asm があるか。別モジュールへの展開の判定に使う
// (asm の中身は解析しないので、中で jsr したり元のモジュールだけに見えるシンボルを参照したりしうる。自動インラインは
// asm を含む関数を最初から対象にしない (autoInlinable) ので、これが効くのは options(inline: true) の関数だけ)。
func hasCallsOrOpaqueAsm(lmd *ir.Lambda) bool {
	for _, op := range lmd.Ops {
		if isCall(op) || (op != nil && op.Code == ir.OpAsm && !flagOnlyAsm(op.Text)) {
			return true
		}
	}
	return false
}

// flagOnlyAsm は asm の本文が、フラグだけを変えてレジスタ・スタック・メモリを触らない命令 (オペランド無し) だけか
// (castle の mmc3.set_pbank の sei / cli)。rts / pha / tax などは写した先の意味が変わるので含めない。
func flagOnlyAsm(text string) bool {
	for _, line := range strings.Split(text, "\n") {
		if i := strings.IndexByte(line, ';'); i >= 0 {
			line = line[:i]
		}
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		if len(f) > 1 || !flagOnlyMnemonics[strings.ToLower(f[0])] {
			return false
		}
	}
	return true
}

var flagOnlyMnemonics = map[string]bool{"sei": true, "cli": true, "clc": true, "sec": true, "cld": true, "sed": true, "clv": true, "nop": true}

// defOwner はモジュールレベルの定義 (シンボル) の置き場所: どのモジュールの、どの種類の定義か。
type defOwner struct {
	mod *ir.Module
	def *ir.Def
}

// defOwners は全モジュールの定義をシンボルで引ける表にする (別モジュールへのインライン化で、本体が読むデータが
// 写した先から見えるかの判定に使う)。
func defOwners(mods []*ir.Module) map[string]defOwner {
	owners := map[string]defOwner{}
	for _, m := range mods {
		for _, d := range m.Defs {
			owners[d.Sym] = defOwner{mod: m, def: d}
		}
	}
	return owners
}

// dataReachable は callee の本体を caller (別のモジュール) に写しても、本体が触るデータが全部見えるか。
//   - RAM (var = DefBss、数値の address: = DefEqu) はバンクに関係なく見える
//   - ROM (const の表 = DefBlock、関数内の文字列などの Defs) は、その持ち主のモジュールが固定バンク (Switchable でない) か、
//     写す先と同じモジュールのときだけ見える。segment: 指定の const は置き場所が分からないので不可
//   - asm のシンボルに束縛したもの (DefExtern、シンボルの address:) は置き場所が分からないので不可
//   - 関数のアドレス (fn 型の値) は数値なので可。ポインタの指す先は呼び出しのままでも同じ条件なので見ない
func dataReachable(callee, caller *ir.Lambda, owners map[string]defOwner) bool {
	at := caller.Module
	if caller.Options.Has("segment") {
		at = nil // 置き場所が別 (sema の placementOf と同じく手動配置): 固定バンクのデータだけ
	}
	romOK := func(owner *ir.Module) bool { return !owner.Switchable() || owner == at }
	for _, d := range callee.Defs {
		if d.Kind == ir.DefBlock && !romOK(callee.Module) {
			return false
		}
	}
	check := func(o ir.Operand) bool {
		for {
			if pa, ok := o.(*ir.PointeredArray); ok {
				o = pa.From
				continue
			}
			break
		}
		v := ir.UnderlyingValue(o)
		if v == nil || v.Kind != ir.KindGlobal || v.Symbol == "" {
			return true
		}
		own, ok := owners[v.Symbol]
		if !ok {
			return false // 出どころが分からない
		}
		switch own.def.Kind {
		case ir.DefBss:
			return true
		case ir.DefEqu:
			return own.def.Equ != nil && own.def.Equ.IsInt
		case ir.DefBlock:
			return own.def.Segment == "" && romOK(own.mod)
		}
		return false // DefExtern など
	}
	for _, op := range callee.Ops {
		if op == nil {
			continue
		}
		defs, uses := ir.DefUse(op)
		for _, o := range defs {
			if !check(o) {
				return false
			}
		}
		for _, o := range uses {
			if !check(o) {
				return false
			}
		}
	}
	return true
}

// inlineCalls は caller の中の inline 関数の呼び出しを 1 巡展開する (展開したら true)。
func inlineCalls(caller *ir.Lambda, inl map[string]*ir.Lambda, owners map[string]defOwner) bool {
	done := false
	for k := 0; k < len(caller.Ops); k++ {
		op := caller.Ops[k]
		callee := inl[calleeSym(op)]
		if callee == nil || callee == caller || caller.NoGrow {
			continue
		}
		if callee.Module != caller.Module && (hasCallsOrOpaqueAsm(callee) || !dataReachable(callee, caller, owners)) {
			// far call の判定が呼び先のモジュール基準なので、呼び出し (と sei / cli 以外の asm) を含む本体は別モジュールに写せない。
			// 本体が読む ROM のデータ (const の表、文字列) が写した先から見えない (別の切替バンク) ときも写せない
			// (コードだけ移って表は元のバンクに残り、farcall のバンク切替が消えて別の表を読んでいた)
			continue
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
	// 関数の中の const の表・文字列 (callee.Defs。callee の .proc の中のラベルとして出る) は、展開先から見えない
	// (callee が使われなくなって出力されないこともある) ので、展開ごとに呼び出し側の Defs へ別の名前で写す
	symMap := map[string]string{}
	for _, d := range callee.Defs {
		symMap[d.Sym] = fmt.Sprintf("_i%d_%s", n, strings.TrimLeft(d.Sym, "_"))
	}
	symVals := map[*ir.Value]*ir.Value{}
	var mapOperand func(o ir.Operand) ir.Operand
	mapOperand = func(o ir.Operand) ir.Operand {
		switch x := o.(type) {
		case nil:
			return nil
		case *ir.Value:
			if nv, ok := vmap[x]; ok {
				return nv
			}
			if ns, ok := symMap[x.Symbol]; ok && x.Symbol != "" && (x.Kind == ir.KindGlobal || x.Kind == ir.KindLiteral) {
				if nv := symVals[x]; nv != nil {
					return nv
				}
				nv := *x
				nv.Symbol = ns
				symVals[x] = &nv
				return &nv
			}
			return x
		case *ir.CastedValue:
			return ir.RebaseCast(x, mapOperand(x.From))
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
	for _, d := range callee.Defs {
		nd := *d
		nd.Sym = symMap[d.Sym]
		nd.Elems = make([]ir.Operand, len(d.Elems))
		for k, e := range d.Elems {
			nd.Elems[k] = mapOperand(e) // 表の要素が関数の中の別の表・文字列を指すとき
		}
		caller.Defs = append(caller.Defs, &nd)
	}

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
		no.Logs = ir.CloneLogs(op.Logs, mapOperand)
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
