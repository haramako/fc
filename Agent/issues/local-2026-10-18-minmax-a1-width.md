# @min / @max / @clamp が A1 の幅を受け取らない

- 出典: ローカル（2026-10-18 のバグ調査。`TestRandomArithV4` の生成器）
- 状態: **未修正**
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
- 関連コミット: なし（変更は戻した）
