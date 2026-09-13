package sema

// 組み込み機能 (doc/v2_grammar.md §3.5, §4.3)。
//
// v1 では fclib/*.rb の Ruby マクロだったものを、include の有無に関係なく常に使える
// 組み込みにした。グローバルスコープにマクロ値として登録する:
//
//   - asm("...")            : インラインアセンブラ
//   - printf(args...)       : 引数の型で stdio.print / stdio.print_int16 を呼び分ける
//   - unittest_run_tests()  : スコープ内の test_* 関数を順に呼ぶ
//   - textmap(path)         : 文字列→文字コード表 (const _T = textmap("..."); _T("…") で int[] 定数)
//
// printf / unittest_run_tests は stdio モジュールの関数を参照する。可視性に関係なく届く
// (LookupInternal) が、stdio がプログラムに読み込まれていなければエラー。

import (
	"bytes"
	"fmt"

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/syntax"
	"github.com/haramako/fc/internal/types"
)

// ConstMacroFn は定数式の中で評価される組み込み (`textmap(...)` など)。結果は定数 (cValue) を返す。
type ConstMacroFn func(h *Hlc, args []*cexpr) *cexpr

func registerBuiltins(p *Program) {
	h := &Hlc{prog: p, scope: p.global}

	h.defmacro("asm", func(h *Hlc, args []*cexpr, block *syntax.Block) macroResult {
		for _, line := range args {
			h.emit(&ir.Op{Code: ir.OpAsm, Text: mustString(line)})
		}
		return macroResult{}
	})

	h.defmacro("printf", func(h *Hlc, args []*cexpr, block *syntax.Block) macroResult {
		stdio := h.stdioModule("printf")
		uint8p := h.prog.Types.PointerTo(h.prog.Types.IntType(1, false))
		print := stdio.LookupInternal("print")
		printInt16 := stdio.LookupInternal("print_int16")
		r := macroResult{stmts: []*cexpr{}}
		for _, arg := range args {
			// 旧実装は引数が定数値でないと ValType が落ちていた。同じく定数値を要求する
			typ := mustValue(arg).Type
			if h.prog.Types.Compatible(uint8p, typ) != nil {
				r.stmts = append(r.stmts, ccall(cv(print), arg))
			} else if typ.Kind == types.Int {
				r.stmts = append(r.stmts, ccall(cv(printInt16), arg))
			}
		}
		return r
	})

	h.defmacro("unittest_run_tests", func(h *Hlc, args []*cexpr, block *syntax.Block) macroResult {
		stdio := h.stdioModule("unittest_run_tests")
		print := stdio.LookupInternal("print")
		exit := stdio.LookupInternal("exit")
		init := stdio.LookupInternal("init")
		r := macroResult{stmts: []*cexpr{ccall(cv(init))}}
		for _, id := range h.scope.IdList() {
			if len(id) >= 5 && id[:5] == "test_" {
				r.stmts = append(r.stmts,
					ccall(cv(print), cstr(fmt.Sprintf("%s:", id))),
					ccall(cident(id)),
					ccall(cv(print), cstr("\n")),
				)
			}
		}
		r.stmts = append(r.stmts, ccall(cv(exit), cint(0)))
		return r
	})

	// textmap(path) は定数式。文字表を持つ新しいマクロ値を返し、それを呼ぶと文字列が int[] 定数になる
	h.defconstmacro("textmap", func(h *Hlc, args []*cexpr) *cexpr {
		if len(args) != 1 {
			panic(&diag.Error{Msg: "textmap takes 1 argument (path of the character table)"})
		}
		path := mustString(args[0])
		conv := NewTextConverter(string(bytes.ReplaceAll(h.readFile(path), []byte("\r\n"), []byte("\n"))))
		m := ir.NewGlobal("", h.prog.Types.Macro(), "")
		h.prog.macros[m] = func(h *Hlc, args []*cexpr, block *syntax.Block) macroResult {
			if len(args) != 1 {
				panic(&diag.Error{Msg: "text conversion takes 1 string argument"})
			}
			codes := append(conv.Conv(mustString(args[0])), 0)
			elems := make([]*cexpr, len(codes))
			for i, c := range codes {
				elems[i] = cint(c)
			}
			return macroResult{expr: carray(elems)}
		}
		return cv(m)
	})
}

// defconstmacro は定数式で評価される組み込みをグローバルに登録する。
func (h *Hlc) defconstmacro(name string, fn ConstMacroFn) *ir.Value {
	v := h.addVar(ir.NewGlobal(name, h.prog.Types.Macro(), ""))
	v.Public = true
	h.prog.constMacros[v] = fn
	return v
}

// stdioModule は組み込みが参照する stdio モジュールを返す (読み込まれていなければエラー)。
func (h *Hlc) stdioModule(builtin string) *ir.ModuleInterface {
	m, ok := h.prog.Modules.Get("stdio")
	if !ok {
		panic(&diag.Error{Msg: fmt.Sprintf("%s requires the stdio module (add `use stdio;` or `use * from stdio;`)", builtin)})
	}
	return m.Interface()
}
