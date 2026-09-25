package driver

// fc 3 の @log のテスト (doc/v3_plan.md §9)。表示は emu の実行 (fcc run -g) で確かめる。

import (
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
)

// logBuild は files を -g (Debug) でビルドして走らせ、(ROM、printf の出力、@log の出力、結果) を返す。
func logBuild(t *testing.T, files map[string]string, level int, debug, every bool) (rom []byte, stdout, logOut string, res *Result, err error) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r) // emu のソフトウェアスタックのあふれなど (fuzz のプログラムの問題)
		}
	}()
	dir := t.TempDir()
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o666); err != nil {
			t.Fatal(err)
		}
	}
	var out, logs strings.Builder
	res, err = NewCompiler(absRepoRoot).BuildContext(t.Context(), "t.fc", &BuildOptions{Dir: dir, BuildDir: filepath.Join(dir, "b"), Out: filepath.Join(dir, "a.bin"),
		Run: true, Stdout: &out, LogOut: &logs, MaxCycles: rpMaxCycles, OptimizeLevel: level, Debug: debug, LogEveryStatement: every})
	if err != nil {
		return nil, "", "", nil, err
	}
	if rom, err = os.ReadFile(filepath.Join(dir, "a.bin")); err != nil {
		t.Fatal(err)
	}
	return rom, out.String(), logs.String(), res, nil
}

// TestLogBasics: 書式 ({} / {0} / {:02x} / {:b} / {:X} / {{ }})、enum の名前、struct のフィールド・定数添字、ローカル・引数、
// インライン展開・ループ展開の写しごとの地点。-O 0 と -O 2 で同じ表示、@log があっても ROM は同じ。
func TestLogBasics(t *testing.T) {
	t.Parallel()
	src := `#fc 3
use * from stdio;

enum State { Stand, Jump, Die }
struct P { x:u8; y:i16; }
var hp:u8;
var st:State;
var pl:P;
const TAB = [10, 20, 30];

function f(a:u8, b:i8):u8
{
	var c = a + 1;
	@log("f: a={} b={} c={:02x}", a, b, c);
	return c + (b as u8);
}

function main():void
{
	hp = 100;
	st = .Jump;
	pl.x = 3;
	pl.y = -5;
	@log("hp={} st={} x={} y={} tab1={} {{ok}} {:d}", hp, st, pl.x, pl.y, TAB[1], st);
	for (var i = 0; i < 3; i += 1) {
		@log("i={0} i={0:b} hp={1:X} {1:5}|{2:c}", i, hp, 65);
		hp += f(i, -1);
	}
	printf(hp, "\n");
	exit(0);
}
`
	want := "hp=100 st=Jump x=3 y=-5 tab1=20 {ok} 1\n" +
		"i=0 i=0 hp=64   100|A\n" +
		"f: a=0 b=-1 c=01\n" +
		"i=1 i=1 hp=64   100|A\n" +
		"f: a=1 b=-1 c=02\n" +
		"i=2 i=10 hp=65   101|A\n" +
		"f: a=2 b=-1 c=03\n"
	var roms [][]byte
	for _, level := range []int{-1, 0} {
		rom, out, logs, _, err := logBuild(t, map[string]string{"t.fc": src}, level, true, false)
		if err != nil {
			t.Fatal(err)
		}
		if out != "103\n" || logs != want {
			t.Errorf("-O %d: out %q\nlogs %q\nwant %q", level, out, logs, want)
		}
		plain, _, _, _, err := logBuild(t, map[string]string{"t.fc": src}, level, false, false)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(rom, plain) {
			t.Errorf("-O %d: @log (-g) で ROM が変わった", level)
		}
		roms = append(roms, rom)
	}
}

