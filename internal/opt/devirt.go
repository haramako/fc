package opt

// 関数ポインタ表の呼び出しの直接化 (devirtualization。doc/v2_ssa.md §8):
//
//	index p = &TBL[i]; pget f = *p          TBL は const の表で要素が全部関数 (castle の en_vtbl.PROCESS)
//	push_result; push_arg ..; call d = f
//	→
//	switch i, #0, [@dv_0, @dv_1, ...]; jump @dv_end   (範囲外は何もしない: 配列の範囲外は元から未定義)
//	@dv_0: push_result; push_arg ..; call d = TBL[0]; jump @dv_end
//	@dv_1: ...
//	@dv_end:
//
// 間接呼び出し (ポインタの読み出し + jmp (ptr)、引数はスタック経由で呼び先のプロローグがフレームに写す) が
// ジャンプテーブル + 直接呼び出し (引数はフレームに直接、最後の引数は A、__direct から入る) になる。
// 表が 16 要素以下で、push と call の間に他の呼び出しが無い (引数の式は先に評価済み) ときだけ。
// far call になる呼び先には Far を付ける (sema の isFarCall と同じ規則)。

import (
	"fmt"

	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/types"
)

const devirtMaxTable = 16

// DevirtualizeProgram は全モジュールの関数について表経由の呼び出しを直接化する (InlineProgram の後、frames.Analyze の前)。
// farEnabled は options(farcall: true)。
func DevirtualizeProgram(mods []*ir.Module, farEnabled bool) {
	if ir.Disabled("devirt") {
		return
	}
	byID := map[string]*ir.Module{}
	lambdas := map[string]*ir.Lambda{}
	tables := map[string][]string{} // 表のシンボル → 要素の関数シンボル (関数でない要素があれば nil)
	addTable := func(d *ir.Def) {
		if d.Kind != ir.DefBlock {
			return
		}
		syms := make([]string, 0, len(d.Elems))
		for _, e := range d.Elems {
			v := ir.ValLiteral(e)
			if v == nil || v.Kind != ir.KindLiteral || v.IsInt || v.Symbol == "" {
				tables[d.Sym] = nil
				return
			}
			syms = append(syms, v.Symbol)
		}
		tables[d.Sym] = syms
	}
	for _, m := range mods {
		byID[m.Id] = m
		for _, d := range m.Defs {
			addTable(d)
			if d.Kind == ir.DefCode {
				lambdas[d.Sym] = d.Lambda
				for _, ld := range d.Lambda.Defs {
					addTable(ld)
				}
			}
		}
	}
	placement := func(lmd *ir.Lambda) *ir.Module {
		if seg := lmd.Segment(); seg != "" {
			return byID[seg] // モジュールでなければ nil (手動配置 = near)
		}
		return lmd.Module
	}
	isFar := func(caller, callee *ir.Lambda) bool {
		if !farEnabled || callee.Options.Has("near") {
			return false
		}
		at, from := placement(callee), placement(caller)
		return at != nil && at != from && at.Switchable()
	}
	for _, m := range mods {
		for _, d := range m.Defs {
			if d.Kind == ir.DefCode && !d.Lambda.Extern && len(d.Lambda.Ops) > 0 {
				devirtualize(d.Lambda, tables, lambdas, isFar)
			}
		}
	}
}

func devirtualize(lmd *ir.Lambda, tables map[string][]string, lambdas map[string]*ir.Lambda, isFar func(caller, callee *ir.Lambda) bool) {
	for n := 0; n < 32; n++ {
		if !devirtualizeOne(lmd, tables, lambdas, isFar) {
			return
		}
	}
}

