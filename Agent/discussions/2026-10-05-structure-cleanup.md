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
