package driver

// fcc build --size-report: セグメントと関数の大きさ (cc65.DbgFile.SizeReport) に、バンクとモジュールの組み合わせを考えるための
// 表を足す: ROM の領域ごとの使用量・空きと置いたモジュール (cc65.DbgFile.BankReport)、モジュールの間の呼び出しの数 (最適化の
// 後の呼び出し。バンクをまたぐ far call の数も)。

import (
	"fmt"
	"sort"

	"github.com/haramako/fc/internal/cc65"
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/sizehtml"
)

// sizeReport は --size-report の行。
func (c *compilation) sizeReport(dbg *cc65.DbgFile) []string {
	r := dbg.SizeReport(40)
	lc, err := cc65.ReadLinkConfig(c.linkCfg)
	if err != nil {
		return r
	}
	r = append(r, dbg.BankReport(lc)...)
	return append(r, moduleCallReport(moduleCalls(c.prog.Modules.List()), lc)...)
}

// writeSizeHTML は --size-html のページを path に書く。
func (c *compilation) writeSizeHTML(dbg *cc65.DbgFile, path, title string) error {
	lc, err := cc65.ReadLinkConfig(c.linkCfg)
	if err != nil {
		lc = nil // リンカ設定が読めなければ領域に分けない
	}
	rep := sizehtml.FromDbg(title, dbg, lc)
	rep.Calls, rep.HasCalls = moduleCalls(c.prog.Modules.List()), true
	return sizehtml.WriteFile(path, rep)
}

// moduleCalls はモジュールの組ごとの呼び出しの数 (呼ぶ側 → 呼ばれる側。同じモジュールの中は数えない)。インライン展開した
// 呼び出しは消えているので数えず、ROM に出さない関数 (Lambda.Unused) の呼び出しも数えない。回数の多い順。
func moduleCalls(mods []*ir.Module) []sizehtml.Call {
	byID := map[string]*ir.Lambda{}
	for _, m := range mods {
		for _, d := range m.Defs {
			if d.Kind == ir.DefCode && d.Lambda != nil {
				byID[d.Lambda.Id] = d.Lambda
			}
		}
	}
	type pair struct{ from, to string }
	counts := map[pair]*sizehtml.Call{}
	for _, m := range mods {
		for _, d := range m.Defs {
			if d.Kind != ir.DefCode || d.Lambda == nil || d.Lambda.Unused {
				continue
			}
			for _, op := range d.Lambda.Ops {
				if op == nil || op.Code != ir.OpCall && op.Code != ir.OpFastcall {
					continue
				}
				v := ir.ValLiteral(op.Src[0])
				if v == nil || v.Kind != ir.KindLiteral || v.Symbol == "" {
					continue // 関数ポインタ
				}
				callee := byID[v.Symbol]
				if callee == nil || callee.Module == nil || callee.Module == m {
					continue
				}
				k := pair{m.Id, callee.Module.Id}
				if counts[k] == nil {
					counts[k] = &sizehtml.Call{From: k.from, To: k.to}
				}
				counts[k].Count++
				if op.Far {
					counts[k].Far++
				}
			}
		}
	}
	r := make([]sizehtml.Call, 0, len(counts))
	for _, c := range counts {
		r = append(r, *c)
	}
	sort.Slice(r, func(i, j int) bool {
		if r[i].Count != r[j].Count {
			return r[i].Count > r[j].Count
		}
		if r[i].From != r[j].From {
			return r[i].From < r[j].From
		}
		return r[i].To < r[j].To
	})
	return r
}

// moduleCallReport は moduleCalls の表示。モジュールの後の括弧は置いた ROM の領域 (リンカ設定のセグメントの load)。
func moduleCallReport(calls []sizehtml.Call, lc *cc65.LinkConfig) []string {
	if len(calls) == 0 {
		return nil
	}
	where := func(mod string) string {
		if area := lc.Load[mod]; area != "" {
			return fmt.Sprintf("%s (%s)", mod, area)
		}
		return mod
	}
	r := []string{"calls between modules (call sites after inlining; far = through the bank trampoline):"}
	for _, n := range calls {
		far := ""
		if n.Far > 0 {
			far = fmt.Sprintf("  far %d", n.Far)
		}
		r = append(r, fmt.Sprintf("  %-24s -> %-24s %5d%s", where(n.From), where(n.To), n.Count, far))
	}
	return r
}
