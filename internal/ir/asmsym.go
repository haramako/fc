package ir

import "regexp"

// reAsmSym は fc のシンボルの形の語 (`_` で始まる)。前が識別子の文字の `_` は語の途中なので数えない (`nsd_main` の `_main`、
// ローカルラベル `@jsr_on_vsync` の `_on_vsync`)。
var reAsmSym = regexp.MustCompile(`(?:^|[^A-Za-z0-9_$@])(_[A-Za-z0-9_$]+)`)

// AsmSymbols はアセンブラのテキストが参照しうる fc のシンボル (関数・変数) を返す (include した asm のファイルと
// インラインアセンブラ。関数の呼び出し規約と変数の volatile の判定用)。
func AsmSymbols(text string) []string {
	var r []string
	for _, m := range reAsmSym.FindAllStringSubmatch(text, -1) {
		r = append(r, m[1])
	}
	return r
}
