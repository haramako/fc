# 24 / 32 ビットの整数（u24 / i24 / u32 / i32）

2026-10-24 に入れた（決めた経緯は [../../discussions/2026-10-24-wide-int.md](../../discussions/2026-10-24-wide-int.md)、
実装の記録は [../../discussions/2026-10-24-wide-int-impl.md](../../discussions/2026-10-24-wide-int-impl.md)）。利用者向けの仕様は
docs/reference/language.md の「24 / 32 ビットの整数」と「整数の規則」。

## 仕様（決めたこと）

- **fc 4 だけ**。fc 2 / 3 のモジュールで型名を書くとエラー（`types.WideIntTypeNames`、sema の `basicType`）。fc 3 では名前が予約される
- **型のない定数は入るいちばん狭い型**（sema の `IntValue`。65536 以上は u24、16777216 以上は u32、-32769 以下は i24、-8388609 以下は i32）。
  32 ビットに入らない定数は、定数どうしの畳み込みの中では正確に持ち（Go の int）、型付きの値との演算はエラー（`litTooWide`）、
  比較は数学の値で畳む（`foldBeyond`）
- **A1 の広げは 3・4 バイトにも当てる**（`var w:u32 = a16 * b16` は 32 ビットで計算する）
- **添字・ポインタの足し算・slice の長さ（`[:u32]`・`@slice`）に 3・4 バイトの値はエラー**（`planIndex`・`planArith`・typeexpr・slice.go）

## 仕組み

- **「16 ビットを超える定数」の境界は版ごと**: `maxIntSize`（fc 4 は 4、fc 2 / 3 は 2）。`literalRule`（`maxSize` を受け取る）・
  `foldBeyond`・consteval の比較の畳み込み・`IntValue`・`?:` の合わせ・F4 が使う。fc 2 / 3 の挙動は変わらない。
  型の大きさの見張り（E / D・A1・switch の case・`warnConstCompare`・`@log`）は `maxIntBytes`（4）で、3・4 バイトの型は fc 4 にしか無い
- **比較で相手が符号付き、定数が大きい型**（`litSignedWide`）は定数の大きさの符号付きの型（`s < 300` は i16、`s16 < 70000` は i24）
- **fc 3 → 4 の migrate**: fc 3 は 16 ビットに入らない型のない定数を u16 / i16 の型で実行時に折り返すので、型付きの値との算術に
  `as` を報告する（`rewriteBeyond16`。TestRandomMigrate で見つかった）。両方が定数の形は畳み込み（consteval）で報告する
  （型付きの定数と型のない定数のとき。TestMigrateBeyond16Folded）。相手が同じ大きさで符号の違う整数なら、fc 3 の結果は符号付き
  （C0）なので式全体にも `as` を付ける（`(u * ((-40807) as i16)) as i16`。付けないと fc 4 では符号の混ざった式で `/` などがエラー）
- **sign_extension は入力の型の幅から広げる**（`ir.SignExtensionWidth`。1〜Dst − 1 バイト、`ir.Verify` が見る）。codegen は入力の w バイトを
  写して最上位のバイトの N で残りを埋める。ssa の畳み込み・interp も同じ幅
- **乗除算**: 3・4 バイトは share/runtime_wide.asm の `__mul_32` / `__div_32(s)` / `__mod_32(s)`（a = reg 0..3、b = reg 4..7、結果 reg 8..11、
  余り reg 12..15。X を保つ。符号付きの印はスタック。除数が 2^31 以上で余りが 33 ビットになる分も扱う）。3 バイトは 4 バイトに広げて渡し
  （符号付きは符号で埋める）、下位 3 バイトを使う。**使ったビルドだけリンクする**（codegen の `Llc.WideRuntime` → driver が
  runtime_wide.asm をアセンブル・リンク。macro.inc の import は使わなければオブジェクトに残らない）ので、使わない ROM はバイト単位で同じ
- **速い形**: 定数シフト（`shiftWide`: 8 ビットごとはバイトの移動、残りは動いたバイトだけ回す）、256 以上の 2 のべきの乗除算はシフト
  （符号付きの割り算は床除算なので算術シフト）、余りはマスク、± 1 は `FormIncMem`（2〜4 バイトの inc / dec の連鎖）。2 バイトの生成は前のまま
- **定数データ**は `.faraddr`（3 バイト）/ `.dword`（4 バイト）
- **出力**: fclib/fmt.fc の `dec_u24` / `dec_i24` / `dec_u32` / `dec_i32`（10 の累乗を引く。表 `POW10_32`）、`hex_u24` / `hex_u32`、
  `bin_u24` / `bin_u32`。sema の書式は名前を幅から作る（format.go）。fc 2 / 3 の `printf` は 3・4 バイトをエラーにする（16 ビットに黙って切れる）
- 常駐（A / Y / X）は 1 バイトだけなので 3・4 バイトの値は常にメモリ。cc65 ABI は 3 バイト以上を断る（frames の `checkCc65`）

## テスト

- `TestWideIntRun`（internal/driver/testdata/wideint.fc の出力を Python で計算して確かめた期待値と -O 0 / -O 2 / インタプリタで比べる）・
  `TestWideIntCalls`・`TestWideIntErrors`・`TestWideIntNoRuntimeUnlessUsed`
- 模型のある fuzz（sema の誤りが見える）: TestRandomArithV4（8 つの型、3・4 バイトになる型のない定数、ループで局所変数を更新する文、
  2 のべきの乗除算）・TestRandomConstFoldV4・TestRandomFormatV4。bugzoo の wide-sext-width / wide-div24-sign / wide-a1
- rpGen（TestRandomPrograms など）は fc 2 のソースなので 3・4 バイトの型を作らない

## 残り（必要になったら）

- `splitWords` を N 個に、`narrowBitTest` / `carryBranch` の 3・4 バイトの印、`expandMul` の費用の見積もり、u24 専用の乗除算、
  変数の量のシフトのバイト単位の分岐
- fclib/math.fc の `mul_u16`（u16 × u16 → u32）・`abs_i32` など
