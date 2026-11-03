#!/usr/bin/env python3
"""fcmesen.py: fc の NES の ROM を Mesen 2 (MesenCE) でヘッドレスに動かし、@log・console の出力・panic を端末に出す。

    python3 tools/nesrun/fcmesen.py game.nes [--frames 600] [--until TEXT] [--input 10-12:right,30:start]
                                             [--peek main.score --peek sys.msg:4] [--lua extra.lua] [--mesen PATH]

- `fcc build -t nes -g` で ROM と一緒に書いた game.fclog.lua があれば読み込む (@log)
- fc のランナーの番地 ($4018 に書いた文字・$4019 に書いた終了コード) を見る: console の出力と sys.panic の理由を出し、
  $4019 に書いたらその値で終わる (panic は 1)
- 終わり方: --until の文字列を含む行が出たら 0、--frames のフレームを過ぎたら 0 (--until を書いたときは 2)、$4019 ならその値
- --peek は終わるときに変数の値を出す (`mod.var`。大きさは .dbg から。`名前:バイト数` で変えられる)
- --png / --shot フレーム:ファイル は画面を PNG に書く (Mesen の emu.takeScreenshot。256×240)
- --nametable N / --palette は終わるときにネームテーブル (32×30) とパレットを出す (PPU を副作用なしに読むので、ゲームの側で
  描画中に PPUADDR / PPUDATA を触って読み戻す必要は無い)
- Mesen は --mesen か環境変数 FC_MESEN (Windows は C:\\Applications\\MesenCE\\Mesen.exe も)。初めてのときは Mesen の隣に
  最小の settings.json を作る (無いと初回のダイアログで止まり、コントローラが無い扱いで入力が効かない)
- Mesen は ROM の名前でセーブデータ (Saves/<名前>.sav) を残すので、ROM を fc_run_<名前> に写して動かし、そのセーブを前後で消す
- Mesen はプロセスに 1 つ: 手で動かしている Mesen があると (Windows) 動かない
"""

import argparse
import os
import shutil
import subprocess
import sys
import tempfile

sys.dont_write_bytecode = True  # tools/nesrun に __pycache__ を作らない
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import fcsym  # noqa: E402

LUA = r"""
-- fcmesen.py が作る (@log を stdout へ、fc のランナーの番地、入力、止める条件)
local FRAMES, UNTIL = %(frames)d, %(until)s
local INPUT = { %(input)s }
local PEEK = { %(peek)s }
local NT = { %(nt)s }
local PALETTE = %(palette)s
local SHOTS = { %(shots)s }   -- フレーム → 1 (そのフレームの終わりの画面を PNG にして 16 進で出す。fcmesen.py がファイルに書く)

local function shot(tag)
  local png = emu.takeScreenshot()
  local hex = {}
  for i = 1, #png do hex[i] = string.format("%%02x", string.byte(png, i)) end
  print("[png " .. tag .. "] " .. table.concat(hex))
end
local frame = 0

local function peek()
  for _, p in ipairs(PEEK) do
    local v, s = 0, {}
    for i = 0, p.size - 1 do
      local b = emu.read(p.addr + i, emu.memType.nesDebug, false)
      v = v | (b << (8 * i))
      s[#s + 1] = string.format("%%02x", b)
    end
    print(string.format("[peek] %%s = %%d (0x%%s)", p.name, v, table.concat(s, " ")))
  end
  -- ネームテーブル (PPU の番地 $2000 + 0x400 * n を副作用なしに読む。ミラーはマッパーのとおり)
  for _, t in ipairs(NT) do
    for row = 0, 29 do
      local s = {}
      for col = 0, 31 do
        local b = emu.read(0x2000 + 0x400 * t.n + row * 32 + col, emu.memType.nesPpuDebug, false)
        if t.hex then
          s[#s + 1] = string.format("%%02x", b)
        elseif b >= 0x20 and b < 0x7f then
          s[#s + 1] = string.char(b)
        else
          s[#s + 1] = "."
        end
      end
      if t.hex then
        print(string.format("[nt%%d %%02d] %%s", t.n, row, table.concat(s, " ")))
      else
        print(string.format("[nt%%d %%02d] |%%s|", t.n, row, table.concat(s)))
      end
    end
  end
  if PALETTE then
    local s = {}
    for i = 0, 31 do
      s[#s + 1] = string.format("%%02x", emu.read(0x3f00 + i, emu.memType.nesPpuDebug, false))
    end
    print("[palette] " .. table.concat(s, " "))
  end
end
local function stop(code)
  peek()
  if SHOTS.last then shot("last") end
  emu.stop(code)
end
local function line(s)
  print(s)
  if UNTIL and string.find(s, UNTIL, 1, true) then stop(0) end
end
emu.log = line

local out = {}
emu.addMemoryCallback(function(addr, value)
  if value == 10 then line(table.concat(out)); out = {} else out[#out + 1] = string.char(value) end
end, emu.callbackType.write, 0x4018)
emu.addMemoryCallback(function(addr, value)
  if #out > 0 then line(table.concat(out)); out = {} end
  print("[exit " .. value .. "]")
  stop(value)
end, emu.callbackType.write, 0x4019)

emu.addEventCallback(function()
  if SHOTS[frame] then shot(tostring(frame)) end
end, emu.eventType.endFrame)
emu.addEventCallback(function()
  frame = frame + 1
  if frame > FRAMES then
    print("[frames " .. FRAMES .. "]")
    stop(UNTIL and 2 or 0)
  end
end, emu.eventType.startFrame)
emu.addEventCallback(function()
  local on = {}
  for _, e in ipairs(INPUT) do
    if e.lo <= frame and frame <= e.hi then
      for _, b in ipairs(e.b) do on[b] = true end
    end
  end
  emu.setInput(on, 0)
end, emu.eventType.inputPolled)
"""


