# ターゲット

`fcc` は 2 つのターゲットに出力する。`-t nes` / `-t emu` で選ぶ。省くと、fc.toml に `[target]` があれば nes、無ければ emu。

| ターゲット | 出力 | 動かし方 |
|---|---|---|
| `nes` | NES の ROM（iNES 形式の `.nes`） | NES のエミュレータか実機。`fcc run -t nes` は内蔵の NES のランナーで `console` の出力だけを見る |
| `emu` | 6502 のバイナリ（`.bin`） | `fcc run`（`fcc` の内蔵の 6502 のエミュレータ） |

## emu

画面もコントローラも無い、6502 と RAM だけの計算機。言語や標準ライブラリの動きを確かめたり、NES に依らない処理をテストしたりするのに使う。
`fcc test` は既定でこのターゲットで走らせる。

- 出力は `console`（`printf` と `@assert` の出力先）で、`fcc run` が端末に出す
- `main` から戻るか `console.exit(コード)` を呼ぶと終わる。`fcc run` の終了コードはそのコード（`main` から戻れば 0）
- `console.bench_start()` と `console.bench_end()` で囲んだ区間のサイクル数を数えられる

## nes

- 電源を入れると、RAM（`$0000`〜`$07FF`）を 0 にしてから `main` を呼ぶ。`main` から戻ると止まったままになる（ゲームは `main` の中で回り続ける）
- vblank の割り込み（NMI）は標準ライブラリの `frame` が持つ。`frame.wait` でフレームを進め、スプライトと VRAM の書き込みはその NMI で送る
- マッパーと ROM の大きさは fc.toml の `[target]` で決める（[fc.toml](./fc-toml)）。NROM / UxROM / MMC1 / MMC3 を使える
- CHR（タイルの絵）は `@include("tiles.chr");` で ROM に入れる。標準ライブラリの内蔵のフォント `font.chr` はタイルの番号が ASCII の文字コードと同じ
- `fcc run -t nes` は内蔵の NES のランナーで `console.exit` まで走らせ、`console` の出力を出す（画面は描かない。1 分で終わらなければエラー）。
  画面を見るにはエミュレータで開く

RAM の番地と初期化の範囲は[メモリ配置](./memory)、バンクをまたぐ呼び出しは[バンクと far call](./banks)を参照。
