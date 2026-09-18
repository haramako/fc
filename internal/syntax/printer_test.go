package syntax

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestFormatStyle はフォーマッタの正規形をピン留めする。
func TestFormatStyle(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"decls",
			"public   var a:int=1,b:int ;\nconst   c = 2;\n",
			"public var a:int = 1, b:int;\nconst c = 2;\n"},
		{"function brace on next line",
			"function f( a:int, b:*int ):void options(fastcall:true){ return ; }\nfunction g():void;\n",
			"function f(a:int, b:*int):void options(fastcall: true)\n{\n\treturn;\n}\nfunction g():void;\n"},
		{"control statements",
			"function f():void{ if(a){x=1;}elsif(b){x=2;}else{x=3;} while(i<10){i+=1;} loop{break;} for(i=0;i<10;i++){continue;} }\n",
			"function f():void\n{\n\tif (a) {\n\t\tx = 1;\n\t} elsif (b) {\n\t\tx = 2;\n\t} else {\n\t\tx = 3;\n\t}\n\twhile (i < 10) {\n\t\ti += 1;\n\t}\n\tloop {\n\t\tbreak;\n\t}\n\tfor (i = 0; i < 10; i++) {\n\t\tcontinue;\n\t}\n}\n"},
		{"if without block",
			"function f():void{ if(a) x=1; else x=2; }\n",
			"function f():void\n{\n\tif (a)\n\t\tx = 1;\n\telse\n\t\tx = 2;\n}\n"},
		{"switch",
			"function f():void{ switch(x){ case 1,2: a(); b(); case 3: c(); default: d(); } }\n",
			"function f():void\n{\n\tswitch (x) {\n\tcase 1, 2:\n\t\ta();\n\t\tb();\n\tcase 3:\n\t\tc();\n\tdefault:\n\t\td();\n\t}\n}\n"},
		{"expressions keep parens and token order",
			"var x = (a+b)*c - -d / e[1] . f as uint8 & !g;\n",
			"var x = (a + b) * c - -d / e[1].f as uint8 & !g;\n"},
		{"tokens that would merge get a space",
			"var x = a < b as int; var y = & &p;\n",
			"var x = a < b as int;\nvar y = & &p;\n"},
		{"use include options",
			"options(bank:-1);\nuse * from nes;\nuse  mem  as m;\ninclude ( \"y.chr\" );\ninclude(\"x.asm\") options(a:1);\n",
			"options(bank: -1);\nuse * from nes;\nuse mem as m;\ninclude(\"y.chr\");\ninclude(\"x.asm\") options(a: 1);\n"},
		{"lambda and call block",
			"const add2 = ->fn(a:int):int{ return a+2; };\nfunction f():void{ g(1){ h(); }; }\n",
			"const add2 = ->fn(a:int):int {\n\treturn a + 2;\n};\nfunction f():void\n{\n\tg(1) {\n\t\th();\n\t};\n}\n"},
		{"array keeps source line breaks",
			"const t = [1,2,3,\n  4,5,6,\n  7,8,9];\nconst u = [\n1,2,\n3,4\n];\nconst v = [1,\n2];\n",
			"const t = [1, 2, 3,\n\t4, 5, 6,\n\t7, 8, 9];\nconst u = [\n\t1, 2,\n\t3, 4\n];\nconst v = [1,\n\t2];\n"},
		{"blank lines collapse, none after brace",
			"var a:int;\n\n\n\nvar b:int;\nfunction f():void\n{\n\n\tvar c:int;\n\n\n\tvar d:int;\n\n}\n",
			"var a:int;\n\nvar b:int;\nfunction f():void\n{\n\tvar c:int;\n\n\tvar d:int;\n}\n"},
		{"comments: leading, trailing, block, end of block, end of file",
			"// head\n\n/* block\n   comment */\nvar a:int; // trailing a\n// before b\nvar b:int;\nfunction f():void\n{\n\tx(); // t\n\t// last\n}\n// eof\n",
			"// head\n\n/* block\n   comment */\nvar a:int; // trailing a\n// before b\nvar b:int;\nfunction f():void\n{\n\tx(); // t\n\t// last\n}\n// eof\n"},
		{"comment inside expression list",
			"const t = [\n\t1, 2, // row 1\n\t3, 4 // row 2\n];\n",
			"const t = [\n\t1, 2, // row 1\n\t3, 4 // row 2\n];\n"},
		{"empty file with comment", "// only\n", "// only\n"},
		{"fc 2 pragma", "#fc 2\n\n// c\nvar a:int;\n", "#fc 2\n\n// c\nvar a:int;\n"},
		{"fc 2 pragma normalized", "#fc   2\nvar a:int;\n", "#fc 2\nvar a:int;\n"},
		{"fc 2 public use", "#fc 2\npublic  use  m;\npublic use * from n;\n", "#fc 2\npublic use m;\npublic use * from n;\n"},
		{"fc 2 c-style for and incdec",
			"#fc 2\nfunction f():void { for(var i:uint8=0;i<10;i++){ x++; } for(;;){ --y; } for(i=0; i<3; i+=1){} }\n",
			"#fc 2\nfunction f():void\n{\n\tfor (var i:uint8 = 0; i < 10; i++) {\n\t\tx++;\n\t}\n\tfor (;;) {\n\t\t--y;\n\t}\n\tfor (i = 0; i < 3; i += 1) {}\n}\n"},
		{"fc 2 trailing comma kept only when multi-line",
			"#fc 2\nconst a = [1, 2,];\nconst b = [\n\t1, 2,\n\t3,\n];\nfunction f():void { g(1,); g(\n\t1,\n\t2,\n); }\n",
			"#fc 2\nconst a = [1, 2];\nconst b = [\n\t1, 2,\n\t3,\n];\nfunction f():void\n{\n\tg(1);\n\tg(\n\t\t1,\n\t\t2,\n\t);\n}\n"},
		{"fc 2 prefix types and casts",
			"#fc 2\nvar a:[4]*int;\nvar p:*[4]int;\nvar t:[]fn(int, *int):void;\nfunction f(x:sint8):fn(a:int):int { var y = x as int * 2; var q = bitcast<*uint16>(p); return f; }\n",
			"#fc 2\nvar a:[4]*int;\nvar p:*[4]int;\nvar t:[]fn(int, *int):void;\nfunction f(x:sint8):fn(a:int):int\n{\n\tvar y = x as int * 2;\n\tvar q = bitcast<*uint16>(p);\n\treturn f;\n}\n"},
		{"fc 2 struct / soa / sizeof / struct literals",
			"#fc 2\npublic struct Point { x:int;  y:int; // c\n next:*Point; }\nstruct E {}\nsoa Points:[4]Point;\npublic soa const Cs:[2]m.Point = [{1,2}, Point{x:3, y:4}] options(bank:1);\nfunction f():void { var p = Point{x: 1, y: 2}; var q:[2]Point = [\n\t{1, 2},\n\t{3, 4},\n]; var n = sizeof(Point) + sizeof([4]*m.T); }\n",
			"#fc 2\npublic struct Point {\n\tx:int;\n\ty:int; // c\n\tnext:*Point;\n}\nstruct E {\n}\nsoa Points:[4]Point;\npublic soa const Cs:[2]m.Point = [{1, 2}, Point{x: 3, y: 4}] options(bank: 1);\nfunction f():void\n{\n\tvar p = Point{x: 1, y: 2};\n\tvar q:[2]Point = [\n\t\t{1, 2},\n\t\t{3, 4},\n\t];\n\tvar n = sizeof(Point) + sizeof([4]*m.T);\n}\n"},
		{"fc 2 compound assignment, ~, empty case, private as identifier",
			"#fc 2\nvar private:int;\nfunction f():void { a|=1; a<<=2; a  >>= 1; a &= ~b; a *= 2; a /= 3; a %= 4; a ^= 5; switch (a) { case 0: case 1: x(); default: } }\n",
			"#fc 2\nvar private:int;\nfunction f():void\n{\n\ta |= 1;\n\ta <<= 2;\n\ta >>= 1;\n\ta &= ~b;\n\ta *= 2;\n\ta /= 3;\n\ta %= 4;\n\ta ^= 5;\n\tswitch (a) {\n\tcase 0:\n\tcase 1:\n\t\tx();\n\tdefault:\n\t}\n}\n"},
		{"fc 2 true / false / null / *void",
			"#fc 2\nvar p:*void;\nfunction f():bool { p = null; if (p == null) { return true; } return false; }\n",
			"#fc 2\nvar p:*void;\nfunction f():bool\n{\n\tp = null;\n\tif (p == null) {\n\t\treturn true;\n\t}\n\treturn false;\n}\n"},
		{"fc 2 selective use", "#fc 2\nuse a,b  from m;\npublic use c from n;\n", "#fc 2\nuse a, b from m;\npublic use c from n;\n"},
		{"fc 2 loop and labels",
			"#fc 2\nfunction f():void { outer:loop{ s:  switch(x){ case 1: break outer; case 2: continue  outer; case 3: break; } } }\n",
			"#fc 2\nfunction f():void\n{\n\touter: loop {\n\t\ts: switch (x) {\n\t\tcase 1:\n\t\t\tbreak outer;\n\t\tcase 2:\n\t\t\tcontinue outer;\n\t\tcase 3:\n\t\t\tbreak;\n\t\t}\n\t}\n}\n"},
		{"empty blocks",
			"function f():void {}\nfunction g():void\n{\n\t// note\n}\nfunction h():void { if (a) {} else { } loop{} }\n",
			"function f():void {}\nfunction g():void\n{\n\t// note\n}\nfunction h():void\n{\n\tif (a) {} else {}\n\tloop {}\n}\n"},
		{"crlf input", "var a:int;\r\n// c\r\nvar b:int;\r\n", "var a:int;\n// c\nvar b:int;\n"},
		{"empty statement and nested block", "function f():void{ ; { a(); } }\n", "function f():void\n{\n\t;\n\t{\n\t\ta();\n\t}\n}\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Format([]byte(tt.in), "t.fc")
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Errorf("format mismatch\n--- want\n%s--- got\n%s", tt.want, got)
			}
			checkFormatInvariants(t, "t.fc", []byte(tt.in), got)
		})
	}
}

