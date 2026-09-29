# 利用者向けドキュメント（GitHub Pages）の計画

作成: 2026-09-30。「決めたこと」はすべて決まった。段 0 は済み、段 1 も済み。

前提（2026-09-30 ユーザー決定）:
- 人間向け、主に**利用者向け**（fc で NES のソフトを作る人）のドキュメントを `docs/` の下に作り、GitHub Pages で公開する
- 今の `docs/language_reference.md` は下敷きにせず参考文献として使い、新しいサイトが揃ったら消す（「進め方」の段 5）
- **fc 4（`#fc 4`）のことだけ書く**。fc 3 以前の規則・移行・歴史は書かない
- Zig（ziglang.org/learn・言語リファレンス）と Go（go.dev/doc）のドキュメントを参考にする

書かないもの: コンパイラの内部（IR・割付・最適化の段。→ `Agent/`）、castle のコード・テキスト・画面（製品）。

書き方の約束は `docs/AGENTS.md`（段 0 で置いた）。

---

## Zig・Go から取り入れること

| 取り入れること | 由来 | fc での形 |
|---|---|---|
| 「はじめに / 使いこなす / リファレンス」の 3 層 | go.dev/doc（Getting Started / Using and understanding Go / References） | 「サイトの構成」の上の段 |
| 言語仕様は 1 ページ（目次つき、Ctrl+F で引ける） | Go の Language Specification、Zig の Language Reference | `reference/language` |
| **例はすべてビルドして動かし、出力を載せる** | Zig の言語リファレンス（docgen が例をテストし、出力を載せる） | 「例のコードを検査する」 |
| 標準ライブラリのリファレンスはソースのコメントから作る | pkg.go.dev・`go doc`、Zig の autodoc | 「標準ライブラリのリファレンスの生成」 |
| コマンドと設定ファイルのリファレンスを別ページに | Go の Command Documentation・go.mod file reference | `reference/fcc`・`reference/fc-toml` |
| 対応プラットフォームのページ | Zig の Platform Support | `reference/targets`（emu・nes・マッパー・メモリの配置） |
| 他の言語を知っている人向けの一巡り | A Tour of Go、Zig の In-depth Overview / Why Zig | `guide/overview`（C を書ける人向けの fc の一巡り） |
| 見て雰囲気をつかむサンプル集 | Zig の Code Examples | `samples/`（画面つき） |
| 版ごとのドキュメント（master と stable） | Zig | 最初は開発版 1 つ。fc 4 のリリース後に分ける（「版」） |
| リリースノート | Go の Release History | `releases`（fc 4 から） |

---

## サイトの構成

公開先: `https://haramako.github.io/fc/`

```
/                       トップ: fc とは、特徴、短いコード（段 0 で作った）
/start/
  install               インストール（リリースのバイナリ: Windows / Linux は cc65 同梱、macOS は cc65 を別に。
                        go install。FC_CC65_BIN。fcc version で確かめる）
  hello-emu             最初のプログラム: emu ターゲットで printf（NES を知らなくても動く。fcc run）
  hello-nes             NES で Hello（examples/hello: frame / vram / pal / pad と内蔵フォント。fcc build -t nes → エミュレータで）
  first-game            小さなゲームを作る（examples/jump を順に: スプライト、パッド、1/16 ピクセルの速さ、@format でスコア）
  project               プロジェクト: fc.toml、複数のモジュールと use、public、fcc test
  editor                VS Code 拡張（色付け・fcc fmt・fcc check）
/guide/
  overview              fc の一巡り（C との違い: 型、slice、for、使わないものは出ない）
  nes-basics            NES の作り方: frame のループと NMI、VRAM のキュー、パレット、スプライト（oam）、パッド、用語
  memory-and-speed      速く小さく書く: 8 ビットと 16 ビット、ゼロページ、静的フレーム、[]T と [:u16]T、fcc size / --size-report
  banks                 バンク切り替え: マッパー（UxROM / MMC1 / MMC3）、@(bank)、far call、関数ポインタ
  asm                   アセンブリと組む: @include、extern と abi（frame / stack / cc65）、インラインアセンブラ
  testing               テスト: @(test)、@assert / @assert_eq、fcc test（-t nes）
  debugging             デバッグ: -g と Mesen のソースレベルデバッグ、@log、fcc check、-d
  libraries             ライブラリ: fc.toml の [lib.*]、fc.lock、fcc lib
  text                  文字と文字列: 文字列定数、@format / printf、独自フォント、@textmap、.po で翻訳
  data                  データ: @incbin、@lz4 / @rle、CHR、NES Screen Tool
  faq
/reference/
  language              言語仕様（1 ページ。組み込み関数も Zig と同じくここに節を置く）
  std/                  標準ライブラリ（生成。モジュールごとに 1 ページ、共通 / nes / emu を分けて表示）
  fcc                   fcc のコマンド・オプション・環境変数
  fc-toml               fc.toml のキー（target・mapper・define・lib など）と fc.lock
  targets               emu / nes、マッパー、メモリの配置、ランタイム、リンカ設定
/samples/               hello・life・jump・statusbar・wave・miku4（画面・説明・ソースへのリンク）
/releases               リリースノート
```

