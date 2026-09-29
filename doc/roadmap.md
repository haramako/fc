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
- [x] 直接化しない関数ポインタ表の呼び出し `PROC[t](i)` は、表から一時変数を経ずに reg へ直接読む（codegen
      `fnPtrToReg`。間が引数の積み込みと単純な演算だけのとき。1 回 12 サイクル。castle area8b −0.6%。fuzz の 4 要素の表は
      20 要素に水増しして間接呼び出しのまま試す） ✅ 2026-09-24
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
- [x] ポインタを 1 ずつ進めるループ（`*p = …; p += 1`）の下位バイトを Y で回す（`opt.walkPointerY`。`lda (p),y; iny;
      bne; inc p+1`、帰りは `cpy lim; bne`。誘導変数の統合を変数の上限にも広げ、ピープホールの `iny; cpy #0` の不具合も
      直した。crc8 −19%、crc16 −9%、sieve −7.6%、oam −2.4%。v2_ssa.md §10） ✅ 2026-09-24
- [x] struct の配列（グローバル、全体が 256 バイト以内）のフィールドの読み書きを `lda a+ofs,y` / `sta a+ofs,y` に
      （`opt.foldFieldIndex`。`index t = &a[i]` + フィールドの pget / pset を、`i * 要素の大きさ` の scaled な index_pget / pset に。
      同じブロックの同じ添字の積は使い回す。今までは先頭のフィールドの読み出し以外、`&a[i]` を 16 ビットで組み立てて `(p),y` で、
      `es[i].hp -= 1` が 1 要素約 20 命令 → 5 命令（SoA と同じ）。定数の添字の `lda #k; asl; tay` も `ldy #2k` に。oam のサイズ −11%、
      entities −1.4%。castle は en が soa なので 1 フレームは変わらない） ✅ 2026-09-27
- [ ] crc8 のように A に常駐する変数とポインタの読み出しを `eor (p),y` に融合する（今は k が X に乗って `stx; ldy; …; ldx`）
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

## v4（2026-09-28 決定）

整数の規則の意味の変更から fc 4（`#fc 4`）にする。fc 2 / fc 3 の意味は変えず、新しい規則は `#fc 4` のモジュールだけに入れる
（主要なプロジェクトと fclib は fc 3 に移行済みのため）。同じ版で、互換性のために後回しにした 2 件と標準ライブラリの拡充も
入れる。設計と論点は [v4_plan.md](v4_plan.md)。

- [x] 整数の規則を決める（v4_plan.md §1.3 の A〜F: 外側からの拡張、8 ビットで上の桁を捨てる形の警告、符号の混在、暗黙の縮小、
      範囲外の定数と const の型の注釈、シフトの結果の型など）。案ごとの影響は 2026-09-28 に数えた（v4_plan.md §1.5。
      castle で A 19・B 187・C 77・D 4・E 19。B は意図どおりの書き方がほとんどで警告に向かず、C は全部が「符号付きが勝つ」に頼っている）。
      決定（2026-09-28）: E（範囲外の定数）はエラー（明示の `as` は通す）、D（大きさが減る暗黙の縮小）はエラー（同じ大きさの
      符号違いは通す）、A は A1（代入先と式の中の一番広い型で計算）、B（8 ビットで上の桁を捨てる形）は警告しない、C は今のまま
      （同じ大きさなら符号付きが勝つ）、F1 はシフトの結果を左辺の型に。2026-09-29: F2（値が必ず 0 になるシフト `hi << 8`）は
      エラー、F3（符号なしの単項マイナス）は今のまま、F4（型を書かない配列リテラル）は全部の定数が入る一番小さい型、F6（符号の
      読み替えになる大小の比較）はエラー。✅ 2026-09-28〜29 で全部実装（migrate も）
- [x] `#fc 4` の受け付けと、sema の版による規則の切り替え ✅ 2026-09-28（規則はまだ fc 3 と同じ）
- [x] fc 3 → fc 4 の migrate: ROM を変えない（2026-09-28 決定。2026-09-29: fc 2 / fc 3 の暗黙の縮小を明示の `as` と同じ cast の形の IR に
      そろえた（値のままだと migrate が足す `as` で -O 2 のレジスタの割付が変わり ROM が変わっていた。fuzz の TestRandomMigrate の
      種 53252143。TestV4MigrateNarrowingROM。今ある castle・miku・test・bench の ROM と数字は変わらない））。✅ 2026-09-28〜29（整数の規則 E・D・A1・F1 と文字列定数の長さ。
      fuzz: TestRandomMigrate / TestRandomProgramsV4 / TestRandomConstFoldV4）。型を見る規則（sema が v4 で意味が変わる式を位置つきで報告し、
      `as` を足す）。examples・test・bench の今の golden とバイト単位で比べる。生成コードが変わるもの（`fastcall` の廃止、
      fclib の新しい API）は後の段で、動作で確かめる
- [x] 名前付きの文字列定数を slice にすると終端の 0 が入る件を直す（v4 だけ。migrate は宣言を長さつきに） ✅ 2026-09-29
- [x] `fastcall` の廃止（asm で書いた関数との規約の指定をどうするかも） ✅ 2026-09-29（asm の関数は `@(abi: "frame")` と `scratch: N`、
      fc 4 の extern は abi の明示が必須、fclib を fc 4 に移した。v4_plan.md §2）
