package fc

// fclib/*.rb の Ruby マクロを Go ネイティブ実装に固定したもの。
// include("stdio.rb") 等はファイル名キーでここに解決される (確定方針参照)。
// 各登録関数は Ruby ファイルの instance_eval 時の動作 (トップレベルの @scope.find 等) を再現する。

import (
	"fmt"
	"strings"
)

var macroFiles = map[string]func(h *Hlc){
	"stdmacro.rb": registerStdmacro,
	"stdio.rb":    registerStdio,
	"math.rb":     registerMath,
	"unittest.rb": registerUnittest,
	"macro.rb":    registerCastleMacros,
}

// fclib/stdmacro.rb
// (注: 現行の Ruby 版では times の展開結果は compile_statement が解釈できない形であり、
//  実際にはどのテストからも使われていない。1:1 移植として同じ構造を返す)
func registerStdmacro(h *Hlc) {
	h.defmacro("times", func(h *Hlc, args []any, block any) any {
		limit := args[0]
		v := Sym("__loops2__")
		blockList, _ := block.([]any)
		return []any{
			[]any{Sym("var"), []any{[]any{v, Sym("int"), limit, NewOMap()}}},
			[]any{Sym("loop"),
				cons(blockList,
					[]any{Sym("exp"), []any{Sym("load"), v, []any{Sym("sub"), v, 1}}},
					[]any{Sym("if"), []any{Sym("not"), v}, []any{[]any{Sym("break")}}},
				),
			},
		}
	})
}

// fclib/stdio.rb
func registerStdio(h *Hlc) {
	uint8p := TypeOf([]any{Sym("pointer"), Sym("uint8")})
	print := h.scope.Find(Sym("print"), true)
	printInt16 := h.scope.Find(Sym("print_int16"), true)

	h.defmacro("printf", func(h *Hlc, args []any, block any) any {
		r := []any{Sym("block")}
		for _, arg := range args {
			if CompatibleTypeOk(uint8p, ValType(arg)) != nil {
				r = append(r, []any{Sym("exp"), []any{Sym("call"), print, []any{arg}}})
			} else if ValType(arg).Kind == "int" {
				r = append(r, []any{Sym("exp"), []any{Sym("call"), printInt16, []any{arg}}})
			}
		}
		return r
	})
}

// fclib/math.rb
func registerMath(h *Hlc) {
	sin := h.scope.Find(Sym("sin"), true)

	h.defmacro("cos", func(h *Hlc, args []any, block any) any {
		return []any{Sym("call"), sin, []any{[]any{Sym("add"), args[0], 64}}, nil}
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

	textArray := func(codes []int) any {
		elems := make([]any, len(codes))
		for i, c := range codes {
			elems[i] = c
		}
		return []any{Sym("array"), elems}
	}

	h.defmacro("_T", func(h *Hlc, args []any, block any) any {
		text := conv.Conv(args[0].(*Value).BaseString.(string))
		return textArray(append(text, 0))
	})

	h.defmacro("_M", func(h *Hlc, args []any, block any) any {
		text := miscConv.Conv(args[0].(*Value).BaseString.(string))
		return textArray(append(text, 0))
	})

	h.defmacro("VERSION_STR", func(h *Hlc, args []any, block any) any {
		version := rubyChomp(readText("../VERSION"))
		text := miscConv.Conv("VERSION " + version)
		return textArray(append(text, 0))
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

	h.defmacro("unittest_run_tests", func(h *Hlc, args []any, block any) any {
		r := []any{Sym("block"), []any{Sym("exp"), []any{Sym("call"), init, []any{}}}}
		for _, id := range h.scope.IdList() {
			if len(id) >= 5 && id[:5] == "test_" {
				r = append(r,
					[]any{Sym("exp"), []any{Sym("call"), print, []any{fmt.Sprintf("%s:", id)}}},
					[]any{Sym("exp"), []any{Sym("call"), id, []any{}}},
					[]any{Sym("exp"), []any{Sym("call"), print, []any{"\n"}}},
				)
			}
		}
		r = append(r, []any{Sym("exp"), []any{Sym("call"), exit, []any{0}}})
		return r
	})
}
