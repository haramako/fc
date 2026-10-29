// Package doccheck は文書を確かめるテストだけを持つ (コードは無い)。
//
//   - TestDocsExamples: docs/ の Markdown の ```fc のブロックをビルド・実行して、書式と出力を確かめる (docs/AGENTS.md)
//   - TestRepoDocRefs: 文書の相対リンク、コメントなどに書いた Agent/ と docs/ の文書のパス、その後の「§ 番号」「見出しの言葉」が
//     実在するものを指しているかを確かめる (Agent/wiki/AGENTS.md の「参照の書き方」)
//   - TestAgentIndex: Agent/ の wiki・discussions・issues の記録と索引の行が 1 対 1 か
//   - TestAgentBudget: Agent/ の毎回読むもの (AGENTS.md・索引・TODO.md) と wiki の記事の大きさが上限の内か (Agent/AGENTS.md の「大きさの予算」)
package doccheck
