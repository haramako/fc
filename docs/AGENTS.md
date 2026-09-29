# docs

## Purpose

利用者向け（fc で NES のソフトを作る人）のドキュメントのサイト。VitePress で作り、GitHub Pages（https://haramako.github.io/fc/）で公開する。
構成・仕組み・進め方の計画は `Agent/wiki/plans/user-docs.md`。

## Ownership

- `*.md` … サイトのページ（今はトップの `index.md` だけ。構成は計画の「サイトの構成」）
- `.vitepress/config.mts` … サイトの設定（`base: '/fc/'`、fc の色付け、日本語の検索、`srcExclude`）
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
- 例は `fcc run` / `fcc build` で実際に確かめ、`fcc fmt` の書式にし、出力は ` ```text ` で載せる（CI での自動の検査はまだ無い。計画の「例のコードを検査する」）
- 生成物（`.vitepress/dist`・`.vitepress/cache`、これから作る `reference/std/`）はコミットしない。`reference/std/` は CI のビルドの中で作る
- `AGENTS.md` / `CLAUDE.md` はサイトに出さない（`srcExclude`）

## Work Guidance

- 手元で見る: `npm ci --prefix docs` の後 `npm run dev --prefix docs`（http://localhost:5173/fc/）
- `npm audit` の警告（esbuild / vite）は開発サーバーのもので、公開する静的なファイルには関わらない。開発サーバーを外に公開しない

## Verification

- `npm run build --prefix docs` が通ること（切れたリンクがあると失敗する）

## Child DOX Index

（なし）