- [ ] slice の引数の受け渡しを縮める: 呼び出し側のフレームで slice を組み立ててから呼び先のフレームへ写し直している（`@format` の 1 回
      約 150 バイト・printf 約 115 バイトの大半。呼び先のフレームに直に組み立てれば半分ほど）。✅ 2026-09-29 の一部: -O 2 の
      `opt.splitSliceArgs`（段の名前 `sliceargs`）が「一時変数のポインタと長さを書いてすぐ push_arg」をポインタと長さの 2 つの
      push_arg（`Op.ArgCont`）にする（printf 2 回と vram.put 3 回の main が 271 → 211 バイト）。✅ 2026-09-29: `[]` → `[:u16]` の変換を
      挟むもの（元の値までたどり、u8 の長さは 1 バイトと上の桁の 0 の 2 つの push_arg に）、定数の添字の配列のアドレス（`buf[2..]` は
      `lda #.LOBYTE(buf+2)` の即値。castle は ROM が変わり 0〜0.2% 速く、QuickNES の 3000 フレームの画面は前と同じ）。✅ 2026-09-29:
      インライン展開した関数の slice の引数の写し（段 `aggcopy`: 3 バイト以上の値の `load L ← P` を伝播。例の包み関数 101 → 69 バイト）、
      `s = s[n..]` の組み立ての一時の値（段 `aggbuild`: 部分をそのまま s に書く）
- [x] LZ4 の展開を速く（今は短い一致の多いデータで 1 バイト約 75 サイクル。列ごとの jsr と長さの読みの手間が大半） ✅ 2026-09-29:
      74.6 → 49.8 サイクル（TestLz4Random の 4000 バイト）。入力の終わりの判定と読む位置の進め方をその場に、長さの続きのバイトは
      15 のときだけ呼ぶ、255 バイトまでの文字と一致はその場で写す、書き先の残りは dst_len を減らして数える。asm は 330 → 439 バイト。
      合わせて opt の ssa: どこからも書かれないローカル（slice の引数の部分）の写しを伝播（`unpack_raw(@ptr(dst), …)` の @ptr の
      一時変数の写し直しが消え、lz4.unpack 107 → 83 バイト）
- [ ] fclib の NES のモジュールを縮める（2026-09-29 の miku4: vram 976 バイト（put_dir 347・fill 146・write_dir 133・reserve 126）、
      frame の NMI 304、pal.shade 133。slice の受け渡しと 16 ビットの演算の生成コードが大きい。速さの要る所は asm に）。
      ✅ 2026-09-29 のコンパイラ側（vram 944 → 868）: インライン展開した bool の関数の結果をフラグのまま分岐（regalloc.allocateCond を
      命令の順に）、`s = s[n..]`（aggbuild）、要素 1 バイトの `p + i` は添字をそのまま adc、X に常駐する添字でグローバルの配列に
      書くループで Y を退避しない（needsY）、`(n as u16) << k` を上位と下位に分けて計算、定数の加減算の連鎖を畳む
      （`room() - 3` → `125 - len`。inline した関数の戻り値の写しを辿る）、飛び先が return だけの jump をその場の return に。
      続けて（vram 868 → 786、pad 213 → 173、en 1975 → 1884）: `(a >> 8) as u8` は a の上位を直に読む、`(i + n) - i` → n、
      `return q[i..i + n]` は戻り値の領域に直に組み立てる、inline した関数に渡した `&g` を通す読み書き（写しとフィールドのずれを
      辿る）は g の直の読み書き（pad.update）、0 との等値の比較は lda の Z をそのまま使う（`cmp #0` を出さない）。fclib 側:
      vram.write_dir を `for (var v in data)`（[:u16] の添字で毎回番地を足す）からポインタを進めるループに。
      ✅ 2026-09-29: `frames.Place` の ZP の選び方（深い順だと main のループの変数が RAM に落ちていた: 参照の少ない関数を
      代わりに RAM へ）、要素 1 バイトで添字が 16 ビットの for-each（`[:u16]` の slice、256 要素を超える配列）は要素を指す
      ポインタを進めるループ（sema の forInElems。write_dir と同じことをコンパイラで）
- [x] `a & b == c`（C と同じ優先順位で `a & (b == c)`）を警告にする（fclib の fmt で踏んだ） ✅ 2026-09-29（警告は構文の lint
      `bitwiseWithComparison` として既にあった。見落としたのは `fcc test` が警告を出していなかったから: `fcc test` で出し、
      pkg/fc の TestFclibModuleTests と TestFclibNoWarnings が fclib のモジュールの警告を見張る）
- [ ] 標準ライブラリの大幅な拡充（slice を使う）と、その API の移行（v4_plan.md §3）。2026-09-29: 互換は考えず作り直す、printf は
      `@format`（snprintf 相当）のラッパーに。計画と決めることは [v4_stdlib.md](v4_stdlib.md)。✅ 2026-09-29: 段 2（どのターゲットでも
      使うもの）と段 3（NES の土台: nes / frame / vram / pal / oam / pad、内蔵のフォント、examples/hello、`fcc test -t nes`）。残りは
      v4_stdlib.md §7 の段 4・5
- [x] fc 4 の使われない private な変数は領域を取らない（@(test) の関数だけが使うバッファなど） ✅ 2026-09-29
- [x] シンプルなパッケージマネージャ（fc.toml の `[lib.NAME]`: path / git + rev + dir、`fc.lock`、`fcc lib`） ✅ 2026-09-29
      （v4_stdlib.md §9。✅ 2026-09-29: `fcc lib add`、`-d` の要約に置き換えを出す。✅ 2026-09-29: ライブラリの依存。残り: `use ライブラリ/モジュール`、tarball）

## v3: 一般的な用途で不便な仕様（2026-09-26 調査）

仕様全般を「普通に書いたら困るもの」の観点で調べた結果。再現と直す方向は [v3_plan.md §10](v3_plan.md)。
済んだもの: const の表の中の slice とポインタ、型を省いた変数の const の引き継ぎ（2026-09-26）。

