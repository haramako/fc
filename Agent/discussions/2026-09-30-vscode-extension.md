# VS Code 拡張を 1 つにする

日付: 2026-09-30

## 背景

VS Code 拡張が 2 つあった: `editors/vscode`（0.1.1。`fcc fmt` の整形、テキストの出力を読む診断、VSIX、文法の検査スクリプト）と
`tools/vscode-fc`（0.1.3。plain JS、`fcc check --json` の診断、fc.toml を見るターゲットの自動、fc 4 の文字のリテラルの色付け）。
文法は editors 側を正として写す決まりだったが、新しい変更は tools 側にだけ入っていた。利用者向けドキュメントでどちらを案内するか決める必要があった。

## 決定事項

- 新しい `tools/vscode-fc` に 1 つにする（ユーザー「古い方は邪魔だから消して」）。古い方にだけあった `fcc fmt` の整形を移して 0.1.4 にし、`editors/` を消した
- 文法の検査スクリプト（`check-grammar.js`。vscode-textmate で全 .fc をトークン化）は npm の依存が要るので移していない。
  文法が読めることは docs/ のサイトのビルド（Shiki が読む）で確かめられる
- 利用者向けのページは `docs/start/editor.md`（フォルダをコピーして入れる）
