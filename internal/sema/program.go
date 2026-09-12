package sema

// Program はプログラム全体 (全モジュール) にまたがる意味解析の状態。
// 個々のモジュールの解析は Hlc (モジュール単位のコンテキスト) が行い、モジュール横断の
// 情報 (型のインターン表・モジュール一覧・グローバル options・組み込みマクロ) だけをここに置く
// (doc/v2_plan.md C4: モジュール単位の sema)。
//
// コンパイルは 2 相:
//  1. CompileModule — モジュールのトップレベル文 (宣言・use・include・options) を処理する。
//     `use X` に出会うと Resolver 経由で X を同じ相まで進める (再帰)。相互 use の途中で
//     再訪したモジュールは処理途中の状態で返る (旧実装と同じ順序依存の部分可視性)
//  2. CompileBodies — 全モジュールの宣言が揃った後、関数本体をコンパイルする

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/syntax"
	"github.com/haramako/fc/internal/types"
)

type Program struct {
	Types   *types.Universe
	Modules *ir.ModuleList // 登録順 (use に出会った深さ優先順)。リンク順にもなる
	Options ir.Options     // グローバルな options (mapper, bank_count, ...)。モジュール処理順の後勝ち

	global *ir.Scope             // 組み込みマクロ (asm) を持つ最上位スコープ
	macros map[*ir.Value]MacroFn // マクロ値 → 本体
}

// NewProgram は空のプログラム状態を作り、組み込みマクロを登録する。
func NewProgram() *Program {
	p := &Program{
		Types:   types.NewUniverse(),
		Modules: ir.NewModuleList(),
		macros:  map[*ir.Value]MacroFn{},
	}
	p.global = ir.NewScope(nil)
	h := &Hlc{prog: p, scope: p.global}
	h.defmacro("asm", func(h *Hlc, args []*cexpr, block *syntax.Block) macroResult {
		for _, line := range args {
			h.emit(&ir.Op{Code: ir.OpAsm, Text: mustString(line)})
		}
		return macroResult{}
	})
	return p
}

// Resolver はモジュールの依存解決。driver (または Loader) が実装する。
type Resolver interface {
	// Module は `use name` の先を、トップレベル宣言まで処理した状態で返す。
	// 処理途中 (相互 use) のモジュールはその状態のまま返す。
	Module(name string) (*ir.Module, error)
	// File は include / incbin のファイル名を解決する。
	// ref は生成物 (アセンブラの .incbin やエラー位置) に埋め込む参照形 (検索パスからの相対)、
	// abs は読み込みに使う実パス。
	File(name string) (ref, abs string, err error)
}

// CompileModule は 1 モジュールのトップレベルを解析し、Program に登録して返す (相 1)。
// 同じ id が登録済みならそれを返す。
func (p *Program) CompileModule(file *syntax.File, deps Resolver) (mod *ir.Module, err error) {
	id := strings.TrimSuffix(filepath.Base(file.Filename), ".fc")
	if m, ok := p.Modules.Get(id); ok {
		return m, nil
	}
	mod = ir.NewModule(id, file.Filename, p.global)
	p.Modules.Add(mod)

	h := &Hlc{prog: p, deps: deps, module: mod, scope: mod.Scope}
	defer h.recoverTo(&err)
	h.compileStmts(file.Stmts)
	return mod, nil
}

// CompileBodies はモジュールの全関数本体をコンパイルする (相 2)。
func (p *Program) CompileBodies(mod *ir.Module, deps Resolver) (err error) {
	h := &Hlc{prog: p, deps: deps, module: mod, scope: mod.Scope}
	defer h.recoverTo(&err)
	// コンパイル中にネストしたlambdaが追加されることがあるため index ループ
	for i := 0; i < len(mod.Lambdas); i++ {
		h.compileLambda(mod.Lambdas[i])
	}
	return nil
}

