# 型を決める段の残りと types の Kind の整理（2026-10-05）

ユーザーの依頼「型を決める段（の続き）」と、構造の整理の「types の Kind」（[2026-10-05-structure-cleanup.md](2026-10-05-structure-cleanup.md)）。
前の段は [2026-10-05-typing-stage-plans.md](2026-10-05-typing-stage-plans.md)。

## やったこと

1. **名前の表の項目を `Symbol` に**（`sema/symbol.go`）: 名前が束縛するのは値（ir.Value）・モジュール・型名・マクロのどれか
   （soa は値と型名を兼ねる）。値でないものは ir.Value にせず、式の中では `cName` の節点にする。値として使うと
   「`math is a module, not a value`」「`@lz4 is a macro, not a value (call it: @lz4(...))`」「`P is a type, not a value`」。
   **`var v = math;`・`var v = @lz4;` は前は黙って通っていた**（module / macro 型の変数ができていた）。マクロの 4 つの表
   （macros・constMacros・macroTypes・textmaps。ir.Value で引いていた）は `macroDef` 1 つに。`types` の module / macro / typename
   の Kind は消した（`var x:module` のような型名も無くなった。使っている所は無かった）
2. **評価の後の変換の判定**: 以前「約 8 %」と書いていたもの。examples・bench・test/ で数えると、評価の後に E・D を判定していたのは
   全部が型を省いた変数の初期値（`var x = e`）で、変数の型は初期値の型なので E・D は起きない。判定し直さず、FC_VERIFY_IR では
   起きないことを確かめる（`verifyNoConv`）
3. **`Compatible` を分けた**: `AssignableTo(to, from)`（代入・初期化・引数・戻り値。向きのある規則: *void → *T、ポインタ → 配列は
   不可）と `CommonType(a, b)`（二項演算・比較・条件式・@min など。対称）。同じ型かは interned なので `==`（`Identical` は作らない）。
   前は *void と「配列 / ポインタ」の規則が引数の順に依存していて、`a == p`（配列とポインタ）・`c ? p : vp`（*void が右）は
   逆の順なら通るのにエラーだった（`TestCommonTypeSymmetric`）。比較の項の順（`compareType` の swap）は生成コードを変えないために残した
4. **soa を Kind `Soa` に**（Array + IsSoa をやめた）: sema の `Kind == Array && !IsSoa` 約 30 か所が `Kind == Array` だけに。
   soa のコンテナは IR に出ないので sema の外は変わらない
5. **`NamedIn(name, version)` をやめた**: fc 2 だけの整数型の名前（int など）の読み替えは sema の `basicType` 1 か所。types は
   文法の版を知らない。「Parse 直後に構文木を書き換える段」は取らなかった（構文木は不変の約束で、migrate が同じ構文木から
   fc 2 の名前を見つけて書き換えるため）

## 分けなかった Kind（決定）

slice（Struct + SliceOf）・enum（Int + Enum）・far な関数（Func + far）は独立した Kind にしない。数えると（2026-10-05）、
バックエンド（codegen の data.go・regalloc・interp・opt の inline）は slice を struct（3〜4 バイトの集まり）、enum を整数、
far を関数として扱うのが正しく、分けると新しい Kind を足さなければならない所（slice: Struct を見る約 40 か所の多く、enum: Int を
見る約 85 か所、far: Func を見る 15 か所）が、消えるフラグの検査（slice の `!IsSlice()` 約 4、far の `!IsFarFunc()` 約 6）より
ずっと多い。soa だけは sema の中に閉じていて、フラグの検査が消えるだけだった。

## 確かめたこと

各段で前の fcc と出力をバイト単位で比べて同じ（examples の castle・miku・miku4・jump・hello・statusbar・wave・life、bench 13 本と
test/ の 18 本の -O 0 / -O 2 の出力と警告、test/*.fc の `fcc check`）。`go test ./...` が通り、fuzz（TestRandom* と fold / mutate /
metamorphic、FC_VERIFY_IR）を各段 120〜160 本回して失敗なし。bugzoo のパッチ 2 つ（string-signed-bytes・soa-value-type）を
今のコードに合わせて作り直した。

## 診断の変化

- 値でない名前を値として使ったとき: 上の「not a value」の文言に（前は `cannot apply + to module and u8` など、または黙って通った）
- `const T:int = textmap(...)`: 「`const T: a macro has no type`」（前は互換でない型のエラー）

## 残り

- 計画の外の節点（代入・slice・リテラル・soa）の診断を型を決める段へ、lval が型を出し直す残り（cast・slice の組み立て）
- `ir.Value` の sema だけが使うフィールド（`Public`・`Build`・`StrConst` など）を Symbol へ
