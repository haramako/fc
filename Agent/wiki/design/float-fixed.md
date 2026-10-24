# 固定小数（u8f8 など）と浮動小数（f16 / f32）

2026-10-24 に入れた（決めた経緯は [../../discussions/2026-10-24-float-fixed.md](../../discussions/2026-10-24-float-fixed.md)、実装の記録は
[../../discussions/2026-10-24-float-fixed-impl.md](../../discussions/2026-10-24-float-fixed-impl.md)）。利用者向けの仕様は
docs/reference/language.md の「固定小数」と「浮動小数」。どちらも fc 4 だけ。

## 型のない実数の定数

- リテラル `1.5` / `2e-3` は字句解析では整数のリテラルの節点（`syntax.IntLit` の `IsReal`、値は綴り）。sema は `cexpr{kind: cInt, s: "real"}` にし、
  評価で正確な有理数（`ir.Value.Real`、型は `types.UntypedReal` = Kind Float・大きさ 0）にする
- 定数どうしは有理数で正確に畳む（`+ - * /`・比較・単項の `-`）。相手が固定小数・浮動小数なら合わせる（`adaptNumeric`）、整数ならエラー
- `const H = 0.5` は型のない実数の定数の名前。`var x = 1.5`（型を書かない変数）は f32
- int64 に入らない整数のリテラル（`340282346638528859811704183484516925440`）も正確な値で持つ（`bigIntLit`。字句解析の値は折り返して
  いた）。整数の所に使えば「入らない」のエラー

## 固定小数

- **型**: `types.Type` の Kind は Int のまま `Frac`（小数のビット数）を持つ（`FixedType`、名前は `[u|i]<整数>f<小数>`）。後段（opt・regalloc・
  codegen）は中身の整数として扱うので、最適化の条件（`Kind == types.Int`）からも外れない。sema では enum と同じく整数の規則から外す
  （`Type.Tagged()` = enum か固定小数。互換型・暗黙の変換・A1 の広げ・F4・F6・switch など、enum を除いていた所で一緒に除く）
- **規則と計画**: real.go の `planNumeric`（typeplan の `planArith` の入口）。同じ型どうしの `+ - % & | ^`・シフトは整数の命令そのまま、
  `* /` は `arithFixMul` / `arithFixDiv`、整数（変数も・型のない整数の定数も）との `* /` は `arithFixScale`（中身に掛ける・割る）。
  型のない定数は最近接（ちょうど中間なら偶数）に丸めて合わせ、入らなければエラー。ビット演算と定数はあいまいなのでエラー
- **下ろし方**（`emitNumericArith`）: 1・2 バイトの `a * b` は 2 倍の幅に符号で広げて掛け、小数のビット数だけ右へ（負の無限大へ切り捨て）。
  `a / b` は 2 倍の幅で左へずらしてから床除算。3・4 バイトは fclib/fixed.fc の `mul_u` / `mul_i` / `div_u` / `div_i`（64 ビットの積・被除数を
  32 ビットの 2 つで持つ。sema が `builtinModule("fixed")` で呼ぶ）。1・2 バイトでは積の右シフトの後に下位だけを使うので、シフトが算術でも
  論理でも結果は同じ（符号で広げることが要る）
- **変換**（`numericCast` / `constNumericCast`）: 整数 ↔ 固定小数 ↔ 固定小数は両方が入る幅で小数のビットの差だけずらす（減らす向きは算術
  右シフト = 負の無限大へ）。型のない実数の定数 → 整数は 0 へ切り捨て。暗黙の変換は型のない定数を合わせるだけで、ほかはエラー
  （`convertNumeric`。代入の照合 `types.AssignableTo` は数どうしなら通し、ここで決める）
- **定数の畳み込み**（`foldNumeric`）: 実行時と同じ計算を big.Int で（`floorDivBig`・`wrapBig`）
- **出力**: `ir.FormatFixed`（sema の定数と fclog の `@log` が使う）と fclib/fmt.fc の `fix_u` / `fix_i`（`fixdec`・`putf`）が同じ手順:
  小数部を 10 倍して桁を取る（指定が無ければ 0 になるまで = 正確な値、`{:.N}` は N 桁の後の余りで絶対値の四捨五入）。小数点の文字のコードは
  sema が引数で渡す（textmap のフォントなら "." を変換したもの）。fmt の今の関数・表（`put`・`ASCII`）は変えない（使わない ROM を変えない）。
  出力できるのは小数のビット数 28 まで（`fixMaxFrac`）、`.N` は 20 まで（`fmtMaxPrec`。printf の書き先 32 バイトに入る）
- 書式の指定 `ir.LogSpec.Prec` は `{:.N}` の N + 1（0 は指定なし）。`x` `X` `b` `c` は固定小数に使えない

