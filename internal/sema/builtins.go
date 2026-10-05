package sema

// 組み込み機能 (Agent/discussions/2026-09-13-v2-grammar.md §3.5, §4.3)。
//
// include の有無に関係なく常に使える
// 組み込みにした。グローバルスコープにマクロ値として登録する:
//
//   - asm("...")            : インラインアセンブラ
//   - printf(args...)       : 引数の型で stdio.print / stdio.print_int16 を呼び分ける
//   - unittest_run_tests()  : スコープ内の test_* 関数を順に呼ぶ
//   - cos(x)                : sin(x + 64) への展開 (math モジュールの sin を使う)
//   - textmap(path [, po])  : 文字列→文字コード表 (const _T = textmap("..."); _T("…" [, "msgctxt"]) で int[] 定数。po で翻訳)
//
// printf / unittest_run_tests / cos は stdio / math モジュールの関数を参照する。可視性に関係なく届く
// (LookupInternal) が、そのモジュールがプログラムに読み込まれていなければエラー。

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/syntax"
	"github.com/haramako/fc/internal/types"
)

// ConstMacroFn は定数式の中で評価される組み込み (`textmap(...)` など)。結果は定数 (cValue) を返す。
type ConstMacroFn func(h *Hlc, args []*cexpr) *cexpr

