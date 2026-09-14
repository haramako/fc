# far call（バンクをまたぐ関数呼び出し）

2026-09-15 設計メモ。v2_idea.md「interbank call を実装する」。実装済み（§7）。利用者向けの説明は language_reference.md §4.4。

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

同じ切替バンクの別モジュール同士（同じ ROM に 2 モジュール）や、`en`（slot 1）→ `en1`（slot 0）のように両方マップ済みの場合も
far call になるが、トランポリンが**実行時にバンクを比べて切替を省く**ので +37 サイクルで済む（§5）。

### 3.3 呼び出しの形（ABI）

引数・戻り値は普通の関数と**完全に同じ**（フレームに積む / fastcall なら FC_FASTCALL_REG）。加えて

```
FC_FARCALL: .res 3         ; base.asm の BSS に追加 (ZP に空きが無いプロジェクトがあるので絶対アドレス)。+0,+1 = 呼び先アドレス、+2 = バンク番号 (.bank)
```

にセットして、普通の関数なら `call farcall, #frame_size`（`call` マクロで X を進める）、fastcall なら `jsr farcall`。
戻り値はトランポリンが触らないので、呼び出し側のコードは呼び先の種類だけで決まる。

生成コード（普通の関数）:

```
	lda #<_bg_mmc__fetch_area
	sta FC_FARCALL+0
	lda #>_bg_mmc__fetch_area
	sta FC_FARCALL+1
	lda #<.bank(_bg_mmc__fetch_area)
	sta FC_FARCALL+2
	call farcall, #12
```

### 3.4 トランポリン `farcall`（ターゲット側が提供）

`runtime_init` と同じく、ターゲットごとに asm で用意する（fc は MMC3 用の参考実装を `fclib/nes/farcall_mmc3.asm` に置く。
emu ターゲットには「切替せず jsr するだけ」の実装を置く）。規約:

- 入力: `FC_FARCALL`（アドレス、バンク）。X（フレームポインタ）と FC_FASTCALL_REG を壊さない。A / Y は自由
- スロットは**呼び先アドレスの上位バイト**で決める（MMC3: $80〜$9F → slot 0、$A0〜$BF → slot 1）。コードはリンクした番地でしか
  動かないので、これで一意に決まる。16KB 領域や特殊なモードは `bank = N` の値の規約でトランポリン側が解釈すればよい
  （fc は値を素通しするだけ）
- そのスロットの今のバンク（castle なら `mmc3.pbank_bak`）と比べて、同じなら `jmp (FC_FARCALL)`（呼び先の `rts` が呼び出し元へ戻る）
- 違えば今のバンクをハードウェアスタックに退避 → 切替 → `jsr` で間接ジャンプ → 復帰 → `rts`。退避がスタックなので入れ子も動く
- IRQ ハンドラもバンクを切るなら、レジスタ書き込みの前後を `sei` / `cli` で囲む（`set_pbank` と同じ）

MMC3 用の参考実装（castle の `mmc3.fc` の `pbank_bak` / `BANK_SELECT` を使う）:

```
	.import FC_FARCALL
	.import _mmc3_pbank_bak
	.export farcall
farcall:
	lda FC_FARCALL+1
	cmp #$A0
	bcs @slot1
	ldy #0                  ; slot 0: pbank_bak+0, BANK_SELECT = 6
	.byte $2C               ; bit abs (次の ldy #1 を飛ばす)
@slot1:
	ldy #1                  ; slot 1: pbank_bak+1, BANK_SELECT = 7
	lda _mmc3_pbank_bak,y
	cmp FC_FARCALL+2
	bne @switch
	jmp (FC_FARCALL)       ; 既にマップ済み: そのまま飛ぶ
@switch:
	pha                     ; 今のバンクを退避
	tya
	pha                     ; スロットも退避
	lda FC_FARCALL+2
	sta _mmc3_pbank_bak,y
	sei
	tya
	clc
	adc #6
	sta $8000
	lda FC_FARCALL+2
	sta $8001
	cli
	jsr @indirect
	pla                     ; スロット
	tay
	pla                     ; 元のバンク
	sta _mmc3_pbank_bak,y
	sei
	pha
	tya
	clc
	adc #6
	sta $8000
	pla
	sta $8001
	cli
	rts
@indirect:
	jmp (FC_FARCALL)
```

### 3.5 対象外（今回）

- 関数ポインタ経由の呼び出し: 今までどおり `jsr_reg`（バンクは呼ぶ側の責任）。`.bank` はデータにも書けるので、
  後で 3 バイトの far ポインタ型（`farfn`: アドレス + バンク）を足せば `en_vtbl` の process 表も far 化できる
- 他バンクの**データ**参照: 今までどおり `set_pbank` で切り替えて読む
- 割込みハンドラからの far call: ハンドラは asm のまま

## 4. オプションと診断

- `options(bank: N)`（モジュール）: 既存。意味に「0 以上なら切替バンク」が加わる（数字は生成 cfg の placement にだけ使う）
- `options(near: true)`（関数）: 呼ぶ側に near call を許す opt-out
- `fcc build` / `check` の末尾に far call の箇所数をモジュールごとに出せるようにする（`-d` のとき）。熱い経路の確認用
- `farcall` が無ければリンクエラー（`Unresolved external 'farcall'`）。emu 既定は fc が用意するので出ない

## 5. コスト（MMC3 参考実装、jsr/rts 込みの追加分）

| 経路 | 追加サイクル |
|---|---|
| near（今までどおり） | 0 |
| far・切替不要（そのスロットに既に入っている） | 約 37（呼び出し側の FC_FARCALL セット 15 + 判定 17 + 間接 jmp 5） |
| far・切替あり | 約 100（退避・切替・復帰・`rts` を含む） |

今の手書きラッパー（`set_pbank` ×2 + ラッパー関数）は 110〜130 サイクルなので、切替ありでも今より軽い。
1 フレーム 29,780 サイクルに対し、敵 8 体 × 100 = 800（2.7%）。エンティティ単位のディスパッチには十分、
タイル単位の内側ループには向かない（そこは `near` か、同じモジュールにまとめる）。

## 6. castle での移行

1. `ld65.cfg` の切替 ROM（ROM0〜ROM19 など）に `bank = N` を付ける（N は `PBANK_*` と同じ値）
2. `farcall` を用意する（参考実装をコピーするか `src/mmc3.asm` などに置いて `include`）
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
