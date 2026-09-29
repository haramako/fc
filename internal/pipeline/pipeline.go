// Package pipeline は意味解析の後、コード生成の前にプログラム全体で行う処理の順序を持つ (doc/v2_frame_alloc.md §6-4):
//
//	インライン展開・関数ポインタ表の直接化 (opt) → asm から参照される変数を volatile に → 呼び出し規約の決定 (frames.Analyze)
//	→ 関数ごとに 最適化 (opt.Optimize) → 引数の Y 渡しの印 (codegen) → 常駐レジスタ (regalloc.AllocateResident)
//	→ 割付 (regalloc.AllocateRegister) → スタックの引数の検査 (codegen) → 静的フレームの配置 (frames.Place)
//
// 以前はこの順序が codegen.Llc (PrepareProgram / Prepare) にあり、codegen が opt と frames に依存していた。
// codegen が担うのは「引数を Y で渡せるか」と「積む引数が FC_STACK に収まるか」の呼び出しの計画だけで、Backend
// インタフェースで受け取る (pipeline は codegen を import しない)。
package pipeline

import (
	"fmt"
	"os"
	"strings"

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/frames"
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/opt"
	"github.com/haramako/fc/internal/regalloc"
	"github.com/haramako/fc/internal/types"
)

// Backend はコード生成側が持つ呼び出しの計画 (codegen.Llc が実装する)。
type Backend interface {
	// SetLambdas は全関数の表 (Id → 関数。frames.Analyze の結果) を渡す (呼び先の呼び出し規約を引く)。
	SetLambdas(map[string]*ir.Lambda)
	// MarkArgY は最後から 2 つ目の引数を Y で渡せる呼び出しに印を付ける (最適化の後、割付の前)。
	MarkArgY(lmd *ir.Lambda)
	// CheckStackPush はスタックに積む引数が FC_STACK に収まるかを検査する (超えれば frame size over の diag.Error を panic)。
	CheckStackPush(lmd *ir.Lambda)
}

// Options は Prepare の設定。
type Options struct {
	OptimizeLevel int // 0 なら最適化しない
	Types         *types.Universe
	Limits        regalloc.Limits // レジスタ領域の大きさ
	FarCall       bool            // options(farcall: true)
	StaticZp      int             // 静的フレームの領域の大きさ (FC_SZP / FC_SRAM)
	StaticRam     int
	// NoGrow は展開 (インライン展開・ループ展開) をしない関数の Id (前の Prepare で frame size over になった関数。
	// driver がやり直しのときに渡す)
	NoGrow  map[string]bool
	Backend Backend
}

// Result は Prepare の結果。エラーのときも FrameOver のために返す。
type Result struct {
	Plan    *frames.Plan          // 静的フレームの配置 (Inc を `_frames.inc` として書く)
	Lambdas map[string]*ir.Lambda // Id → 関数 (extern も含む)
	// FrameOver は -O 2 で frame size over になった関数の Id (エラーのときだけ。driver が NoGrow に足してやり直す)
	FrameOver string
}

// Prepare はコード生成の前にプログラム全体で 1 回行う処理。
func Prepare(mods []*ir.Module, o *Options) (*Result, error) {
	res := &Result{}
	if o.OptimizeLevel > 0 {
		for _, m := range mods {
			for _, d := range m.Defs {
				if d.Kind == ir.DefCode && o.NoGrow[d.Lambda.Id] {
					d.Lambda.NoGrow = true
				}
			}
		}
		keep := snapshotProgramLogs(mods) // @log の注釈を、置き換わった呼び出しから付け替える
		if err := opt.InlineProgram(mods); err != nil {
			return res, err
		}
		opt.DevirtualizeProgram(mods, o.FarCall) // 表経由の呼び出しを直接に (frames.Analyze が直接の辺として見る)
		keep()
	}
	markVolatile(mods)
	graph, err := frames.Analyze(mods)
	if err != nil {
		return res, err
	}
	res.Lambdas = graph.ByID
	o.Backend.SetLambdas(graph.ByID)
	for _, lmd := range graph.Lambdas {
		if lmd.Unused || lmd.Extern {
			continue // extern は abi: "frame" の asm の関数 (フレームの大きさは frames.Analyze が決めた)
		}
		if err := prepareLambda(lmd, o); err != nil {
			if o.OptimizeLevel > 0 && strings.HasPrefix(err.Msg, "frame size over") {
				res.FrameOver = lmd.Id
			}
			return res, err
		}
	}
	markUnusedGlobals(mods)
	res.Plan, err = frames.Place(graph, o.StaticZp, o.StaticRam)
	return res, err
}

