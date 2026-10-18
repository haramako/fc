# Issues

## Purpose

multica（エージェント向け Issue トラッキングシステム）や GitHub Issue など、**Issue ごとの作業記録**を蓄積する。

## Local Contracts

- 1 Issue につき1ファイル。ファイル名は `<source>-<id>-<slug>.md`（`<source>` は `multica` / `gh` 等、`<id>` は Issue 番号、`<slug>` は内容を表す kebab-case。例: `gh-123-pathfinder-crash.md`、`multica-45-turn-order.md`）
- 推奨内容: Issue へのリンク／概要／調査メモ／対応内容／関連コミット・PR／状態
- 作業の途中経過・調査ログを残す場所。恒久的な知識になったら `../wiki/` または該当ディレクトリの `AGENTS.md` へ昇格させる
- ファイルを追加・状態変更したら、この `AGENTS.md` の **Index** を更新する

## Index

- [`local-2026-10-18-minmax-a1-width.md`](local-2026-10-18-minmax-a1-width.md) — `@min` / `@max` / `@clamp` が A1 の幅を受け取らず 8 ビットで折り返す — **未修正**
- [`local-2026-10-18-dead-compare.md`](local-2026-10-18-dead-compare.md) — 結果を使わない比較が残る・`x & 0` と定数の switch を畳まない（最適化の取りこぼし。出力は正しい） — **未修正**
- [`local-2026-10-18-mixed-sign-false-error.md`](local-2026-10-18-mixed-sign-false-error.md) — F6 の符号の混ざりのエラーが 8 ビットの式に対して誤検出する（`a - (b >> 2)` が通らない） — **未修正**
