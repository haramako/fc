package driver

// fc 3 の @log のテスト (Agent/discussions/2026-09-20-v3-plan.md §9)。表示は emu の実行 (fcc run -g) で確かめる。

import (
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/haramako/fc/internal/fclog"
)

// logBuild は files を -g (Debug) でビルドして走らせ、(ROM、printf の出力、@log の出力、結果) を返す。
func logBuild(t *testing.T, files map[string]string, level int, debug, every bool) (rom []byte, stdout, logOut string, res *Result, err error) {
	t.Helper()
	r := testBuild(t, buildSpec{Files: files, Run: true, Level: level, MaxCycles: rpMaxCycles, Debug: debug, LogEvery: every, Recover: true})
	if r.Err != nil {
		return nil, "", "", nil, r.Err
	}
	return r.ROM(t), r.Stdout, r.LogOut, r.Res, nil
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

// TestLogLoopEntry: ループの前の @log (ループの先頭のラベルの注釈) は入るときだけ出る。呼び出しの直後 (直前の命令が jsr の
// 合流点: emu は rts の後を jsr の後とみなす)、ループ展開 (写しの先頭のラベルに注釈を写さない)、空の無限ループを
// 回るジャンプの連鎖 (飛び先のラベルの注釈を拾わない)。Mesen での同じ確認は internal/nes の TestMesenLog。
func TestLogLoopEntry(t *testing.T) {
	t.Parallel()
	src := `#fc 3
use * from stdio;
var g:u8;
function f():void @(noinline) { g += 1; }
function main():void
{
	var i:u8 = 0;
	f();
	@log("before g={}", g);
	while (i < 3) { i += 1; g += i; }
	@log("after g={}", g);
	f();
	@log("before2 g={}", g);
	while (true) {
		g += 1;
		if (g > 10) { break; }
	}
	@log("end g={}", g);
	exit(0);
}
`
	want := "before g=1\nafter g=7\nbefore2 g=8\nend g=11\n"
	for _, level := range []int{-1, 1, 2} {
		_, _, logs, _, err := logBuild(t, map[string]string{"t.fc": src}, level, true, false)
		if err != nil {
			t.Fatal(err)
		}
		if logs != want {
			t.Errorf("-O %d: logs %q\nwant %q", level, logs, want)
		}
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
// 変わらない (-O 0 / -O 2)。@log の値は、両方のレベルで取れた値を比べ、1 つでも食い違えば失敗。ただし -O 2 で地点がずれうる
// @log (logImprecise: 合流点の地点、丸ごと展開したループの見出しと最後のブロック、ループの回転で付け替えた地点。2026-10-13 に
// 200 本の種で測って地点の約 32%) は比べない。@log は命令を持たない注釈なので、展開したループの写しや回転したループの入口の
// 注釈がジャンプの整理で合流点や出口へ寄り、別の周の値を出すことがある (Agent/wiki/log-annotation-rules.md)。
// 以前は全部を比べて「食い違った種が 1% + 3 個まで」としていて、その中に本物の不具合 (消した代入の古い値を出す。
// TestLogDeadDefCopy) が埋もれていた (2026-10-13。食い違った 29 件を分けると、28 件がずれうる地点だった)。
func TestLogZeroCost(t *testing.T) {
	t.Parallel()
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
			var skip map[int]bool
			for _, level := range []int{-1, 0} {
				plain, out, _, _, err := logBuild(t, files, level, false, false)
				if err != nil {
					t.Skipf("生成したプログラムがビルド・実行できない (seed %d): %v", seed, err)
				}
				rom, outLog, logs, res, err := logBuild(t, files, level, true, true)
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
				if level == 0 {
					skip = logImprecise(res.Log)
				}
			}
			bad, diffs := compareLogValues(logsByLevel[0], logsByLevel[1], skip)
			if bad != "" {
				t.Errorf("-O 0 と -O 2 で @log の値が違う (seed %d): %s。最適化のパスの変更で注釈の引き継ぎ・値の印 (LogStale / LogNoValue / LogArg.Stale) が崩れていないか (Agent/wiki/log-annotation-rules.md)\n%s", seed, bad, g.allSource())
			}
			if diffs > 0 {
				t.Logf("実行回数が -O 0 と -O 2 で違う地点: %d (最適化で地点が動いたもの。値は比べない)", diffs)
			}
		})
	}
}

