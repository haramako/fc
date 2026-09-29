package fclog

import (
	"regexp"
	"strings"
	"testing"
)

// TestMesenLogScriptFields: Mesen 用の Lua の中の地点の項目は `{ p = p, site = site }` で作り、`e.site` / `x.site` で読む
// (2026-09-28 の driver を分けたときの名前の置き換えが Lua の文字列の中の `site` まで `Site` にしていて、Lua のエラーで
// TestMesenLog が止まっていた。Mesen が起動中はそのテストが飛ぶので気づかなかった)。Lua の識別子は小文字だけ。
func TestMesenLogScriptFields(t *testing.T) {
	s := mesenLogScript(&LogFile{})
	if !strings.Contains(s, "{ p = p, site = site }") {
		t.Fatal("地点の項目の作り方が変わった (この検査も直す)")
	}
	if m := regexp.MustCompile(`\b[a-z]+\.Site\b|\[e\.Site\]`).FindString(s); m != "" {
		t.Errorf("Lua が大文字の Site を読んでいる: %s", m)
	}
	for _, want := range []string{"x.site.seq < y.site.seq", "e.site.prevs", "e.site.prg", "show(e.p, e.site)"} {
		if !strings.Contains(s, want) {
			t.Errorf("%q が無い", want)
		}
	}
}
