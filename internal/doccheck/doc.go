// Package doccheck は文書を確かめるテストだけを持つ (コードは無い)。
//
//   - TestDocsExamples: docs/ の Markdown の ```fc のブロックをビルド・実行して、書式と出力を確かめる (docs/AGENTS.md)
//   - TestRepoDocRefs: 文書の相対リンク、コメントなどに書いた Agent/ と docs/ の文書のパス、その後の「§ 番号」「見出しの言葉」が
//     実在するものを指しているかを確かめる (Agent/wiki/AGENTS.md の「参照の書き方」)
package doccheck