// markUnusedGlobals は、出力する関数 (最適化の後の命令)・定数の表・equ・include した asm・インラインアセンブラのどれからも
// 参照されないモジュールの private な変数 (fc 4 の既定の BSS のもの: Def.Private) に Unused を付ける (codegen が領域を
// 取らない。@(test) の関数だけが使う大きなバッファなど、使わない機能の状態で RAM を食わないように)。
func markUnusedGlobals(mods []*ir.Module) {
	used := map[string]bool{}
	var visit func(o ir.Operand)
	visit = func(o ir.Operand) {
		switch x := o.(type) {
		case *ir.Value:
			// (private な配列定数の要素が参照するものも数える: 表そのものが使われなくても。控えめに残す)
			if x == nil {
				return
			}
			if x.Symbol != "" {
				used[x.Symbol] = true
			}
			for _, e := range x.Elems {
				visit(e)
			}
			if x.Home != nil {
				visit(x.Home) // レジスタに常駐させた一時変数のメモリ側 (入口・出口の写しが参照する)
			}
		case *ir.CastedValue:
			visit(x.From)
		case *ir.PointeredArray:
			visit(x.From)
		}
	}
	defs := func(ds []*ir.Def) {
		for _, d := range ds {
			switch d.Kind {
			case ir.DefBlock:
				for _, e := range d.Elems {
					visit(e)
				}
			case ir.DefEqu:
				visit(d.Equ)
			}
		}
	}
	for _, m := range mods {
		for _, s := range m.AsmSymbols {
			used[s] = true
		}
		defs(m.Defs)
		for _, d := range m.Defs {
			if d.Kind != ir.DefCode || d.Lambda.Unused {
				continue
			}
			lmd := d.Lambda
			defs(lmd.Defs)
			for _, op := range lmd.Ops {
				if op == nil {
					continue
				}
				if op.Code == ir.OpAsm {
					for _, s := range ir.AsmSymbols(op.Text) {
						used[s] = true
					}
				}
				visit(op.Dst)
				for _, s := range op.Src {
					visit(s)
				}
				for _, r := range op.Res {
					if r.V != nil {
						visit(r.V)
					}
				}
				for _, l := range op.Logs {
					for _, a := range l.Args {
						visit(a.Val)
						for _, b := range a.Bytes {
							visit(b)
						}
					}
				}
			}
		}
	}
	for _, m := range mods {
		if m.Cfg().Disabled("unused-globals") {
			continue
		}
		for _, d := range m.Defs {
			if (d.Kind == ir.DefBss || d.Kind == ir.DefBlock) && d.Private {
				d.Unused = !used[d.Sym]
			}
		}
	}
}

