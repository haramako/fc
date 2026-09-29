package nes

// fc 4 の NES の標準ライブラリ (fclib/nes の frame / vram / pal / oam / pad。doc/v4_stdlib.md §3.2) を内蔵ランナーで走らせ、
// ネームテーブル・パレット・OAM・vblank の中に収まっているかを見る。

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haramako/fc/internal/driver"
)

// nesProg はビルドした ROM を載せたランナーとシンボルの表。
type nesProg struct {
	*Machine
	syms map[string]int
}

// buildNes は files (名前 → ソース。main は t.fc) を NES の ROM にしてランナーに載せる。
func buildNes(t *testing.T, files map[string]string) *nesProg {
	t.Helper()
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o666); err != nil {
			t.Fatal(err)
		}
	}
	rom := filepath.Join(dir, "t.nes")
	if _, err := driver.NewCompiler(repoRoot).BuildContext(context.Background(), "t.fc", &driver.BuildOptions{
		Target: "nes", Dir: dir, BuildDir: filepath.Join(dir, "b"), Out: rom,
	}); err != nil {
		t.Fatalf("ビルド失敗: %v", err)
	}
	m, err := LoadFile(rom)
	if err != nil {
		t.Fatal(err)
	}
	return &nesProg{Machine: m, syms: parseLd65MapAll(t, strings.TrimSuffix(rom, ".nes")+".dbg")}
}

func (p *nesProg) run(t *testing.T, frames int) {
	t.Helper()
	if err := p.RunFrames(frames); err != nil {
		t.Fatal(err)
	}
}

// ntRow はネームテーブル (番地 base) の y 行目 (末尾の 0 を除く)。
func (p *nesProg) ntRow(base, y int) string {
	var b []byte
	for a := base + y*32; a < base+(y+1)*32; a++ {
		b = append(b, p.readVram(a))
	}
	return string(bytes.TrimRight(b, "\x00"))
}

// peek はシンボル sym の番地 + off の RAM の値。
func (p *nesProg) peek(t *testing.T, sym string, off int) int {
	t.Helper()
	a, ok := p.syms[sym]
	if !ok {
		t.Fatalf("シンボル %s が無い", sym)
	}
	return p.Get(a + off)
}

// checkVblank は描画中の VRAM の書き込みが vblank に収まっていたか。
func (p *nesProg) checkVblank(t *testing.T) {
	t.Helper()
	t.Logf("vblank の使用: 最大 %d / %d サイクル, NMI %d 回, OAM DMA %d 回", p.Stats.MaxVblankUse, VblankCycles, p.Stats.NmiCount, p.Stats.OamDmaCount)
	if p.Stats.LateVramWrites != 0 {
		t.Errorf("vblank の外で VRAM に書いた: %d 回", p.Stats.LateVramWrites)
	}
}

// TestNesFrameVram: 描画を止めて write_now で書き、パレットを置いて描画を出す。毎フレーム put した数がネームテーブルに出て、
// NMI の書き込みが vblank に収まる。
func TestNesFrameVram(t *testing.T) {
	t.Parallel()
	p := buildNes(t, map[string]string{"t.fc": `#fc 4
use frame;
use vram;
use pal;
const PALETTE = [0x0f, 0x30, 0x16, 0x27, 0x0f, 0x11];
var line:[16]u8;
public var n:u8;
function main():void
{
	frame.init();
	vram.write_now(vram.addr(0, 2, 2), "HELLO");
	vram.fill_now(vram.addr(0, 0, 3), 0x2d, 4);
	vram.write_v_now(vram.addr(0, 30, 10), "VERT");
	pal.set_all(PALETTE);
	frame.render_on();
	vram.put_v(vram.addr(1, 1, 1), "AB");
	vram.fill(vram.addr(0, 10, 3), 0x2a, 3);
	while (true) {
		n += 1;
		vram.put(vram.addr(0, 2, 4), @format(line, "F{:3}", n));
		frame.wait();
	}
}
`})
	p.run(t, 20)
	if got := p.ntRow(0x2000, 2); got != "\x00\x00HELLO" {
		t.Errorf("2 行目: %q", got)
	}
	if got := p.ntRow(0x2000, 3); got != "\x2d\x2d\x2d\x2d\x00\x00\x00\x00\x00\x00\x2a\x2a\x2a" {
		t.Errorf("3 行目: %q", got)
	}
	if got := p.ntRow(0x2000, 4); !strings.HasPrefix(got, "\x00\x00F ") {
		t.Errorf("4 行目: %q", got)
	}
	var v []byte
	for y := 10; y < 14; y++ {
		v = append(v, p.readVram(0x2000+y*32+30))
	}
	if string(v) != "VERT" {
		t.Errorf("縦: %q", v)
	}
	if p.readVram(0x2400+32+1) != 'A' || p.readVram(0x2400+64+1) != 'B' {
		t.Errorf("put_v: %q %q", p.readVram(0x2400+32+1), p.readVram(0x2400+64+1))
	}
	if !bytes.Equal(p.palette[:6], []byte{0x0f, 0x30, 0x16, 0x27, 0x0f, 0x11}) {
		t.Errorf("パレット: % x", p.palette[:6])
	}
	// 数は毎フレーム 1 つ進む
	n := p.peek(t, "_t_n", 0)
	row := p.ntRow(0x2000, 4)
	p.run(t, 1)
	if p.peek(t, "_t_n", 0) != n+1 {
		t.Errorf("フレームごとに進まない: %d → %d", n, p.peek(t, "_t_n", 0))
	}
	if p.ntRow(0x2000, 4) == row {
		t.Errorf("4 行目が変わらない: %q", row)
	}
	p.checkVblank(t)
}

