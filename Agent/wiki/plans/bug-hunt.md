# fc コンパイラのバグ調査計画（bug hunt）

作成: 2026-10-18（ユーザーの「コンパイラにバグがないかを詳しく調査したい」の相談）。**進行中の計画**なので済んだ項目は消し、見つけた事実は [../testing-and-fuzzing.md](../testing-and-fuzzing.md) と [../discussions/](../../discussions/AGENTS.md) へ落とす。

## 0. 方針

既存の基盤（差分 fuzz・bugzoo・IR 検証器・asm のレジスタ検査）は**すでに強い**。だからこの調査の主役は「もっと回す」ではなく、**今のオークル（正しさの判定）が構造的に見つけられないバグの穴を特定して、そこ専用の判定を足してから回す**こと。

- 判定の穴 = ①実行系が 1 系統（内蔵 6502 のみを信じている）②sema / 型規則の誤りは 3 判定に共通して現れない ③生成器が出ていない言語機能 ④テスト自体が黙って空になる（2026-09-30 に bugzoo 22 中 16 が空振り、TestRandomConstFold が 0 本で通っていた実績）
- 各フェーズは「**穴 → 判定 → 回す → 記録**」の順で完結させる。判定を増やさずに本数だけ増やすフェーズは Phase 1 で打ち切る
- 直すときは必ず bugzoo に patch を足す（[../testing-and-fuzzing.md](../testing-and-fuzzing.md)「バグを直したら、そのバグを戻すパッチを足す」）。足さない修正は「また同じ穴に戻る」

## 1. ベースライン（2026-10-18 にこの開発機で実行して確認）

| 項目 | 実測 |
|---|---|
| `go test ./...` | **全緑**（Linux、go1.27.1、ca65/ld65 は `~/.local/bin`。driver 22 秒、nes 13.5 秒） |
| 本体の規模 | 非テストの Go 51,716 行（sema 13,936 / syntax 6,598 / opt 6,542 / codegen 4,802 / ir 3,410 / regalloc 2,732） |
| panic 箇所 | 494（sema 404、codegen 35） |
| bugzoo | 33 patch（`testdata/bugzoo/`） |
| fuzz の生成器 | `internal/driver/rand*_test.go` 15 本、`rpFeatures` の機能 42 |
| Go native fuzz | FuzzParse / FuzzFormat（syntax）、FuzzCheck（driver）。コミット済みの種 25 件 |
| CI | ci.yml = ubuntu + windows の `go test ./...`（**`-race` は無い**）、fuzz.yml = 毎晩 1500 本 + go fuzz、日曜 bugzoo |
| Mesen / QuickNES | **この機では必ず Skip**（MesenCE と libretro の DLL が Windows のみ）→ 実機描画・scanline IRQ の見張りが Linux で完全に効いていない |
| 実行系 | emu も内蔵 NES ランナーも同じ `internal/r6502`（`internal/m6502` は命令の性質の表で、エミュレータではない） |
| メモリ | 総 123GB / 空き 34GB → fuzz はメモリ上限つきで回す（過去に OOM でセッションが落ちている） |

## 2. 穴の一覧（どこにバグが残りうるか）

### A. 実行オークルが 1 系統

`-O 0` / `-O 2` / IR インタプリタのうち 2 つが 6502 を走るが、その 6502 は内蔵の 1 実装だけ。**r6502 のバグは codegen のバグとして見え、逆に r6502 が間違っていると codegen の誤りが隠れる**（実際に `plp` が S を戻していなかった例がある）。QuickNES / Mesen は Windows のみ。

### B. fc 4 の整数規則に Go の模型が無い

sema の誤りは 3 判定に共通して現れないので、模型は Go 側で書く（randmethod / randformat / randgrid / randstr / bank が採用している形）。**fc 4 で書き換えたばかりの式の型と値の規則（A1・F1、暗黙の変換、符号の混ざった演算、変数の回数のシフト）には模型の生成器が無く**、`mixedsign` / `widen` は差分（=3 判定が同じ間違いをしたら見えない）と `TestTypeRuleMatrix`（=通る/エラーの表で、値は見ない）だけ。