// prepareLambda は関数 1 つの最適化とレジスタ割付 (エラーは関数の位置を補完して返す)。
func prepareLambda(lmd *ir.Lambda, o *Options) (err *diag.Error) {
	defer func() {
		if r := recover(); r != nil {
			if ce, ok := r.(*diag.Error); ok {
				if !ce.Pos.IsValid() {
					ce.Pos = lmd.Pos
				}
				err = ce
				return
			}
			panic(r)
		}
	}()
	verify(lmd, "sema")
	opt.Optimize(lmd, o.OptimizeLevel, o.Types)
	opt.ZeroEmptyCasts(lmd)
	prev := ir.SnapshotLogs(lmd)
	o.Backend.MarkArgY(lmd) // 最適化で命令の並びが決まってから (割付は印を Y の clobber と見る)
	if o.OptimizeLevel > 0 {
		regalloc.AllocateResident(lmd)
		verify(lmd, "resident")
	}
	// -O 0 でも同じ割付器を使う (静的フレーム (ABIStatic) の関数はフレームでなく F_f の固定番地に置く必要があり、
	// 以前あった「全部フレーム」の簡易版は静的フレームの導入後は壊れていた)
	regalloc.AllocateRegister(lmd, o.Limits)
	regalloc.DeleteUnuse(lmd)
	verify(lmd, "regalloc")
	ir.KeepLogs(lmd, prev)
	o.Backend.CheckStackPush(lmd)
	if lmd.Cfg().DumpIR() {
		// 調査用: 最適化と割付の後の IR を stderr に出す (golden の allocir と同じ形式)
		fmt.Fprint(os.Stderr, ir.DumpAllocLambda(lmd.Module.Id, lmd.Id, lmd))
	}
	return nil
}

// verify は FC_VERIFY_IR のとき IR を検査する (ir.Verify。opt の各段の後は opt.Pass.Apply が行う)。
func verify(lmd *ir.Lambda, after string) {
	if !lmd.Cfg().VerifyIR() {
		return
	}
	if err := ir.Verify(lmd); err != nil {
		panic(&diag.Error{Msg: fmt.Sprintf("internal: IR verify after %s in %s: %v", after, lmd.Id, err)})
	}
}

// snapshotProgramLogs は全関数の命令の並びを覚え、返す関数で @log の注釈を付け替える (ir.KeepLogs)。
func snapshotProgramLogs(mods []*ir.Module) func() {
	type snap struct {
		lmd  *ir.Lambda
		prev []*ir.Op
	}
	var snaps []snap
	for _, m := range mods {
		for _, d := range m.Defs {
			if d.Kind == ir.DefCode {
				if prev := ir.SnapshotLogs(d.Lambda); prev != nil {
					snaps = append(snaps, snap{d.Lambda, prev})
				}
			}
		}
	}
	return func() {
		for _, s := range snaps {
			ir.KeepLogs(s.lmd, s.prev)
		}
	}
}

// markVolatile は asm (include したファイルとインラインアセンブラ) から参照されるグローバル変数を volatile にする
// (options(address:) と options(volatile: true) は sema が付けている)。割り込みや asm が書き換える変数をレジスタに
// 置いたままにしないため (doc/language_reference.md §2)。
func markVolatile(mods []*ir.Module) {
	syms := map[string]bool{}
	for _, m := range mods {
		for _, s := range m.AsmSymbols {
			syms[s] = true
		}
	}
	for _, m := range mods {
		for _, d := range m.Defs {
			if d.Kind != ir.DefCode || d.Lambda.Extern {
				continue
			}
			for _, op := range d.Lambda.Ops {
				if op != nil && op.Code == ir.OpAsm {
					for _, s := range ir.AsmSymbols(op.Text) {
						syms[s] = true
					}
				}
			}
		}
	}
	if len(syms) == 0 {
		return
	}
	mark := func(o ir.Operand) {
		if o == nil {
			return
		}
		if pa, ok := o.(*ir.PointeredArray); ok {
			o = pa.From
		}
		if v := ir.UnderlyingValue(o); v != nil && v.Kind == ir.KindGlobal && syms[v.Symbol] {
			v.Volatile = true
		}
	}
	for _, m := range mods {
		for _, d := range m.Defs {
			if d.Kind != ir.DefCode || d.Lambda.Extern {
				continue
			}
			for _, op := range d.Lambda.Ops {
				if op == nil {
					continue
				}
				mark(op.Dst)
				for _, s := range op.Src {
					mark(s)
				}
			}
		}
	}
}