// TestLogErrors: 書式と引数の検査 (-g でなくても)。
func TestLogErrors(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ src, msg string }{
		{`var a:u8; function main():void { @log("{} {}", a); }`, "there is no argument 1"},
		{`var a:u8; function main():void { @log("x", a); }`, "argument 0 (a) is not used"},
		{`var a:u8; function main():void { @log("{:q}", a); }`, "unknown format `q`"},
		{`var a:u16; function main():void { @log("{:c}", a); }`, "`c` shows a 1-byte integer"},
		{`var a:u8; function main():void { @log("{", a); }`, "unclosed `{`"},
		{`var a:u8; function main():void { @log("}", a); }`, "unmatched `}`"},
		{`var a:u8; function f():u8 { return 1; } function main():void { @log("{}", f()); }`, "@log can show variables"},
		{`var a:[4]u8; function main():void { var i = 1; @log("{}", a[i]); }`, "constant index"},
		{`var a:[4]u8; function main():void { @log("{}", a[4]); }`, "out of range"},
		{`struct S { x:u8; } var s:S; function main():void { @log("{}", s); }`, "cannot show a value of type"},
		{`@log("x");`, "inside a function"},
	} {
		_, err := buildFiles(t, map[string]string{"t.fc": "#fc 3\n" + c.src + "\n"})
		if err == nil || !strings.Contains(err.Error(), c.msg) {
			t.Errorf("%s: got %v, want %q", c.src, err, c.msg)
		}
	}
}

// TestLogWarnings: 分岐の最後の @log、値の取れない引数 (「?」)。
func TestLogWarnings(t *testing.T) {
	t.Parallel()
	src := `#fc 3
use * from stdio;
var g:u8;
function main():void
{
	var k:u8 = 5;
	if (g == 0) {
		g = 1;
		@log("then {}", g);
	}
	g += k;
	var d = g + 1;
	@log("d? {}", k);
	printf(d, "\n");
	exit(0);
}
`
	_, _, logs, res, err := logBuild(t, map[string]string{"t.fc": src}, 0, true, false)
	if err != nil {
		t.Fatal(err)
	}
	var msgs []string
	for _, w := range res.Warnings {
		msgs = append(msgs, w.Msg)
	}
	all := strings.Join(msgs, "\n")
	if !strings.Contains(all, "@log at the end of an if / else branch") {
		t.Errorf("分岐の最後の警告が無い: %s", all)
	}
	if !strings.Contains(logs, "then 1\n") {
		t.Errorf("logs %q", logs)
	}
}

// TestLogZeroCost: fuzz のプログラムの全部の文の前に、見えている変数を全部出す @log を置いても、ROM とプログラムの出力が
// 変わらない (-O 0 / -O 2)。@log の値は、両方のレベルで取れた値を比べる。食い違いは種ごとにはログに出すだけ (丸ごと
// 畳まれたループの写しが同じ命令に集まる形などで、-O 2 の地点が元の地点からずれることがある。doc/v3_plan.md §9) だが、
// 食い違った種が比べた種の logValueDiffLimit を超えたら失敗 (最適化のパスの変更で注釈の扱いが崩れたことに気づくため。
// 2026-09-25 の時点で 464 個中 1 個)。
func TestLogZeroCost(t *testing.T) {
	t.Parallel()
	var compared, differed atomic.Int32
	t.Cleanup(func() { // 並列の子のテストが全部終わった後
		n, d := compared.Load(), differed.Load()
		if float64(d) > float64(n)*logValueDiffLimit {
			t.Errorf("@log の値が -O 0 と -O 2 で食い違った種が多すぎる: %d / %d (上限 %.0f%%)。最適化のパスの変更で注釈の引き継ぎ・値の印 (LogStale / LogNoValue) が崩れていないか (doc/development_notes.md)", d, n, logValueDiffLimit*100)
		}
	})
	n := 20
	if *randN > 30 {
		n = *randN / 10
	}
	base := *randSeed
	if base == 0 {
		base = 1
	}
	for k := 0; k < n; k++ {
		seed := base + int64(k)
		t.Run(fmt.Sprintf("seed%d", seed), func(t *testing.T) {
			t.Parallel()
			g := &rpGen{r: rand.New(rand.NewSource(seed))}
			g.genProgram()
			files := g.sources()
			var logsByLevel []string
			for _, level := range []int{-1, 0} {
				plain, out, _, _, err := logBuild(t, files, level, false, false)
				if err != nil {
					t.Skipf("生成したプログラムがビルド・実行できない (seed %d): %v", seed, err)
				}
				rom, outLog, logs, _, err := logBuild(t, files, level, true, true)
				if err != nil {
					t.Fatalf("-O %d: @log 入りのビルドが失敗 (seed %d): %v\n%s", level, seed, err, g.allSource())
				}
				if !bytes.Equal(rom, plain) {
					t.Fatalf("-O %d: @log で ROM が変わった (seed %d)\n%s", level, seed, g.allSource())
				}
				if out != outLog {
					t.Fatalf("-O %d: @log で出力が変わった (seed %d)\n%s\n%s", level, seed, out, outLog)
				}
				logsByLevel = append(logsByLevel, logs)
			}
			bad, diffs := compareLogValues(logsByLevel[0], logsByLevel[1])
			compared.Add(1)
			if bad != "" {
				differed.Add(1)
				t.Logf("-O 0 と -O 2 で @log の値が違う (seed %d): %s", seed, bad)
			}
			if diffs > 0 {
				t.Logf("実行回数が -O 0 と -O 2 で違う地点: %d (最適化で地点が動いたもの。値は比べない)", diffs)
			}
		})
	}
}

