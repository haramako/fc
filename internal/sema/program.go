package sema

// Program はプログラム全体 (全モジュール) にまたがる意味解析の状態。
// 個々のモジュールの解析は Hlc (モジュール単位のコンテキスト) が行い、モジュール横断の
// 情報 (型のインターン表・モジュール一覧・グローバル options・組み込みマクロ) だけをここに置く
// (doc/archive/v2_plan.md C4: モジュール単位の sema)。
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
	"sort"
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

	// Sources はモジュール id → 読み込んだソース (fcc migrate / ツール用。Loader が登録する)
	Sources map[string]*Source
	// Warnings は意味解析で見つけた警告 (出現順)
	Warnings []diag.Warning
	// Errors は意味解析で見つけたエラー (出現順)。文ごとに回復して集める。MaxErrors で打ち切る
	Errors diag.ErrorList
	// CastKinds は v1 の `<T>x` の位置 → v2 で書くべき種類 (as / bitcast)。fcc migrate が使う
	CastKinds map[syntax.Position]syntax.CastKind

	soas    map[*types.Type]*soaInfo // SoA コンテナ型 → フィールドごとの配列 (soa.go)
	lambdas map[string]*ir.Lambda    // シンボル → 関数 (far call の判定で呼び先のモジュールを引く)
	// FarCalls は far call になった呼び出しの一覧 ("caller -> callee" と位置)。fcc build -d で表示する
	FarCalls    []FarCall
	global      *ir.Scope                  // 組み込みマクロ (asm) を持つ最上位スコープ
	macros      map[*ir.Value]MacroFn      // マクロ値 → 本体
	constMacros map[*ir.Value]ConstMacroFn // 定数式で評価する組み込み (textmap) → 本体
	curModule   string                     // 名前解決を行っている (= 参照元の) モジュール id (Trace 用)
}

// Source は読み込んだソースファイル。
type Source struct {
	File *syntax.File
	Abs  string // 読み込みに使った実パス
	Src  []byte // CRLF 正規化後の内容
}

// SetTrace は名前解決の観測を有効にする (fcc migrate の参照解析)。
// fn には (参照元モジュール id, 観測) が渡る。
func (p *Program) SetTrace(fn func(origin string, ev ir.TraceEvent)) {
	p.global.SetTrace(func(ev ir.TraceEvent) { fn(p.curModule, ev) })
}

// NewProgram は空のプログラム状態を作り、組み込みマクロを登録する。
func NewProgram() *Program {
	p := &Program{
		Types:       types.NewUniverse(),
		Modules:     ir.NewModuleList(),
		Sources:     map[string]*Source{},
		CastKinds:   map[syntax.Position]syntax.CastKind{},
		macros:      map[*ir.Value]MacroFn{},
		constMacros: map[*ir.Value]ConstMacroFn{},
		soas:        map[*types.Type]*soaInfo{},
		lambdas:     map[string]*ir.Lambda{},
	}
	p.global = ir.NewScope(nil)
	registerBuiltins(p)
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

	// use で別モジュールの相 1 にネストして入るので、参照元モジュールを保存・復帰する
	outer := p.curModule
	p.curModule = id
	defer func() { p.curModule = outer }()

	h := &Hlc{prog: p, deps: deps, module: mod, scope: mod.Scope}
	defer h.recoverTo(&err)
	h.compileStmts(file.Stmts)
	// Apply the final module default to every unqualified global, including
	// declarations before options(bss:...). Imported modules own their defaults.
	if bss, ok := mod.Options.Get("bss"); ok {
		for _, d := range mod.Defs {
			if d.Kind == ir.DefBss && d.Segment == "" {
				d.Segment = bss.Str
			}
		}
	}
	return mod, nil
}

// CompileBodies はモジュールの全関数本体をコンパイルする (相 2)。
func (p *Program) CompileBodies(mod *ir.Module, deps Resolver) (err error) {
	outer := p.curModule
	p.curModule = mod.Id
	defer func() { p.curModule = outer }()
	h := &Hlc{prog: p, deps: deps, module: mod, scope: mod.Scope}
	defer h.recoverTo(&err)
	// コンパイル中にネストしたlambdaが追加されることがあるため index ループ
	for i := 0; i < len(mod.Lambdas); i++ {
		h.compileLambda(mod.Lambdas[i])
	}
	return nil
}

