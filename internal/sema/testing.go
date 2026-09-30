package sema

// テストの組み込み (Agent/wiki/plans/v4-stdlib.md §7.1): @assert / @assert_eq と、fc 4 の @run_tests (@(test) の関数を集めて順に呼ぶ)。
//
//   @assert(式 [, "文言"])      式が偽なら「ファイル:行: assert failed: 式の綴り」を出して止まる (sys.panic。終了コード 1)
//   @assert_eq(実際, 期待)       違えば「ファイル:行: assert_eq failed: 実際の綴り is 値 (want 値)」を出して止まる。整数・bool・enum
//   @run_tests()                 fc 4: プログラムの全モジュールの @(test) の関数を、モジュールの順・宣言の順に「名前: 」を出して
//                                から呼び、「N tests ok」を出して終える (console)。fc 2 / fc 3 は今までどおり (スコープの test_*)
//
// 文言の綴り・位置はコンパイル時に埋め込む。sys / console は use しなくても読み込む (builtinModule)。

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/syntax"
	"github.com/haramako/fc/internal/types"
)

func registerTestingBuiltins(h *Hlc) {
	h.defmacro("@assert", func(h *Hlc, args []*cexpr, block *syntax.Block) macroResult {
		if len(args) < 1 || len(args) > 2 {
			panic(&diag.Error{Msg: "@assert takes a condition and an optional message (@assert(x > 0, \"x must be positive\"))"})
		}
		msg := fmt.Sprintf("%s: assert failed: %s", h.srcLoc(args[0]), h.srcText(args[0]))
		if len(args) == 2 {
			m := h.constEval(args[1])
			if m.kind != cValue || !m.val.IsString {
				panic(&diag.Error{Msg: "@assert: the message must be a constant string"})
			}
			msg += ": " + m.val.Str
		}
		sys := h.builtinModule("sys")
		ok := h.newLabel("assert")
		h.compileCond(args[0], ok, true)
		h.lval(ccall(cv(h.moduleFunc(sys, "sys", "panic")), cstr(msg)))
		h.emit(&ir.Op{Code: ir.OpLabel, Label: ok})
		return macroResult{}
	})

	h.defmacro("@assert_eq", func(h *Hlc, args []*cexpr, block *syntax.Block) macroResult {
		if len(args) != 2 {
			panic(&diag.Error{Msg: "@assert_eq takes the actual and the expected value (@assert_eq(f(3), 9))"})
		}
		got := h.freeze(h.rval(args[0]))
		want := h.freeze(h.rval(h.withExpected(args[1], ir.ValType(got))))
		for _, v := range []ir.Operand{got, want} {
			if t := ir.ValType(v); t.Kind != types.Int && t.Kind != types.Bool {
				panic(&diag.Error{Msg: fmt.Sprintf("@assert_eq compares integers, bool and enums (got %s); use @assert for other values", t)})
			}
		}
		ci := h.builtinModule("console")
		ok := h.newLabel("assert")
		h.compileCond(cop2(opEq, cv(h.operandValue(got)), cv(h.operandValue(want))), ok, true)
		esc := strings.NewReplacer("{", "{{", "}", "}}")
		format := fmt.Sprintf("%s: assert_eq failed: %s is {} (want {})\n", esc.Replace(h.srcLoc(args[0])), esc.Replace(h.srcText(args[0])))
		h.printf4([]*cexpr{cstr(format), cv(h.operandValue(got)), cv(h.operandValue(want))})
		h.lval(ccall(cv(h.moduleFunc(ci, "console", "exit")), cint(1)))
		h.emit(&ir.Op{Code: ir.OpLabel, Label: ok})
		return macroResult{}
	})
}

// runTests4 は fc 4 の @run_tests: プログラムの全モジュールの @(test) の関数を呼ぶ。
func (h *Hlc) runTests4() macroResult {
	ci := h.builtinModule("console")
	console := func(name string, args ...*cexpr) { h.lval(ccall(cv(h.moduleFunc(ci, "console", name)), args...)) }
	console("init")
	n := 0
	for _, m := range h.prog.Modules.List() {
		mi := m.Interface()
		for _, lmd := range m.Lambdas {
			if lmd.Name == "" || !lmd.Options.Flag("test") {
				continue
			}
			if len(lmd.Type.Params) != 0 || lmd.Type.Base.Kind != types.Void {
				panic(&diag.Error{Msg: fmt.Sprintf("%s.%s: a @(test) function takes no arguments and returns nothing", m.Id, lmd.Name), Pos: lmd.Pos})
			}
			fn := mi.LookupInternal(lmd.Name)
			if fn == nil {
				continue
			}
			console("write_z", h.cstrZ(fmt.Sprintf("%s.%s: ", m.Id, lmd.Name)))
			h.lval(ccall(cv(fn)))
			console("write_z", h.cstrZ("ok\n"))
			n++
		}
	}
	console("write_z", h.cstrZ(fmt.Sprintf("%d tests ok\n", n)))
	console("exit", cint(0))
	return macroResult{}
}

// srcLoc は式 c の位置 (「ファイル名:行」)。
func (h *Hlc) srcLoc(c *cexpr) string {
	name := filepath.Base(h.module.Path)
	if c != nil && c.pos.IsValid() {
		return fmt.Sprintf("%s:%d", name, c.pos.Line)
	}
	return fmt.Sprintf("%s:%d", name, h.curPos.Line)
}

// srcText は式 c のソースの綴り (分からなければ "?")。
func (h *Hlc) srcText(c *cexpr) string {
	src := h.prog.Sources[h.module.Id]
	if c == nil || src == nil || !c.pos.IsValid() || !c.end.IsValid() || c.pos.Offset < 0 || c.end.Offset > len(src.Src) || c.pos.Offset >= c.end.Offset {
		return "?"
	}
	return string(src.Src[c.pos.Offset:c.end.Offset])
}
