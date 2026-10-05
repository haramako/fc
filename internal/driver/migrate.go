package driver

// fcc migrate の手順 (プログラム単位)。fc 2 → 3 は構文の書き換え (internal/migrate の Rules。ファイルごと)、fc 3 → 4 は意味の
// 変わる所の書き換えで、プログラムとしてコンパイルして型を見る (sema の Rewrites。Agent/wiki/plans/v4-plan.md §0)。

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/migrate"
	"github.com/haramako/fc/internal/project"
	"github.com/haramako/fc/internal/sema"
	"github.com/haramako/fc/internal/syntax"
)

// MigrateOptions は fcc migrate の設定。
type MigrateOptions struct {
	Target  string   // 入口としてコンパイルするときのターゲット (fclib の探し方。既定 emu)
	Defines []string // CLI の -D (fcc build と同じく fc.toml の後に当てる)
	// Rules が空でなければ、fc 3 → 4 の書き換えのうちその規則 (sema.Rewrite.Rule) のものだけを当てる (テスト用: 範囲外の定数と
	// 縮小だけを直して、A1・F1 の fc 4 の意味がそのまま効く fc 4 のプログラムを作る)
	Rules []string
}

// Migrate は files (fc 2 / fc 3 / fc 4 のソース) を fc 4 に書き換えた内容を返す (キーは files の要素。改行は LF)。
//  1. fc 2 のソースは構文の書き換えで fc 3 に (migrate.Migrate)
//  2. fc 3 のソースは、それを入口にしてプログラムとしてコンパイルし (ほかの入口のプログラムで既にコンパイルしたモジュールは
//     飛ばす)、fc 4 で意味が変わる所の書き換えを集めて当てる (migrate.ToV4)。1 の結果はファイルに書く前なので、sema には
//     メモリの上の内容を渡す (sema.Program.Overlay)。fclib などの files に無いモジュールは書き換えない (fc 3 のまま使える)
func (c *Compiler) Migrate(files []string, opt *MigrateOptions) (map[string][]byte, error) {
	target := opt.Target
	if target == "" {
		target = "emu"
	}
	overlay := map[string][]byte{}
	out := map[string][]byte{}
	var v3 []string
	for _, path := range files {
		src, err := sema.ReadSource(path)
		if err != nil {
			return nil, err
		}
		f, err := syntax.Parse(src, path)
		if err != nil {
			return nil, err
		}
		if f.Version == syntax.Version2 {
			if src, err = migrate.Migrate(src, path); err != nil {
				return nil, err
			}
		}
		overlay[sema.OverlayKey(path)] = src
		if f.Version == syntax.Version4 {
			out[path] = src
		} else {
			v3 = append(v3, path)
		}
	}
	rewrites := map[string][]sema.Rewrite{}
	seen := map[sema.Rewrite]bool{}
	compiled := map[string]bool{}
	for _, path := range v3 {
		key := sema.OverlayKey(path)
		if compiled[key] {
			continue
		}
		prog, err := c.newCompilation(nil, filepath.Dir(path), target).collectRewrites(path, opt.Defines, overlay)
		if err != nil {
			return nil, err
		}
		for _, src := range prog.Sources {
			compiled[sema.OverlayKey(src.Abs)] = true
		}
		var errs []string
		for _, e := range prog.RewriteErrors {
			if _, ok := overlay[sema.OverlayKey(e.File)]; ok && ruleWanted(opt.Rules, e.Rule) {
				errs = append(errs, e.Msg)
			}
		}
		if len(errs) > 0 {
			return nil, fmt.Errorf("%s", strings.Join(errs, "\n"))
		}
		for _, r := range prog.Rewrites {
			k := sema.OverlayKey(r.File)
			r.File = k
			if _, ok := overlay[k]; ok && !seen[r] && ruleWanted(opt.Rules, r.Rule) {
				seen[r] = true
				rewrites[k] = append(rewrites[k], r)
			}
		}
	}
	for _, path := range v3 {
		key := sema.OverlayKey(path)
		res, err := migrate.ToV4(overlay[key], path, rewrites[key])
		if err != nil {
			return nil, err
		}
		out[path] = res
	}
	return out, nil
}

// ruleWanted は書き換えの規則 rule を当てるか (rules が空ならすべて)。
func ruleWanted(rules []string, rule string) bool {
	if len(rules) == 0 {
		return true
	}
	for _, r := range rules {
		if r == rule {
			return true
		}
	}
	return false
}

// collectRewrites は path を入口にしたプログラムを意味解析まで通し、fc 3 → 4 の書き換えを集める。
func (c *compilation) collectRewrites(path string, cli []string, overlay map[string][]byte) (prog *sema.Program, err error) {
	defer func() {
		if r := recover(); r != nil {
			if ce, ok := r.(*diag.Error); ok {
				err = ce
				return
			}
			panic(r)
		}
	}()
	defs, err := c.projectDefines(cli)
	if err != nil {
		return nil, err
	}
	prog = sema.NewProgram()
	macros, done, err := c.projectMacros()
	if err != nil {
		return nil, err
	}
	defer done()
	for _, m := range macros {
		if err := prog.UseMacros(m); err != nil {
			return nil, err
		}
	}
	prog.Defines = project.CopyDefines(defs)
	prog.Banks = c.banks()
	prog.CollectRewrites = true
	prog.Overlay = overlay
	if err := sema.CompileProgram(prog, c.dir, c.libPath(c.target), filepath.Base(path)); err != nil {
		return nil, err
	}
	return prog, nil
}