### C. 生成器が出ていない言語機能（`rpFeatures` に 1 つも無いもの）

- `@(volatile)` と MMIO（NMI に割り込まれる側の読み直しを最適化が消さない・並べ替えない）
- `@log`（2026-10-13 に古い値を出す実バグが出た所。規則は [../log-annotation-rules.md](../log-annotation-rules.md)、生成器に形が無い）
- 配置: `@(address:)` / `@(zeropage)` / `options(static_ram:)` の境界 / BSS / バンク超過（=リンクのエラーを出す側）
- `@incbin` / `@rle` / `@textmap` / `@include` / `asm` の張り込み（fclib の `.asm` は手書きで fuzz されない）
- `@sizeof` / `@len` / `@ptr` / `@slice` / soa の `@len` の組み合わせ
- マクロ（Starlark・外部マクロの定数プロトコル）と `fc.toml` の `[lib]`（path / git / rev / dir）・`fc.lock`
- マッパー: fuzz は MMC3 と UXROM のみ。MMC1（5 ビットシリアル）・CNROM（CHR 切替）・NROM・4-screen・CHR RAM
- 割り込み（NMI / IRQ）と再入、割り込みから届く far call の禁止の判定
- CLI の `fcc watch` / `fcc migrate` / `fcc test -t nes`（カバレッジ調査でも「通っていない」と記録されている）

### D. コンパイラ自身の堅牢性と決定性

FuzzCheck は構文〜コード生成まで（ca65 / ld65 / emu を通らないので、リンクがらみの経路と大きいビルドの回復点を見ない）。`-race` が CI に無い（ca65 の並列、`fcc watch`、QuickNES の大域錠）。同じソースの 2 回ビルドのバイト一致、`Jobs` を変えた一致、相対/絶対パスの正規化（[../../discussions/2026-10-06-build-run-split.md](../../discussions/2026-10-06-build-run-split.md)）を見るテストが無い。

### E. 変更直後の周辺（歴史的にここが出ている）

fc 4 の migrate（ROM バイト一致が要求される）、interface / soa / far interface（2026-10-06）、`@log` のゼロコスト化（2026-10-13）、強さの低減（2026-10-08）、静的フレームの RAM の予約（2026-10-08）。

## 3. フェーズ

### Phase 0 — 計測（0.5 日・機械待ち中心）

1. `go test ./... -coverpkg=./... -coverprofile=cover.out` を集計し、**コンパイラ本体で通っていない block の上位パッケージ**を出す（エラー文言以外が対象）
2. `go run ./tools/fuzzmeasure zoo -randn 300 -foldn 150` をこの機で回し、**空振りしている bugzoo patch の一覧**を出す（2026-09-30 に 16/22 が空振りしたきり、fc 4 の作り直しでまたずれている可能性がある）
3. `go run ./tools/fuzzmeasure cover` で生成器の機能ごとの固有経路を表にする
4. 結果をこのファイルの「4. 結果」に追記（空振り patch は Phase 2 の優先順位の材料にする）

### Phase 1 — 既存の fuzz を長く（1 日・機械待ち）

- `TestRandom*` を種を替げて **20,000 本**。32 コアあるので種番号の範囲を分割したプロセスを 6 つ（`-randseed` を 100 万刻みでずらす）。`-timeout 120m` を付け、`systemd-run --scope -p MemoryLimit=8G` で回す
- `TestRandomBankPrograms -randn 2000`、`-metan 500`、`-mutn 2000`
- `FuzzCheck`（2 ワーカー）+ `FuzzParse` + `FuzzFormat` を 3〜4 時間（`setsid` で起動し、止めるときはプロセスグループごと）
- 失敗は `scratchpad/fuzz/<日付>/` に種番号・ログ・最小化後の `.fc` を保存。**この段階で直さない・疑わない**（擬陽性の切り分けは Phase 6）

### Phase 2 — 穴を塞ぐ生成器（2〜4 日・効果の高い順）

