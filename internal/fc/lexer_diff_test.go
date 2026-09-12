package fc

// 旧レキサ (lexer.go) と新レキサ (internal/syntax) の差分テスト (doc/v2_plan.md R1-a)。
// コーパス全体 (test/ fclib/ examples/ の全 .fc) でトークン列が一致することを確認する。
// 旧レキサは R1-c で削除されるので、このテストもそのとき削除する。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haramako/fc/internal/syntax"
)

// 旧トークンコード → 新 Kind
var oldToNewKind = map[int]syntax.Kind{
	NUMBER: syntax.Number, IDENT: syntax.Ident, STRING: syntax.String,
	kINCLUDE: syntax.KwInclude, kFUNCTION: syntax.KwFunction, kCONST: syntax.KwConst, kVAR: syntax.KwVar,
	kOPTIONS: syntax.KwOptions, kIF: syntax.KwIf, kELSE: syntax.KwElse, kELSIF: syntax.KwElsif,
	kLOOP: syntax.KwLoop, kWHILE: syntax.KwWhile, kFOR: syntax.KwFor, kRETURN: syntax.KwReturn,
	kBREAK: syntax.KwBreak, kCONTINUE: syntax.KwContinue, kINCBIN: syntax.KwIncbin,
	kSWITCH: syntax.KwSwitch, kCASE: syntax.KwCase, kDEFAULT: syntax.KwDefault,
	kUSE: syntax.KwUse, kAS: syntax.KwAs, kFROM: syntax.KwFrom, kPUBLIC: syntax.KwPublic, kPRIVATE: syntax.KwPrivate,
	LEQ: syntax.Leq, GEQ: syntax.Geq, EQEQ: syntax.EqEq, ADDEQ: syntax.AddEq, SUBEQ: syntax.SubEq,
	NEQ: syntax.Neq, ARROW: syntax.Arrow, LSHIFT: syntax.Shl, RSHIFT: syntax.Shr,
	ANDAND: syntax.AndAnd, OROR: syntax.OrOr,
	'(': syntax.LParen, ')': syntax.RParen, '{': syntax.LBrace, '}': syntax.RBrace, ';': syntax.Semicolon,
	':': syntax.Colon, '<': syntax.Lt, '>': syntax.Gt, '[': syntax.LBrack, ']': syntax.RBrack,
	'+': syntax.Plus, '-': syntax.Minus, '*': syntax.Star, '/': syntax.Slash, '%': syntax.Percent,
	'&': syntax.Amp, '|': syntax.Pipe, '^': syntax.Caret, '=': syntax.Assign, ',': syntax.Comma,
	'.': syntax.Dot, '!': syntax.Not,
}

func corpusFiles(t *testing.T) []string {
	t.Helper()
	var files []string
	for _, dir := range []string{"test", "fclib", "examples"} {
		err := filepath.WalkDir(filepath.Join(absRepoRoot, dir), func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() && strings.HasSuffix(path, ".fc") {
				files = append(files, path)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(files) < 50 {
		t.Fatalf("コーパスが少なすぎる: %d", len(files))
	}
	return files
}

func TestLexerDiff(t *testing.T) {
	for _, path := range corpusFiles(t) {
		rel, _ := filepath.Rel(absRepoRoot, path)
		t.Run(filepath.ToSlash(rel), func(t *testing.T) {
			src, err := ReadSource(path)
			if err != nil {
				t.Fatal(err)
			}
			newToks, _, nerr := syntax.Tokenize(src, path)

			old := NewLexer(src, path)
			i := 0
			for {
				code, val, oerr := old.NextToken()
				if oerr != nil {
					if nerr == nil {
						t.Fatalf("旧レキサのみエラー: %v", oerr)
					}
					return // 両方エラー (errors.fc 等)
				}
				if code == 0 {
					break
				}
				if nerr != nil {
					t.Fatalf("新レキサのみエラー: %v", nerr)
				}
				if i >= len(newToks) {
					t.Fatalf("新レキサのトークンが足りない (旧 %d 個目 %v)", i, val)
				}
				nt := newToks[i]
				wantKind, ok := oldToNewKind[code]
				if !ok {
					t.Fatalf("未知の旧トークン %d", code)
				}
				if nt.Kind != wantKind {
					t.Fatalf("%d 個目: 種別不一致 old=%v(%d) new=%v", i, val, code, nt)
				}
				switch code {
				case NUMBER:
					if nt.Int != val.(int) {
						t.Fatalf("%d 個目: 数値不一致 old=%d new=%d", i, val, nt.Int)
					}
				case STRING:
					if nt.Str != val.(string) {
						t.Fatalf("%d 個目: 文字列不一致 old=%q new=%q", i, val, nt.Str)
					}
				case IDENT:
					if nt.Text != string(val.(Sym)) {
						t.Fatalf("%d 個目: 識別子不一致 old=%s new=%s", i, val, nt.Text)
					}
				default:
					if nt.Text != val.(string) {
						t.Fatalf("%d 個目: 綴り不一致 old=%s new=%s", i, val, nt.Text)
					}
				}
				i++
			}
			if i != len(newToks) {
				t.Fatalf("新レキサのトークンが多い: old=%d new=%d (次: %v)", i, len(newToks), newToks[i])
			}
		})
	}
}