**決定（2026-09-26）:** 配列リテラルの要素がアドレスになる件、ポインタの加減算の単位、コンパイラが落ちる・asm が壊れるもの
（case の文字列、インライン展開した関数の中の const 表、ローカルの `[?]` の配列）は**修正する**。名前付きの文字列定数の長さは
互換性のため今はこのまま（後で直す候補）。整数の変換・拡張（拡張の規則、途中の上の桁を捨てる形の警告、符号の混在、リテラルの型、暗黙の縮小）は**保留**（検討の経過は v3_plan.md §10.2）。
**2026-09-28: 整数の変換・拡張と名前付きの文字列定数の長さは fc 4 で直す（上の「v4」と v4_plan.md）。** 下の該当する項目は v4 で決める。
警告にするもの: 必ず真・偽になる比較、未知・無効な属性（エラーにするかは後で）。
割り込みは asm で書く前提で、未定義なら空のハンドラを作り stdio の NMI とぶつからないようにする。文字リテラル・条件式・
do-while は「やること候補（すぐではない）」。

### バグ（誤った結果・クラッシュ）: 先に直す

- [x] 高: **修正する（2026-09-26 決定）** 実行時の変数を要素に持つ配列リテラルが変数のアドレスになる / panic / ca65 のエラー（`[n, m]`。実行時に組み立てる） ✅ 2026-09-26（fix/v3-survey）
- [x] 高: **修正する（2026-09-26 決定）** switch の case に文字列（`case "A":`）で panic ✅ 2026-09-26（fix/v3-survey）
- [x] 高: 範囲外の整数リテラルが正規化されず -O で結果が変わる（`var y:i8 = 200`）。-O で結果が変わるのはバグ。範囲外の定数を
      エラーにするかは整数の変換の方針（**保留（2026-09-26）**）と一緒に決める ✅ 2026-09-27（fix/survey2。原因はリテラルでなく、
      i8 の変数で i16 を初期化するときに符号拡張していなかったこと。-O で結果が変わる件は直った。範囲外の定数をエラーにするかは保留のまま）
- [x] 高: **修正する（2026-09-26 決定）** インライン展開した関数の中のローカル const 表で ca65 の `Symbol undefined` ✅ 2026-09-26（fix/v3-survey）
- [x] 高: **修正する（2026-09-26 決定）** ローカルの `var a:[?]u8 = [1, 2, 3]` の領域が確保されない ✅ 2026-09-26（fix/v3-survey）
- [ ] 低: 名前付きの文字列定数を slice にすると終端の 0 が入る（`const NM = "joe"` の `@len` が 4）。**互換性のため今はこのまま、後で直す候補（2026-09-26）** → **v4 で直す（2026-09-28）**
- [x] 高: **修正する（2026-09-26 決定）** ポインタの加減算がバイト単位（`p += 1` が要素の大きさを掛けない。`p[1]` と `*(p + 1)` が違う） ✅ 2026-09-26（fix/v3-survey）
- [x] 高: **修正する（2026-09-26 決定）** NES の `stdio.print_int16` が壊れている（fastcall の渡し方、16 進の表の D） ✅ 2026-09-26（fix/v3-survey）
- [x] 高: **修正する（2026-09-26 決定）** `@(address:)` / fc.toml の `[ram.*]` と fc の ZP / SRAM の重なりを検出しない（OAM を $0200 に置くと FC_FARCALL と重なる） ✅ 2026-09-26（fix/v3-survey）
- [ ] 高: **する（2026-09-26 決定。後回し: 割り込みの定型文と一緒に 2026-09-27）** `@(farcall)` を忘れると切替バンクの関数を `jsr` で呼んで黙って壊れる → fc.toml に切替バンクが
      あれば far call を既定で有効にする（トランポリン・状態変数はマッパーのプロファイルから自動で。手動で管理するモジュールは `@(near)`）
- [ ] 中: **保留（2026-09-26）→ v4 で決める（2026-09-28。v4_plan.md §1.3 E）** スカラーの const の型の注釈が無視される（`const B:u8 = 300` が u16）。範囲内の値は今も宣言の型になり、
      無視されるのは範囲外の値だけ（`300`・`-1` を u8、`200` を i8）なので、整数の変換の方針（範囲外の扱い）と一緒に決める。
      fc 2 の test_var が `const INT16:int = 65535` の値 65535 に頼っていて、migrate の ROM 一致の方針とも関係する
- [x] 中: **する（2026-09-26 決定）** struct・配列・ポインタに算術・順序比較が通る → `==` / `!=`（中身の比較）は正式な仕様にし、
      それ以外（算術・ビット演算・`<` など・`!`・`if (a)`）はいったんエラーに。ポインタは ± 整数、順序比較（`p < end`）、
      null の確認（`if (p)` / `!p`）、ポインタ − ポインタ（要素数）を残す（2026-09-26）。`==` はバイトの比較:
      正規化されていない bool（`5 as bool`）が入っていると、真理値としては等しくても偽、
      slice のフィールドは先頭と長さの比較（中身が同じでも別の配列なら偽）。これを仕様として文書に書く ✅ 2026-09-27（feat/v3-checks）
- [x] 中: **修正する（2026-09-26 決定）** `const X:[4]u8 = [1, 2]` の長さが無視される（ローカル var・struct のフィールドと扱いが違う。0 で埋めるか
      エラーにするかは実装のときに確認 → 0 で埋める、多すぎればエラー） ✅ 2026-09-26（fix/v3-survey）
