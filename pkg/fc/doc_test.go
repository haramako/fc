package fc

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestStdDocs: 標準ライブラリの fc 4 のモジュールは、どれもモジュールの説明と、public の関数ごとの説明を持つ
// (利用者向けのサイトの docs/reference/std は fcc doc -md がこれから作る)。説明に開発者向けの Agent/ への参照を書かない。
func TestStdDocs(t *testing.T) {
	home, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	mods, err := NewWithHome(home).StdDocs()
	if err != nil {
		t.Fatal(err)
	}
	if len(mods) < 20 {
		t.Fatalf("モジュールが %d 個しかない", len(mods))
	}
	for _, m := range mods {
		if m.Doc == "" {
			t.Errorf("%s: モジュールの説明 (#fc の次の行からのコメント) が無い", m.Path)
		}
		if strings.Contains(m.Doc, "Agent/") {
			t.Errorf("%s: モジュールの説明に Agent/ への参照がある", m.Path)
		}
		for _, it := range m.Items {
			if it.Kind == "function" && it.Doc == "" {
				t.Errorf("%s:%d: public の関数 %s に説明 (直前のコメント) が無い", m.Path, it.Line, it.Names[0])
			}
			if strings.Contains(it.Doc, "Agent/") {
				t.Errorf("%s:%d: %s の説明に Agent/ への参照がある", m.Path, it.Line, it.Names[0])
			}
		}
	}
}