// FarCall は far call になった呼び出し箇所。
type FarCall struct {
	Pos    syntax.Position
	Caller string // 関数のシンボル
	Callee string
}

// FarCallEnabled は far call の仕組みが有効か (メインモジュールの options(farcall: true))。
func (p *Program) FarCallEnabled() bool {
	v, ok := p.Options.Get("farcall")
	return ok && (v.Kind == ir.OptInt && v.Int != 0 || v.Kind == ir.OptIdent && v.Str == "true")
}

// MaxErrors はこれ以上エラーが集まったら処理を打ち切る件数。
const MaxErrors = 30

// report はエラーを記録する (同じ位置・同じ文言は 1 回だけ)。件数が上限に達したら Fatal なエラーを投げて打ち切る。
func (p *Program) report(e *diag.Error) {
	if e.Suppressed {
		return
	}
	for _, prev := range p.Errors {
		if prev.Pos == e.Pos && prev.Msg == e.Msg {
			return
		}
	}
	p.Errors = append(p.Errors, e)
	if len(p.Errors) >= MaxErrors {
		panic(&diag.Error{Msg: fmt.Sprintf("too many errors (%d); stopping", len(p.Errors)), Pos: e.Pos, Fatal: true})
	}
}

// ErrorList は集めたエラーをファイル・位置順に並べて error として返す (無ければ nil)。
func (p *Program) ErrorList() error {
	if len(p.Errors) == 0 {
		return nil
	}
	sort.SliceStable(p.Errors, func(i, j int) bool {
		a, b := p.Errors[i].Pos, p.Errors[j].Pos
		if a.Filename != b.Filename {
			return a.Filename < b.Filename
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.Col < b.Col
	})
	return p.Errors
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

// recoverTo はモジュール単位の回復点: 文単位の回復をすり抜けた *diag.Error (Fatal など) を記録する。
// 記録したエラーは Program.Errors に集まり、呼び出し側が ErrorList() でまとめて受け取る (err には入れない)。
func (h *Hlc) recoverTo(err *error) {
	if r := recover(); r != nil {
		ce, ok := r.(*diag.Error)
		if !ok {
			panic(r)
		}
		if !ce.Pos.IsValid() {
			ce.Pos = h.curPos
		}
		if ce.Fatal && ce.Suppressed {
			return // 上限到達の再送 (report 済み)
		}
		if ce.Fatal {
			h.prog.Errors = append(h.prog.Errors, ce)
			return
		}
		h.prog.report(ce)
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
		ref = joinPath(p, name)
		abs = l.abs(ref)
		if _, err := os.Stat(abs); err == nil {
			return ref, abs, nil
		}
	}
	return "", "", &diag.Error{Msg: fmt.Sprintf("file %s not found", name)}
}

// joinPath は検索パス p とファイル名を '/' でつなぐ (p が "." なら f そのもの。生成物に埋め込む参照形なので filepath は使わない)。
func joinPath(p, f string) string {
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
	id := strings.TrimSuffix(filepath.Base(ref), ".fc")
	if _, ok := l.prog.Sources[id]; !ok {
		l.prog.Sources[id] = &Source{File: file, Abs: abs, Src: src}
	}
	return l.prog.CompileModule(file, l)
}

// Compile は libPath 上の mainFile から始めてプログラム全体を解析する (相 1 → 相 2)。
// baseDir は libPath の相対エントリの基準 ("" なら作業ディレクトリ)。
func Compile(baseDir string, libPath []string, mainFile string) (*Program, error) {
	prog := NewProgram()
	if err := CompileProgram(prog, baseDir, libPath, mainFile); err != nil {
		return nil, err
	}
	return prog, nil
}

// CompileProgram は Compile と同じだが、呼び出し側が用意した Program (SetTrace 済みなど) に対して行う。
func CompileProgram(prog *Program, baseDir string, libPath []string, mainFile string) error {
	loader := NewLoader(prog, baseDir, libPath)
	if _, err := loader.Load(mainFile); err != nil {
		return err
	}
	if err := prog.CompileAllBodies(loader); err != nil {
		return err
	}
	return prog.ErrorList()
}