// TestNesOamPad: pad.poll の held / pressed / released と、oam の spr / meta / end (使わなかった分を隠す) / reserve。
// 2P のパッドも読む。
func TestNesOamPad(t *testing.T) {
	t.Parallel()
	p := buildNes(t, map[string]string{"t.fc": `#fc 4
use nes;
use frame;
use oam;
use pad;
// 2 枚のメタスプライト: (0, 0) と (8, 0)
const META = [0, 0, 0x10, 1, 8, 0, 0x11, 1];
public var x:u8;
public var presses:u8;
public var releases:u8;
public var p2held:u8;
public var nspr:u8;
function main():void
{
	frame.init();
	frame.render_on();
	oam.reserve(1);
	oam.set(0, 200, 50, 0x55, 0);
	x = 100;
	while (true) {
		pad.poll();
		if ((pad.p1.held & pad.RIGHT) != 0) {
			x += 1;
		}
		if ((pad.p1.pressed & pad.A) != 0) {
			presses += 1;
		}
		if ((pad.p1.released & pad.A) != 0) {
			releases += 1;
		}
		p2held = pad.p2.held;
		oam.begin();
		oam.spr(x, 60, 1, 2);
		if (presses < 2) {
			oam.meta(250, 70, META, 0);         // 2 枚目は右の端の外 (258) なので置かない
			oam.meta(20, 80, META, nes.ATTR_FLIP_H);
		}
		nspr = oam.count();
		oam.end();
		frame.wait();
	}
}
`})
	p.run(t, 10)
	oam := func(i int) [4]byte { return [4]byte{p.oam[i*4], p.oam[i*4+1], p.oam[i*4+2], p.oam[i*4+3]} }
	if got := oam(0); got != [4]byte{50, 0x55, 0, 200} {
		t.Errorf("固定の 0 枚目: %v", got)
	}
	if got := oam(1); got != [4]byte{60, 1, 2, 100} {
		t.Errorf("1 枚目: %v", got)
	}
	// meta (250, 70): (0, 0) だけ置かれる。反転の (20, 80): dx は -0-8 = -8 → x 12 (タイル 0x10)、-8-8 = -16 → x 4
	if got := oam(2); got != [4]byte{70, 0x10, 1, 250} {
		t.Errorf("2 枚目: %v", got)
	}
	if got := oam(3); got != [4]byte{80, 0x10, 1 ^ 0x40, 12} {
		t.Errorf("3 枚目: %v", got)
	}
	if got := oam(4); got != [4]byte{80, 0x11, 1 ^ 0x40, 4} {
		t.Errorf("4 枚目: %v", got)
	}
	if got := oam(5); got[0] != 0xff {
		t.Errorf("5 枚目が隠れていない: %v", got)
	}
	if p.peek(t, "_t_nspr", 0) != 5 {
		t.Errorf("count: %d", p.peek(t, "_t_nspr", 0))
	}
	// 右を押している間だけ進む。2P は別に読む
	x0 := p.peek(t, "_t_x", 0)
	p.SetButtons(ButtonRight)
	p.SetButtons2(ButtonA | ButtonUp)
	p.run(t, 3)
	p.SetButtons(0)
	p.run(t, 2)
	if d := p.peek(t, "_t_x", 0) - x0; d != 3 {
		t.Errorf("右で進んだ数: %d", d)
	}
	if got := p.peek(t, "_t_p2held", 0); got != 0x80|0x08 {
		t.Errorf("2P: %#x", got)
	}
	// A を 2 回押して離す: pressed / released はその瞬間の 1 フレームだけ
	for i := 0; i < 2; i++ {
		p.SetButtons(ButtonA)
		p.run(t, 3)
		p.SetButtons(0)
		p.run(t, 3)
	}
	if p.peek(t, "_t_presses", 0) != 2 || p.peek(t, "_t_releases", 0) != 2 {
		t.Errorf("pressed %d, released %d", p.peek(t, "_t_presses", 0), p.peek(t, "_t_releases", 0))
	}
	// meta を置かなくなったので end が 2〜4 枚目を隠す
	for i := 2; i <= 4; i++ {
		if got := oam(i); got[0] != 0xff {
			t.Errorf("%d 枚目が隠れていない: %v", i, got)
		}
	}
	if got := oam(0); got != [4]byte{50, 0x55, 0, 200} {
		t.Errorf("固定の 0 枚目が消えた: %v", got)
	}
	p.checkVblank(t)
}

