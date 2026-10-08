# games

## Purpose

fc のデバッグを兼ねた小さなゲーム（fc 4、nes ターゲット）。実際のゲームの形のコードでコンパイラと fclib を使い、バグと
fclib の足りないところを見つける。`examples/`（実プロジェクト由来の回帰サンプル・ライブラリの例）とは別。

## Ownership

- `common/` … ゲームで共通に使う CHR（`tiles.txt` が元、`tiles.chr`・`tiles.fc`・`tiles.png` は `tools/chrgen` の生成物）と、
  共通のライブラリ（fclib に足りない所を補うもの。終わってから fclib に移すかを決める）: `gfx.fc`（16×16 の BG・8×8 / 8×16 の
  モードの 16×16 のスプライト）、`text.fc`（文字の窓）、`bitmap.fc`（CHR RAM の線画）
- `<名前>/` … ゲーム 1 本（`main.fc` と fc.toml。fc.toml は `[lib.common] path = "../common"`）: breakout・snake・mines・sokoban・
  blocks・shooter・chase・adventure・platform・rogue・wire

## Local Contracts

- `common/tiles.{chr,fc,png}` は手で書き換えない。`common/tiles.txt` を直して `go run ./tools/chrgen games/common/tiles.txt`
  （書き方は `tools/chrgen/main.go` の頭。色の番号は 0 透明・1 輪郭・2 本体・3 ハイライト）
- CHR の配置: BG は $0000（$20〜$7F が ASCII の文字。16×16 は n, n+1, n+16, n+17）、スプライトは $1000（8×8 のモードは
  `nes.CTRL_SPR_1000`。16×16 は左上・左下・右上・右下の n..n+3 で、8×16 のモードでもそのまま使える）
- ゲームを作っていて見つけたコンパイラのバグは、コンパイラを直して回帰テストを足す（fuzz で見つけたバグと同じ扱い。
  `Agent/discussions/2026-09-19-fuzz-findings-log.md`）。fclib の足りないところは `Agent/discussions/` に記録して相談する
- 公開のリポジトリなので、castle など非公開のゲームの絵・テキストを持ち込まない
- どのゲームも `const AUTO = false @(build);`（true で自分で遊ぶ。テストは `-D main.AUTO=true`）と、メインループの終わりに呼ぶ
  `function end_frame():void @(noinline) { ticks += 1; frame.wait(); }` を持つ（テストが手数 `ticks` で止めて -O 0 と -O 2 を比べる）。
  AUTO の判断もゲームの論理も手数ごとに決まるように書く（`frame.count` など、かかったフレーム数で変わるものを使わない）
- 静的フレームの RAM の予約の既定 512 バイトでは RAM が足りないことが多い。`@(static_ram: N)` を main に書く（`fcc build -d` の
  `FC_SRAM` が実際に使う量）
- CHR RAM のゲーム（wire）は fc.toml に `chr = 0` と `[define.tiles] CHR_ROM = false`（tiles の定数だけ使い、絵は `@incbin` で写す）

## Work Guidance

- 候補は `Agent/discussions/2026-10-08-debug-games.md`、作って見つかったことは `Agent/discussions/2026-10-08-debug-games-findings.md`
- ゲームを足したら `internal/nes/games_test.go` に AUTO のテストと `TestGamesSameAtTick` の行を足す

## Verification

- `go test ./tools/chrgen`（生成物が tiles.txt と合っている・tiles.fc を使うプログラムがビルドできる）
- `go test ./internal/nes -run 'TestGame'`（AUTO で遊んで進むこと・vblank に収まること・手数を決めた所の -O 0 / -O 2 の一致。
  `FC_GAMES_PNG_DIR=dir` で最後の画面を書く。内蔵のランナーはスクロールを無視して描く）
