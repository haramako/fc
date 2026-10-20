# 常駐の書き戻しがループの頭のラベルの後に置かれ、外側のループが終わらない（-O 2）

- 出典: ローカル（2026-10-20、`Agent/scripts/fuzz-loop/` の `TestRandomV3Programs`、種 `2026102012190700`）
- 状態: **修正済み**（2026-10-20）
- 入ったコミット: `e1ccbc8`（jumps の `dropDeadTemps`）で表に出た。`3c3dfb7` では通り、`e1ccbc8` で止まらなくなる（両方の fcc で確かめた）

## 症状

`-O 0` は正しく終わり、`-O 2` だけがサイクルの上限（10 億）まで回る。テストの切り分けは「最適化の後の IR はインタプリタで
同じ出力: レジスタ割付か codegen」。`FC_NO_RESIDENT=1` と `FC_DISABLE=func-resident`（と `rotate` / `dup` / `jumps`）で直る。

## 小さな再現（fc 2。`TestResidentExitToLoopHeadCopyBlock`）

```fc
function ff0(p0:sint):sint
{
var l5:int = 0;
for (var l2:int = 0; l2 < 2; l2++) {
for (var l3:sint16 = 0; l3 < 3; l3++) {
var l4:sint16 = min((((l3 + 5) - l3) as sint16), ((p0 == p0) as sint16));
}
while (5 < (p0 as sint16) && l5 < 5) {
l5++;
}
}
return p0;
}
```

`ff0(-57)` は -57 を返すはずが、`-O 2` で止まらない。

## 原因

関数全体の常駐の領域（ループの外）で `l2` が Y にあり、外側のループの頭 `@body_27` へ入る辺で書き戻す（`sty l2`）。
内側の while の領域は `l5` を Y に置き、その入口の写し（`ldy l5`）が `@body_27` のブロックにある。`l3` のループは展開されて
使われない比較だけになり、`dropDeadTemps` がそれを消したので、`@body_27` は「ラベル; `l5@Y = l5`; jmp」だけになった。

`onEdge` の `copyBlock` はこの形を「先の領域が辺を分割して作った写しだけのブロック」と見て、`l2` の書き戻しをその中
（ラベルの後）に置いた。`@body_27` はループの頭なので戻りの辺（`if_true @body_27`）でも通り、そのとき Y は `l5`（= 0）なので
`l2` が毎周 0 に戻っていた。

## 直したこと

`copyBlock` は入る辺が 1 つだけのブロックに限る（辺を分割して作ったブロックは必ずそう）。入る辺が複数なら通常の辺の扱い
（fallthrough なら from の末尾 = ラベルの前）になる。`go test ./...` は緑、castle の ROM の golden は変わらない。

## 関連

- `internal/regalloc/resident.go` の `makeResident` の `copyBlock` / `onEdge`
- `internal/driver/ops_test.go` の `TestResidentExitToLoopHeadCopyBlock`、`TestResidentExitThroughCopyBlock`（同じ写しだけのブロックの別の罠）
- `local-2026-10-18-dead-compare.md`（`dropDeadTemps` を入れた Issue）