- [x] 中: **修正する（2026-09-26 決定）** 素のブロック `{ }` がスコープを作らない ✅ 2026-09-26（fix/v3-survey）
- [x] 中: **修正する（2026-09-26 決定）** u16 の可変量シフトが -O で通ったり通らなかったり ✅ 2026-09-26（fix/v3-survey）
- [x] 中: **修正する（2026-09-26 決定）** `use` の検索が作業ディレクトリ基準（ソースのディレクトリにする） ✅ 2026-09-26（fix/v3-survey）
- [x] 中: **修正する（2026-09-26 決定）** `mem.zero` / `mem.compare` / `stdio.ppu_put` の長さ 0 が 256 バイト（mem.set / zero / compare の長さを u16 に。256 を渡すと 0 に切り詰められて頼っていたため） ✅ 2026-09-26（fix/v3-survey）
- [x] 中: **修正する（2026-09-26 決定）** printf が slice・struct を黙って捨てる、符号付きを符号なしで出す ✅ 2026-09-26（fix/v3-survey）
- [x] 中: **修正する（2026-09-26 決定）** `@(symbol: "_interrupt")` の NMI のフレームが main の呼び出しと重なる（interrupt を暗黙に、両方から届く関数を警告） ✅ 2026-09-26（fix/v3-survey）

### 言語: 整数と式

- [ ] 高: **保留（2026-09-26）→ v4 で決める（2026-09-28。v4_plan.md §1.3 A）** 整数の拡張（案）: 式全体を「代入先の型」と「式の中で一番広い型」の広いほうで計算する
      （`var d:u16 = a + b`、`score += pts * 10`、`0x2000 + y * 64`、u16 の引数が正しくなる。全部 8 ビットの式は今のまま）
- [ ] 高: **保留（2026-09-26）→ v4 で決める（2026-09-28。v4_plan.md §1.3 B）** 8 ビットで計算して途中の上の桁を捨てる形を警告（`(x1 + x2) / 2`、`hp * 64 / max`、
      `(v * 3) >> 2`、`a.x + a.w > b.x`）。値の範囲（定数・`& m`・`% n`）で収まると分かるものは除く。消す書き方は
      `(a + b) as u8`（8 ビットのつもり）と `a as u16 + b`（広げる）
- [ ] 高: **保留（2026-09-26）→ v4 で決める（2026-09-28。v4_plan.md §1.3 C）** 符号の混在（u8 + i8 が i8 になる）。ほかの言語との比較と案は v3_plan.md §10.2
- [x] 高: **する（2026-09-27 決定）** リテラルの型: **リテラルは相手の型に合わせる**（Go・Rust と同じ。今は値だけで u8 / i8 / u16 / i16 が
      決まり、同じ大きさなら符号付きが勝つので `s < 200`（s:i8）が -56 との比較になる）。castle の `vx < 0`（i8 と非負のリテラル、
      約 60 か所）は今と同じ意味のまま。相手の型に収まらないリテラルは、**演算（`+ - * / % & | ^ << >>`）では相手の型に切り詰める**
      （今と同じ結果。広げると fclib/math.fc の `x - 128`（i8 の表の添字）が負の添字になるので広げない）、**比較（`== != < > <= >=`）では
      エラー**（`200 does not fit in i8`。2026-09-27 決定）。影響は 2026-09-27 に数えて、演算 4 か所（fclib/math.fc 2、miku 1、test_op 1。
      結果は同じ）と比較 1 か所（darius の enemy.fc:123 `i != -1`。u8 と負の定数の将来対応と一緒に直す）。castle は 0。符号の混在の
      規則（保留）とは別に入れられる ✅ 2026-09-27（feat/literal-typing。型のない定数 `ir.Value.Untyped` と `sema.adaptLiteral`。
      定数のほうが大きい型（`x * 300`、`0x2000 + y * 32`）は今までどおり広げる。castle・miku・darius の ROM は変わらない。darius の
      `i != -1` は作業ツリーで `i != 255` に直っていた（未コミット））
- [ ] 高: **保留（2026-09-26）→ v4 で決める（2026-09-28。v4_plan.md §1.3 D / E）** 暗黙の縮小（範囲外の定数はエラー、変数の縮小は警告か文書を直す）
- [x] 高: **する（2026-09-26 決定）** 必ず真・偽になる比較を警告（`i < 256`（u8）の無限ループ、`hp - dmg < 0`）（符号なしと範囲外の正の定数・`< 0` だけ） ✅ 2026-09-27（feat/v3-checks）
- [x] 高: **将来対応（2026-09-27）** 符号なしの値と負の定数の比較（`x == -1`、x は u8）を正しく警告かエラーにする。
      ✅ 2026-09-27 リテラルの型（上）でエラーになった（`-1 does not fit in u8`。castle・darius は書き換え済み）。今の規則
      （同じ大きさなら符号付きが勝つ）では x == 255 として働き、「無し」を表す慣用表現として castle に 46 か所・darius に 1 か所ある。
      符号の混在・リテラルの型の方針（保留）を決めた後に、警告かエラーにして castle 側も書き換える（`NONE = 255` の定数など）
- [ ] 高: **やること候補（すぐではない。2026-09-26）** 文字リテラル `'A'`（u8 の定数。migrate で fc 2 の `'…'` を `"…"` に）
- [ ] 中: **やること候補（すぐではない。2026-09-26）** 条件式（`c ? a : b`）
- [ ] 中: `as bool` / bool への代入を `!= 0` に → **正規化はしない（処理が増えるため。2026-09-26）**。`5 as bool` が 5 のままで
      あることと、struct・配列の `==` への影響をリファレンスに書く
- [x] 中: **する（2026-09-26 決定）** bool 同士の `==` / `!=` は真理値で比べる（今はバイトの比較で、`x = 5, y = 3` の
      `(x as bool) == (y as bool)` も `(x as bool) == true` も偽）。`f == true` / `f == false` は `lda f / bne`（今より短い）、
      変数同士だけ分岐が数命令増える、0 / 1 と分かる値（比較の結果・`true` / `false`）同士は今のまま `cmp`。`if (f)` と代入は変わらない ✅ 2026-09-27（feat/v3-checks）
