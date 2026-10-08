# fuzz で見つかったバグと生成器の拡張の記録（2026-09-19〜）

`doc/development_notes.md` の「テストの三段構え」から切り出した時系列の記録（種の範囲・見つかったバグ・固定したテスト名・生成器を広げた経緯）。回し方と調べ方は [../wiki/testing-and-fuzzing.md](../wiki/testing-and-fuzzing.md)。追記型: 続きは日付を付けて足していく。

- 種 160000〜 で 2 件: splitWords が途中で 1 バイトに狭めた cast の連鎖 `((p0 as int) as int16)` を分けた変数の上位で
  読む（`plainWord`。`TestSplitNarrowWiden`）、sign_extension の入力が A にある（呼び出しの戻り値。call を A 割付の
  producer にしてから）とき N が A を反映しないまま `bpl` していた（`TestSignExtendCallResult`）
- 種 170000〜 で 1 件: A に常駐したグローバルを `return g0` で返すと、return が friendly（戻り値を A から書く）扱いで
  g0 の書き戻しが出なかった（グローバルの常駐は return を clobber に。`TestResidentGlobalReturn`）
- **2026-09-20 に生成器をさらに広げた**: far call（`far1.fc` = `options(bank: 1)` のモジュールに関数を 1〜2 個置いて
  main から `far1.ff0()`。main のグローバルは見えないので引数とローカルだけ）、密な switch（case 0〜11 で
  ジャンプテーブル = `switch` 命令になる形）、const の表（`const ct0:[16]T = [...]`）、関数ポインタ表
  （`const fp0:[N]fn(...):R = [t0, ...]` を `fp0[(e & (N-1))](args)` で呼ぶ。要素の関数は Entry になる）。
  初回の 30 本で 1 件: regalloc の live range の流れ（`CalcLiveRange` の `flow`）に `switch` 命令の飛び先が無く、
  飛び先で使う変数の生存区間が切れて直後の一時変数と番地を共有していた（`TestSwitchTableLiveRange`。
  ジャンプテーブルは 2026-09-19 からあり、castle では偶然重なっていなかった）
- 種 190000〜 で 1 件: 常駐レジスタへの差し替え（`makeResident` の replace）が cast を落としていて、`(x as int) >= 0` の x が
  X に常駐すると比較が符号付きになった（cast を残す。`TestResidentKeepsCast`）

- 2 かたまり目（種 372000〜、6 万本）で 2 件: ピープホールが番地を綴りで追跡していて `1+<F+5` と `0+<F+6`（同じ番地）
  を別物と見て、必要な `ldy` を消した（`canonAddr` で `k+<L+n` → `<L+(n+k)`、`+0` は落とす。`TestPeepholeAddressSpelling`。
  最初は `<F+0` の綴りが揃わず calls +3% になった: 正規化は**全部の綴り**に掛ける）。内側の領域から次の内側の領域へ
  移る辺で、外側の常駐の復帰 `ldx p1` が次の領域の入口の写し `ldx l3` の後ろに出て、X が p1 のままループに入った
  （復帰は前の領域の退避の後・次の領域の入口の写しの前: `isResSpill` で区別。`TestResidentEntryAfterRestore`）。
  失敗した種は `git worktree add /c/Work/fc_base <前のコミット>` で古い版と比べると、並行して入った他の変更の影響を
  切り分けられる

- 7 かたまり目（種 682000〜、6 万本）は失敗なし（長時間 fuzz で初めて）。8 かたまり目（種 742000〜781999 の 4 万本で止めた）で 1 件: 2 バイトの
  `g2++` は `inc lo; bne @s; inc hi` なので、下位が 0 に折り返すと Z は上位を映すのに、直後の `if ((g2 as sint))`
  （下位バイトの検査）が `flagsFromIncDec` でその Z を使っていた（2 バイトの inc の後は使わない。2 バイトの dec は最後が
  `dec lo` なので下位を映す。`TestIfLowByteAfterInc16`）

- 広げた生成器の 10 万本（種 1000000〜1099999）で 2 件（どちらも同じ原因）: 再帰関数（stack 系）に -O 2 の
  インライン展開が入ってフレームが 223 / 260 バイトになり、`<S+127,x` のようなゼロページの外の番地を出して ld65 / ca65 が
  範囲エラーになった。stack 系のフレームは FC_STACK（128 バイト、`regalloc.StackSize`）を超えたら `frame size over`
  に（`TestStackFrameTooLarge`。fuzz はプログラムが大きすぎるとして飛ばす）。実行結果の食い違いは 0 件
- 続けて種 1100000〜1249999 の 15 万本（e544757、インタプリタとバンク切替込み）は失敗なし。広げた生成器で累計 25 万本、
  見つかったのはフレームの上限の 1 件だけ。これ以上は生成する形を広げる（struct の入れ子・配列フィールド、alias、ラムダ…）
