# @min / @max / @clamp が A1 の幅を受け取らない

- 出典: ローカル（2026-10-18 のバグ調査。`TestRandomArithV4` の生成器）
- 状態: **修正済み**（2026-10-18、N100 機。下の「対応」）
- 詳細: `../discussions/2026-10-18-minmax-a1-width.md`

## 概要

`var v:i16 = @max(a + a, 0);`（`a:u8 = 200`）が **144**（8 ビットで折り返し）になる。同じ式を条件式に書いた
`var v:i16 = 1 ? (a + a) : 0;` は **400**（A1 で 16 ビット）。`@min` / `@max` / `@clamp` は純粋マクロで展開すると
条件式になるので、A1 の「条件式は区切りではない（上から来た幅を両方の枝に渡す）」が及ぶはずで、これが壊れている。
`@clamp` も同じ。

## 原因（`internal/sema/expr.go`）

1. 純粋マクロを展開したあと `r = h.rval(x.expr)`（= `rvalIn(c, 0)`）で上から来た幅を捨ている
2. `case opMin, opMax, opClamp:` の引数を `h.rval(a)`（幅 0）で評価し、計算型も `planMinMax` の互換型だけ

## 修正するときに気をつけること

実行時側だけ直すと**定数の畳み込みと実行時の値がずれる**（`TestRandomConstFoldV4` が落ちる）。畳み込みは上から来た幅を
知らないので、算術と同じ「狭い幅で畳んで印を付け、広い幅で使い直す」仕組み（`internal/sema/widen.go`）に `@min` 系を
入れる必要がある。試した範囲では `isArithNode` / `wideValue` / 畳み込み側の狭い引数の広げ直しまで必要で、最後に
`@min(<i16>, <i8>) as i8` の**符号**の違いが残った（畳んだ方は u8、実行時は i8）。`foldType(args, cmp=true)` と
`planMinMax` の符号の決め方の差が怪しい。

## 関連

- `Agent/wiki/fc4-facts.md`（A1 と区切りになるもの）
- `Agent/wiki/plans/bug-hunt.md`（Phase 2 の 1 の生成器が見つけた）
- 関連コミット: この Issue を直したコミット（`sema: @min / @max / @clamp を A1 の幅で計算する`）

## 対応（2026-10-18）

実行時と畳み込みの両方で、`@min` 系を算術と同じ「区切りではない」節点として扱った。

- 実行時（`expr.go`）: 純粋マクロの展開を `rvalIn(x.expr, hint)` に。`opMin` 系は `wideWidth` で幅を決めて引数を `rvalIn`
  し、広い幅なら**揃える型（`planMinMax`）の符号**の広い型で比べる。広い幅で計算した引数は自分の符号の型（`u8 ^ i8` は i16）
  なので、同じ大きさなら揃える型として読み替える（符号付きの比較になっていた）。折り返した定数の引数は `widenArith` で
  畳み直す（前に残った「畳んだ方は 233、実行時は -23」はこれ: 実行時の経路が定数の引数を畳み直していなかった）
- 畳み込み（`consteval.go` / `widen.go`）: 引数を `foldWidenArgs` で揃える型の幅に畳み直し、印のある引数・揃える型に読み替えて
  値が変わった引数があれば結果に印。`isArithNode` / `wideValue` に `@min` 系、`widenConstAt` / `wideValue` は純粋マクロの
  呼び出しを展開して見る（`pureExpansion`）。`wideValue` は印の付いた定数を元の式で計算する（定数式の中の純粋マクロは評価した
  引数で展開するので、引数が畳んだ値になっている）
- 回帰テスト `TestMinMaxWidth`、bugzoo `minmax-a1-width`。`TestRandomArithV4` の生成器の回避（8 ビットの引数を作らない）は外した
- 根本の対策: 演算ごとの A1 の役割の表 `a1Roles` と `TestA1RolesComplete` は済み。定数も上から決めた幅で畳む計画は `Agent/wiki/plans/a1-const-width.md`
