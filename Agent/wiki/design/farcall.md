# far call（バンクをまたぐ関数呼び出し）

2026-09-15 設計メモ。Agent/discussions/2026-09-14-v2-idea.md「interbank call を実装する」。実装済み（§7）。利用者向けの説明は docs/reference/banks.md。

## 1. 動機

MMC3 のように PRG が切替バンクに分かれていると、別バンクの関数を呼ぶには「今のバンクを覚える → 切替 → 呼ぶ →
戻す」が要る。castle は `bg.fc` の `fetch_area` / `scroll` / `update_palette` のように呼ぶ側に手書きのラッパーを置いていて、
どのモジュールがどのバンクに載っているかを人が管理している（`common.fc` の `PBANK_*` 定数と `ld65.cfg` の二重管理）。
これをコンパイラに任せる。

## 2. 先行事例

| 処理系 | やり方 |
|---|---|
| SDCC / GBDK（Z80, GB） | 関数の `__banked` 属性。呼ぶ側は普通に書き、コンパイラがプラットフォーム提供の `__sdcc_banked_call` 経由の呼び出しを出す |
| KickC（6502） | `__bank(name, id)` で所属バンクを宣言。別バンクへの呼び出しはコンパイラがトランポリンを生成 |
| Prog8（X16） | `sub foo() @bank 4`。呼び出しは普通に書き、JSRFAR を出す |
| cc65 / neslib | 言語機能なし。`banked_call(bank, func)` を手で呼ぶ |

共通点: **所属バンクは宣言側の属性、far call はコンパイラが自動、トランポリンはプラットフォーム側**。呼ぶ側に構文は無い。

## 3. 決めたこと

### 3.1 バンク番号はリンカから取る（`.bank`）

castle は `ld65.cfg` の SEGMENTS で ROM を割り当てており、`options(bank: N)` の数字は placement に使われていない（古い値が残っている）。
そこでコンパイラはバンク番号を知らず、生成コードで ca65 の `.bank(シンボル)` を使う。ld65.cfg の MEMORY に `bank = N`
（マッパーのレジスタに書く値）を付けておくと、リンク時にそのシンボルの領域のバンク番号に解決される（別モジュールの import でも可、確認済み）:

```
MEMORY { ROM5: start = $a000, size = $2000, file = %O, fill = yes, bank = 5; ... }
	lda #<.bank(_my_process__foo)      ; → lda #5
```

fc が cfg を生成する構成（emu / nes 既定）では `bank = N` を自動で付ける。自前の cfg を持つプロジェクトは切替 ROM に付け足す
（付け忘れると 0 に解決されるので、参考 cfg を examples/castle に置く）。

### 3.2 far か near かの判定（コンパイル時）

`options(bank:)` の**数字は見ず、有無と符号だけ**を見る:

| モジュール | 判定 | castle の例 |
|---|---|---|
| `bank` 無し、または負 | 固定バンク | ROML の math / mem / ppu / bg（`-2`）/ common（`-1`）… |
| `bank` が 0 以上 | 切替バンク | my（0）/ en1〜8 / menu（2）/ boxelev（0）… |

呼び出しの規則:

- 呼び先が**固定バンク**、または**同じモジュール**内 → 今までどおり `jsr`（near）
- 呼び先が**切替バンクの別モジュール** → far call。呼び出し元が固定でも切替でも同じ（呼び出し元の code は呼び先の実行中に
  アンマップされてよい。戻りはトランポリンがバンクを戻してから `rts` する）
- 呼び先の関数に `options(near: true)` があれば、呼ぶ側はマップ済みと仮定して `jsr`（opt-out。熱い経路用。責任は書いた人）
- `options(segment: X)` で別のセグメントに置いた関数は、X がモジュール名ならそのモジュールの置き場所として判定する
  （castle の `function chest_event() options(segment: my)` は `my` のバンク扱い、`segment: common` は固定）。
  X がモジュール名でなければ配置は手動なので near

同じ切替バンクの別モジュール同士（同じ ROM に 2 モジュール）や、`en`（slot 1）→ `en1`（slot 0）のように両方マップ済みの場合も
far call になるが、トランポリンが**実行時にバンクを比べて切替を省く**ので +44 サイクル程度で済む（§5）。

### 3.3 呼び出しの形（ABI）

引数・戻り値は普通の関数と**完全に同じ**（フレームに積む / fastcall なら FC_FASTCALL_REG。static の呼び先の A / Y の引数と A の
戻り値のレジスタ渡しも同じ: 2026-10-05、§3.7）。加えて