| # | 追加する判定 | 穴 | 受け入れ条件 |
|---|---|---|---|
| 1 | `TestRandomArithV4`: fc 4 の式の型と値を **Go で計算**し、-O 0 / -O 2 / インタプリタ / 定数畳み込みと比べる。型の全組合せ（u8/i8/u16/i16/enum/bool × 演算 × 変換）はランダムでなく**網羅の表**を 1 プログラムに固めて golden 化し、ランダムは境界値（0、1、0x7F/0x80/0xFF、15/16 のシフト）に絞る | B | 型の表の golden ができ、bugzoo の `untyped-cast` / `fold-typed` / `valint-width` / `cmp-fold-mixed` の少なくとも 3 つを newly 見つける |
| 2 | volatile / NMI: 読み出し回数と順序を出力に書く自己検査（同じことを 2 通りに計算して違えば `exit(77)` の形）。NMI の中で書く値とメインループの変数を共有する形 | C | 最適化で volatile の読みが消えたら落ちることを、わざと消す patch で確認 |
| 3 | `@log`: 生成したプログラムで -O 0 と -O 2 の `@log` の出力が同じ意味（同じ変数の同じ時点の値）であることを検査 | C | 注釈を落とす・古い値を出す patch を見つける（`log-annotation-rules.md` の規則の実行可能な見張り） |
| 4 | 配置とリンク: `@(address:)` / `@(zeropage)` / `static_ram` / バンク超過を生成して**診断を出す側**を検査（TestMustError の表を生成器で拡張） | C | 区画あふれ・重なり・ZP のページ越えの診断が、位置と数を伴って出る |
| 5 | マッパー拡張: MMC1（5 ビットシリアル）と CNROM（CHR 切替）を [bank fuzz](../implementation-notes.md) に足す（`fclib/nes/farcall_mmc1.asm` を通す） | C | 規則を切ると落ちることを確認（MMC3 のときと同じ手順） |
| 6 | `@incbin` / `@rle` / `@textmap` / `@include` / `asm` 張り込みのスモーク（生成でなく表でよい）と `@sizeof` / `@len` / `@ptr` / slice の組み合わせ | C | 各機能の golden が 1 つ以上、fclib の `.asm` を通る |
| 7 | `fc.toml` の `[lib]`（一時ディレクトリに作った git リポジトリを含む）と `fc.lock` の解決、マクロの `FuzzMacro`（壊れたマクロで panic しない・戻り値の型検査） | C/D | 壊れた入力でコンパイラが panic しない |
| 8 | 壊れたプログラム（範囲外添字・0 除算・未初期化・あふれ）を**わざと**生成し、「通さない・診断が正しい」側を見る | C | 通ってはいけない表が生成器で増える |

### Phase 3 — 第 2 の実行系（2〜3 日・穴 A）

1. **`m6502` の表と `r6502` の実装を突き合わせるテーブルテスト**（安い方から）。命令 × アドレッシングモード × 境界入力で、書くレジスタ・N/Z/C/V・サイクル数を照合。`plp` が S を戻さない種のバグを構造的に潰す
2. **独立実装の 6502**（公式の表から新規に書く。`r6502` のコードも `m6502` の表も引かない）を `internal/r6502` と同じ口（`Bus`）で実装し、emu の `a.bin` と NES の ROM を両方で走らせて出力と最終状態が一致することを fuzz の 4 つ目の判定に加える
3. NES 側: Linux で動くコア（QuickNES の Linux ビルド、または cgo の要らない別コア）を `internal/quicknes` の口に接続するか、**Windows ジョブで castle の前後比較を CI に入れる**（現状 Linux では完全に暗い）

### Phase 4 — 不変条件の検査器（1〜2 日・穴 D）

- **生成 asm の静的検査**（`internal/codegen/verify.go` に足す）: 存在しないアドレッシングモード（`TestCompareOperandModes` の一般化）、即値の幅、ca65 の 1 行 80 文字、ZP のページ越え（`r6502.TrapZpWrap` 相当をビルド時に）、未定義シンボルへの参照
- **フラグ生存検査**: 比較の結果（N/Z/C）を読む命令の前に、それを壊す命令（`lda`/`ldx`/`ldy`/`inc`/`dec`/shift/常駐の復帰）が挟まっていないかを asm レベルで追う。過去のバグ（梯子バグ、踏むスイッチ、`TestIfCallResultFlags`、`TestResidentCondRestore`、`php`/`plp`）が全部この系統で、**bugzoo にも 1 つも patch が無い**
- `ir.Verify` の項目の棚卸し（幅と符号、常駐の印、live range と退避の対応など、まだ見ていない不変条件を列挙して足す）
- **決定性**: 同じソースの 2 回ビルドのバイト一致、`Jobs = 1 / N` で一致、相対/絶対パスで一致。CI に `-race` を 1 ジョブ足す

