#!/usr/bin/env python3
"""fcretro.py: fc の NES の ROM を libretro のコア (QuickNES) で動かし、画面を PNG に、変数の値を端末に出す。

    python3 tools/nesrun/fcretro.py game.nes [--frames 600] [--input 10-12:right,30:start]
                                             [--peek main.score] [--watch main.x] [--png out.png] [--shot 120:title.png]
                                             [--core quicknes_libretro.so]

- 画面は本物の PPU と同じく走査線ごとに描く (スクロールの分割・MMC3 の走査線の IRQ も出る)
- RAM は libretro の retro_get_memory_data / size に RETRO_MEMORY_SYSTEM_RAM (2) を渡して読む ($0000〜$07FF。$6000〜 の
  カートリッジの RAM は SAVE_RAM (0))。変数の番地と大きさは ROM の隣の .dbg (fcc build が書く) から引く
- @log と fc のランナーの番地 ($4018 / $4019) は見えない (libretro には命令ごとのフックが無い)。@log は fcmesen.py で
- コアは --core か環境変数 FC_QUICKNES (無ければ C:\\Applications\\libretro\\quicknes_libretro.dll・
  ~/.local/lib/libretro/quicknes_libretro.so)。Linux は https://github.com/libretro/QuickNES_Core を make で作れる
- Python の標準ライブラリだけ (ctypes・zlib)
"""

import argparse
import ctypes
import os
import struct
import sys
import zlib

sys.dont_write_bytecode = True  # tools/nesrun に __pycache__ を作らない
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import fcsym  # noqa: E402

ENV_SET_PIXEL_FORMAT, ENV_GET_VARIABLE = 10, 15
# コアの設定: 上下左右を切り落とさず NES の画面そのまま (256×240) にし、スプライトの 1 ラインの 8 枚の制限は本物どおり
VARIABLES = {b"quicknes_use_overscan_h": b"enabled", b"quicknes_use_overscan_v": b"enabled", b"quicknes_no_sprite_limit": b"disabled"}
FORMAT_0RGB1555, FORMAT_XRGB8888, FORMAT_RGB565 = 0, 1, 2
MEMORY_SAVE_RAM, MEMORY_SYSTEM_RAM = 0, 2
DEVICE_JOYPAD = 1
JOYPAD_ID = {"b": 0, "select": 2, "start": 3, "up": 4, "down": 5, "left": 6, "right": 7, "a": 8}

ENV = ctypes.CFUNCTYPE(ctypes.c_bool, ctypes.c_uint, ctypes.c_void_p)
VIDEO = ctypes.CFUNCTYPE(None, ctypes.c_void_p, ctypes.c_uint, ctypes.c_uint, ctypes.c_size_t)
AUDIO = ctypes.CFUNCTYPE(None, ctypes.c_int16, ctypes.c_int16)
AUDIO_BATCH = ctypes.CFUNCTYPE(ctypes.c_size_t, ctypes.c_void_p, ctypes.c_size_t)
POLL = ctypes.CFUNCTYPE(None)
INPUT = ctypes.CFUNCTYPE(ctypes.c_int16, ctypes.c_uint, ctypes.c_uint, ctypes.c_uint, ctypes.c_uint)


class Variable(ctypes.Structure):
    _fields_ = [("key", ctypes.c_char_p), ("value", ctypes.c_char_p)]


class GameInfo(ctypes.Structure):
    _fields_ = [("path", ctypes.c_char_p), ("data", ctypes.c_void_p), ("size", ctypes.c_size_t), ("meta", ctypes.c_char_p)]