def find_mesen(arg):
    for p in (arg, os.environ.get("FC_MESEN"), shutil.which("Mesen"), r"C:\Applications\MesenCE\Mesen.exe"):
        if p and os.path.exists(p):
            return p
    raise SystemExit("Mesen が見つからない (--mesen か環境変数 FC_MESEN に Mesen の実行ファイルのパスを書く)")


def ensure_settings(mesen):
    p = os.path.join(os.path.dirname(os.path.abspath(mesen)), "settings.json")
    if not os.path.exists(p):
        with open(p, "w") as f:
            f.write('{ "Nes": { "Port1": { "Type": "NesController" } } }')
        print(f"fcmesen: Mesen の settings.json を作った: {p}", file=sys.stderr)


def lua_str(s):
    return "nil" if s is None else '"' + s.replace("\\", "\\\\").replace('"', '\\"') + '"'


def main():
    ap = argparse.ArgumentParser(description="fc の NES の ROM を Mesen でヘッドレスに動かし、@log と console の出力を出す")
    ap.add_argument("rom")
    ap.add_argument("--frames", type=int, default=600, help="動かすフレーム数 (既定 600 = 10 秒)")
    ap.add_argument("--until", help="この文字列を含む行 (@log・console) が出たら止める")
    ap.add_argument("--input", default="", help="パッド 1 の入力: 10-12:right,30:start+a (フレームは 1 から)")
    ap.add_argument("--peek", action="append", default=[], help="終わるときに出す変数 (mod.var、mod.var:バイト数)")
    ap.add_argument("--nametable", action="append", default=[], help="止めるときにネームテーブル N (0〜3) を出す (タイルの番号が ASCII の字なら字、"
                    "ほかは .。N:hex で 16 進)")
    ap.add_argument("--palette", action="store_true", help="止めるときにパレット ($3F00〜$3F1F) を出す")
    ap.add_argument("--png", help="止めるときの画面を書く PNG")
    ap.add_argument("--shot", action="append", default=[], help="フレーム:ファイル (そのフレームの終わりの画面を PNG に)")
    ap.add_argument("--lua", action="append", default=[], help="後ろに足す Lua のスクリプト")
    ap.add_argument("--mesen", help="Mesen の実行ファイル")
    ap.add_argument("--timeout", type=float, default=300, help="Mesen を待つ秒数")
    a = ap.parse_args()

    mesen = find_mesen(a.mesen)
    ensure_settings(mesen)
    script = fcsym.parse_input(a.input)
    peeks = fcsym.resolve(fcsym.load_symbols(a.rom), a.peek)
    nts = []
    for spec in a.nametable:
        num, _, form = spec.partition(":")
        if num not in ("0", "1", "2", "3") or form not in ("", "hex"):
            raise SystemExit(f"--nametable は 0〜3 (16 進なら 0:hex): {spec}")
        nts.append("{ n = %s, hex = %s }" % (num, "true" if form else "false"))
    shots = {}
    for spec in a.shot:
        f, _, path = spec.partition(":")
        if not f.isdigit() or not path:
            raise SystemExit(f"--shot はフレーム:ファイル: {spec}")
        shots[f] = path
    if a.png:
        shots["last"] = a.png
    body = LUA % {
        "frames": a.frames,
        "until": lua_str(a.until),
        "input": ", ".join("{ lo = %d, hi = %d, b = { %s } }" % (lo, hi, ", ".join(lua_str(b) for b in sorted(bs))) for lo, hi, bs in script),
        "peek": ", ".join("{ name = %s, addr = %d, size = %d }" % (lua_str(p.name), p.addr, p.size) for p in peeks),
        "nt": ", ".join(nts),
        "palette": "true" if a.palette else "false",
        "shots": ", ".join(("last = 1" if k == "last" else "[%s] = 1" % k) for k in shots),
    }
    fclog = os.path.splitext(a.rom)[0] + ".fclog.lua"
    if os.path.exists(fclog):
        body += "\n" + open(fclog, encoding="utf-8").read()
    for extra in a.lua:
        body += "\n" + open(extra, encoding="utf-8").read()

    saves = os.path.join(os.path.dirname(os.path.abspath(mesen)), "Saves")
    with tempfile.TemporaryDirectory(prefix="fcmesen") as tmp:
        name = "fc_run_" + os.path.basename(a.rom)
        rom = os.path.join(tmp, name)
        shutil.copyfile(a.rom, rom)
        lua = os.path.join(tmp, "run.lua")
        with open(lua, "w", encoding="utf-8") as f:
            f.write(body)
        sav = os.path.join(saves, os.path.splitext(name)[0] + ".sav")

        def drop_save():
            if os.path.exists(sav):
                os.remove(sav)

        drop_save()
        try:
            p = subprocess.run([mesen, "--testrunner", rom, lua], stdout=subprocess.PIPE, timeout=a.timeout)
        except subprocess.TimeoutExpired:
            raise SystemExit(f"fcmesen: Mesen が {a.timeout} 秒で終わらなかった")
        finally:
            drop_save()
    for l in p.stdout.decode("utf-8", "replace").replace("\r\n", "\n").split("\n"):
        if l.startswith("[png "):
            tag, _, data = l[5:].partition("] ")
            with open(shots[tag], "wb") as f:
                f.write(bytes.fromhex(data))
        elif l and not l.startswith("fc @log:"):
            print(l)
    return p.returncode


if __name__ == "__main__":
    sys.exit(main())
