package nes

// games/ (fc のデバッグを兼ねた小さなゲーム) のテスト。どのゲームも AUTO (`-D main.AUTO=true`) で自分で遊ぶので、ビルドして
// 内蔵のランナーで走らせ、止まらずに進んでいることを変数で確かめる。FC_GAMES_PNG_DIR を与えると最後の画面を <名前>.png に書く。

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/haramako/fc/internal/driver"
)

// buildGame は games/<name>/main.fc を defines を付けて nes にビルドする (ビルドの生成物は一時ディレクトリに)。
func buildGame(t *testing.T, name string, level int, defines ...string) *nesProg {
	t.Helper()
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	rom := filepath.Join(tmp, name+".nes")
	if _, err := driver.NewCompiler(repoRoot).BuildContext(context.Background(), "main.fc", &driver.BuildOptions{
		Target: "nes", Dir: filepath.Join(repoRoot, "games", name), BuildDir: filepath.Join(tmp, "b"), Out: rom,
		OptimizeLevel: level, Defines: defines,
	}); err != nil {
		t.Fatalf("ビルド失敗: %v", err)
	}
	m, err := LoadFile(rom)
	if err != nil {
		t.Fatal(err)
	}
	return &nesProg{Machine: m, syms: parseLd65MapAll(t, strings.TrimSuffix(rom, ".nes")+".dbg"), rom: rom}
}

// saveGame は FC_GAMES_PNG_DIR があれば今の画面を <name>.png に書く。
func saveGame(t *testing.T, p *nesProg, name string) {
	t.Helper()
	if dir := os.Getenv("FC_GAMES_PNG_DIR"); dir != "" {
		if err := p.WriteScreenshot(filepath.Join(dir, name+".png")); err != nil {
			t.Fatal(err)
		}
	}
}

// peek16 は 2 バイトの変数。
func (p *nesProg) peek16(t *testing.T, sym string) int {
	t.Helper()
	return p.peek(t, sym, 0) | p.peek(t, sym, 1)<<8
}

// runToTick は main の end_frame に手数 ticks == k で入るまで (最大 maxFrames フレーム) 走らせる。
func (p *nesProg) runToTick(t *testing.T, k, maxFrames int) {
	t.Helper()
	entry, ok := p.syms["_main_end_frame"]
	if !ok {
		t.Fatal("_main_end_frame が無い (@(noinline) の end_frame)")
	}
	p.Stop = func() bool { return p.Cpu.Pc == entry && p.peek16(t, "_main_ticks") == k }
	defer func() { p.Stop = nil }()
	if err := p.RunFrames(maxFrames); err != nil {
		t.Fatal(err)
	}
	if !p.Stopped {
		t.Fatalf("%d フレームで手数 %d に届かない (今 %d)", maxFrames, k, p.peek16(t, "_main_ticks"))
	}
}

var mapBSS = regexp.MustCompile(`(?m)^_main\.o:\n(?:    .*\n)*?    BSS\s+Offs=([0-9A-F]+)\s+Size=([0-9A-F]+)`)
var mapSeg = regexp.MustCompile(`(?m)^BSS\s+([0-9A-F]+)\s`)

// mainVars は main のモジュールの変数 (BSS) の番地の範囲 (ld65 の .map から)。
func (p *nesProg) mainVars(t *testing.T) (int, int) {
	t.Helper()
	b, err := os.ReadFile(strings.TrimSuffix(p.rom, ".nes") + ".map")
	if err != nil {
		t.Fatal(err)
	}
	m, seg := mapBSS.FindSubmatch(b), mapSeg.FindSubmatch(b)
	if m == nil || seg == nil {
		t.Fatalf("map に _main.o の BSS が無い")
	}
	off, _ := strconv.ParseInt(string(m[1]), 16, 32)
	size, _ := strconv.ParseInt(string(m[2]), 16, 32)
	start, _ := strconv.ParseInt(string(seg[1]), 16, 32)
	return int(start + off), int(start + off + size)
}