// TestFormatCorpus はリポジトリ内の全 .fc について冪等性・往復・コメント保持を確認する。
func TestFormatCorpus(t *testing.T) {
	root := repoRoot(t)
	var files []string
	for _, dir := range []string{"test", "fclib", "examples"} {
		filepath.WalkDir(filepath.Join(root, dir), func(path string, d os.DirEntry, err error) error {
			if err == nil && !d.IsDir() && strings.HasSuffix(path, ".fc") && filepath.Base(path) != "errors.fc" {
				files = append(files, path)
			}
			return nil
		})
	}
	if len(files) < 30 {
		t.Fatalf("コーパスが少なすぎる: %d", len(files))
	}
	for _, path := range files {
		rel, _ := filepath.Rel(root, path)
		t.Run(filepath.ToSlash(rel), func(t *testing.T) {
			src, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			src = bytes.ReplaceAll(src, []byte("\r\n"), []byte("\n"))
			got, err := Format(src, path)
			if err != nil {
				t.Fatal(err)
			}
			checkFormatInvariants(t, path, src, got)
		})
	}
}

// checkFormatInvariants: fmt(fmt(x)) == fmt(x)、parse(fmt(x)) ≡ parse(x) (位置を除く)、コメント全保持。
func checkFormatInvariants(t *testing.T, filename string, src, got []byte) {
	t.Helper()
	again, err := Format(got, filename)
	if err != nil {
		t.Fatalf("整形結果がパースできない: %v\n%s", err, got)
	}
	if !bytes.Equal(again, got) {
		t.Errorf("冪等でない\n--- 1 回目\n%s--- 2 回目\n%s", got, again)
	}
	f1, _ := Parse(bytes.ReplaceAll(src, []byte("\r\n"), []byte("\n")), filename)
	f2, _ := Parse(got, filename)
	if len(f1.Comments) != len(f2.Comments) {
		t.Fatalf("コメント数が変わった: %d → %d", len(f1.Comments), len(f2.Comments))
	}
	for i := range f1.Comments {
		if f1.Comments[i].Text != f2.Comments[i].Text {
			t.Errorf("コメント %d が変わった: %q → %q", i, f1.Comments[i].Text, f2.Comments[i].Text)
		}
	}
	s1, s2 := stripPos(f1.Stmts), stripPos(f2.Stmts)
	if !reflect.DeepEqual(s1, s2) {
		t.Errorf("構文木が変わった\n%s", got)
	}
}

