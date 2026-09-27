package driver

// 型の規則の総当たり (TestTypeRuleMatrix)。型の種類 × 使う場所 の小さなプログラムを Check に通し、「ok」か「エラーの文言」を
// testdata/golden/typerules.txt と比べる (`-update` で書き直す)。
//
// 「通ってはいけないのに通る」(soa の名前を値の型に使える: 42f5734、ポインタの算術で const が外れる、など) は fuzz では
// 見つからず、人手の調査だけが頼りだった。表にしておけば、検査が緩む・厳しくなるの変化が golden の差分で見え、今の
// 表で怪しいセル (ok なのに意味が無い・危ない組み合わせ) を見直せる。

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// trTypes は型の種類 (名前と、その型の値を作る式。"" なら値を作れない)。
var trTypes = []struct{ name, zero string }{
	{"u8", "0"},
	{"i16", "0"},
	{"bool", "false"},
	{"E", ".A"},
	{"S", "S{1, 2}"},
	{"Es", ""},
	{"[4]u8", ""},
	{"[]u8", ""},
	{"[:u16]u8", ""},
	{"*u8", "null"},
	{"*const u8", "null"},
	{"*S", "null"},
	{"*Es", ""},
	{"fn(u8):u8", ""},
	{"void", ""},
}

// trContexts は使う場所 (%T を型、%Z をその型の値に置き換える。値の要る場所は値の無い型では飛ばす)。
var trContexts = []struct{ name, src string }{
	{"大域変数", "var v:%T;"},
	{"ローカル変数", "function f():void { var v:%T; }"},
	{"引数", "function f(p:%T):void { }"},
	{"戻り値", "function f(p:%T):%T { return p; }"},
	{"struct のフィールド", "struct Q { f:%T; }"},
	{"配列の要素", "var v:[2]%T;"},
	{"ポインタ", "var v:*%T;"},
	{"u8 からの as", "function f(x:u8):void { var v = x as %T; }"},
	{"u8 の代入", "function f(x:u8, v:%T):void { v = x; }"},
	{"加算", "function f(a:%T, b:%T):void { var c = a + b; }"},
	{"==", "function f(a:%T, b:%T):bool { return a == b; }"},
	{"<", "function f(a:%T, b:%T):bool { return a < b; }"},
	{"条件", "function f(a:%T):void { if (a) { } }"},
	{"for-each", "function f(a:%T):void { for (var x in a) { } }"},
	{"@sizeof", "const N = @sizeof(%T);"},
	{"printf", "function f(a:%T):void { printf(a); }"},
	{"値の初期化", "function f():void { var v:%T = %Z; }"},
}

var trPos = regexp.MustCompile(`^[^:]*\.fc:\d+:\d+: `)

func TestTypeRuleMatrix(t *testing.T) {
	t.Parallel()
	pre := "#fc 3\nuse * from stdio;\nstruct S { a:u8; b:u8; }\nenum E { A, B }\nsoa Es:[4]S;\n"
	var b strings.Builder
	b.WriteString("# 型の規則の総当たり (TestTypeRuleMatrix。`go test ./internal/driver -run TestTypeRuleMatrix -update` で書き直す)\n")
	for _, c := range trContexts {
		fmt.Fprintf(&b, "\n## %s: %s\n", c.name, c.src)
		for _, ty := range trTypes {
			if strings.Contains(c.src, "%Z") && ty.zero == "" {
				continue
			}
			src := strings.ReplaceAll(strings.ReplaceAll(c.src, "%T", ty.name), "%Z", ty.zero)
			res := trCheck(t, pre+src+"\nfunction main():void { exit(0); }\n")
			fmt.Fprintf(&b, "%-10s %s\n", ty.name, res)
		}
	}
	got := b.String()
	path := filepath.Join(absGoldenRoot, "typerules.txt")
	if *update {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (`-update` で作る)", err)
	}
	compareText(t, "typerules.txt", got, strings.ReplaceAll(string(want), "\r\n", "\n"))
}

// trCheck は src を Check に通した結果 ("ok" か、最初のエラーの文言。位置は外す)。
func trCheck(t *testing.T, src string) (res string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "t.fc"), []byte(src), 0o666); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if r := recover(); r != nil {
			res = "PANIC " + firstLine(fmt.Sprint(r))
		}
	}()
	_, err := NewCompiler(absRepoRoot).Check("t.fc", &CheckOptions{Dir: dir})
	if err == nil {
		return "ok"
	}
	msg := firstLine(err.Error())
	msg = trPos.ReplaceAllString(msg, "")
	msg = strings.TrimSuffix(msg, " (and 1 more error(s))")
	if len(msg) > 110 {
		msg = msg[:110] + "…"
	}
	return "E " + msg
}