- [x] 中: 定数添字の範囲外をエラーに（値が飛び飛びの enum の表も） ✅ 2026-09-29（配列の定数の添字。slice・ポインタは長さが分からないので見ない。飛び飛びの enum の表は残り）
- [ ] 低: `if (x = 0)` の警告、enum の `++` / `--`、const 表の定数添字を定数に（`MONS[1].hp`）
- [ ] 中: 最適化: u8 × u8 → u16（`m as u16 * n`）を 8×8→16 のルーチンに（今は 16×16 の `__mul_16`。`__mul_8t16` は「ほぼ未実装」）、
      `((a as u16 + b) / 2) as u8` を `clc / lda / adc / ror a` に（v3_plan.md §10.2）。✅ 2026-09-29: 前半（`__mul_8t16` を x²/4 の
      16 ビットの表引きで作り直し、0〜255 どうしの 16 ビットの掛け算で呼ぶ。約 400 → 約 60 サイクル。ランタイムの並びが変わるので
      ROM はみな変わり、bench は置き場所のページ跨ぎで ±0.4%。castle の画面は前と同じ）。残り: 後半の平均

### 言語: 宣言・データ

- [x] 高: **する（2026-09-26 決定）** 未知・無効な属性を警告に（`@(adress:)` が黙って RAM 変数になる。エラーにするかは後で決める） ✅ 2026-09-27（feat/v3-checks）
- [ ] 中: グローバル変数の初期値（既存の項目「グローバル変数の初期化」）。**まだやらない（2026-09-26）**
- [ ] 中: 型名を省いた struct リテラルを引数に（`add({1, 2}, p)`）、`mod.Point{…}`、`var s = {1, 2}` のエラーの文言
- [ ] 中: 名前付きの const（struct・配列）を別の const 表の要素に（`[ORIGIN, {1, 2}]`）、診断に位置を
- [ ] 中: グローバル変数のアドレスを const の表に（`[&gp, &g[0]]`）
- [ ] 中: 2 次元配列の行を値の文脈で配列のままに（`var b = a[1]` がポインタになる）
- [ ] 低: var の `@(zeropage)` と `.importzp`、`@(address:)` の定数式、文字列のエスケープ（`\t` `\0` `\"`）と連結、soa の `@len`、
      型の表示（`[?]T`、`*const`、配列のエラーの要素）

### 言語: 制御構文・関数・モジュール

- [ ] 中〜高: **やること候補（すぐではない。2026-09-26）** do-while（少なくとも専用のエラー）
- [x] 中〜高: **する（2026-09-26 決定）** 後ろに case / default が続く、文の無い case を警告（`case 2, 3:` / `fallthrough;` を案内）。
      `:` から次の case までにコメントがある、または `break;` などの文があれば警告しない（何もしない case は書く）。
      コメントは `syntax.File.Comments` にあるので `internal/syntax/lint.go` で判定できる ✅ 2026-09-27（feat/v3-checks）
- [x] 中: **する（2026-09-27 決定）** return 忘れの検査を広げる（`terminates()` に足す。誤検出が減るだけで、コード生成は変わらない）:
      `while (true)` / `for (;true;)` を無限ループと認める、`@if` / `else` の両方（または選ばれた側）が終端文なら終端文。
      全メンバーの enum の switch は**しない**（範囲外の値（`5 as Dir` など）は来うるものとする。default が要る。エラーの文言で案内する）
      ✅ 2026-09-27（feat/v3-syntax。`@if` は選ばれた側を見る）
- [x] 中: **する（2026-09-26 決定）** 初期化していないローカル変数の読み出しを警告（ゼロで初期化はしない。SSA で「書く前に読む」を検出） ✅ 2026-09-27（feat/v3-checks）
- [x] 中: **する（2026-09-27 決定）** ローカル変数のアドレス・slice を返すと警告（静的フレームでは、呼び出し経路の重ならない
      関数とフレームを共有するので、返した後の呼び出しで黙って上書きされる）。`return &x`、`return a`（配列 → ポインタ / slice）、
      `return a[1..3]`、`return &s.f` / `&a[i]`。引数（呼ぶ側の値）・static / グローバル・const の表は対象外。**直接の形だけ**
      （return の式そのものを見る。`var p = &x; return p;` のような変数の経由は追わない。2026-09-27） ✅ 2026-09-27（feat/v3-syntax。sema/escape.go）
- [ ] 中: C 風の前方宣言を案内する（本体の無い関数は `@(symbol:)` を必須に）
- [x] 中: **する（2026-09-27 決定）** for-each（fc 3。`in` は fc 3 の予約語。fc 2 のコードの名前 `in` は migrate が書き換える）:
      `for (var x in A)`（要素の値のコピー、読み取り専用）、`for (var i, x in A)`（添字と要素）、`for (var i in 0..n)`（範囲。
      `0..@len(buf)` は 256 要素でも 256 回）。`var` は必須（`for (var i = 0; …)` とそろえる。型も書ける `var i:u16 in 0..300`）。
      **ポインタの形は回す値の型で決める**（Rust の `&v` と同じ）: `[N]T` / `[]T` は要素の値、`*[N]T` / `*[]T` は要素のポインタ `*T`
      （const の表なら `*const T`）。`&A` は今の意味（`*[N]T`）のまま `for (var p in &A) { p.hp -= 1; }`、引数 `es:*[8]E` も同じ。
      p は本物の変数にせず `&A[i]` の別名として扱う（静的な配列なら `p.hp` は `A[i].hp` と同じ `lda A+ofs,x`。値として使うときだけ
      `&A[i]` を作る）。添字は長さ 256 以下なら u8、超えれば u16。break / continue / ラベルは普通の for と同じ。JS の `for in`
      （キーを回す）と違い値を回すことをリファレンスに書く。後で: enum の全メンバー（`for (var d in Dir)`）、逆順。
      ついでに確かめる: const の表のアドレス `&C` の型が `*const [3]u8` でなく `*[3]u8`（書き込めてしまうか未確認）
      ✅ 2026-09-27（feat/v3-syntax。sema/forin.go。fc 2 の名前 `in` / `enum` / `fallthrough` は migrate の reserved-names が `in_` に。
      `&C` は表示の型が `*[3]u8` だが読み取り専用の印は付いていて書き込みはエラー。for-each がポインタ変数を 1 回だけ評価する
      一時変数に写すときに印が落ちていたのを直した。静的な配列の `&A` は `A[i]` と同じ IR、`0..n` は C 型 for と同じ IR）