var posType = reflect.TypeOf(Pos{})

// stripPos は構文木を複製して位置情報をゼロにする (構造比較用)。
func stripPos(v any) any {
	return stripPosValue(reflect.ValueOf(v)).Interface()
}

func stripPosValue(v reflect.Value) reflect.Value {
	switch v.Kind() {
	case reflect.Interface:
		if v.IsNil() {
			return v
		}
		r := reflect.New(v.Type()).Elem()
		r.Set(stripPosValue(v.Elem()))
		return r
	case reflect.Ptr:
		if v.IsNil() {
			return v
		}
		r := reflect.New(v.Type().Elem())
		r.Elem().Set(stripPosValue(v.Elem()))
		return r
	case reflect.Slice:
		if v.IsNil() {
			return v
		}
		r := reflect.MakeSlice(v.Type(), v.Len(), v.Len())
		for i := 0; i < v.Len(); i++ {
			r.Index(i).Set(stripPosValue(v.Index(i)))
		}
		return r
	case reflect.Struct:
		if v.Type() == posType {
			return reflect.Zero(posType)
		}
		r := reflect.New(v.Type()).Elem()
		for i := 0; i < v.NumField(); i++ {
			if v.Type().Field(i).IsExported() {
				r.Field(i).Set(stripPosValue(v.Field(i)))
			}
		}
		return r
	default:
		return v
	}
}

func repoRoot(t *testing.T) string {
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod が見つからない")
		}
		dir = parent
	}
}
