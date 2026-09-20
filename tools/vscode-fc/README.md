# fc (VS Code 拡張)

fc ソースのシンタックスハイライトと、保存時の `fcc check --json` による診断 (Problems パネル) を提供する薄い拡張。
言語サーバは持たない。

## 導入

ビルドは要らない (plain JS)。このフォルダを `~/.vscode/extensions/fc-lang` にコピー (またはシンボリックリンク) して
VS Code を再起動する。`fcc` にパスが通っていない場合は設定 `fc.fccPath` に実行ファイルのパスを書く。

## 設定

| 設定 | 既定 | 意味 |
|---|---|---|
| `fc.fccPath` | `fcc` | fcc の実行ファイル |
| `fc.target` | `nes` | `fcc check -t` に渡すターゲット |
| `fc.checkOnSave` | `true` | 保存時に検査する |
| `fc.mainFile` | (空) | 検査の起点 (ワークスペース相対)。空なら保存したファイル自身 |

castle のように `use` で辿るプログラムは、`fc.mainFile` を `src/main.fc` にしておくと、どのファイルを保存しても
プログラム全体が検査され、他モジュールのエラーもそのファイルに付く。

コマンド `fc: Check current file` で手動でも検査できる。fcc の出力は「fc」出力チャネルに残る。

## 言語対応

現行 v2 の alias、BSS 配置ブロック、farfn、デフォルト引数に対応するハイライトとスニペットを同梱。
`alias` / `palias`、`bss` / `bssblock`、`farfn`、`functiondefault`、`inline` で挿入できる。
診断には各機能に対応した最新の `fcc` を使う。検討段階の V3 構文は対象外。

言語用のファイルは [editors/vscode](../../editors/vscode/) と共通。
変更はそちらで行い、`npm run sync-language` で反映し、`npm run check-grammar` で検査する。
VSIX パッケージとフォーマット機能が必要なら `editors/vscode/` 版を使用する。
両者は同じ拡張 ID (`haramako.fc-lang`) のため、どちらか一方を導入する。
