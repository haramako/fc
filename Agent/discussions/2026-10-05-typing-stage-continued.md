# 型を決める段の続き（2026-10-05）

ユーザーの依頼「型を決める段への移行の続き」（roadmap.md の構造の整理「sema の式に型付きの中間表現を」の残り: IR を出す段と
型を決める段を本当に分ける、sema が自分の Symbol / Scope を持つ）。一度に lval を書き直すのは大きすぎるので、生成物を変えない
単位に分けて進めた。

## やったこと

1. **型を決める段の網羅**（efe55e2）: lval が評価する式のうち型を決められるのは、数えると値の式の約 81 % だった（examples・
   test・bench・fclib。外れは代入の式・void の呼び出し・マクロの呼び出し・slice の範囲と変換・リテラル）。これらを足して
   99.9 % 以上に（残りはエラーになる形・@copy・soa の 2 バイトのフィールドへの代入の式）。
   - マクロは呼び出しの型を宣言する（`macroTyping`）。展開が IR を出さないマクロ（min / max / clamp・cos・textmap の変換器）は
     型を決める段が展開して型を見て、lval も同じ展開を使う（展開を 2 回しない: textmap の変換は文字を表に足す副作用がある）。
     IR を出すマクロ（@slice・@ptr・@format）は型の規則を登録、値を返さないマクロは Void
   - 結果を節点ごとに文の間覚える（constEval の cmemo と同じ寿命）。castle の check の時間は変わらない（8.3 秒 / 5 回）
   - FC_VERIFY_IR の照合を全部の節点に。最初の fuzz で soa の 2 バイト以上のフィールドへの代入の式の型が食い違った
     （左辺の場所が無く、変換した右辺を値にする。型は変換の仕方による）ので、その形は型を決めない
2. **A1 の幅を引数で渡す**（3ab2e07）: `Hlc.wide`（次に入った lval が受け取って 0 に戻す）をやめ、`lvalIn(c, hint)` の引数に。
   間に別の lval が入ると幅を横取りされる形が無くなる
3. **名前の表と本体を ir から出す**（8949b44）: `Scope`・`ModuleInterface` を sema に、`ir.Lambda.Body` を `Program.bodies` に。
   `ir.Value.Module` はモジュールの id。使われていなかった名前解決の観測（SetTrace）を消した

どの段も、examples（castle・miku・miku4・jump・hello・statusbar・wave・life）の ROM / バイナリ、bench（-O 0 / 2）、test/ の出力、
fclib の @(test) の結果が前とバイト単位で同じことを確かめた（スクリプトで前後の fcc の出力を比べた）。

## 決めたこと・残り

- **次の段は「暗黙の変換を型を決める段が節点に付ける」形**: 今の lval は二項演算などで tryMakeCompatible / adaptLiteral を
  呼んで型を出し直し、照合で一致を確かめている。型が出し直しにならないようにするには、型を決める段が「この項を T に変換する」
  まで決めて節点に持たせ、lval はそれに従って IR を出すだけにする。型の誤りの診断を型を決める段に寄せるのも同じ段で
- **Symbol の分離は後**: sema の名前は今も `ir.Value`（置き場所・リテラルと兼用）。Scope は sema に移したので、次は
  Scope が持つものを sema の Symbol にして、`ir.Value` を置き場所とリテラルだけにする
- 確かめた事実: `fcc test -t emu fclib/nes/*.fc` は vblank を待って止まらない（nes のモジュールは `-t nes` で走らせる）
