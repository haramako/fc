package nes

// 実プロジェクトの退行の切り分け用 (環境変数が無ければ Skip。doc/development_notes.md「実プロジェクトの退行の切り分け」):
//   TestProbeDiff: 2 つの ROM (FC_PROBE_ROM_A / _B。最適化のパスを FC_DISABLE で切ったものなど) を同じ入力で並走させ、
//     両方が vsync 待ちに入ったフレームだけゲームの状態 (BSS / WRAM) を比べて、最初に食い違うフレームと番地を出す。
//     FC_PROBE_DBG (/ _B) の dbgfile で番地 → 名前。FC_PROBE_PREFIX=_my_,_en_ で比べる変数を絞る。FC_PROBE_WATCH=名前,... で
//     終わりに値を出す。
//   TestProbePlay: TestMesenPlayCastle と同じ筋書きを内蔵ランナーで走らせ、エリアと状態の推移を出す。
//   TestProbeSwitch: 踏むスイッチのあるエリア (FC_PROBE_AREA、既定 de) に飛んでスイッチの上に落ち、沈むかを見る
//     (チェックポイント 0 のエリアを ROM 上で書き換えて任意のエリアから始める例)。
// 例: FC_PROBE_ROM_A=a.nes FC_PROBE_ROM_B=b.nes FC_PROBE_DBG=a.dbg FC_PROBE_DBG_B=b.dbg go test ./internal/nes -run TestProbeDiff -v

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/haramako/fc/internal/driver"
)

