# 2026-10-18 fc 4 の整数の規則の Go モデルと、定数の A1 の広げのバグ

bug hunt（`../wiki/plans/bug-hunt.md`）の Phase 2 の一番目の生成器として、fc 4 の整数の規則の Go モデル
`internal/driver/randarith_test.go`（`TestRandomArithV4`）を作った。作っている間に見つかったバグと、
モデルをコンパイラに合わせる過程で確かめた規則をここに記録する（規則そのものは `../wiki/fc4-facts.md` に移した）。

## 直したバグ: 定数の式を代入先の幅で畳み込んでいなかった（A1 の抜け）

`const C:u16 = (128 as u8) + (200 as u8);` が **72**（u8 で折り返した値）になっていた。実行時の式は代入先まで含めて
広い幅で計算する（A1）のに、定数は畳み込んだ時点で幅が決まるため、広い型と出会っても畳み込み直していなかった。
同じ式の `var v:u16 = …` は 328 になるので、**var と const で値が変わる**。

同じ機構の所（`widenArith` / `foldWidenArgs` は「広い型と出会った所」で畳み込み直す）が効かないのは、畳み込みが
先に値を決めてしまう場所:

- `const C:T = …`（const 宣言）
- グローバルの変数の初期値
- 配列リテラルの要素・struct リテラルのフィールド（`const CARR:[2]u16 = [(128 as u8) + (200 as u8), 5]` など）
- 関数の引数・戻り値（引数は `TestRandomArithV4` では `id_T(x)` の形で張っている）

`internal/sema/widen.go` に `widenConstAt` を足し、`internal/sema/struct.go` の `withExpected`（文脈の型を式に入れる
唯一の所）の先で呼ぶようにした。回帰テストは `TestBugConstWidenA1`（bugs_test.go）で、bugzoo のパッチは
`testdata/bugzoo/const-init-widen.patch`（当てると `TestBugConstWidenA1` が落ちることを確認済み）。

見つけ方: 生成器が「型つき定数だけの式」と「変数を使う式」を同じ型・同じ期待値で出す設計で、種 1 の 1 文目で出た。

## 規則の調査で確かめたこと（モデルが合わなかった場所 = 誤解しやすい所）

1. **演算の型のない定数は相手の型に切り詰める**（`sema/literal.go` の `litTruncate`）。同じ大きさで符号だけ違うとき
   広くしない: `x_u8 * (-98)` は u8 で計算する。型のない定数の型のほうが広いときだけ広がる（`x_u8 * 300` は u16）。
2. **A1 で 16 ビットに広がった式の項は元の値を広い型で読む**（`widenOperand`。「型のない定数は元の値から」）。
   なので `var v:u16 = x_u8 * (-98)` は 99 * 65438 = 55834 で、u8 で計算して広げた 80 にはならない。
   1 と 2 の違い（幅 1 では切り詰め、幅 2 では元の値）が、実行時・畳み込み・モデルのどれでも落とし穴。
3. **畳み込んだ式の値も型のない定数として相手に合わせる**。`if (v_i8 != ((-106) & 148))` は「148 does not fit in i8,
   the type of the other operand」になる（比較は切り詰めない）。
4. **定数は代入先の型に入ることが必須**で、同じ大きさの符号違いもエラー。`const c:i16 = <u16 の値 55834>` はエラー、
   同じ式の `var v:i16` は入る。これは仕様どおり（定数は黙って切り詰めない）で、生成器側の検査が足りていなかった。
5. **符号の混ざった演算の印**（`sema/mixedarith.go`）は、同じ大きさで符号の違う**型が決まっている値**の `+ - *` が起点。
   A1 で広い幅で計算する所は印が付きません（葉をそれぞれの符号で広げるため）。生成器は印が出る式を作らない。
6. 16 ビットに収まらない型のない定数だけの式（`(603 << 6) * 110` など）は `foldBeyond16` など別経路なので生成しない。

## TestRandomArithV4 の中身

- 境界値のグローバル（u8/i8/u16/i16 × 0, 1, 0x7F, 0x80, 0xFF, 0x7FFF, 0x8000, 0xFFFF, -128, -1 …）と
  `id_T()`（呼ぶと区切り、`as` と同じで型を決める。畳み込みの形では `(x as T)` になる）を種に、
  二項演算・`as`・`+%`・シフト・`(<式> | 1)`（0 で割らない）を深さ 3 まで。
- 期待する値は Go のモデル（`arTy` と `arValue`）。**領域の計算の型**が肝で、
  「型が決まっている値の互換型（広いほう、同じ大きさなら符号つき）を、型のない定数を相手に合わせた型で combine した型」を
  幅だけ代入先まで広げ、**符号は広げる前の型のまま**、ノードごとに上から幅を流す（`sema/widen.go` の wideWidth と同じ手順）。
- 同じ式を型つき定数の形にも書いて `@printf` で 2 つ並べ、var と const（畳み込み）も比べる。比較の文は真偽を `hits` に積んで
  最後に出力するので、1 プログラムで何十個もの比較の正しさも見る。
- 15000 プログラムを -O 0 / -O 2 の両方で通した（2026-10-18）。

## 交差検証の副産物: bugzoo の死んでいるパッチ

同じ実行で `go run ./tools/fuzzmeasure zoo -randn 300 -foldn 150` を回した（`/tmp/fcbug/bugzoo.log`）。
**現在の生成器が検出できないパッチ**（全項目 0）: `enum-short-twice`、`fwdmem-disp-after-replace`、
`interp-array-eq`、`memshift-narrow`、`migrate-conv-order`（+ 終了後に追記）。
それぞれを捕まえる機能を生むのが Phase 1 の作業。

## その 2: @min / @max / @clamp が定数式で畳めなかった

生成器に `@min` / `@max` を入れたら、型つき定数の形のほうが `const c0 must be constant` で通らない。最小にすると
`const D = @min(3, 5);`（fc 3 でも同じ）と `var arr:[@min(4, 8)]u8;`（「array size must be constant」）。

`@min` は「展開が IR を出さない純粋マクロ」で、`constEval` はマクロを展開しない（`sema/typing.go` のコメントに
「constEval はマクロを展開しない」と書いてある通り）ので、畳み込みの `case opMin, opMax, opClamp` に到達していなかった。
`constEval` の `opCall` で、`constFn` の他に **純粋マクロを展開して畳む**ようにした。textmap の変換器は純粋マクロだが、
展開すると「変換器」でなくなる（`@format` が書式を textmap のまま探す）ので除外した（当て忘れると
`TestFormatTextmap` / `TestFormatTextmapPO` が落ちる）。

`TestBugConstMinMax` と bugzoo の `macro-const-minmax` を追加。生成器は `@min` / `@max` を両辺の型が符号違いのものは
作らない（中の比較で F6 のエラーになる）。

## その 3: 2 バイトの値の 8〜15 のシフトを生むと `valint-width` が見えるようになった

`valint-width`（`ValIntLiteral` がリテラルの一部を読む cast をそのバイトの値にしない）は、これまで
`TestConstFoldCases` の 1 件しか見つけていなかった。TestRandomArithV4 のシフトの量を「型の幅未満」まで広して
（2 バイトの値には 8〜15 = 上位バイトの抽出）、`@min` / `@max` を足すと、ランダム生成でも落ちるようになった。
**教訓: 「1 件の単体テストだけが見ている」規則は、生成器の値の範囲・演算の幅が足りないことが多い**
