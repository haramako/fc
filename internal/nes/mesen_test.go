package nes

// MesenCE (--testrunner) による高精度な自動プレイテスト。
// internal/nes のスモークと同じシナリオ (タイトルでA → 右移動+ジャンプ) を
// 実機精度のエミュレータで実行し、ゲーム内変数 (_bg_cur_area) の変化で
// エリア移動を直接アサートする。
//
// Mesen が見つからない場合は Skip する:
//   1. 環境変数 FC_MESEN (Mesen.exe のパス)
//   2. C:\Applications\MesenCE\Mesen.exe (既定の導入場所)

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/haramako/fc/internal/driver"
)

func runTool(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v failed: %v\n%s", name, args, err, out)
	}
}

func findMesen() string {
	if p := os.Getenv("FC_MESEN"); p != "" {
		return p
	}
	if runtime.GOOS == "windows" {
		p := `C:\Applications\MesenCE\Mesen.exe`
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// ensureMesenSettings はヘッドレス初回起動用の settings.json を用意する。
// settings.json が無いと初回起動ダイアログで止まり、コントローラ設定が無いと
// 入力注入 (setInput) が効かない (ポートに何も接続されていない扱いになる)。
// 既存の settings.json には手を触れない。
func ensureMesenSettings(t *testing.T, mesenPath string) {
	t.Helper()
	p := filepath.Join(filepath.Dir(mesenPath), "settings.json")
	if _, err := os.Stat(p); err == nil {
		return
	}
	cfg := `{ "Nes": { "Port1": { "Type": "NesController" } } }`
	if err := os.WriteFile(p, []byte(cfg), 0o666); err != nil {
		t.Fatalf("Mesen settings.json の作成に失敗: %v", err)
	}
	t.Logf("Mesen settings.json を作成した: %s", p)
}

// copyTree は src ディレクトリを dst へ再帰コピーする (.fc-build は除く)。
func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".fc-build" {
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(dst, rel), 0o777)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dst, rel), b, 0o666)
	})
	if err != nil {
		t.Fatalf("コピーに失敗: %v", err)
	}
}

// buildCastleWithMap は examples/castle をビルドし、ROM と ld65 マップのパスを返す。
// examples/castle を一時ディレクトリに複製してからビルドする:
// internal/fc の TestExampleCastle と `go test ./...` で並列に走るため、
// リポジトリ内の .fc-build を共有すると競合する。
func buildCastleWithMap(t *testing.T) (romPath, mapPath string) {
	t.Helper()
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "castle")
	copyTree(t, filepath.Join(repoRoot, "examples", "castle"), dir)
	src := filepath.Join(dir, "src")
	t.Chdir(src)

	compiler := driver.NewCompiler(repoRoot)
	if _, err := compiler.Build("main.fc", &driver.BuildOptions{Target: "nes", CompileOnly: true}); err != nil {
		t.Fatalf("コンパイル失敗: %v", err)
	}
	runTool(t, src, "ca65", "data.asm", "-o", ".fc-build/data.o")

	objs, err := filepath.Glob(filepath.Join(src, ".fc-build", "*.o"))
	if err != nil || len(objs) == 0 {
		t.Fatalf("オブジェクトファイルが見つからない: %v", err)
	}
	tmp := t.TempDir()
	romPath = filepath.Join(tmp, "castle.nes")
	mapPath = filepath.Join(tmp, "castle.map")
	args := []string{"-o", romPath, "-vm", "-m", mapPath, "-C", "ld65.cfg"}
	for _, o := range objs {
		rel, _ := filepath.Rel(dir, o)
		args = append(args, filepath.ToSlash(rel))
	}
	args = append(args, "res/sound/bgm.o", "res/sound/castle.o", "nsd/lib/NSD.lib")
	runTool(t, dir, "ld65", args...)
	return romPath, mapPath
}

