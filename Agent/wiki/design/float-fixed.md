# 固定小数（u8f8 など）と浮動小数（f16 / f32）

2026-10-24 に入れた（決めた経緯は [../../discussions/2026-10-24-float-fixed.md](../../discussions/2026-10-24-float-fixed.md)）。利用者向けの仕様は
docs/reference/language.md の「固定小数」と「浮動小数」。どちらも fc 4 だけ。

## 型のない実数の定数

- リテラル `1.5` / `2e-3` は字句解析では整数のリテラルの節点（`syntax.IntLit` の `IsReal`、値は綴り）。sema は `cexpr{kind: cInt, s: "real"}` にし、
  評価で正確な有理数（`ir.Value.Real`、型は `types.UntypedReal` = Kind Float・大きさ 0）にする
- 定数どうしは有理数で正確に畳む（`+ - * /`・比較・単項の `-`）。相手が固定小数・浮動小数なら合わせる（`adaptNumeric`）、整数ならエラー
- `const H = 0.5` は型のない実数の定数の名前。`var x = 1.5`（型を書かない変数）は型が決まらないのでエラー（浮動小数の段で f32 にする）

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
