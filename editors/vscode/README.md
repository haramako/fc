# FC Language for VS Code

[fc](https://github.com/haramako/fc)（NES 向けコンパイラ）のソース `.fc` 用の拡張。

- シンタックスハイライト（現行 v2 文法）、コメント・括弧・インデントの設定、スニペット
- フォーマット: `fcc fmt`（コマンド **FC: Format Document**、または `editor.formatOnSave`）
- 診断: `fcc check` の結果を Problems パネルに表示（保存時、または **FC: Check**）

## インストール

`fcc` が必要です（[リリース](https://github.com/haramako/fc/releases) か `go build ./cmd/fcc`）。

```
cd editors/vscode
npm install
npm run package            # fc-lang-<version>.vsix ができる
code --install-extension fc-lang-0.1.1.vsix
```

## 設定

| 設定 | 既定 | 意味 |
|---|---|---|
| `fc.fccPath` | `""` | `fcc` の実行パス。空なら PATH の `fcc`、無ければワークスペースの `bin/windows/fcc.exe` / `bin/linux/fcc` |
| `fc.target` | `emu` | `fcc check -t` に渡すターゲット（`nes` / `emu`） |
| `fc.mainFile` | `""` | `fcc check` の起点（ワークスペース相対。例 `src/main.fc`）。空なら開いているファイルと同じディレクトリの `main.fc`、無ければそのファイル。実際のメインモジュールを指定すると、プロジェクト設定と参照するモジュール全体をビルド時と同じ条件で検査できる |
| `fc.checkOnSave` | `true` | 保存時に `fcc check` を走らせる |

castle のようなプロジェクトなら `.vscode/settings.json` に:

```json
{
  "fc.target": "nes",
  "fc.mainFile": "src/main.fc",
  "[fc]": { "editor.formatOnSave": true }
}
```

## 開発

- 文法の検査: `npm run check-grammar`（リポジトリ内の全 `.fc` をトークン化し、代表的なトークンのスコープを確認）
- デバッグ実行: このディレクトリを VS Code で開いて F5（Extension Development Host）

## 現行言語への対応

- `alias name:Type = storage;` / `public alias`、`block { ... } options(bss: ...)`、`fn` / `farfn` を強調表示する。
- `alias` / `block` は文脈に応じて強調し、同名の通常の変数や関数も使える。
- デフォルト引数の定数式、入れ子の式を含む `options`、`textmap` 等の組み込み呼び出しに対応。
- スニペット: `alias` / `palias`、`bss` / `bssblock`、`farfn`、`functiondefault`、`inline`。
  `fastcall` は外部 asm 用の宣言を挿入する（本体を持つ FC 関数では効果がない）。
- 診断・整形は設定された `fcc` に委譲するため、alias 等を利用するにはコンパイラも最新にする。
  型を解析する補完や定義ジャンプを提供する言語サーバは含まない。
- V3 の検討中の構文（`@(...)`、slice / vector など）はまだ対象外。

構文強調・スニペット・言語設定の編集元はこのディレクトリ。
変更後に `npm run sync-language` で `tools/vscode-fc/` に反映する。
`npm run check-grammar` は両拡張の一致も検査する。
