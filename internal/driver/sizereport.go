package driver

// fcc build --size-report: セグメントと関数の大きさ (cc65.DbgFile.SizeReport) に、バンクとモジュールの組み合わせを考えるための
// 表を足す: ROM の領域ごとの使用量・空きと置いたモジュール (cc65.DbgFile.BankReport)、モジュールの間の呼び出しの数 (最適化の
// 後の呼び出し。バンクをまたぐ far call の数も)。

import (
	"fmt"
	"sort"

	"github.com/haramako/fc/internal/cc65"
	"github.com/haramako/fc/internal/ir"
)

// sizeReport は --size-report の行。
func (c *Compiler) sizeReport(dbg *cc65.DbgFile) []string {
	r := dbg.SizeReport(40)
	lc, err := cc65.ReadLinkConfig(c.linkCfg)
	if err != nil {
		return r
	}
	r = append(r, dbg.BankReport(lc)...)
	return append(r, moduleCallReport(c.prog.Modules.List(), lc)...)
}

// moduleCallReport はモジュールの組ごとの呼び出しの数 (呼ぶ側 → 呼ばれる側。同じモジュールの中は数えない)。モジュールの後の
// 括弧は置いた ROM の領域 (リンカ設定のセグメントの load)。インライン展開した呼び出しは消えているので数えない。
func moduleCallReport(mods []*ir.Module, lc *cc65.LinkConfig) []string {
	byID := map[string]*ir.Lambda{}
	for _, m := range mods {
		for _, d := range m.Defs {
			if d.Kind == ir.DefCode && d.Lambda != nil {
				byID[d.Lambda.Id] = d.Lambda
			}
		}
	}
	type pair struct{ from, to string }
	type count struct{ calls, far int }
	counts := map[pair]*count{}
	for _, m := range mods {
		for _, d := range m.Defs {
			if d.Kind != ir.DefCode || d.Lambda == nil || d.Lambda.Unused {
				continue // ROM に出さない関数 (どこからも届かない) の呼び出しは数えない
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
					counts[k] = &count{}
				}
				counts[k].calls++
				if op.Far {
					counts[k].far++
				}
			}
		}
	}
	if len(counts) == 0 {
		return nil
	}
	keys := make([]pair, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := counts[keys[i]], counts[keys[j]]
		if a.calls != b.calls {
			return a.calls > b.calls
		}
		if keys[i].from != keys[j].from {
			return keys[i].from < keys[j].from
		}
		return keys[i].to < keys[j].to
	})
	where := func(mod string) string {
		if area := lc.Load[mod]; area != "" {
			return fmt.Sprintf("%s (%s)", mod, area)
		}
		return mod
	}
	r := []string{"calls between modules (call sites after inlining; far = through the bank trampoline):"}
	for _, k := range keys {
		n := counts[k]
		far := ""
		if n.far > 0 {
			far = fmt.Sprintf("  far %d", n.far)
		}
		r = append(r, fmt.Sprintf("  %-24s -> %-24s %5d%s", where(k.from), where(k.to), n.calls, far))
	}
	return r
}
