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
- [x] `dec x; lda x; bne` → `dec x; bne`（codegen の `flagsFromIncDec`。第 4 弾で実装） ✅ 2026-09-16
- [x] `lda #k` の即値も A の追跡に含める（ピープホールの即値追跡。第 4 弾で実装） ✅ 2026-09-16

結果（bench/README.md の第 2 弾の経過）: `entities` -24%、`plasma` -30%、`oam` -22%、`bgdecode` -15%、`fib` -7%、
`textprint` -4%。v0.0.2 比では `entities` -29%、`oam` -46%、`plasma` -49%、`bgdecode` -37%。

### 第 3 弾: フレームの静的割付（B）✅ 2026-09-16（ブランチ `feature/static-frame`）

[v2_frame_alloc.md](v2_frame_alloc.md) §6。bench: `calls` -21%、`entities` -21%、`textprint` -10%、`plasma` -10%。
castle は 440 関数中 **438 が static**（ゼロページ 54 バイト、RAM 0）。stack は本当に自己再帰している 2 つだけ。

- [x] 間接呼び出しの飛び先を「そのポインタ変数に代入された関数」「const 表の要素」に絞る（castle の偽の再帰 88 関数が消えた） ✅
- [x] `fcc build -d` で配置の要約（種類ごとの数、ZP / RAM の使用量、stack に残った再帰の連鎖） ✅
- [x] Entry 関数の直接呼び出しはプロローグを飛ばす（`__direct`） ✅
- [ ] 関数ポインタのローカル変数・struct フィールド経由の呼び出しはまだ型ベース（同じ型の Entry 全部）

### 第 4 弾（レジスタ割付。[v2_regalloc.md](v2_regalloc.md)）

- [x] 最上位 / 最下位ビットの検査 + 両枝の 1 ビットシフトを C フラグ分岐に（`carryBranch`、`if_carry`） ✅ 2026-09-16
- [x] `dec x` 直後の `if x` のフラグ再利用、A にある添字の `tay` ✅
- [x] **最内ループの 1 バイト変数を A に常駐**（`regalloc.AllocateResident`。crc8 が `asl a; bcc; eor #29` に。
      castle では 28 のループが対象になった） ✅ 2026-09-16
- [x] Y の常駐: 添字 / カウンタを Y に置いたまま `iny` / `cpy` / `lda a,y` で回す（A と同時に可） ✅ 2026-09-16
- [x] 外側のループの常駐（内側のループを退避 / 復帰で通過する） ✅ 2026-09-16
- [x] **X の開放**: スタックの空き先頭を X からゼロページ `FC_SP` に移し、X も常駐に使う（グローバル配列の添字とカウンタ） ✅ 2026-09-16
- [x] **volatile** の仕様（`options(address:)`、asm から参照される変数、`options(volatile: true)`。language_reference §2）と
      グローバル変数の常駐（呼び出し・ポインタ経由の書き込み・asm の前後でメモリと同期。castle で 43 ループ） ✅ 2026-09-16
- [x] 要素 2 バイトの配列の添字の 2 倍をブロック内で共有（`opt.scaleIndex`、`Op.Scaled`。`i*2` が Y に常駐して
      `lda px,y` だけになる。math16 -4%） ✅ 2026-09-16
- [x] 関数全体を常駐の領域に（ループの外の直線コード。引数など書き換えない変数は書き戻し無し (`Clean`)。castle で 134 関数。
      bench には効かず。ついでに符号付き `lt` が A を壊すのに friendly だったバグを修正） ✅ 2026-09-16
- [x] 配列全体が 256 バイト以内の `&objs[i]`（要素 3 バイト以上）の積を 8 ビットで（`indexLarge`。oam -11%） ✅ 2026-09-16
- [x] ピープホール: `sta x; ldy x` → `tay`、`ldx x` → `tax`、次の命令が N/Z を立て直すなら `sta x; …; lda x` の lda を消す ✅ 2026-09-16
- [x] 可換な演算の第 2 入力が直前の一時変数なら入れ替え（`opt.commuteTemp`。allocateA は第 1 入力しか A に置かない。
      entities / oam -4%、crc8 -3.5%） ✅ 2026-09-16
- [x] Y / X に常駐する添字の `i += k`（k ≤ 4）を `iny` × k に（`isStep` / `StepMax`）。ループの出口のラベルが外側の if の終端と
      同じでも回転するように（`rotateLoops`）。castle のスプライト消去ループ 26 → 20 サイクル/スロット、フレーム −1〜3% ✅ 2026-09-19
- [x] 非可換な形 `x - tab[i]` / `x < tab[i]` の第 2 入力を直前の index_pget と融合（codegen `fusableIndex`: 添字を Y / X に
      用意して一時変数を `tab+0,y` として読む。bench には形が無く、castle は数か所） ✅ 2026-09-19
