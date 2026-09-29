# fcc コマンド

```bash
fcc <コマンド> [オプション] <ファイル> ...
```

`fcc` だけで使い方の一覧を、`fcc <コマンド> -h` でそのコマンドの使い方を出す。

## コマンド

| コマンド | 働き |
|---|---|
| `build`（`b`） | ビルドして ROM（`-t nes`）か emu のバイナリを作る |
| `run` | ビルドして動かす。emu は内蔵のエミュレータで、NES は内蔵の NES のランナーで `console` の出力だけを見る |
| `compile`（`c`） | アセンブルまで（リンクしない） |
| `check` | ファイルを作らずにコンパイルし、エラーと警告を出す |
| `test` | モジュールの `@(test)` の関数を走らせる |
| `fmt` | ソースを整形する |
| `doc` | モジュールの public の宣言と説明を出す |
| `lib` | fc.toml の `[lib.*]` のライブラリを取ってくる・更新する・並べる |
| `watch` | ソースが変わるたびにビルドし直す |
| `size` | ld65 のデバッグ情報から関数ごとの大きさを出す |
| `migrate` | 古い版のソースを今の fc に書き換える |
| `version` | 版と、使う `ca65` / `ld65` の場所を出す |

## build / run / compile

```bash
fcc build [オプション] main.fc
```

| オプション | 意味 |
|---|---|
| `-t nes` / `-t emu` | ターゲット。省くと、fc.toml に `[target]` があれば nes、無ければ emu |
| `-o FILE` | 出力のファイル（既定は `a.nes` か `a.bin`） |
| `-O LEVEL` | 最適化の段階（0〜2。既定は 2） |
| `-g` | Mesen 用のデバッグ情報（ROM の隣に `.dbg` と `.mlb`）を書く |
| `--size-report` | セグメントと関数ごとのコードの大きさを出す |
| `-D MOD.NAME=VAL` | モジュール MOD の `@(build)` の定数 NAME の値を変える（何度でも。fc.toml の `[define.MOD]` の後に当てる） |
| `-d` | 静的フレームの配置と far call を出す |
| `--offline` | fc.toml の git のライブラリを取ってこない（キャッシュに無ければエラー） |
| `-e` | ビルドの後に動かす（`run` と同じ） |

ビルドの途中のファイルは、ソースのディレクトリの `.fc-build/` に置く。

## check

```bash
fcc check [-t TARGET] [--json] [-D MOD.NAME=VAL] main.fc
```

エラーは `ファイル:行:桁: error: …`、警告は `ファイル:行:桁: warning: …` の形で出す。`--json` なら 1 行に 1 つの JSON
（`file`・`line`・`col`・`severity`・`message`）で出す（エディタの拡張が使う）。

## test

```bash
fcc test [-t emu|nes] [-O LEVEL] module.fc ...
```

与えたモジュールの `@(test)` の関数を順に走らせ、`モジュール.関数: ok` と出す。全部通れば終了コード 0、失敗すればそこで止まって 1。
`-t nes` なら内蔵の NES のランナーで走らせる。

## fmt

```bash
fcc fmt [-l] [-w] [-d] file.fc ...
```

整形した結果を標準出力に出す。`-w` はファイルを書き換え、`-l` は書式の違うファイルの名前だけを出し、`-d` は差分を出す。

## doc

```bash
fcc doc                  # 標準ライブラリのモジュールの一覧
fcc doc vram             # モジュール (nes/vram のようにターゲットを付けてもよい)
fcc doc vram.put         # 宣言 1 つ
fcc doc game.fc          # 自分のファイル (game.fc:name で宣言 1 つ)
fcc doc -md DIR          # 標準ライブラリのページ (このサイトの「標準ライブラリ」) を DIR に書く
```

`-t nes` / `-t emu` で、そのターゲットのモジュールだけにする。説明は、宣言の直前のコメント（モジュールの説明はファイルの頭のコメント）から作る。

## lib

```bash
fcc lib fetch            # fc.lock のとおりにライブラリを揃える (build でも取ってくる)
fcc lib update [名前...]  # git のライブラリを rev の今のコミットに進めて fc.lock を書き直す
fcc lib list             # ライブラリと、使っているコミットを並べる
fcc lib add 名前 場所     # fc.toml に [lib.名前] を足して取ってくる (場所はフォルダか git の URL。-rev REV・-dir DIR)
```

`-C DIR` で fc.toml を探し始めるディレクトリを変える。

## watch

```bash
fcc watch [-t TARGET] [-o FILE] [-O LEVEL] [-g] [-c] main.fc
```

ソースのディレクトリと標準ライブラリのファイルが変わるたびにビルドし直す。Ctrl-C で止める。

## size

```bash
fcc size [-n N] game.dbg
```

ld65 の `--dbgfile` が書いたデバッグ情報から、セグメントごとの合計と、大きい順に N 個（既定 40、0 なら全部）の関数の大きさを出す。
`fcc` がリンクするなら `fcc build --size-report` でよい。

## 環境変数

| 変数 | 意味 |
|---|---|
| `FC_CC65_BIN` | `ca65` / `ld65` のあるディレクトリ。無ければ `fcc` と同じディレクトリ、次にパスの通った場所を探す |
| `FC_HOME` | 標準ライブラリ（`fclib/`）とランタイム（`share/`）のあるディレクトリ。無ければ `fcc` の場所とカレントディレクトリから上へ探し、見つからなければ `fcc` が中に持っているものを使う |
| `FC_CACHE_DIR` | `fcc` が中に持っている標準ライブラリを展開する場所（既定はユーザーのキャッシュのディレクトリの `fc`） |
| `FC_LIB_CACHE` | git のライブラリを取ってくる場所（既定はユーザーのキャッシュのディレクトリの `fc/lib`） |
| `FC_NO_ASM_CACHE` | 1 なら、変わっていないアセンブリも毎回アセンブルし直す |
