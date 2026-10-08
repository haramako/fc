package nes

// games/ (fc のデバッグを兼ねた小さなゲーム) のテスト。どのゲームも AUTO (`-D main.AUTO=true`) で自分で遊ぶので、ビルドして
// 内蔵のランナーで走らせ、止まらずに進んでいることを変数で確かめる。FC_GAMES_PNG_DIR を与えると最後の画面を <名前>.png に書く。

import (
	"context"
	"os"
	"path/filepath"
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
