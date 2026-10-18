# 定数も上から決めた幅で畳む（A1 の印の仕組みをやめる）の残り

2026-10-18 に `@min` / `@max` / `@clamp` が A1 の幅を受け取らないバグ（`../../issues/local-2026-10-18-minmax-a1-width.md`）を
直したあと、ユーザーと決めた根本の対策（経緯は `../../discussions/2026-10-18-a1-root-fix.md`）。

**fc 4 の分は済んだ**（2026-10-18）。仕組みは `internal/sema/widen.go` の頭のコメント（「型付きの定数の演算と A1」）:

- 演算ごとの A1 の役割は表 `a1Roles` 1 つ（`TestA1RolesComplete` が全部の演算が表にあるかを見る）
- 区切りにならない演算を畳んだ定数には元の式（`cexpr.src`）が残り、幅が決まる所（`lvalIn` の入口、定数の畳み込みの中の
  `foldWidenArgs`、代入先の `widenConstAt` / `preConvert`）で元の式からその幅で畳み直す（`foldAtWidth` / `wideValue`）
- fc 4 は印（`Hlc.taint`）を使わない。消す前に、印の経路で値が変わったら panic するようにして全テストと関係する差分 fuzz
  （ArithV4・ConstFold・ConstFoldV4・ProgramsV4・V3ProgramsV4・GridV4・Metamorphic・Migrate を数千本）を回し、0 回だった

## 残り: fc 3 → 4 の migrate の印

fc 3 のモジュールで書き換えを集めるとき（`h.rewriting()`）は、まだ印で「fc 4 では広がる畳み込み」の位置を報告している
（`foldOverflows` / `markTaint` が付け、`foldWidenArgs` と `widenArith` が `rewriteAs("widen", …)`）。fc 4 の計算とは
独立しているので急がない。やるなら:

- 広い型と出会う所で `src` から `foldAtWidth` した値と狭い値を比べ、違えば報告する形にする。`widenArith` は `ir.Operand` しか
  受け取らないので、呼び出し側（算術・比較・`@min` 系の項、`convertValue`、`rvalPreConv`）から評価した cexpr を渡す
- 同じ位置への `as` の書き換えの順（`rvalPreConv` のコメント）を崩さないこと
- 確かめ方: `TestMigrateExamples` / `TestMigrateGoldenPrograms` / `TestMigrateBench` / `TestRandomMigrate` が今と同じ書き換え