func registerBuiltins(p *Program) {
	h := &Hlc{prog: p, scope: p.global}

	h.defmacroTyped("asm", voidMacro, func(h *Hlc, args []*cexpr, block *syntax.Block) macroResult {
		for _, line := range args {
			h.emit(&ir.Op{Code: ir.OpAsm, Text: mustString(line)})
		}
		return macroResult{}
	})

	// fc 4 の @printf: 書式文字列で console に出す (@format と同じ書式。format.go)。printf は fc 3 までの綴り (2026-09-30 に改名:
	// 書式をコンパイル時に分解する組み込みなので @format と同じく @ を付ける)
	h.defmacroTyped("@printf", voidMacro, func(h *Hlc, args []*cexpr, block *syntax.Block) macroResult {
		if !h.v4() {
			panic(&diag.Error{Msg: "@printf is fc 4 (write printf in fc 3 and older modules)"})
		}
		h.printf4(args)
		return macroResult{}
	})

	h.defmacroTyped("printf", voidMacro, func(h *Hlc, args []*cexpr, block *syntax.Block) macroResult {
		if h.v4() {
			panic(&diag.Error{Msg: "printf is written @printf in fc 4 (`fcc migrate` rewrites fc 3 sources)"})
		}
		stdio := h.stdioModule("printf")
		uint8p := h.prog.Types.PointerTo(h.prog.Types.IntType(1, false))
		print := h.moduleFunc(stdio, "stdio", "print")
		printInt16 := h.moduleFunc(stdio, "stdio", "print_int16")
		r := macroResult{stmts: []*cexpr{}}
		typs := make([]*types.Type, len(args))
		if h.rewriting() {
			defer func() { h.rewritePrintf(args, typs) }() // fc 4 の書式文字列の形に (format.go)
		}
		for i, arg := range args {
			// 旧実装は引数が定数値 (変数・リテラル) しか受けなかった。式 (struct のフィールドなど) は先に評価して値にする
			if arg.kind != cValue {
				arg = cv(h.operandValue(h.rval(arg)))
			}
			typ := arg.val.Type
			typs[i] = typ
			switch {
			case h.prog.Types.Compatible(uint8p, typ) != nil:
				r.stmts = append(r.stmts, ccall(cv(print), arg))
			case typ.IsSlice() && typ.SliceOf.Kind == types.Int && typ.SliceOf.Size == 1 && !typ.IsWideSlice():
				// slice は長さの分だけ (文字列の slice。黙って捨てていた)
				r.stmts = append(r.stmts, ccall(cv(h.moduleFunc(stdio, "stdio", "print_slice")), arg))
			case typ.Kind == types.Int && typ.Enum != nil:
				// enum は値 (print_int16 の引数の型のエラーだった)
				u16 := h.prog.Types.IntType(2, typ.Signed)
				n := &cexpr{kind: cCast, args: []*cexpr{arg}, ty: u16, ck: syntax.CastAs}
				if typ.Signed {
					r.stmts = append(r.stmts, ccall(cv(h.moduleFunc(stdio, "stdio", "print_sint16")), n))
				} else {
					r.stmts = append(r.stmts, ccall(cv(printInt16), n))
				}
			case typ.Kind == types.Int && typ.Signed:
				// 符号付きは符号付きで (符号なしで 65531 と出ていた)
				r.stmts = append(r.stmts, ccall(cv(h.moduleFunc(stdio, "stdio", "print_sint16")), arg))
			case typ.Kind == types.Int || typ.Kind == types.Bool:
				r.stmts = append(r.stmts, ccall(cv(printInt16), arg))
			default:
				panic(&diag.Error{Msg: fmt.Sprintf("printf cannot print a value of type %s (strings (*u8 / []u8), integers, bool and enums)", typ)})
			}
		}
		return r
	})

	h.defmacroTyped("unittest_run_tests", voidMacro, func(h *Hlc, args []*cexpr, block *syntax.Block) macroResult {
		if h.v4() {
			return h.runTests4() // fc 4: @(test) の関数を集める (testing.go)
		}
		return runTestsV3(h)
	})
	// @run_tests_v3(): fc 3 までの @run_tests (スコープの test_* を呼んで stdio に出す)。fc 3 → 4 の migrate が @run_tests を
	// これに書き換える (出力が変わらないように)
	h.defmacroTyped("@run_tests_v3", voidMacro, func(h *Hlc, args []*cexpr, block *syntax.Block) macroResult { return runTestsV3(h) })

	// min(a, b) / max(a, b) / clamp(x, lo, hi): 型は引数の互換型で決まる (符号付きなら符号付き比較)。
	// 定数なら畳み込み、そうでなければ比較して入れ替えるコードをその場に出す (関数呼び出しは無い)
	for _, bi := range []struct {
		name string
		op   cop
		n    int
	}{{"min", opMin, 2}, {"max", opMax, 2}, {"clamp", opClamp, 3}} {
		h.defmacroTyped(bi.name, pureMacro, func(h *Hlc, args []*cexpr, block *syntax.Block) macroResult {
			if len(args) != bi.n {
				panic(&diag.Error{Msg: fmt.Sprintf("%s takes %d arguments", bi.name, bi.n)})
			}
			return macroResult{expr: &cexpr{kind: cOp, op: bi.op, args: args}}
		})
	}

	// cos(x) = sin(x + 64) のマクロ展開 (v1 の math.rb)。math モジュールの sin を参照する。
	// 関数にすると呼び出し側の asm が変わるので、インライン関数 (F2) が入るまでは組み込みマクロのまま。
	// `math.cos(x)` はドット参照が math のスコープから親 (グローバル) に辿り着くので従来どおり書ける
	h.defmacroTyped("cos", pureMacro, func(h *Hlc, args []*cexpr, block *syntax.Block) macroResult {
		m, ok := h.prog.Modules.Get("math")
		if !ok {
			panic(&diag.Error{Msg: "cos requires the math module (add `use math;`)"})
		}
		if len(args) != 1 {
			panic(&diag.Error{Msg: "cos takes 1 argument"})
		}
		arg := cop2(opAdd, args[0], cint(64))
		if h.v4() || h.rewriting() {
			// 角度の足し算は i8 で折り返す (`cos(127)` は sin(-65))。fc 4 は範囲外の定数を引数に渡せないので明示する。fc 3 も
			// 値のバイトは同じ (書き換えを集めるときに、展開で作った位置の無い式を報告しないため)
			arg = &cexpr{kind: cCast, args: []*cexpr{arg}, ty: h.prog.Types.IntType(1, true), ck: syntax.CastAs}
		}
		return macroResult{expr: ccall(cv(h.moduleFunc(h.prog.iface(m), "math", "sin")), arg)}
	})

	registerSliceBuiltins(h)
	registerFormatBuiltins(h)
	registerTestingBuiltins(h)
	registerCompressBuiltins(h)
	registerLogBuiltin(h)

	// @bank("name") は fc.toml の [bank.<name>] の番号 (コンパイル時に決まる u8。手動のバンク切り替え用。Agent/discussions/2026-09-20-v3-plan.md §3)
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

	// textmap(table [, po]) は定数式。文字表を持つ新しいマクロ値を返し、それを呼ぶと文字列が int[] 定数になる。
	// po (gettext の .po。"" なら無し) を渡すと、変換器の呼び出し `_T("原文" [, "msgctxt"])` を訳文に差し替える
	// (`_T("原文", null)` は翻訳しない)
	h.defconstmacro("textmap", func(h *Hlc, args []*cexpr) *cexpr {
		if len(args) != 1 && len(args) != 2 {
			panic(&diag.Error{Msg: "textmap takes 1 or 2 arguments (path of the character table, and optionally a .po file)"})
		}
		table := mustString(args[0])
		conv := NewTextConverter(string(bytes.ReplaceAll(h.readFile(table), []byte("\r\n"), []byte("\n"))))
		var cat *poCatalog
		if len(args) == 2 {
			if po := mustString(args[1]); po != "" {
				cat = h.readPO(po)
			}
		}
		m := &macroDef{typing: &pureMacro} // 展開は定数 (型を決める段が展開して型を見る)
		tm := &textmapConv{m: m, table: table, conv: conv, cat: cat, warned: map[string]bool{}}
		m.textmap = tm
		m.fn = func(h *Hlc, args []*cexpr, block *syntax.Block) macroResult {
			if len(args) == 1 && args[0].kind == cInt && args[0].s == "char" {
				// _T('あ'): 1 文字を表で引いた 1 つのコード (整数の定数。濁点などで 2 つ以上のコードになる文字はエラー)
				c := tm.codes(h, string(rune(args[0].n)))
				if len(c) != 1 {
					panic(&diag.Error{Msg: fmt.Sprintf("%s converts to %d codes with the textmap (a character literal needs exactly 1; use a string: _T(\"...\"))", args[0].name, len(c))})
				}
				return macroResult{expr: cint(c[0])}
			}
			codes := tm.codes(h, tm.text(h, args))
			codes = append(codes, 0)
			elems := make([]*cexpr, len(codes))
			for i, c := range codes {
				elems[i] = cint(c)
			}
			return macroResult{expr: carray(elems)}
		}
		return symName(&Symbol{Macro: m})
	})
}

