package syntax

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func corpus(t *testing.T) []string {
	t.Helper()
	root := filepath.Join("..", "..")
	var files []string
	for _, dir := range []string{"test", "fclib", "examples"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() && strings.HasSuffix(path, ".fc") && filepath.Base(path) != "errors.fc" {
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

func readSrc(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
}

func before(a, b Pos) bool { return a.Offset <= b.Offset }

// TestParseCorpusPositions はコーパス全体をパースし、全ノードの Pos/End が
// (1) 有効、(2) Pos < End、(3) 親が子を包含、(4) ファイル範囲内、であることを検査する。
// 併せて Ident/IntLit/StringLit の Text がソースの該当範囲と一致することも確認する。
func TestParseCorpusPositions(t *testing.T) {
	for _, path := range corpus(t) {
		t.Run(filepath.ToSlash(path), func(t *testing.T) {
			src := readSrc(t, path)
			f, err := Parse(src, path)
			if err != nil {
				t.Fatalf("パース失敗: %v", err)
			}
			nodes := 0
			var check func(n Node)
			check = func(n Node) {
				nodes++
				p, e := n.Pos(), n.End()
				if !p.IsValid() || !e.IsValid() {
					t.Fatalf("%T: 位置が無効 pos=%+v end=%+v", n, p, e)
				}
				if !(p.Offset < e.Offset) {
					t.Fatalf("%T at %s: Pos >= End (%+v, %+v)", n, p, p, e)
				}
				if e.Offset > len(src) {
					t.Fatalf("%T at %s: End がファイル範囲外", n, p)
				}
				switch x := n.(type) {
				case *Ident:
					if string(src[p.Offset:e.Offset]) != x.Name {
						t.Fatalf("Ident at %s: Text 不一致 %q", p, src[p.Offset:e.Offset])
					}
				case *IntLit:
					if string(src[p.Offset:e.Offset]) != x.Text {
						t.Fatalf("IntLit at %s: Text 不一致 %q", p, src[p.Offset:e.Offset])
					}
				case *StringLit:
					if string(src[p.Offset:e.Offset]) != x.Text {
						t.Fatalf("StringLit at %s: Text 不一致 %q", p, src[p.Offset:e.Offset])
					}
				}
				prev := p
				for _, c := range Children(n) {
					cp, ce := c.Pos(), c.End()
					if !before(p, cp) || !before(ce, e) {
						t.Fatalf("%T at %s [%s,%s): 子 %T [%s,%s) を包含しない", n, p, p, e, c, cp, ce)
					}
					if !before(prev, cp) {
						t.Fatalf("%T at %s: 子 %T at %s がソース順でない", n, p, c, cp)
					}
					prev = ce
					check(c)
				}
			}
			check(f)
			// コメントは出現順・非重複
			for i := 1; i < len(f.Comments); i++ {
				if !(f.Comments[i-1].End.Offset <= f.Comments[i].Pos.Offset) {
					t.Fatalf("コメントの順序不正: %+v, %+v", f.Comments[i-1], f.Comments[i])
				}
			}
			if nodes < 3 && len(src) > 50 {
				t.Fatalf("ノード数が少なすぎる: %d", nodes)
			}
		})
	}
}

func TestParseErrors(t *testing.T) {
	cases := []struct{ src, want string }{
		{"hoge fuga", "parse error"},
		{"var a = #;", "invalid token at 1"},
		{"function f():void {", "parse error"},
		{"var a:int = ;", "parse error"},
	}
	for _, c := range cases {
		_, err := Parse([]byte(c.src), "e.fc")
		if err == nil {
			t.Errorf("%q: エラーになるべき", c.src)
			continue
		}
		e, ok := err.(*Error)
		if !ok {
			t.Errorf("%q: *Error であるべき: %T", c.src, err)
			continue
		}
		if !strings.HasPrefix(e.Msg, c.want) {
			t.Errorf("%q: メッセージ %q は %q で始まるべき", c.src, e.Msg, c.want)
		}
		if e.Filename != "e.fc" || !e.Pos.IsValid() {
			t.Errorf("%q: 位置情報が不正: %+v", c.src, e)
		}
	}
}

// TestParseShapes は代表的な構文の木の形を確認する (Lower の差分テストとは独立した直接検査)。
func TestParseShapes(t *testing.T) {
	src := `// c1
public var a:int = 1, b = 2 options(address: 0x10);
function f(x:int):void options(bank:1) { if (x) a += 1; elsif (b) {} else b -= 1; }
use * from common;
use mem as m;
include macro("x.rb");
switch (a) { case 1, 2: break; default: continue; }
var p:int*[4](int, y:int) = -> void { };
`
	f, err := Parse([]byte(src), "s.fc")
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Comments) != 1 || f.Comments[0].Text != "// c1" {
		t.Errorf("comments: %+v", f.Comments)
	}
	if len(f.Stmts) != 7 {
		t.Fatalf("stmts: %d", len(f.Stmts))
	}
	vd := f.Stmts[0].(*VarDecl)
	if !vd.PublicPos.IsValid() || vd.Const || len(vd.Specs) != 2 || vd.Specs[1].Type != nil ||
		vd.Specs[1].Options.Get("address").(*IntLit).Value != 0x10 {
		t.Errorf("VarDecl: %+v", vd)
	}
	fd := f.Stmts[1].(*FuncDecl)
	if fd.Name.Name != "f" || len(fd.Params) != 1 || fd.Options.Get("bank") == nil || fd.Body == nil {
		t.Errorf("FuncDecl: %+v", fd)
	}
	ifs := fd.Body.Stmts[0].(*IfStmt)
	if ifs.Then.(*ExprStmt).X.(*AssignExpr).Op != AddEq {
		t.Errorf("then: %+v", ifs.Then)
	}
	elsif := ifs.Else.(*IfStmt)
	if !elsif.IsElsif || elsif.Else.(*ExprStmt).X.(*AssignExpr).Op != SubEq {
		t.Errorf("elsif: %+v", elsif)
	}
	u1 := f.Stmts[2].(*UseDecl)
	u2 := f.Stmts[3].(*UseDecl)
	if !u1.FromAll || u1.Module.Name != "common" || u2.FromAll || u2.As.Name != "m" {
		t.Errorf("use: %+v %+v", u1, u2)
	}
	inc := f.Stmts[4].(*IncludeDecl)
	if inc.Kind.Name != "macro" || inc.Path.Value != "x.rb" {
		t.Errorf("include: %+v", inc)
	}
	sw := f.Stmts[5].(*SwitchStmt)
	if len(sw.Cases) != 1 || len(sw.Cases[0].Values) != 2 || sw.Default == nil {
		t.Errorf("switch: %+v", sw)
	}
	// int*[4](int, y:int) → FuncType(ArrayType(PointerType(int)))
	pt := f.Stmts[6].(*VarDecl).Specs[0]
	ft := pt.Type.(*FuncType)
	at := ft.Result.(*ArrayType)
	if _, ok := at.Elem.(*PointerType); !ok || at.Len.(*IntLit).Value != 4 || len(ft.Params) != 2 ||
		ft.Params[0].Name != nil || ft.Params[1].Name.Name != "y" {
		t.Errorf("type: %+v", pt.Type)
	}
	if lam := pt.Init.(*LambdaExpr); lam.Body == nil || lam.Type.(*NamedType).Name.Name != "void" {
		t.Errorf("lambda: %+v", pt.Init)
	}
}

// TestVersionPragma: 先頭行の `#fc N` プラグマと、バージョンごとの受理範囲。
func TestVersionPragma(t *testing.T) {
	f, err := Parse([]byte("#fc 2\nvar a:int;\n"), "t.fc")
	if err != nil {
		t.Fatal(err)
	}
	if f.Version != Version2 || f.Pragma != "#fc 2" {
		t.Errorf("version=%d pragma=%q", f.Version, f.Pragma)
	}
	if got := f.Stmts[0].Pos(); got.Line != 2 || got.Col != 1 {
		t.Errorf("first statement at %v, want 2:1", got)
	}
	f, err = Parse([]byte("var a:int;\n"), "t.fc")
	if err != nil || f.Version != Version1 || f.Pragma != "" {
		t.Errorf("v1: err=%v version=%d pragma=%q", err, f.Version, f.Pragma)
	}

	bad := []struct{ src, msg string }{
		{"#fc 3\n", "invalid version pragma"},
		{"#fc\n", "invalid version pragma"},
		{"#fc 2 extra\n", "invalid version pragma"},
		{"var a:int;\n#fc 2\n", "invalid token"},
		{"#fc 2\npublic:\nvar a:int;\n", "labels are not allowed in fc 2"},
		{"#fc 2\nprivate:\nvar a:int;\n", "a label must be placed on"}, // v2 では private は予約語でない
		{"var a:int;\nfunction f():void { a |= 1; }\n", "`|=` requires fc 2"},
		{"var a:int;\nfunction f():void { a <<= 1; }\n", "`<<=` requires fc 2"},
		{"var a:int;\nfunction f():void { a = ~a; }\n", "`~` requires fc 2"},
		{"#fc 2\ninclude macro(\"x.rb\");\n", "include macro(...) is not allowed in fc 2"},
		{"const a = [1, 2,];\n", "trailing comma requires fc 2"},
		{"#fc 2\nvar a:int*;\n", "postfix types (int*, int[4], void(int)) are written prefix in fc 2"},
		{"#fc 2\nvar a:int[4];\n", "postfix types"},
		{"#fc 2\nvar a:void(int);\n", "postfix types"},
		{"#fc 2\nvar a:[4]int*;\n", "postfix types"},
		{"#fc 2\nvar x = <int>y;\n", "`<T>x` is written `x as T`"},
		{"var a:*int;\n", "prefix types (*int, [4]int, fn(int):void) require fc 2"},
		{"var a:[4]int;\n", "prefix types"},
		{"var a:fn(int):void;\n", "prefix types"},
		{"var x = y as int;\n", "`as` / `bitcast` require fc 2"},
		{"var x = bitcast<int>(y);\n", "`as` / `bitcast` require fc 2"},
		{"function f():void { g(1,); }\n", "trailing comma requires fc 2"},
		{"#fc 2\nconst a = [,];\n", "parse error"},
		{"struct P { x:int; }\n", "`struct` requires fc 2"},
		{"soa P:P[4];\n", "`soa` requires fc 2"},
		{"var p = P{1, 2};\n", "struct literals require fc 2"},
		{"var n = sizeof(int);\n", "`sizeof` requires fc 2"},
		{"var t:m.T;\n", "qualified type names (mod.T) require fc 2"},
		{"#fc 2\nstruct P { x:int }\n", "parse error"},
	}
	for _, b := range bad {
		_, err := Parse([]byte(b.src), "t.fc")
		if err == nil || !strings.Contains(err.Error(), b.msg) {
			t.Errorf("%q: got %v, want /%s/", b.src, err, b.msg)
		}
	}
	// v1 では従来どおり受理する
	for _, ok := range []string{"private:\nvar a:int;\n", "include macro(\"x.rb\");\n"} {
		if _, err := Parse([]byte(ok), "t.fc"); err != nil {
			t.Errorf("%q: %v", ok, err)
		}
	}
}