// symbolOf は番地 a を含む main の変数の名前 (a 以下で一番近いシンボル)。
func (p *nesProg) symbolOf(a int) string {
	var names []string
	for n := range p.syms {
		if strings.HasPrefix(n, "_main_") {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	best, bestAt := "?", -1
	for _, n := range names {
		if v := p.syms[n]; v <= a && v > bestAt {
			best, bestAt = n, v
		}
	}
	return fmt.Sprintf("%s+%d", best, a-bestAt)
}

// compareAtTick は name を -O 0 と -O 2 でビルドし、手数 k の所の main の変数 (BSS) が同じかを見る。AUTO の考える時間で
// 進むフレームは違っても、ゲームの論理 (乱数も) は手数ごとに決まるので、違えばコンパイラのバグ。
func compareAtTick(t *testing.T, name string, k, maxFrames int) {
	t.Helper()
	var states [2][]byte
	var progs [2]*nesProg
	for i, level := range []int{-1, 0} {
		p := buildGame(t, name, level, "main.AUTO=true")
		p.runToTick(t, k, maxFrames)
		lo, hi := p.mainVars(t)
		for a := lo; a < hi; a++ {
			states[i] = append(states[i], byte(p.Get(a)))
		}
		progs[i] = p
	}
	lo, _ := progs[0].mainVars(t)
	lo2, _ := progs[1].mainVars(t)
	if len(states[0]) != len(states[1]) || lo != lo2 {
		t.Fatalf("変数の配置が -O 0 と -O 2 で違う (%#x %d / %#x %d)", lo, len(states[0]), lo2, len(states[1]))
	}
	for i := range states[0] {
		if states[0][i] != states[1][i] {
			t.Fatalf("手数 %d で %s が違う: -O 0 は %d、-O 2 は %d", k, progs[0].symbolOf(lo+i), states[0][i], states[1][i])
		}
	}
}

// forLevels は -O 0 と -O 2 で f を走らせる。
func forLevels(t *testing.T, f func(t *testing.T, level int)) {
	for _, level := range []int{-1, 0} {
		t.Run(map[int]string{-1: "O0", 0: "O2"}[level], func(t *testing.T) {
			t.Parallel()
			f(t, level)
		})
	}
}

// TestGameBreakout: パドルが球を追って、ブロックを消して得点が入る。
func TestGameBreakout(t *testing.T) {
	t.Parallel()
	forLevels(t, func(t *testing.T, level int) {
		p := buildGame(t, "breakout", level, "main.AUTO=true")
		p.run(t, 3000)
		score, left := p.peek16(t, "_main_score"), p.peek(t, "_main_left", 0)
		t.Logf("score %d, 残り %d, 面 %d, 球 %d", score, left, p.peek(t, "_main_stage", 0), p.peek(t, "_main_balls", 0))
		if score == 0 || left >= 84 && p.peek(t, "_main_stage", 0) == 1 {
			t.Errorf("ブロックが消えていない: score %d, 残り %d", score, left)
		}
		if row := p.ntRow(0x2000, 1); !strings.Contains(row, "SCORE") {
			t.Errorf("上の行: %q", row)
		}
		p.checkVblank(t)
		if level == 0 {
			saveGame(t, p, "breakout")
		}
	})
}

// TestGameSnake: リンゴへ向かって進み、食べて伸びる (BG だけ)。
func TestGameSnake(t *testing.T) {
	t.Parallel()
	forLevels(t, func(t *testing.T, level int) {
		p := buildGame(t, "snake", level, "main.AUTO=true")
		p.run(t, 3000)
		best := p.peek(t, "_main_best", 0)
		t.Logf("長さ %d, 最長 %d", p.peek(t, "_main_len", 0), best)
		if best <= 4 {
			t.Errorf("伸びていない: 最長 %d", best)
		}
		p.checkVblank(t)
		if level == 0 {
			saveGame(t, p, "snake")
		}
	})
}

// TestGameMines: 確かなマスを開け・旗を立てて、何回か勝つか負ける (480 マスで添字は u16)。
func TestGameMines(t *testing.T) {
	t.Parallel()
	forLevels(t, func(t *testing.T, level int) {
		p := buildGame(t, "mines", level, "main.AUTO=true")
		p.run(t, 12000) // AUTO は 1 手ごとに盤を見直すので、-O 0 では 1 手に数十フレームかかる
		wins, losses := p.peek(t, "_main_wins", 0), p.peek(t, "_main_losses", 0)
		t.Logf("勝ち %d, 負け %d, 開けた %d, 旗 %d", wins, losses, p.peek16(t, "_main_opened"), p.peek(t, "_main_flags", 0))
		if wins+losses == 0 {
			t.Errorf("1 回も終わっていない")
		}
		p.checkVblank(t)
		if level == 0 {
			saveGame(t, p, "mines")
		}
	})
}

// TestGameSokoban: 面ごとの解き方をなぞって (1 手戻すのも混ぜて) 全部の面を解く。16×16 のメタタイルと属性。
func TestGameSokoban(t *testing.T) {
	t.Parallel()
	forLevels(t, func(t *testing.T, level int) {
		p := buildGame(t, "sokoban", level, "main.AUTO=true")
		p.run(t, 2400)
		cleared := p.peek(t, "_main_cleared", 0)
		t.Logf("解いた面 %d, 今の面 %d, 残りの箱 %d", cleared, p.peek(t, "_main_level", 0), p.peek(t, "_main_boxes_left", 0))
		if cleared < 5 {
			t.Errorf("5 面を解いていない: %d", cleared)
		}
		p.checkVblank(t)
		if level == 0 {
			saveGame(t, p, "sokoban")
		}
	})
}

// TestGameBlocks: 置ける所を全部試して落とし、行を消す (2 次元配列の行の写し、i8 の座標)。
func TestGameBlocks(t *testing.T) {
	t.Parallel()
	forLevels(t, func(t *testing.T, level int) {
		p := buildGame(t, "blocks", level, "main.AUTO=true")
		p.run(t, 3600)
		lines := p.peek16(t, "_main_lines")
		t.Logf("消した行 %d, 得点 %d, 終わった回数 %d", lines, p.peek16(t, "_main_score"), p.peek(t, "_main_games", 0))
		if lines == 0 && p.peek(t, "_main_games", 0) == 0 {
			t.Errorf("行を消していない")
		}
		p.checkVblank(t)
		if level == 0 {
			saveGame(t, p, "blocks")
		}
	})
}

// TestGamesSameAtTick: どのゲームも、手数を決めた所の main の変数が -O 0 と -O 2 で同じ。
func TestGamesSameAtTick(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name   string
		tick   int
		frames int
	}{
		{"breakout", 2500, 4000},
		{"snake", 2500, 4000},
		{"mines", 300, 20000},
		{"sokoban", 2000, 4000},
		{"blocks", 300, 8000},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			compareAtTick(t, c.name, c.tick, c.frames)
		})
	}
}
