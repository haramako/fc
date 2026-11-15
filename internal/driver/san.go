package driver

import (
	"github.com/haramako/fc/internal/cc65"
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/san"
)

// sanInfo は実行のときの検査 (internal/san) に渡す情報: リンクした後の関数の入口・静的フレームの番地と大きさ・引数と戻り値の
// 置き場所 (出力しない関数・番地の無い関数は入れない)。出力の番地は emu のホストとのやり取りの番地 ($fff0〜)。
func sanInfo(mods []*ir.Module, dbg *cc65.DbgFile) *san.Info {
	addr := map[string]int{}
	for _, s := range dbg.Symbols {
		if _, dup := addr[s.Name]; !dup {
			addr[s.Name] = s.Val
		}
	}
	info := &san.Info{OutLo: 0xfff0, OutHi: 0x10000}
	for _, m := range mods {
		for _, l := range m.Lambdas {
			if l.Unused {
				continue
			}
			f := &san.Func{Name: l.Id}
			sym := ir.Mangle(l.Id)
			if a, ok := addr[sym]; ok {
				f.Entries = append(f.Entries, a)
			}
			for _, e := range l.Conv.Entries {
				if a, ok := addr[sym+e.Suffix]; ok && e.Suffix != "" {
					f.Entries = append(f.Entries, a)
				}
			}
			if len(f.Entries) == 0 {
				continue
			}
			if l.Conv.ABI == ir.ABIStatic && l.FrameSize > 0 {
				base, ok := addr["FC_SRAM"]
				if l.FrameZp {
					base, ok = addr["FC_SZP"]
				}
				if ok {
					f.Frame, f.Size = base+l.FrameBase, l.FrameSize
					for _, p := range l.Conv.Params {
						f.Params = append(f.Params, san.Range{Off: p.Off, Size: p.Size})
					}
					if !l.Conv.Result.OnlyA {
						f.Result = l.Conv.Result.Size
					}
				}
			}
			info.Funcs = append(info.Funcs, f)
		}
	}
	return info
}
