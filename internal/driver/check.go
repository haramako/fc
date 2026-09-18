package driver

// 警告の集約と fcc check。

import (
	"sort"

	"github.com/haramako/fc/internal/codegen"
	"github.com/haramako/fc/internal/diag"
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
	Target string // emu (既定) / nes
	Dir    string // ソースの基準ディレクトリ ("" なら作業ディレクトリ)
}

// Check は filename から始まるプログラムを意味解析・コード生成まで通し (ファイルは書かない)、
// 警告を返す。エラーがあれば error (*diag.Error)。
func (c *Compiler) Check(filename string, opt *CheckOptions) ([]diag.Warning, error) {
	target := opt.Target
	if target == "" {
		target = "emu"
	}
	prog, err := c.compileNoWrite(opt.Dir, target, filename)
	if err != nil {
		return nil, err
	}
	return collectWarnings(prog), nil
}

// compileNoWrite は意味解析からコード生成まで通す (ファイルは書かない)。
func (c *Compiler) compileNoWrite(dir, target, main string) (*sema.Program, error) {
	prog := sema.NewProgram()
	if err := sema.CompileProgram(prog, dir, c.libPath(target), main); err != nil {
		return nil, err
	}
	llc := codegen.NewLlc(2, prog.Types)
	if _, err := llc.PrepareProgram(prog.Modules.List(), DefaultStaticZp, DefaultStaticRam); err != nil {
		return nil, err
	}
	for _, mod := range prog.Modules.List() {
		if mod.FromFcm {
			continue
		}
		if _, _, err := llc.Compile(mod); err != nil {
			return nil, err
		}
	}
	return prog, nil
}