// textmapConv は textmap(...) が作った変換器 (`const _T = @textmap(...)` の _T)。@format(buf, _T("…"), ...) も使う (format.go)。
type textmapConv struct {
	m      *macroDef // 変換器のマクロ (警告の名前)
	table  string
	conv   *TextConverter
	cat    *poCatalog // .po (nil なら翻訳しない)
	warned map[string]bool
}

// text は変換器の呼び出しの引数 (原文 [, msgctxt か null]) の、翻訳した文字列。
func (t *textmapConv) text(h *Hlc, args []*cexpr) string {
	if len(args) != 1 && len(args) != 2 {
		panic(&diag.Error{Msg: "text conversion takes a string and an optional msgctxt string (or null not to translate)"})
	}
	src := mustString(h.constEval(args[0]))
	key := poKey{id: textKey(src)}
	translate := true
	if len(args) == 2 {
		if args[1].kind == cNull {
			translate = false // _T("…", null): 翻訳しない (デバッグ表示など。.pot にも入れない)
		} else {
			key.hasCtxt, key.ctxt = true, mustString(h.constEval(args[1]))
		}
	}
	if t.cat != nil && translate && src != "" { // "" はヘッダーの msgid と同じなので照合しない
		if s, ok, reason := t.cat.lookup(key); ok {
			return s
		} else {
			h.textWarn(t.warned, "%s: no translation for %q%s in %s (%s); using the original text", textmapName(t.m), src, ctxtNote(key), t.cat.path, reason)
		}
	}
	return src
}