// TestNesQueueFull: キューに収まる一番重い形 (1 バイトずつ写す項目で満杯) と OAM の DMA が、同じ NMI で vblank に収まる。
// 満杯を超える put は分けて次の NMI を待ってから積み、全部が画面に届く。
func TestNesQueueFull(t *testing.T) {
	t.Parallel()
	p := buildNes(t, map[string]string{"t.fc": `#fc 4
use frame;
use vram;
use oam;
var big:[200]u8;
public var phase:u8;
function main():void
{
	frame.init();
	frame.render_on();
	for (var i:u8 = 0; i < 200; i += 1) {
		big[i] = i;
	}
	while (true) {
		oam.begin();
		oam.spr(1, 2, 3, 0);
		oam.end();
		if (phase < 5) {
			// 1 項目 = 3 + 125 バイトでキュー (128) を満たす
			vram.put(vram.addr(0, 0, 0), big[..125]);
		} elsif (phase == 5) {
			// キューより大きい put (分けて積む)
			vram.put(vram.addr(1, 0, 0), big);
		} else {
			// 小さい項目をたくさん
			var k:u8 = 0;
			while (vram.try_put(vram.addr(0, k, 8), big[k..k + 2])) {
				k += 1;
			}
		}
		phase += 1;
		frame.wait();
	}
}
`})
	p.run(t, 12)
	for i := 0; i < 200; i++ {
		if got := p.readVram(0x2400 + i); got != byte(i) {
			t.Fatalf("分けた put の %d バイト目: %d", i, got)
		}
	}
	p.checkVblank(t)
	if p.Stats.OamDmaCount == 0 {
		t.Errorf("OAM の DMA が無い")
	}
}

// TestNesHookRender: NMI の呼び出し口 (@(interrupt) の fc の関数) が毎フレーム呼ばれ、render_off の後は描画が止まって
// write_now で書け、render_on で戻る。処理落ちのフレーム (wait より長い処理) でもフレームの数は進む。
func TestNesHookRender(t *testing.T) {
	t.Parallel()
	p := buildNes(t, map[string]string{"t.fc": `#fc 4
use frame;
use vram;
public var ticks:u8;
public var stage:u8;
function tick():void @(interrupt)
{
	ticks += 1;
}
function main():void
{
	frame.init();
	frame.hook = tick;
	frame.render_on();
	frame.wait_n(3);
	frame.render_off();
	stage = 1;
	vram.write_now(vram.addr(0, 0, 5), "OFF");
	vram.put(vram.addr(0, 0, 6), "PUT");    // 描画を止めている間はその場で書く
	frame.render_on();
	stage = 2;
	// 処理落ち: 数フレーム分の計算
	var s:u16 = 0;
	for (var i:u16 = 0; i < 3000; i += 1) {
		s += i;
	}
	vram.put(vram.addr(0, 0, 7), "ON");
	frame.wait();
	stage = 3;
	while (true) {
		frame.wait();
	}
}
`})
	p.run(t, 30)
	if p.peek(t, "_t_stage", 0) != 3 {
		t.Fatalf("stage %d", p.peek(t, "_t_stage", 0))
	}
	for y, want := range map[int]string{5: "OFF", 6: "PUT", 7: "ON"} {
		if got := p.ntRow(0x2000, y); got != want {
			t.Errorf("%d 行目: %q", y, got)
		}
	}
	ticks := p.peek(t, "_t_ticks", 0)
	count := p.peek(t, "_frame_count", 0)
	if ticks == 0 || ticks != count {
		t.Errorf("呼び出し口 %d 回、フレームの数 %d", ticks, count)
	}
	if !p.renderingEnabled() {
		t.Errorf("描画が戻っていない")
	}
	p.checkVblank(t)
}

