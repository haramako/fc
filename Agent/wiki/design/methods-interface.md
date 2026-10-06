# struct のメソッドと interface

2026-10-06 に相談して実装した（[経緯](../../discussions/2026-10-06-methods-interface.md)）。利用者向けの仕様は
docs/reference/language.md の「メソッド」と「interface」。ここは内部の作りと、そうした理由。

## 目的

ゲームの敵・弾・エフェクトのような「型ごとに処理の違う要素を、決まった数のスロットで回す」形を、言語の機能で書けるようにする。
castle の敵は手で書いている: 型の番号の列、フィールドごとの列、型ごとに意味の変わる共有の列 `p1`〜`p8`、型の番号で引く関数の表と
バンクの表（6502 のゲームでは定番の形）。表・番号・共有の列の意味の対応をコンパイラが型で確かめ、表を作る。
効率の目標は「手で書いた形より遅くしない」。

## 決めたこと（2026-10-06、ユーザー）

- 段の順: ① struct のメソッド → ② interface と soa → ③ far の interface。3 つとも同じ日に実装した
- メソッドは struct の外に書く（`function Point.add(...)`。実装を別のモジュールに置ける）。受け取り手の型はふつうの引数として書く
  （Zig と同じ: `self:Point` / `self:*Point` / `self:*const T` / soa のハンドル `self:*Enemies`。`self` は予約語でない）
- メソッドを足せるのは型を宣言したモジュールだけ
- 一般の struct の埋め込みはやらない。共通のフィールドは interface に書く
- interface の構文は `interface` / `soa interface`、実装は `struct Slime: Task = 1 { ... }`。1 つの interface は soa か普通かのどちらか
- ID は手動・自動の両方。ID は interface ごとの enum `Task.Id`（`none` = 0 と実装の名前）。データの番号から作る `@set_id`
- 要素を値として写すのは禁止（soa の interface。後の段で考える）
- near / far は interface の宣言で決める（`fn` / `farfn` と同じ関係）。far は呼ぶたびに切り替えて戻す

## メソッド（sema/method.go）

- メソッドはモジュールの名前の表に入れず、受け取り手の型の修飾名（`mod.T`。struct も soa も types.Type.Name）→ 名前の表
  `Program.methods` に置く。中身は普通の関数（シンボルは `_mod_T__m`）
- `x.m(args)` は constEval の呼び出しで `T.m(受け取り手, args)` に書き換える。型を決める段も lval も constEval を通るので、同じ
  書き換えを見る。受け取り手は self の形に合わせる: 値の self には値（ポインタなら `*x`、soa のハンドルなら `*h`）、ポインタの self には
  `&x`（変数でなければエラー。読むだけの `*const T` は一時の値でよい）、soa のハンドルの self にはハンドル（要素の式 `S[i]` は `&S[i]`）
- フィールドと同じ名前のメソッドはエラー（`x.f(...)` が関数ポインタのフィールドの呼び出しと紛れない）。`T.m` は関数の値

## interface（sema/iface.go）

型

- interface も実装も struct 型。先頭に ID のフィールド `$id`（型は `Task.Id`）、続けて共通のフィールド（ここまでを見出しと呼ぶ）。
  実装はその後ろに自分のフィールド。`$` で始まるフィールドはソースから書けず、struct のリテラルの位置指定からも外す（`litFields`）。
  リテラルで省いた `$id` はその実装の ID
- 普通の interface の struct は見出しと `$data:[K]u8`（K は実装の最大）。`*Task` はポインタ
- soa の interface: `*Task` は置き場所の soa（Tasks）のハンドル。`*Slime` は「Tasks を Slime として見た」soa（見方。名前は実装の struct の
  修飾名）のハンドルで、見出しのリーフは Tasks の配列、実装のフィールドのリーフは共有の列 `Tasks_$v0`, `Tasks_$v1`, ...（使う所まで作る）を
  指す（`buildView`）。フィールドの読み書きは soa の仕組みそのまま
- 置き場所の soa は interface と同じモジュールに 1 つ（`ifaceContainer` が宣言を探す。実装のメソッドの型 `*Slime` を決めるのに要る）

実装の一覧と ID

- 全モジュールを読み込んで宣言の解決を始める前に決める（`finalizeInterfaces`。`CompileModule` の一番外で 1 回）。こうすると enum の
  メンバー・普通の interface の大きさが、ほかの宣言を解決するときには決まっている（遅らせると、どこで決まるかの扱いが要る）