### Phase 5 — 実プロジェクトとゲーム（1 日）

- `games/` の 12 本をフレーム上限を上げて長く AUTO で走らせ、`TestGamesSameAtTick` の本数・ゲーム数を増やす（-O 0 / -O 2 の main の BSS の一致）
- castle / miku の ROM golden、`bench` のチェックサム（出力の違い = バグ）、`bench/castle_frames.json` の 1 フレームのサイクル（退行 = 性能バグ）
- Windows が使えるなら QuickNES の `TestComparePlay` で前 ROM との画面 100 枚比較、Mesen の `TestMesenPlayCastle`

### Phase 6 — トリアージと記録（常設）

1. 失敗したら **まず擬陽性を疑う**: 最小化でローカル配列の初期化が消えていないか / ソフトウェアスタックのあふれ / `-O 0` だけのサイクル上限 / フレームの位相 / ca65 の並列の一時的失敗
2. 切り分け: `-randn 1 -randseed N` で再現 → `rpLocate`（最初に出力が変わった opt の段）→ `FC_DISABLE=パス` → `FC_DUMP_IR=1` → **症状の行の `.s` を読む** → invalid opcode なら `FC_TRACE_PC=1` + `.dbg`
3. 直したら: bugzoo に patch → 生成器に形（`rpFeatures` に名前付き）→ `fuzzmeasure zoo` で効くことを確認 → [../discussions/](../../discussions/AGENTS.md) に経緯 → [../testing-and-fuzzing.md](../testing-and-fuzzing.md) に実行して確かめた事実
4. 深刻度の分類で報告する: `ROM の出力が変わる` / `診断が間違う（通るべきでないものを通す）` / `コンパイラが落ちる` / `遅くなる・大きくなる` / `診断の文言`

## 4. 結果（追記）

### Phase 1（2026-10-18、差分 fuzz を増やした分まわした）

`TestRandomPrograms / V3 / V4 / Grid / Strings` を 4 プロセス・シードを 100 万ずらして `randn=2000`（約 8,000 プログラム × 5 生成器）。
**3-way 差分（-O0 / -O2 / 解釈系）のバグは 0 件**（実行系は堅い）。落ちたのは:

- `TestRandomV3ProgramsV4` 1 件（seed 4000146）: 生成した fc 3 プログラムが fc 4 で `?:` の両方の枝が -61624（16 ビットに収まらない）
  で弾かれる。**コンパイラは正しく、生成器が範囲外の定数式を出す**（低優先）
- `TestRandomArithV4` は**既定の種（1〜7）は緑だがその外で落ちた**（種 8 から。1000 本で 11 件）。原因はすべて
  `@min` / `@max` が 8 ビットの引数に A1 の幅を流さないバグ（`Agent/issues/local-2026-10-18-minmax-a1-width.md`）。
  生成器が 16 ビットの引数の `@min` / `@max` を作るようにして 0 本にした（バグが直ったら戻す）。
  **毎晩の fuzz（`-randn 1500`）が赤くなる状態をコミットしていた**（レビューで指摘）
- その後も種 1〜1000 でしか確かめておらず、**大きな種（1000000〜）では 1000 本中 4 本**落ちていた（2 回目のレビュー）。
  3 本は F6 の誤検出（`Agent/issues/local-2026-10-18-mixed-sign-false-error.md`）、1 本は `arMixed` が `id_T(…)` の中を
  見ていなかった生成器の穴。両方を生成器で避け、種 1・1000000・5000000・20261018000 の各 1000 本（`-randn 4000`）で
  0 本を確かめた。`const-init-widen` の patch を当てると 100 本中 13 本落ちる（判定は効いている）。
  **`-randn N` の本数は生成器ごとに違う**（TestRandomArithV4 は N / 4。「1000 本」と書くときは本数か `-randn` かを書く）