// TestNesVramRandom: 乱数で作った put / put_v / fill / reserve / wait の並びを走らせ、ネームテーブル 0 を Go の模型と比べる
// (キューの順番・分け方・手間の上限)。OAM の DMA もしながら、vblank の外で書かない。
func TestNesVramRandom(t *testing.T) {
	t.Parallel()
	for seed := int64(1); seed <= 4; seed++ {
		t.Run(fmt.Sprint(seed), func(t *testing.T) {
			t.Parallel()
			r := rand.New(rand.NewSource(seed))
			var model [960]byte
			var ops []string
			waits := 0
			for len(ops) < 150 {
				switch k := r.Intn(10); {
				case k == 0:
					ops = append(ops, "255, 0, 0, 0, 0")
					waits++
				case k <= 4: // put: 横に n バイト (行をまたいでよい。ネームテーブルの中に収める)
					n := 1 + r.Intn(60)
					if r.Intn(4) == 0 {
						n = 1 + r.Intn(200)
					}
					a := r.Intn(960 - n)
					off := r.Intn(256 - n)
					for i := 0; i < n; i++ {
						model[a+i] = byte(off+i) ^ 0x5a
					}
					ops = append(ops, fmt.Sprintf("0, %d, %d, %d, %d", a%32, a/32, n, off))
				case k <= 6: // put_v: 縦に n バイト
					n := 1 + r.Intn(29)
					x, y := r.Intn(32), r.Intn(30-n+1)
					off := r.Intn(256 - n)
					for i := 0; i < n; i++ {
						model[(y+i)*32+x] = byte(off+i) ^ 0x5a
					}
					ops = append(ops, fmt.Sprintf("1, %d, %d, %d, %d", x, y, n, off))
				case k <= 8: // fill
					n := 1 + r.Intn(255)
					a := r.Intn(960 - n)
					v := r.Intn(256)
					for i := 0; i < n; i++ {
						model[a+i] = byte(v)
					}
					ops = append(ops, fmt.Sprintf("2, %d, %d, %d, %d", a%32, a/32, n, v))
				default: // reserve に直に書く
					n := 1 + r.Intn(40)
					a := r.Intn(960 - n)
					v := r.Intn(256)
					for i := 0; i < n; i++ {
						model[a+i] = byte(v + i)
					}
					ops = append(ops, fmt.Sprintf("3, %d, %d, %d, %d", a%32, a/32, n, v))
				}
			}
			src := `#fc 4
use frame;
use vram;
use oam;
const OPS = [` + strings.Join(ops, ", ") + `];
var data:[256]u8;
public var done:u8;
function main():void
{
	frame.init();
	frame.render_on();
	for (var i:u16 = 0; i < 256; i += 1) {
		data[i as u8] = (i as u8) ^ 0x5a;
	}
	var p:u16 = 0;
	while (p < @len(OPS)) {
		var k = OPS[p];
		var a = vram.addr(0, OPS[p + 1], OPS[p + 2]);
		var n = OPS[p + 3];
		var v = OPS[p + 4];
		if (k == 255) {
			oam.begin();
			oam.spr(v, n, 1, 0);
			oam.end();
			frame.wait();
		} elsif (k == 0) {
			vram.put(a, @slice(&data[v], n));
		} elsif (k == 1) {
			vram.put_v(a, @slice(&data[v], n));
		} elsif (k == 2) {
			vram.fill(a, v, n);
		} else {
			var s = vram.reserve(a, n);
			for (var j:u8 = 0; j < n; j += 1) {
				s[j] = v + j;
			}
		}
		p += 5;
	}
	frame.wait();
	done = 1;
	while (true) {
		frame.wait();
	}
}
`
			p := buildNes(t, map[string]string{"t.fc": src})
			for f := 0; f < 400 && p.peek(t, "_t_done", 0) == 0; f++ {
				p.run(t, 1)
			}
			if p.peek(t, "_t_done", 0) == 0 {
				t.Fatalf("終わらない")
			}
			for i := 0; i < 960; i++ {
				if got := p.readVram(0x2000 + i); got != model[i] {
					t.Fatalf("(%d, %d): %#x, 模型は %#x", i%32, i/32, got, model[i])
				}
			}
			t.Logf("wait %d 回, フレーム %d", waits, p.Stats.Frames)
			p.checkVblank(t)
		})
	}
}

