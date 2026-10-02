package driver

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// テストの外部マクロのサーバー: テストのバイナリ自身を、この引数を付けて起動する (fc.toml の command)。
const testMacroServerArg = "--fc-test-macro-server"

func init() {
	for _, a := range os.Args[1:] {
		if a == testMacroServerArg {
			runTestMacroServer()
			os.Exit(0)
		}
	}
}

// runTestMacroServer は extmacro のプロトコルのサーバー (起動するたびに作業ディレクトリの starts.log に 1 行足す)。
func runTestMacroServer() {
	if f, err := os.OpenFile("starts.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o666); err == nil {
		fmt.Fprintln(f, "start")
		f.Close()
	}
	out := bufio.NewWriter(os.Stdout)
	send := func(v any) {
		b, _ := json.Marshal(v)
		out.Write(append(b, '\n'))
		out.Flush()
	}
	send(map[string]any{"macros": []string{"sq", "ramp", "rev", "greet", "fail", "filelen"}})
	sc := bufio.NewScanner(os.Stdin)
	for sc.Scan() {
		var req struct {
			ID    int               `json:"id"`
			Macro string            `json:"macro"`
			Args  []json.RawMessage `json:"args"`
		}
		if err := json.Unmarshal(sc.Bytes(), &req); err != nil {
			fmt.Fprintln(os.Stderr, "bad request:", err)
			return
		}
		resp := map[string]any{"id": req.ID}
		intArg := func(i int) int { var n int; json.Unmarshal(req.Args[i], &n); return n }
		switch req.Macro {
		case "sq": // @sq(n): n * n (u16)
			resp["result"] = map[string]any{"type": "u16", "int": intArg(0) * intArg(0)}
		case "ramp": // @ramp(n): [0, -1, -2, ...] (i8)
			d := []int{}
			for i := 0; i < intArg(0); i++ {
				d = append(d, -i)
			}
			resp["result"] = map[string]any{"type": "i8", "data": d}
		case "rev": // @rev(bytes): 逆順のバイト列
			var b struct{ Bytes string }
			json.Unmarshal(req.Args[0], &b)
			raw, _ := base64.StdEncoding.DecodeString(b.Bytes)
			for i, j := 0, len(raw)-1; i < j; i, j = i+1, j-1 {
				raw[i], raw[j] = raw[j], raw[i]
			}
			resp["result"] = map[string]any{"bytes": base64.StdEncoding.EncodeToString(raw)}
		case "greet": // @greet("x"): "hi x"
			var s string
			json.Unmarshal(req.Args[0], &s)
			resp["result"] = map[string]any{"string": "hi " + s}
		case "filelen": // @filelen("f"): f の長さ (deps に f)
			var s string
			json.Unmarshal(req.Args[0], &s)
			b, err := os.ReadFile(s)
			if err != nil {
				resp["error"] = err.Error()
				break
			}
			resp["result"] = map[string]any{"int": len(b)}
			resp["deps"] = []string{s}
		default:
			resp["error"] = "no such macro: " + req.Macro
		}
		send(resp)
	}
}

func testMacroToml(extra string) string {
	exe, _ := json.Marshal(os.Args[0])
	return fmt.Sprintf("[macro_server.t]\ncommand = [%s, %q]\nmacros = [\"sq\", \"ramp\", \"rev\", \"greet\", \"fail\", \"filelen\"]\n%s", exe, testMacroServerArg, extra)
}

// TestExternalMacros: fc.toml の [macro_server.*] の外部コマンドの定数マクロ (internal/extmacro)。整数 (型付き)・整数の配列・
// バイト列・文字列の結果と、マクロのエラー・名前の衝突。
func TestExternalMacros(t *testing.T) {
	t.Parallel()
	out, err := buildBothLevels(t, map[string]string{
		"fc.toml": testMacroToml(""),
		"t.fc": `#fc 4
use console;
const N = @sq(20);
const R = @ramp(4);
const D:[3]u8 = [1, 2, 3];
const B = @rev(D);
const B2 = @rev([4, 5]);
const G = @greet("fc");
function main():void
{
	var s:i16 = 0;
	for (var x in R) { s += x; }
	@printf("{} {} {} {} {} {} {} {}\n", N, @sizeof(N), s, @len(R), B[0], B[2], B2[0], G);
	console.exit(0);
}
`})
	if want := "400 2 -6 4 3 1 5 hi fc\n"; err != nil || out != want {
		t.Errorf("got %q, %v, want %q", out, err, want)
	}
	for _, c := range []struct{ src, toml, msg string }{
		{"const X = @fail(1);", "", "fail"},
		{"const X = @nosuch(1);", "", "@nosuch not found"},
		{"var v:u8; function f():void { const X = @sq(v); }", "", "argument 1 of @sq"},
		{"const X = @sq(1);", "", ""},
	} {
		files := map[string]string{"fc.toml": testMacroToml(c.toml), "t.fc": "#fc 4\n" + c.src + "\nfunction main():void {}\n"}
		_, err := buildFiles(t, files)
		if c.msg == "" {
			if err != nil {
				t.Errorf("%s: %v", c.src, err)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), c.msg) {
			t.Errorf("%s: got %v, want /%s/", c.src, err, c.msg)
		}
	}
	// 組み込みと同じ名前
	toml := strings.Replace(testMacroToml(""), `"sq"`, `"lz4"`, 1)
	if _, err := buildFiles(t, map[string]string{"fc.toml": toml, "t.fc": "#fc 4\nfunction main():void {}\n"}); err == nil || !strings.Contains(err.Error(), "same name as a built-in") {
		t.Errorf("built-in name: got %v", err)
	}
}

// TestExternalMacroCache: inputs を書くと結果をディスクに置き、次のビルドではマクロのコマンドを起動しない。マクロが読んだ
// ファイル (deps) が変わったら呼び直す。マクロを使わないビルドでは起動しない。
func TestExternalMacroCache(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	write := func(name, s string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(s), 0o666); err != nil {
			t.Fatal(err)
		}
	}
	write("fc.toml", testMacroToml(`inputs = ["fc.toml"]`+"\n"))
	write("data.txt", "abc")
	write("t.fc", "#fc 4\nuse console;\nconst L = @filelen(\"data.txt\");\nfunction main():void { console.exit(L); }\n")
	starts := func() int {
		b, _ := os.ReadFile(filepath.Join(dir, "starts.log"))
		return strings.Count(string(b), "start")
	}
	build := func() int {
		t.Helper()
		code, err := NewCompiler(absRepoRoot).Build("t.fc", &BuildOptions{Dir: dir, Target: "emu", Run: true, Out: filepath.Join(dir, "a.bin")})
		if err != nil {
			t.Fatal(err)
		}
		return code
	}
	if code := build(); code != 3 || starts() != 1 {
		t.Fatalf("1 回目: exit %d, 起動 %d", code, starts())
	}
	if code := build(); code != 3 || starts() != 1 {
		t.Errorf("2 回目はキャッシュ (起動しない): exit %d, 起動 %d", code, starts())
	}
	write("data.txt", "abcde")
	if code := build(); code != 5 || starts() != 2 {
		t.Errorf("deps が変わったら呼び直す: exit %d, 起動 %d", code, starts())
	}
	write("t.fc", "#fc 4\nuse console;\nfunction main():void { console.exit(0); }\n")
	if code := build(); code != 0 || starts() != 2 {
		t.Errorf("マクロを使わないビルドでは起動しない: exit %d, 起動 %d", code, starts())
	}
}
