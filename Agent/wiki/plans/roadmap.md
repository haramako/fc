# ロードマップ（未着手・未決の項目）

未着手・未決の項目だけを置く。済んだら項目を消す（仕様は `../design/`、経緯は `../../discussions/`、消した項目は git の履歴にある）。
アイデア段階のものは [Agent/discussions/2026-09-14-v2-idea.md](../../discussions/2026-09-14-v2-idea.md)。

## 最適化

計測は [../bench/](../../../bench/README.md)（`go test ./bench`）。

- [ ] 関数ポインタのローカル変数・struct フィールド経由の呼び出しはまだ型ベース（同じ型の Entry 全部）
- [ ] （検討メモ・やる見込みは薄い）far call のバンク復帰を関数の出口まで遅らせる: 関数 F の入口で呼び先のスロットの
      バンクを覚え、F の中の far call は「違えば切り替えて飛ぶだけ」の戻さない版のトランポリンで呼び、F の出口で
      1 回だけ戻す（castle の `en.process` の手動 `set_pbank` と同じ形。farfn の表でも同じバンクが続けば切り替えない）。
      自動で判定できる条件: F がそのスロットに無い、far call の後でそのスロットの const を読まない。判定しにくいのは
      上から受け取ったポインタがそのスロットを指す場合と割り込みの前提なので、`options(farcall_restore: "exit")` の
      ような明示の属性か、「far call の後でポインタを読まない・渡さない」関数に限る自動化になる（2026-09-24 検討）
- [ ] （棚上げ・アイデアだけ）fc.toml の `[bank.名前]` に `near = true`: そのバンクのモジュール全部に `@(near);` を付けたのと同じ
      （面の間ずっと `$A000` に入れておくバンクを 1 か所で決める。GitHub の Issue #12）。実装は sema の bankByName で近いバンクなら
      モジュールに near を付けるだけで小さい。懸念は「呼ぶときにそのバンクが入っている」という実行時の約束がモジュールのソースから
      見えなくなること（破ると暴走、警告も出ない）と、同じことの書き方が 2 つになること。入れるなら banks のページの警告の枠と、
      `-d` に near とみなした理由を出す（2026-10-27 ユーザーと相談して棚上げ）
- [ ] SSA の上で: volatile でないグローバル / 配列要素の読み出しの前送り（呼び出し・ポインタ書き込み・asm を障壁に）、
      共通部分式、呼び出しをまたぐ mod/ref 解析。6502 では `lda a,y` と `lda t` の差が 1 サイクルで、値が定数になる場合以外は
      効果が薄い（Agent/wiki/design/ssa.md §7）
- [ ] 書き換えルールの DSL（Go コンパイラの rulegen の縮小版）— パスが 10 個を超えて手書きの照合が辛くなってから
- [ ] デッドストア除去、live range の精度（穴あき区間の共有）。常駐の割付の後のリテラルの伝播（`regalloc.PropagateLiteralLoads`）は済み
- [ ] fclib の NES のモジュールを縮める。残り: vram の put_dir（254）・fill（133）・reserve（92）の生成コード（slice の受け渡し・
      16 ビットの番地の進め方）。frame の NMI（304）は vblank の手間のために 8 バイトずつ展開しているので縮めない。
      キューに積む put_dir / fill を asm にする案は小さくならず取りやめた（Agent/discussions/2026-10-05-nes-asm.md）

## 言語機能

- [ ] interface の残り: 確かめる下向きの変換 `as? Slime`、soa の interface の要素を値として写すこと、far call でバンクを戻さない
      呼び出しの記法（far の interface にも普通の `farfn` にも。その後のバンクは自己責任。2026-10-06 ユーザーのメモ）、続けて far call
      するときに戻すのを後に回す最適化、敵の群れのサンプルで手書きの表と比べる bench
- [ ] 標準ライブラリ: 残りは castle を fc 4 の新しい API に移すこと（[Agent/wiki/plans/v4-stdlib.md](v4-stdlib.md)）

候補の置き場所（採用・仕様・実装順は未確定）:

- 配置・デバッグ環境（モジュール / 宣言グループの BSS 指定の残り、256 バイト境界をまたがない配置、Mesen の farcall スタック表示）:
  [placement-debugging.md](../placement-debugging.md)
- 言語機能の追加候補（`len` / `static_assert` → `enum` → 読み取り専用ポインタの順を推奨）:
  [language-feature-candidates.md](language-feature-candidates.md)
- slice・固定容量 vector の後続の設計案: [Agent/discussions/2026-09-20-v3-slices-vector.md](../../discussions/2026-09-20-v3-slices-vector.md)
- static if、CHR パディング、差分コンパイル、機能の使用許可などの候補: [FC V3 検討メモ](../../discussions/2026-09-20-v3-plan.md)

## v3: 一般的な用途で不便な仕様（2026-09-26 調査）

仕様全般を「普通に書いたら困るもの」の観点で調べた結果の残り。再現と直す方向は
[Agent/discussions/2026-09-20-v3-plan.md §10](../../discussions/2026-09-20-v3-plan.md)。

### バグ（誤った結果・クラッシュ）: 先に直す

- [ ] 高: **する（2026-09-26 決定。後回し: 割り込みの定型文と一緒に 2026-09-27）** `@(farcall)` を忘れると切替バンクの関数を `jsr` で呼んで黙って壊れる → fc.toml に切替バンクが
      あれば far call を既定で有効にする（トランポリン・状態変数はマッパーのプロファイルから自動で。手動で管理するモジュールは `@(near)`）