- **ポインタ経由の配列フィールド**（2026-09-23、生成器を広げる前の手計算で発覚）: `ta[i].arr[j]` / `p.arr[j]` で、
  sema の rval がフィールドへのポインタを pget して配列の中身を読み、それを番地として添字を足していた（別の場所に書く）。
  配列の値は番地なので、要素へのポインタとして読み替える（`TestArrayFieldViaPointer`）。-O 0 / -O 2 / インタプリタが
  同じ IR を忠実に実行するので 3 つとも同じ値になり、**sema のバグは差分でもインタプリタでも見えない**（期待値を
  別に計算する判定が要る。バンク切替の fuzz は生成器が期待値を計算している）
- **生成器をさらに広げた（2 回目）**（2026-09-23）: struct の入れ子と配列フィールド（`struct T { s:S; arr:[4]…; x:… }`、
  `u0` / `ua:[2]T`、`pt:*T`、T 全体のコピー）、ストレージ alias（`var ab:[16]int; alias w:S = ab;` で同じ場所を配列と
  struct で読み書き）、ローカルの struct 変数（`var ls:S = {…}`）、struct の値渡しと値返し（`sv(v:S, k):S`）、main の
  ラムダ（`lf`）、ラベル付きのループと `break L` / `continue L`（ポインタをずらすループの本体からは外に抜けない:
  後のポインタの戻しを飛ばすので）。見つかったもの: 上のポインタ経由の配列フィールド（sema）、関数でない値の呼び出しで
  sema が panic（`TestCallNonFunction`）、常駐の出口の写しの順序（`TestResidentExitThroughCopyBlock`: 内側のループの
  出口を分割した写しだけのブロックが関数全体の領域の出口でもあるとき、書き戻しを内側の退避の前に置いていた）、
  インタプリタの 3 件（struct の定数データ、関数の中のシンボル `_2` の衝突、8 バイトを超える値のコピー）。
  関数ポインタ表の関数の名前 `t0`〜 とぶつからないよう、T の大域変数は `u0` / `ua`
- 広げた生成器の 10 万本（種 3200000〜）の途中で 1 件: splitWords の後の伝播（`propagateBytes`）が、sint8 の一時変数
  （`l0 * 0`）を uint8 のリテラル 0 に置き換えて、`(7 << l4) >= (l0 * 0)`（1 バイトの符号付きの比較。224 は -32）を
  符号なしの比較に変えていた（`TestPropagateBytesKeepsType`）。**入力を置き換えるときは元の型を保つ**（リテラルは元の型で
  作り直し、型の違う一時変数は cast で包む。SSA の定数の置き換えとコピーの伝播は元から型を保っている）。2026-09-28 から
  比較の幅と符号・除算と右シフトの符号は命令が持つ（`ir/sign.go`）ので、この類の食い違いは起きない。狭い入力のゼロ拡張と
  uminus の読む幅は今も入力の大きさで決まる。最小化は初期化文（main の先頭の大域変数・配列・struct・soa の代入と
  ローカル配列の要素）を全部残すように（`rpStmt.keep`。以前は `laN[k] = …` だけで、大域配列の初期化が消えると
  未初期化の読み出しで emu とインタプリタの値が違った）
- 続けて種 3400000〜3549999 の 15 万本（c762cb9、インタプリタとバンク切替込み）は失敗なし。2 回目に広げた生成器で、
  修正後に累計約 16 万本
- 2026-09-24（a659671: asm 入り inline の別モジュール展開、フレーム超過時の展開の取り消し、関数ポインタ表から reg へ直接）:
  種 3550000〜3569999 の 2 万本とバンク切替の fuzz 1000 本は失敗なし。4 要素の関数ポインタ表を 20 要素に水増しして
  間接呼び出しを試すようにした（わざと壊すと 300 本中 23 本で検出）。飛ばした種は約 7%（ROM に入らない 6.3%、両方で
  サイクル上限 0.4%、フレームが大きすぎる 0.2% = -O 0 でも超える）で、水増しの前と同じ割合
- 2026-09-25（4b5535e: ポインタの下位を Y で回す walkPointerY、変数の上限の誘導変数）: 生成器に walkLoop（300 バイトの
  バッファを 17〜290 回なめる）を足した。既存のポインタのループは回数が 8 以下で展開されるので、変換が起きるのは 500 本中
  5 回だったのが 157 回に（出口で p を戻さないように壊すと 150 本中 13 本で検出）。種 3572000〜の 2000 本で 1 件:
  stack 系の関数が大きいフレームの後ろに引数を積む位置 `<S+128,x` が ld65 の範囲エラー（main でも再現。bb74d04 で
  frame size over にして展開を止めてやり直す）。修正後、種 3600000〜3619999 の 2 万本とバンク切替 1000 本は失敗なし
- 続けて main（4402194）で種 3620000〜3679999 の 6 万本とバンク切替 2000 本は失敗なし（飛ばした種 7.3%: ROM に
  入らない 6.4%、両方でサイクル上限 0.7%、フレームが大きすぎる 0.2%、ソフトウェアスタックのあふれ 8 本）

