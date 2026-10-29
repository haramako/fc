# TODO

セッションをまたいで残す、Claude 中心の継続作業メモ。issue 化するほどではないが忘れたくないものを置く。

**短く保つ前提**（項目が増えてきたらタスクが滞っているサイン。消化を優先する）。

- 完了した項目は削除する
- issue 化すべきものは `issues/` へ、決定事項は `discussions/` へ、恒久的な事実・ゴールは `wiki/` へ移す
- 形式は `- [ ] <内容>`（必要なら末尾に `(YYYY-MM-DD)` で追加日）

## Items

- [ ] `plans/v4-plan.md`・`plans/v4-stdlib.md`・`discussions/2026-09-20-v3-plan.md` のいまも正しい仕様を `wiki/design/` に抜き出し、
      コードからの参照を付け替える。済んだら `TestAgentBudget` の例外を消し、コードから `discussions/`・`plans/` を指したら落ちる
      検査を足す（残りは例外として名前で表に）(2026-10-29)
- [ ] `wiki/testing-and-fuzzing.md` を回し方と事例の記録に分け、`TestAgentBudget` の例外を消す (2026-10-29)