// CompileAllBodies は登録済み全モジュールの関数本体をコンパイルする。
func (p *Program) CompileAllBodies(deps Resolver) error {
	for _, mod := range p.Modules.List() {
		if mod.FromFcm {
			continue
		}
		if err := p.CompileBodies(mod, deps); err != nil {
			return err
		}
	}
	return nil
}

// recoverTo は回復点: panic された *diag.Error に処理中の位置を補完して err に入れる。
func (h *Hlc) recoverTo(err *error) {
	if r := recover(); r != nil {
		ce, ok := r.(*diag.Error)
		if !ok {
			panic(r)
		}
		if !ce.Pos.IsValid() {
			ce.Pos = h.curPos
		}
		*err = ce
	}
}

// ---------------------------------------------------------------
// Loader: ファイルシステムからの読み込みと依存解決 (Resolver の標準実装)
// ---------------------------------------------------------------

// Loader は libPath からソースを探して読み込み、use の先を再帰的にコンパイルする Resolver。
// libPath の相対エントリは baseDir を基準に解決するので、作業ディレクトリに依存しない (C7)。
// 生成物に埋め込む参照形 (File の ref) は libPath エントリからの相対のまま保つ。
type Loader struct {
	prog    *Program
	baseDir string   // 相対パスの基準 ("" なら作業ディレクトリ)
	libPath []string // Fc::LIB_PATH 相当。先頭から順に探す
}

func NewLoader(prog *Program, baseDir string, libPath []string) *Loader {
	return &Loader{prog: prog, baseDir: baseDir, libPath: libPath}
}

// abs は参照形のパスを読み込み用の実パスにする。
func (l *Loader) abs(ref string) string {
	if l.baseDir == "" || filepath.IsAbs(ref) {
		return ref
	}
	return filepath.Join(l.baseDir, ref)
}

// File は libPath 上でファイルを探す。
func (l *Loader) File(name string) (ref, abs string, err error) {
	for _, p := range l.libPath {
		ref = joinRubyPath(p, name)
		abs = l.abs(ref)
		if _, err := os.Stat(abs); err == nil {
			return ref, abs, nil
		}
	}
	return "", "", &diag.Error{Msg: fmt.Sprintf("file %s not found", name)}
}

// joinRubyPath は Ruby の Pathname#+ 相当 ('.' + f は f になる)。
func joinRubyPath(p, f string) string {
	if p == "." {
		return f
	}
	return p + "/" + f
}

// Module は name (拡張子なし) のモジュールを読み込んで相 1 まで進める。
func (l *Loader) Module(name string) (*ir.Module, error) {
	if m, ok := l.prog.Modules.Get(name); ok {
		return m, nil
	}
	return l.Load(name + ".fc")
}

// Load はファイル名でモジュールを読み込んで相 1 まで進める (メインモジュールの入口)。
func (l *Loader) Load(filename string) (*ir.Module, error) {
	ref, abs, err := l.File(filename)
	if err != nil {
		return nil, err
	}
	src, err := ReadSource(abs)
	if err != nil {
		return nil, &diag.Error{Msg: err.Error()}
	}
	file, perr := syntax.Parse(src, ref)
	if perr != nil {
		se := perr.(*syntax.Error)
		return nil, &diag.Error{Msg: se.Msg, Pos: se.Position()}
	}
	return l.prog.CompileModule(file, l)
}

// Compile は libPath 上の mainFile から始めてプログラム全体を解析する (相 1 → 相 2)。
// baseDir は libPath の相対エントリの基準 ("" なら作業ディレクトリ)。
func Compile(baseDir string, libPath []string, mainFile string) (*Program, error) {
	prog := NewProgram()
	loader := NewLoader(prog, baseDir, libPath)
	if _, err := loader.Load(mainFile); err != nil {
		return nil, err
	}
	if err := prog.CompileAllBodies(loader); err != nil {
		return nil, err
	}
	return prog, nil
}