- 自動の ID はモジュールのパスの順・宣言の順で空いた番号を 1 から。`= ID` は数・定数の名前・`mod.名前`・括弧の式だけ（式をそのまま
  書くと `= N {` が struct のリテラル・後置ブロックと文法で紛れる）
- 実装はプログラムのどこかで `use` されたものだけ（読み込んだモジュールの中から探す）

呼び出し

- interface のメソッド `Task.m` は振り分けの関数で、`x.m(...)` は普通のメソッドの呼び出しになる。本体 `TBL[self.$id as u8](self, ...)` と
  ID で引く関数の表 `$Task_m` は、関数の本体をコンパイルする前に作る（`finishInterfaces`）。受け取り手の評価は 1 回で済み、静的フレームの
  飛び先の絞り込み（const の表の要素）・farfn の呼び出し・インライン展開がそのまま効く
- 実装の数が少なければ、最適化の const の表の直接化で、RTS の飛び先の表と直接呼び出しの分岐になる。`self`（1 バイトの添字）は A で渡る
  （2026-10-06 に生成コードで確かめた）
- 表の空き（`.none`・実装の無い番号・実装に無いメソッド）は既定の本体、無ければ何もしない関数（値を返すメソッドは 0・null・すべて 0 の
  struct を返す）。**計画では「止める関数」だったが変えた**: 止めるには console が要り、使っていない NES のプログラムまで引き込むため。
  値を返すメソッドに既定の本体を必須にする案も試したが、`.none` が常にあるので値を返すメソッドすべてに要ることになり、やめた
- 表は interface のモジュールに置き、実装のモジュールを `Uses` に足す（`.inc` の `.import`。use はしていない）
- 実装のメソッドの型は、self のほかは interface のメソッドと同じ（違えばエラー）。interface のメソッドは既定の引数・属性を持てない
- 実装のメソッドから別の要素のメソッドを呼ぶと、振り分けの関数を通して呼び出しが輪になり、再帰の関数（スタック）として扱われる
  （`TestInterfaceRecursive`）

far

- `@(far)` なら表の要素は farfn（呼ぶたびに実装のバンクに切り替えて戻す。`farcall_ay` のトランポリン）。プログラムに `@(farcall)` が要る
- `@bank_of_id(id)` は ID → 最初のメソッドの関数のバンクの表 `$Task_bank`（使わなければ出さない）。near の interface と合わせて、
  手書きと同じ「ループの中で切り替え、戻すのはループの後に 1 回」が書ける（`TestInterfaceNearBankNES`）

そのほか

- `Tasks[i] = Slime{...}`（右辺が実装の struct の値で、左辺がその interface の要素）は `*@bitcast(*Slime, &Tasks[i]) = ...` に書き換える
  （`implAssign`。ID も書く）。soa への書き込み (scatter) は実装の見方にだけ許し、読み出し (gather) はどちらも禁止
- interface のハンドルを実装のハンドルとして読むのは `@bitcast(*Slime, e)`（確かめない）
- `fcc build -d` に interface ごとの実装と ID の一覧（`Program.InterfaceSummary`）

## 残り（Agent/wiki/plans/roadmap.md の「言語機能」）

- 確かめる下向きの変換 `as? Slime`、soa の interface の要素を値として写すこと
- far call でバンクを戻さない呼び出しの記法（far の interface にも普通の `farfn` にも。その後のバンクは自己責任）
- 続けて far call するときに戻すのを後に回す最適化（呼ぶたびに戻す形を測って、遅さが問題になったら）
- castle でない例（敵の群れのサンプル）で、手書きの表の版と大きさ・サイクルを比べる（bench）

## 前例（調べ直していない、知識による）

- 宣言の形: Pascal・Modula-2・Ada の可変レコード（共通のフィールド + タグ + 重ねて置くフィールド）。メソッドの表は無い
- 閉じた組をタグで呼ぶ: Rust の `enum_dispatch`、C++ の `std::variant` + `std::visit`、Zig の `union(enum)` と `inline else`
- SoA: Odin の `#soa`、Zig の `MultiArrayList`（タグ付きの共用体ならタグの列と中身の列）
- 置き方: 6502 のゲームの手書きの形（スーパーマリオブラザーズの敵の ID の配列・フィールドごとの配列と `JumpEngine`）
- Zig は埋め込みを持たない（`s.base.x`、`@fieldParentPtr`）。フィールドがどこから来たか読んで分かることを優先していると思われる
