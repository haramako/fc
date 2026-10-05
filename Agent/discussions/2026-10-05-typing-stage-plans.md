# 型を決める段の計画（2026-10-05）

ユーザーの依頼「型を決める段（の続き）をよろしく」。前の段（[2026-10-05-typing-stage-continued.md](2026-10-05-typing-stage-continued.md)）
の「残り」: 暗黙の変換を型を決める段が節点に付け、lval は IR を出すだけにする / 型の誤りの診断を型を決める段に寄せる /
可能なら sema の名前を ir.Value から分ける。

## やったこと

1. **演算の計画**（`sema/typeplan.go`）: 節点ごとに「結果の型・項の変換（型のない定数を相手に合わせる・互換型に揃える・
   ポインタの加減算）・型の誤りの診断」を項の型（`exprInfo`）だけから決める関数にした。二項の算術（`planArith`）・比較
   （`planCompare` / `compareType`）・単項（`planUnary`）・@min / @max / @clamp（`planMinMax`）・添字（`planIndex`）・参照はがし
   （`planDeref`）・フィールド（`planField`）・明示の変換（`planCast`）・呼び出し（`planCall`）。
   - 型を決める段（`opType` など）は計画の結果の型を節点の型にする。計画が診断を panic する形は型を決めない（ok = false）
   - lval は項を評価したあと、同じ計画を受け取ってその変換のとおりに IR を出す。項の型は型を決める段の型（`planInfo`。
     決められない項だけ評価した値の型）。FC_VERIFY_IR のときは、型のない定数かどうかと値が評価した値と同じかも確かめる
   - これで lval が値から型を出し直す `adaptLiteral` + `tryMakeCompatible` / `makeCompatible` の経路が無くなった
     （`makeCompatible` は削除、`adaptLiteral` は計画の `adaptLit` を値に当てる薄い包みで、for-each の範囲と定数の畳み込みが使う）。
     型を決める段の側の別実装（`literalAdapted`・`minMaxType` の独自の合わせ方）も計画に一本化した
   - 比較の項の合わせ方は「計算する幅の型」（A1 で広げた項は広い型）、F6 は「広げる前の型」で見る（lval と同じ。`wideInfo`）
2. **名前の束縛を ir から出す**（`sema/binding.go`）: モジュールの束縛（`use mod;`）と型名の束縛（struct・enum・soa）の中身を
   `ir.Value.Module` / `TypeRef` から sema の表（`Program.bindings`）に移し、IR の変数の一覧にも入れない（golden の IR から
   `mod:` / `type:` の変数が消えただけ）。

## 確かめたこと

各段で、前の fcc と出力をバイト単位で比べて同じ（examples の castle・miku・miku4・jump・hello・statusbar・wave・life、
bench の 13 本 × -O 0 / -O 2、test/ の 18 本 × -O 0 / -O 2 の出力と警告、fclib の @(test)、test/*.fc の `fcc check`）。
`go test ./...` が通り、fuzz（TestRandom* と fold / mutate / metamorphic、FC_VERIFY_IR）を計約 1500 本回して失敗なし。

## 診断の変化

- 文言は変えていない。名前付きの定数の「does not fit … the other operand」は名前を出すまま（`exprInfo.name` で持つ）
- 1 つの文に誤りが 2 つあるとき、どちらを先に出すかが変わりうる形が 2 つ（どちらも fuzz・テストでは出ていない）:
  比較で片方が enum、もう片方が符号の混ざった算術の結果のとき（以前は enum の誤りが先、今は符号の混ざった値の誤りが先）、
  二項の算術で互換型の誤りと符号の混ざった値の誤りが重なるとき

## 残り

- 代入のような変換の、評価の後の convert（約 8 % の評価の前に判定できない形）。計画の外の節点（代入・slice・リテラル・soa）の診断
- マクロとモジュール・型名の束縛を、値でないものとして cexpr で表すこと（今は ir.Value の入れ物を名前の表に置く）
- sema の名前を `ir.Value` から分けた Symbol にすること（`ir.Value` を置き場所とリテラルだけに。名前の表の引き当ては 27 か所）
