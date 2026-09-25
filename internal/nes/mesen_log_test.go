package nes

// @log の Mesen 2 用スクリプト (<rom>.fclog.lua) を MesenCE (--testrunner) で実際に動かし、emu ターゲットの表示
// (fcc run -g) と行ごとに同じになることを確かめる (doc/v3_plan.md §9)。testrunner では emu.log が stdout に出ないので、
// emu.log を print に差し替えてから生成したスクリプトを読む。

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/haramako/fc/internal/driver"
)

// mesenLogBody は nes / emu で共通の本体: ループ先頭・if の合流点・呼び出しの直後の地点、再帰関数 (スタックのフレーム)、
// 符号付き・16 ビット・enum・bool の書式、ループ展開される for。
const mesenLogBody = `#fc 3
enum Dir { Up, Down, Left, Right }
var g8:u8;
var g16:u16;
var gi16:i16;
var flag:bool;
var dir:Dir;
var buf:[8]u8;

function sum(n:u8):u16 @(noinline)
{
	var s:u16 = 0;
	var k:u8 = 0;
	while (k < n) {
		@log("sum head k={} s={}", k, s);
		s += (k as u16) * 300;
		k += 1;
	}
	@log("sum done s={} {:04X}", s, s);
	return s;
}

function branch(x:u8):u8 @(noinline)
{
	var r:u8;
	if ((x & 1) != 0) {
		r = x * 3;
		@log("odd x={} r={}", x, r);
	} else {
		r = x / 2;
	}
	@log("merge r={}", r);
	return r;
}

function rec(d:u8, acc:u16):u16
{
	var loc:u16 = acc + (d as u16);
	@log("rec d={} acc={} loc={}", d, acc, loc);
	if (d == 0) { return loc; }
	return rec(d - 1, loc * 2);
}

function signed(v:i16):i16 @(noinline)
{
	var w:i16 = v - 1000;
	@log("signed v={} w={} w={:x} w={:6d}|{:06}", v, w, w, w, w);
	return w;
}

function bump():void @(noinline) { g8 += 1; }

public function run():void
{
	g8 = 7;
	for (var i = 0; i < 4; i += 1) {
		buf[i] = branch(i + g8);
	}
	@log("buf={} {} {} {}", buf[0], buf[1], buf[2], buf[3]);
	g16 = sum(5);
	gi16 = signed(g16 as i16);
	gi16 = signed(-300);
	g16 = rec(3, 1);
	flag = g16 > 20;
	dir = .Left;
	@log("g16={} flag={} dir={} dir={:d}", g16, flag, dir, dir);
	var j:u8 = 0;
	bump();
	@log("before loop g8={}", g8);
	while (j < 3) { j += 1; g8 += j; }
	@log("after loop g8={}", g8);
	while (true) {
		@log("in loop g8={}", g8);
		if (g8 > 20) { break; }
		g8 += 3;
	}
	for (var k:u8 = 0; k < 2; k += 1) {
		if (k == 1) { @log("k1"); } else { g8 += 1; }
		@log("k={} g8={}", k, g8);
	}
}
`

// mesenLogLua は生成したスクリプトの前後に、emu.log を print にする差し替えと、DONE で止める・上限フレームで失敗する
// 処理を足す。
func mesenLogLua(script string) string {
	return `-- @log の表示を stdout へ
emu.log = function(s)
  print(s)
  if s == "DONE" then emu.stop(0) end
end
` + script + `
local frame = 0
emu.addEventCallback(function()
  frame = frame + 1
  if frame > 600 then emu.stop(1) end
end, emu.eventType.startFrame)
`
}

func TestMesenLog(t *testing.T) {
	mesen := findMesen()
	if mesen == "" {
		t.Skip("MesenCE が見つからない (FC_MESEN を設定するか C:\\Applications\\MesenCE に導入)")
	}
	ensureMesenSettings(t, mesen)
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	mains := map[string]string{
		"emu": "#fc 3\nuse * from stdio;\nuse body;\nfunction main():void\n{\n\tbody.run();\n\t@log(\"DONE\");\n\texit(0);\n}\n",
		"nes": "#fc 3\nuse nes;\nuse body;\n" +
			"function nmi():void @(symbol: \"_interrupt\") { }\nfunction irq():void @(symbol: \"_interrupt_irq\") { }\n" +
			"function main():void\n{\n\tbody.run();\n\t@log(\"DONE\");\n\twhile (true) { }\n}\n",
	}
	for _, level := range []int{-1, 1, 2} {
		// emu (fcc run -g) の表示が期待値
		var want strings.Builder
		{
			dir := filepath.Join(t.TempDir(), "emu")
			writeFiles(t, dir, map[string]string{"t.fc": mains["emu"], "body.fc": mesenLogBody})
			var out strings.Builder
			_, err := driver.NewCompiler(repoRoot).BuildContext(context.Background(), "t.fc", &driver.BuildOptions{
				Dir: dir, BuildDir: filepath.Join(dir, "b"), Out: filepath.Join(dir, "a.bin"),
				Run: true, Stdout: &out, LogOut: &want, OptimizeLevel: level, Debug: true, MaxCycles: 10_000_000,
			})
			if err != nil {
				t.Fatalf("-O %d: emu のビルド・実行に失敗: %v", level, err)
			}
		}
		if !strings.Contains(want.String(), "k=1 ") || !strings.HasSuffix(want.String(), "DONE\n") {
			t.Fatalf("-O %d: emu の表示が足りない:\n%s", level, want.String())
		}

		// nes -g の ROM と .fclog.lua を Mesen で
		dir := filepath.Join(t.TempDir(), "nes")
		writeFiles(t, dir, map[string]string{"t.fc": mains["nes"], "body.fc": mesenLogBody})
		rom := filepath.Join(dir, "t.nes")
		if _, err := driver.NewCompiler(repoRoot).BuildContext(context.Background(), "t.fc", &driver.BuildOptions{
			Target: "nes", Dir: dir, BuildDir: filepath.Join(dir, "b"), Out: rom, OptimizeLevel: level, Debug: true,
		}); err != nil {
			t.Fatalf("-O %d: nes のビルドに失敗: %v", level, err)
		}
		script, err := os.ReadFile(filepath.Join(dir, "t.fclog.lua"))
		if err != nil {
			t.Fatal(err)
		}
		luaPath := filepath.Join(dir, "run.lua")
		if err := os.WriteFile(luaPath, []byte(mesenLogLua(string(script))), 0o666); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		out, err := exec.CommandContext(ctx, mesen, "--testrunner", rom, luaPath).Output()
		cancel()
		if ctx.Err() == context.DeadlineExceeded {
			t.Fatalf("-O %d: Mesen がタイムアウトした", level)
		}
		var got []string
		for _, line := range strings.Split(strings.ReplaceAll(string(out), "\r\n", "\n"), "\n") {
			if line != "" && !strings.HasPrefix(line, "fc @log:") {
				got = append(got, line)
			}
		}
		if err != nil {
			t.Errorf("-O %d: Mesen が DONE まで来なかった (%v):\n%s", level, err, strings.Join(got, "\n"))
			continue
		}
		if g := strings.Join(got, "\n") + "\n"; g != want.String() {
			t.Errorf("-O %d: Mesen の表示が emu と違う\n--- Mesen\n%s--- emu\n%s", level, g, want.String())
		}
	}
}

func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o777); err != nil {
		t.Fatal(err)
	}
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o666); err != nil {
			t.Fatal(err)
		}
	}
}
