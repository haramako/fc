# games

## Purpose

fc のデバッグを兼ねた小さなゲーム（fc 4、nes ターゲット）。実際のゲームの形のコードでコンパイラと fclib を使い、バグと
fclib の足りないところを見つける。`examples/`（実プロジェクト由来の回帰サンプル・ライブラリの例）とは別。

## Ownership

- `common/` … ゲームで共通に使う CHR（`tiles.txt` が元、`tiles.chr`・`tiles.fc`・`tiles.png` は `tools/chrgen` の生成物）と、
  共通のライブラリ（fclib に入れるほどではないもの）
- `<名前>/` … ゲーム 1 本（`main.fc` と fc.toml）

## Local Contracts

- `common/tiles.{chr,fc,png}` は手で書き換えない。`common/tiles.txt` を直して `go run ./tools/chrgen games/common/tiles.txt`
  （書き方は `tools/chrgen/main.go` の頭。色の番号は 0 透明・1 輪郭・2 本体・3 ハイライト）
- CHR の配置: BG は $0000（$20〜$7F が ASCII の文字。16×16 は n, n+1, n+16, n+17）、スプライトは $1000（8×8 のモードは
  `nes.CTRL_SPR_1000`。16×16 は左上・左下・右上・右下の n..n+3 で、8×16 のモードでもそのまま使える）
- ゲームを作っていて見つけたコンパイラのバグは、コンパイラを直して回帰テストを足す（fuzz で見つけたバグと同じ扱い。
  `Agent/discussions/2026-09-19-fuzz-findings-log.md`）。fclib の足りないところは `Agent/discussions/` に記録して相談する
- 公開のリポジトリなので、castle など非公開のゲームの絵・テキストを持ち込まない

## Work Guidance

- 候補と作る順は `Agent/discussions/2026-10-08-debug-games.md`

## Verification

- `go test ./tools/chrgen`（生成物が tiles.txt と合っている・tiles.fc を使うプログラムがビルドできる）
