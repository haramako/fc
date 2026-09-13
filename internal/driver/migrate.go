package driver

// fcc migrate: v1 ソースを文法 v2 に移行する (doc/v2_grammar.md §5)。
//
//  1. 各 main を v1 としてコンパイルし (sema + codegen、アセンブラは呼ばない)、
//     名前解決を観測してモジュール間参照を集める。移行前の asm も取っておく
//  2. 読み込まれた全ソースの構文木を書き換えて印字する
//  3. Write なら書き込み、同じ main をもう一度コンパイルして asm を比較する。
//     差分があれば (Force でなければ) 元に戻してエラーにする

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/haramako/fc/internal/codegen"
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/migrate"
	"github.com/haramako/fc/internal/sema"
	"github.com/haramako/fc/internal/syntax"
)

// MigrateOptions は fcc migrate の設定。
type MigrateOptions struct {
	Target string   // emu (既定) / nes
	Dir    string   // ソースの基準ディレクトリ ("" なら作業ディレクトリ)
	Mains  []string // 解析の起点 (Dir 相対)。全 main の参照を合わせて可視性を決める
	// Libs はライブラリのディレクトリ (Dir 相対または絶対)。この下のファイルは preserve で移行する
	// (利用者が複数いるので参照の和集合では public 集合が決まらない)
	Libs []string
	// Visibility は Libs 以外の既定 (Minimal)
	Visibility migrate.Visibility
	// Textmaps は castle の macro.rb の置換 (NAME → 文字表パス)
	Textmaps map[string]string
	// Write が偽なら書き換え結果を報告するだけ (検証もしない)
	Write bool
	// Force なら asm に差分があっても書き込んだままにする
	Force bool
	// Out は報告の出力先 (nil なら os.Stdout)
	Out io.Writer
}

// MigrateResult は移行の結果。
type MigrateResult struct {
	Files   []string // 書き換えたファイル (実パス)
	Notes   []string // 手作業が要る箇所などの注記
	Diffs   []string // asm が変わったモジュール
	Written bool
}

// Migrate は v1 ソースを v2 に移行する。
func (c *Compiler) Migrate(opt *MigrateOptions) (*MigrateResult, error) {
	if opt.Target == "" {
		opt.Target = "emu"
	}
	if opt.Out == nil {
		opt.Out = os.Stdout
	}
	dir := opt.Dir
	if dir == "" {
		dir = "."
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	var libs []string
	for _, l := range opt.Libs {
		if !filepath.IsAbs(l) {
			l = filepath.Join(absDir, l)
		}
		libs = append(libs, filepath.Clean(l))
	}
	under := func(root, abs string) bool {
		rel, err := filepath.Rel(root, abs)
		return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
	}
	isLib := func(abs string) bool {
		for _, l := range libs {
			if under(l, abs) {
				return true
			}
		}
		return false
	}
	// 書き換えるのは Dir と Libs の下のファイルだけ。それ以外 (FC_HOME の fclib など) は v1 のまま残す
	// (v1/v2 は混在できる)
	inScope := func(path string) bool {
		abs, err := filepath.Abs(path) // Loader の実パスは Dir 相対のことがある
		if err != nil {
			return false
		}
		return under(absDir, abs) || isLib(abs)
	}

	// 1. 解析
	a := migrate.NewAnalysis()
	before := map[string]string{} // module id → asm (移行前)
	sources := map[string]*sema.Source{}
	for _, main := range opt.Mains {
		prog, asm, err := c.compileToAsm(opt.Dir, opt.Target, main, a.Trace)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", main, err)
		}
		for id, s := range prog.Sources {
			sources[id] = s
		}
		for id, text := range asm {
			before[id] = text
		}
	}

	// 2. 書き換え
	res := &MigrateResult{}
	type change struct {
		abs       string
		orig      []byte
		rewritten []byte
	}
	var changes []change
	ids := make([]string, 0, len(sources))
	for id := range sources {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		src := sources[id]
		if src.File.Version >= syntax.Version2 || !inScope(src.Abs) {
			continue
		}
		// コンパイルに使った構文木は触らず、ソースから作り直す
		f, perr := syntax.Parse(src.Src, src.File.Filename)
		if perr != nil {
			return nil, perr
		}
		mopt := migrate.Options{Visibility: opt.Visibility, Textmaps: opt.Textmaps}
		if isLib(src.Abs) {
			mopt.Visibility = migrate.Preserve
		}
		notes, err := migrate.Rewrite(f, id, a, mopt)
		if err != nil {
			return nil, err
		}
		res.Notes = append(res.Notes, notes...)
		out := syntax.Print(f)
		orig, err := os.ReadFile(src.Abs)
		if err != nil {
			return nil, err
		}
		if bytes.Contains(orig, []byte("\r\n")) {
			out = bytes.ReplaceAll(out, []byte("\n"), []byte("\r\n"))
		}
		changes = append(changes, change{abs: src.Abs, orig: orig, rewritten: out})
		res.Files = append(res.Files, src.Abs)
	}
	for _, n := range res.Notes {
		fmt.Fprintln(opt.Out, "note:", n)
	}
	if !opt.Write {
		for _, ch := range changes {
			fmt.Fprintln(opt.Out, ch.abs)
		}
		return res, nil
	}

	// 3. 書き込みと検証
	restore := func() {
		for _, ch := range changes {
			os.WriteFile(ch.abs, ch.orig, 0o666)
		}
	}
	for _, ch := range changes {
		if err := os.WriteFile(ch.abs, ch.rewritten, 0o666); err != nil {
			restore()
			return nil, err
		}
	}
	res.Written = true
	after := map[string]string{}
	for _, main := range opt.Mains {
		_, asm, err := c.compileToAsm(opt.Dir, opt.Target, main, nil)
		if err != nil {
			if !opt.Force {
				restore()
				res.Written = false
			}
			return res, fmt.Errorf("移行後のコンパイルに失敗 (%s): %w", main, err)
		}
		for id, text := range asm {
			after[id] = text
		}
	}
	for id, b := range before {
		if after[id] != b {
			res.Diffs = append(res.Diffs, id)
		}
	}
	sort.Strings(res.Diffs)
	if len(res.Diffs) > 0 && !opt.Force {
		restore()
		res.Written = false
		return res, fmt.Errorf("移行前後で asm が一致しないモジュール: %s (--force で受け入れる)", strings.Join(res.Diffs, ", "))
	}
	return res, nil
}

// compileToAsm は main を sema + codegen まで処理し、モジュール id → asm (.s と .inc の連結) を返す。
// trace が非 nil なら名前解決を観測する。
func (c *Compiler) compileToAsm(dir, target, main string, trace func(string, ir.TraceEvent)) (*sema.Program, map[string]string, error) {
	prog := sema.NewProgram()
	if trace != nil {
		prog.SetTrace(trace)
	}
	if err := sema.CompileProgram(prog, dir, c.libPath(target), main); err != nil {
		return nil, nil, err
	}
	llc := codegen.NewLlc(2, prog.Types)
	asm := map[string]string{}
	for _, mod := range prog.Modules.List() {
		if mod.FromFcm {
			continue
		}
		s, inc, err := llc.Compile(mod)
		if err != nil {
			return nil, nil, err
		}
		// 一時変数やラベルの番号は書き換えでずれうる (textmap の文字列リテラルなど) ので正規化して比べる
		asm[mod.Id] = normalizeLabels(normalizeAsm(s) + normalizeAsm(inc))
	}
	return prog, asm, nil
}
