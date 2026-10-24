# 24 / 32 ビット整数（u24 / i24 / u32 / i32）の計画

2026-10-24 にユーザーから依頼（「fc に 24bit 整数と 32bit 整数を入れてほしい。プランして」）。コードを読んで調べた結果（実行はまだ）と、
進め方の案。未決の項目は「決めること」にまとめた。

## 今の状態（2026-10-24、コードを読んで確かめた）

- **型の表現はバイト数で一般化済み**: `types.IntType(size, signed)`、名前の表示（`%s%d`）、`CommonType`、struct / soa の配置、
  `@sizeof`、`@bitcast`、`ir.Value.Int`（Go の int）、ssa の定数畳み込み（`normBits` / `evalConst`）、codegen の加減算・論理・比較・
  `if`・否定・シフトの一般形・2 のべきの乗除・load / store・呼び出しの引数と戻り値の写し、fclog の値の読み取り。
  どれもバイト数のループで書いてあり、3・4 バイトでは**一度も実行されていない**（slice・farfn の 3〜4 バイトの写しは通っている）
- **名前が無い**: `types.IntTypeNames` は u8 / i8 / u16 / i16 だけ
- **壊れている所（3・4 バイトで黙って間違える）**
  - `sign_extension` が元の幅を持たない（元は 1 バイト前提）: codegen の `genSignExtension`（TODO のまま 2 バイト目までしか書かない）、
    ssa の畳み込み（`normBits(a, 1, true)`）、interp（2 バイトを超えると unsupported）。i16 → i32 は符号を取るバイトから違う
  - 乗除算のランタイム呼び出し（`codegen/arith.go` の `mulDivMod`）: 名前が `"8"` / `"16"` の決め打ちで、オペランドを `reg+0` /
    `reg+2` に置くので 3・4 バイトでは重なる。`share/runtime.asm` に 24 / 32 ビットの乗除算が無い。interp も乗除算は 2 バイトまで
  - 定数データ（`codegen/data.go` の `dataDirective`）: `.byte` / `.word` だけで、u32 の配列・フィールドの初期値は内部エラー
- **sema の規則が `Size <= 2` で止まっている**（3・4 バイトは検査も広げもせず素通り）
  - 代入の検査（E: 定数の範囲 / D: 暗黙の狭め。`convert.go`）、A1 の広げ（`widen.go` の `wideWidth` / `compareWidth` / `rvalWide`）、
    型のない定数の扱い（`literal.go` の `literalRule`・`litSigned16`・`foldBeyond16`、`consteval.go` の 16 ビットの境界）、
    `?:` の合わせ（`cond.go`）、F4 の配列の要素型（16 ビットに入らなければエラー）、F6 の文言（128 / 32768 の決め打ち）、
    switch の case の範囲、`warnConstCompare`
  - 型のない定数の既定の型（`IntValue`）: 256 以上はすべて u16（100000 も u16 の型で値はそのまま）。`{:x}` に 0x12345 を渡すと
    `2345` と出る（今もある食い違い）
  - 黙って狭くなる所: `[:u32]T` が普通の slice に、`@slice(p, n32)` の長さが下位バイトに、外部マクロの `"u32"` が u8 に
    （`externalIntType`）、starmacro は `AsInt32` で 2^31 以上が失敗、fc 2 / 3 の `printf` は全部 `print_int16`
- **出力**: `@format` / `@printf` は `fmt.dec_u32` などを名前で呼ぶ（sema は一般化済み）が、fclib/fmt.fc に 24 / 32 の関数が無い。
  `@log` は 3 バイト以上の整数を型のエラーにしている（安全）
- **最適化の速い形は 2 バイトまで**（3・4 バイトは飛ばすので正しいが遅い）: バイト単位のシフト（`shiftByte`。u32 の `>> 24` が
  24 回 × 4 バイトの ror になる）、メモリ上の inc / シフト（regalloc の forms）、`splitWords`、`narrowBitTest`、`carryBranch`、
  2 のべきの検出（128 まで）、`expandMul` の費用の見積もり
- 常駐（A / Y / X）は 1 バイトだけなので、3・4 バイトの値は常にメモリ（変更不要）。cc65 ABI は 3 バイト以上を診断で断っている

## 決めること（ユーザーに確認）

1. **版**: fc 4 だけにするか（案: fc 4 だけ。fc 2 / 3 の `printf` と暗黙の狭めが黙って切り詰めるため。fc 3 では予約語になるだけ）
2. **型のない定数の既定の型**: 65536 以上 / -32769 以下を u24 / i24（さらに大きければ u32 / i32）にするか、24 を飛ばして u32 / i32 にするか
   （案: 入る一番狭い型。今の u8 → u16 の段と同じ考え）。32 ビットに入らない定数は、定数の畳み込みの中では正確に持ち、実行時の値に
   なる所でエラー。**今 u16 として扱われている 65536 以上の定数の意味が変わる**（`x16 * 70000` が 24 ビットの計算になる）
3. **A1 の広げを 3・4 バイトにも当てるか**: `var w:u32 = a16 * b16` を 32 ビットで計算する（仕様の「代入先の型と式の中で一番広い型」
   のとおり。6502 では遅いが、書いた人が u32 を選んだ結果）か、広げは 16 ビットで止めて `as` を求めるか（案: 仕様のとおり当てる）
