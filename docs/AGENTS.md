# docs

## Purpose

利用者向け（fc で NES のソフトを作る人）のドキュメントのサイト。VitePress で作り、GitHub Pages（https://haramako.github.io/fc/）で公開する。
構成・仕組み・進め方の計画は `Agent/wiki/plans/user-docs.md`。

## Ownership

- `*.md` … サイトのページ（`index.md`・`start/`・`samples/`。構成は計画の「サイトの構成」）
- `.vitepress/config.mts` … サイトの設定（`base: '/fc/'`、ナビとサイドバー、fc の色付け、日本語の検索、`srcExclude`）。
  ページを足したらサイドバーにも足す
- `.vitepress/theme/` … 既定のテーマに足す CSS だけ（NES の画面を 2 倍にぼかさずに出す）
- `public/samples/*.png` … サンプルの画面。QuickNES の画面をテストが書く（Windows で QuickNES のコアがあるとき）。サンプルを変えたら撮り直す:
  `FC_SAMPLE_PNG_DIR=docs/public/samples go test ./internal/nes -run 'TestExample(Jump|Statusbar|Wave)$|TestSampleScreens'`
- `package.json` / `package-lock.json` … Node の依存（VitePress）。Node は `docs/` に閉じる（ルートは Go だけ）
- `language_reference.md` … 作り直す前の言語仕様。参考文献として残すだけでサイトには出さない（`srcExclude`）。新しい言語仕様ができたら消す
- 公開は `.github/workflows/docs.yml`（`feature/v4` への push でビルドして Pages へ。`main` にマージしたら `main` に変える）

## Local Contracts

- **fc 4（`#fc 4`）のことだけ書く**。fc 3 以前の規則・移行・歴史は書かない（「以前は」「fc 3 では」と書かない）
- 日本語だけ。はじめに・ガイドは「です・ます」、リファレンスは「である」
- `Agent/` へリンクしない（Pages に無い）。リポジトリのファイルへは GitHub の絶対 URL
- コンパイラの内部の用語（IR・常駐・割付）を出さない。性能の話は利用者ができること（書き方・型の選び方）で語る
- castle のもの（コード・テキスト・画面）を載せない。サンプルは examples の hello・life・jump・statusbar・wave・miku4
- fc のコードは ` ```fc ` のブロックに書く。色付けは `tools/vscode-fc/syntaxes/fc.tmLanguage.json` をそのまま使う（文法を直せばサイトにも効く）
- **例のコードは `go test ./internal/doccheck`（`TestDocsExamples`）が確かめる**。` ```fc ` の後に印を付ける:
  - 印なし: 断片。構文が通り `fcc fmt` の書式どおり（`#fc` の行が無ければ fc 4、トップレベルで読めなければ関数の本体として読む）
  - `run`: `#fc 4` から始まる 1 つのプログラム。emu で走らせ、終了コード 0・警告なしで、次の ` ```text ` のブロックと出力が同じ
  - `test`: `@(test)` の関数が通る。`nes`: `-t nes` でビルドが通る（警告なし）。`error`: エラーになり、次の ` ```text ` の文言を含む
  - `ignore`: 確かめない（使うときは理由を書く）
  - `file=名前.fc`: そのページの後のブロックが一緒にビルドするファイルにする（同じ名前が既にあれば後ろに足す）。`run` / `test` に
    付ければそのファイルを入口（テストするモジュール）にする。複数のファイルにまたがる例（`start/project.md`）に使う
- examples のファイルを丸ごと見せるときは写さずに VitePress の `<<< @/../examples/…/x.fc` で読み込む（動くことは examples のテストが見る）
- 生成物（`.vitepress/dist`・`.vitepress/cache`、これから作る `reference/std/`）はコミットしない。`reference/std/` は CI のビルドの中で作る
- `AGENTS.md` / `CLAUDE.md` はサイトに出さない（`srcExclude`）

## Work Guidance

- 手元で見る: `npm ci --prefix docs` の後 `npm run dev --prefix docs`（http://localhost:5173/fc/）
- `package.json` の `overrides` で vite を 6.4.3 以上にしている（VitePress 1.6.4 の依存の vite 5 と esbuild 0.21 に開発サーバーの
  脆弱性があり、vite 5 には直した版が無い。vite 6 でビルド・開発サーバー・検索が動くことを確かめた）。VitePress を vite 6 以降に
  依存する版に上げたら `overrides` を外す。`npm audit` が 0 件であることを見る

## Verification

- `npm run build --prefix docs` が通ること（切れたリンクがあると失敗する）
- `go test ./internal/doccheck` が通ること（例のコード。`go test ./...` にも入る）

## Child DOX Index

（なし）
