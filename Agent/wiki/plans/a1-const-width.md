# 定数も上から決めた幅で畳む（A1 の印の仕組みをやめる）

2026-10-18 に `@min` / `@max` / `@clamp` が A1 の幅を受け取らないバグ（`../../issues/local-2026-10-18-minmax-a1-width.md`）を
直したあと、ユーザーと決めた根本の対策。第 1 段（演算ごとの役割の表 `a1Roles` と、全部の演算が入っているかのテスト
`TestA1RolesComplete`）は済み。ここは第 2 段の計画。

## なぜやるか

A1（式を代入先と式の中の一番広い型で計算する）は今 2 つの仕組みで実装されている。

- **実行時の式**: 幅を上から決めて最初から広い命令を出す（`lvalIn` の `hint`、`wideWidth`。2026-09-30 に作り直した）
- **定数の式**: `constEval` は幅を知らずに下から畳み、cexpr ごとにメモする。折り返して値が変わった結果に元の式の印（`taint`）を
  付け、広い型と出会った所で元の式から畳み直す（`widenArith` / `widenOperand` / `preConvert` / `foldWidenArgs` /
  `widenConstAt` / `wideValue`）

定数と実行時の値がずれるバグはこの 2 つの食い違いから出ている。2026-09-27 以降に 5 件（`TestRandomConstFold` の最初の実行で
見つかったもの、型付きの定数と型のない定数の比較、型付きの定数の演算、定数の初期値・引数・要素、`@min` 系）。
`@min` 系では、印の鎖が切れていた: 定数式の中の純粋マクロは評価した引数で展開するので、印に残る「元の式」が畳んだ値に
なっていて、外の幅で畳み直せなかった（`wideValue` に印をたどる分岐を足して塞いだ）。印の仕組みは「どこで折り返したか」を
覚えて後で直す形なので、新しい形（演算・マクロ・定数の置き場所）が増えるたびに、直す所を足し忘れる余地が残る。

## 形

**幅つきの定数の評価 `constEvalAt(c, w)` を 1 つ作り、A1 の幅が決まる所で定数の部分木をこれで畳む。**

- `constEvalAt(c, w)`: `c` が通す演算（`a1Roles` の `a1Through`。純粋マクロは展開した式）なら、子を元の式（未評価の
  `c.args`）から `constEvalAt(子, max(w, 自分の型の大きさ))` で畳み、結果は「その幅・自分の広げる前の型の符号」の型。
  区切りの演算は今の `constEval`（中は自分の型）。条件式は枝を `constEvalAt(枝, w)`。メモは `(c, w)` で引く。
  今の `wideValue` がほぼこれ（印をたどる所と、狭く畳んだ値から始める所が違う）
- 実行時の `lvalIn(c, hint)` は、最初の `constEval(c)` を、`c` が通す演算で `hint` が自分の型より広いとき
  `constEvalAt(c, hint)` にする（算術の子に来る定数の部分木も同じ）。これで `widenArith` / `widenOperand` の印の分岐が要らない
- 定数の中の演算（`constEval0` の算術・`@min` 系）は、型 `t` が子の型より広いとき、その子を元の式から `constEvalAt(子, t.Size)`
  で畳み直す（今の `foldWidenArgs` を印でなく元の式で）。代入先の型で畳む所（`withExpected` の `widenConstAt`）と
  `preConvert` も `constEvalAt`
- **消すもの**: `Hlc.taint`、`markTaint` / `taintOf` / `foldOverflows`、`widenArith` と `widenOperand` の印の分岐、
  `preConvert` の印の分岐、`wideValue` の印をたどる分岐

## 先に片付ける前提

1. **純粋マクロを元の引数で展開する**: 定数の評価（`constEval0` の `opCall`）も実行時（`lvalIn` の `expandMacro(e, …)`、
   `e` は評価済み）も、評価した引数でマクロを展開している。幅つきで畳むには元の式が要る。展開を元の引数（`c.args[1:]`）で
   行い、展開した式を評価する形に変える。単独で入れて、golden がバイト一致・全テストが緑であることを確かめる
2. **fc 3 の migrate**: 書き換えを集めるとき（`h.rewriting()`）は、印の付いた所に `(元の式) as T` を報告している
   （`foldWidenArgs` / `widenArith`）。印をやめたら、幅の決まる所で `constEvalAt(c, w)` と `constEval(c)` を比べ、違えば
   報告する形にする。`TestMigrateExamples` / `TestMigrateGoldenPrograms` / `TestMigrateBench` で今と同じ書き換えになることを見る
3. **型を決める段（`exprType`）は広げる前の型のまま**（変えない）。`constEvalAt` の結果の型は計算の型で、型の照合には使わない

## 手順

1. 純粋マクロを元の引数で展開する（前提 1）。golden のバイト一致
2. `constEvalAt` を入れ、`wideValue` をそれで書き直す（`widenConstAt` / `foldAtWidth` は `constEvalAt` を呼ぶだけに）
3. 呼び出し側を 1 か所ずつ `constEvalAt` に替える（`lvalIn` の入口、`constEval0` の算術と `@min` 系、`preConvert`）。
   1 か所ごとに下の確かめ方を回す
4. 印の仕組みを消し、fc 3 の migrate の報告を比較の形にする（前提 2）
5. `widen.go` の頭のコメント（A1 の説明）と `Agent/wiki/fc4-facts.md` を新しい形に直し、この計画を `design/` へ移す

## 確かめ方

- golden（`testdata/golden/`）がバイト一致（生成するコードを変えない作り直し）
- `TestRandomConstFold` / `TestRandomConstFoldV4`（定数と変数の形の比較）・`TestRandomArithV4`（Go のモデル）を数千本、
  種を変えて 0 件
- bugzoo の `const-init-widen`・`minmax-a1-width` と、A1 / 畳み込みの他のパッチが引き続き見つかる（`go run ./tools/fuzzmeasure zoo`）
- `TestMigrate*` が今と同じ書き換え

## 決めること・気をつけること

- 他のホストが sema の型を決める段（`typeplan.go` / `typing.go`）を作っている最中。着手の前に main の状況を見て、
  手順 1〜4 を短い期間でまとめて入れる（長く枝分かれさせない）
- 規模は `consteval.go` / `widen.go` / `expr.go` / `typing.go` の数百行の見込み