- [x] 2 バイト変数の上位 / 下位の分割（`opt.splitWords`、`rolc` / `rorc`。v2_regalloc.md §7。crc16 -13%。sieve のポインタは
      加算で結合しているので対象外 = 誘導変数の統合（SSA の後）の仕事） ✅ 2026-09-19
- [x] ループの回転で条件が末尾に来た後の `dey; cpy #0; bne`（ラベルが間に入る）を `dey; bne` に（テストの複製: 入口の条件は
      そのまま、本体の末尾に条件の写し (新しい一時変数) を置く。1 バイトの条件だけ。crc8 -13%、crc16 -7%） ✅ 2026-09-19
- [x] SSA 化（Braun 方式）+ 定数伝播 / コピー伝播 / DCE（[v2_ssa.md](v2_ssa.md)。IR は変えず CFG の上の解析として持ち、
      結果で命令列を書き換える。sieve -1.3%、bgdecode -0.6%、castle -0.1〜0.3%。`!` の 16 ビットの codegen バグも発覚） ✅ 2026-09-20
- [x] SSA の上の代数の簡約（`y / 16 * 16` → `and #$f0`。castle フレーム −3.4%）と誘導変数の統合（`opt.eliminateInduction`。
      比較にしか使われないカウンタをポインタの比較に。sieve −15.9%。v2_ssa.md §4–5） ✅ 2026-09-20
- [x] 小さなループの完全展開（`opt.unrollLoops`。回数がリテラルで 8 回まで、本体 × 回数 ≤ 64 命令。crc8 −30%、crc16 −15%、
      castle −0.7% / ROM +1.5 KB。v2_ssa.md §6） ✅ 2026-09-20
- [x] 定数との乗算のシフト・加減算への展開（`opt.expandMul`。立っているビットが 3 つまで、または 2^n − 1。
      math16 −5.5%、calls −3.6%、textprint −1.9%、castle −0.5%） ✅ 2026-09-20
- [x] fuzz の生成器にポインタ・ローカル配列・struct・ポインタをずらすループ（初回で codegen のバグ 2 件と SSA の panic 1 件、
      次の 1000 本で fusePointer と splitWords の struct のバグ 2 件） ✅ 2026-09-20
- [x] static 関数のレジスタ渡し（最後の 1 バイトの引数を A、1 バイトの戻り値を A。v2_frame_alloc.md §7。calls −6.5%、
      plasma −3.2%、entities −2.6%、castle −1%） ✅ 2026-09-20
- [x] 関数ポインタ表の呼び出しの直接化（`opt.DevirtualizeProgram`。const の表 16 要素まで。calls −5.6%。v2_ssa.md §7。
      castle の `en_vtbl.PROCESS` は 83 要素で対象外: 上限を上げれば 1.6 KB の ROM で −4%） ✅ 2026-09-20
- [x] fuzz に far call・密な switch・const 表・関数ポインタ表（switch 命令の飛び先が live range の流れに無いバグ、
      stack 関数の switch が X (フレームポインタ) を壊すバグ） ✅ 2026-09-20
- [x] 最後から 2 つ目の引数を Y で渡す（`Lambda.RegArgY`、`codegen.markArgY`。本体の先頭で A / Y に引数がある形にして
      ピープホールが先頭の `ldy` / `lda` を消す。calls −2.4%、castle −0.5%。v2_frame_alloc.md §7.1） ✅ 2026-09-20
- [x] `a[i + k]` の添字の加算を配列側に畳む（`opt.foldIndexOffset` → `sta a+k,y`。bgdecode −11.7%、castle の
      `ppu.sprite_idx` が手書き asm より速く。添字の式は折り返さない規則を §6 に。v2_ssa.md §9） ✅ 2026-09-20
- [x] 展開・自動インラインでフレームが 256 バイトを超えるときは引く（-O 2 で frame size over になった関数に
      `Lambda.NoGrow` を付け、インライン展開とループ展開をせずに sema からやり直す。`driver.retryFrameOver`。
      fuzz で 5000 本中 -O 2 だけ落ちていた 3 本が通るように。`TestFrameOverNoGrow`） ✅ 2026-09-24
- [ ] far call にもレジスタで渡す（トランポリンの速い経路を X だけで書き、切替の経路で A / Y をスタックに退避。
      FC_FARCALL の設定を引数の読み出しの前に。castle の `farcall` も書き換え。v2_frame_alloc.md §7.1）
