# ロードマップ（未着手・未決の項目）

2026-09-15 時点で残っている項目。済んだものは消す。設計の経緯は各 v2_*.md、日々の運用は
[development_notes.md](development_notes.md)、アイデア段階のものは [v2_idea.md](v2_idea.md)。
（2026-09 以前の作業ログは [archive/](archive/) にある）

## 最適化（次の主題）

計測基盤は [../bench/](../bench/README.md)（`go test ./bench`）。v0.0.2 時点で fc は cc65 の 1.3〜2.2 倍、Oscar64 の 4〜10 倍
遅かった。2026-09-16 の第 1 弾で cc65 は抜いた（bench/README.md の比較表と経過）。

- [x] A の値の追跡による冗長 `lda` の除去、一時変数経由のコピーの除去 ✅ 2026-09-16
- [x] ポインタ参照で `L`（ZP）を `reg` に写さず `(L+n),y` を直接使う ✅ 2026-09-16
- [x] ジャンプの連鎖・分岐の反転・ループの回転 ✅ 2026-09-16
- [x] `dec` / `inc`、メモリ上の定数シフト ✅ 2026-09-16

### 第 2 弾（sema / opt / ピープホール。フレームの配置に依存しない。目安 3〜4 日）

2026-09-16 に `entities`（castle の `en.process` 相当）の生成コードをレビューして見つけた無駄の順。
括弧は castle（36 モジュール、452 関数）での出現数。どれもフレームの配置（B）と独立なので B より先に入れる。

- [x] switch の case 判定を `eq t; if t goto next`（`cmp #k; bne`）に ✅ 2026-09-16
- [x] `&&` / `||` と `if` の条件の直接分岐化（`compileCond`） ✅ 2026-09-16
- [x] アドレス計算を使用位置へ寄せる（`sinkAddress`）、`pset` 系の値の A 割付 ✅ 2026-09-16
- [x] `return <式>` の直接計算 ✅ 2026-09-16
- [x] Y の追跡ピープホール、`lda` 直後の `cmp #0` ✅ 2026-09-16
- [x] extend_jump の精密化（実サイズで固定点まで） ✅ 2026-09-16
- [x] `internal/opt` の単体テスト ✅ 2026-09-16
- [ ] `dec x; lda x; bne` → `dec x; bne`（A が分岐の先で死んでいることの確認が要る。asm テキストでは分からないので
      codegen が `if` の直前の演算を見て出す方が筋がいい）
- [ ] `lda #k` の即値も A の追跡に含める（`lda #0` の重複。SoA の scatter で多い）

結果（bench/README.md の第 2 弾の経過）: `entities` -24%、`plasma` -30%、`oam` -22%、`bgdecode` -15%、`fib` -7%、
`textprint` -4%。v0.0.2 比では `entities` -29%、`oam` -46%、`plasma` -49%、`bgdecode` -37%。

### 第 3 弾: フレームの静的割付（B）

[v2_frame_alloc.md](v2_frame_alloc.md) §4。`calls` / `entities`（`solid` の呼び出し）と castle 全体の呼び出しコスト
（`call` マクロ 25 サイクル → `jsr` 6、`S+n,x` 4 → 3 サイクル）に効く。**`fib` は再帰なので B の対象外**（スタック方式のまま）。
castle の frame size over の根本対策でもある。決めること: Q3（間接呼び出しは共通引数バッファ + プロローグコピー）、
Q4（fastcall を静的フレームに統合）。別ブランチで 1〜2 週。castle 側は `ppu.asm`（`S+2,x` で引数を読む 5 箇所）を
`abi: "stack"` にするか書き換え。

### 第 4 弾（B の後）

- [ ] SSA 化（Braun 方式）+ 定数伝播 / コピー伝播 / DCE、**A / X / Y を跨ぐレジスタ割付**（crc を A に置いたまま
      `asl; bcc; eor` を回す Oscar64 の水準。残る 2〜6 倍差の本体）。関数の到達解析（tree shaking、v2_idea.md）と同じ基盤
- [ ] 書き換えルールの DSL（Go コンパイラの rulegen の縮小版）— パスが 10 個を超えて手書きの照合が辛くなってから
- [ ] デッドストア除去、live range の精度（穴あき区間の共有）
- [ ] マクロベンチ: `examples/castle` を `internal/nes` で走らせて 1 フレームあたりのサイクル数

## 言語機能

- [ ] bool を比較演算の結果型にする（今は `uint8`。language_reference.md §2）
- [ ] グローバル変数の初期化（今は "can't init global variable"。DATA セグメント + 起動時コピー）
- [ ] const の二重配列・ポインタ配列
- [ ] switch のジャンプテーブル
- [ ] cc65 の呼び出し規約（`__fastcall__`）の extern 関数（v2_idea.md。NSD などの asm ライブラリ向け）
- [ ] インライン関数 / goto（イベント処理の状態機械）— 要否を再検討してから
- [ ] 使われない関数を出力しない（tree shaking、v2_idea.md）

## ツール・開発体験

- [ ] **デバッグ情報**: ca65 `-g` + ld65 `--dbgfile` を配線し、Mesen 向けのシンボル / ソース対応（.mlb / .dbg）を出す
- [ ] watch モード、LSP（VS Code 拡張は `fcc check` を呼ぶだけの薄い実装）
- [ ] コードサイズレポート（関数単位の内訳、`fcc build --size-report`）
- [ ] モジュール単位のキャッシュとインクリメンタルビルド（`ir.ModuleInterface` のシリアライズ、内容ハッシュ、
      依存グラフ。ca65 の並列アセンブルは済み）
- [ ] TTY での診断の色付け

## 整理・判断待ち

- [ ] **v1 パーサの削除時期**: castle / miku は v2 に移行済み。`test/*.fc`（v1）と `test/v2/`（v2）の二重管理、
      `fcc migrate`、`include("*.rb")` の互換処理（`internal/sema/macros.go`、`internal/migrate/`）が v1 と一緒に消える
- [ ] `memo.txt`（初期の TODO メモ。ほとんど済み）の整理
- [ ] `examples/castle` と実プロジェクト `C:\Work\castle` の同期（`tools/sync_examples.ps1`）と公開可否
- [ ] castle 側: far call のラッパ撤去（v2_farcall.md §6）、en.fc の soa 化の実験、NSD 呼び出しのトランポリン統合
- [ ] ca65 / ld65 は当面維持（内製アセンブラはやらない。2026-09-14 決定）