func devirtualizeOne(lmd *ir.Lambda, tables map[string][]string, lambdas map[string]*ir.Lambda, isFar func(caller, callee *ir.Lambda) bool) bool {
	ops := lmd.Ops
	ud := ir.BuildUseDef(lmd)
	for i, op := range ops {
		if op == nil || op.Code != ir.OpIndex || i+2 >= len(ops) {
			continue
		}
		tbl, ok := op.Src[0].(*ir.Value)
		if !ok || tbl.Kind != ir.KindGlobal || tbl.Symbol == "" || tbl.Type.Kind != types.Array || tbl.Type.Base.Kind != types.Func {
			continue
		}
		syms := tables[tbl.Symbol]
		if syms == nil || len(syms) > devirtMaxTable || ir.ValType(op.Src[1]).Size != 1 {
			continue
		}
		p, ok := op.Dst.(*ir.Value)
		if !ok || p.LocalType != ir.LTTemp {
			continue
		}
		pget := ops[i+1]
		if pget == nil || pget.Code != ir.OpPget || pget.Src[0] != ir.Operand(p) || len(ud.Uses[p]) != 1 {
			continue
		}
		f, ok := pget.Dst.(*ir.Value)
		if !ok || f.LocalType != ir.LTTemp || len(ud.Uses[f]) != 1 || len(ud.Defs[f]) != 1 {
			continue
		}
		// pget の直後から call まで: push_result と push_arg だけ
		c := i + 2
		if ops[c] == nil || (ops[c].Code != ir.OpPushResult && ops[c].Code != ir.OpPushFastcallResult) {
			continue
		}
		e := -1
		for k := c + 1; k < len(ops); k++ {
			o := ops[k]
			if o == nil {
				continue
			}
			if (o.Code == ir.OpCall || o.Code == ir.OpFastcall) && o.Src[0] == ir.Operand(f) {
				e = k
				break
			}
			if o.Code != ir.OpPushArg && o.Code != ir.OpPushFastcallArg {
				break
			}
		}
		if e < 0 {
			continue
		}
		// 呼び先が全部分かること (extern でも直接呼べる。cc65 規約の関数のアドレスは sema がエラーにしている)
		callees := make([]*ir.Lambda, len(syms))
		for k, s := range syms {
			callees[k] = lambdas[s]
			if callees[k] == nil {
				break
			}
		}
		if callees[len(callees)-1] == nil {
			continue
		}
		// 書き換え
		base := maxLabelNumber(lmd)
		label := func(k int) string { return fmt.Sprintf("@dv_%d", base+1+k) }
		end := fmt.Sprintf("@dv_%d", base+1+len(syms))
		call := ops[e]
		var out []*ir.Op
		out = append(out, ops[:i]...)
		if len(syms) > 1 {
			labels := make([]string, len(syms))
			for k := range syms {
				labels[k] = label(k)
			}
			out = append(out, &ir.Op{Code: ir.OpSwitch, Src: []ir.Operand{op.Src[1], ir.NewIntLiteral("", ir.ValType(op.Src[1]), 0)}, Labels: labels, Pos: call.Pos})
			out = append(out, &ir.Op{Code: ir.OpJump, Label: end, Pos: call.Pos})
		}
		for k, s := range syms {
			if len(syms) > 1 {
				out = append(out, &ir.Op{Code: ir.OpLabel, Label: label(k), Pos: call.Pos})
			}
			for j := c; j <= e; j++ {
				o := ops[j]
				if o == nil {
					continue
				}
				no := *o
				no.Src = append([]ir.Operand{}, o.Src...)
				if j == e {
					no.Src[0] = ir.NewSymbolLiteral(callees[k].Name, f.Type, s)
					no.Far = isFar(lmd, callees[k])
				}
				out = append(out, &no)
			}
			if len(syms) > 1 {
				out = append(out, &ir.Op{Code: ir.OpJump, Label: end, Pos: call.Pos})
			}
		}
		if len(syms) > 1 {
			out = append(out, &ir.Op{Code: ir.OpLabel, Label: end, Pos: call.Pos})
		}
		out = append(out, ops[e+1:]...)
		lmd.Ops = out
		return true
	}
	return false
}
