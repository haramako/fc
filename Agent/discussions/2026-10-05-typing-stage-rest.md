# 型を決める段の残り（2026-10-05、3 回目）

ユーザーの依頼「型を決める段の残り」。前の段は [2026-10-05-sema-types-cleanup.md](2026-10-05-sema-types-cleanup.md)。そこで残した
3 つ（計画の外の節点の型と診断、lval が型を出し直す残り、`ir.Value` の sema だけが使う印）と、条件式の残り
（[2026-10-05-cond-expr-do-while.md](2026-10-05-cond-expr-do-while.md) の `var x = c ? …` の初期化）。

## やったこと

1. **`ir.Value` の sema だけが使う印を sema へ**: `Public` は名前の表の `Symbol` が持つ（`Hlc.setPublic`。`Symbol.Public()` は値でも
   値でない束縛でも同じ欄）、`Build`（@(build) の const）は `Program.buildConsts`、`StrConst`（名前付きの文字列定数）は既にあった
   `Program.strConsts` を見る。IR のダンプの ` pub` は消えた（golden の IR を再生成。生成コードは同じ）。
   **移さなかったもの**: `Untyped`（型のない定数。リテラルの値の表現で、sema が作る値のあちこちにあり、表に移すと値と表の
   ずれが起きやすい）、`IsString` / `StrTerm` / `Str`（リテラルのデータの表現。IR のダンプも使う）、`ReadOnly`（opt の aggcopy が使う）
2. **計画（typeplan.go）を広げた**: slice（`planSliceOf`・`planSliceBound`・`planToSlice`。a[lo..hi]・slice への変換・@len・
   for-each・@ptr・@copy・@format が使う `sliceParts`）、実行時の struct と配列のリテラル（`planStructLit`・`planRuntimeArray`）、
   soa の添字とフィールド（`planSoaIndex`・`planSoaField`）、代入の左辺の形（`planAssignTarget`: 呼び出しの結果に代入できない）。
   型を決める段と lval が同じ関数を使い、lval は計画を受け取って値を作るだけ（`partsOf` は計画の要素の型・長さ・幅で値を
   組み立てる。前は値の型から arrayParts が出し直していた）。明示の変換の検査（`planCast`）は型を決める段の項の型で
3. **lval が型を出し直していた残り**: 代入先の型（`assignTarget`）、型を省いた変数の型、呼ぶ関数の型（引数・戻り値）を型を
   決める段の型で。soa の要素は型を決める段ではハンドルなので、値として読む所（`var w = *Enemies[5]`、要素への代入）は要素の
   struct にする（ハンドルの型のフィールド `n.next`（next:*Nodes）はハンドルのまま。最初の試しで取り違えて test_soa が落ちた）
4. **`var x = c ? a : b` の初期化**: 条件式の値を集めた一時変数への枝ごとの書き込み（`condValue`）を覚えておき、変数の初期化で
   書き先を変数に替える（`retargetCond`。一時変数からの写しと、その一時変数を出さない）。代入（`condAssign`）と違い、変数を
   宣言する前に初期値を評価する順（初期値の中の同じ名前は外側の変数）を保つためにこの形にした。8 回展開したループの
   `var v = arr[i] > 4 ? arr[i] - 4 : 0;` で 219 → 199 命令。小さい関数では、変数が 2 か所で書かれるので常駐の割付の選び方が
   変わり 2 命令増えた例もある（92 → 94）。fuzz の条件式の書き換え（`rpCondRewrite`）に関数の中の変数の初期値を足した
   （100 本で 436 か所。定数・リテラルの初期値は型が変わりうるので包まない）

## 確かめたこと

各段で前の fcc と出力をバイト単位で比べて同じ（examples の castle・miku・miku4・hello・jump・statusbar・wave・life の ROM、
bench と test/ の -O 0 / -O 2 の出力と警告、test/*.fc の `fcc check`、fclib と fclib/nes の `fcc test`）。条件式の初期化を使う
コードだけが変わる（examples・bench・test/ には無い）。`go test ./...` が通り、fuzz（TestRandom* と fold / mutate / metamorphic、
FC_VERIFY_IR）計 740 本で失敗なし。bugzoo の init-signext.patch を今のコードに合わせて作り直した。

## 診断の変化

文言は変えていない。変わりうるのは、ポインタ経由の配列のフィールド（`p.arr`）を要素の違う slice にしたときのエラーの
型の表示（前は要素へのポインタ `*u8`、今は配列の型 `[4]u8`）。

## 残り

- 左辺値であること（代入できる場所か）と読み取り専用は値の印（`h.readOnly`・`ir.ValAssignable`）で、lval が評価した値で
  判定している（型を決める段は左辺値性を持たない）。型を決める段に左辺値性を持たせるかは、lval を型付きの木から IR を出す
  形に作り直すときに決める
- soa の要素の型の二重の意味（型を決める段ではハンドル、値として読むと struct）
- `ir.Value.Untyped`（上の 1）
