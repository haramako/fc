# デバッグ用のゲームを 11 本作って見つかったこと（2026-10-08）

`games/` に 11 本（ブロック崩し・スネーク・マインスイーパー・倉庫番・落ちものパズル・縦シューティング・ドットイートの迷路・
テキストアドベンチャー・横スクロールのジャンプアクション・ローグライク・ワイヤーフレームのシューティング）を作った。どれも
`AUTO`（`-D main.AUTO=true`）で自分で遊び、`internal/nes/games_test.go` が内蔵のランナーで走らせて進み具合を変数で確かめる。
加えて、メインループの手数（`end_frame` の `ticks`）を決めた所で止め、`-O 0` と `-O 2` の main の変数をバイト単位で比べる
（`TestGamesSameAtTick`。AUTO の考える時間でフレーム数は違っても、論理は手数ごとに決まる）。11 本とも一致した。
fclib の足りない所は、ユーザーの指示（2026-10-08）で `games/common/` の追加のライブラリにして進め、終わってから fclib に移すかを決める。

## 直したもの

- **条件式の枝に型名を省いた struct のリテラル**（`var t:Cell = c ? {1, 2} : g.target();`）が構文エラーだった。仕様（language.md の
  条件式）は書けるとしていて、sema は扱えていた。文法に枝の `anon_struct_lit` を足した（6a90614、`TestCondExprAnonStruct`）。
  fuzz の生成器はこの形を作らないので bugzoo には足していない

## 相談したいこと（コンパイラ）

1. **静的フレームの RAM の予約が、使う量によらず 512 バイト**（`DefaultStaticRam`。base.s の `FC_SRAM: .res FC_SRAM_SIZE`）。
   NES で変数に使える RAM は $0200〜$06FF の 1280 バイトで、その 4 割。11 本のうち 7 本が RAM からあふれ、`@(static_ram: N)` で
   避けた（実際に使ったのは 0〜71 バイト）。frames の計画（`plan.RamUsed`）は base.s を作る前に出ているので、`static_ram` を
   書かなければ使う分だけにできる。グローバル変数の番地が変わるので examples の ROM（miku・castle）の golden が変わる
2. RAM があふれたときのエラーが ld65 の生の文言（`Segment ‘BSS’ overflows memory area ‘SRAM’ by 91 bytes`）。fc がモジュールごとの
   量と静的フレームの予約を示せるとよい
3. **CHR RAM（`chr = 0`）で `.chr` を `@include` すると ld65 の生のエラー**（`Missing memory area assignment for segment ‘CHARS’`）。
   しかも `fcc build` は失敗するのに、壊れた `.nes` が残る（RAM があふれたときは残らない）
4. soa に `@len` が使えない（`@len(Tasks)` が「soa は添字か型にしか使えない」）。配列と同じく要素の数を返すのが自然
5. interface の slice を返すメソッドは、既定の本体が必須（`.none` 用の「何もしない」関数が長さ 0 の slice を作れない）
6. 大きい配列の一部を定数の長さで切った slice（`bits[i..i + 8]`、i は u16）が `[:u16]` になり、`[]const u8` に渡せない
   （`@slice(&bits[i], 8)` で避けた）

## 相談したいこと（fclib。今は games/common に置いたもの）

- `gfx.fc`: tiles の並びの 16×16 の BG を置く `put16`（`vram.put_meta` に 4 つの番号を並べる）、16×16 のスプライトを 8×8 / 8×16 の
  モードで置く `spr16` / `spr16_tall`（パレットと反転を呼ぶ側で。`oam.meta` は上下の反転が 8×8 前提で、パレットも表に固定）
- `text.fc`: 文字の窓（単語で折り返し、下まで行ったら上へずらす。大きさの上限は `@(build)`）。ずらすと全部の行を書き直すので、
  キューで 4〜5 フレームかけて送る間は画面が中途半端（BG のスクロールを使う形の候補）
- `bitmap.fc`: CHR RAM のビットマップ（RAM に描いた 64×64 の線画を 1 フレームに 6 タイルずつ送り、パターン表を切り替える。
  1 枚に約 11 フレーム）
- スプライト 0 の当たりで画面を分ける処理（今はジャンプアクションの中。待ちに上限を付けた）

## fclib の不足（相談）

- **NES の `sys.panic` が画面に何も出さずに止まる**: console.init を呼んでいないゲームでは文言が画面に出ない（デバッグ出力の
  $4018 には出る）。fmt の書き先があふれたときなどに黙って止まって見えた（2 回）。panic が自分で console を準備するのがよい
- 書式に文字列の幅（左寄せで空白を埋める）が無い。前に出した長い文字を消すのに、別に空白を埋める
- `bits` の添字が u8（256 ビットまで）。盤のビットは行ごとの配列にした

## テストの仕組みで足したもの

- `nes.Machine.Stop`（命令ごとの止める条件）、スプライト 0 の当たりの近似（BG との重なりは見ない）、CHR RAM（iNES の CHR が 0）
- games のテストは panic（console.exit）で止まったら、その文言（`Machine.Output`）を出して落ちる
- `TestGamesSameAtTick` は bugzoo の最適化のバグ 5 つ（index2-carry など）はどれも見つけなかった（fuzz の作る特別な形のバグ）。
  実際のゲームの形のコードで出る新しいバグに効く