```
FC_FARCALL: .res 3         ; base.asm の BSS に追加 (ZP に空きが無いプロジェクトがあるので絶対アドレス)。+0,+1 = 呼び先アドレス、+2 = バンク番号 (.bank)
```

にセットして、トランポリン `farcall_ay` を `jsr` する（stack 系の呼び先は `ldx FC_SP` してから）。A / Y の引数と A の戻り値は
トランポリンがそのまま通すので、呼び出し側のコードは呼び先の種類だけで決まる。

生成コード（static の呼び先。最後の 2 つの引数は Y と A に置いたまま。FC_FARCALL は X で置く）:

```
	ldy #1                          ; 最後から 2 つ目の引数
	lda #2                          ; 最後の引数
	ldx #<_far1_add                 ; 呼び先の入口 (A / Y で受け取る入口)
	stx FC_FARCALL+0
	ldx #>_far1_add
	stx FC_FARCALL+1
	ldx #<.bank(_far1_add)
	stx FC_FARCALL+2
	jsr farcall_ay
	sta 0+<F_main+3                 ; 戻り値は A から
```

### 3.4 トランポリン `farcall_ay`（ターゲット側が提供）

`runtime_init` と同じく、ターゲットごとに asm で用意する。fc 4 の fclib は uxrom / mmc1 / mmc3 のモジュールがトランポリンを持つ
（`fclib/nes/farcall_uxrom.asm` / `farcall_mmc1.asm` / `mapper_mmc3.asm`）。自前の mmc3 モジュールを持つプロジェクト向けに
`fclib/nes/farcall_mmc3.asm` を参考実装として置く。emu とバンク切替の無い nes には「切り替えずに飛ぶだけ」の実装
（`fclib/<target>/farcall.asm`）を fc がリンクする。規約（2026-10-05 に今の形。§3.7）:

- 入力: `FC_FARCALL`（アドレス、バンク）。**A / Y は呼び先の引数としてそのまま呼び先へ渡す**。X は自由だが、呼び先へは
  **X = FC_SP** で入る（stack 系の呼び先は X を引数の底として読む）。FC_FASTCALL_REG は触らない
- 出力: **A は呼び先の戻り値をそのまま返す**。X / Y は壊してよい（呼び出しは X / Y を壊す扱い）
- スロットは**呼び先アドレスの上位バイト**で決める（MMC3: $80〜$9F → slot 0、$A0〜$BF → slot 1）。コードはリンクした番地でしか
  動かないので、これで一意に決まる。16KB 領域や特殊なモードは `bank = N` の値の規約でトランポリン側が解釈すればよい
  （fc は値を素通しするだけ）
- そのスロットの今のバンク（castle なら `mmc3.pbank_bak`）と比べて、同じなら `ldx FC_SP; jmp (FC_FARCALL)`（呼び先の `rts` が
  呼び出し元へ戻る）。比べるのは X だけで行う（`ldx` / `cpx`。A / Y は引数）
- 違えば A の引数と今のバンクをハードウェアスタックに退避 → 切替 → A を戻して（`tsx; lda $0102,x`）`jsr` で間接ジャンプ →
  戻り値を Y に避けて復帰 → `tya; rts`。退避がスタックなので入れ子も動く。MMC3 はスロットごとに切替の経路を分けて書く
  （スロットの番号を積まず、BANK_SELECT の値を即値にする）
- IRQ ハンドラもバンクを切るなら、レジスタ書き込みの前後を `sei` / `cli` で囲む（`set_pbank` と同じ）
- **`farcall_ay` 自身は固定バンクに置く**。参考実装は `.segment` を書かず、include したモジュール（mmc3 → ROML）のセグメントに
  入る。`.segment "CODE"` などを書くと、castle では CODE が切替領域（ROM20、$8000）なので、切替の瞬間に自分自身が
  アンマップされて暴走する（初回適用時に踏んだ。§3.6）

MMC3 の参考実装は `fclib/nes/farcall_mmc3.asm`（castle の `mmc3.fc` の `pbank_bak` / `BANK_SELECT` を使う形。castle の
`src/mmc3.asm` は同じものに BANK_SELECT の写し `_mmc3_select_shadow` を足したもの）。

### 3.5 関数ポインタと対象外の範囲

- `fn(...):T` は従来どおり 2 バイト。間接呼び出しのバンクは呼ぶ側が管理する。
- `farfn(...):T` を 3 バイト（アドレス + バンク）の型として追加した。
  `options(farcall: true)` のもとで通常の呼び出し構文を使い、バンクを切り替えて呼び、元へ戻す。
  仕様・castle での使い分けは [farcall 対応の関数ポインタ](far-function-pointers.md)。
