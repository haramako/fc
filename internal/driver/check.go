package driver

// 警告の集約と fcc check。

import (
	"sort"

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/sema"
	"github.com/haramako/fc/internal/syntax"
)

// collectWarnings は構文検査 (syntax.Lint) と意味解析の警告を集めて、ファイル名・位置順に並べる。
func collectWarnings(prog *sema.Program) []diag.Warning {
	var ws []diag.Warning
	for _, src := range prog.Sources {
		for _, w := range syntax.Lint(src.File) {
			ws = append(ws, diag.Warning{Msg: w.Msg, Pos: syntax.At(src.File.Filename, w.Pos)})
		}
	}
	ws = append(ws, prog.Warnings...)
	sort.SliceStable(ws, func(i, j int) bool {
		a, b := ws[i].Pos, ws[j].Pos
		if a.Filename != b.Filename {
			return a.Filename < b.Filename
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.Col < b.Col
	})
	return ws
}

// CheckOptions は fcc check の設定。
type CheckOptions struct {
	Target  string   // emu (既定) / nes
	Dir     string   // ソースの基準ディレクトリ ("" なら作業ディレクトリ)
	Defines []string // CLI の -D (fcc build と同じく fc.toml の後に当てる)
	Config  *ir.Config // 調査用の設定 (nil なら環境変数から)
}

// Check は filename から始まるプログラムを意味解析・コード生成まで通し (ファイルは書かない)、
// 警告を返す。エラーがあれば error (*diag.Error)。
func (c *Compiler) Check(filename string, opt *CheckOptions) ([]diag.Warning, error) {
	target := opt.Target
	if target == "" {
		target = "emu"
	}
	cfg := opt.Config
	if cfg == nil {
		cfg = ir.ConfigFromEnv()
	}
	prog, err := c.compileNoWrite(opt.Dir, target, filename, opt.Defines, cfg)
	if err != nil {
		return nil, err
	}
	return collectWarnings(prog), nil
}

// compileNoWrite は意味解析からコード生成まで通す (ファイルは書かない)。前段は fcc build と同じ compileFront。
func (c *Compiler) compileNoWrite(dir, target, main string, cli []string, cfg *ir.Config) (prog *sema.Program, err error) {
	defer func() {
		if r := recover(); r != nil {
			if ce, ok := r.(*diag.Error); ok {
				err = ce
				return
			}
			panic(r)
		}
	}()
	c.dir = dir
	if c.dir == "" {
		c.dir = "."
	}
	defs, err := c.projectDefines(cli)
	if err != nil {
		return nil, err
	}
	front, err := c.compileFront(&frontOptions{Dir: dir, Target: target, Main: main, Defines: defs, OptimizeLevel: 2, Config: cfg})
	if err != nil {
		return nil, err
	}
	for _, mod := range front.Prog.Modules.List() {
		if mod.FromFcm {
			continue
		}
		if _, _, err := front.Llc.Compile(mod); err != nil {
			return nil, err
		}
	}
	return front.Prog, nil
}
