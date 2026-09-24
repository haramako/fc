;;; farcall (MMC3 用の参考実装。doc/v2_farcall.md §3.4)
;;;
;;; プロジェクトにコピーして、mmc3 モジュールから include("farcall_mmc3.asm") する。
;;; 前提: fc の mmc3 モジュールが pbank_bak:[2]int (各スロットの今のバンク) を持ち、
;;;       BANK_SELECT ($8000) / BANK_DATA ($8001) で切り替える (castle の src/mmc3.fc と同じ)。
;;;
;;; 入力: FC_FARCALL+0,1 = 呼び先アドレス、+2 = バンク番号 (ld65.cfg の bank = N)
;;; 規約: X (フレームポインタ) と FC_FASTCALL_REG を壊さない。A / Y は自由。
;;; スロットは呼び先アドレスの上位バイトで決める ($80-$9F → slot 0、$A0-$BF → slot 1)。
;;; そのスロットに既に同じバンクが入っていれば切り替えずに飛ぶ (+37 サイクル)。違えば今のバンクを
;;; ハードウェアスタックに退避して切り替え、戻ってから復帰する (+100 サイクル程度)。入れ子も可。

	.import FC_FARCALL
	.global _mmc3_pbank_bak         ; mmc3 モジュールから include するので .import でなく .global (定義側なら export になる)
	.global _mmc3_select_shadow     ; $8000 に書く値のシャドウ (macro.asm の規則。割り込みが出口で $8000 を戻すのに使う)
	.global farcall

farcall:
	lda FC_FARCALL+1
	cmp #$C0
	bcs @fixed              ; $C000 以上は固定バンク: 切り替えずに飛ぶ (segment 指定で固定領域に置いた関数など)
	cmp #$A0
	bcs @slot1
	ldy #0                  ; slot 0: pbank_bak+0, BANK_SELECT = 6
	.byte $2C               ; bit abs (次の ldy #1 を飛ばす)
@slot1:
	ldy #1                  ; slot 1: pbank_bak+1, BANK_SELECT = 7
	lda _mmc3_pbank_bak,y
	cmp FC_FARCALL+2
	bne @switch
@fixed:
	jmp (FC_FARCALL)        ; 既にマップ済み: そのまま飛ぶ (呼び先の rts が呼び出し元へ戻る)
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
	sta _mmc3_select_shadow ; ハードより先にシャドウ
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
	sta _mmc3_select_shadow ; ハードより先にシャドウ
	sta $8000
	pla
	sta $8001
	cli
	rts
@indirect:
	jmp (FC_FARCALL)
