# @min / @max / @clamp は A1 の幅を受け取らない（8 ビットで静かに折り返す）

日付: 2026-10-18
ステータス: **バグ（未修正）**。修正を試みて全テストを緑に戻したところで止めた（下に経緯）。

## 再現

```fc
#fc 4
use console;
var a:u8 = 200;
function main():void
{
	var v1:i16 = a + a;             // 400  … A1 で 16 ビット
	var v3:i16 = 1 ? (a + a) : 0;   // 400  … 条件式は区切りではない (docs/reference/types.md)
	var v4:i16 = @max(a + a, 0);    // 144  ✗ 8 ビットで折り返している
	var v5:i16 = @min(a + a, 500);  // 144  ✗
	var v6:i16 = @clamp(a + a, 0, 500); // 144 ✗
}
```

`@min` / `@max` / `@clamp` は純粋マクロで、展開すると条件式になる。**fc 4 の条件式は区切りではない**（上から来た幅を
両方の枝に渡す: `internal/sema/cond.go`）ので、`@max(a + a, 0)` も 16 ビットで計算されて 400 になるべき。
今は 8 ビットで折り返した 144 になる（`docs/reference/types.md` の A1 と、`docs/reference/macros.md` の
「`@min` は `a < b ? a : b` と同じ」の両方に反する）。

## 原因

`internal/sema/expr.go` の 2 箇所が上から来た幅を捨てている。

1. 純粋マクロの展開後: `r = h.rval(x.expr)`（`rval` は `rvalIn(c, 0)`）で hint を渡さない
2. `case opMin, opMax, opClamp:` で引数を `h.rval(a)`（hint 0）で評価し、計算型も `planMinMax` の互換型だけ

## 直そうとして起きたこと（次にやる人のための記録）

実行時側は `(1) を rvalIn(x.expr, hint)`、`(2) を wideWidth(e, hint) で得た幅で引数を rvalIn し、計算型も
「広げる前の型の符号のまま広い幅」にすれば w1.fc は直った。ただし**畳み込み側が合わなくて**全テストが緑にならなかった。

- 畳み込みは下から上で、上から来た幅を知らない。算術は「狭い幅で畳んで印を付け、広い幅で使い直したときに元の式を広い幅で
  畳み直す」（`widen.go` の `widenArith` / `widenConstAt` / `foldAtWidth`）ので揃う
- `@min` 系は `isArithNode` でなかったので、`isArithNode` に `opMin/opMax/opClamp` を足し、`wideValue` に
  min/max/clamp のケースを足し、畳み込み側でも「`t` より狭い引数は元の式を `t` の幅で畳み直す」を入れた
- それでも `TestRandomConstFoldV4` の seed 3 が残った: `@min(<i16 の式>, <i8 の式>) as i8` で、畳んだ方の引数の
  **符号**が実行時と違う（畳んだ方は u8 の 233、実行時は i8 の -23 として使われる）。`foldType(args, cmp=true)` と
  `planMinMax` の符号の決め方の差が怪しい
- 変更を全部戻して `go test ./...` は緑

関連: `Agent/issues/2026-10-18-minmax-a1-width.md`、`Agent/wiki/fc4-facts.md`（A1 と区切りになるもの）、
`Agent/discussions/2026-10-18-arith-model-and-const-widen.md`（型のない定数の規則の実測）。