- [x] 中: **する（2026-09-27 決定）** case の範囲（`case 16..24:`。今は `Kind(90)` の構文エラー）と、範囲の書き方の統一:
      **範囲 `a..b` はどこでも終わりを含まない**（slice `a[1..3]`、for-each `0..n`、case。ほかの言語の case は含むものが多い
      （GNU C・Zig・Kotlin）が、記号で意味が変わるのを避けてそろえる）。**終わりを含む `a..=b` を全部の文脈に足す**（Rust と同じ。
      `case 16..=23:`、`a[1..=3]`、`for (var i in 0..=255)` は u8 のまま 256 回）。switch の表の該当する所を埋める（広い範囲は比較に）。
      範囲どうし・ほかの case との重なり、空の範囲（`5..5`、`5..=4`）はエラー ✅ 2026-09-27（feat/v3-syntax。比較の連鎖では
      `(x - lo) < 個数` の 1 回の比較。`a..b` は for-each と case の値にだけ書ける `syntax.RangeExpr`（式ではない））
- [ ] 低: 構文エラーの `Kind(N)`（`kindNames` に足す。`?` / `..` / `..=` は足した: 2026-09-27。残りは `@` の組み込み）、`@asm` から変数を参照する手段、サブディレクトリのモジュール、
      fc 3 で fc 2 の書き方を案内するエラーの文言（`options(...)`、`use bitcast` など）、emu のスタックのあふれの表示

### 2 回目の調査（2026-09-27）

詳細は v3_plan.md §10.7。

- [x] 判断の要らないバグ 15 件（SSA の φ と未定義、i8 の初期値の符号拡張、2 バイトの要素の 129 番目以降、ポインタの負の添字、
      型付きの定数の畳み込み、16 ビットを超える定数の比較、@min / case の定数、return 忘れの見逃し、ポインタの算術の const、
      const の表の無名関数、panic と壊れた asm、for-each の p への代入・1 回だけの評価・文字列、ローカルのアドレスの警告の漏れ）
      ✅ 2026-09-27（fix/survey2）
- [x] u8 の `case -1:` も比較と同じくエラーにする（2026-09-27 決定。castle の debug_menu.fc の 4 か所は `case NONE:` に） ✅ 2026-09-27
- [ ] 未判断: 不便な仕様（for-each で回せないもの、`*[N]T` の添字、`@sizeof(式)`、配列リテラルの要素の型、属性の値の検査、
      `@log` の `?`）、NES・ツール（`fcc run` の時間の上限、ソースの後ろのオプション、`fcc check` とリンクのエラー、NES の printf の
      16 進、textmap の const、ランタイム 1.5KB）、文書（リファレンスの fc 2 の記述、VS Code 拡張、NES の最小サンプルと fclib の API）、
      エラーメッセージ（`#fc 3` の無いファイル、範囲のエラー、`Kind(43)` など）

### fclib と NES の開発の流れ

- [x] 高: **する（2026-09-26 決定。後回し: far call の既定化と一緒に 2026-09-27）** 割り込みの定型文を不要に: `_interrupt` / `_interrupt_irq` が未定義なら空のハンドラを作る、
      stdio の NMI と利用者の NMI をぶつからないようにする（割り込みは asm で書く前提。fc で書きやすくする方向は優先しない）
      ✅ 2026-09-29（空の入口は fc が足す。fc 4 の fclib は `frame` が NMI を持ち、利用者は `frame.hook` に。旧 stdio は今までどおり）
- [x] 中: NES の hello world（init で NMI と描画を有効に、ASCII フォントの CHR） ✅ 2026-09-29（examples/hello、fclib/nes/font.chr）
- [x] 中: `nes/ppu.fc`（OAM と DMA、vblank のキュー、パレット・ネームテーブルの転送）と OAM の置き場所の文書 ✅ 2026-09-29（fc 4 の
      frame / vram / pal / oam。OAM は既定 $0700、`[define.oam] ADDR`）
- [ ] 中: マッパーのプロファイルからトランポリン・状態変数・reset の初期化を自動で（MMC3 の固定バンクの配置、`cli`）、fclib の mmc3 / mmc1 / uxrom
      （2026-09-29: fc 4 の fclib に uxrom / mmc1 / mmc3 のモジュール（トランポリン・状態変数・init・IRQ の呼び出し口）。残りは
      fc.toml からの自動の初期化）
- [x] 中: fc.toml の `[target]` に mirroring / battery / CHR-RAM、未知のキー・セクションをエラーに ✅ 2026-09-29
- [x] 中: fc.toml の `[target]` があれば nes を既定に、`fcc run -t nes` の扱い、VS Code 拡張の既定のそろえ ✅ 2026-09-29（`fcc run` の nes は
      内蔵の NES のランナーで console.exit まで走らせて console の出力を出す。拡張の fc.target の既定は auto（-t を渡さない。0.1.3））
