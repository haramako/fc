# FC Language for VS Code

[fc](https://github.com/haramako/fc)（NES 向けコンパイラ）のソース `.fc` 用の拡張。

- シンタックスハイライト（v1 / v2 両方の文法）、コメント・括弧・インデントの設定、スニペット
- フォーマット: `fcc fmt`（コマンド **FC: Format Document**、または `editor.formatOnSave`）
- 診断: `fcc check` の結果を Problems パネルに表示（保存時、または **FC: Check**）

## インストール

`fcc` が必要です（[リリース](https://github.com/haramako/fc/releases) か `go build ./cmd/fcc`）。

```
cd editors/vscode
npm install
npm run package            # fc-lang-<version>.vsix ができる
code --install-extension fc-lang-0.1.0.vsix
```

## 設定

| 設定 | 既定 | 意味 |
|---|---|---|
| `fc.fccPath` | `""` | `fcc` の実行パス。空なら PATH の `fcc`、無ければワークスペースの `bin/windows/fcc.exe` / `bin/linux/fcc` |
| `fc.target` | `emu` | `fcc check -t` に渡すターゲット（`nes` / `emu`） |
| `fc.mainFile` | `""` | `fcc check` の起点（ワークスペース相対。例 `src/main.fc`）。空なら開いているファイルと同じディレクトリの `main.fc`、無ければそのファイル。`use` の解決は順序依存なので、実際のメインモジュールから検査するとビルドと同じ結果になる |
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