- 他バンクの**データ**参照: 今までどおり `set_pbank` で切り替えて読む。
- 割込みハンドラからの far call: 引き続き対象外。トランポリンの状態（`FC_FARCALL` と、スロットの今のバンクの写し）は主の処理と
  共有なので、主の far call の途中（FC_FARCALL を置いてから飛ぶまで、写しとマッパーを書き換えている間）に割り込みの中で far
  call すると、主の呼び出しが別の関数へ飛ぶかバンクがずれる。MMC3 の BANK_SELECT / BANK_DATA の 2 回の書き込みも、`sei` は
  IRQ しか止めないので NMI には効かない。2026-10-05 から、割り込みから届く関数（frames の irqTree）の far call は警告にした
  （`frames.Place`。`TestFarCallInInterruptWarns`）。レジスタ渡し（§3.7）で増えた危険は無い（A の退避はハードウェアスタックで、
  割り込みはその下に積む）

### 3.7 レジスタ渡し（2026-10-05）

static の呼び先へのレジスタ渡し（最後の 1 バイトの引数を A、その前の 1 バイトの引数を Y、1 バイトの戻り値を A。
[frame-alloc.md](frame-alloc.md) §7.1）を far call でも使う。前はトランポリンが A / Y を壊したので、far call は引数を全部
フレームに書いて `__frame` の入口から入り、戻り値もフレームから読んでいた。

- **トランポリンの規約を変えた**（§3.4）: A / Y の引数を通し、A の戻り値を返す。速い経路は X だけで比べ、切替の経路では A の
  引数をハードウェアスタックに退避して呼ぶ直前に戻す（Y は触らない。MMC1 は `mmc1_set` が Y を壊すので Y も積む）。呼び先へは
  X = FC_SP で入る（stack 系の呼び先の引数の底。前は X をそのまま通していた）
- **名前を `farcall` から `farcall_ay` に変えた**: 前の規約のトランポリン（プロジェクトがコピーしたもの）を黙って使うと引数と
  戻り値が壊れるので、リンクのエラー（`Unresolved external 'farcall_ay'`）で気づくようにした。examples/castle の `src/mmc3.asm`
  も書き換えた（実プロジェクトの castle も同期のときに書き換える）
- **FC_FARCALL は X で置く**（`ldx #<sym; stx FC_FARCALL`…）: 引数を A / Y に置いた後で置くので A / Y を使えない。X は呼び出しが
  壊す扱いで、call の命令の前に X の常駐は退避済み（codegen の compileOp）なので、call の命令の中で使ってよい。stack 系の
  呼び先（引数を `S+k,x` に積む）と extern の fastcall は今までどおり A で置く
- 呼ぶ側の変更は印と規約の条件から far を外しただけ: `callPlan.regParam`、`markArgY`、`CallConv.ResultFromA`（frames の
  `Result.OnlyA` もこれを見るので、far call で呼ばれる関数も戻り値を A だけで返せる）
- extern の fastcall の far call は、トランポリンが X = FC_SP にして入るので、stack の関数から呼んだら X（フレームの底）を戻す
- 効果（castle、`TestCastleFrameCycles`）: field 8626 → 8559（−0.8%）、area8b −0.6%。トランポリンの時間は field で 1 フレーム
  610 → 543 サイクル（ほとんどが slot 1 の切替の経路で、切替の経路をスロットごとに分けて書いた分が効いた。呼ぶ側の関数の時間は
  変わらない: castle の熱い far call は引数が無いか 2 バイト）。ROM は 244474 → 244454（呼ぶ側 −75、トランポリン +55）
- 確かめ方: `TestFarCallRegisters16K`（UxROM / MMC1）・`TestFarCallRegistersMMC3`（fclib のトランポリンで、切替あり・無し、
  同じスロットの入れ子、スロット 1 の切替、stack の呼び先・stack の関数からの呼び出し、ループの中、2 バイトの戻り値）、
  `TestRandomBankPrograms`（farcall_mmc3.asm。引数 2 つの関数と再帰する関数を生成器に足した。トランポリンで Y を壊すと
  100 本中 20 本が落ちることを確かめた）

## 4. オプションと診断

- `options(bank: N)`（モジュール）: 既存。意味に「0 以上なら切替バンク」が加わる（数字は生成 cfg の placement にだけ使う）
- `options(near: true)`（関数）: 呼ぶ側に near call を許す opt-out
- `fcc build` / `check` の末尾に far call の箇所数をモジュールごとに出せるようにする（`-d` のとき）。熱い経路の確認用
- `farcall_ay` が無ければリンクエラー（`Unresolved external 'farcall_ay'`）。emu とバンク切替の無い nes は fc が用意し、
  fclib の uxrom / mmc1 / mmc3 のモジュールも持つので出ない