### 言語: 整数と式

- [ ] 中: `as bool` / bool への代入を `!= 0` に → **正規化はしない（処理が増えるため。2026-09-26）**。`5 as bool` が 5 のままで
      あることと、struct・配列の `==` への影響をリファレンスに書く
- [ ] 低: `if (x = 0)` の警告、enum の `++` / `--`

### 言語: 宣言・データ

- [ ] 低: var の `@(zeropage)` と `.importzp`、`@(address:)` の定数式、文字列の連結、soa の `@len`、
      型の表示（`[?]T`、`*const`、配列のエラーの要素）

### 言語: 制御構文・関数・モジュール

- [ ] 中: C 風の前方宣言を案内する（本体の無い関数は `@(symbol:)` を必須に）
- [ ] 低: 構文エラーの `Kind(N)`（残りは `@` の組み込み）、`@asm` から変数を参照する手段、サブディレクトリのモジュール、
      fc 3 で fc 2 の書き方を案内するエラーの文言（`options(...)`、`use bitcast` など）、emu のスタックのあふれの表示

### 2 回目の調査（2026-09-27）

詳細は Agent/discussions/2026-09-20-v3-plan.md §10.7。

- [ ] 未判断: 不便な仕様（for-each で回せないもの、`*[N]T` の添字、`@sizeof(式)`、配列リテラルの要素の型、属性の値の検査、
      `@log` の `?`）、NES・ツール（`fcc run` の時間の上限、`fcc check` とリンクのエラー、NES の printf の
      16 進、textmap の const、ランタイム 1.5KB）、文書（VS Code 拡張、NES の最小サンプルと fclib の API）、
      エラーメッセージ（範囲のエラー、`Kind(43)` など）

### fclib と NES の開発の流れ

- [ ] 中: fc.toml のマッパーのプロファイルから、トランポリン・状態変数・reset の初期化を自動で（MMC3 の固定バンクの配置、`cli`）。
      fclib の uxrom / mmc1 / mmc3 のモジュール（トランポリン・状態変数・init・IRQ の呼び出し口）は済み
- [ ] 低: 文書が実装より古い所の残り: struct の `==`（正式な仕様にする: 2026-09-26）・ポインタの加減算の単位・`5 as bool` が 5 のままのこと
      （上の `as bool` の項目）

## ツール・開発体験

- [ ] モジュール単位のキャッシュとインクリメンタルビルド（`sema.ModuleInterface` のシリアライズ、内容ハッシュ、
      依存グラフ。ca65 の並列アセンブルは済み）
- [ ] TTY での診断の色付け

## 構造の整理（2026-09-28 の調査の残り）

大きいものほど後ろ。

- [ ] **IR の命令の同一性を `*Op` に**の残り（大部分は 2026-10-05 に済み。Agent/discussions/2026-10-05-op-identity.md）: SSA
      （`opt/ssa.go`）の添字の鍵（1 回の書き換えの間は命令を挿さないので今は困らない）、挿す変換（induction・unroll・ywalk）を
      作り直さずに続けること（SSA と CFG の差分の更新が要る）
- [ ] **sema の式に型付きの中間表現を**の残り（型を決める段 `sema/typing.go`・計画 `sema/typeplan.go`・`sema.Symbol` は済み。
      Agent/discussions/2026-10-05-typing-stage-rest.md）: 左辺値性と読み取り専用は lval が値の印で判定している（型を決める段は
      左辺値性を持たない）、lval を型付きの木から IR を出す形に作り直すこと、`ir.Value.Untyped`（リテラルの表現なので残した）
- [ ] **テストの共通部品**の残り: ランダム生成器（約 3,000 行）を `internal/fuzzgen` に出せば `tools/fuzzmeasure` が
      `go test -json` を経由せずに直接呼べる

## 整理・判断待ち

**記録のみ・修正しない（2026-09-20、ユーザー方針）:** interrupt 属性では、暗黙の乗除算・剰余ルーチンが
使う共有 `reg` を保護できず、割り込みによって通常処理の演算結果を壊し得る。割り込みは asm で書く方針
（docs/reference/assembly.md）のため対策を実装しない。

- [ ] `examples/castle` と実プロジェクト `C:\Work\castle` の同期（`tools/sync_examples.ps1`）と公開可否
- [ ] castle 側（**SSA が終わってからまとめて反映**。2026-09-19 決定）: examples/castle に入れた変更（`data.asm` の
      `FC_SZP` / `FC_SRAM` / `FC_SP`、`mmc3.fc` の `options(static_zp:, static_ram:)`、`ppu.fc` のスプライト消去）、
      `en.process` のバンク切り替えを「変わるときだけ」に、`bg.cell_type` / `bg.cell` の `options(inline: true)`、
      NSD の `options(abi: "cc65")` 化、far call のラッパ撤去（Agent/wiki/design/farcall.md §6）、en.fc の soa 化の実験、
      `src/mmc3.asm` のトランポリンを `farcall_ay` に（2026-10-05。今の fcc では古い `farcall` のままだとリンクで落ちる）
- [ ] ca65 / ld65 は当面維持（内製アセンブラはやらない。2026-09-14 決定）
