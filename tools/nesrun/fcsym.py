"""fcmesen.py / fcretro.py の共通部分: ld65 の .dbg からシンボルを引く・入力の台本を読む。"""

import os
import re

BUTTONS = ("a", "b", "select", "start", "up", "down", "left", "right")


class Symbol:
    def __init__(self, name, addr, size):
        self.name, self.addr, self.size = name, addr, size


def dbg_path(rom):
    """ROM の隣の .dbg (fcc build が書く)。"""
    return os.path.splitext(rom)[0] + ".dbg"


def load_symbols(rom):
    """ROM の隣の .dbg のシンボル (名前 → Symbol)。無ければ空。"""
    p = dbg_path(rom)
    if not os.path.exists(p):
        return {}
    syms = {}
    for line in open(p, encoding="utf-8", errors="replace"):
        if not line.startswith("sym\t"):
            continue
        f = dict(kv.split("=", 1) for kv in line.rstrip("\n").split("\t", 1)[1].split(","))
        if "val" not in f or "name" not in f:
            continue
        name = f["name"].strip('"')
        syms[name] = Symbol(name, int(f["val"], 16), int(f.get("size", "1")))
    return syms


def mangle(name):
    """fc の `mod.var` をアセンブラのシンボル `_mod_var` に (`_` で始まればそのまま)。"""
    if name.startswith("_") or "." not in name:
        return name
    mod, var = name.rsplit(".", 1)
    return "_" + mod.replace("/", "_") + "_" + var


def resolve(syms, names):
    """--peek の名前の並びを Symbol に (`名前:大きさ` で大きさを変えられる)。見つからなければ SystemExit。"""
    out = []
    for spec in names:
        name, _, size = spec.partition(":")
        sym = syms.get(mangle(name))
        if sym is None:
            raise SystemExit(f"{name}: シンボルが ROM の隣の .dbg に無い ({mangle(name)})")
        out.append(Symbol(name, sym.addr, int(size) if size else sym.size))
    return out


def parse_input(spec):
    """入力の台本 `10-12:right,30:start+a` を [(最初のフレーム, 最後のフレーム, ボタンの集合)] に (フレームは 1 から)。"""
    out = []
    if not spec:
        return out
    for item in spec.split(","):
        frames, _, buttons = item.strip().partition(":")
        m = re.fullmatch(r"(\d+)(?:-(\d+))?", frames)
        if not m or not buttons:
            raise SystemExit(f"入力の台本が読めない: {item!r} (例: 10-12:right,30:start+a)")
        names = set(buttons.split("+"))
        bad = names - set(BUTTONS)
        if bad:
            raise SystemExit(f"知らないボタン: {', '.join(sorted(bad))} ({', '.join(BUTTONS)})")
        lo = int(m.group(1))
        out.append((lo, int(m.group(2) or lo), names))
    return out


def buttons_at(script, frame):
    """フレーム frame に押しているボタンの集合。"""
    on = set()
    for lo, hi, names in script:
        if lo <= frame <= hi:
            on |= names
    return on
