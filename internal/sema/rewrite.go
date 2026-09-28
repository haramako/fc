package sema

// fc 3 → fc 4 の migrate のための書き換えの報告 (doc/v4_plan.md §0)。
//
// fc 4 は整数の規則などの意味を変えるので、構文だけを見る書き換え (fc 2 → 3。internal/migrate の Rules) では移せない。
// Program.CollectRewrites なら、sema が fc 3 のモジュールを fc 3 の意味でコンパイルしながら、fc 4 で意味が変わる所を
// 「fc 4 でも今と同じ意味になる書き方」への書き換えとして集める (ROM を変えない移行: 2026-09-28 決定)。migrate がそれを
// ソースに当てて `#fc 4` にする。書き換えを作れない所 (位置の分からない合成の式など) は RewriteErrors に残し、migrate は
// エラーにする (黙って意味が変わるのを防ぐ)。

import (
	"fmt"
	"path/filepath"

	"github.com/haramako/fc/internal/syntax"
)

// Rewrite はソース File の [Start, End) を Text に置き換える書き換え 1 つ (Start == End なら挿入)。
type Rewrite struct {
	File       string // 読み込みに使った実パス (Source.Abs)
	Start, End int    // バイト位置 (CRLF を LF にした後の内容で)
	Text       string
	Rule       string // 規則の名前 (表示・テスト用)
}

// RewriteError は書き換えを作れなかった所。
type RewriteError struct {
	File string // 読み込みに使った実パス (分からなければ "")
	Msg  string
}

// OverlayKey は Program.Overlay のキー (絶対パス)。
func OverlayKey(path string) string {
	if a, err := filepath.Abs(path); err == nil {
		return a
	}
	return filepath.Clean(path)
}

// rewriting は fc 3 → fc 4 の書き換えを集めている最中か (fc 3 のモジュールをコンパイルしているときだけ)。
func (h *Hlc) rewriting() bool {
	return h.prog.CollectRewrites && h.module != nil && h.module.Version == syntax.Version3
}

// addRewrite は書き換えを 1 つ足す。同じものは 1 つにまとめる (1 つのモジュールは入口ごとにコンパイルされうる)。
func (h *Hlc) addRewrite(rule string, start, end int, text string) {
	src := h.prog.Sources[h.module.Id]
	if src == nil {
		h.rewriteError(rule, "the source of the module is unknown")
		return
	}
	r := Rewrite{File: src.Abs, Start: start, End: end, Text: text, Rule: rule}
	if h.prog.rewriteSeen == nil {
		h.prog.rewriteSeen = map[Rewrite]bool{}
	}
	if h.prog.rewriteSeen[r] {
		return
	}
	h.prog.rewriteSeen[r] = true
	h.prog.Rewrites = append(h.prog.Rewrites, r)
}

// rewriteError は書き換えを作れなかった所を残す。
func (h *Hlc) rewriteError(rule, why string) {
	file := ""
	if src := h.prog.Sources[h.module.Id]; src != nil {
		file = src.Abs
	}
	h.prog.RewriteErrors = append(h.prog.RewriteErrors, RewriteError{File: file, Msg: fmt.Sprintf("%s: %s: cannot rewrite for fc 4 (%s)", h.curPos, rule, why)})
}

// rewriteAs は式 c を `c as T` にする書き換えを足す (c が 1 語か括弧で囲まれていなければ `(c) as T`)。c の範囲が分からなければ
// rewriteError。
func (h *Hlc) rewriteAs(rule string, c *cexpr, typ string) {
	if c == nil || !c.pos.IsValid() || !c.end.IsValid() {
		h.rewriteError(rule, "the expression has no position")
		return
	}
	src := h.prog.Sources[h.module.Id]
	if src == nil {
		h.rewriteError(rule, "the source of the module is unknown")
		return
	}
	s, e := c.pos.Offset, c.end.Offset
	if s < 0 || e > len(src.Src) || s >= e {
		h.rewriteError(rule, "the expression has no position")
		return
	}
	if simpleExpr(src.Src[s:e]) {
		h.addRewrite(rule, e, e, " as "+typ)
		return
	}
	h.addRewrite(rule, s, s, "(")
	h.addRewrite(rule, e, e, ") as "+typ)
}

// simpleExpr は b が `as` を後ろに付けてもそのまま読める式か: 1 語 (名前・数・`a.b`。前に `-` があってもよい: 単項演算子は
// `as` より強い)、または全体が 1 組の括弧。
func simpleExpr(b []byte) bool {
	if len(b) > 1 && b[0] == '-' {
		b = b[1:]
	}
	word := true
	for _, ch := range b {
		if !(ch == '_' || ch == '.' || ch >= '0' && ch <= '9' || ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z') {
			word = false
			break
		}
	}
	if word {
		return true
	}
	if len(b) < 2 || b[0] != '(' || b[len(b)-1] != ')' {
		return false
	}
	depth := 0
	for i, ch := range b {
		switch ch {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 && i != len(b)-1 {
				return false // `(a) + (b)`
			}
		}
	}
	return true
}
