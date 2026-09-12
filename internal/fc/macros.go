package fc

// fclib/*.rb の Ruby マクロを Go ネイティブ実装に固定したもの。
// include("stdio.rb") 等はファイル名キーでここに解決される (確定方針参照)。
// 各登録関数は Ruby ファイルの instance_eval 時の動作 (トップレベルの @scope.find 等) を再現する。

import (
	"fmt"
	"strings"

	"github.com/haramako/fc/internal/syntax"
)

var macroFiles = map[string]func(h *Hlc){
	"stdmacro.rb": registerStdmacro,
	"stdio.rb":    registerStdio,
	"math.rb":     registerMath,
	"unittest.rb": registerUnittest,
	"macro.rb":    registerCastleMacros,
}

// fclib/stdmacro.rb
// times(n){...} は現行の Ruby 版でも展開結果が compile_statement で解釈できない形
// (ループ本体が文リストのまま) で、実際にはどこからも使われていない。
// include("stdmacro.rb") 自体は受理し、呼び出されたらエラーにする (v2 で削除予定: v2_decisions.md §3)。
func registerStdmacro(h *Hlc) {
	h.defmacro("times", func(h *Hlc, args []*cexpr, block *syntax.Block) macroResult {
		panic(&CompileError{Msg: "times macro is not supported"})
	})
}

// fclib/stdio.rb
func registerStdio(h *Hlc) {
	uint8p := TypeOf([]any{Sym("pointer"), Sym("uint8")})
	print := h.scope.Find(Sym("print"), true)
	printInt16 := h.scope.Find(Sym("print_int16"), true)

	h.defmacro("printf", func(h *Hlc, args []*cexpr, block *syntax.Block) macroResult {
		r := macroResult{stmts: []*cexpr{}}
		for _, arg := range args {
			// 旧実装は引数が定数値でないと ValType が落ちていた。同じく定数値を要求する
			typ := mustValue(arg).Type
			if CompatibleTypeOk(uint8p, typ) != nil {
				r.stmts = append(r.stmts, ccall(cv(print), arg))
			} else if typ.Kind == "int" {
				r.stmts = append(r.stmts, ccall(cv(printInt16), arg))
			}
		}
		return r
	})
}

// fclib/math.rb
func registerMath(h *Hlc) {
	sin := h.scope.Find(Sym("sin"), true)

	h.defmacro("cos", func(h *Hlc, args []*cexpr, block *syntax.Block) macroResult {
		return macroResult{expr: ccall(cv(sin), cop2(opAdd, args[0], cint(64)))}
	})
}

// castle プロジェクトの src/macro.rb (テキスト変換マクロ _T / _M / VERSION_STR)。
// Ruby版と同じく、フォント文字表 (../tmp/font/*.chr.txt) は登録時に、
// ../VERSION はマクロ実行時に、カレントディレクトリ相対で読む。
func registerCastleMacros(h *Hlc) {
	readText := func(path string) string {
		b, err := ReadSource(path)
		if err != nil {
			panic(&CompileError{Msg: err.Error()})
		}
		return string(b)
	}
	conv := NewTextConverter(readText("../tmp/font/text.chr.txt"))
	miscConv := NewTextConverter(readText("../tmp/font/misc_text.chr.txt"))

	textArray := func(codes []int) macroResult {
		elems := make([]*cexpr, len(codes))
		for i, c := range codes {
			elems[i] = cint(c)
		}
		return macroResult{expr: carray(elems)}
	}
	// 引数は文字列リテラル (BaseString を持つ定数値) でなければならない
	baseString := func(c *cexpr) string {
		s, ok := mustValue(c).BaseString.(string)
		if !ok {
			panic(&CompileError{Msg: "string literal required"})
		}
		return s
	}

	h.defmacro("_T", func(h *Hlc, args []*cexpr, block *syntax.Block) macroResult {
		return textArray(append(conv.Conv(baseString(args[0])), 0))
	})

	h.defmacro("_M", func(h *Hlc, args []*cexpr, block *syntax.Block) macroResult {
		return textArray(append(miscConv.Conv(baseString(args[0])), 0))
	})

	h.defmacro("VERSION_STR", func(h *Hlc, args []*cexpr, block *syntax.Block) macroResult {
		version := rubyChomp(readText("../VERSION"))
		return textArray(append(miscConv.Conv("VERSION "+version), 0))
	})
}

// rubyChomp は String#chomp 相当 (末尾の改行1つを除去)。
func rubyChomp(s string) string {
	if strings.HasSuffix(s, "\r\n") {
		return s[:len(s)-2]
	}
	if strings.HasSuffix(s, "\n") || strings.HasSuffix(s, "\r") {
		return s[:len(s)-1]
	}
	return s
}

// fclib/unittest.rb
func registerUnittest(h *Hlc) {
	stdioMod := h.scope.FindMust(Sym("stdio"), true).Val.(*Module)
	print := stdioMod.Scope.Find(Sym("print"), true)
	exit := stdioMod.Scope.Find(Sym("exit"), true)
	init := stdioMod.Scope.Find(Sym("init"), true)

	h.defmacro("unittest_run_tests", func(h *Hlc, args []*cexpr, block *syntax.Block) macroResult {
		r := macroResult{stmts: []*cexpr{ccall(cv(init))}}
		for _, id := range h.scope.IdList() {
			if len(id) >= 5 && id[:5] == "test_" {
				r.stmts = append(r.stmts,
					ccall(cv(print), cstr(fmt.Sprintf("%s:", id))),
					ccall(cident(string(id))),
					ccall(cv(print), cstr("\n")),
				)
			}
		}
		r.stmts = append(r.stmts, ccall(cv(exit), cint(0)))
		return r
	})
}
