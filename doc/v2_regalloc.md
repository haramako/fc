# レジスタ割付（第 4 弾）: ループ内の A / Y 常駐

2026-09-16 設計。目標は crc8 の内側ループを Oscar64 と同じ `asl a; bcc; eor #k` にすること、一般には
「ループの中で毎回 `lda x … sta x` している 1 バイト変数を A に置いたまま回す」こと。

SSA 化は先送りした。この割付は既存の IR（変数 + 命令列）の上で、命令ごとの生存集合（`LiveRangeCalculator` が
持っていて区間に潰していたもの）を使えば作れて、SSA 化後もそのまま使える部品（領域の選択、コストモデル、
A 占有中の codegen）だから。SSA の残りの価値（定数 / コピー伝播、DCE）は別の段で。

## 1. 考え方

- 対象: 最内ループ（`ir.Loops`。支配木と back edge から自然ループを求める）ごとに **1 バイトのローカル変数を 1 つ**
  （A は 1 本しかない）。アドレスを取られた変数・配列・struct・引数以外の 2 バイト値は対象外
- 候補 v のコスト: ループ内の各命令を 3 つに分類して合計する
  1. **v を扱えて A のまま実行できる命令**（friendly）: `load v = x`（lda）、`load x = v`（sta）、`op v = v, k`
     （add / sub / and / or / xor / 定数シフト。v が第 1 入力で結果も v）、`if v`、`eq / lt cond = v, k`（cmp）、
     `index_pget v = a[i]` / `index_pset a[i] = v` / `pset` 系（sta … ,y）、`push_arg v`、`uminus v = v`（eor #$ff; clc; adc #1）。
     得: lda / sta が消える分（3 サイクル / 回）
  2. **v を触らず A も壊さない命令**（A-free）: ラベル、jump、if_carry、inc / dec、メモリ上の定数シフト、
     **Y で代用できるもの**（1 バイトの `load x = y` → `ldy y; sty x`、1 バイト変数の `if x` → `ldy x; bne`、
     1 バイト符号なしの `eq / lt` の cond → `ldy a; cpy b`）。損得なし
  3. **それ以外**（A を壊す。乗除算、呼び出し、2 バイトの演算、v が第 2 入力の演算…）: 前後で **退避 / 復帰**
     （v が live-in なら `sta home`、live-out なら `lda home`。3 サイクルずつ）
- 正味（1 の得 − 3 の損）が正で最大の候補を採用。無ければそのループは何もしない
- 変換: 新しい一時変数 `vA`（`Location = LocA`、home = v 自身のメモリ）を作り、ループ内の v を vA に書き換える。
  ループの入口の辺に `load vA = v`（v が live-in のとき）、出口の辺に `load v = vA`（v が live-out のとき）を挿す。
  出口先のブロックに他からの流入があれば辺を分割する（`@exit_N: load v = vA; jump 先`）。
  ループ内の各命令に「この命令で A を占有している変数」（`Op.Resident`）と v の live-in / live-out を記録する
- codegen: `Op.Resident` が付いた命令は、friendly なら A のまま（`loadA(vA)` / `storeA(vA)` は何も出さない。
  `if vA` は `cmp #0` を出す（直前が A の演算ならピープホールが消す））、A-free なら Y 版、それ以外は
  `sta home` / メモリ版の codegen（vA を home として参照）/ `lda home` で挟む
- 既存の A 割付（定義直後に 1 回使う一時変数を A に）は、`Op.Resident` の付いた命令では行わない（A が塞がっている）

## 2. 制約と後回しにするもの

- ~~X は静的フレームの関数でもスタックの底（extern を呼ぶときに使う）なので、作業レジスタには使わない~~ →
  §4 で X を開放した。Y は命令の間で空いているので Y 版の codegen に使える
- ネストしたループは最内だけ。外側のループの変数は次の段（Y の常駐、または外側の A 常駐と内側での退避）
- 2 バイト変数は対象外（A は 8 ビット。上位バイトだけ A に置く、は効果が薄い）
- 関数呼び出しをまたぐループ（castle の `en.process` の中など）: 呼び出しごとに退避 / 復帰 6 サイクルなので、
  呼び出し 1 つに対して friendly な命令が 3 つ以上あれば得になる。コストモデルがそのまま判断する

## 3. 実装の段取り（1〜4 は 2026-09-16 に実装済み。bench: crc8 -17%、plasma -10%、entities -5%、bgdecode -3%、
textprint -2%。castle は 28 ループが対象）

実装で決めた細部:

- 候補は `LTNone` / `LTArg` の変数だけ（一時変数は定義の直後に使うものが大半で、既存の `allocateA` が扱う。候補に
  含めると「得 6」に見えるが実際は何も変わらず、しかも区間内の他の一時変数の A 割付を止めてしまう）
- `inc / dec` できる `x = x ± 1` は A でも `clc; adc #1` で 4 サイクル、メモリで 5 なので得は 1（6 と数えると
  crc16 でループカウンタを選んで退行した）