// logValueDiffLimit は TestLogZeroCost の、@log の値が食い違ってよい種の割合の上限。
const logValueDiffLimit = 0.01

var reLogField = regexp.MustCompile(` (\S+)=(\S+)`)

// compareLogValues は -O 0 / -O 2 の @log の出力 (文ごとの `file:line#id a=1 b=?`) を地点ごとに比べる。-O 2 は地点が
// 最適化で動く (回転したループの条件は 1 回ずれる、丸ごと畳まれたループの写しは 1 つの命令に集まって順番が崩れる) ので、
// 実行回数が同じ地点だけ、両方で取れた値 (「?」でなく、番地がレベルで違うポインタ `$...` でない) の組を多重集合として
// 比べる (順番は比べない)。回数の違いの数も返す。
func compareLogValues(o0, o2 string) (bad string, countDiffs int) {
	parse := func(s string) map[string][]map[string]string {
		r := map[string][]map[string]string{}
		for _, line := range strings.Split(strings.TrimSpace(s), "\n") {
			if line == "" {
				continue
			}
			loc, rest, _ := strings.Cut(line, " ")
			m := map[string]string{}
			for _, f := range reLogField.FindAllStringSubmatch(" "+rest, -1) {
				m[f[1]] = f[2]
			}
			r[loc] = append(r[loc], m)
		}
		return r
	}
	a, b := parse(o0), parse(o2)
	// key は両方で取れた変数の値の組 (変数の名前順)
	key := func(x, y map[string]string) string {
		var names []string
		for name, v := range x {
			if w, ok := y[name]; ok && v != "?" && w != "?" && !strings.HasPrefix(v, "$") {
				names = append(names, name)
			}
		}
		sort.Strings(names)
		var s strings.Builder
		for _, n := range names {
			s.WriteString(n + "=" + x[n] + " ")
		}
		return s.String()
	}
	for loc, as := range a {
		bs := b[loc]
		if len(as) != len(bs) {
			countDiffs++
			continue
		}
		count := map[string]int{}
		for i := range as {
			count[key(as[i], bs[i])]++
			count[key(bs[i], as[i])]--
		}
		for k, n := range count {
			if n != 0 {
				return fmt.Sprintf("%s: 値の組 %q の回数が -O 0 と -O 2 で %d 違う", loc, k, n), countDiffs
			}
		}
	}
	return "", countDiffs
}
