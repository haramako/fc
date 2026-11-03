---
name: fc-nes-game
description: fc 言語 (fcc コンパイラ) で NES (ファミコン) のゲームを作る・直す・デバッグするときに使う。.fc のソース、fc.toml、fcc build / run / test、vram・frame・pad・oam などの fclib、@log、画面に出ない・止まる・値がおかしいときの調べ方。
---

# fc で NES のゲームを作る

細かい仕様はここに書かない。必ず次で調べる（fc は更新されるので、覚えている知識より優先する）。

- 標準ライブラリの API: `fcc doc`（一覧）、`fcc doc vram`（モジュール）、`fcc doc vram.put`（関数）。手元の fcc の版と一致する
- 言語仕様・使い方: https://haramako.github.io/fc/ （言語は reference/language、デバッグは reference/debug）
- エージェント向けの目次: https://haramako.github.io/fc/llms.txt

## 始めるとき

- 最初のプログラムは https://haramako.github.io/fc/start/hello-nes の形を写して始める（初期化・描画・毎フレームの処理の順）
- マッパーなし（NROM。PRG 32K・CHR 8K）なら fc.toml は要らない（`-t nes` でビルドする）。マッパーを使うときだけ fc.toml を書く:
  ```toml
  [target]
  mapper = "MMC3"
  prg = "32K"
  chr = "8K"
  ```

## 作業の手順（毎回この順で）

1. 変更は小さく 1 つずつ。変えたらすぐ `fcc build -t nes -g -o build/game.nes src/main.fc` でビルドする（`-g` が無いと `@log` は
   何も出さない。`-o` の先のフォルダは fcc が作る）
2. エラーが出たら、エラーの文言をそのまま読む（多くは直し方が書いてある）。分からない関数は `fcc doc 名前` で引く
3. 動きは「デバッグの手順」の道具で確かめる。画面を見ずに「直った」と言わない
4. ロジックの単体テストは `@(test)` の関数を書いて `fcc test -t nes src/x.fc`
5. 書式を整えるのは `fcc fmt -w src/main.fc`（`-w` が無いと整えた結果を表示するだけでファイルは変わらない）

## まちがえやすい所（先に読む）

- ソースの 1 行目は `#fc 4`
- 書式は `{}` で書く。C の `%d` / `%02X` は使えない: `@format(buf, "{:02X}", c)`、`@printf("{} {}\n", x, y)`
- `switch` は次の case へ続かない。複数の値は `case 1, 2:`。空の `case` は何もしない
- u8 どうしの足し算は 8 ビットで折り返す。比較の中でも同じ: `if (y + 96 < next)` は y が 160 以上で壊れる。`next - y > 96` のように
  書き直すか `(y as u16) + 96` にする
- ローカル変数は 0 で始まらない（初期値を書く）。グローバル変数は 0 で始まる
- `use mod;` はソースのフォルダ → fc.toml の `[lib.名前]` → 標準ライブラリの順に探す。別のフォルダのモジュールは fc.toml に
  `[lib.gfx]` と `path = "gfx"` を書く（写さない）
- **描画中（`frame.render_on()` の後）に PPU のレジスタ（`nes.PPUADDR` / `PPUDATA` / `PPUSCROLL`）を触らない。読み戻しもしない。**
  画面がずれ、書いていない所に文字が出る。描画中の書き込みは `vram.put`（キューに積み、`frame.wait()` の NMI で送る）。
  `vram.write_now` は描画を止めているときだけ
- 1 フレームに送れる量には限りがある（キューと手間の予算）。大きな書き込みは何フレームかに分かれる
- 割り込み（NMI・IRQ）の中身はアセンブリで書く（reference/assembly の「割り込みから呼ぶ関数」）
- MMC3 で画面を横の帯に分けてスクロールを変える（HUD を止める・帯ごとに流す）なら、IRQ を自分で書かずに `raster` モジュールを使う
  （`fcc doc raster`、reference/raster の「raster モジュールで分ける」）
- 切り替えるバンクの `const` の表は、別のバンクからは読めない（reference/banks の「別のバンクのデータ」）

## デバッグの手順

道具は fc のリポジトリの `tools/nesrun/`（Python 3 の標準ライブラリだけ）。無ければプロジェクトに取ってくる:

```bash
mkdir -p tools/nesrun
for f in fcsym.py fcmesen.py fcretro.py; do
  curl -fsSL -o tools/nesrun/$f https://raw.githubusercontent.com/haramako/fc/main/tools/nesrun/$f
done
```

症状ごとに次を使う。ROM は `-g` を付けて作る。

| 知りたいこと | コマンド |
|---|---|
| 変数の値の移り変わり | ソースに `@log("x={} y={}", x, y);` を書き（引数は変数・定数・フィールドだけ。式はいったん変数に入れる）、`python3 tools/nesrun/fcmesen.py build/game.nes --frames 300` |
| 書いた文字が画面に入ったか | `python3 tools/nesrun/fcmesen.py build/game.nes --frames 300 --nametable 0` |
| 止まった理由 (panic) | `fcmesen.py` が `panic: 理由` と `[exit 1]` を出す |
| 終わりの時点の変数 | `--peek main.score`（`モジュール.変数`） |
| 画面の見た目 | `python3 tools/nesrun/fcretro.py build/game.nes --frames 300 --png shot.png` で PNG を見る |
| ボタンを押す | `--input 60:start,120-180:right+a`（フレームは 1 から） |

- fcmesen.py は Mesen 2（MesenCE）が要る: `--mesen パス` か環境変数 `FC_MESEN`。画面の無い Linux でも動く
- fcretro.py は QuickNES の libretro コアが要る: `--core パス` か環境変数 `FC_QUICKNES`
- ネームテーブルに文字が入っているのに画面に出ないなら、スクロール・属性・パレット・CHR を疑う。入っていなければ、書く側
  （番地・長さ・呼ぶ時機）を疑う
- 自分で libretro を呼ぶ道具を書かない（RAM の種類の id の渡し忘れ、画面の端の切り落としで誤った結論になる）

## コンパイラの不具合かと思ったら

1. 最小のプログラム（数十行）で再現させる。再現しなければ、ゲームの側の書き方を疑う
2. 最小のプログラムで、仕様（reference/language）と比べる
3. 報告には `fcc version` の出力、最小のプログラム、期待した結果と実際の結果を書く（https://github.com/haramako/fc/issues）
