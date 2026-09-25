package sema

// 組み込み機能 (doc/v2_grammar.md §3.5, §4.3)。
//
// include の有無に関係なく常に使える
// 組み込みにした。グローバルスコープにマクロ値として登録する:
//
//   - asm("...")            : インラインアセンブラ
//   - printf(args...)       : 引数の型で stdio.print / stdio.print_int16 を呼び分ける
//   - unittest_run_tests()  : スコープ内の test_* 関数を順に呼ぶ
//   - cos(x)                : sin(x + 64) への展開 (math モジュールの sin を使う)
//   - textmap(path)         : 文字列→文字コード表 (const _T = textmap("..."); _T("…") で int[] 定数)
//
// printf / unittest_run_tests / cos は stdio / math モジュールの関数を参照する。可視性に関係なく届く
// (LookupInternal) が、そのモジュールがプログラムに読み込まれていなければエラー。

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
			// 旧実装は引数が定数値 (変数・リテラル) しか受けなかった。式 (struct のフィールドなど) は先に評価して値にする
			if arg.kind != cValue {
				arg = cv(h.operandValue(h.rval(arg)))
			}
			typ := arg.val.Type
			if h.prog.Types.Compatible(uint8p, typ) != nil {
				r.stmts = append(r.stmts, ccall(cv(print), arg))
			} else if typ.Kind == types.Int || typ.Kind == types.Bool {
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

	// min(a, b) / max(a, b) / clamp(x, lo, hi): 型は引数の互換型で決まる (符号付きなら符号付き比較)。
	// 定数なら畳み込み、そうでなければ比較して入れ替えるコードをその場に出す (関数呼び出しは無い)
	for _, bi := range []struct {
		name string
		op   cop
		n    int
	}{{"min", opMin, 2}, {"max", opMax, 2}, {"clamp", opClamp, 3}} {
		h.defmacro(bi.name, func(h *Hlc, args []*cexpr, block *syntax.Block) macroResult {
			if len(args) != bi.n {
				panic(&diag.Error{Msg: fmt.Sprintf("%s takes %d arguments", bi.name, bi.n)})
			}
			return macroResult{expr: &cexpr{kind: cOp, op: bi.op, args: args}}
		})
	}

	// cos(x) = sin(x + 64) のマクロ展開 (v1 の math.rb)。math モジュールの sin を参照する。
	// 関数にすると呼び出し側の asm が変わるので、インライン関数 (F2) が入るまでは組み込みマクロのまま。
	// `math.cos(x)` はドット参照が math のスコープから親 (グローバル) に辿り着くので従来どおり書ける
	h.defmacro("cos", func(h *Hlc, args []*cexpr, block *syntax.Block) macroResult {
		m, ok := h.prog.Modules.Get("math")
		if !ok {
			panic(&diag.Error{Msg: "cos requires the math module (add `use math;`)"})
		}
		if len(args) != 1 {
			panic(&diag.Error{Msg: "cos takes 1 argument"})
		}
		return macroResult{expr: ccall(cv(m.Interface().LookupInternal("sin")), cop2(opAdd, args[0], cint(64)))}
	})

	registerSliceBuiltins(h)
	registerLogBuiltin(h)

	// @bank("name") は fc.toml の [bank.<name>] の番号 (コンパイル時に決まる u8。手動のバンク切り替え用。doc/v3_plan.md §3)
	h.defconstmacro("@bank", func(h *Hlc, args []*cexpr) *cexpr {
		if len(args) != 1 || args[0].kind != cValue || !args[0].val.IsString {
			panic(&diag.Error{Msg: "@bank takes 1 string argument (a bank name of fc.toml [bank.<name>])"})
		}
		name := args[0].val.Str
		v := h.bankByName(name)
		if v.Int < 0 {
			panic(&diag.Error{Msg: fmt.Sprintf("@bank(%q): the fixed area has no bank number", name)})
		}
		return cv(h.IntValue(v.Int))
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

// nullFnSymbol は @null_fn のシンボル (share/runtime.asm の rts だけの関数)。
const nullFnSymbol = "__fc_null_fn"

// nullFn は型 t (戻り値の無い関数の型。引数・fastcall・farfn は問わない) の @null_fn。呼ぶ側が引数を積み、呼び出しの後に
// レジスタを戻すので (呼び先は引数を片付けない)、rts だけでどの型としても呼べる。戻り値のある型は値が不定になるのでエラー。
func (h *Hlc) nullFn(t *types.Type) *ir.Value {
	if t.Kind != types.Func {
		panic(&diag.Error{Msg: fmt.Sprintf("@null_fn cannot be used as %s (it is a function that does nothing)", t)})
	}
	if t.Base.Kind != types.Void {
		panic(&diag.Error{Msg: fmt.Sprintf("@null_fn cannot be used as %s: it returns nothing (only fn(...):void)", t)})
	}
	return ir.NewSymbolLiteral("", t, nullFnSymbol)
}