// logImprecise は -O 2 で地点がずれうる @log の ID: 合流点の地点 (Prevs がある: ほかの経路からも来るラベルに寄った) と、
// 地点の情報が approx のもの (丸ごと展開したループの見出しと最後のブロック・ループの回転で付け替えた注釈。ジャンプの整理で
// 合流点や出口へ寄る)。回転で付け替えた地点を比べると、1 万 2 千本で 4 本食い違った (どれも jumps を切ると消える)。
func logImprecise(lf *fclog.LogFile) map[int]bool {
	r := map[int]bool{}
	if lf == nil {
		return r
	}
	for _, p := range lf.Points {
		for _, s := range p.Sites {
			if s.Prevs != nil || s.Approx {
				r[p.ID] = true
			}
		}
	}
	return r
}

var reLogField = regexp.MustCompile(` (\S+)=(\S+)`)

// compareLogValues は -O 0 / -O 2 の @log の出力 (文ごとの `file:line#id a=1 b=?`) を地点ごとに比べる。-O 2 は地点が
// 最適化で動く (回転したループの条件は 1 回ずれる、丸ごと畳まれたループの写しは 1 つの命令に集まって順番が崩れる) ので、
// 実行回数が同じ地点だけ、その地点のどの回でも両方で取れた変数の値の組を多重集合として比べる (順番は比べない)。
// skip の ID の地点は比べない。回数の違いの数も返す。
func compareLogValues(o0, o2 string, skip map[int]bool) (bad string, countDiffs int) {
	// 行は元の文字列の部分文字列のまま地点ごとにまとめ (写さない)、名前と値は地点ごとに比べるときだけ取り出す。全部の行を先に
	// map にすると、@log を 58 万行出す種 (長いループ) で 1GB を超え、nomad-lab の 2GB の上限で fuzz が落ちていた (2026-10-22)
	group := func(s string) map[string][]string {
		r := map[string][]string{}
		for s != "" {
			line := s
			if i := strings.IndexByte(s, '\n'); i >= 0 {
				line, s = s[:i], s[i+1:]
			} else {
				s = ""
			}
			if line = strings.TrimSpace(line); line == "" {
				continue
			}
			loc, rest, _ := strings.Cut(line, " ")
			r[loc] = append(r[loc], rest)
		}
		return r
	}
	// fields は `a=1 b=?` の名前と値 (reLogField と同じく最後の = で分ける)
	fields := func(lines []string) []map[string]string {
		ms := make([]map[string]string, len(lines))
		for i, rest := range lines {
			m := map[string]string{}
			for _, f := range strings.Fields(rest) {
				if k := strings.LastIndexByte(f, '='); k > 0 && k < len(f)-1 {
					m[f[:k]] = f[k+1:]
				}
			}
			ms[i] = m
		}
		return ms
	}
	a, b := group(o0), group(o2)
	for loc, aLines := range a {
		if i := strings.LastIndexByte(loc, '#'); i >= 0 {
			if id, err := strconv.Atoi(loc[i+1:]); err == nil && skip[id] {
				continue
			}
		}
		if len(aLines) != len(b[loc]) {
			countDiffs++
			continue
		}
		as, bs := fields(aLines), fields(b[loc])
		// 比べる変数: この地点のどの回でも、両方のレベルで値が取れたもの (「?」でなく、番地がレベルで違うポインタ `$...` でない)。
		// 回ごとに組にすると、回転したループで出る順が入れ替わり、片方だけ「?」の変数があるときに別の回どうしを比べていた
		names := map[string]bool{}
		for name := range as[0] {
			names[name] = true
		}
		for _, m := range append(append([]map[string]string{}, as...), bs...) {
			for name := range names {
				if v, ok := m[name]; !ok || v == "?" || strings.HasPrefix(v, "$") {
					delete(names, name)
				}
			}
		}
		var keys []string
		for name := range names {
			keys = append(keys, name)
		}
		sort.Strings(keys)
		key := func(m map[string]string) string {
			var s strings.Builder
			for _, n := range keys {
				s.WriteString(n + "=" + m[n] + " ")
			}
			return s.String()
		}
		count := map[string]int{}
		for i := range as {
			count[key(as[i])]++
			count[key(bs[i])]--
		}
		for k, n := range count {
			if n != 0 {
				return fmt.Sprintf("%s: 値の組 %q の回数が -O 0 と -O 2 で %d 違う", loc, k, n), countDiffs
			}
		}
	}
	return "", countDiffs
}

