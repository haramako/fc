package syntax

import (
	"reflect"
	"testing"
)

// tok はテスト用の簡易トークン表現。
type tok struct {
	kind Kind
	text string // Ident/記号/キーワードの綴り、Number は 10 進表記、String は解釈後の値
}

func toks(src string) ([]tok, []Comment) {
	ts, cs, err := Tokenize([]byte(src), "test.fc")
	if err != nil {
		panic(err)
	}
	var r []tok
	for _, t := range ts {
		switch t.Kind {
		case Number:
			r = append(r, tok{Number, itoa(t.Int)})
		case String:
			r = append(r, tok{String, t.Str})
		default:
			r = append(r, tok{t.Kind, t.Text})
		}
	}
	return r, cs
}

func itoa(n int) string {
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
		if n == 0 {
			break
		}
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}

func TestTokenize(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []tok
	}{
		{"symbols", "<= >= == += -= != -> << >> && || ( ) { } ; : < > [ ] + - * / % & | ^ = , . !",
			[]tok{{Leq, "<="}, {Geq, ">="}, {EqEq, "=="}, {AddEq, "+="}, {SubEq, "-="}, {Neq, "!="},
				{Arrow, "->"}, {Shl, "<<"}, {Shr, ">>"}, {AndAnd, "&&"}, {OrOr, "||"},
				{LParen, "("}, {RParen, ")"}, {LBrace, "{"}, {RBrace, "}"}, {Semicolon, ";"}, {Colon, ":"},
				{Lt, "<"}, {Gt, ">"}, {LBrack, "["}, {RBrack, "]"}, {Plus, "+"}, {Minus, "-"},
				{Star, "*"}, {Slash, "/"}, {Percent, "%"}, {Amp, "&"}, {Pipe, "|"}, {Caret, "^"},
				{Assign, "="}, {Comma, ","}, {Dot, "."}, {Not, "!"}}},
		{"symbol precedence", "<<=>>=",
			// 先頭一致の並び順: "<<" が "<=" より後なので "<=" が先に当たる... ではなく、並び順は <= >= == ... << >> 。
			// 先頭 "<<=" に対して "<=" は一致せず、"<<" が当たる。次 "=>>=" は "=" の後 ">>" の後 "="。
			[]tok{{Shl, "<<"}, {Assign, "="}, {Shr, ">>"}, {Assign, "="}}},
		{"keywords", "include function const var options if else elsif loop while for return break continue incbin switch case default use as from public private",
			[]tok{{KwInclude, "include"}, {KwFunction, "function"}, {KwConst, "const"}, {KwVar, "var"},
				{KwOptions, "options"}, {KwIf, "if"}, {KwElse, "else"}, {KwElsif, "elsif"}, {KwLoop, "loop"},
				{KwWhile, "while"}, {KwFor, "for"}, {KwReturn, "return"}, {KwBreak, "break"},
				{KwContinue, "continue"}, {KwIncbin, "incbin"}, {KwSwitch, "switch"}, {KwCase, "case"},
				{KwDefault, "default"}, {KwUse, "use"}, {KwAs, "as"}, {KwFrom, "from"}, {KwPublic, "public"},
				{KwPrivate, "private"}}},
		{"identifiers", "foo _bar baz9 If Function",
			[]tok{{Ident, "foo"}, {Ident, "_bar"}, {Ident, "baz9"}, {Ident, "If"}, {Ident, "Function"}}},
		// 10 進は \d+ のみ。`1_000` は 1 + 識別子 `_000` (16 進の \w+ とは異なる)
		{"decimal", "0 42 1_000 007",
			[]tok{{Number, "0"}, {Number, "42"}, {Number, "1"}, {Ident, "_000"}, {Number, "7"}}},
		{"hex", "0x10 0XfF 0xab_cd",
			[]tok{{Number, "16"}, {Number, "255"}, {Number, "43981"}}},
		{"binary", "0b101 0B11",
			[]tok{{Number, "5"}, {Number, "3"}}},
		{"negative is separate token", "-5 -0x10",
			[]tok{{Minus, "-"}, {Number, "5"}, {Minus, "-"}, {Number, "16"}}},
		// Ruby 由来の癖 (保存する): 無効な桁は値として無視されるが文字としては消費される
		{"quirk: 0b2 is accepted as 0", "0b2 0b12",
			[]tok{{Number, "0"}, {Number, "1"}}},
		{"quirk: 0xZZ is 0, 0x1g consumes g", "0xZZ 0x1g",
			[]tok{{Number, "0"}, {Number, "1"}}},
		{"quirk: 12ab splits", "12ab",
			[]tok{{Number, "12"}, {Ident, "ab"}}},
		{"quirk: 0b without digit", "0bx",
			[]tok{{Number, "0"}, {Ident, "bx"}}},
		{"quirk: underscore rules (hex)", "0x1__0 0x_1 0x1_ 0x1_f",
			// \w+ で全部消費し、to_i(16) は `_` を「前後が数字」のときだけ区切りとして許す
			[]tok{{Number, "1"}, {Number, "0"}, {Number, "1"}, {Number, "31"}}},
		{"dq string", `"abc" "a\nb" "\x41\x4a" "\t\q" ""`,
			[]tok{{String, "abc"}, {String, "a\nb"}, {String, "AJ"}, {String, `\t\q`}, {String, ""}}},
		{"quirk: \\xZZ is 0, \\x with one hex digit", `"\xZZ" "\x4Z"`,
			[]tok{{String, "\x00"}, {String, "\x04"}}},
		{"dq string with raw newline", "\"a\nb\"",
			[]tok{{String, "a\nb"}}},
		{"sq string (no escape)", `'abc' 'a\nb' 'x\'y'`,
			[]tok{{String, "abc"}, {String, `a\nb`}, {String, `x\'y`}}},
		{"triple string", "\"\"\"line1\nline2 \"quoted\"\\n\"\"\" x",
			[]tok{{String, "line1\nline2 \"quoted\"\n"}, {Ident, "x"}}},
		{"triple string fallback to dq when unterminated", `"""abc" d`,
			// """abc" → 閉じ """ が無いので "" として解釈: `""` (空) + `"abc"` + d
			[]tok{{String, ""}, {String, "abc"}, {Ident, "d"}}},
		{"line comment", "a // comment\nb",
			[]tok{{Ident, "a"}, {Ident, "b"}}},
		{"empty line comment does not swallow next line", "//\nb\n//\n//\nc",
			[]tok{{Ident, "b"}, {Ident, "c"}}},
		{"line comment at EOF without newline", "a // c",
			[]tok{{Ident, "a"}}},
		{"block comment", "a /* x\ny */ b /**/ c /***/ d",
			[]tok{{Ident, "a"}, {Ident, "b"}, {Ident, "c"}, {Ident, "d"}}},
		{"unterminated block comment falls back to tokens", "a /* b",
			[]tok{{Ident, "a"}, {Slash, "/"}, {Star, "*"}, {Ident, "b"}}},
		{"whitespace kinds", "a\t\r\n\f\vb",
			[]tok{{Ident, "a"}, {Ident, "b"}}},
		{"empty", "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := toks(tt.src)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("tokens mismatch\n got:  %v\n want: %v", got, tt.want)
			}
		})
	}
}