// codes は text を文字コードの並びにする (翻訳ありなら、文字表に無い文字を警告する)。
func (t *textmapConv) codes(h *Hlc, text string) []int {
	codes, added := t.conv.convReport(text)
	if t.cat != nil && len(added) > 0 {
		// 翻訳ありでは、文字表に無い文字を知らせる (表の後ろに足されて表示が化ける。同じ文字は最初の 1 回だけ)
		var cs []string
		for _, c := range added {
			cs = append(cs, fmt.Sprintf("%q (U+%04X)", c, c))
		}
		h.textWarn(t.warned, "%s: %q has characters not in the character table %s: %s", textmapName(t.m), text, t.table, strings.Join(cs, ", "))
	}
	return codes
}

// readPO は .po を読む (同じファイルは 1 回だけ読む。_T / _M / _I が同じ .po を渡すため)。
func (h *Hlc) readPO(path string) *poCatalog {
	_, abs := h.resolveFile(path)
	if cat := h.prog.poCatalogs[abs]; cat != nil {
		return cat
	}
	cat, err := parsePO(path, h.readFile(path))
	if err != nil {
		panic(err)
	}
	h.prog.poCatalogs[abs] = cat
	return cat
}

// textWarn は textmap の警告を出す (同じ位置の同じ警告は 1 回だけ。定数式が評価し直されることがあるため)。
func (h *Hlc) textWarn(warned map[string]bool, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	k := h.curPos.String() + "\x00" + msg
	if warned[k] {
		return
	}
	warned[k] = true
	h.warn("%s", msg)
}

// textmapName は警告に出す変換器の名前 (`const _T = @textmap(...)` の _T)。
func textmapName(m *macroDef) string {
	if m.name == "" {
		return "text conversion"
	}
	return m.name
}

// ctxtNote は警告に添える msgctxt。
func ctxtNote(k poKey) string {
	if !k.hasCtxt {
		return " (no msgctxt)"
	}
	return fmt.Sprintf(" (msgctxt %q)", k.ctxt)
}

// defconstmacro は定数式で評価される組み込みをグローバルに登録する。
func (h *Hlc) defconstmacro(name string, fn ConstMacroFn) *macroDef {
	return h.declareMacro(&macroDef{name: name, constFn: fn})
}

// stdioModule は組み込みが参照する stdio モジュールを返す (読み込まれていなければエラー)。
func (h *Hlc) stdioModule(builtin string) *ModuleInterface {
	m, ok := h.prog.Modules.Get("stdio")
	if !ok {
		panic(&diag.Error{Msg: fmt.Sprintf("%s requires the stdio module (add `use stdio;` or `use * from stdio;`)", builtin)})
	}
	return h.prog.iface(m)
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

// moduleFunc は組み込みが呼ぶモジュールの関数 name を引く。無ければエラー (ソースのディレクトリに同じ名前のモジュール
// (stdio.fc など) があると fclib のものが隠れ、nil のまま呼んで panic していた。survey 2026-09-27)。
func (h *Hlc) moduleFunc(m *ModuleInterface, mod, name string) *ir.Value {
	var v *ir.Value
	if sym := m.LookupInternal(name); sym != nil {
		v = sym.Val
	}
	if v == nil {
		panic(&diag.Error{Msg: fmt.Sprintf("module %s has no %s (needed by a builtin); is a %s.fc in your source directory hiding fclib's %s?", mod, name, mod, mod)})
	}
	return v
}

// runTestsV3 は fc 3 までの @run_tests: stdio.init の後、スコープの test_* の関数を宣言の順に「名前:」を出して呼び、stdio.exit(0)。
func runTestsV3(h *Hlc) macroResult {
	stdio := h.stdioModule("unittest_run_tests")
	print := h.moduleFunc(stdio, "stdio", "print")
	exit := h.moduleFunc(stdio, "stdio", "exit")
	init := h.moduleFunc(stdio, "stdio", "init")
	r := macroResult{stmts: []*cexpr{ccall(cv(init))}}
	for _, id := range h.scope.IdList() {
		if len(id) >= 5 && id[:5] == "test_" {
			r.stmts = append(r.stmts,
				ccall(cv(print), h.cstrZ(fmt.Sprintf("%s:", id))),
				ccall(cident(id)),
				ccall(cv(print), h.cstrZ("\n")),
			)
		}
	}
	r.stmts = append(r.stmts, ccall(cv(exit), cint(0)))
	return r
}