// parseLd65Map は ld65 のマップファイルからシンボル→アドレスの表を作る。
func parseLd65Map(t *testing.T, mapPath string, symbols ...string) map[string]int {
	t.Helper()
	b, err := os.ReadFile(mapPath)
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`([A-Za-z_][A-Za-z0-9_]*)\s+([0-9A-Fa-f]{6})\s+[A-Z]{2,3}`)
	all := map[string]int{}
	for _, m := range re.FindAllStringSubmatch(string(b), -1) {
		if _, ok := all[m[1]]; !ok {
			n, _ := strconv.ParseInt(m[2], 16, 32)
			all[m[1]] = int(n)
		}
	}
	r := map[string]int{}
	for _, s := range symbols {
		addr, ok := all[s]
		if !ok {
			t.Fatalf("シンボル %s がマップファイルにない", s)
		}
		r[s] = addr
	}
	return r
}

// castlePlayLua は自動プレイの Lua スクリプトを生成する。
// シナリオ: タイトルで A (数回試行) → 右移動しつつ、たまに A(ジャンプ)を1秒 →
// _bg_cur_area が2回変化したら成功 (exit 0)、上限フレームで失敗 (exit 1)。
func castlePlayLua(addrMyX, addrCurArea int) string {
	return fmt.Sprintf(`
local ADDR_MY_X = 0x%04x
local ADDR_CUR_AREA = 0x%04x
local frame = 0
local lastArea = -1
local areaChanges = 0
local startX = -1

local function buttonsAt(f)
  -- タイトル決定の A (取りこぼし対策で3回試行)
  if (f >= 180 and f < 190) or (f >= 300 and f < 310) or (f >= 420 and f < 430) then
    return {a=true}
  end
  -- フィールド: 右移動 + 180フレーム周期でAを1秒
  if f >= 490 then
    local phase = (f - 490) %% 180
    local t = {right=true}
    if phase >= 60 and phase < 120 then t.a = true end
    return t
  end
  return {}
end

emu.addEventCallback(function()
  local t = buttonsAt(frame)
  local input = {a=false,b=false,select=false,start=false,up=false,down=false,left=false,right=false}
  for k,v in pairs(t) do input[k] = v end
  emu.setInput(input, 0)
end, emu.eventType.inputPolled)

emu.addEventCallback(function()
  frame = frame + 1
  if frame == 480 then
    startX = emu.read(ADDR_MY_X, emu.memType.nesDebug, false)
    lastArea = emu.read(ADDR_CUR_AREA, emu.memType.nesDebug, false)
    emu.log("field start: my_x=" .. startX .. " area=" .. lastArea)
  end
  if frame > 490 and frame %% 10 == 0 then
    local area = emu.read(ADDR_CUR_AREA, emu.memType.nesDebug, false)
    if area ~= lastArea then
      areaChanges = areaChanges + 1
      emu.log("area change #" .. areaChanges .. ": " .. lastArea .. " -> " .. area .. " (frame " .. frame .. ")")
      lastArea = area
      if areaChanges >= 2 then
        emu.log("PASS: my_x=" .. emu.read(ADDR_MY_X, emu.memType.nesDebug, false))
        emu.stop(0)
      end
    end
  end
  if frame > 6000 then
    emu.log("FAIL: area changes=" .. areaChanges)
    emu.stop(1)
  end
end, emu.eventType.startFrame)
`, addrMyX, addrCurArea)
}

func TestMesenPlayCastle(t *testing.T) {
	mesen := findMesen()
	if mesen == "" {
		t.Skip("MesenCE が見つからない (FC_MESEN を設定するか C:\\Applications\\MesenCE に導入)")
	}
	ensureMesenSettings(t, mesen)

	rom, mapPath := buildCastleWithMap(t)
	addrs := parseLd65Map(t, mapPath, "_my_x", "_bg_cur_area")
	t.Logf("シンボル: _my_x=$%04x _bg_cur_area=$%04x", addrs["_my_x"], addrs["_bg_cur_area"])

	luaPath := filepath.Join(t.TempDir(), "play_castle.lua")
	if err := os.WriteFile(luaPath, []byte(castlePlayLua(addrs["_my_x"], addrs["_bg_cur_area"])), 0o666); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, mesen, "--testrunner", rom, luaPath)
	out, err := cmd.CombinedOutput()
	t.Logf("Mesen 出力:\n%s", out)
	if ctx.Err() != nil {
		t.Fatal("Mesen がタイムアウトした")
	}
	if err != nil {
		t.Fatalf("Mesen testrunner が失敗 (エリア移動が検出できなかった): %v", err)
	}
}
