package fcdoc

import (
	"strings"
	"testing"
)

const sample = `#fc 4
// 見本のモジュール。説明の 2 文目。
//
//   sample.f(1);
@(bank: -1);

public const A = 1;
public const B = 2;   // B の説明

// 長い表
public const TABLE = [1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21];

// 足す (x < 10 のとき)
// 2 行目
public function f(x:u8):u8
{
	return x + 1;
}

function private_fn():void {}

public struct P {
	x:u8;
}

// {{ と <b> を含む説明
public var v:u8;
`

func TestParse(t *testing.T) {
	m, err := Parse("lib/sample.fc", []byte(strings.ReplaceAll(sample, "\n", "\r\n")))
	if err != nil {
		t.Fatal(err)
	}
	if m.Name != "sample" || m.Doc != "見本のモジュール。説明の 2 文目。\n\n  sample.f(1);" {
		t.Errorf("module: %q %q", m.Name, m.Doc)
	}
	if got := m.Summary(); got != "見本のモジュール。" {
		t.Errorf("summary: %q", got)
	}
	var names []string
	for _, it := range m.Items {
		names = append(names, it.Heading())
	}
	if got := strings.Join(names, " / "); got != "A, B / TABLE / f / P / v" {
		t.Fatalf("items: %s", got)
	}
	if ab := m.Items[0]; ab.Kind != "const" || ab.Code != "public const A = 1;\npublic const B = 2;   // B の説明" {
		t.Errorf("A, B: %s %q", ab.Kind, ab.Code)
	}
	if tb := m.Items[1]; tb.Code != "public const TABLE = …;" || tb.Doc != "長い表" {
		t.Errorf("TABLE: %q %q", tb.Code, tb.Doc)
	}
	if f := m.Find("f"); f.Code != "public function f(x:u8):u8" || f.Doc != "足す (x < 10 のとき)\n2 行目" {
		t.Errorf("f: %q %q", f.Code, f.Doc)
	}
	if m.Find("private_fn") != nil {
		t.Error("private の関数が出ている")
	}
	md := m.Markdown("https://example.com/sample.fc")
	for _, want := range []string{
		"# sample\n", "::: v-pre", "```fc\nsample.f(1);\n```", "## f\n\n```fc\npublic function f(x:u8):u8\n```\n\n足す (x &lt; 10 のとき) 2 行目\n",
		"{{ と &lt;b&gt; を含む説明", "[lib/sample.fc](https://example.com/sample.fc)",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("Markdown に %q が無い:\n%s", want, md)
		}
	}
}

func TestJoinLines(t *testing.T) {
	if got := joinLines([]string{"abc", "def", "日本", "語", "x", "描画を止めている間は、", "frame の"}); got != "abc def 日本語 x 描画を止めている間は、frame の" {
		t.Errorf("got %q", got)
	}
}