- 割り込みから届く関数の far call は警告（§3.5）

## 5. コスト（MMC3 のトランポリン、near の `jsr` に対する追加分。2026-10-05 の形）

| 経路 | 追加サイクル |
|---|---|
| near（今までどおり） | 0 |
| far・切替不要（そのスロットに既に入っている） | slot 0 約 44、slot 1 約 50、固定バンク約 40（呼び出し側の FC_FARCALL の設定 18 + 判定 + `ldx FC_SP` 3 + 間接 jmp 5） |
| far・切替あり | 約 130（退避・切替・復帰・`rts` を含む） |

レジスタ渡し（§3.7）の分、呼び出し側の引数の `sta` と呼び先の入口の `lda` / `ldy`、戻り値の `lda` が減る。

今の手書きラッパー（`set_pbank` ×2 + ラッパー関数）は 110〜130 サイクルなので、切替ありでも今より軽い。
1 フレーム 29,780 サイクルに対し、敵 8 体 × 100 = 800（2.7%）。エンティティ単位のディスパッチには十分、
タイル単位の内側ループには向かない（そこは `near` か、同じモジュールにまとめる）。

### 3.6 castle で分かったこと（2026-09-15、初回適用時）

- `FC_FARCALL` は **BSS でなくてもよい**（絶対アドレスならどこでも）。castle は BSS（SRAM $0200〜）も満杯だったので `BSS_EX` に置く
- トランポリンを fc のモジュール（mmc3.fc）から `include` すると、そのモジュールの asm には fc が `.global farcall_ay` を出し
  （2026-10-05 までは `farcall`）、include した側が `.export`/`.global farcall_ay` していれば定義側として export になる。参考実装の `_mmc3_pbank_bak` も
  同じ理由で `.import` でなく `.global`
- 参考実装に `.segment "CODE"` を書いていたら、castle の CODE は ROM20（$8000 の切替領域）なので `farcall` が $8000 に
  置かれ、起動直後から暴走した。map ファイルの `farcall` の番地が $C000 以上であることを確認する
- far call の呼び出し側は near より **12 バイト大きい**（`FC_FARCALL` への 3 回の `lda`/`sta`）。castle は 155 箇所が far になり、
  そのうち約 100 箇所が `en1〜8 → en`（常に slot 1 にマップされている）で、ROM15 (en7) / ROM16 (my_process) が溢れた。
  `en.fc` に `options(near: true)` を付けるのが現実的（バンクが満杯なので）。将来、呼び出し側を 6 バイトにする
  「`jsr farcall` の直後にアドレスとバンクを置く」形（トランポリンが戻り番地から読む。+20 サイクル）を検討する

## 6. castle での移行

1. `ld65.cfg` の切替 ROM（ROM0〜ROM19 など）に `bank = N` を付ける（N は `PBANK_*` と同じ値）
2. `farcall_ay` を用意する（参考実装をコピーするか `src/mmc3.asm` などに置いて `include`）。`.segment` は書かず mmc3 の
   セグメント（ROML、固定）に入れる
3. `bg.fc` の `fetch_area` 系ラッパーは不要になる（残しても動く）。`set_pbank` の手動切替はデータ参照のためだけに残る
4. `fcc check` の far call 一覧で、熱い経路が far になっていないか確認。必要なら `options(near: true)`

## 7. 実装計画（fc 側、1 日程度）✅ 2026-09-15 実装済み（1〜6 すべて。NES 実機での確認は castle 側で）

1. base.asm に `FC_FARCALL: .res 3`（BSS、`.export`）。fc 生成の ld65.cfg に `bank = N`
2. sema: 呼び出しで「呼び先モジュールが切替バンク（`bank` ≥ 0）かつ別モジュール、かつ `near` でない」を判定し、
   `OpCall` / `OpFastcall` に `Far: true` を付ける（IR ダンプは `(:call ... far)`）
3. codegen: Far なら FC_FARCALL のセットと `call farcall, #n` / `jsr farcall`。fastcall の引数積みの後にセットする
4. `fclib/emu/farcall.asm`（jsr するだけ）と `fclib/nes/farcall_mmc3.asm`（上の参考実装）。emu 既定でリンク
5. テスト: emu で「切替バンクのモジュール」を作って far call が通ること（トランポリンに通ったことをカウンタで確認）、
   `near` の opt-out、fastcall の far、入れ子。NES は castle の自動プレイで確認
6. リファレンス（§4 関数の options に `near`、§1.5 に `bank` の意味）と本メモの更新
