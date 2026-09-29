# 利用者向けドキュメントの方針

日付: 2026-09-30

## 背景

`docs/` を人間向けにした（[2026-09-30-agent-workspace-and-doc-split.md](2026-09-30-agent-workspace-and-doc-split.md)）が、
中身は fc 2 の頃からの `language_reference.md` 1 つで、利用者が始めるための文書が無かった。

## 決定事項

- 主に**利用者向け**のドキュメントを `docs/` の下に作り、GitHub Pages で公開する。Zig と Go のドキュメントを参考にする
- **fc 4 のことだけ書く**（fc 3 以前の規則・移行・歴史は書かない）
- 今の `docs/language_reference.md` は参考文献として使い、新しいサイトが揃ったら消す
- 構成・仕組み・進め方は [wiki/plans/user-docs.md](../wiki/plans/user-docs.md)

同日に決めたこと（計画の「決めたこと」）:
- 生成器は **VitePress**。VS Code 拡張の TextMate の文法（`tools/vscode-fc/syntaxes/fc.tmLanguage.json`）を Shiki がそのまま
  読めるので fc のコードに色が付く（Pages 標準の Jekyll ではできない）。段 0（VitePress・色付け・日本語の検索・トップ・`docs.yml`・
  `docs/AGENTS.md`）を入れた
- **日本語だけ**。文体は、はじめに・ガイドが「です・ます」、リファレンスが「である」
- **`feature/v4` から公開**する（ユーザーが Pages を `feature/v4` に設定済み。`main` にマージしたら変える）。VitePress はビルドが
  要るので Pages の Source は「GitHub Actions」にする
- サンプルに miku4 を載せてよい
- `fcc doc` で生成する標準ライブラリのページは**コミットせず**、CI のビルドの中で作る
