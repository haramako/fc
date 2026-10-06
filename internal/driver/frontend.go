package driver

// 前段 (意味解析 → 全関数の最適化と割付 → 静的フレームの配置) の共通の入口。
//
// fcc build (BuildContext)、fcc check (compileNoWrite)、golden のテスト (newLlcForGolden) が同じ手順で
// sema.Program と codegen.Llc を用意する。以前は 3 か所に写しがあり、check だけ options(static_zp / static_ram /
// fastcall_reg) と far call を反映していなかった (build と check でフレーム超過の判定がずれ得た)。

import (
	"fmt"

	"github.com/haramako/fc/internal/codegen"
	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/frames"
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/pipeline"
	"github.com/haramako/fc/internal/project"
	"github.com/haramako/fc/internal/regalloc"
	"github.com/haramako/fc/internal/sema"
)

// frontOptions は前段の設定 (BuildOptions / CheckOptions から作る)。
type frontOptions struct {
	Main              string // 入口のファイル (基準ディレクトリとターゲットは compilation の c.dir / c.target)
	Defines           map[string]*sema.DefineUse
	OptimizeLevel     int  // 0 (-O 0) または 2
	Debug             bool // -g: @log の注釈を付ける
	LogEveryStatement bool
	Config            *ir.Config // 調査用の設定 (nil なら何も切らない)
	// 以下は codegen.Llc へそのまま渡す (BuildContext だけが使う)
	MisclassifyResident bool
	DebugFile           func(ref string) string
}

// frontResult は前段の結果。
type frontResult struct {
	Prog *sema.Program
	Llc  *codegen.Llc
	Plan *frames.Plan
}

// compileFront は意味解析から静的フレームの配置までを行う。-O 2 でフレームが上限を超えた関数があれば、その関数の展開を
// 止めて (Lambda.NoGrow) 意味解析からやり直す (最適化は IR をその場で書き換えるので、やり直しは sema から。
// 失敗したときだけ走るので、通るプログラムのコンパイル時間は変わらない)。
func (c *compilation) compileFront(o *frontOptions) (*frontResult, error) {
	macros, done, err := c.projectMacros()
	if err != nil {
		return nil, err
	}
	defer done() // 外部コマンドのマクロは意味解析の間だけ (やり直しでも同じプロセスと結果を使う)
	for noGrow := map[string]bool{}; ; {
		prog, err := c.newProgram(macros, o.Defines)
		if err != nil {
			return nil, err
		}
		prog.LogEnabled = o.Debug // @log の注釈は -g のときだけ (Agent/discussions/2026-09-20-v3-plan.md §9)
		prog.LogEveryStatement = o.LogEveryStatement
		prog.Config = o.Config
		if err := sema.CompileProgram(prog, c.dir, c.libPath(), o.Main); err != nil {
			return nil, err
		}
		if err := c.checkDefines(prog); err != nil {
			return nil, err
		}
		llc, err := newLlc(prog, o)
		if err != nil {
			return nil, err
		}
		res, perr := prepareProgram(prog, llc, noGrow)
		if retryFrameOver(res, perr, noGrow) {
			continue
		}
		if perr != nil {
			return nil, perr
		}
		prog.Warnings = append(prog.Warnings, res.Plan.Warnings...) // 割り込みと共有するフレームなど (frames.Place)
		return &frontResult{Prog: prog, Llc: llc, Plan: res.Plan}, nil
	}
}

// newLlc は prog のコード生成器を作る (PrepareProgram の前の設定まで)。
func newLlc(prog *sema.Program, o *frontOptions) (*codegen.Llc, error) {
	fastcallReg, err := fastcallRegSize(prog)
	if err != nil {
		return nil, err
	}
	llc := codegen.NewLlc(o.OptimizeLevel, prog.Types)
	llc.DebugFile = o.DebugFile
	llc.Limits.FastcallReg = fastcallReg
	llc.FarCall = prog.FarCallEnabled()
	llc.MisclassifyResident = o.MisclassifyResident
	return llc, nil
}

