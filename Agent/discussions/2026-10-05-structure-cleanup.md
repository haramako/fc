# 構造の整理の残り 4 項目を進める（2026-10-05）

roadmap.md「構造の整理」の残り（2026-09-28 の調査で挙げたもの）について、ユーザーから「コストのわりに効果がない、という判断で
残っているのか」と聞かれ、どれも効果のある整理で、大きいものほど後ろに回していただけと確認した。ユーザーの決定: 型を決める段の
作業の後に、4 つとも進める。

| 項目 | 今の問題 | 効果 | 大きさ |
|---|---|---|---|
| IR の命令の同一性を `*Op` に | 1 命令の挿入で解析を全部作り直す、「直したら作り直す」ループの上限（8〜32）に達すると黙って止まる、nil の穴で隣接判定が黙って効かない所が 6 か所 | 最適化が黙って効かない形が減る、パスを足しやすい | 大 |
| ABI / 呼び出しの計画の一本化 | 判定が 20 ファイル 150 か所、入口のシンボルが最大 4 つ | 呼び出しまわりの不具合が減る、far call のレジスタ渡しの土台 | 大 |
| types の Kind | slice / enum / soa / far を Kind + フラグで表し、Kind の分岐がフラグの検査も並べる | フラグの見落としが減る、`Compatible` を `Identical` / `AssignableTo` / `CommonType` に分けると型を決める段と合う | 中 |
| driver の `Compiler` をビルド単位に | 並行ビルドできない、メモリ配置の番地が 3 か所に直書き | 並行ビルド、`MemoryMap` から ld65.cfg と base.s を出す | 小 |

進め方: types の Kind は型を決める段の残りと続けて（sema・types が重なる）、`*Op` と driver は並行、ABI は regalloc / codegen が
`*Op` と重なるので `*Op` の後。

## driver の `Compiler`（2026-10-05 実施）

- ビルドの状態は `compilation`（`*Compiler` を埋め込み、ビルドの手順のメソッドはこちらに付ける）。`Compiler` に残すのは FC_HOME と
  ca65 の起動の数（ビルドをまたいで数えるテスト用の atomic）だけ。公開 API（`driver.Compiler` の Build / BuildContext / Check /
  Migrate、pkg/fc、cmd/fcc）は変えない。並行のビルドは中間生成物ディレクトリを別々にすること（同じディレクトリは今まで通り不可）
- RAM の配置は `project.MemoryMap`（project に置くのは、fc.toml の `[ram.*]` の重なりの検査も同じ番地を使うため。project は
  regalloc に依存できないので、FC_STACK の大きさと `regalloc.StackSize` が同じことは driver の `TestMemoryMapStack` が見張る）。
  emu の ROM・ベクタの番地（$1000〜）と IR のインタプリタの番地は対象外（RAM の重なりと関係しない）

## ABI / 呼び出しの計画の一本化（2026-10-05 実施）

- 呼ばれる側の規約は `ir.CallConv`（`Lambda.Conv`）1 つ: ABI・引数ごとの置き場所と受け取るレジスタ（RegA / RegY）・戻り値を A で
  返すか（InA / OnlyA）・入口の一覧（`sym`・`__direct`・`__frame`・`__a` と、そこで A / Y に引数があるか）。入口の選び方
  （`DirectEntry(inA, inY)`）と、戻り値を A から受け取れるか（`ResultFromA`。frames の OnlyA の判定と genCall で別々だった）も
  CallConv に置いた。入口の数（最大 4 つ）は規約そのものなので変えていない（作る所・選ぶ所が 1 か所になった）
- 呼ぶ側は `codegen/callplan.go`: planCalls が呼び出しの組・呼び先・渡し方を 1 回で作り、割付の前の印と CheckStackPush と
  コード生成が共有する。コード生成の段の layoutCalls が引数ごとの置き場所・レジスタ渡し・X の保持を先に決めるので、
  コード生成の途中の状態（積んだバイト数・holdA・holdX・resultFrom）と命令の出し直しでの保存・復元が無くなった
- Op の印（ArgY / HoldY / HoldX / ResultArg）は regalloc の見積もりとの約束として残した（割付の前に付け、regalloc はそれで
  Y / X を壊す命令と見る。codegen の計画から導けるが、regalloc は codegen に依存しない）
- 生成コードは変えない方針で、各段で examples の ROM・bench と test/ の -O 0 / -O 2 の出力と実行結果・fclib の @(test) を
  前の fcc とバイト単位で比べた（同じ）。far call のレジスタ渡しは、この上で将来やる（roadmap の別項目）
