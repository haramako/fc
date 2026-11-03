# デバッグ

値を見る `@log`、Mesen でソースの行を見ながらのステップ実行、`sys.panic` で止まったときの理由の見方。

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
  地点は番地で覚えているので、ROM と一緒に書いた `.fclog.lua` を使う（ROM を作り直したらスクリプトも読み込み直す）
- `-g` を付けなければ `@log` は何もしない。`fcc run -t nes`（内蔵の NES のランナー）は `@log` を出さない

## Mesen でソースを見る {#mesen}

`-g` で NES の ROM を作ると、Mesen は ROM を開くときに同じ名前の `.dbg` と `.mlb` を読む。デバッガに関数と変数の名前が付き、
`.fc` の行でブレークポイントを置いてステップ実行できる。`.dbg` はソースの場所を ROM の隣からの相対のパスで持つので、ROM か
ソースを動かしたら作り直す。

## panic と assert {#panic}

`sys.panic(msg)`・`sys.assert`・`@assert` は、理由を残して止まる。`@format` の書き先が足りないときなど、標準ライブラリも使う。

- emu: `panic: 理由` を出して終了コード 1 で終わる
- NES: 理由を `sys.msg`（`[:u16]const u8`。番地 2 バイトと長さ 2 バイト）に残し、NMI を止めてその場で止まる。画面はそのまま
  （画面に文字を出すための console を ROM に入れないため）。Mesen のメモリのウィンドウで `_sys_msg` の番地を見れば、理由の
  文字列の番地と長さが分かる。`fcc test -t nes` と `fcc run -t nes` では `panic: 理由` が端末に出る