- **生成器をさらに広げた**（2026-09-23）: soa（`soa E:[8]S`。フィールドの読み書き、要素ハンドル `h:*E`、要素の
  gather / scatter）、struct の値のコピー（`s0 = sa[i]`、重なりうる `sa[i] = sa[j]`、`*ps = …`）、`ps:*S` の引数、
  far1 の関数の farfn 表（`const fq0:[2]farfn(…)` をトランポリン経由で呼ぶ）、main の関数ポインタのローカル変数
  （`fv`。付け替えて呼ぶ）、自分を呼ぶ関数 `r0(n, a)`（stack ABI。深さは `(e & 3)` で 4 まで）、`min` / `max`、
  2 バイトや符号付きのループカウンタ。**乱数の使い方が変わったので、これより前の種の番号は別のプログラムになる**
  （古い種を再現するときはこのコミットより前で）。ROM に入らずに飛ばす種が 300 本中 5 → 11 本に増えた

- 6 かたまり目（種 618000〜、6 万本）で 2 件、長時間 fuzz はここでいったん止めた（累計 37 万本で 14 件）:
  (1) stack 系（関数ポインタ経由）の呼び出しは push_result で `ldx FC_SP` してから `sta <S+k,x` で引数を積むが、X に
  常駐した変数の復帰 `ldx g1` が push_result の直後に出て X が戻り、引数が別の場所に書かれていた（push_result から
  call までは X の常駐をメモリ側にする `holdX`。入れ子の深さで数える。`TestStackCallHoldsX`）。(2) SSA の書き換えが
  `load l1 = ((l1 as int) as int16)` を「同じ場所・同じ型」だけ見て `load x = x` として消していた。内側で 1 バイトに
  狭めてからゼロ拡張するので切り詰めが消える（cast の各段が内側の幅に収まるときだけ消す `castFits`。
  `TestSSACastTruncateReload`）。**cast の連鎖の等価性は外側の型とオフセットの合計だけでは決まらない**（`castBits` と
  同じ穴）。ほかに `frame size over` が -O 2 だけで出る種（展開・インラインでフレームが 256 バイトを超える。runner は
  skip、roadmap に）
- 5 かたまり目（種 556000〜、6 万本）で 1 件: 2 バイトの `g0 -= 1` は `lda lo; bne; dec hi; dec lo` で下位を見るので
  A を壊すのに、freeA が 1 バイトの dec と同じく「A を使わない」と見ていて、A に常駐した g1 が消えた（2 バイトの inc は
  `inc lo; bne; inc hi` で壊さない。`TestResidentDec16`）
- 4 かたまり目（種 496000〜、6 万本）で 1 件: 符号付きの `x < 0` は x の最上位バイトの N フラグを見るが、x が呼び出し
  （除算のランタイム）の戻り値で A にあるとき、call の後の常駐の復帰 `ldx` が N を壊していた（`if` の戻り値検査と同じ
  形。A にある値は `testA` で `cmp #0`。`TestSignedLtZeroAfterCall`）。**call の後に常駐の復帰が出るので、call の直後の
  命令が「直前の lda のフラグ」を当てにする形は全部この穴がある**（if、符号付き `< 0`。残りは cmp / sbc を自分で出す）
- 3 かたまり目（種 434000〜、6 万本）で 1 件（2 本）: `x++` の直後の `if (x)` で x が A に常駐していると
  `flagsFromIncDec` が `byte()` で綴りを比べようとして codegen が panic（`invalid location a`）。A にある値は if の
  codegen が `cmp #0` で検査するので、A にある値では false に（`TestIfAfterIncResident`）
- 2026-09-30、main（77bc152〜d7fe34f）の連続 fuzz（種 54722000〜55238000）で 1 件: SSA の simplify で
  `(a + b) - b → a` と置き換えた命令の入力の版（`useAt`）が古い命令のまま残り、後ろの命令がその命令を写し（`load $2 = g3`）
  として辿ると古い入力 `$1 = g3 + g0` に着いて、もう一度 `(g3 + g0) - g0` として畳んでいた（`g3 = ((g3 + g0) - g0) - g0`
  が `g3 = g3` に）。置き換えたら `useAt` を捨てる（rewrite の命令ごとの置き換えも同じ。`TestSSASimplifyAfterRewrite`、
  bugzoo の ssa-stale-useat）。ほかは TestLogZeroCost の上限超え（既知）だけ
- 2026-10-08、メソッド・interface・書式の生成器を足した（TestRandomMethodsV4 / TestRandomFormatV4 / TestRandomFarIfaceNES。
  Agent/wiki/testing-and-fuzzing.md「fc 4 の機能の生成器」）。TestRandomFarIfaceNES の最初の 50 本で 1 件: near の interface で
  `uxrom.prg(@bank_of_id(@id_of(e)))` してから呼ぶと、実装が最初のメソッドを書かず既定の本体を使うとき、`@bank_of_id` が既定の本体
  （固定の所）のバンクを返して違うバンクに切り替わり、実装のメソッドが別のバンクの中身を実行して止まった。`$Task_bank` を最初の
  メソッドの振り分けの表の要素から作っていた（実装が書いた最初のメソッドのバンクに。`TestInterfaceBankOfIdDefault`、bugzoo の
  bank-of-id-default）。TestRandomMethodsV4 と TestRandomFormatV4 は 2000 本ずつ通った
