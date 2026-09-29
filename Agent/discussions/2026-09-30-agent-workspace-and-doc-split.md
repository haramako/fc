# Agent/ の導入と doc/ の振り分け

日付: 2026-09-30 / ブランチ `chore/agent-workspace`（`feature/v4` から）

## 背景

エージェント向けの知識（環境・テストと fuzz の記録・設計メモ・計画）が `doc/` に人間向けの言語仕様と混ざっていた。
ルートに AGENTS.md / CLAUDE.md も無く、エージェントは `doc/development_notes.md` を入口にしていた。
オーナーの方針は「`doc/` は人間向けにする」。そこで castle・linen-work と同じ `agent-work` のテンプレート
（DOX の AGENTS.md 階層 + `Agent/{wiki,discussions,issues,scripts}` + `TODO.md`）を入れ、`doc/` の中身を振り分けた。

## 決定事項

1. **`doc/` には人間向けの `language_reference.md` だけ残す**。README の文書表も人間向けに絞り、開発者向けは `Agent/` を指す
2. 振り分けの規則:
   - 実装済み機能の設計（v2_* の設計メモ・ir_memops）→ `Agent/wiki/design/`（中身は変えず、`v2_` を外した名前に）
   - 進行中の計画（roadmap・v4_plan・v4_stdlib・language_feature_candidates）→ `Agent/wiki/plans/`（テンプレートに無いサブフォルダを足した。TODO.md の「短く保つ」に収まらないため）
   - 役目を終えた検討（v2_decisions・v2_grammar・v2_idea・v3_*・activation_memory_ideas・archive/*）→ `Agent/discussions/`（ファイル名の日付は git の初出日）
   - `development_notes.md` は分割: 環境・ブランチ・PowerShell の罠 → ルート `AGENTS.md`、テストと fuzz の回し方 → `wiki/testing-and-fuzzing.md`、fuzz の発見の時系列 → `discussions/2026-09-19-fuzz-findings-log.md`、機能別の実装メモ → `wiki/implementation-notes.md`、コードの構造 → `wiki/code-structure.md`、@log の規則 → `wiki/log-annotation-rules.md`、MesenCE とデバッグ → `wiki/mesen-and-debugging.md`、feature/v2 時代のブランチ運用 → `discussions/2026-09-12-branch-history.md`
3. **移動と中身の編集は別のコミット**（`git mv` とパスの書き換えだけのコミットを先に。rename を検出させ、他ブランチとの衝突を解きやすくする）。コードのコメント・テスト・README など約 190 ファイルの `doc/...` 参照を新しいパスへ書き換えた
4. **コミットメッセージはこれまでどおり `<領域>: <説明>`**。テンプレートの `[AI]` 接頭辞は入れない
5. リポジトリが公開なので、`Agent/` も公開される前提で書く（`Agent/AGENTS.md` に明記）
6. Codex も AGENTS.md を読むので、ルートの本文に「`Agent/AGENTS.md` も読むこと」と書いた（`@` の import は Claude Code だけが解釈する）

## 積み残し

- 「コードの構造」を `internal/ir`・`regalloc`・`opt`・`driver` などの子 AGENTS.md に移す（DOX の「特定ディレクトリに閉じた知識はそのディレクトリへ」）
- `doc/` の人間向け文書を作る: `language_reference.md` は「文法 v2」のままなので fc 4 へ更新、利用ガイド・CLI・標準ライブラリのリファレンス
- `wiki/design/` の各記事は設計当時の記述を含む。コードと食い違いに気づいたら記事側を直す（wiki の鮮度規則）
- `wiki/placement-debugging.md` に、今は無い `internal/driver/dbgfile.go` / `home.go` へのリンクが残っている（移動前から切れていた）