- [ ] （検討メモ・やる見込みは薄い）far call のバンク復帰を関数の出口まで遅らせる: 関数 F の入口で呼び先のスロットの
      バンクを覚え、F の中の far call は「違えば切り替えて飛ぶだけ」の戻さない版のトランポリンで呼び、F の出口で
      1 回だけ戻す（castle の `en.process` の手動 `set_pbank` と同じ形。farfn の表でも同じバンクが続けば切り替えない）。
      自動で判定できる条件: F がそのスロットに無い、far call の後でそのスロットの const を読まない。判定しにくいのは
      上から受け取ったポインタがそのスロットを指す場合と割り込みの前提なので、`options(farcall_restore: "exit")` の
      ような明示の属性か、「far call の後でポインタを読まない・渡さない」関数に限る自動化になる（2026-09-24 検討）
- [x] 小さな static 関数の自動インライン（12 命令以下でループ・呼び出し無し。6 命令以下は無条件、それより大きいものは
      呼び出し 2 か所まで。calls −22%、entities −16%、castle は ROM +1.6 KB で −0.2%。`options(noinline: true)` /
      `FC_DISABLE=autoinline`。v2_ssa.md §8） ✅ 2026-09-20
- [ ] SSA の上で: volatile でないグローバル / 配列要素の読み出しの前送り（呼び出し・ポインタ書き込み・asm を障壁に）、
      共通部分式、呼び出しをまたぐ mod/ref 解析。6502 では `lda a,y` と `lda t` の差が 1 サイクルで、値が定数になる場合以外は
      効果が薄い（v2_ssa.md §7）
- [ ] 書き換えルールの DSL（Go コンパイラの rulegen の縮小版）— パスが 10 個を超えて手書きの照合が辛くなってから
- [ ] デッドストア除去、live range の精度（穴あき区間の共有）
- [ ] ポインタを 1 ずつ進めるループ（`*p = …; p += 1`）を、ポインタを動かさず Y を進める `sta (p),y; iny` に
      （回数が 256 以下なら Y だけで回してループの後で `p += Y`、超えるなら Y の一周で上位を `inc`）。今は 1 回 約 26 →
      14 サイクルになる見込み。1 バイトの添字の `p[i]` はすでに Y 常駐で最適。sieve は時間の半分が素数ずつ進む内側の
      ループで対象外なので 1〜2 割。castle には今この形のループがほとんど無い（2026-09-24 検討。将来の項目）
- [x] マクロベンチ: `examples/castle` を `internal/nes` で自動プレイし、局面ごとの 1 フレームの busy サイクル
      （フレーム長 − vsync 待ち）を `bench/castle_frames.json` と比べる（`TestCastleFrameCycles`） ✅ 2026-09-19

## 言語機能