func TestTokenizeError(t *testing.T) {
	for _, src := range []string{"#", "a @ b", "\"unterminated", "'x", "a \"b\\\nc\""} {
		_, _, err := Tokenize([]byte(src), "e.fc")
		if err == nil {
			t.Errorf("%q: エラーになるべき", src)
			continue
		}
		e, ok := err.(*Error)
		if !ok {
			t.Errorf("%q: *Error であるべき: %T", src, err)
			continue
		}
		if e.Filename != "e.fc" || !e.Pos.IsValid() {
			t.Errorf("%q: 位置情報が不正: %+v", src, e)
		}
	}
	// 旧レキサのメッセージ形式を維持 (invalid token at <line>)
	_, _, err := Tokenize([]byte("a\nb\n  #"), "e.fc")
	if err == nil || err.Error() != "invalid token at 3" {
		t.Errorf("メッセージ不一致: %v", err)
	}
}

func TestComments(t *testing.T) {
	src := "// head\nvar a; /* mid\nline */ var b; //tail"
	_, cs := toks(src)
	want := []Comment{
		{Pos{0, 1, 1}, Pos{7, 1, 8}, "// head"},
		{Pos{15, 2, 8}, Pos{29, 3, 8}, "/* mid\nline */"},
		{Pos{37, 3, 16}, Pos{43, 3, 22}, "//tail"},
	}
	if !reflect.DeepEqual(cs, want) {
		t.Errorf("comments mismatch\n got:  %+v\n want: %+v", cs, want)
	}
	if !cs[0].IsLine() || cs[1].IsLine() {
		t.Error("IsLine が不正")
	}
}

func TestPositions(t *testing.T) {
	// 文字列内の改行・コメント内の改行・タブを含めて行と列が正確であること
	src := "ab \"x\ny\" /* c\n */\n\tz"
	ts, _, err := Tokenize([]byte(src), "p.fc")
	if err != nil {
		t.Fatal(err)
	}
	type pe struct{ pos, end Pos }
	want := []pe{
		{Pos{0, 1, 1}, Pos{2, 1, 3}},   // ab
		{Pos{3, 1, 4}, Pos{8, 2, 3}},   // "x\ny"
		{Pos{19, 4, 2}, Pos{20, 4, 3}}, // z (タブは 1 列)
	}
	if len(ts) != len(want) {
		t.Fatalf("トークン数: got %d want %d", len(ts), len(want))
	}
	for i, tk := range ts {
		if tk.Pos != want[i].pos || tk.End != want[i].end {
			t.Errorf("%d: %s pos=%+v end=%+v want pos=%+v end=%+v", i, tk, tk.Pos, tk.End, want[i].pos, want[i].end)
		}
	}
	// EOF の位置は末尾
	l := NewLexer([]byte(src), "p.fc")
	var last Token
	for {
		tk, err := l.Next()
		if err != nil {
			t.Fatal(err)
		}
		if tk.Kind == EOF {
			last = tk
			break
		}
	}
	if last.Pos != (Pos{20, 4, 3}) {
		t.Errorf("EOF pos: %+v", last.Pos)
	}
}

func TestKindString(t *testing.T) {
	if KwVar.String() != "var" || Leq.String() != "<=" || Ident.String() != "Ident" {
		t.Error("Kind.String")
	}
	if !KwVar.IsKeyword() || Ident.IsKeyword() || Leq.IsKeyword() {
		t.Error("IsKeyword")
	}
}