class Core:
    """libretro のコア 1 つ (プロセスに 1 つだけ動かせる)。"""

    def __init__(self, path):
        self.lib = ctypes.CDLL(path)
        self.format = FORMAT_0RGB1555
        self.frame = (0, 0, 0, b"")  # 幅・高さ・pitch・画素
        self.buttons = set()
        self._values = {k: ctypes.create_string_buffer(v) for k, v in VARIABLES.items()}  # コアが指す間は生かす
        lib = self.lib
        lib.retro_load_game.restype = ctypes.c_bool
        lib.retro_load_game.argtypes = [ctypes.POINTER(GameInfo)]
        lib.retro_get_memory_size.restype = ctypes.c_size_t
        lib.retro_get_memory_size.argtypes = [ctypes.c_uint]  # 種類の id を必ず渡す (渡さないと 0 が返ることがある)
        lib.retro_get_memory_data.restype = ctypes.c_void_p
        lib.retro_get_memory_data.argtypes = [ctypes.c_uint]
        self._cbs = [ENV(self._env), VIDEO(self._video), AUDIO(lambda l, r: None), AUDIO_BATCH(lambda d, n: n),
                     POLL(lambda: None), INPUT(self._input)]
        for name, cb in zip(("environment", "video_refresh", "audio_sample", "audio_sample_batch", "input_poll", "input_state"), self._cbs):
            getattr(lib, "retro_set_" + name)(cb)
        lib.retro_init()

    def _env(self, cmd, data):
        if cmd == ENV_SET_PIXEL_FORMAT and data:
            self.format = ctypes.cast(data, ctypes.POINTER(ctypes.c_int)).contents.value
            return True
        if cmd == ENV_GET_VARIABLE and data:
            v = ctypes.cast(data, ctypes.POINTER(Variable)).contents
            if v.key in self._values:
                ctypes.cast(ctypes.addressof(v) + Variable.value.offset, ctypes.POINTER(ctypes.c_void_p)).contents.value = ctypes.addressof(self._values[v.key])
                return True
        return False

    def _video(self, data, w, h, pitch):
        if data:  # None は前のフレームのまま (frame dupe)
            self.frame = (w, h, pitch, ctypes.string_at(data, pitch * h))

    def _input(self, port, device, index, id):
        return int(port == 0 and device == DEVICE_JOYPAD and any(JOYPAD_ID[b] == id for b in self.buttons))

    def load(self, rom):
        self._rom = ctypes.create_string_buffer(rom, len(rom))
        info = GameInfo(None, ctypes.cast(self._rom, ctypes.c_void_p), len(rom), None)
        if not self.lib.retro_load_game(ctypes.byref(info)):
            raise SystemExit("コアが ROM を読み込めなかった")

    def run(self):
        self.lib.retro_run()

    def memory(self, kind):
        n = self.lib.retro_get_memory_size(kind)
        p = self.lib.retro_get_memory_data(kind)
        return ctypes.string_at(p, n) if n and p else b""

    def read(self, addr, size):
        """CPU の番地 addr から size バイト ($0000〜$1FFF は内部の RAM、$6000〜$7FFF はカートリッジの RAM)。"""
        if addr < 0x2000:
            ram, base = self.memory(MEMORY_SYSTEM_RAM), addr & 0x7FF
        elif 0x6000 <= addr < 0x8000:
            ram, base = self.memory(MEMORY_SAVE_RAM), addr - 0x6000
        else:
            raise SystemExit(f"${addr:04X} は RAM ではないので読めない")
        if base + size > len(ram):
            raise SystemExit(f"${addr:04X} は読めない (コアの RAM は {len(ram)} バイト)")
        return ram[base:base + size]

    def png(self, path):
        w, h, pitch, buf = self.frame
        if not w:
            raise SystemExit("まだ画面が無い")
        rows = []
        for y in range(h):
            row = bytearray(b"\0")
            line = buf[y * pitch:]
            for x in range(w):
                if self.format == FORMAT_XRGB8888:
                    b, g, r = line[4 * x], line[4 * x + 1], line[4 * x + 2]
                else:
                    v = line[2 * x] | line[2 * x + 1] << 8
                    if self.format == FORMAT_RGB565:
                        r, g, b = (v >> 11 & 31) * 255 // 31, (v >> 5 & 63) * 255 // 63, (v & 31) * 255 // 31
                    else:
                        r, g, b = (v >> 10 & 31) * 255 // 31, (v >> 5 & 31) * 255 // 31, (v & 31) * 255 // 31
                row += bytes((r, g, b))
            rows.append(bytes(row))

        def chunk(kind, data):
            return struct.pack(">I", len(data)) + kind + data + struct.pack(">I", zlib.crc32(kind + data))

        with open(path, "wb") as f:
            f.write(b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", struct.pack(">IIBBBBB", w, h, 8, 2, 0, 0, 0)) +
                    chunk(b"IDAT", zlib.compress(b"".join(rows))) + chunk(b"IEND", b""))


def find_core(arg):
    for p in (arg, os.environ.get("FC_QUICKNES"), r"C:\Applications\libretro\quicknes_libretro.dll",
              os.path.expanduser("~/.local/lib/libretro/quicknes_libretro.so")):
        if p and os.path.exists(p):
            return p
    raise SystemExit("QuickNES のコアが見つからない (--core か環境変数 FC_QUICKNES に quicknes_libretro.so / .dll のパスを書く)")


def value(core, sym):
    b = core.read(sym.addr, sym.size)
    return int.from_bytes(b, "little"), b.hex(" ")


def main():
    ap = argparse.ArgumentParser(description="fc の NES の ROM を QuickNES (libretro) で動かし、画面と変数を出す")
    ap.add_argument("rom")
    ap.add_argument("--frames", type=int, default=600, help="動かすフレーム数 (既定 600 = 10 秒)")
    ap.add_argument("--input", default="", help="パッド 1 の入力: 10-12:right,30:start+a (フレームは 1 から)")
    ap.add_argument("--peek", action="append", default=[], help="終わるときに出す変数 (mod.var、mod.var:バイト数)")
    ap.add_argument("--watch", action="append", default=[], help="フレームの終わりごとに見て、変わったら出す変数")
    ap.add_argument("--png", help="最後の画面を書く PNG")
    ap.add_argument("--shot", action="append", default=[], help="フレーム:ファイル (そのフレームの終わりの画面を PNG に)")
    ap.add_argument("--core", help="libretro のコア (quicknes_libretro.so / .dll)")
    a = ap.parse_args()

    syms = fcsym.load_symbols(a.rom)
    peeks, watches = fcsym.resolve(syms, a.peek), fcsym.resolve(syms, a.watch)
    script = fcsym.parse_input(a.input)
    shots = {}
    for s in a.shot:
        f, _, path = s.partition(":")
        shots[int(f)] = path
    core = Core(find_core(a.core))
    core.load(open(a.rom, "rb").read())
    last = {}
    for frame in range(1, a.frames + 1):
        core.buttons = fcsym.buttons_at(script, frame)
        core.run()
        for w in watches:
            v = value(core, w)
            if last.get(w.name) != v[0]:
                print(f"[{frame}] {w.name} = {v[0]} (0x{v[1]})")
                last[w.name] = v[0]
        if frame in shots:
            core.png(shots[frame])
    for p in peeks:
        v = value(core, p)
        print(f"[peek] {p.name} = {v[0]} (0x{v[1]})")
    if a.png:
        core.png(a.png)
    return 0


if __name__ == "__main__":
    sys.exit(main())