- [x] **トップレベルの宣言順への依存を減らす**: 宣言名と `use` を先に収集し、関数・定数・型・配列長を依存関係に従って解決。
      後ろの関数を使う定数テーブル、後ろの定数を使う計算、相互 `use`、struct / SoA の前方参照に対応。
      値・サイズの循環は依存経路付きで診断し、ポインタ / SoA ハンドルを介する再帰は許可する。
      ローカルの可視範囲・実行時評価順・配置属性の規則は維持。詳細は [言語リファレンス §1.3](language_reference.md#13-名前解決)。✅ 2026-09-20

- [x] モジュール `options(bss: "...");` と `block { ... } options(bss: "...");` によるグローバル変数・可変 soa の既定配置。個別 `segment` 優先、モジュール間非伝播。詳細は [v2_bss.md](v2_bss.md)。✅ 2026-09-20

V2 の追加検討（2026-09-20）は [配置・デバッグ環境の検討](v2_placement_debugging.md)。
モジュール / 宣言グループの BSS 指定、関数・変数のブロックを 256 バイト境界をまたがず配置する要求、
Mesen の farcall スタック表示、一時ディレクトリ作成失敗の調査を記録する。BSS 指定と一時ディレクトリ失敗時の診断は実装済み、その他は検討段階。

追加候補の検討記録（2026-09-20）は [language_feature_candidates.md](language_feature_candidates.md)。
`len` / `static_assert` → `enum` → 読み取り専用ポインタの順を推奨する提案で、採用・仕様・実装順は未確定。

slice・固定容量 vector の後続の設計案は [v3_slices_vector.md](v3_slices_vector.md)
（将来の `#fc 3` 向け。検討中・未実装。今すぐ実装する項目ではない）。
static if、CHR パディング、バンクの名前指定、差分コンパイル、組み込み・低頻度の予約語の `@` 表記、
`options(...)` → `@(...)` の決定（未実装）、機能の使用許可の候補は
[FC V3 検討メモ](v3_plan.md) に記録する。仕様確認中の項目を含み、実装開始は未指示。
同メモ §7〜§9 に、`int` の廃止と整数型名、`char`、`printf` の見直しと NES エミュレータの
PC ログポイント（NES 側の追加命令なし・Lua 等で整形）も記録する。

- [x] bool を比較・論理演算の結果型に（`uint8` と互換なので既存コードの変更は不要。添字にも使える。定数畳み込みも bool） ✅ 2026-09-19
- [ ] グローバル変数の初期化（今は "can't init global variable"。DATA セグメント + 起動時コピー）
- [x] const の二重配列（すでに動いていた）・ポインタ配列（`[N]*T`。要素の文字列 / 配列リテラルは無名の配列定数に切り出す） ✅ 2026-09-19
- [x] switch のジャンプテーブル（IR の `switch` 命令。整数の case が 10 個以上で密なとき。`pha; pha; rts`） ✅ 2026-09-19
- [x] cc65 の呼び出し規約（`__fastcall__`）の extern 関数: `options(abi: "cc65")`（引数 0〜1 個を A / A,X、戻り値 A / A,X。
      関数ポインタ不可、別バンク不可） ✅ 2026-09-19。castle の sound.asm のグルー（nsd_begin/nsd_end のバンク切り替え）は
      NSD 側の都合なので残る
- [x] インライン関数: `options(inline: true)`（IR レベルで呼び出し側に展開。castle は `math.abs` 55 か所、`rand` 53 か所、
      `ppu.lock`/`unlock`/`wait_vsync` 90 か所が数命令の関数）✅ 2026-09-19。**goto は入れない**: castle の状態機械は
      `switch (state)` + 配列に持つ状態を毎フレーム回す形（en*.fc、my.fc）、イベント（event.fc）は `wait_vsync` で
      ブロックする直列コードで、どちらも関数内ジャンプは要らない。ラベル付き `break`/`continue`（§5.2）で足りる
- [x] 使われない関数を出力しない（tree shaking。`frames.Analyze` の到達解析で `Lambda.Unused`。コード・静的フレーム・
      `.export` を出さない。`fcc build -d` に一覧） ✅ 2026-09-19

## ツール・開発体験

- [x] 同梱ライブラリ用の一時ディレクトリ作成失敗に、親ディレクトリ・OS エラー・環境変数による対処を表示。探索・展開方法は変更しない（[検討記録 §4](v2_placement_debugging.md)）。✅ 2026-09-20

- [x] **デバッグ情報**: `fcc build -g` が ROM の隣に `.dbg`（fc のソース行入り）と `.mlb` を書く。Mesen が自動で読む
      （development_notes「Mesen でのソースレベルデバッグ」） ✅ 2026-09-19
- [x] watch モード（`fcc watch`: ソースの更新時刻を 0.5 秒ごとに見て再ビルド）、エディタ連携（`fcc check --json` +
      `tools/vscode-fc/` の VS Code 拡張: ハイライトと保存時の診断。言語サーバは無し） ✅ 2026-09-19
- [x] コードサイズレポート（`fcc build --size-report`、`fcc size game.dbg`。dbgfile のラベルから関数ごとの大きさ） ✅ 2026-09-19
- [ ] モジュール単位のキャッシュとインクリメンタルビルド（`ir.ModuleInterface` のシリアライズ、内容ハッシュ、
      依存グラフ。ca65 の並列アセンブルは済み）
- [ ] TTY での診断の色付け

## 整理・判断待ち

**記録のみ・修正しない（2026-09-20、ユーザー方針）:** interrupt 属性では、暗黙の乗除算・剰余ルーチンが
使う共有 `reg` を保護できず、割り込みによって通常処理の演算結果を壊し得る。
タイミング上の要求から interrupt 属性はほぼ使わない想定のため、現時点では対策を実装しない。
詳細は [V3 メモ §6 の既知の問題](v3_plan.md) を参照。

- [x] **v1 パーサの削除**: `fcc migrate` / `internal/migrate` / `.rb` マクロの互換 / `test/*.fc` の v1 版を削除。
      `#fc 2` は任意に、`#fc 1` はエラー。v1 だけの構文は「v2 ではこう書く」のエラーのまま残す ✅ 2026-09-19
- [ ] `memo.txt`（初期の TODO メモ。ほとんど済み）の整理
- [ ] `examples/castle` と実プロジェクト `C:\Work\castle` の同期（`tools/sync_examples.ps1`）と公開可否
- [ ] castle 側（**SSA が終わってからまとめて反映**。2026-09-19 決定）: examples/castle に入れた変更（`data.asm` の
      `FC_SZP` / `FC_SRAM` / `FC_SP`、`mmc3.fc` の `options(static_zp:, static_ram:)`、`ppu.fc` のスプライト消去）、
      `en.process` のバンク切り替えを「変わるときだけ」に、`bg.cell_type` / `bg.cell` の `options(inline: true)`、
      NSD の `options(abi: "cc65")` 化、far call のラッパ撤去（v2_farcall.md §6）、en.fc の soa 化の実験。
      その後 feature/static-frame → feature/v2 のマージ
- [ ] ca65 / ld65 は当面維持（内製アセンブラはやらない。2026-09-14 決定）
