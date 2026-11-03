# デバッグ

値を見る `@log`、Mesen でソースの行を見ながらのステップ実行、Mesen と libretro で画面なしに動かす道具、`sys.panic` で止まったときの理由の見方。

## ビルドが書くファイル {#files}

`fcc build` は ROM の隣に次のファイルを書く（`game.nes` なら `game.dbg` など）。

| ファイル | いつ | 中身 |
|---|---|---|
| `.dbg` | いつも | ld65 のデバッグ情報（fc のソースの行・シンボル・セグメント）。Mesen がソースの行を出すのに使う |
| `.map` | いつも | ld65 のマップ（セグメントの番地と大きさ） |
| `.mlb` | `-g` と NES | Mesen のラベル（関数・変数の名前） |
| `.fclog.json` / `.fclog.lua` | `-g` で `@log` があるとき | `@log` の地点と値の場所 / それを Mesen で出すスクリプト |

## @log {#log}

`@log` は、そこを通ったときの値をエミュレータの側で出す。ROM に出す命令は足さない（地点はラベル、値はそのときのメモリと
レジスタから読む）。書式は `@printf` と同じ（[書式](./language#format)。寄せ方の `<` / `>` は使えない）。

```fc
@log("x={} y={} hp={}", x, y, hp);
```

- emu: `fcc run -g main.fc` で、`@printf` の出力と同じ順に端末へ出る
- NES: `fcc build -t nes -g -o game.nes main.fc` で書いた `game.fclog.lua` を、Mesen で `game.nes` を開いてから
  スクリプトのウィンドウ（Debug > Script Window）で読み込んで動かす。出力はスクリプトのウィンドウのログに出る。
  地点は番地で覚えているので、ROM と一緒に書いた `.fclog.lua` を使う（ROM を作り直したらスクリプトも読み込み直す）。
  画面なしで端末に出すなら [fcmesen.py](#mesen-headless)
- `-g` を付けなければ `@log` は何もしない。`fcc run -t nes`（内蔵の NES のランナー）は `@log` を出さない

## Mesen でソースを見る {#mesen}

`-g` で NES の ROM を作ると、Mesen は ROM を開くときに同じ名前の `.dbg` と `.mlb` を読む。デバッガに関数と変数の名前が付き、
`.fc` の行でブレークポイントを置いてステップ実行できる。`.dbg` はソースの場所を ROM の隣からの相対のパスで持つので、ROM か
ソースを動かしたら作り直す。

## Mesen をヘッドレスで動かす {#mesen-headless}

[tools/nesrun/fcmesen.py](https://github.com/haramako/fc/blob/main/tools/nesrun/fcmesen.py) は、Mesen を画面なしで動かし
（`--testrunner`）、`@log`・console の出力・`sys.panic` の理由を端末に出す。Python 3 の標準ライブラリだけで動く。Mesen は
[MesenCE](https://github.com/nesdev-org/MesenCE/releases) の Windows 版と Linux 版で動く（Linux は画面の無い所でも動く）。

```bash
fcc build -t nes -g -o game.nes main.fc
python3 tools/nesrun/fcmesen.py game.nes --frames 600 --input 60:start,120-180:right+a --peek main.score
```

| オプション | 意味 |
|---|---|
| `--frames N` | N フレーム動かして止める（既定 600 = 10 秒） |
| `--until 文字列` | その文字列を含む行（`@log`・console）が出たら止める |
| `--input 台本` | パッド 1 の入力。`フレーム:ボタン` をコンマで並べる（`10-12:right`、`30:start+a`。フレームは 1 から） |
| `--peek mod.var` | 止めるときに変数の値を出す（大きさは `.dbg` から。`mod.var:バイト数` で変えられる。何度でも） |
| `--lua ファイル` | 後ろに足す Lua のスクリプト（Mesen の Lua の API が使える） |
| `--mesen パス` | Mesen の実行ファイル（環境変数 `FC_MESEN` でもよい） |

- ROM の隣の `.fclog.lua`（`-g`）があれば読み込むので、`@log` がそのまま出る
- `$4019` に書いたら（`sys.panic`・`console.exit`）その値で終わる。`--until` で止まれば 0、フレームが尽きれば 0
  （`--until` を書いたときは 2）
- 初めてのとき、Mesen の隣に最小の `settings.json` を作る（無いと初回のダイアログで止まり、コントローラが無い扱いで入力が効かない）
- Mesen は ROM の名前でセーブデータを残すので、ROM を写した別の名前で動かし、そのセーブを前後で消す
- Mesen はプロセスに 1 つだけ動く。手で動かしている Mesen があると（Windows）動かない

## libretro で動かす {#libretro}

[tools/nesrun/fcretro.py](https://github.com/haramako/fc/blob/main/tools/nesrun/fcretro.py) は、libretro の QuickNES のコアで
ROM を動かし、画面を PNG に、変数の値を端末に出す。本物の PPU と同じく走査線ごとに描くので、スクロールの分割や MMC3 の
走査線の IRQ も画面に出る。Mesen より速い（1 秒に数千フレーム）ので、多くの入力を試すのに向く。

```bash
python3 tools/nesrun/fcretro.py game.nes --frames 300 --input 60:start --watch main.x --png last.png --shot 120:title.png
```

- `--frames`・`--input`・`--peek` は fcmesen.py と同じ。`--watch mod.var` はフレームの終わりごとに見て、変わったら出す。
  `--png` は最後の画面、`--shot フレーム:ファイル` はそのフレームの画面を PNG にする
- コアは `--core` か環境変数 `FC_QUICKNES`（`quicknes_libretro.so` / `.dll`）。Linux は
  [QuickNES_Core](https://github.com/libretro/QuickNES_Core) を `make` で作れる
- RAM は `retro_get_memory_data` / `retro_get_memory_size` に `RETRO_MEMORY_SYSTEM_RAM`（2）を渡して読む（$0000〜$07FF。
  カートリッジの RAM（$6000〜）は `RETRO_MEMORY_SAVE_RAM`（0））。自分で libretro を呼ぶときも種類の id を渡す
- libretro には命令ごとに止める仕組みが無いので、`@log` と `sys.panic` の理由は出ない（fcmesen.py を使う）。止まったかは
  `--peek sys.msg`（長さが 0 でない）で分かる

## panic と assert {#panic}

`sys.panic(msg)`・`sys.assert`・`@assert` は、理由を残して止まる。`@format` の書き先が足りないときなど、標準ライブラリも使う。

- emu: `panic: 理由` を出して終了コード 1 で終わる
- NES: 理由を `sys.msg`（`[:u16]const u8`。番地 2 バイトと長さ 2 バイト）に残し、NMI を止めてその場で止まる。画面はそのまま
  （画面に文字を出すための console を ROM に入れないため）。Mesen のメモリのウィンドウで `_sys_msg` の番地を見れば、理由の
  文字列の番地と長さが分かる。`fcc test -t nes` と `fcc run -t nes` では `panic: 理由` が端末に出る