// prepareProgram は呼び出し規約の決定 → 全関数の最適化と割付 → 静的フレームの配置 (pipeline.Prepare。
// Agent/wiki/design/frame-alloc.md §6-4)。noGrow は展開をしない関数 (やり直しのとき)。
func prepareProgram(prog *sema.Program, llc *codegen.Llc, noGrow map[string]bool) (*pipeline.Result, error) {
	zp, err := staticZpSize(prog)
	if err != nil {
		return nil, err
	}
	ram, err := staticRamSize(prog)
	if err != nil {
		return nil, err
	}
	return pipeline.Prepare(prog.Modules.List(), &pipeline.Options{
		OptimizeLevel: llc.OptimizeLevel, Types: prog.Types, Limits: llc.Limits, FarCall: llc.FarCall,
		StaticZp: zp, StaticRam: ram, NoGrow: noGrow, Backend: llc,
	})
}

// retryFrameOver は Prepare の結果を見て、-O 2 のフレームが上限を超えた関数 (res.FrameOver) をまだ noGrow に
// 入れていなければ入れて true (展開を止めて sema からやり直す)。止めても超えるなら false
// (エラーをそのまま返す。-O 0 でも超える大きすぎる関数)。
func retryFrameOver(res *pipeline.Result, err error, noGrow map[string]bool) bool {
	if err == nil || res == nil || res.FrameOver == "" || noGrow[res.FrameOver] {
		return false
	}
	noGrow[res.FrameOver] = true
	return true
}

// 静的フレームの領域の既定の大きさ (fc が生成する base.asm と一致)。
//
// fc が生成する base.asm のゼロページ配置: $00-$0F L (stack 関数のレジスタ領域)、$10-$1F reg、$20-$2F FC_FASTCALL_REG
// (extern の fastcall 用、既定 16)、$30-$6F FC_SZP (静的フレーム、既定 64)、$70-$7F は `options(segment: "ZEROPAGE")` の
// 変数用に空けておく、$80-$FF スタック S。$00-$7F に `options(address:)` で固定番地の変数を置くのは、base.asm を自前で
// 持つプロジェクト (castle) だけにする (2026-09-16 決定。miku は固定番地をやめて BSS に)。
const (
	DefaultStaticZp  = 64
	DefaultStaticRam = 512
)

// optionInt は main モジュールの options(name: N) を範囲を確かめて読む (無ければ def)。
func optionInt(prog *sema.Program, name string, def, lo, hi int) (int, error) {
	n, ok := prog.Options.Int(name)
	if !ok {
		return def, nil
	}
	if n < lo || n > hi {
		return 0, &diag.Error{Msg: fmt.Sprintf("options(%s: %d): must be %d..%d", name, n, lo, hi)}
	}
	return n, nil
}

func staticZpSize(prog *sema.Program) (int, error) {
	return optionInt(prog, "static_zp", DefaultStaticZp, 0, 256)
}
func staticRamSize(prog *sema.Program) (int, error) {
	return optionInt(prog, "static_ram", DefaultStaticRam, 0, 8192)
}

// fastcallRegSize は FC_FASTCALL_REG の大きさ (options(fastcall_reg: N)。既定は regalloc.DefaultLimits)。
func fastcallRegSize(prog *sema.Program) (int, error) {
	return optionInt(prog, "fastcall_reg", regalloc.DefaultLimits.FastcallReg, 16, 128)
}

// validated は compileFront で範囲を確かめ済みの options の値を取り出す (base.s の雛形はその後に作る)。
func validated(n int, err error) int {
	if err != nil {
		panic(err)
	}
	return n
}

// newProgram は意味解析の状態を作る (プロジェクトのマクロ・@(build) の上書き・バンクの表。build・check・migrate で共通)。
func (c *compilation) newProgram(macros []sema.MacroSource, defs map[string]*sema.DefineUse) (*sema.Program, error) {
	prog := sema.NewProgram()
	for _, m := range macros {
		if err := prog.UseMacros(m); err != nil {
			return nil, err
		}
	}
	prog.Defines = project.CopyDefines(defs)
	prog.Banks = c.banks()
	return prog, nil
}
