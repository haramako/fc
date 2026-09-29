# Discussions

## Purpose

エージェントとユーザーの会話における**相談内容・決定事項・経緯**を歴史ログとして蓄積する。Wiki や各 `AGENTS.md`（＝「現在の正しい状態」を書く場所）には載らない、**「なぜそうなったか」という歴史的事項**を残す。

## Local Contracts

- 1トピックにつき1ファイル。ファイル名は `YYYY-MM-DD-<slug>.md`（`<slug>` は内容を表す kebab-case。例: `2026-06-21-dox-tree-bootstrap.md`）
- 日付はその相談・決定が行われた日
- 推奨内容: 背景／相談したこと／選択肢と検討／**決定事項**／理由
- **追記型・不変**: 過去ログは原則書き換えない。方針が変わったら新しい日付のファイルを追加する
- 決定がコードや運用の現在ルールに昇格する場合は、該当する `AGENTS.md`（または `../wiki/`）にも反映し、ここには経緯として残す
- ファイルを追加したら、この `AGENTS.md` の **Log** インデックスを更新する（新しい順）

## Log

- 2026-09-30 [VS Code 拡張を 1 つにする](2026-09-30-vscode-extension.md) — tools/vscode-fc に整形を移し、古い editors/vscode を消した
- 2026-09-30 [開発を main に移す](2026-09-30-move-to-main.md) — 旧 main に v0.0.3 のタグ、feature/v4 を main にマージ。go install …@main が使える
- 2026-09-30 [else if と elsif](2026-09-30-else-if-and-elsif.md) — `fcc fmt` が `else if` を続けて整形するようにした。elsif は fc 4 でなくす候補
- 2026-09-30 [文書への参照の書き方](2026-09-30-doc-reference-style.md) — § の番号をやめ、ファイル名と見出しの言葉で指す。コードからは design/ か docs/ を指す
- 2026-09-30 [利用者向けドキュメントの方針](2026-09-30-user-docs-direction.md) — docs/ に利用者向けを作り Pages で公開、fc 4 だけ書く、旧 language_reference は参考にして後で消す（計画は wiki/plans/user-docs.md）
- 2026-09-30 [今のブランチへの直接コミット](2026-09-30-direct-commit-to-current-branch.md) — トピックブランチを切らず今のブランチ（feature/v4）に直接コミットしてよい
- 2026-09-30 [Agent/ の導入と doc/ の振り分け](2026-09-30-agent-workspace-and-doc-split.md) — doc/ を人間向けに絞り、設計は wiki/design・計画は wiki/plans・過去の検討はここへ。development_notes を分割
- 2026-09-21 [アクティベーション単位のグローバル領域](2026-09-21-activation-memory-ideas.md) — 検討メモ
- 2026-09-20 [FC V3 検討メモ](2026-09-20-v3-plan.md) — v3 の言語変更の検討と実装記録（v3 は main にマージ済み）
- 2026-09-20 [slice の長さ・ABI・sentinel・移行](2026-09-20-v3-slice-tradeoffs.md) / [slice・固定容量 vector の設計案](2026-09-20-v3-slices-vector.md) / [slice・vector を使う API の利用案](2026-09-20-v3-slices-api-examples.md)
- 2026-09-19 [fuzz で見つかったバグと生成器の拡張の記録](2026-09-19-fuzz-findings-log.md) — 種の範囲ごとの発見と固定したテスト名（追記型）
- 2026-09-14 [v2 のアイデアメモ](2026-09-14-v2-idea.md)
- 2026-09-13 [文法 v2 設計メモ](2026-09-13-v2-grammar.md)
- 2026-09-12 [v2 未決事項の検討](2026-09-12-v2-decisions.md) / [v2 作業計画（R0〜R3）](2026-09-12-v2-plan.md) / [ブランチ運用の経緯](2026-09-12-branch-history.md)
- 2026-08-29 [Go 実装の進化計画（脱・厳密クローン）](2026-08-29-go-evolution-plan.md)（添付: [castle の符号付き比較の箇所](2026-08-29-castle-signed-compare-sites.txt)）
