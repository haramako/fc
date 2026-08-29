package nes

// examples の ROM がヘッドレスNESランナーで「大雑把に動く」ことのスモークテスト。
// 判定基準 (ゆるい):
//   - Nフレーム実行してクラッシュ (不正オペコード) しない
//   - 描画が有効化される
//   - VRAM への意味のある書き込みがある (画面が作られている)
//   - スクリーンショットが単色でない
//
// FC_NES_SNAPSHOT_DIR 環境変数を設定すると、スクリーンショットPNGをそこに保存する
// (目視確認用)。

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"testing"
)

func loadExample(t *testing.T, name string) *Machine {
	t.Helper()
	path := filepath.Join("..", "..", "testdata", "golden", "examples", name+".nes")
	m, err := LoadFile(path)
	if err != nil {
		t.Fatalf("ROM読み込み失敗: %v", err)
	}
	return m
}

func distinctColors(img *image.RGBA) int {
	seen := map[uint32]bool{}
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y += 2 {
		for x := b.Min.X; x < b.Max.X; x += 2 {
			r, g, bb, _ := img.At(x, y).RGBA()
			seen[r<<16|g<<8|bb] = true
		}
	}
	return len(seen)
}

func snapshot(t *testing.T, m *Machine, name string) {
	t.Helper()
	if dir := os.Getenv("FC_NES_SNAPSHOT_DIR"); dir != "" {
		path := filepath.Join(dir, name+".png")
		if err := m.WriteScreenshot(path); err != nil {
			t.Logf("スクリーンショット保存失敗: %v", err)
		} else {
			t.Logf("スクリーンショット: %s", path)
		}
	}
}

func assertAlive(t *testing.T, m *Machine, label string) {
	t.Helper()
	s := m.Stats
	t.Logf("%s: frames=%d instr=%d nmi=%d irq=%d vramWrites=%d oamDma=%d rendering=%v",
		label, s.Frames, s.Instructions, s.NmiCount, s.IrqCount, s.VramWrites, s.OamDmaCount, s.RenderingEverEnabled)
	if !s.RenderingEverEnabled {
		t.Errorf("%s: 描画が一度も有効化されていない", label)
	}
	if s.VramWrites < 256 {
		t.Errorf("%s: VRAM書き込みが少なすぎる (%d)", label, s.VramWrites)
	}
}

func TestSmokeMiku(t *testing.T) {
	m := loadExample(t, "miku")
	if err := m.RunFrames(120); err != nil {
		t.Fatal(err)
	}
	assertAlive(t, m, "miku")
	img := m.Screenshot()
	if n := distinctColors(img); n < 2 {
		t.Errorf("画面が単色 (%d色) — 描画されていない", n)
	}
	snapshot(t, m, "miku-120")

	// スタートを押してさらに実行してもクラッシュしない
	m.SetButtons(ButtonStart)
	if err := m.RunFrames(10); err != nil {
		t.Fatal(err)
	}
	m.SetButtons(0)
	if err := m.RunFrames(170); err != nil {
		t.Fatal(err)
	}
	snapshot(t, m, "miku-300")
}

// diffRatio は2枚の画面の異なるピクセルの割合 (0.0〜1.0)。
func diffRatio(a, b *image.RGBA) float64 {
	diff := 0
	total := 0
	for i := 0; i < len(a.Pix); i += 4 {
		total++
		if a.Pix[i] != b.Pix[i] || a.Pix[i+1] != b.Pix[i+1] || a.Pix[i+2] != b.Pix[i+2] {
			diff++
		}
	}
	return float64(diff) / float64(total)
}

// TestPlayCastle は castle を実際に「プレイ」する:
// タイトルで A → 待ち → フィールドで右移動しつつ、たまに A(ジャンプ)を1秒押す、
// を繰り返し、画面(エリア)が切り替わることを確認する。
func TestPlayCastle(t *testing.T) {
	m := loadExample(t, "castle")

	// タイトル表示まで
	if err := m.RunFrames(180); err != nil {
		t.Fatal(err)
	}
	title := m.Screenshot()

	// A で「はじめる」を決定
	m.SetButtons(ButtonA)
	if err := m.RunFrames(10); err != nil {
		t.Fatal(err)
	}
	m.SetButtons(0)

	// ジングル(30f) + フェード(40f) + フィールドロード待ち
	if err := m.RunFrames(300); err != nil {
		t.Fatal(err)
	}
	field := m.Screenshot()
	snapshot(t, m, "castle-play-field")
	if r := diffRatio(title, field); r < 0.2 {
		t.Fatalf("タイトルから画面が変わっていない (差分 %.1f%%) — ゲームが始まっていない", r*100)
	}
	t.Logf("フィールド到達 (タイトルとの差分 %.1f%%)", diffRatio(title, field)*100)

	// 右移動 + たまに A(ジャンプ)を1秒。画面切り替え(大きな画面変化)を数える
	prev := field
	transitions := 0
	for cycle := 0; cycle < 25 && transitions < 2; cycle++ {
		m.SetButtons(ButtonRight)
		if err := m.RunFrames(60); err != nil {
			t.Fatal(err)
		}
		m.SetButtons(ButtonRight | ButtonA) // ジャンプを1秒
		if err := m.RunFrames(60); err != nil {
			t.Fatal(err)
		}
		m.SetButtons(ButtonRight)
		if err := m.RunFrames(60); err != nil {
			t.Fatal(err)
		}
		cur := m.Screenshot()
		r := diffRatio(prev, cur)
		if r > 0.25 {
			transitions++
			t.Logf("画面切り替え検出 #%d (cycle %d, 差分 %.1f%%)", transitions, cycle, r*100)
			snapshot(t, m, fmt.Sprintf("castle-play-area%d", transitions))
		}
		prev = cur
	}
	m.SetButtons(0)
	if transitions == 0 {
		t.Errorf("画面切り替えが検出できなかった (右移動+ジャンプで進行していない可能性)")
	}
	t.Logf("プレイ結果: frames=%d 画面切り替え=%d回", m.Stats.Frames, transitions)
}

func TestSmokeCastle(t *testing.T) {
	m := loadExample(t, "castle")
	if err := m.RunFrames(180); err != nil {
		t.Fatal(err)
	}
	assertAlive(t, m, "castle-title")
	img := m.Screenshot()
	// castle のタイトルは白黒2色なので、閾値は「単色でない」こと
	if n := distinctColors(img); n < 2 {
		t.Errorf("画面が単色 (%d色) — 描画されていない", n)
	}
	snapshot(t, m, "castle-180")

	// タイトルでスタートを押してゲーム開始方向に進める
	before := fmt.Sprintf("%v", img.Pix)
	m.SetButtons(ButtonStart)
	if err := m.RunFrames(10); err != nil {
		t.Fatal(err)
	}
	m.SetButtons(0)
	if err := m.RunFrames(300); err != nil {
		t.Fatal(err)
	}
	assertAlive(t, m, "castle-after-start")
	after := m.Screenshot()
	snapshot(t, m, "castle-490")
	if fmt.Sprintf("%v", after.Pix) == before {
		t.Logf("注: スタート押下後も画面に変化なし (進行していない可能性はあるがエラーにはしない)")
	}
}