### Phase 0（2026-10-18）

**カバレッジ**（`go test ./... -coverpkg=./...`、全体で block の 88.3%）。通っていない block の上位は
`internal/syntax/ast.go`（77）・`internal/interp/interp.go`（65）・`internal/fcdoc/fcdoc.go`（49）・
`internal/extmacro/extmacro.go`（46）・`internal/starmacro/starmacro.go`（39）・`internal/ir/verify.go`（30、56.5%）。
`fcdoc` とマクロの 2 つは「テストが薄い」より「fuzz がマクロを触らない」が原因（穴 C と同じ）。`ir/verify.go` は検査器そのもので、
検査している式の種類が足りない。

**bugzoo**（`go run ./tools/fuzzmeasure zoo -randn 300 -foldn 150`、34 patch + 当てない基準。2026-09-30 の 16/22 空振りから大きく改善）:

| 状態 | 数 | patch |
|---|---|---|
| ランダム生成器が見つける | 26 | （それ以外） |
| 文字列の生成器だけが見つける（zoo の `-run` の名前に無いので「その他」に出る） | 3 | `interp-array-eq`、`string-signed-bytes`、`zero-size-eq` |
| 決まった単体テストだけが見つける | 4 | `fwdmem-disp-after-replace`（TestForwardFields）、`memshift-narrow`（TestMemShiftNarrowCast）、`migrate-conv-order`（TestV4MigrateWidenThenConvert）、`wrap-untyped-operand`（TestWrapOps） |
| **誰も見つけられない** | 1 | `enum-short-twice` |

`enum-short-twice` は、直した場所（`evalOnce`）が fc 4 では型を決める段（`resolveEnumShortPair` の `exprType`）に置き換わって
通らない経路になっていた。同じバグの今の形（型が決まっている側を型を知るために評価して捨てる）を当てる patch に**書き換えた**
（当てると `TestV3EnumShortCallOnce` が `b5` になって落ちることを確認）。**教訓: patch は「その規則が生きている所」を壊さないと
死にます**（コードの作り直しのたびに `fuzzmeasure zoo` で確認する）。

**見つけたバグ**: 定数の式を代入先の幅で畳み込んでいなかった（A1 の抜け。`const C:u16 = (128 as u8) + (200 as u8)` が 72）。
`TestRandomArithV4`（Phase 2 の 1 番）の種 1。詳細と規則は [2026-10-18 の記録](../../discussions/2026-10-18-arith-model-and-const-widen.md)。

### Phase 2 の 1（TestRandomArithV4、2026-10-18）

`internal/driver/randarith_test.go`。fc 4 の式の型と値の Go 模型を作り、生成した式の実行結果（-O 0 / -O 2）と
同じ式の型つき定数の形（畳み込み）を同時に比べる。15,000 プログラムで通っている。
受け入れ条件の「bugzoo の `untyped-cast` / `fold-typed` / `valint-width` / `cmp-fold-mixed` を newly 見つける」は
これから測る（`fuzzmeasure zoo -bugs untyped-cast,fold-typed,valint-width,cmp-fold-mixed`）。

**測った結果（2026-10-18）**:`fold-typed` は TestRandomArithV4 も見つける（他に TestRandomConstFoldV4 139 件と単体テスト）。
`fold-typed` は TestRandomArithV4 も見つける。**`valint-width` を見つけたのは誤り**で、その種でパッチ無しでも
落ちていただけだった（詳しくは `Agent/discussions/2026-10-18-arith-model-and-const-widen.md` の その 3）。
`valint-width` は経路を持つプログラムが今のテストに 1 つも無かったので、`internal/ir/value_test.go` の契約の単体テストで
見張るようにした。`untyped-cast` と `cmp-fold-mixed` は今までどおり他の生成器が見つける。

受け入れ条件の「`fuzzmeasure zoo` の既定の `-run` に入る」は `TestRandomArithV4` を追加した（2026-10-18）。

### bugzoo を新しい方法論で測り直した（2026-10-18、34 patch + `none`）