- [ ] 低〜中: fclib の API の slice 版（`print`、mem）、lzw の ZP の固定番地と §4.5 の ZP の配置の文書、inflate の `unpack`
- [x] 低: math（`sign` の戻り値、`atan` の範囲、i16 の abs、rand のシード、10 進・BCD の表示）、nes.fc の APU レジスタ・ビット定数、pad の 2P
      ✅ 2026-09-29（fc 4 の math・rand・fmt・nes・pad）
- [ ] 低: 文書が実装より古い所（struct の `==`（正式な仕様にする: 2026-09-26）、u16 の添字、代入の大きさ、ポインタ演算、ZP の配置。v3_plan.md §10.6）

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

- [x] 自動テストの強化（2026-09-27）: 定数の畳み込みと実行時の計算の差分テスト（TestRandomConstFold。畳み込みの型の食い違いを
      4 種類直した）、v3m の自己検査、生成器の拡張（初期値なしの変数・暗黙の拡張・大きな配列・負のずれ・fc 3 の新しい文法）、
      インタプリタの判定の範囲、夜間の CI（fuzz.yml）、通ってはいけないプログラムの表（TestMustError / TestMustWarn）。
      development_notes.md の「差分テストの判定の弱点」 ✅ 2026-09-27

## 構造の整理（残り。2026-09-28 の調査）

2026-09-28 に `refactor/structure` で整理したもの: 前段の一本化（check が options を反映していなかった）、`ir/opinfo.go`、
`ir.Config`、`pipeline`、`Op.Res`、codegen の `asm.go` と `funcGen`、sema の hlc.go の分割、driver の分割（cc65 / project /
fclog / fchome / emu）、fc 1 の残骸の削除、`f() == .A` の二重評価の修正。続けて `refactor/opt-infra` で: opt の段を宣言的に
（`Pass.Apply` が FC_DISABLE・compact・@log の付け替えを一括で）、`ir.Verify`（FC_VERIFY_IR。テストと fuzz では常に有効）、
支配木とループの入れ子（`ir.DomTree` / `Loop.Parent`）と induction / unroll の照合の共有、driver のテストの共通の手順
（`harness_test.go` の `testBuild`）、`r6502.Memory` の配列化。残りは大きいものほど後ろ。

- [x] **regalloc の常駐の見積もりと codegen の命令選択の一本化** ✅ 2026-09-28: 命令の性質とサイクル数を `internal/m6502` の
      表 1 つに（codegen の後処理と regalloc の見積もりが共有）。常駐の形（Y / X に置いた変数の ldy / sty / cpy / iny、
      A が塞がっているときの Y での代用、A を使わないメモリ上の inc / シフト / rol、フラグの分岐）を `regalloc/forms.go`
      に 1 回だけ書き、codegen はそれで命令を出し、regalloc は同じ命令列から friendly / free と得（手書きの 3 / 6 / 1 を
      m6502 のサイクル差に）を計算する。関数ごと最大 8 回の作り直しは命令単位の出し直しに置き換え、テストと fuzz では
      食い違いをコンパイルエラーにした。生成コードは変わらない（golden・castle / miku の ROM・bench が同じ）。見積もりは
      入口 / 出口の写し (`ldx i`) と cast を挟んだフラグの分岐を「A を壊す」と見ていた分だけ正確になった（選択は変わらず）。
      残り: 表の外の予測（汎用の出力が A の値をそのまま扱う形の規則 friendlyA の残り、添字を Y / X のまま使う形、
      needsX / needsY、allocateA / allocateCond の「この命令は A / フラグを受けられるか」）は予測のまま（codegen の出力から
      導くには、置き場所が決まる前に命令を選ぶ構造 = 割付の前の命令選択が要る）
- [ ] **ABI / 呼び出しの計画の一本化**: Stack / Fastcall / Static / Cc65 に Entry・RegArg・RegArgY・RegResult・FrameZp が重なり、
      入口のシンボルが最大 4 つ（`sym` / `__direct` / `__frame` / `__a`）。判定が 20 ファイル 150 か所に散る。frames が関数ごとに
      `CallConv{Params []Loc, Result Loc, Entries}` を作り、呼び出しごとの計画を codegen の前に 1 回計算する
      （`markArgY` / `resolveCall` / `pendingCall` / holdA / holdX / HoldY の状態機械をまとめる）
- [ ] **IR の命令の同一性を `*Op` に**: DefUse / SSA / Liveness が `lmd.Ops` の添字で引き、消した命令を nil で残すので、
      命令を 1 つ挿すと解析を全部作り直す。「1 か所直したら return して再構築」のループが 7 パス（上限は 8 / 16 / 20 / 32 と
      場当たりで、達すると黙って止まる）、`compact()` が Passes に 11 か所、`ops[i+1] == nil` の穴で黙って効かない隣接判定が
      6 か所。`*Op` を鍵にして def-use を差分で更新し、CFG / 支配木 / ループを無効化つきのキャッシュにする
      （`ir.Verify` は 2026-09-28 に入れた。CFG の中の支配木・ループのキャッシュも。命令列を変えたら CFG を作り直す前提はそのまま）
- [x] **メモリアクセスの集約** ([ir_memops.md](ir_memops.md)) ✅ 2026-09-28: 7 つのオペコード + `Scaled` + 配列への cast を
      `load_mem` / `store_mem` と `Base + Index * Scale + Disp`（store は `Width`）に。生成コードは変えていない（ROM はバイト一致）。
      定数の添字を Disp に畳んで絶対番地 (`lda a+3`) で読む最適化 (`constidx`) と、struct の配列フィールドをポインタ経由で
      引く形のポインタ + 添字 + disp (`fieldptr`) も入れた。
