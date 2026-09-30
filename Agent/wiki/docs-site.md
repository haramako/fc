# 利用者向けドキュメントのサイトの仕組み

https://haramako.github.io/fc/ （`docs/`）を作って公開する仕組みと、作るときに踏んだ罠。書き方の約束は `docs/AGENTS.md`、
構成と進め方は `plans/user-docs.md`。2026-09-30 に確かめた内容。

## 部品

| 部品 | 場所 | 役目 |
|---|---|---|
| サイト | `docs/`（VitePress 1.6.4、vite は `overrides` で 6.4.3 以上） | Markdown のページ。`docs/.vitepress/config.mts` がナビ・サイドバー・色付け・検索 |
| fc の色付け | `tools/vscode-fc/syntaxes/fc.tmLanguage.json` | VS Code 拡張の TextMate の文法を Shiki にそのまま渡す（` ```fc `） |
| 例の検査 | `internal/doccheck`（`go test ./...` に入る） | ` ```fc` の印（無し / `run` / `test` / `nes` / `error` / `ignore`、`file=`）ごとにビルドして比べる。文書への参照（リンク・パス・`§`・「言葉」）も |
| 標準ライブラリのページ | `internal/fcdoc` → `fcc doc -md docs/reference/std`（`npm run gen`） | fclib の public の宣言と直前のコメントから作る。コミットしない |
| サンプルの画面 | `internal/nes` の `TestExample(Jump\|Statusbar\|Wave)` と `TestSampleScreens` | `FC_SAMPLE_PNG_DIR` を与えると QuickNES の画面を PNG に。`docs/public/samples/` にコミットする |
| 公開 | `.github/workflows/docs.yml` | `main` への push（docs・examples・fclib・fcc など）で Go と Node を入れて `npm run build`（prebuild で `fcc doc -md`）→ Pages |

## 手元での作業

- 基本の言語仕様は `docs/reference/language.md`、応用は `assembly.md`・`banks.md`・`memory.md`。サイドバーの「応用」から開く。
  旧仕様の参照は現行ページへ移し、リリースの同梱文書も README からサイトを案内する形にした（2026-10-01）。

- 見る: `npm ci --prefix docs` → `npm run dev --prefix docs`（http://localhost:5173/fc/）。Claude のデスクトップアプリでは
  `.claude/launch.json` の `docs`（`preview_start`）
- 確かめる: `go test ./internal/doccheck` と `npm run build --prefix docs`（切れたリンクで落ちる）
- 標準ライブラリのページのサイドバーは、`config.mts` を読むときの `docs/reference/std/` の中身から作る。開発サーバーを
  起動してから `npm run gen` したら、サイドバーに出すには開発サーバーを起動し直す
- サンプルの画面の撮り直し: `FC_SAMPLE_PNG_DIR=docs/public/samples go test ./internal/nes -run 'TestExample(Jump|Statusbar|Wave)$|TestSampleScreens'`
  （QuickNES は Windows の libretro のコア `C:\Applications\libretro\quicknes_libretro.dll`。CI の Linux では撮れない）

## 罠（踏んだもの）

- **日本語見出しへのリンク**（2026-10-01、生成 HTML の id と href を照合）: 濁点を含む見出しの自動 id は Unicode の分解形に
  なることがある。本文に直接書いた合成形のフラグメントと一致しないため、ページ間で参照する見出しには `{#calling-conventions}`
  のような英字の id を明示する。VitePress のビルド成功だけでは見出しリンクの正しさを保証しない。

- **検査の範囲**（2026-10-01 確認）: `npm run build` は標準ライブラリのページ生成と VitePress ビルドだけで、掲載例の実行はしない。
  `go test ./internal/doccheck` は別に必要。印のない fc ブロックは構文・書式だけ、生成した `reference/std/` の例は対象外。
  この日に `go test ./internal/doccheck ./internal/fcdoc ./pkg/fc` とサイトビルドが通った。
- **生成 API 説明の照合**（2026-10-01、fc 4 の小さなプログラムを `fcc run -t emu` で実行）:
  `math.subpixel(16, phase)` を phase=0〜15 で足した値は 16。16 フレームの移動量は v ピクセルであり、v / 16 ではない。
  `var s = "abc"; @sizeof(s)` は 3 で終端 0 を自動付加しない。math.subpixel / fmt.str_z_in のコメントもこの挙動に合わせた。

- **Vue が `{{` を式として読む**: Markdown の本文や表の中の `` `{{` `` でビルドが落ちる（コードのブロックは VitePress が v-pre に
  するので平気）。`::: v-pre` で囲む。`fcc doc -md` の出力はページ全体を `::: v-pre` で囲んでいる
- **本文の `<` がタグになる**: コメントから作るページは `` `…` `` の外の `<` `>` を文字参照にしている（`fcdoc.escape`）
- **日本語の検索**: MiniSearch の既定の分け方は空白なので、`tokenize` を `Intl.Segmenter` にしている。VitePress はテーマの設定の
  関数を文字列にしてブラウザへ渡す（`_vp-fn_`）ので、関数の中だけで完結させる（外の変数を参照しない）。home のレイアウトの
  ページは最初の見出しより前が索引に入らない
- **Pages の設定**: VitePress はビルドが要るので、Settings → Pages の Source は「GitHub Actions」（`gh api repos/haramako/fc/pages` の
  `build_type` が `workflow`）。「ブランチから公開」（`legacy`）のままだと Jekyll の `pages-build-deployment` も走り、Actions で
  出したサイトを後から Jekyll の版で上書きする。`github-pages` 環境の許可するブランチに `main` が要る
- **`go install …@feature/v4` は通らない**: Go はブランチ名の `/` を版の文字列として受け付けない。これが開発を `main` に移した理由
  （`../discussions/2026-09-30-move-to-main.md`）。今は `go install github.com/haramako/fc/cmd/fcc@main`
- **VitePress の `<<<` での読み込み**: `<<< @/../examples/hello/hello.fc` で docs の外のファイルも読める（拡張子 `.fc` で色が付く）。
  読み込んだソースは例の検査の対象外（動くことは examples のテストが見る）。読み込むソースのコメントに `Agent/` への参照を書かない
- **ブラウザのパネルのスクリーンショット**: スクロールした位置では真っ黒になることがある。表示の確認は `javascript_tool` で DOM を
  見るほうが確か
- **Bash の heredoc から Python で書き換えるとき**: 文字列の中の `\n` や `\x` が壊れることがある（本物の改行になる・エスケープの
  エラー）。Go や JS のソースの書き換えは Edit / Write で