- 得は 1 周 3 サイクル以上を要求する（入口 / 出口の写しの分）
- 常駐変数が死んでいる区間 (`ResOut == false`) では既存の A 割付をそのまま行う（止めると textprint が退行）
- 退避先 (Home) の変数はループ内では vA の名前で使われて live range が途切れるので、専用の場所を与える
- 調査用: `FC_NO_RESIDENT=1` で無効化、`FC_TRACE_RESIDENT=1` で選んだ変数と見積もりを表示
- **Y の常駐**（同日、A と同じ枠組み）: 添字（`index_pget` / `index_pset` の `ldy i` が消える）とカウンタ（`iny` / `dey`、
  `cpy`）を Y に。A と Y は同じループで両方使える（`Classify` が (vA, vY) の組で命令を分類し、`bestPair` が組の得で
  選ぶ）。Y が塞がっている間は「A 占有時の Y 代用」ができないので、代用の代わりに A を退避する。Y の常駐変数を
  扱う friendly な命令（`iny` / `cpy` / `ldy` / `sty`）は A を使わないので A 側は free。bench: plasma -17%、crc8 -15%、
  textprint -9%、crc16 -9%、bgdecode / oam -6%。castle は 39 ループ
- ピープホールの `ldy x` の重複除去は、直後が分岐（Y 代用の `ldy x; bne`）のときだけ「フラグも Y を反映している」
  ことを要求する（常に要求したら entities が +6% 退行した）

**X の開放（§4）**: スタックの空き先頭を X からゼロページの `FC_SP` に移した。static / entry 関数は X を自由に使い、
stack 系（stack / entry / extern）の呼び先には `ldx FC_SP` してから S+k,x に引数を書き、戻り値を読む前にもう一度 `ldx FC_SP`
（呼び先が X を壊しうる）。stack（再帰）関数は X = 自分のフレームの底のままで、入口で `FC_SP = X + FrameSize`、return で
戻し、呼び出しの後は `X = FC_SP - FrameSize` で X を戻す。`call` マクロは使わなくなった。runtime は `ldx #0; stx FC_SP`
で始める。base.asm を自前で持つプロジェクトは `FC_SP: .res 1`（`.exportzp`）を足す（castle はスタックを 1 バイト削って充てた）。
X の常駐は static 関数だけ（グローバル配列の添字 `lda a,x` / `sta a,x`、カウンタ `inx` / `cpx`。ポインタの添字は Y しか
使えない）。同点なら Y を優先する。bench: bgdecode -4%、oam -3%、コストは fib +17%（再帰）、calls +4%（関数ポインタ経由）。

**グローバル変数の常駐（§5）**: volatile でない 1 バイトのグローバル変数も候補にする。生存解析は
`ir.BuildLivenessWithGlobals`（呼び出し・インラインアセンブラ・ポインタ経由の書き込みは全グローバルを読んで書く、
return は全グローバルを読む、とみなす）。それらの命令は常駐変数の退避 / 復帰になる（`ir.MayTouchGlobals`）。
volatile は sema（`options(address:)` / `options(volatile: true)`）と `codegen.markVolatile`（asm から参照されるシンボル。
sema が `include` した asm ファイルを読んで `Module.AsmSymbols` に控える）が付ける。

**関数全体の領域（§6、2026-09-16）**: ループごとの割付が終わった後、ループに含まれないブロック全部を 1 つの領域にして
同じ `bestPair` / `makeResident` を通す（入口は関数の先頭で 1 回、return はグローバルの書き戻しのため `needsY` /
`needsX` / A の clobber 扱い、中のループは「通過する区間」）。castle は 134 関数で引数などが A / Y / X に乗る
（`en.check_hit_rect` は y2 を Y、x2 を X に置いて `cpy` / `cpx` で比べる）。細部:

- 領域が重なるとき（外側のループ、関数全体）、内側の写し `load i@Y = i` / `load i = i@Y` には印を付けず
  （`isResCopy`）、この領域の写しは同じ辺で **退避は内側の復帰の前、復帰は内側の退避の後** に置く（`onEdge` の
  `spill`）。内側で常駐している変数（Home）は候補から外す
- 領域内で書き換えられない変数（引数など）は `Value.Clean`: レジスタを壊す命令の前の退避（書き戻し）も
  ループ境界の書き戻しも出さない（Home が常に最新。復帰だけ）
- **friendly の判定はその命令がレジスタを壊さないことまで含める**: 符号付きの `lt` は `sec; sbc; bvc; eor` で A を壊す
  （`cmp` と違って）ので、変数がその後も生きているなら friendly ではない（math.sin で `x < 64` の後の `tab[x]` が
  壊れた値を添字にしていた。plasma の出力チェックで発覚）
- 要素 2 バイトの配列 / ポインタの添字は `opt.scaleIndex` がブロック内で 1 度だけ 2 倍して `Op.Scaled` を付けるので、
  その `i*2` も Y / X の常駐の対象（`byteIndex`。math16 -4.5%）

段取り:

1. `ir.Dominators` / `ir.Loops`（`internal/ir/loops.go`）、命令ごとの生存集合 `ir.Liveness`（`internal/ir/live.go`）
2. `regalloc.AllocateResident(lmd)`（候補の評価と変換。`codegen.Prepare` で `opt.Optimize` の後、
   `AllocateRegister` の前に呼ぶ）
3. codegen の A 占有モード（friendly / Y 版 / 退避）
4. bench で確認（crc8 / crc16 / sieve / plasma / oam）、castle は Mesen まで