- [x] **演算命令の幅と符号** ✅ 2026-09-28 (`ir/sign.go`、development_notes.md「コードの構造」): eq / lt は比較の幅
      `Op.Width`、lt / div / mod / shift_right は `Op.Sign` を持ち、sema が作るときに型から決める (`InferWidthSign`)。
      「比較は広い方の幅、どちらかが符号付きなら符号付き」「除算は Dst の符号」「右シフトは入力の符号」を opt/ssa・unroll・
      codegen・regalloc・carry・split がそれぞれ型から導いていたのを、命令の値を読む形に (入力を差し替えても意味が
      変わらない。常駐の差し替えが cast を落として比較が符号付きになった、propagateBytes が型の違うリテラルにした、の 2 件の
      バグの類)。決め忘れは Verify が落とす (Sign は未設定 / 符号なし / 符号付きの 3 値)。生成コードは変わらない (移行中に
      「命令の値 = 型から導いた値」を Verify で全段・fuzz で確かめた)。**zext / sext / trunc の命令化はしない**: 暗黙の変換
      (ゼロ拡張と切り詰め、リテラルは値のバイト) は規則が 1 つで各所の実装も 1 つずつ、符号拡張は元から命令
      (`sign_extension`)。命令にすると IR が膨らみ、codegen が畳み直すことになる。sema の定数の畳み込み (式の型の規則) と
      interp (差分テストの独立した判定役) は型から自分で導く
- [ ] **sema の式に型付きの中間表現を**: `lval`（470 行）が型検査・暗黙変換・診断・IR 出力を同時にやり、式の型は IR を出す
      まで分からない（`f() == .A` の二重評価はこの構造の結果）。「検査して型・定数値・左辺値性を持つ木を作る段」と「IR を
      出す段」に分ける。あわせて sema が自分の Symbol / Scope を持ち、`ir.Scope` と `ir.Lambda.Body`（AST）を ir から出す。
      `ir.Value` は置き場所とリテラルだけに。`Hlc` の寿命の違う状態（モジュール / 関数 / 式）も分ける
- [ ] **types の Kind**: slice（Struct + SliceOf）、enum（Int + Enum）、soa（Array + IsSoa）、far な関数（Func + far）を
      独立した Kind に（Kind で分岐する所は全部フラグの検査も並べている）。`Compatible` を `Identical` / `AssignableTo` /
      `CommonType` に分ける。`NamedIn(name, version)` の版番号は Parse 直後に fc 2 → fc 3 の正規形へ書き換える段を置けば要らない
- [x] **ループ解析**: `ir.DomTree`、`Loop.Parent / Depth`、`CFG.Preheader`、`Loop.EveryIteration` と、induction / unroll が
      共有する `loopHeader` / `loopDefs` / `singleStep`（opt/loopmatch.go）✅ 2026-09-28。ywalk は回転後の線形の形を見るので別のまま
- [ ] **テストの共通部品**: 「TempDir に書いて Build」は `harness_test.go` の `testBuild` に寄せた（✅ 2026-09-28。包み関数は
      名前を残して中身だけ共通に）。残り: ランダム生成器（約 3,000 行）を `internal/fuzzgen` に出せば `tools/fuzzmeasure` が
      `go test -json` を経由せずに直接呼べる
- [ ] **driver の `Compiler`** はビルド単位の状態（ctx / target / dir / buildDir / prog / layout）をフィールドに持つので同じ
      Compiler で並行ビルドできない。ビルドごとの struct に分け、ld65 のメモリ配置（ZP / SRAM の番地が 3 か所に直書き）を
      `MemoryMap` から ld65.cfg と base.s の両方に出す
- [ ] sema の `Loader` が直接ディスクを読む（`fs.FS` にすればテストがメモリ上で済む）。`fc3Seeds` が syntax と driver の
      fuzz に同じ内容で 2 つ（`r6502.Memory` の配列化は ✅ 2026-09-28）

## 整理・判断待ち

**記録のみ・修正しない（2026-09-20、ユーザー方針）:** interrupt 属性では、暗黙の乗除算・剰余ルーチンが
使う共有 `reg` を保護できず、割り込みによって通常処理の演算結果を壊し得る。
タイミング上の要求から interrupt 属性はほぼ使わない想定のため、現時点では対策を実装しない。
詳細は [V3 メモ §6 の既知の問題](v3_plan.md) を参照。

- [x] **v1 パーサの削除**: `fcc migrate` / `internal/migrate` / `.rb` マクロの互換 / `test/*.fc` の v1 版を削除。
      `#fc 2` は任意に、`#fc 1` はエラー。v1 だけの構文は「v2 ではこう書く」のエラーのまま残す ✅ 2026-09-19
- [x] `memo.txt`（初期の TODO メモ）は追跡から外した ✅ 2026-09-28
- [ ] `examples/castle` と実プロジェクト `C:\Work\castle` の同期（`tools/sync_examples.ps1`）と公開可否
- [ ] castle 側（**SSA が終わってからまとめて反映**。2026-09-19 決定）: examples/castle に入れた変更（`data.asm` の
      `FC_SZP` / `FC_SRAM` / `FC_SP`、`mmc3.fc` の `options(static_zp:, static_ram:)`、`ppu.fc` のスプライト消去）、
      `en.process` のバンク切り替えを「変わるときだけ」に、`bg.cell_type` / `bg.cell` の `options(inline: true)`、
      NSD の `options(abi: "cc65")` 化、far call のラッパ撤去（v2_farcall.md §6）、en.fc の soa 化の実験。
      その後 feature/static-frame → feature/v2 のマージ
- [ ] ca65 / ld65 は当面維持（内製アセンブラはやらない。2026-09-14 決定）