手で書くページは**概念と使い方**を中心にし、関数の一覧・引数は生成したリファレンス（`reference/std/`）へリンクする
（fclib は作り直しの途中で API が動くため）。

---

## 仕組み

### サイトの生成器: VitePress（段 0 で入れた）

- **Shiki が TextMate の文法をそのまま読める**。`tools/vscode-fc/syntaxes/fc.tmLanguage.json`（VS Code 拡張の文法）を `.vitepress/config.mts` で読んで ` ```fc ` に色を付ける。
  Hugo の Chroma・Jekyll の Rouge・MkDocs の Pygments は独自の字句解析器が要り、Pages 標準の Jekyll はプラグインも使えない
- ローカル検索の日本語は `Intl.Segmenter` で語に分ける（MiniSearch の `tokenize`。VitePress が関数を文字列にしてブラウザへ渡すので、
  関数の中だけで完結させる）。「内蔵」「エミュレータ」で引けることを確かめた。home のレイアウトのページは最初の見出しより前が索引に入らない
- VitePress 1.6.4（vite は `overrides` で 6.4.3 以上。「気をつけること」）

置き方（今あるもの＋これから足すもの）:

```
docs/
  package.json, package-lock.json   Node の依存は docs/ に閉じる（ルートは Go だけのまま）
  .vitepress/config.mts             base: '/fc/'、fc の文法、日本語の検索、srcExclude（AGENTS.md / CLAUDE.md / language_reference.md）
  index.md                          トップ
  start/, guide/, reference/, samples/, releases.md   これから
  reference/std/                    生成物（コミットしない。CI のビルドの中で作る）
  public/                           画像・サンプルの画面
  AGENTS.md, CLAUDE.md              書き方の約束
```

### 例のコードを検査する（Zig の docgen に当たる）

コードブロックの info string に印を付け、Go のテストが `docs/**/*.md` から取り出して確かめる（`go test ./...` に入るので CI で走る）。

| 印 | 確かめること |
|---|---|
| ` ```fc ` | 断片。構文が通り、`fcc fmt` の書式と同じ |
| ` ```fc run ` | emu でビルドして走らせ、直後の ` ```text ` のブロックと出力が一致 |
| ` ```fc test ` | `fcc test` が通る |
| ` ```fc nes ` | `-t nes` でビルドが通る |
| ` ```fc error ` | コンパイルエラーになり、直後の ` ```text ` の文言を含む |
| ` ```fc ignore ` | 検査しない（使うときは理由を書く） |

- テストは `internal/` の側に置き（例: `internal/doccheck`）、`docs/` を Go のパッケージにしない
- 同じテストで、コードのコメントから `Agent/`・`docs/` への参照が実在するファイルと見出しを指しているかも確かめる
  （`Agent/wiki/AGENTS.md` の「参照の書き方」）
- 印の後ろの語を VitePress / Shiki が無視することを確かめる（まだ。今のトップは印の無い ` ```fc `）

### 標準ライブラリのリファレンスの生成（pkg.go.dev に当たる）

- **`fcc doc` を作る**（`go doc` と同じ使い方）。`fcc doc vram` / `fcc doc vram.put` は端末に、`fcc doc -md -o DIR` はサイト用の Markdown
- `internal/syntax` で読んで、`public` の宣言（関数・定数・変数・struct・enum）のシグネチャと直前のコメントを集める
- **ドキュメントコメントの約束（Go と同じ形）**: `public` の宣言の直前に空行なしで続く `//` の塊。モジュールの説明は `#fc 4` の後の
  最初の塊。字下げした行は例として等幅で出す
- 今の fclib のコメントを整える: 見出しの `Agent/wiki/plans/v4-stdlib.md §…` の参照や測定の記録は、空行で離すか宣言の中へ移す
  （利用者のページに出さない）
- ターゲット（`fclib/`＝共通、`fclib/nes/`、`fclib/emu/`）をページに出す
- **生成したページはコミットしない**（2026-09-30 決定）。`docs.yml` のビルドで `go run ./cmd/fcc doc -md -o docs/reference/std` を
  VitePress の前に走らせる（Go のセットアップと、`fclib/**` を push の条件に足す）。手元では `npm run dev` の前に同じコマンド

### サンプルの画面

- 載せるのは hello・life・jump・statusbar・wave・miku4（miku4 は 2026-09-30 に載せてよいと決まった）。castle は載せない
- 画面は QuickNES（`internal/quicknes`。Windows の libretro のコアなので CI の Linux では撮れない）の画面をテストが書き、
  `docs/public/samples/` にコミットする（2026-09-30。撮り直し方は `docs/AGENTS.md`）
