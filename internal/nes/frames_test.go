package nes

// castle のマクロベンチ: 内蔵 NES ランナーで自動プレイし、フレームごとの CPU 使用量 (busy サイクル = フレーム長 −
// vsync 待ちループの時間) を局面ごとに集計して bench/castle_frames.json と比べる。ゲーム形ベンチ (bench/) が
// 拾えない「実ゲーム 1 フレームの重さ」を見るためのもの。入力の時刻が固定でランナーが決定的なので、値は
// コンパイラの出力だけで決まる。意図した変化なら `go test ./internal/nes -run CastleFrame -update` で更新する。

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

var updateFrames = flag.Bool("update", false, "bench/castle_frames.json を現在の値で書き換える")

// FrameStat は 1 つの局面の集計。
type FrameStat struct {
	Frames int   `json:"frames"`
	Avg    int64 `json:"avg"`  // 1 フレームあたりの平均 busy サイクル
	Max    int64 `json:"max"`  // 最大
	Over   int   `json:"over"` // 待ちループに入らなかった (1 フレームに収まらなかった) フレーム数
}

func frameStat(busy []int64) FrameStat {
	st := FrameStat{Frames: len(busy)}
	var sum int64
	for _, b := range busy {
		sum += b
		st.Max = max(st.Max, b)
		if b >= FrameCycles {
			st.Over++
		}
	}
	if len(busy) > 0 {
		st.Avg = sum / int64(len(busy))
	}
	return st
}

// logProfile はフィールドの局面で時間を使った関数の上位を出す (待ちループの関数は除く。-v で見る)。
func logProfile(t *testing.T, m *Machine, st FrameStat) {
	type ent struct {
		name string
		cyc  int64
	}
	var ents []ent
	var total int64
	for k, s := range m.ProfileSymbols {
		c := m.ProfileCycles[k]
		if c == 0 || s.Name == "_ppu_wait_vsync_with_flag" {
			continue
		}
		ents = append(ents, ent{s.Name, c})
		total += c
	}
	sort.Slice(ents, func(i, j int) bool { return ents[i].cyc > ents[j].cyc })
	var b strings.Builder
	fmt.Fprintf(&b, "field の関数別 (1 フレームあたり、待ちループを除く busy %d の内訳):\n", st.Avg)
	for i, e := range ents {
		if i >= 25 {
			break
		}
		per := e.cyc / int64(st.Frames)
		fmt.Fprintf(&b, "  %-40s %7d  %5.1f%%\n", e.name, per, float64(e.cyc)*100/float64(total))
	}
	t.Log(b.String())
}

func TestCastleFrameCycles(t *testing.T) {
	rom, mapPath := buildCastleWithMap(t)
	syms := parseLd65Map(t, mapPath, "_ppu_vsync_flag")
	m, err := LoadFile(rom)
	if err != nil {
		t.Fatal(err)
	}
	m.IdleFlag = syms["_ppu_vsync_flag"]
	// 関数ごとのプロファイル (マップの全シンボル。__direct は本体に合算する)
	var profile []ProfileSymbol
	for name, addr := range parseLd65MapAll(t, mapPath) {
		if addr >= 0x8000 && !strings.HasSuffix(name, "__direct") {
			profile = append(profile, ProfileSymbol{Name: name, Addr: addr})
		}
	}
	m.SetProfile(profile)
	run := func(n int) {
		if err := m.RunFrames(n); err != nil {
			t.Fatal(err)
		}
	}
	got := map[string]FrameStat{}
	measure := func(name string, f func()) {
		start := len(m.FrameBusy)
		f()
		got[name] = frameStat(m.FrameBusy[start:])
	}
	// TestPlayCastle と同じ筋書き (時刻は固定): タイトル → A で開始 → ジングルとフェード → フィールドで右移動とジャンプ
	measure("title", func() { run(180) })
	m.SetButtons(ButtonA)
	run(10)
	m.SetButtons(0)
	measure("start", func() { run(300) })
	for k := range m.ProfileCycles {
		m.ProfileCycles[k] = 0 // フィールドだけを見る
	}
	measure("field", func() {
		for cycle := 0; cycle < 6; cycle++ {
			m.SetButtons(ButtonRight)
			run(60)
			m.SetButtons(ButtonRight | ButtonA)
			run(60)
			m.SetButtons(ButtonRight)
			run(60)
		}
	})
	m.SetButtons(0)
	if m.idlePc == 0 {
		t.Fatal("vsync 待ちループ (lda _ppu_vsync_flag; bne) が見つからなかった")
	}
	logProfile(t, m, got["field"])

	path := filepath.Join("..", "..", "bench", "castle_frames.json")
	want := map[string]FrameStat{}
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &want); err != nil {
			t.Fatal(err)
		}
	}
	names := make([]string, 0, len(got))
	for n := range got {
		names = append(names, n)
	}
	sort.Strings(names)
	var table strings.Builder
	fmt.Fprintf(&table, "%-8s %7s %10s %10s %8s %10s %6s   (1 frame = %d cycles)\n", "phase", "frames", "avg", "prev", "diff", "max", "over", FrameCycles)
	changed := false
	for _, n := range names {
		g, w := got[n], want[n]
		diff := ""
		if w.Frames > 0 {
			diff = fmt.Sprintf("%+.1f%%", float64(g.Avg-w.Avg)*100/float64(w.Avg))
		}
		fmt.Fprintf(&table, "%-8s %7d %10d %10d %8s %10d %6d\n", n, g.Frames, g.Avg, w.Avg, diff, g.Max, g.Over)
		if g != w {
			changed = true
		}
	}
	t.Log("\n" + table.String())
	if *updateFrames {
		data, _ := json.MarshalIndent(got, "", "  ")
		if err := os.WriteFile(path, append(data, '\n'), 0o666); err != nil {
			t.Fatal(err)
		}
		return
	}
	if changed {
		t.Errorf("フレームごとのサイクル数が castle_frames.json と違う。意図した変化なら `go test ./internal/nes -run CastleFrame -update` で更新する\n%s", table.String())
	}
}