// TestExampleHello: examples/hello (fc 4 の NES の標準ライブラリの最小の例) がビルドでき、文字が出てフェードが終わり、十字キーで
// スプライトが動く。FC_HELLO_PNG を与えると画面を PNG に書く (目で見る用)。
func TestExampleHello(t *testing.T) {
	t.Parallel()
	src, err := os.ReadFile(filepath.Join("..", "..", "examples", "hello", "hello.fc"))
	if err != nil {
		t.Fatal(err)
	}
	p := buildNes(t, map[string]string{"t.fc": string(src)})
	p.run(t, 60)
	if got := p.ntRow(0x2000, 10); got != strings.Repeat("\x00", 9)+"HELLO, WORLD!" {
		t.Errorf("10 行目: %q", got)
	}
	if got := p.ntRow(0x2000, 14); got != strings.Repeat("\x00", 10)+"X 124  Y 130" {
		t.Errorf("14 行目: %q", got)
	}
	if p.palette[1] != 0x30 || p.palette[0x11] != 0x27 {
		t.Errorf("フェードの後のパレット: % x", p.palette)
	}
	p.SetButtons(ButtonRight | ButtonDown)
	p.run(t, 5)
	p.SetButtons(0)
	p.run(t, 2)
	if x, y := p.oam[3], p.oam[0]; x != 129 || y != 135 {
		t.Errorf("スプライト: (%d, %d)", x, y)
	}
	if path := os.Getenv("FC_HELLO_PNG"); path != "" {
		if err := p.WriteScreenshot(path); err != nil {
			t.Fatal(err)
		}
	}
	p.checkVblank(t)
}

// TestNesRle: @rle で圧縮した画面を、描画を止めて write_rle_now で書き、描画中に put_rle でキューに積む (繰り返しは埋める項目)。
func TestNesRle(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewSource(3))
	screen := make([]byte, 1024)
	for i := range screen {
		if i > 0 && r.Intn(4) != 0 {
			screen[i] = screen[i-1]
		} else {
			screen[i] = byte(r.Intn(50))
		}
	}
	row := make([]byte, 300)
	for i := range row {
		row[i] = byte(i/37) + 100
	}
	p := buildNes(t, map[string]string{"t.fc": `#fc 4
use frame;
use vram;
const SCREEN = @rle(@incbin("screen.bin"));
const ROW = @rle(@incbin("row.bin"));
public var done:u8;
function main():void
{
	frame.init();
	vram.write_rle_now(0x2000, SCREEN);
	frame.render_on();
	vram.put_rle(0x2400, ROW);
	frame.wait();
	done = 1;
	while (true) {
		frame.wait();
	}
}
`, "screen.bin": string(screen), "row.bin": string(row)})
	p.run(t, 30)
	if p.peek(t, "_t_done", 0) != 1 {
		t.Fatal("終わらない")
	}
	for i := 0; i < 1024; i++ {
		if got := p.readVram(0x2000 + i); got != screen[i] {
			t.Fatalf("write_rle_now の %d バイト目: %d, want %d", i, got, screen[i])
		}
	}
	// fc の既定の iNES のヘッダは縦のミラーなので、ネームテーブル 1 ($2400) は $2000 と別の所
	for i := 0; i < len(row); i++ {
		if got := p.readVram(0x2400 + i); got != row[i] {
			t.Fatalf("put_rle の %d バイト目: %d, want %d", i, got, row[i])
		}
	}
	p.checkVblank(t)
}
