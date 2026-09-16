# レジスタ割付（第 4 弾）: ループ内の A 常駐

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

- X は静的フレームの関数でもスタックの底（extern を呼ぶときに使う）なので、作業レジスタには使わない。Y は
  命令の間で空いているので Y 版の codegen に使える
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

段取り:

1. `ir.Dominators` / `ir.Loops`（`internal/ir/loops.go`）、命令ごとの生存集合 `ir.Liveness`（`internal/ir/live.go`）
2. `regalloc.AllocateResident(lmd)`（候補の評価と変換。`codegen.Prepare` で `opt.Optimize` の後、
   `AllocateRegister` の前に呼ぶ）
3. codegen の A 占有モード（friendly / Y 版 / 退避）
4. bench で確認（crc8 / crc16 / sieve / plasma / oam）、castle は Mesen まで