- 後で: ROM を Pages に置き、ブラウザの NES エミュレータで動かせるようにする（Go の Playground の代わりに「動くものを見る」）

### 公開（GitHub Actions）

- `.github/workflows/docs.yml`（段 0 で置いた）: `main` への push で `docs/**`・`examples/**`・文法・ワークフローが変わったとき、
  `npm ci` → `npm run build` → `actions/upload-pages-artifact` → `actions/deploy-pages`。PR ではビルドだけ（切れたリンクで落ちる）
- **公開するブランチは `main`**（2026-09-30 に `feature/v4` を `main` にマージした。github-pages 環境の許可するブランチも `main` に）
- リポジトリの Pages の設定は「ブランチから公開（`feature/v4` の `/docs`、Jekyll）」だった。VitePress はビルドが要るので、
  Source を「GitHub Actions」に切り替える（切り替えるまで、`docs/` を push すると Jekyll が VitePress の Markdown をそのまま変換してしまう）

### 版

- 最初は 1 つ（fc 4 の開発版）。トップに「fc 4 は開発中」の注意書きを出している
- fc 4 をリリースしたら Zig の形にする: `/` を最新のリリース、`/master/` を開発版（リリースのタグで作ったものを置く）。やり方はそのときに

---

## 進め方

1. **段 0 土台**: ✅ 2026-09-30: VitePress・fc の色付け・日本語の検索・トップ・`docs.yml`・`docs/AGENTS.md`・例の検査のテスト
   （`internal/doccheck`）・インストールのページ。残り: Pages の Source の切り替え（ユーザー）
2. **段 1 はじめに**: `start/` の 6 ページとサンプル集。API が変わっても例の検査で気づける。✅ 2026-09-30: install・hello-emu・hello-nes・
   first-game・project・editor・サンプル集（画面は QuickNES）。VS Code 拡張は `tools/vscode-fc` に 1 つにした（古い `editors/vscode` の整形を移して消した）
3. **段 2 リファレンス（生成と表）**: `fcc doc` と `reference/std/`、`reference/fcc`・`fc-toml`・`targets`
4. **段 3 言語仕様**: fc 4 の規則を 1 ページに。整数の規則など v4 で未決の所（`plans/v4-plan.md` の「決めること」の残り）は決まってから書く
5. **段 4 ガイド**: `guide/` の各ページ。`overview` と `nes-basics` を先に
6. **段 5 後始末**: 旧 `docs/language_reference.md` を消す。コードのコメントの `docs/language_reference.md §N`（約 20 か所）と
   `Agent/discussions/2026-09-20-v3-plan.md` を仕様として指す所（約 40 か所）を、新しい言語仕様の見出しか `Agent/wiki/design/` に
   付け替える。`.goreleaser.yaml` の `files` から外す。README を短くしてサイトへ誘導

段 1 を先にするのは、入口として一番効き、中身（hello・jump）がすでに動いているため。言語仕様を後にするのは v4 の規則がまだ動くため。

---

## 気をつけること

- **fc 4 の API はまだ動く**（fclib の作り直しの途中）: 関数の一覧は生成に任せ、手で書くページの例は検査で壊れたら気づくようにする
- **Pages は公開**: castle のものを載せない。サンプルの画面・ROM も公開してよいものだけ
- ドキュメントの例が長いテストの時間を増やさないよう、検査は並列にして emu で走らせる（NES は印のあるものだけ）
- 段 0 で気づいて直したこと（2026-09-30）:
  - emu で `main` から戻ると `fcc run` が終わらなかった → 終了コード 0 で終わるようにした。例に `console.exit(0)` は要らない
    （NES では `main` から戻らない。`fcc run -t nes` は 3600 フレームで「終わらなかった」と止まる）
  - `else if` を `fcc fmt` が `else` の中の `if` として字下げしていた → `} else if (...) {` と続けるようにした。例は `else if` で書く
    （`elsif` は fc 4 でなくす候補。`plans/roadmap.md` の v4）
- VitePress 1.6.4 の依存の vite 5 に開発サーバーの脆弱性があるので、`overrides` で vite 6.4.3 以上にした（`docs/AGENTS.md`）

---

## 決めたこと（2026-09-30）

1. **生成器**: VitePress
2. **言語**: 日本語だけ（英語版は作らない。VitePress の i18n で後から足せる）
3. **公開するブランチ**: `feature/v4`。`main` にマージしたら `main` に変える（→ 同日に `main` へ移った）
4. **miku4**: サンプルに載せてよい
5. **文体**: はじめに・ガイドは「です・ます」、リファレンスは「である」
6. **生成した `reference/std/`**: コミットしない。CI のビルドの中で作る