func TestProbeDiff(t *testing.T) {
	romA, romB := os.Getenv("FC_PROBE_ROM_A"), os.Getenv("FC_PROBE_ROM_B")
	if romA == "" || romB == "" {
		t.Skip("FC_PROBE_ROM_A / B が無い")
	}
	// 番地 → 名前 (A の dbgfile)。FC_PROBE_DBG_B があれば B の配置が違うものとして、名前ごとに対応する番地を比べる
	type sym struct {
		name string
		addr int
	}
	// 名前 → 番地と、次のラベルまでの長さ
	loadSyms := func(path string) ([]sym, map[string]int, map[string]int) {
		d, err := driver.ParseDbgFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var syms []sym
		byName := map[string]int{}
		for _, s := range d.Symbols {
			if s.Lab && s.Val >= 0x200 && s.Val < 0x8000 {
				syms = append(syms, sym{s.Name, s.Val})
				byName[s.Name] = s.Val
			}
		}
		sort.Slice(syms, func(i, j int) bool { return syms[i].addr < syms[j].addr })
		size := map[string]int{}
		for i, s := range syms {
			end := 0x8000
			if i+1 < len(syms) {
				end = syms[i+1].addr
			}
			if end > 0x800 && s.addr < 0x800 {
				end = 0x800
			}
			if end > 0x7e00 {
				end = 0x7e00
			}
			size[s.name] = end - s.addr
		}
		return syms, byName, size
	}
	symsA, _, sizeA := loadSyms(os.Getenv("FC_PROBE_DBG"))
	var byNameB, sizeB map[string]int
	if p := os.Getenv("FC_PROBE_DBG_B"); p != "" {
		_, byNameB, sizeB = loadSyms(p)
	}
	// 比べる区間: (A の番地, B の番地, 長さ, 名前)
	type span struct {
		a, b, n int
		name    string
	}
	var spans []span
	inRange := func(a int) bool { return a >= 0x200 && a < 0x800 || a >= 0x6000 && a < 0x7e00 }
	prefixes := strings.Split(os.Getenv("FC_PROBE_PREFIX"), ",") // 比べる名前の接頭辞 (空なら全部)
	for _, s := range symsA {
		if !inRange(s.addr) || s.name == "FC_SRAM" || s.name == "FC_STACK" || s.name == "FC_SP" {
			continue
		}
		if os.Getenv("FC_PROBE_PREFIX") != "" && s.name != "_ppu_vsync_flag" {
			ok := false
			for _, p := range prefixes {
				if strings.HasPrefix(s.name, p) {
					ok = true
				}
			}
			if !ok {
				continue
			}
		}
		n := sizeA[s.name]
		b := s.addr
		if byNameB != nil {
			var ok bool
			if b, ok = byNameB[s.name]; !ok {
				continue
			}
			if sizeB[s.name] < n {
				n = sizeB[s.name]
			}
		}
		spans = append(spans, span{s.addr, b, n, s.name})
	}
	nameOf := func(addr int) string {
		for _, sp := range spans {
			if addr >= sp.a && addr < sp.a+sp.n {
				if addr == sp.a {
					return sp.name
				}
				return fmt.Sprintf("%s+%d", sp.name, addr-sp.a)
			}
		}
		return ""
	}
	a, err := LoadFile(romA)
	if err != nil {
		t.Fatal(err)
	}
	b, err := LoadFile(romB)
	if err != nil {
		t.Fatal(err)
	}
	copyAll := func() {
		if byNameB != nil {
			return // 配置が違うとポインタの値が違うので写せない
		}
		for _, sp := range spans {
			for i := 0; i < sp.n; i++ {
				b.Set(sp.b+i, a.Get(sp.a+i))
			}
		}
	}
	_ = copyAll
	// 入力の筋書き: タイトル → A → 右移動 / ジャンプ / B / 上 を混ぜる
	script := func(frame int) byte {
		switch {
		case frame < 180:
			return 0
		case frame < 190:
			return ButtonA
		case frame < 480:
			return 0
		}
		f := (frame - 480) % 240
		switch {
		case f < 60:
			return ButtonRight
		case f < 90:
			return ButtonRight | ButtonA
		case f < 120:
			return ButtonB
		case f < 150:
			return ButtonUp
		case f < 180:
			return ButtonLeft
		default:
			return ButtonLeft | ButtonA
		}
	}
	for _, sp := range spans {
		if sp.name == "_ppu_vsync_flag" {
			a.IdleFlag, b.IdleFlag = sp.a, sp.b
		}
	}
	if a.IdleFlag == 0 {
		t.Fatal("_ppu_vsync_flag が無い")
	}
	diffs := 0
	compared := 0
	resync := false
	for frame := 0; frame < 4000; frame++ {
		btn := script(frame)
		a.SetButtons(btn)
		b.SetButtons(btn)
		if err := a.RunFrames(1); err != nil {
			t.Fatalf("A: %v", err)
		}
		if err := b.RunFrames(1); err != nil {
			t.Fatalf("B: %v", err)
		}
		// 両方が 1 フレームに収まった (vsync 待ちに入った) フレームだけ比べる。収まらなかった期間 (ロード) の後は
		// 進み方が違いうるので、グローバルを A に合わせてから続ける
		if !a.FrameWaited || !b.FrameWaited {
			resync = true
			continue
		}
		if resync {
			resync = false
			copyAll()
			continue
		}
		compared++
		var bad []int
		badB := map[int]int{}
		for _, sp := range spans {
			if sp.name == "_ppu_vsync_flag" {
				continue
			}
			for i := 0; i < sp.n; i++ {
				if a.Get(sp.a+i) != b.Get(sp.b+i) {
					bad = append(bad, sp.a+i)
					badB[sp.a+i] = sp.b + i
				}
			}
		}
		if len(bad) > 0 {
			sort.Ints(bad)
			t.Logf("frame %d (buttons %02x): %d bytes differ", frame, btn, len(bad))
			for i, addr := range bad {
				if i >= 12 {
					t.Logf("  ...")
					break
				}
				t.Logf("  $%04x %-32s A=%3d B=%3d", addr, nameOf(addr), a.Get(addr), b.Get(badB[addr]))
			}
			diffs++
			if diffs >= 3 {
				return
			}
			// 以降は B を A に合わせて続ける (次の食い違いを見るため)
			if byNameB == nil {
				for _, addr := range bad {
					b.Set(badB[addr], a.Get(addr))
				}
			}
		}
	}
	t.Logf("no divergence in 4000 frames (compared %d)", compared)
	for _, n := range strings.Split(os.Getenv("FC_PROBE_WATCH"), ",") {
		for _, sp := range spans {
			if sp.name == n {
				t.Logf("  %s A=%d B=%d", n, a.Get(sp.a), b.Get(sp.b))
			}
		}
	}
}

// TestProbePlay は TestMesenPlayCastle と同じ筋書き (右移動 + 周期ジャンプ) を内蔵ランナーで再現し、エリア・状態の推移を出す。
func TestProbePlay(t *testing.T) {
	rom := os.Getenv("FC_PROBE_ROM_A")
	if rom == "" {
		t.Skip()
	}
	d, err := driver.ParseDbgFile(os.Getenv("FC_PROBE_DBG"))
	if err != nil {
		t.Fatal(err)
	}
	syms := map[string]int{}
	for _, s := range d.Symbols {
		if s.Lab && s.Val < 0x8000 {
			if _, ok := syms[s.Name]; !ok {
				syms[s.Name] = s.Val
			}
		}
	}
	m, err := LoadFile(rom)
	if err != nil {
		t.Fatal(err)
	}
	m.IdleFlag = syms["_ppu_vsync_flag"]
	last := ""
	for f := 0; f < 3000; f++ {
		var btn byte
		if (f >= 180 && f < 190) || (f >= 300 && f < 310) || (f >= 420 && f < 430) {
			btn = ButtonA
		}
		if f >= 490 {
			phase := (f - 490) % 180
			btn = ButtonRight
			if phase >= 60 && phase < 120 {
				btn |= ButtonA
			}
		}
		m.SetButtons(btn)
		if err := m.RunFrames(1); err != nil {
			t.Fatal(err)
		}
		cur := fmt.Sprintf("area=%d state=%d", m.Get(syms["_bg_cur_area"]), m.Get(syms["_my_state"]))
		if cur != last {
			t.Logf("f%d: %s x=%d y=%d", f, cur, m.Get(syms["_my_x"]), m.Get(syms["_my_y"]))
			last = cur
		}
	}
}

