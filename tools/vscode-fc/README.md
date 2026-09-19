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
