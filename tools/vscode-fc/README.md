# fc (VS Code 拡張)

fc ソースのシンタックスハイライト、`fcc fmt` による整形、保存時の `fcc check --json` による診断 (Problems パネル) を提供する
薄い拡張。言語サーバは持たない。

## 導入

ビルドは要らない (plain JS)。このフォルダを `~/.vscode/extensions/fc-lang` にコピー (またはシンボリックリンク) して
VS Code を再起動する。`fcc` にパスが通っていない場合は設定 `fc.fccPath` に実行ファイルのパスを書く。

## 設定

| 設定 | 既定 | 意味 |
|---|---|---|
| `fc.fccPath` | `fcc` | fcc の実行ファイル |
| `fc.target` | `auto` | `fcc check -t` に渡すターゲット（`auto` は渡さない: fcc が fc.toml に `[target]` があれば nes、無ければ emu にする。fc.toml を使わない NES のプロジェクトは `nes` に） |
| `fc.checkOnSave` | `true` | 保存時に検査する |
| `fc.mainFile` | (空) | 検査の起点 (ワークスペース相対)。空なら保存したファイル自身 |

`use` で辿るプログラムは、`fc.mainFile` を `src/main.fc` のようにしておくと、どのファイルを保存してもプログラム全体が
検査され、他モジュールのエラーもそのファイルに付く。

コマンド `fc: Check current file` で手動でも検査できる。fcc の出力は「fc」出力チャネルに残る。

## 整形

「ドキュメントのフォーマット」(Shift+Alt+F) で `fcc fmt` の書式にする。保存のたびに整形するには、設定で
`"[fc]": { "editor.formatOnSave": true }`。構文エラーがあるときは整形せず、ステータスバーと「fc」出力チャネルに出す。

## 言語のファイル

`syntaxes/fc.tmLanguage.json` は利用者向けドキュメントのサイト (docs/) の fc のコードの色付けにも使う (`docs/.vitepress/config.mts`)。
文法を変えたら `npm run build --prefix docs` でサイトのビルドが通ることも確かめる。
