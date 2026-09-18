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

// logProfile は局面 name で時間を使った関数の上位を出す (待ちループの関数は除く。-v で見る)。
func logProfile(t *testing.T, m *Machine, name string, st FrameStat) {
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
	fmt.Fprintf(&b, "%s の関数別 (1 フレームあたり、待ちループを除く busy %d の内訳):\n", name, st.Avg)
	for i, e := range ents {
		if i >= 25 {
			break
		}
		per := e.cyc / int64(st.Frames)
		fmt.Fprintf(&b, "  %-40s %7d  %5.1f%%\n", e.name, per, float64(e.cyc)*100/float64(total))
	}
	t.Log(b.String())
}

// castleMachine は castle の ROM を読み、待ちループの検出と関数ごとのプロファイルを設定したマシンを返す。
func castleMachine(t *testing.T, rom string, syms map[string]int, profile []ProfileSymbol) *Machine {
	t.Helper()
	m, err := LoadFile(rom)
	if err != nil {
		t.Fatal(err)
	}
	m.IdleFlag = syms["_ppu_vsync_flag"]
	m.SetProfile(profile)
	return m
}

func TestCastleFrameCycles(t *testing.T) {
	rom, mapPath, dbgPath := buildCastle(t)
	syms := parseLd65MapAll(t, mapPath)
	// 関数ごとのプロファイル (dbgfile のラベル。__direct と ca65 の無名ラベルは本体に合算する)
	var profile []ProfileSymbol
	for _, ps := range parseLd65Dbg(t, dbgPath) {
		if !strings.HasSuffix(ps.Name, "__direct") && !strings.HasPrefix(ps.Name, "@") && !strings.HasPrefix(ps.Name, ".") {
			profile = append(profile, ps)
		}
	}
	for _, s := range []string{"_ppu_vsync_flag", "_my_last_checkpoint", "_bg_cur_area"} {
		if _, ok := syms[s]; !ok {
			t.Fatalf("シンボル %s がマップファイルにない", s)
		}
	}
	got := map[string]FrameStat{}
	var m *Machine
	run := func(n int) {
		if err := m.RunFrames(n); err != nil {
			t.Fatal(err)
		}
	}
	measure := func(name string, f func()) {
		start := len(m.FrameBusy)
		for k := range m.ProfileCycles {
			m.ProfileCycles[k] = 0
		}
		f()
		got[name] = frameStat(m.FrameBusy[start:])
	}
	walk := func(cycles int) {
		for cycle := 0; cycle < cycles; cycle++ {
			m.SetButtons(ButtonRight)
			run(60)
			m.SetButtons(ButtonRight | ButtonA)
			run(60)
			m.SetButtons(ButtonRight)
			run(60)
		}
		m.SetButtons(0)
	}

	// 1. TestPlayCastle と同じ筋書き (時刻は固定): タイトル → A で開始 → ジングルとフェード → フィールドで右移動とジャンプ
	m = castleMachine(t, rom, syms, profile)
	measure("title", func() { run(180) })
	m.SetButtons(ButtonA)
	run(10)
	m.SetButtons(0)
	measure("start", func() { run(300) })
	measure("field", func() { walk(6) })
	if m.idlePc == 0 {
		t.Fatal("vsync 待ちループ (lda _ppu_vsync_flag; bne) が見つからなかった")
	}
	logProfile(t, m, "field", got["field"])

	// 2. 敵 (スライム) が複数いるエリア $33 (チェックポイント 13) に強制移動: タイトルで A を押した後、game.start が
	//    my.last_checkpoint を読むまで毎フレーム 13 を書き込む (セーブデータの読み込みが 0 に戻すので)
	m = castleMachine(t, rom, syms, profile)
	run(180)
	m.SetButtons(ButtonA)
	run(10)
	m.SetButtons(0)
	for i := 0; i < 300 && m.Get(syms["_bg_cur_area"]) != 0x33; i++ {
		m.Set(syms["_my_last_checkpoint"], 13)
		run(1)
	}
	if area := m.Get(syms["_bg_cur_area"]); area != 0x33 {
		t.Fatalf("エリア $33 に移動できなかった (cur_area = $%02x)", area)
	}
	run(60)                                     // 出現の演出を待つ
	measure("area33_idle", func() { run(300) }) // 立ち止まって敵だけが動く
	logProfile(t, m, "area33_idle", got["area33_idle"])
	measure("area33_jump", func() { // その場でジャンプ (歩くと敵に当たって死に、チェックポイントに戻ってしまう)
		for cycle := 0; cycle < 9; cycle++ {
			m.SetButtons(ButtonA)
			run(15)
			m.SetButtons(0)
			run(45)
		}
	})
	if area := m.Get(syms["_bg_cur_area"]); area != 0x33 {
		t.Errorf("area33_jump の途中でエリアを出た (cur_area = $%02x)", area)
	}
	logProfile(t, m, "area33_jump", got["area33_jump"])

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
