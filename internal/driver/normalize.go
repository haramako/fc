package driver

// 生成アセンブラの正規化 (golden 比較と fcc migrate の検証で共用)。

import (
	"regexp"
	"strconv"
	"strings"
)

// コンパイラが連番で生成する識別子:
//
//	@<name>_<N>  HLC のラベル (then/else/end/begin)
//	@<N>         LLC のラベル
//	_D<N>        無名関数 ($N) のマングル名
//	_<N> / __<N> 配列リテラルのデータブロック (関数内は _<N>、モジュールレベルは _<mod>__<N>)
var reGenLabel = regexp.MustCompile(`@[a-z]+_\d+|@\d+|_D\d+|__\d+\b|(?:^|[^\w])_\d+\b`)

// normalizeLabels は連番識別子を出現順の通し番号に置き換える (採番方式の変更を吸収する: doc/v2_plan.md R3-c)。
func normalizeLabels(s string) string {
	seen := map[string]int{}
	return reGenLabel.ReplaceAllStringFunc(s, func(m string) string {
		// (?:^|[^\w]) の先行文字を保つ
		prefix := ""
		if m[0] != '@' && m[0] != '_' {
			prefix, m = m[:1], m[1:]
		}
		n, ok := seen[m]
		if !ok {
			n = len(seen) + 1
			seen[m] = n
		}
		// 種別ごとの接頭辞を残し、数字だけを通し番号に
		i := len(m)
		for i > 0 && m[i-1] >= '0' && m[i-1] <= '9' {
			i--
		}
		return prefix + m[:i] + "N" + strconv.Itoa(n)
	})
}

var reIRComment = regexp.MustCompile(`^\s*; \d{4}:`)

// normalizeAsm は IRコメント行を除去する (dumper.rb の normalize_asm 相当 + 末尾改行)。
func normalizeAsm(lines []string) string {
	var out []string
	for _, line := range lines {
		if reIRComment.MatchString(line) {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n") + "\n"
}