// TestProbeSwitch は踏むスイッチ (en TYPE_SWITCH=13) のあるエリアに飛び (FC_PROBE_AREA、既定 0x62)、スイッチの上に
// 落ちて my.state が ON_ENEMY (4) になり flags が立つかを見る。チェックポイント 0 の [area, x, y] を ROM 上で書き換える。
func TestProbeSwitch(t *testing.T) {
	rom := os.Getenv("FC_PROBE_ROM_A")
	if rom == "" {
		t.Skip()
	}
	d, err := driver.ParseDbgFile(os.Getenv("FC_PROBE_DBG"))
	if err != nil {
		t.Fatal(err)
	}
	syms := map[string]int{}
	var cpOffset = -1
	for _, s := range d.Symbols {
		if s.Lab && s.Val < 0x8000 {
			if _, ok := syms[s.Name]; !ok {
				syms[s.Name] = s.Val
			}
		}
		if s.Name == "_resource_MAP_CHECKPOINT_DATA" && s.Lab {
			seg := d.Segments[s.Seg]
			if seg != nil && seg.Ooffs >= 0 {
				cpOffset = seg.Ooffs + s.Val - seg.Start
			}
		}
	}
	if cpOffset < 0 {
		t.Fatal("MAP_CHECKPOINT_DATA が無い")
	}
	b, err := os.ReadFile(rom)
	if err != nil {
		t.Fatal(err)
	}
	area := 0x62
	if v := os.Getenv("FC_PROBE_AREA"); v != "" {
		fmt.Sscanf(v, "%x", &area)
	}
	t.Logf("checkpoint 0: area=%d x=%d y=%d → area=$%02x", b[cpOffset], b[cpOffset+1], b[cpOffset+2], area)
	b[cpOffset] = byte(area)
	m, err := New(b)
	if err != nil {
		t.Fatal(err)
	}
	m.IdleFlag = syms["_ppu_vsync_flag"]
	run := func(n int) {
		if err := m.RunFrames(n); err != nil {
			t.Fatal(err)
		}
	}
	run(180)
	m.SetButtons(ButtonA)
	run(10)
	m.SetButtons(0)
	run(300)
	t.Logf("area=$%02x my=(%d,%d) state=%d", m.Get(syms["_bg_cur_area"]), m.Get(syms["_my_x"]), m.Get(syms["_my_y"]), m.Get(syms["_my_state"]))
	sw := -1
	for i := 0; i < 16; i++ {
		ty := m.Get(syms["_en_type"] + i)
		if ty != 0 {
			t.Logf("en[%d] type=%d (%d,%d) p1=%d p2=%d", i, ty, m.Get(syms["_en_px"]+i), m.Get(syms["_en_py"]+i), m.Get(syms["_en_p1"]+i), m.Get(syms["_en_p2"]+i))
		}
		if ty == 13 && sw < 0 {
			sw = i
		}
	}
	if sw < 0 {
		t.Fatal("スイッチが無い")
	}
	// スイッチの少し上に置いて落とす
	m.Set(syms["_my_x"], m.Get(syms["_en_px"]+sw))
	m.Set(syms["_my_y"], m.Get(syms["_en_py"]+sw)-40)
	m.Set(syms["_my_state"], 2) // STATE_JUMP
	flag := m.Get(syms["_en_p1"] + sw)
	last := ""
	for f := 0; f < 120; f++ {
		run(1)
		cur := fmt.Sprintf("my=(%d,%d) state=%d on_idx=%d sw_y=%d flag[%d]=%d", m.Get(syms["_my_x"]), m.Get(syms["_my_y"]), m.Get(syms["_my_state"]), m.Get(syms["_my_on_idx"]), m.Get(syms["_en_py"]+sw), flag, m.Get(syms["_my_flags"]+flag))
		if cur != last {
			t.Logf("f%d: %s", f, cur)
			last = cur
		}
	}
}