## テスト

- `TestFixedRun`（testdata/fixedpoint.fc・fixedpoint_fmt.fc。-O 0 / -O 2 / インタプリタ）・`TestFixedErrors`
- 模型の fuzz `TestRandomFixedV4`（変数の形・定数の形・Go の模型の 3 つを比べる。14 の型、四則・整数との `* /`・定数・単項・シフト・`as`）。
  bugzoo の fixed-mul-sign / fixed-div-floor が落ちることを確かめた

## 浮動小数

- **型**: Kind Float（`FloatType`、f16 は大きさ 2、f32 は 4）。値はビットの並びで、定数は `ir.Value.Int` に符号なしのビット。後段は同じ
  大きさの整数として運ぶだけ（演算は呼び出し）
- **ライブラリ**: fclib/float.fc（fc で書いた IEEE 754 binary32。asm でなく fc にしたのは正しさを詰めやすく、インタプリタでも走るため）。
  仮数を 27 ビット（下の 3 ビットが丸めの印）で持ち `round_pack` で最近接偶数に丸めて詰める（非正規化数への詰め直しも）。掛け算の
  24×24 ビットは 8 ビットの表引きの掛け算（`__mul_8t16`）を組み合わせる。f16 は f32 に広げて計算して `to16` で丸め直す（binary32 は
  binary16 の + - * / と sqrt を二重に丸めても正しい精度がある）。NaN を作る演算の結果は 0x7fc00000（f16 は 0x7e00）に揃える
- **sema**（float.go）: `planFloat`（同じ型どうしか型のない定数の + - * /）、`emitFloatArith` / `emitFloatCompare` / `floatUnary`
  （単項の - は符号のビットの xor）/ `floatCast`。比較の `<=` / `>=` は `!(b < a)` に直すと NaN で違うので `le32` を呼ぶ
  （`isFloatCompare` で expr.go と compileCond の書き換えを止める）。浮動小数を条件に使うのはエラー（-0.0 が真になる）
- **定数の畳み込み**: Go の float32（f16 は `ratToFloatBitsAbs` で自前に丸める）。型のない定数から: 正確な有理数を最近接に（範囲の外は
  エラー、`as` は無限大）。`as` の規則は実行時と同じ（整数へは 0 へ切り捨てて飽和、固定小数へは `ldexp32` の後に最近接、NaN は 0）
- **数学の組み込み**: `@sqrt` `@floor` `@ceil` `@trunc` `@round`（`registerFloatBuiltins`。f16 / f32、型のない実数の定数は f32）
- **出力**: `ir.FormatFloat`（sema の定数・fclog の `@log`・テストの模型）と fmt.fc の `f32` が同じ手順: 正確な 10 進の桁を作り
  （整数の部分はバイトの列を 10 で割り、小数の部分は 10 倍する）、`{}` は C の %g と同じく 7 桁に丸めた指数が -4〜8 なら小数点以下
  6 桁で末尾の 0 を省き、ほかは指数の形。`{:.N}` は 1e9 未満なら小数点以下 N 桁。四捨五入は絶対値で。nan / inf / 小数点 / e / + の
  文字のコードは表 `fmt.FCHARS`（textmap のフォントなら sema が変換した表）で渡す。Mesen の Lua の `@log` は string.format で近い形
  （ちょうど中間の丸めが違うことがある）
- **速さ（2026-10-24 に emu で測った 1 回あたり）**: f32 の + 約 3200 サイクル、* 約 4300、/ 約 4900、@sqrt 約 21500、比較 約 870。
  f16 の + 約 6000。固定小数の u8f8 の * 約 1700、/ 約 3400、i16f16 の * 約 9300、/ 約 34000

## テスト（浮動小数）

- `TestFloatLib`（float.fc の各関数の結果のビットを Go の float32 と。端の値・中くらいの値を多めに）・`TestFloatRun`（testdata/floating.fc・
  floating_fmt.fc）・`TestFloatErrors`・ir の `TestFormatFloat`
- 模型の fuzz `TestRandomFloatV4`（変数の形・定数の形・Go の模型: 四則・単項・型のない定数・f16 ↔ f32・整数と i8f8 から・数学の組み込み、
  出力の書式・比較・整数へ・i16f16 へ）。bugzoo の float-rne-tie / float-le / float-to-fixed-round が落ちることを確かめた

## 残り（必要になったら）

- 速さ: float.fc の中の変数の量のシフト（1 ビットずつ回す）を減らす・asm に、u8f8 の掛け算を 16×16 の表引きの積に、i16f16 の割り算を
  32 ビットの引き放しに
- `f16` の演算の専用の関数（f32 に広げない）、三角関数