4. **添字・ポインタの足し算・slice の長さに 3・4 バイトの値**: エラーにして `as u16` を求めるか、黙って 16 ビットに切るか（案: エラー）
5. **ランタイムの乗除算**: 24 / 32 ビットの関数を使ったときだけ ROM に入れるか（案: 別のオブジェクトにして使う時だけリンク。
   今の ROM・golden・castle をバイト単位で変えない）。u24 の乗除算は 32 ビットの関数で代用してよいか（案: 最初は代用、後で専用）

## 進め方（案）

各段で `go test ./...` と日付の種の fuzz を通してからコミットする。段 1 と 2 は一緒に入れないと書けない（名前が無いとソースで試せない）ので、
段 1 の確かめは段 2 の型名を入れた後のテストで行う。

### 段 1: 後段の正しさ

- `sign_extension` に元の幅を持たせる（IR の `Op.Width` か元の値の型から。`ir.Verify` を合わせる）→ codegen（元の w バイトを写し、
  上を w-1 バイト目の符号で埋める）、ssa の畳み込み、interp
- 乗除算: `share/runtime.asm`（か別ファイル）に `__mul_32` / `__div_32` / `__div_32s` / `__mod_32` / `__mod_32s`（16 ビットの形を伸ばす）。
  FC_REG は 16 バイトなので、商を被除数の場所に作る形で 12 バイト + 符号の印に収める。X を壊さない約束（`codegen/asm.go` の
  `jsr __mul_` の例外、m6502）を守る。`mulDivMod` の名前とオペランドの配置を幅ごとに。interp の乗除算
- 定数データ: 3 バイトは `.faraddr`（か `.byte` 並び）、4 バイトは `.dword`

### 段 2: 型と sema の規則（fc 4）

- `IntTypeNames` に u24 / i24 / u32 / i32（版の判定を入れる）
- `IntValue` の段を 24 / 32 ビットまで。`litSigned16` → 定数の大きさの符号付きの型、`foldBeyond16` と 16 ビットの境界 → 32 ビットの境界
- `Size <= 2` の見張りを外す: convert（E / D）、widen（A1）、literal、cond（`?:`）、F4、switch の case の範囲、`warnConstCompare`。
  F6 の文言を `1 << (8*Size-1)` に
- 3・4 バイトを断る所: slice の長さの型（`[:u32]`）、`@slice` の長さ、添字・ポインタの足し算（決めること 4）
- 外部マクロ（`externalIntType`・`extmacro.intRange`）、starmacro（int64 に）、`@log` の型の検査を 4 バイトまで

### 段 3: 出力

- fclib/fmt.fc に `dec_u24` / `dec_i24` / `dec_u32` / `dec_i32`（10 の累乗の引き算を 10 桁に）、`hex_u24` / `hex_u32`、`bin_u24` / `bin_u32`。
  `pbuf:[32]u8` の大きさの約束を見直す（32 桁の 2 進数）。`@(test)` を足す。docs/reference/std/fmt.md は `fcc doc -md` で作り直す

### 段 4: テストと fuzz

- golden: `test/test_wide.fc`（加減算・比較・シフト・符号拡張・乗除算・`@printf`・定数データ）
- TestTypeRuleMatrix の `trTypes` に足して typerules.txt を作り直す
- 模型のある生成器（ここで sema の誤りが見つかる）: TestRandomArithV4（`arMinType` / `arAdaptLit` / `arTooWide` / `tyOf` /
  境界の値。型のない定数の積が int64 をはみ出すので大きさを抑えるか `math/big`）、TestRandomConstFold（`widen`、出力の `as i16` を
  広く、シフト量 0〜31）、TestRandomFormatV4（型と境界の値を足すだけ）
- 差分の生成器 rpGen: 型・リテラル・シフト量・`mixedSign` を 4 バイトまで。fc 2 の `printf` では出せないので、出力は 16 ビットの
  半分ずつに分けて出す（or v4 の経路だけで使う）。TestRandomMutate の型の入れ替え。機能の名前（例 `wide32`）を `rpFeatures` に
- 直したバグは bugzoo に、生成器で避ける既知のバグは known_bugs_test.go に（root の AGENTS.md の約束）

### 段 5: 速さ（後回しでよい）

- バイト単位のシフト（`shiftByte`・`shiftInMemory`・regalloc の shift / inc の形）を N バイトに。いちばん効く
- `splitWords` を N 個に、`narrowBitTest` / `carryBranch` の 3・4 バイトの印、2 のべきの検出を 128 より上に、`expandMul` の費用
- u24 専用の乗除算、fclib/math.fc に `mul_u16`（u16 × u16 → u32）・`abs_i32` など
- `aggcopy` が 3 バイト以上の局所変数を集まりとして扱う（Kind を見ない）ので、整数でも動くことをテストするか Kind で除く

### 文書

- docs/reference/language.md: 型の表、計算の型（型のない定数の段）、A1 の文言（「16 ビットの代入先」など）、F4 / F6、`?:`、添字、
  slice の長さ、`@bitcast`。docs/reference/assembly.md: 3・4 バイトの引数と戻り値（cc65 ABI は断ったまま）
- Agent/wiki/fc4-facts.md に実行して確かめた挙動、設計は済んだら `design/` へ

## 見積もり

大きい順に: 乗除算（ランタイム + 配置 + 使う時だけリンク）、sema の規則の見直し（見張りを外した後の検査の挙動を模型の fuzz で詰める）、
符号拡張（IR から interp まで）、fmt の出力、生成器。段 5 は別の仕事として扱える。
