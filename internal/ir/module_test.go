package ir

import "testing"

func TestOptions(t *testing.T) {
	var o Options
	o.Set("a", OptionValue{Kind: OptInt, Int: 1})
	o.Set("b", OptionValue{Kind: OptStr, Str: "x"})
	o.Set("a", OptionValue{Kind: OptInt, Int: 2}) // 後勝ち・位置維持
	if len(o) != 2 || o[0].Key != "a" || o[0].Value.Int != 2 || o[1].Key != "b" {
		t.Errorf("Options: %+v", o)
	}
	if n, ok := o.Int("a"); !ok || n != 2 {
		t.Error("Int")
	}
	if _, ok := o.Int("b"); ok {
		t.Error("文字列を Int で取れてはいけない")
	}
	if !o.Has("b") || o.Has("c") {
		t.Error("Has")
	}
	if (OptionValue{Kind: OptIdent, Str: "my"}).Text() != "my" || (OptionValue{Kind: OptInt, Int: 5}).Text() != "5" {
		t.Error("Text")
	}
}