`none` 行は完全にクリーン（= 誤検出なし）。**33 patch は何かが見つける**（`internal/ir` などの単体テストも数えるようにした）。
**`untyped-cast` だけ誰も見つけられず、ガード自体が発火 0 回**（golden + examples + bug 回帰 + ランダム 250 本 × 3 生成器で
`explicitCast` の「型のない定数への同じ型の `as`」の節に行くプログラムが 1 つも無い）。`@min` などを定数式で畳むようにした
（`8c3f111`）ので、`(@min(0, -6) as i8) + x` が型付きの定数になって経路が塞がった。ガードのコードは防御として残し、
**patch は削除**（何も見張れない patch は表のノイズ）。

### レビュー（2026-10-18、他のエージェント）で出て直したもの

- TestRandomArithV4 が既定の種の外で落ちていた（上の通り。毎晩の fuzz が赤い）→ 直した
- `volPress`（volatile の文）を**どこからも呼んでいなかった**（`stmt` の `pick(21)` に対応する `case` が無く、
  `k=20` が `default` の代入に落ちていた）→ `case 20` を追加
- bugzoo の測り方に誤検出の穴（`none` 行を見ない / `internal/driver` 以外を見ない）→ 両方ふさいだ
- 「生成器の模型の穴」と分類した `mixes i16 and u16` は、**コンパイラの誤検出**だった
  （`a - (b >> 2)` が通らない。`Agent/issues/local-2026-10-18-mixed-sign-false-error.md`）
- 挙動の変更（`const C2:i16 = -(100 as u8);` が通らなくなった）を記録していなかった → `fc4-facts.md` に足した

**この生成器が見つけたバグその 2**: `@min` / `@max` / `@clamp` が定数式で畳めず、`const D = @min(3, 5);` が
「const D must be constant」、`var arr:[@min(4, 8)]u8;` が「array size must be constant」になっていた（fc 3 から同じ。
純粋マクロを `constEval` が展開していなかった）。回帰テスト `TestBugConstMinMax`、bugzoo は `macro-const-minmax`。


### Phase 2 の 2（volatile、2026-10-18 に途中まで）

生成器に機能 `volatile` を追加（グローバルのいくつかを `options(volatile: true)` にし、その読み・書き込み・呼び出しを混ぜた
ループ `volPress`）。**ただし `Volatile` のフラグを外しても出力が変わらなかった**（手書きの `poke()` を挟むループでも同じ）。
`Volatile` を見る規則（`opt/licm.go`・`regalloc/resident.go`・`opt/litprop.go` など）は、呼び越しの不変量として読みを
持ち上げたり、常駐レジスタに置いたりする形でしか効かず、**emu には外部の書き手が無いので値がずれない**。
volatile が本当に効くのは「割り込み・asm が同じ場所を書く」形で、それには NES ターゲットで NMI を扱うか `@asm` の書き手が要る
（`pipeline.markVolatile` は asm が参照するシンボルを勝手に volatile にするので、`markVolatile` を外す patch を見張る形にすると
テストできる）。NMI 側は引き続き Phase 2 の 2 のまま残す。

## 5. やらないこと

- **未定義動作の自動検出（範囲外添字の trap など）は作らない**。仕様決定が必要なので、見つかったら Issue にして「診断を出す」側に寄せる
- 新しい最適化の規則は入れない（退行とバグが混ざって判定できなくなる）
- 生成物（`internal/syntax/parser.go`、`testdata/golden/`）を直接編集しない。golden は差分を読んでから `-update`（意図しない差分を消さない）
- 推測で直さない。実行して再現したものと、再現コマンドだけを記録する

## 6. 完了条件

- bugzoo の空振りが 0（全 patch を何らかのテストが見つける）
- Phase 2 の生成器が `fuzzmeasure zoo` の既定の `-run` に入り、それぞれが対応する patch を 1 つ以上見つける
- 6502 の 2 系統目が命令・モード・境界値を網羅して一致
- 20,000 本 × 4 判定 + native fuzz 4 時間で、新規の失敗が 0 か、Issue 化済み
- 報告（`Agent/discussions/<日付>-bug-hunt-report.md`）に、見つけた件数・内訳・直したもの・意図的に残したものを書く
