package sema

// v1 の include("xxx.rb") (Ruby マクロ) の Go 実装。ファイル名キーで解決する。
// v2 では include("*.rb") は廃止 (doc/v2_grammar.md §3.4, §3.5):
//   - stdio.rb / unittest.rb / stdmacro.rb の中身は組み込み (builtins.go) になったので include は無視する
//   - math.rb (cos) も組み込み (math モジュールの sin を参照する)
//   - castle の macro.rb (_T / _M) は v1 のあいだだけ。v2 では const _T = textmap("...") に置き換える

import (
	"bytes"

	"github.com/haramako/fc/internal/syntax"
)

var macroFiles = map[string]func(h *Hlc){
	"stdmacro.rb": func(h *Hlc) {}, // times は未使用のまま削除
	"stdio.rb":    func(h *Hlc) {}, // printf は組み込み
	"unittest.rb": func(h *Hlc) {}, // unittest_run_tests は組み込み
	"math.rb":     func(h *Hlc) {}, // cos は組み込み
	"macro.rb":    registerCastleMacros,
}

// castle プロジェクトの src/macro.rb (テキスト変換マクロ _T / _M)。
// VERSION_STR は 2026-09-13 に廃止 (castle 側を固定文字列 _M("VERSION 0.5.0") にした)。
// Ruby版と同じく、フォント文字表 (../tmp/font/*.chr.txt) は登録時に読む。
// パスはソースの検索パス (先頭はソースディレクトリ) 基準。
func registerCastleMacros(h *Hlc) {
	readText := func(path string) string {
		return string(bytes.ReplaceAll(h.readFile(path), []byte("\r\n"), []byte("\n")))
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

	h.defmacro("_T", func(h *Hlc, args []*cexpr, block *syntax.Block) macroResult {
		return textArray(append(conv.Conv(mustString(args[0])), 0))
	})

	h.defmacro("_M", func(h *Hlc, args []*cexpr, block *syntax.Block) macroResult {
		return textArray(append(miscConv.Conv(mustString(args[0])), 0))
	})
}