// TestLogDeadDefCopy: 使われない代入を消した後、写しの伝播で同じ変数が別の値 (前の版) の置き場所として生きている地点の @log は、
// 消した代入の値ではなく前の版を出していた (`p0 = l0; l0 = (l0 + p0) % 327; return p0` が return l0 になる。fuzz の
// TestLogZeroCost の種 66600070)。-O 2 では「?」(LogArg.Stale)、-O 0 では代入した値。
func TestLogDeadDefCopy(t *testing.T) {
	t.Parallel()
	src := `#fc 4
use console;
var g:i16;
function f(p0:i16):i16 @(noinline)
{
	var l0:i16 = g;
	p0 = l0;
	l0 = (l0 + p0) % 327;
	@log("l0={}", l0);
	return p0;
}
function main():void
{
	g = 31773;
	@printf("{}\n", f(1));
	console.exit(0);
}
`
	for _, c := range []struct {
		level int
		want  string
	}{{-1, "l0=299"}, {0, "l0=?"}} {
		_, out, logs, _, err := logBuild(t, map[string]string{"t.fc": src}, c.level, true, false)
		if err != nil {
			t.Fatal(err)
		}
		if out != "31773\n" || !strings.Contains(logs, c.want) {
			t.Errorf("level %d: out %q, logs %q (want %q)", c.level, out, logs, c.want)
		}
	}
}

// TestLogResidentHomeAtEntry: 常駐 (l1@A) のメモリ側 (Home) は書き戻しが IR の定義に見えず、関数の入口から生きて見える。
// その間 (丸ごと展開したループの初めの周) の @log が Home の古い値を出していた (fuzz の種 50042125。codegen の liveHere)。
// プログラムは testdata/regress/log50042125 (生成器を変えると同じ種でも別のプログラムになるので、そのときの生成物を固定した)。
func TestLogResidentHomeAtEntry(t *testing.T) {
	t.Parallel()
	files := regressFiles(t, "log50042125")
	var logs []string
	for _, level := range []int{-1, 0} {
		_, _, l, _, err := logBuild(t, files, level, true, true)
		if err != nil {
			t.Fatal(err)
		}
		logs = append(logs, l)
	}
	if bad, _ := compareLogValues(logs[0], logs[1], nil); bad != "" {
		t.Errorf("-O 0 と -O 2 で @log の値が違う: %s", bad)
	}
}

// TestLogLitpropHome: 常駐の割付の後の litprop がループの変数の初期値の書き込み (`load i = 0`) を消すと、常駐のメモリ側 (Home) には
// 初期値が入らないのに、常駐がレジスタにない地点の @log がメモリ側を読んでいた (fuzz の種 63000128。codegen の logLoc)。
// プログラムは testdata/regress/log63000128 (TestLogResidentHomeAtEntry と同じく固定した)。
func TestLogLitpropHome(t *testing.T) {
	t.Parallel()
	files := regressFiles(t, "log63000128")
	var logs []string
	for _, level := range []int{-1, 0} {
		_, _, l, _, err := logBuild(t, files, level, true, true)
		if err != nil {
			t.Fatal(err)
		}
		logs = append(logs, l)
	}
	if bad, _ := compareLogValues(logs[0], logs[1], nil); bad != "" {
		t.Errorf("-O 0 と -O 2 で @log の値が違う: %s", bad)
	}
}

// regressFiles は testdata/regress/<name> の .fc を読む (ファイル名 → 中身)。fuzz で見つけたプログラムを、生成器が変わっても
// 同じ形で試すために固定したもの。
func regressFiles(t *testing.T, name string) map[string]string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("testdata", "regress", name, "*.fc"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("testdata/regress/%s: %v", name, err)
	}
	files := map[string]string{}
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		files[filepath.Base(p)] = string(b)
	}
	return files
}
